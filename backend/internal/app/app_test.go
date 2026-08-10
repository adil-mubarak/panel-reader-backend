package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"database/sql"
	_ "modernc.org/sqlite"
)

func TestNaturalLess(t *testing.T) {
	names := []string{"10.jpg", "3.jpg", "1.jpg", "2.jpg"}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if naturalLess(names[j], names[i]) {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	want := []string{"1.jpg", "2.jpg", "3.jpg", "10.jpg"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("natural order = %v, want %v", names, want)
		}
	}
}

func TestImportTimeoutConfiguration(t *testing.T) {
	for _, test := range []struct {
		name       string
		configured time.Duration
		want       time.Duration
	}{
		{name: "default", want: 2 * time.Hour},
		{name: "configured", configured: 45 * time.Minute, want: 45 * time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := t.TempDir()
			a, err := New(Config{
				StorageRoot:   storage,
				DatabasePath:  filepath.Join(storage, "test.db"),
				ImportTimeout: test.configured,
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if a.config.ImportTimeout != test.want {
				t.Fatalf("ImportTimeout = %v, want %v", a.config.ImportTimeout, test.want)
			}
		})
	}
}

func TestImportDeadlineErrorIncludesPhase(t *testing.T) {
	got := importDeadlineError("detecting panels on page 47 of 120").Error()
	want := "Import exceeded the processing time limit while detecting panels on page 47 of 120."
	if got != want {
		t.Fatalf("importDeadlineError() = %q, want %q", got, want)
	}
}

func TestSafeArchivePath(t *testing.T) {
	for _, path := range []string{"../page.jpg", "/page.jpg", "pages/../../page.jpg", `pages\page.jpg`, ""} {
		if _, ok := safeArchivePath(path); ok {
			t.Errorf("safeArchivePath(%q) accepted an unsafe path", path)
		}
	}
	for _, path := range []string{"page.jpg", "chapter-1/001.png"} {
		if _, ok := safeArchivePath(path); !ok {
			t.Errorf("safeArchivePath(%q) rejected a safe path", path)
		}
	}
}

func TestUploadComicAndReadPages(t *testing.T) {
	storage := t.TempDir()
	a, err := New(Config{
		StorageRoot:  storage,
		DatabasePath: filepath.Join(storage, "test.db"),
		MaxUpload:    10 << 20,
		MaxEntries:   20,
		MaxExtracted: 10 << 20,
		MaxFile:      2 << 20,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	archive := comicArchive(t, []string{"10.png", "2.png", "1.png"})
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	if err := writer.WriteField("content_type", "manga"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "Example.cbz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/comics", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("upload status = %d, body = %s", response.Code, response.Body.String())
	}
	var comic Comic
	if err := json.Unmarshal(response.Body.Bytes(), &comic); err != nil {
		t.Fatal(err)
	}
	if comic.Status != "processing" || comic.Progress != 0 || comic.Phase != "queued" {
		t.Fatalf("initial comic = %#v", comic)
	}
	if comic.ContentType != "manga" || comic.ReadingDirection != "rtl" || comic.DefaultReadingMode != "panel" {
		t.Fatalf("comic metadata = %#v", comic)
	}
	comic = waitForComic(t, a, comic.ID, "ready")
	if comic.PageCount != 3 || comic.Progress != 100 || comic.Phase != "ready" {
		t.Fatalf("completed comic = %#v", comic)
	}

	pagesResponse := httptest.NewRecorder()
	a.Handler().ServeHTTP(pagesResponse, httptest.NewRequest(http.MethodGet, "/api/v1/comics/"+comic.ID+"/pages", nil))
	if pagesResponse.Code != http.StatusOK {
		t.Fatalf("pages status = %d", pagesResponse.Code)
	}
	var pages []Page
	if err := json.Unmarshal(pagesResponse.Body.Bytes(), &pages); err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 || pages[0].Number != 1 {
		t.Fatalf("unexpected pages: %#v", pages)
	}
	if len(pages[0].Panels) != 1 || pages[0].Panels[0].Source != "detected" || pages[0].Panels[0].FrameType != "full_page" || pages[0].Panels[0].Order != 1 {
		t.Fatalf("detected panels = %#v", pages[0].Panels)
	}
	for _, panel := range pages[0].Panels {
		if panel.X < 0 || panel.Y < 0 || panel.Width <= 0 || panel.Height <= 0 || panel.X+panel.Width > 1 || panel.Y+panel.Height > 1 {
			t.Fatalf("generated panel is outside normalized bounds: %#v", panel)
		}
	}

	invalidBody := bytes.NewBufferString(`[{"order":0,"x":0,"y":0,"width":1,"height":0.6,"source":"manual"},{"order":0,"x":0,"y":0.4,"width":1,"height":0.6,"source":"manual"}]`)
	invalidResponse := httptest.NewRecorder()
	a.Handler().ServeHTTP(invalidResponse, httptest.NewRequest(http.MethodPut, "/api/v1/comics/"+comic.ID+"/pages/1/panels", invalidBody))
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid panels status = %d, body = %s", invalidResponse.Code, invalidResponse.Body.String())
	}
	var persisted int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM panels f JOIN pages p ON p.id=f.page_id WHERE p.comic_id=? AND p.number=1`, comic.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != len(pages[0].Panels) {
		t.Fatalf("invalid PUT changed panels: count = %d, want %d", persisted, len(pages[0].Panels))
	}

	polygon := defaultFrame(1)
	polygon.Name = "Polygon"
	polygon.ShapeType = "polygon"
	polygon.Source = "manual"
	polygon.X, polygon.Y, polygon.Width, polygon.Height = 0, 0, 0, 0
	polygon.Polygon = []Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}}
	putBody, _ := json.Marshal(replaceFramesRequest{Revision: 0, Frames: []Panel{polygon}})
	putResponse := httptest.NewRecorder()
	a.Handler().ServeHTTP(putResponse, httptest.NewRequest(http.MethodPut, "/api/v1/comics/"+comic.ID+"/pages/1/frames", bytes.NewReader(putBody)))
	if putResponse.Code != http.StatusOK {
		t.Fatalf("rich frames status = %d, body = %s", putResponse.Code, putResponse.Body.String())
	}
	var saved framesResponse
	if err := json.Unmarshal(putResponse.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 || len(saved.Frames) != 1 || saved.Frames[0].ShapeType != "polygon" {
		t.Fatalf("saved rich frames = %#v", saved)
	}
	conflictResponse := httptest.NewRecorder()
	a.Handler().ServeHTTP(conflictResponse, httptest.NewRequest(http.MethodPut, "/api/v1/comics/"+comic.ID+"/pages/1/frames", bytes.NewReader(putBody)))
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("stale revision status = %d, body = %s", conflictResponse.Code, conflictResponse.Body.String())
	}
	polygon.Polygon = polygon.Polygon[:2]
	invalidPolygonBody, _ := json.Marshal(replaceFramesRequest{Revision: 1, Frames: []Panel{polygon}})
	invalidPolygonResponse := httptest.NewRecorder()
	a.Handler().ServeHTTP(invalidPolygonResponse, httptest.NewRequest(http.MethodPut, "/api/v1/comics/"+comic.ID+"/pages/1/frames", bytes.NewReader(invalidPolygonBody)))
	if invalidPolygonResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid polygon status = %d, body = %s", invalidPolygonResponse.Code, invalidPolygonResponse.Body.String())
	}
	detectConflict := httptest.NewRecorder()
	a.Handler().ServeHTTP(detectConflict, httptest.NewRequest(http.MethodPost, "/api/v1/comics/"+comic.ID+"/pages/1/detect", nil))
	if detectConflict.Code != http.StatusConflict {
		t.Fatalf("manual detection status = %d, body = %s", detectConflict.Code, detectConflict.Body.String())
	}
	var preservedSource string
	var preservedRevision int
	if err := a.db.QueryRow(`SELECT f.source,p.revision FROM panels f JOIN pages p ON p.id=f.page_id WHERE p.comic_id=? AND p.number=1`, comic.ID).Scan(&preservedSource, &preservedRevision); err != nil {
		t.Fatal(err)
	}
	if preservedSource != "manual" || preservedRevision != 1 {
		t.Fatalf("manual frame changed after conflict: source=%q revision=%d", preservedSource, preservedRevision)
	}
	detectReset := httptest.NewRecorder()
	a.Handler().ServeHTTP(detectReset, httptest.NewRequest(http.MethodPost, "/api/v1/comics/"+comic.ID+"/pages/1/detect?reset=true", nil))
	if detectReset.Code != http.StatusOK {
		t.Fatalf("reset detection status = %d, body = %s", detectReset.Code, detectReset.Body.String())
	}
	var reset framesResponse
	if err := json.Unmarshal(detectReset.Body.Bytes(), &reset); err != nil {
		t.Fatal(err)
	}
	if reset.Revision != 2 || len(reset.Frames) != 1 || reset.Frames[0].Source != "detected" {
		t.Fatalf("reset detection = %#v", reset)
	}

	imageResponse := httptest.NewRecorder()
	a.Handler().ServeHTTP(imageResponse, httptest.NewRequest(http.MethodGet, "/api/v1/comics/"+comic.ID+"/pages/1/image", nil))
	if imageResponse.Code != http.StatusOK {
		t.Fatalf("image status = %d", imageResponse.Code)
	}
	if imageResponse.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("content type = %q", imageResponse.Header().Get("Content-Type"))
	}
}

func TestDetectPanelsFindsGridGutters(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.White)
		}
	}
	regions := []image.Rectangle{
		image.Rect(5, 5, 190, 140), image.Rect(210, 5, 395, 140),
		image.Rect(5, 160, 190, 295), image.Rect(210, 160, 395, 295),
	}
	colors := []color.Color{color.Black, color.RGBA{R: 180, A: 255}, color.RGBA{G: 150, A: 255}, color.RGBA{B: 180, A: 255}}
	for i, region := range regions {
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				img.Set(x, y, colors[i])
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	frames, err := detectPanels(t.Context(), bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 4 {
		t.Fatalf("detected %d frames, want 4: %#v", len(frames), frames)
	}
	if !(frames[0].X < frames[1].X && frames[0].Y < frames[2].Y && frames[2].X < frames[3].X) {
		t.Fatalf("frames are not in row-major LTR order: %#v", frames)
	}
}

func TestDetectPanelsFallsBackWithoutGutter(t *testing.T) {
	img := image.NewUniform(color.Black)
	frames, err := detectPanels(t.Context(), imageReader(t, img, image.Rect(0, 0, 300, 400)))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].FrameType != "full_page" || frames[0].Width != 1 || frames[0].Height != 1 {
		t.Fatalf("fallback = %#v", frames)
	}
}

func imageReader(t *testing.T, source image.Image, bounds image.Rectangle) io.Reader {
	t.Helper()
	img := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, source.At(x, y))
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(encoded.Bytes())
}

func TestGeneratedPanelsFollowPageOrientation(t *testing.T) {
	portrait := generatedPanels(800, 1600)
	if len(portrait) < 2 || portrait[0].Width != 1 || portrait[0].Y >= portrait[1].Y || portrait[0].Y+portrait[0].Height <= portrait[1].Y {
		t.Fatalf("portrait panels are not overlapping horizontal bands: %#v", portrait)
	}
	landscape := generatedPanels(1600, 800)
	if len(landscape) < 2 || landscape[0].Height != 1 || landscape[0].X >= landscape[1].X || landscape[0].X+landscape[0].Width <= landscape[1].X {
		t.Fatalf("landscape panels are not overlapping left-to-right regions: %#v", landscape)
	}
}

func TestMigrateExistingDatabaseAddsProgressColumns(t *testing.T) {
	storage := t.TempDir()
	databasePath := filepath.Join(storage, "test.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE comics (id TEXT PRIMARY KEY, title TEXT NOT NULL, status TEXT NOT NULL, page_count INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL); CREATE TABLE pages (id INTEGER PRIMARY KEY, comic_id TEXT NOT NULL, number INTEGER NOT NULL, image_path TEXT NOT NULL, width INTEGER NOT NULL, height INTEGER NOT NULL, media_type TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO comics(id,title,status,page_count,created_at) VALUES('old','Old','ready',0,?)`, created); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{StorageRoot: storage, DatabasePath: databasePath, MaxUpload: 1 << 20, MaxEntries: 20, MaxExtracted: 1 << 20, MaxFile: 1 << 20}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	comic, err := a.comic(t.Context(), "old")
	if err != nil {
		t.Fatal(err)
	}
	if comic.Progress != 100 || comic.Phase != "ready" || comic.ContentType != "comic" || comic.ReadingDirection != "ltr" || comic.DefaultReadingMode != "panel" {
		t.Fatalf("migrated comic = %#v", comic)
	}
}

func waitForComic(t *testing.T, a *App, id, status string) Comic {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response := httptest.NewRecorder()
		a.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/comics/"+id, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("comic status = %d, body = %s", response.Code, response.Body.String())
		}
		var comic Comic
		if err := json.Unmarshal(response.Body.Bytes(), &comic); err != nil {
			t.Fatal(err)
		}
		if comic.Status == status {
			return comic
		}
		if comic.Status == "failed" {
			t.Fatalf("comic import failed: %s", comic.ErrorMessage)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("comic %s did not reach %s", id, status)
	return Comic{}
}

func TestExtractRejectsTraversal(t *testing.T) {
	storage := t.TempDir()
	a, err := New(Config{StorageRoot: storage, DatabasePath: filepath.Join(storage, "test.db"), MaxUpload: 1 << 20, MaxEntries: 20, MaxExtracted: 1 << 20, MaxFile: 1 << 20}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	archivePath := filepath.Join(storage, "bad.cbz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape.png")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write(pngImage(t))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := a.extractCBZ(archivePath, filepath.Join(storage, "pages"), "comic_test"); err == nil {
		t.Fatal("expected traversal archive to be rejected")
	}
	if _, err := os.Stat(filepath.Join(storage, "escape.png")); !os.IsNotExist(err) {
		t.Fatal("archive escaped destination")
	}
}

func TestInvalidCBRIsRejected(t *testing.T) {
	storage := t.TempDir()
	a, err := New(Config{StorageRoot: storage, DatabasePath: filepath.Join(storage, "test.db"), MaxUpload: 1 << 20, MaxEntries: 20, MaxExtracted: 1 << 20, MaxFile: 1 << 20}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	archivePath := filepath.Join(storage, "invalid.cbr")
	if err := os.WriteFile(archivePath, []byte("not a rar archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.extractCBR(archivePath, filepath.Join(storage, "pages"), "comic_test"); err == nil {
		t.Fatal("expected invalid CBR to be rejected")
	}
}

func TestUnsupportedUploadTypeIsRejected(t *testing.T) {
	storage := t.TempDir()
	a, err := New(Config{StorageRoot: storage, DatabasePath: filepath.Join(storage, "test.db"), MaxUpload: 1 << 20, MaxEntries: 20, MaxExtracted: 1 << 20, MaxFile: 1 << 20}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("not a comic")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/comics", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("upload status = %d, want %d", response.Code, http.StatusUnsupportedMediaType)
	}
}

func comicArchive(t *testing.T, names []string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(pngImage(t)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func pngImage(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.White)
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
