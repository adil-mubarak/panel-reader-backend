package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExternalDetectorValidResponseAndOrdering(t *testing.T) {
	var request externalDetectionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/panel-detection" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"width":1000,"height":1500,"modelVersion":"seg-v2","panels":[{"confidence":0.7,"boundingBox":{"x":0.55,"y":0.1,"width":0.35,"height":0.3}},{"confidence":0.9,"polygon":[{"x":0.1,"y":0.1},{"x":0.45,"y":0.1},{"x":0.4,"y":0.4}],"boundingBox":{"x":0.1,"y":0.1,"width":0.35,"height":0.3}},{"confidence":0.8,"boundingBox":{"x":0.1,"y":0.55,"width":0.8,"height":0.35}}]}`)
	}))
	defer server.Close()

	a, imagePath := detectorTestApp(t, server.URL)
	frames, err := a.detectPanelsFile(t.Context(), imagePath, "comic-1", 7, "ltr", "comic")
	if err != nil {
		t.Fatal(err)
	}
	if request.ComicID != "comic-1" || request.Page != 7 || request.ImagePath != imagePath || request.ReadingDirection != "ltr" || request.ContentType != "comic" {
		t.Fatalf("request = %#v", request)
	}
	if len(frames) != 3 || frames[0].X != 0.1 || frames[1].X != 0.55 || frames[2].Y != 0.55 {
		t.Fatalf("LTR ordering = %#v", frames)
	}
	if frames[0].ShapeType != "polygon" || frames[0].Confidence != 0.9 || !strings.HasPrefix(frames[0].ModelVersion, "hybrid/roboflow/seg-v2;") {
		t.Fatalf("rich polygon = %#v", frames[0])
	}
	for i := range frames {
		if frames[i].Order != i+1 {
			t.Fatalf("orders = %#v", frames)
		}
	}
	sortDetectedPanels(frames, "rtl")
	if frames[0].X != 0.55 || frames[1].X != 0.1 || frames[2].Y != 0.55 {
		t.Fatalf("RTL ordering = %#v", frames)
	}
}

func TestExternalDetectorSendsMangaContentTypeAndRTLDirection(t *testing.T) {
	var request externalDetectionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, `{"width":1000,"height":1500,"modelVersion":"seg-v2","panels":[]}`)
	}))
	defer server.Close()

	a, imagePath := detectorTestApp(t, server.URL)
	if _, err := a.detectPanelsExternal(t.Context(), imagePath, "manga-1", 3, "rtl", "manga"); err != nil {
		t.Fatal(err)
	}
	if request.ContentType != "manga" || request.ReadingDirection != "rtl" {
		t.Fatalf("request = %#v", request)
	}
}

func TestExternalDetectorRejectsInvalidContentType(t *testing.T) {
	a, imagePath := detectorTestApp(t, "http://127.0.0.1:1")
	if _, err := a.detectPanelsExternal(t.Context(), imagePath, "comic", 1, "ltr", "novel"); err == nil || err.Error() != "invalid content type" {
		t.Fatalf("error = %v", err)
	}
}

func TestExternalDetectorInvalidCoordinateFallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"width":2,"height":3,"modelVersion":"bad","panels":[{"confidence":0.9,"boundingBox":{"x":0.8,"y":0,"width":0.3,"height":1}}]}`)
	}))
	defer server.Close()
	a, imagePath := detectorTestApp(t, server.URL)
	frames, err := a.detectPanelsFile(t.Context(), imagePath, "comic", 1, "ltr", "comic")
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].FrameType != "full_page" || frames[0].ModelVersion != "structural/v1" {
		t.Fatalf("fallback = %#v", frames)
	}
}

func TestExternalDetectorUnavailableFallsBack(t *testing.T) {
	a, imagePath := detectorTestApp(t, "http://127.0.0.1:1")
	frames, err := a.detectPanelsFile(t.Context(), imagePath, "comic", 1, "ltr", "comic")
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].FrameType != "full_page" {
		t.Fatalf("fallback = %#v", frames)
	}
}

func detectorTestApp(t *testing.T, detectorURL string) (*App, string) {
	t.Helper()
	storage := t.TempDir()
	imagePath := filepath.Join(storage, "page.png")
	if err := writeTestImage(imagePath, pngImage(t)); err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{StorageRoot: storage, DatabasePath: filepath.Join(storage, "test.db"), MaxUpload: 1 << 20, MaxEntries: 20, MaxExtracted: 1 << 20, MaxFile: 1 << 20, PanelDetectorURL: detectorURL, PanelDetectorTimeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a, imagePath
}

func writeTestImage(path string, contents []byte) error {
	return os.WriteFile(path, contents, 0o600)
}
