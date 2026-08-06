package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPostprocessDetectionsRemovesDuplicatesButKeepsFocus(t *testing.T) {
	panel := defaultFrame(1)
	panel.X, panel.Y, panel.Width, panel.Height, panel.Confidence = .1, .1, .5, .5, .9
	duplicate := panel
	duplicate.X, duplicate.Y, duplicate.Confidence = .11, .11, .8
	focus := panel
	focus.FrameType, focus.Width, focus.Height = "focus", .2, .2
	got := postprocessDetections([]Panel{duplicate, focus, panel}, "ltr", .25, true)
	if len(got) != 2 || (got[0].FrameType != "focus" && got[1].FrameType != "focus") {
		t.Fatalf("postprocessed frames = %#v", got)
	}
}

func TestDetectionReportWarnings(t *testing.T) {
	f := defaultFrame(1)
	f.X, f.Y, f.Width, f.Height, f.Confidence = .1, .1, .2, .2, .3
	report := buildDetectionReport([]Panel{f})
	if !containsWarning(report.Warnings, "low_confidence") || !containsWarning(report.Warnings, "large_uncovered_area") {
		t.Fatalf("warnings = %v", report.Warnings)
	}
	if !containsWarning(buildDetectionReport(nil).Warnings, "no_panels") {
		t.Fatal("missing no_panels warning")
	}
}

func TestApprovalAndYOLOExport(t *testing.T) {
	storage := t.TempDir()
	a, err := New(Config{StorageRoot: storage, DatabasePath: filepath.Join(storage, "test.db")}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	dir := filepath.Join(storage, "comics", "test", "pages")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0001.png"), pngImage(t), 0o640); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = a.db.Exec(`INSERT INTO comics(id,title,status,page_count,created_at) VALUES('test','Test','ready',1,?); INSERT INTO pages(comic_id,number,image_path,width,height,media_type) VALUES('test',1,'comics/test/pages/0001.png',2,3,'image/png')`, now); err != nil {
		t.Fatal(err)
	}
	var pageID int64
	if err = a.db.QueryRow(`SELECT id FROM pages WHERE comic_id='test'`).Scan(&pageID); err != nil {
		t.Fatal(err)
	}
	f := defaultFrame(1)
	f.Width, f.Height = 1, 1
	if err = insertFrame(t.Context(), a.db, pageID, &f); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/comics/test/pages/1/approve", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", response.Code, response.Body.String())
	}
	export := httptest.NewRecorder()
	a.Handler().ServeHTTP(export, httptest.NewRequest(http.MethodGet, "/api/v1/comics/test/training-export?format=yolo", nil))
	if export.Code != http.StatusOK {
		t.Fatalf("export: %d %s", export.Code, export.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(export.Body.Bytes()), int64(export.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range zr.File {
		if entry.Name == "labels/page-0001.txt" {
			found = true
		}
	}
	if !found {
		names, _ := json.Marshal(zr.File)
		t.Fatalf("YOLO label missing: %s", names)
	}
}

func containsWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}
