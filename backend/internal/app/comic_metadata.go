package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func comicReadingMetadata(contentType string) (direction, mode string, ok bool) {
	switch contentType {
	case "comic":
		return "ltr", "panel", true
	case "manga":
		return "rtl", "panel", true
	case "webtoon":
		return "vertical", "vertical", true
	default:
		return "", "", false
	}
}

type putComicRequest struct {
	ContentType string `json:"contentType"`
}

func (a *App) putComic(w http.ResponseWriter, r *http.Request) {
	var request putComicRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_comic", "contentType is required.")
		return
	}
	direction, mode, ok := comicReadingMetadata(request.ContentType)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_content_type", "contentType must be comic, manga, or webtoon.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "database_error", "Could not update comic metadata.")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `UPDATE comics SET content_type=?,reading_direction=?,default_reading_mode=? WHERE id=?`, request.ContentType, direction, mode, chi.URLParam(r, "comicID"))
	if err == nil {
		var changed int64
		changed, err = result.RowsAffected()
		if changed == 0 {
			writeError(w, http.StatusNotFound, "not_found", "Comic not found.")
			return
		}
	}
	if err == nil {
		rows, queryErr := tx.QueryContext(r.Context(), `SELECT p.id FROM pages p JOIN panels f ON f.page_id=p.id WHERE p.comic_id=? GROUP BY p.id HAVING COUNT(*)=SUM(CASE WHEN f.source='detected' THEN 1 ELSE 0 END)`, chi.URLParam(r, "comicID"))
		err = queryErr
		var pageIDs []int64
		if err == nil {
			for rows.Next() {
				var id int64
				if scanErr := rows.Scan(&id); scanErr != nil {
					err = scanErr
					break
				}
				pageIDs = append(pageIDs, id)
			}
			err = errors.Join(err, rows.Err(), rows.Close())
		}
		for _, pageID := range pageIDs {
			if err != nil {
				break
			}
			var frames []Panel
			frames, err = loadFrames(r.Context(), tx, pageID)
			if err != nil {
				break
			}
			sortDetectedPanels(frames, direction)
			_, err = tx.ExecContext(r.Context(), `UPDATE panels SET "order"="order"+100000 WHERE page_id=?`, pageID)
			for _, frame := range frames {
				if err != nil {
					break
				}
				_, err = tx.ExecContext(r.Context(), `UPDATE panels SET "order"=?,name=? WHERE id=?`, frame.Order, frame.Name, frame.ID)
			}
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not update comic metadata.")
		return
	}
	comic, err := a.comic(r.Context(), chi.URLParam(r, "comicID"))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, 500, "database_error", "Could not load comic.")
		return
	}
	writeJSON(w, http.StatusOK, comic)
}
