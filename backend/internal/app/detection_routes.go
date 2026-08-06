package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

var errManualFrames = errors.New("page contains manual frames")

type detectedPageResponse struct {
	Number   int     `json:"number"`
	Revision int     `json:"revision"`
	Frames   []Panel `json:"frames"`
}

type detectComicResponse struct {
	Pages   []detectedPageResponse `json:"pages"`
	Skipped []int                  `json:"skipped"`
}

func (a *App) detectPage(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(chi.URLParam(r, "pageNumber"))
	if err != nil || number < 1 {
		writeError(w, http.StatusBadRequest, "invalid_page", "Page number is invalid.")
		return
	}
	pageID, revision, path, direction, contentType, err := a.pageDetectionInfo(r.Context(), chi.URLParam(r, "comicID"), number)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Page not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "Could not load page.")
		return
	}
	frames, err := a.detectPanelsFile(r.Context(), path, chi.URLParam(r, "comicID"), number, direction, contentType)
	if err == nil {
		revision, err = a.replaceDetectedFrames(r.Context(), pageID, revision, frames, queryReset(r))
	}
	if errors.Is(err, errManualFrames) {
		writeError(w, http.StatusConflict, "manual_frames", "Page contains manual or manually edited frames; use reset=true to replace them.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "detection_error", "Could not detect panels.")
		return
	}
	report := buildDetectionReport(frames)
	writeJSON(w, http.StatusOK, framesResponse{Revision: revision, Frames: frames, ReviewStatus: reviewStatusForReport(report), DetectionReport: report})
}

func (a *App) detectComic(w http.ResponseWriter, r *http.Request) {
	id, reset := chi.URLParam(r, "comicID"), queryReset(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT p.id,p.number,p.image_path,p.revision,c.reading_direction,c.content_type FROM pages p JOIN comics c ON c.id=p.comic_id WHERE p.comic_id=? ORDER BY p.number`, id)
	if err != nil {
		writeError(w, 500, "database_error", "Could not load pages.")
		return
	}
	type info struct {
		id               int64
		number, revision int
		path             string
		direction        string
		contentType      string
	}
	var pages []info
	for rows.Next() {
		var p info
		err = rows.Scan(&p.id, &p.number, &p.path, &p.revision, &p.direction, &p.contentType)
		pages = append(pages, p)
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		writeError(w, 500, "database_error", "Could not load pages.")
		return
	}
	if len(pages) == 0 {
		if _, comicErr := a.comic(r.Context(), id); errors.Is(comicErr, sql.ErrNoRows) {
			writeError(w, 404, "not_found", "Comic not found.")
			return
		}
	}
	response := detectComicResponse{Pages: []detectedPageResponse{}, Skipped: []int{}}
	for _, p := range pages {
		path, pathErr := a.safeStoredPath(p.path)
		var frames []Panel
		detectionErr := pathErr
		if detectionErr == nil {
			frames, detectionErr = a.detectPanelsFile(r.Context(), path, id, p.number, p.direction, p.contentType)
		}
		if detectionErr == nil {
			p.revision, detectionErr = a.replaceDetectedFrames(r.Context(), p.id, p.revision, frames, reset)
		}
		if errors.Is(detectionErr, errManualFrames) {
			response.Skipped = append(response.Skipped, p.number)
			continue
		}
		if detectionErr != nil {
			writeError(w, 500, "detection_error", "Could not detect panels on every page.")
			return
		}
		response.Pages = append(response.Pages, detectedPageResponse{Number: p.number, Revision: p.revision, Frames: frames})
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) pageDetectionInfo(ctx context.Context, comicID string, number int) (int64, int, string, string, string, error) {
	var pageID int64
	var revision int
	var relative string
	var direction string
	var contentType string
	err := a.db.QueryRowContext(ctx, `SELECT p.id,p.revision,p.image_path,c.reading_direction,c.content_type FROM pages p JOIN comics c ON c.id=p.comic_id WHERE p.comic_id=? AND p.number=?`, comicID, number).Scan(&pageID, &revision, &relative, &direction, &contentType)
	if err != nil {
		return 0, 0, "", "", "", err
	}
	path, err := a.safeStoredPath(relative)
	return pageID, revision, path, direction, contentType, err
}

func (a *App) safeStoredPath(relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", errors.New("invalid stored page path")
	}
	path := filepath.Join(a.config.StorageRoot, filepath.Clean(relative))
	rel, err := filepath.Rel(a.config.StorageRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid stored page path")
	}
	return path, nil
}

func (a *App) replaceDetectedFrames(ctx context.Context, pageID int64, expectedRevision int, frames []Panel, reset bool) (int, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return expectedRevision, err
	}
	defer tx.Rollback()
	var revision, manual int
	err = tx.QueryRowContext(ctx, `SELECT p.revision,COUNT(CASE WHEN f.source IN ('manual','manual_edited') THEN 1 END) FROM pages p LEFT JOIN panels f ON f.page_id=p.id WHERE p.id=? GROUP BY p.id`, pageID).Scan(&revision, &manual)
	if err != nil {
		return revision, err
	}
	if revision != expectedRevision {
		return revision, errors.New("page revision changed during detection")
	}
	if manual > 0 && !reset {
		return revision, errManualFrames
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM panels WHERE page_id=?`, pageID); err != nil {
		return revision, err
	}
	for i := range frames {
		frames[i].ID = 0
		if err = insertFrame(ctx, tx, pageID, &frames[i]); err != nil {
			return revision, err
		}
	}
	report := buildDetectionReport(frames)
	reportJSON, _ := json.Marshal(report)
	if _, err = tx.ExecContext(ctx, `UPDATE pages SET revision=revision+1,review_status=?,detection_report_json=? WHERE id=?`, reviewStatusForReport(report), reportJSON, pageID); err != nil {
		return revision, err
	}
	return revision + 1, tx.Commit()
}

func queryReset(r *http.Request) bool { return strings.EqualFold(r.URL.Query().Get("reset"), "true") }
