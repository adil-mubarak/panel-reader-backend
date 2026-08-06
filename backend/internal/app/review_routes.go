package app

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (a *App) approvePage(w http.ResponseWriter, r *http.Request)   { a.setApproval(w, r, true) }
func (a *App) unapprovePage(w http.ResponseWriter, r *http.Request) { a.setApproval(w, r, false) }

func (a *App) setApproval(w http.ResponseWriter, r *http.Request, approve bool) {
	number, err := strconv.Atoi(chi.URLParam(r, "pageNumber"))
	if err != nil || number < 1 {
		writeError(w, 400, "invalid_page", "Page number is invalid.")
		return
	}
	var pageID int64
	var current, reportJSON string
	err = a.db.QueryRowContext(r.Context(), `SELECT id,review_status,detection_report_json FROM pages WHERE comic_id=? AND number=?`, chi.URLParam(r, "comicID"), number).Scan(&pageID, &current, &reportJSON)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Page not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not update page review status.")
		return
	}
	status := current
	if approve {
		var valid int
		err = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM panels WHERE page_id=? AND is_enabled=1 AND ((shape_type='rectangle' AND width>0 AND height>0) OR (shape_type='polygon' AND json_array_length(polygon_json)>=3))`, pageID).Scan(&valid)
		if err == nil && valid == 0 {
			writeError(w, 400, "invalid_frames", "At least one enabled valid frame is required.")
			return
		}
		status = "approved"
	} else {
		var report DetectionReport
		_ = json.Unmarshal([]byte(reportJSON), &report)
		status = reviewStatusForReport(report)
		var manual int
		_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM panels WHERE page_id=? AND source IN ('manual','manual_edited')`, pageID).Scan(&manual)
		if manual > 0 {
			status = "manually_corrected"
		}
	}
	if err == nil {
		_, err = a.db.ExecContext(r.Context(), `UPDATE pages SET review_status=? WHERE id=?`, status, pageID)
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not update page review status.")
		return
	}
	writeJSON(w, 200, map[string]any{"page": number, "reviewStatus": status})
}

type exportPage struct {
	ID                    int64
	Number, Width, Height int
	Path, MediaType       string
}

func (a *App) trainingExport(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format != "yolo" && format != "coco" {
		writeError(w, 400, "invalid_format", "format must be yolo or coco.")
		return
	}
	comicID := chi.URLParam(r, "comicID")
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,number,width,height,image_path,media_type FROM pages WHERE comic_id=? AND review_status='approved' ORDER BY number`, comicID)
	if err != nil {
		writeError(w, 500, "database_error", "Could not export training data.")
		return
	}
	var pages []exportPage
	for rows.Next() {
		var p exportPage
		err = rows.Scan(&p.ID, &p.Number, &p.Width, &p.Height, &p.Path, &p.MediaType)
		pages = append(pages, p)
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		writeError(w, 500, "database_error", "Could not export training data.")
		return
	}
	if len(pages) == 0 {
		writeError(w, 404, "no_approved_pages", "No approved pages are available for export.")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s-training.zip"`, comicID, format))
	zw := zip.NewWriter(w)
	manifest := map[string]any{"format": format, "comicId": comicID, "normalization": "coordinates are normalized to [0,1]", "pages": []any{}}
	var cocoImages, cocoAnnotations []map[string]any
	annotationID := 1
	for _, p := range pages {
		frames, loadErr := loadFrames(r.Context(), a.db, p.ID)
		if loadErr != nil {
			err = loadErr
			break
		}
		ext := filepath.Ext(p.Path)
		imageName := fmt.Sprintf("images/page-%04d%s", p.Number, ext)
		path, pathErr := a.safeStoredPath(p.Path)
		if pathErr != nil {
			err = pathErr
			break
		}
		src, openErr := os.Open(path)
		if openErr != nil {
			err = openErr
			break
		}
		dst, createErr := zw.Create(imageName)
		if createErr == nil {
			_, createErr = io.Copy(dst, src)
		}
		src.Close()
		if createErr != nil {
			err = createErr
			break
		}
		manifest["pages"] = append(manifest["pages"].([]any), map[string]any{"page": p.Number, "image": imageName, "width": p.Width, "height": p.Height})
		if format == "yolo" {
			var lines []string
			for _, f := range frames {
				if !f.IsEnabled {
					continue
				}
				points := framePolygon(f)
				var fields = []string{"0"}
				for _, pt := range points {
					fields = append(fields, strconv.FormatFloat(pt.X, 'f', 6, 64), strconv.FormatFloat(pt.Y, 'f', 6, 64))
				}
				lines = append(lines, strings.Join(fields, " "))
			}
			entry, _ := zw.Create(fmt.Sprintf("labels/page-%04d.txt", p.Number))
			_, _ = io.WriteString(entry, strings.Join(lines, "\n")+"\n")
		} else {
			cocoImages = append(cocoImages, map[string]any{"id": p.Number, "file_name": imageName, "width": p.Width, "height": p.Height})
			for _, f := range frames {
				if !f.IsEnabled {
					continue
				}
				pts := framePolygon(f)
				segmentation := make([]float64, 0, len(pts)*2)
				for _, pt := range pts {
					segmentation = append(segmentation, pt.X*float64(p.Width), pt.Y*float64(p.Height))
				}
				cocoAnnotations = append(cocoAnnotations, map[string]any{"id": annotationID, "image_id": p.Number, "category_id": 1, "segmentation": [][]float64{segmentation}, "bbox": []float64{f.X * float64(p.Width), f.Y * float64(p.Height), f.Width * float64(p.Width), f.Height * float64(p.Height)}, "area": frameArea(f) * float64(p.Width*p.Height), "iscrowd": 0, "attributes": map[string]any{"source": f.Source, "confidence": f.Confidence, "modelVersion": f.ModelVersion}})
				annotationID++
			}
		}
	}
	if err == nil {
		if format == "coco" {
			entry, _ := zw.Create("annotations.json")
			_ = json.NewEncoder(entry).Encode(map[string]any{
				"images": cocoImages, "annotations": cocoAnnotations,
				"categories": []map[string]any{{"id": 1, "name": "panel"}},
			})
		}
		entry, _ := zw.Create("manifest.json")
		_ = json.NewEncoder(entry).Encode(manifest)
	}
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		a.logger.Warn("training export interrupted", "error", err)
	}
}

func framePolygon(f Panel) []Point {
	if f.ShapeType == "polygon" && len(f.Polygon) >= 3 {
		return f.Polygon
	}
	return []Point{{f.X, f.Y}, {f.X + f.Width, f.Y}, {f.X + f.Width, f.Y + f.Height}, {f.X, f.Y + f.Height}}
}
