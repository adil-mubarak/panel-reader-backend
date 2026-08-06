package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectedOrderingByReadingDirection(t *testing.T) {
	frames := []Panel{{X: .1, Y: .1, Width: .3, Height: .3}, {X: .6, Y: .1, Width: .3, Height: .3}, {X: .4, Y: .6, Width: .3, Height: .3}}
	sortDetectedPanels(frames, "rtl")
	if frames[0].X != .6 || frames[1].X != .1 {
		t.Fatalf("RTL order = %#v", frames)
	}
	frames = []Panel{{X: .6, Y: .2, Width: .2, Height: .2}, {X: .1, Y: .2, Width: .2, Height: .2}, {X: .8, Y: .1, Width: .1, Height: .1}}
	sortDetectedPanels(frames, "vertical")
	if frames[0].Y != .1 || frames[1].X != .1 || frames[2].X != .6 {
		t.Fatalf("vertical order = %#v", frames)
	}
}

func TestDetectedOrderingUsesStableRows(t *testing.T) {
	input := []Panel{
		{X: .58, Y: .12, Width: .32, Height: .24},
		{X: .08, Y: .1, Width: .38, Height: .28},
		{X: .55, Y: .55, Width: .35, Height: .3},
		{X: .1, Y: .53, Width: .35, Height: .32},
	}

	comic := append([]Panel(nil), input...)
	sortDetectedPanels(comic, "ltr")
	wantComic := []float64{.08, .58, .1, .55}
	for i, want := range wantComic {
		if comic[i].X != want || comic[i].Order != i+1 {
			t.Fatalf("comic frame %d = x %.2f order %d; want x %.2f order %d", i, comic[i].X, comic[i].Order, want, i+1)
		}
	}

	manga := append([]Panel(nil), input...)
	sortDetectedPanels(manga, "rtl")
	wantManga := []float64{.58, .08, .55, .1}
	for i, want := range wantManga {
		if manga[i].X != want || manga[i].Order != i+1 {
			t.Fatalf("manga frame %d = x %.2f order %d; want x %.2f order %d", i, manga[i].X, manga[i].Order, want, i+1)
		}
	}
}

func TestPostprocessConservativeFallbackAndShapes(t *testing.T) {
	splash := defaultFrame(1)
	splash.Width, splash.Height, splash.Confidence = .8, .8, .9
	got := postprocessDetections([]Panel{splash}, "ltr", .25, true)
	if len(got) != 1 || got[0].FrameType != "full_page" {
		t.Fatalf("splash = %#v", got)
	}
	a, b := defaultFrame(1), defaultFrame(2)
	a.X, a.Y, a.Width, a.Height, a.Confidence = 0, 0, .45, 1, .9
	b.X, b.Y, b.Width, b.Height, b.Confidence = .55, 0, .45, 1, .9
	got = postprocessDetections([]Panel{a, b}, "ltr", .25, true)
	if len(got) != 2 {
		t.Fatalf("clear panels collapsed = %#v", got)
	}
	rect := a
	rect.ShapeType = "polygon"
	rect.Polygon = []Point{{0, 0}, {.45, 0}, {.45, 1}, {0, 1}}
	got = postprocessDetections([]Panel{rect, b}, "ltr", .25, true)
	if got[0].ShapeType != "rectangle" || len(got[0].Polygon) != 0 {
		t.Fatalf("rectangular polygon = %#v", got[0])
	}
	bow := a
	bow.ShapeType = "polygon"
	bow.Polygon = []Point{{0, 0}, {.45, 1}, {0, 1}, {.45, 0}}
	got = postprocessDetections([]Panel{bow, b}, "ltr", .25, true)
	if got[0].ShapeType != "rectangle" {
		t.Fatalf("self-intersecting polygon = %#v", got[0])
	}
}

func TestComicMetadataUpdatePreservesManualAndReordersDetected(t *testing.T) {
	storage := t.TempDir()
	a, err := New(Config{StorageRoot: storage, DatabasePath: filepath.Join(storage, "test.db")}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = a.db.Exec(`INSERT INTO comics(id,title,status,page_count,created_at) VALUES('c','C','ready',2,?); INSERT INTO pages(comic_id,number,image_path,width,height,media_type) VALUES('c',1,'a',1,1,'image/png'),('c',2,'b',1,1,'image/png')`, now)
	if err != nil {
		t.Fatal(err)
	}
	for page := 1; page <= 2; page++ {
		var pageID int64
		if err := a.db.QueryRow(`SELECT id FROM pages WHERE comic_id='c' AND number=?`, page).Scan(&pageID); err != nil {
			t.Fatal(err)
		}
		left, right := defaultFrame(1), defaultFrame(2)
		left.X, left.Width, left.Height = .1, .3, .3
		right.X, right.Width, right.Height = .6, .3, .3
		if page == 2 {
			left.Source, right.Source = "manual", "manual"
		}
		if err := insertFrame(t.Context(), a.db, pageID, &left); err != nil {
			t.Fatal(err)
		}
		if err := insertFrame(t.Context(), a.db, pageID, &right); err != nil {
			t.Fatal(err)
		}
	}
	body, _ := json.Marshal(putComicRequest{ContentType: "manga"})
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/v1/comics/c", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", response.Code, response.Body.String())
	}
	var detectedFirstX, manualFirstX float64
	if err := a.db.QueryRow(`SELECT f.x FROM panels f JOIN pages p ON p.id=f.page_id WHERE p.comic_id='c' AND p.number=1 ORDER BY f."order" LIMIT 1`).Scan(&detectedFirstX); err != nil {
		t.Fatal(err)
	}
	if err := a.db.QueryRow(`SELECT f.x FROM panels f JOIN pages p ON p.id=f.page_id WHERE p.comic_id='c' AND p.number=2 ORDER BY f."order" LIMIT 1`).Scan(&manualFirstX); err != nil {
		t.Fatal(err)
	}
	if detectedFirstX != .6 || manualFirstX != .1 {
		t.Fatalf("orders detected=%v manual=%v", detectedFirstX, manualFirstX)
	}
}
