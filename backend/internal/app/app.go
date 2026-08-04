package app

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

type Config struct {
	StorageRoot  string
	DatabasePath string
	MaxUpload    int64
	MaxEntries   int
	MaxExtracted int64
	MaxFile      int64
}

type App struct {
	config Config
	db     *sql.DB
	logger *slog.Logger
}

type Comic struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	Progress     int       `json:"progress"`
	Phase        string    `json:"phase"`
	ErrorMessage string    `json:"error_message,omitempty"`
	PageCount    int       `json:"page_count"`
	CoverURL     string    `json:"cover_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Page struct {
	Number             int     `json:"number"`
	Width              int     `json:"width"`
	Height             int     `json:"height"`
	MediaType          string  `json:"media_type"`
	ImageURL           string  `json:"image_url"`
	Panels             []Panel `json:"panels"`
	Frames             []Panel `json:"frames"`
	Revision           int     `json:"revision"`
	FrameSetupComplete bool    `json:"frameSetupComplete"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Panel struct {
	ID                   int64     `json:"id,omitempty"`
	Name                 string    `json:"name"`
	Order                int       `json:"order"`
	ShapeType            string    `json:"shapeType"`
	FrameType            string    `json:"frameType"`
	X                    float64   `json:"x"`
	Y                    float64   `json:"y"`
	Width                float64   `json:"width"`
	Height               float64   `json:"height"`
	Polygon              []Point   `json:"polygon"`
	FitMode              string    `json:"fitMode"`
	PaddingPercent       float64   `json:"paddingPercent"`
	MaskOpacity          float64   `json:"maskOpacity"`
	TransitionDurationMS int       `json:"transitionDurationMs"`
	Easing               string    `json:"easing"`
	IsEnabled            bool      `json:"isEnabled"`
	Source               string    `json:"source"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type framesResponse struct {
	Revision int     `json:"revision"`
	Frames   []Panel `json:"frames"`
}

type ReadingProgress struct {
	Page      int       `json:"page"`
	Frame     int       `json:"frame"`
	Mode      string    `json:"mode"`
	Direction string    `json:"direction"`
	UpdatedAt time.Time `json:"updated_at"`
}

func New(config Config, logger *slog.Logger) (*App, error) {
	if config.StorageRoot == "" || config.DatabasePath == "" {
		return nil, errors.New("storage root and database path are required")
	}
	if err := os.MkdirAll(filepath.Join(config.StorageRoot, "tmp"), 0o750); err != nil {
		return nil, fmt.Errorf("create temporary storage: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(config.StorageRoot, "comics"), 0o750); err != nil {
		return nil, fmt.Errorf("create comic storage: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(config.DatabasePath), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	a := &App{config: config, db: db, logger: logger}
	if err := a.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) Close() error { return a.db.Close() }

func (a *App) migrate() error {
	_, err := a.db.Exec(`
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
		CREATE TABLE IF NOT EXISTS comics (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			page_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS pages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			comic_id TEXT NOT NULL REFERENCES comics(id) ON DELETE CASCADE,
			number INTEGER NOT NULL,
			image_path TEXT NOT NULL,
			width INTEGER NOT NULL,
			height INTEGER NOT NULL,
			media_type TEXT NOT NULL,
			UNIQUE(comic_id, number)
		);
		CREATE TABLE IF NOT EXISTS panels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
			"order" INTEGER NOT NULL,
			x REAL NOT NULL,
			y REAL NOT NULL,
			width REAL NOT NULL,
			height REAL NOT NULL,
			source TEXT NOT NULL,
			UNIQUE(page_id, "order")
		);`)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	columns := map[string]bool{}
	rows, err := a.db.Query(`PRAGMA table_info(comics)`)
	if err != nil {
		return fmt.Errorf("inspect comics schema: %w", err)
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("inspect comics schema: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("inspect comics schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("inspect comics schema: %w", err)
	}
	additions := []struct{ name, definition string }{
		{"progress", `INTEGER NOT NULL DEFAULT 100`},
		{"phase", `TEXT NOT NULL DEFAULT 'ready'`},
		{"error_message", `TEXT`},
	}
	for _, addition := range additions {
		if !columns[addition.name] {
			if _, err := a.db.Exec(`ALTER TABLE comics ADD COLUMN ` + addition.name + ` ` + addition.definition); err != nil {
				return fmt.Errorf("migrate comics.%s: %w", addition.name, err)
			}
		}
	}
	pageColumns, err := tableColumns(a.db, "pages")
	if err != nil {
		return err
	}
	for _, addition := range []struct{ name, definition string }{
		{"revision", `INTEGER NOT NULL DEFAULT 0`},
		{"frame_setup_complete", `INTEGER NOT NULL DEFAULT 0`},
	} {
		if !pageColumns[addition.name] {
			if _, err := a.db.Exec(`ALTER TABLE pages ADD COLUMN ` + addition.name + ` ` + addition.definition); err != nil {
				return fmt.Errorf("migrate pages.%s: %w", addition.name, err)
			}
		}
	}
	panelColumns, err := tableColumns(a.db, "panels")
	if err != nil {
		return err
	}
	for _, addition := range []struct{ name, definition string }{
		{"name", `TEXT NOT NULL DEFAULT ''`},
		{"shape_type", `TEXT NOT NULL DEFAULT 'rectangle'`},
		{"frame_type", `TEXT NOT NULL DEFAULT 'panel'`},
		{"polygon_json", `TEXT NOT NULL DEFAULT '[]'`},
		{"fit_mode", `TEXT NOT NULL DEFAULT 'contain'`},
		{"padding_percent", `REAL NOT NULL DEFAULT 0`},
		{"mask_opacity", `REAL NOT NULL DEFAULT 0.65`},
		{"transition_duration_ms", `INTEGER NOT NULL DEFAULT 300`},
		{"easing", `TEXT NOT NULL DEFAULT 'ease'`},
		{"is_enabled", `INTEGER NOT NULL DEFAULT 1`},
		{"created_at", `TEXT NOT NULL DEFAULT ''`},
		{"updated_at", `TEXT NOT NULL DEFAULT ''`},
	} {
		if !panelColumns[addition.name] {
			if _, err := a.db.Exec(`ALTER TABLE panels ADD COLUMN ` + addition.name + ` ` + addition.definition); err != nil {
				return fmt.Errorf("migrate panels.%s: %w", addition.name, err)
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := a.db.Exec(`UPDATE panels SET "order"="order"+100000 WHERE page_id IN (SELECT page_id FROM panels GROUP BY page_id HAVING MIN("order")=0); UPDATE panels SET "order"="order"-99999 WHERE "order">=100000; UPDATE panels SET source='detected' WHERE source='generated'; UPDATE panels SET created_at=? WHERE created_at=''; UPDATE panels SET updated_at=? WHERE updated_at=''`, now, now); err != nil {
		return fmt.Errorf("backfill rich frames: %w", err)
	}
	if _, err := a.db.Exec(`
		CREATE TABLE IF NOT EXISTS reading_progress (
			comic_id TEXT PRIMARY KEY REFERENCES comics(id) ON DELETE CASCADE,
			page INTEGER NOT NULL,
			frame INTEGER NOT NULL,
			mode TEXT NOT NULL,
			direction TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("migrate reading progress: %w", err)
	}
	missing, err := a.db.Query(`SELECT p.id,p.width,p.height FROM pages p WHERE NOT EXISTS (SELECT 1 FROM panels f WHERE f.page_id=p.id)`)
	if err != nil {
		return fmt.Errorf("find pages without focus frames: %w", err)
	}
	type pageWithoutFrames struct {
		id            int64
		width, height int
	}
	var pagesWithoutFrames []pageWithoutFrames
	for missing.Next() {
		var page pageWithoutFrames
		if err := missing.Scan(&page.id, &page.width, &page.height); err != nil {
			missing.Close()
			return fmt.Errorf("read page without focus frames: %w", err)
		}
		pagesWithoutFrames = append(pagesWithoutFrames, page)
	}
	if err := missing.Close(); err != nil {
		return fmt.Errorf("close focus frame migration rows: %w", err)
	}
	if len(pagesWithoutFrames) > 0 {
		tx, err := a.db.Begin()
		if err != nil {
			return fmt.Errorf("begin focus frame migration: %w", err)
		}
		for _, page := range pagesWithoutFrames {
			for _, panel := range generatedPanels(page.width, page.height) {
				if err = insertFrame(context.Background(), tx, page.id, &panel); err != nil {
					tx.Rollback()
					return fmt.Errorf("backfill focus frames: %w", err)
				}
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit focus frame migration: %w", err)
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("inspect %s schema: %w", table, err)
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func (a *App) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(a.recoverer, a.requestLog, cors)
	r.Get("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Post("/api/v1/comics", a.uploadComic)
	r.Get("/api/v1/comics", a.listComics)
	r.Get("/api/v1/comics/{comicID}", a.getComic)
	r.Get("/api/v1/comics/{comicID}/progress", a.getProgress)
	r.Put("/api/v1/comics/{comicID}/progress", a.putProgress)
	r.Get("/api/v1/comics/{comicID}/pages", a.listPages)
	r.Put("/api/v1/comics/{comicID}/pages/{pageNumber}/panels", a.replacePanels)
	r.Get("/api/v1/comics/{comicID}/pages/{pageNumber}/frames", a.getFrames)
	r.Put("/api/v1/comics/{comicID}/pages/{pageNumber}/frames", a.replaceFrames)
	r.Post("/api/v1/comics/{comicID}/pages/{pageNumber}/frames/full-page", a.addFullPageFrame)
	r.Get("/api/v1/comics/{comicID}/pages/{pageNumber}/image", a.pageImage)
	return r
}

func (a *App) uploadComic(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.config.MaxUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "A CBZ, CBR, or PDF file is required.")
		return
	}
	defer file.Close()
	extension := strings.ToLower(filepath.Ext(header.Filename))
	if extension != ".cbz" && extension != ".cbr" && extension != ".pdf" {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_file_type", "Only CBZ, CBR, and PDF files are supported.")
		return
	}

	id, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not create comic.")
		return
	}
	tmpArchive, err := os.CreateTemp(filepath.Join(a.config.StorageRoot, "tmp"), "upload-*"+extension)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Could not store upload.")
		return
	}
	tmpArchivePath := tmpArchive.Name()
	removeUpload := true
	defer func() {
		if removeUpload {
			os.Remove(tmpArchivePath)
		}
	}()
	if _, err = io.Copy(tmpArchive, file); err != nil {
		tmpArchive.Close()
		writeError(w, http.StatusBadRequest, "invalid_upload", "Could not read upload.")
		return
	}
	if err = tmpArchive.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Could not store upload.")
		return
	}

	title := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	created := time.Now().UTC()
	comic := Comic{ID: id, Title: title, Status: "processing", Progress: 0, Phase: "queued", CreatedAt: created}
	if _, err := a.db.ExecContext(r.Context(), `INSERT INTO comics(id,title,status,progress,phase,page_count,created_at) VALUES(?,?,?,?,?,?,?)`, id, title, comic.Status, comic.Progress, comic.Phase, 0, created.Format(time.RFC3339Nano)); err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "Could not save comic metadata.")
		return
	}
	removeUpload = false
	go a.processComic(id, extension, tmpArchivePath)
	writeJSON(w, http.StatusAccepted, comic)
}

func (a *App) processComic(id, extension, uploadPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	defer os.Remove(uploadPath)
	tmpDir, err := os.MkdirTemp(filepath.Join(a.config.StorageRoot, "tmp"), "comic-*")
	if err != nil {
		a.failComic(id, err)
		return
	}
	defer os.RemoveAll(tmpDir)
	progress := func(value int, phase string) {
		if value < 0 {
			value = 0
		}
		if value > 99 {
			value = 99
		}
		if _, err := a.db.ExecContext(ctx, `UPDATE comics SET progress=?,phase=? WHERE id=? AND status='processing'`, value, phase, id); err != nil {
			a.logger.Warn("persist comic progress", "comic_id", id, "error", err)
		}
	}
	progress(1, "extracting")
	pages, err := a.importPages(ctx, extension, uploadPath, filepath.Join(tmpDir, "pages"), id, progress)
	if err != nil {
		a.failComic(id, err)
		return
	}
	progress(95, "publishing")
	finalDir := filepath.Join(a.config.StorageRoot, "comics", id)
	if err := os.Rename(tmpDir, finalDir); err != nil {
		a.failComic(id, errors.New("Could not publish comic."))
		return
	}
	tx, err := a.db.BeginTx(ctx, nil)
	for _, page := range pages {
		if err != nil {
			break
		}
		var pageID int64
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO pages(comic_id,number,image_path,width,height,media_type) VALUES(?,?,?,?,?,?)`, id, page.Number, page.path, page.Width, page.Height, page.MediaType)
		if insertErr != nil {
			err = insertErr
			continue
		}
		pageID, err = result.LastInsertId()
		for _, panel := range generatedPanels(page.Width, page.Height) {
			if err != nil {
				break
			}
			err = insertFrame(ctx, tx, pageID, &panel)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE comics SET status='ready',progress=100,phase='ready',error_message=NULL,page_count=? WHERE id=?`, len(pages), id)
	}
	if err == nil {
		err = tx.Commit()
	} else if tx != nil {
		_ = tx.Rollback()
	}
	if err != nil {
		os.RemoveAll(finalDir)
		a.failComic(id, errors.New("Could not save comic metadata."))
	}
}

func (a *App) failComic(id string, cause error) {
	message := cause.Error()
	if errors.Is(cause, errPDFRendererUnavailable) {
		message = "PDF support requires pdftocairo from the poppler-utils package."
	}
	if _, err := a.db.Exec(`UPDATE comics SET status='failed',phase='failed',error_message=? WHERE id=?`, message, id); err != nil {
		a.logger.Error("persist comic failure", "comic_id", id, "error", err)
	}
	a.logger.Warn("comic import failed", "comic_id", id, "error", cause)
}

type extractedPage struct {
	Number, Width, Height int
	MediaType, path       string
}

func (a *App) extractCBZ(archivePath, destination, comicID string) ([]extractedPage, error) {
	return a.extractCBZProgress(archivePath, destination, comicID, func(int, string) {})
}

func (a *App) extractCBZProgress(archivePath, destination, comicID string, progress progressFunc) ([]extractedPage, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, errors.New("The uploaded file is not a valid ZIP archive.")
	}
	defer zr.Close()
	if len(zr.File) > a.config.MaxEntries {
		return nil, errors.New("The archive contains too many entries.")
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return nil, fmt.Errorf("create page storage: %w", err)
	}

	var candidates []*zip.File
	var total int64
	for _, entry := range zr.File {
		clean, ok := safeArchivePath(entry.Name)
		if !ok {
			return nil, errors.New("The archive contains an unsafe path.")
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("The archive contains a symbolic link.")
		}
		if entry.FileInfo().IsDir() || ignoredEntry(clean) {
			continue
		}
		if !supportedExtension(filepath.Ext(clean)) {
			continue
		}
		if int64(entry.UncompressedSize64) > a.config.MaxFile {
			return nil, errors.New("An archive entry is too large.")
		}
		total += int64(entry.UncompressedSize64)
		if total > a.config.MaxExtracted {
			return nil, errors.New("The extracted archive is too large.")
		}
		candidates = append(candidates, entry)
	}
	if len(candidates) == 0 {
		return nil, errors.New("The archive does not contain supported page images.")
	}
	sort.SliceStable(candidates, func(i, j int) bool { return naturalLess(candidates[i].Name, candidates[j].Name) })

	pages := make([]extractedPage, 0, len(candidates))
	for index, entry := range candidates {
		rc, err := entry.Open()
		if err != nil {
			return nil, errors.New("An archive entry could not be read.")
		}
		ext := strings.ToLower(filepath.Ext(entry.Name))
		name := fmt.Sprintf("%04d%s", index+1, ext)
		outputPath := filepath.Join(destination, name)
		out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
		if err == nil {
			_, err = io.Copy(out, io.LimitReader(rc, a.config.MaxFile+1))
		}
		if out != nil {
			err = errors.Join(err, out.Close())
		}
		err = errors.Join(err, rc.Close())
		if err != nil {
			return nil, errors.New("An archive entry could not be extracted.")
		}
		imageFile, err := os.Open(outputPath)
		if err != nil {
			return nil, errors.New("An extracted image could not be opened.")
		}
		cfg, format, err := image.DecodeConfig(imageFile)
		imageFile.Close()
		if err != nil {
			return nil, fmt.Errorf("%s is not a valid image", entry.Name)
		}
		mediaType := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
		if mediaType == "" {
			return nil, fmt.Errorf("%s uses an unsupported image format", entry.Name)
		}
		pages = append(pages, extractedPage{Number: index + 1, Width: cfg.Width, Height: cfg.Height, MediaType: mediaType, path: filepath.Join("comics", comicID, "pages", name)})
		progress(5+85*(index+1)/len(candidates), fmt.Sprintf("extracting page %d of %d", index+1, len(candidates)))
	}
	return pages, nil
}

func (a *App) listComics(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,title,status,progress,phase,COALESCE(error_message,''),page_count,created_at FROM comics ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, "database_error", "Could not list comics.")
		return
	}
	defer rows.Close()
	comics := []Comic{}
	for rows.Next() {
		var c Comic
		var created string
		if err := rows.Scan(&c.ID, &c.Title, &c.Status, &c.Progress, &c.Phase, &c.ErrorMessage, &c.PageCount, &created); err != nil {
			writeError(w, 500, "database_error", "Could not list comics.")
			return
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if c.PageCount > 0 && c.Status == "ready" {
			c.CoverURL = fmt.Sprintf("/api/v1/comics/%s/pages/1/image", c.ID)
		}
		comics = append(comics, c)
	}
	writeJSON(w, http.StatusOK, comics)
}

func (a *App) getComic(w http.ResponseWriter, r *http.Request) {
	c, err := a.comic(r.Context(), chi.URLParam(r, "comicID"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Comic not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not load comic.")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (a *App) comic(ctx context.Context, id string) (Comic, error) {
	var c Comic
	var created string
	err := a.db.QueryRowContext(ctx, `SELECT id,title,status,progress,phase,COALESCE(error_message,''),page_count,created_at FROM comics WHERE id=?`, id).Scan(&c.ID, &c.Title, &c.Status, &c.Progress, &c.Phase, &c.ErrorMessage, &c.PageCount, &created)
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if c.PageCount > 0 && c.Status == "ready" {
		c.CoverURL = fmt.Sprintf("/api/v1/comics/%s/pages/1/image", c.ID)
	}
	return c, err
}

func (a *App) listPages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "comicID")
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT p.number,p.width,p.height,p.media_type,p.revision,p.frame_setup_complete,
		f.id,f.name,f."order",f.shape_type,f.frame_type,f.x,f.y,f.width,f.height,f.polygon_json,f.fit_mode,f.padding_percent,f.mask_opacity,f.transition_duration_ms,f.easing,f.is_enabled,f.source,f.created_at,f.updated_at
		FROM pages p LEFT JOIN panels f ON f.page_id=p.id
		WHERE p.comic_id=? ORDER BY p.number,f."order"`, id)
	if err != nil {
		writeError(w, 500, "database_error", "Could not list pages.")
		return
	}
	defer rows.Close()
	pages := []Page{}
	for rows.Next() {
		var number, width, height int
		var mediaType string
		var revision int
		var setup bool
		var panelID, panelOrder, transition sql.NullInt64
		var name, shapeType, frameType, polygon, fitMode, easing, source, created, updated sql.NullString
		var x, y, panelWidth, panelHeight, padding, opacity sql.NullFloat64
		var enabled sql.NullBool
		if err := rows.Scan(&number, &width, &height, &mediaType, &revision, &setup, &panelID, &name, &panelOrder, &shapeType, &frameType, &x, &y, &panelWidth, &panelHeight, &polygon, &fitMode, &padding, &opacity, &transition, &easing, &enabled, &source, &created, &updated); err != nil {
			writeError(w, 500, "database_error", "Could not list pages.")
			return
		}
		if len(pages) == 0 || pages[len(pages)-1].Number != number {
			pages = append(pages, Page{Number: number, Width: width, Height: height, MediaType: mediaType, ImageURL: fmt.Sprintf("/api/v1/comics/%s/pages/%d/image", id, number), Panels: []Panel{}, Frames: []Panel{}, Revision: revision, FrameSetupComplete: setup})
		}
		if panelID.Valid {
			frame := Panel{ID: panelID.Int64, Name: name.String, Order: int(panelOrder.Int64), ShapeType: shapeType.String, FrameType: frameType.String, X: x.Float64, Y: y.Float64, Width: panelWidth.Float64, Height: panelHeight.Float64, Polygon: []Point{}, FitMode: fitMode.String, PaddingPercent: padding.Float64, MaskOpacity: opacity.Float64, TransitionDurationMS: int(transition.Int64), Easing: easing.String, IsEnabled: enabled.Bool, Source: source.String}
			_ = json.Unmarshal([]byte(polygon.String), &frame.Polygon)
			frame.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
			frame.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated.String)
			pages[len(pages)-1].Panels = append(pages[len(pages)-1].Panels, frame)
			pages[len(pages)-1].Frames = pages[len(pages)-1].Panels
		}
	}
	if len(pages) == 0 {
		if _, err := a.comic(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "not_found", "Comic not found.")
			return
		}
	}
	writeJSON(w, http.StatusOK, pages)
}

func (a *App) getFrames(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(chi.URLParam(r, "pageNumber"))
	if err != nil || number < 1 {
		writeError(w, http.StatusBadRequest, "invalid_page", "Page number is invalid.")
		return
	}
	var pageID int64
	var revision int
	if err := a.db.QueryRowContext(r.Context(), `SELECT id,revision FROM pages WHERE comic_id=? AND number=?`, chi.URLParam(r, "comicID"), number).Scan(&pageID, &revision); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Page not found.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "Could not load frames.")
		return
	}
	frames, err := loadFrames(r.Context(), a.db, pageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "Could not load frames.")
		return
	}
	writeJSON(w, http.StatusOK, framesResponse{Revision: revision, Frames: frames})
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadFrames(ctx context.Context, db queryer, pageID int64) ([]Panel, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,name,"order",shape_type,frame_type,x,y,width,height,polygon_json,fit_mode,padding_percent,mask_opacity,transition_duration_ms,easing,is_enabled,source,created_at,updated_at FROM panels WHERE page_id=? ORDER BY "order",id`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	frames := []Panel{}
	for rows.Next() {
		var frame Panel
		var polygon, created, updated string
		if err := rows.Scan(&frame.ID, &frame.Name, &frame.Order, &frame.ShapeType, &frame.FrameType, &frame.X, &frame.Y, &frame.Width, &frame.Height, &polygon, &frame.FitMode, &frame.PaddingPercent, &frame.MaskOpacity, &frame.TransitionDurationMS, &frame.Easing, &frame.IsEnabled, &frame.Source, &created, &updated); err != nil {
			return nil, err
		}
		frame.Polygon = []Point{}
		if err := json.Unmarshal([]byte(polygon), &frame.Polygon); err != nil {
			return nil, err
		}
		frame.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		frame.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		frames = append(frames, frame)
	}
	return frames, rows.Err()
}

type replaceFramesRequest struct {
	Revision int     `json:"revision"`
	Frames   []Panel `json:"frames"`
}

func (a *App) replaceFrames(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(chi.URLParam(r, "pageNumber"))
	if err != nil || number < 1 {
		writeError(w, http.StatusBadRequest, "invalid_page", "Page number is invalid.")
		return
	}
	var request replaceFramesRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_frames", "A revision and frames collection are required.")
		return
	}
	if message := validateFrames(request.Frames); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_frames", message)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "database_error", "Could not save frames.")
		return
	}
	defer tx.Rollback()
	var pageID int64
	var currentRevision int
	err = tx.QueryRowContext(r.Context(), `SELECT id,revision FROM pages WHERE comic_id=? AND number=?`, chi.URLParam(r, "comicID"), number).Scan(&pageID, &currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Page not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not save frames.")
		return
	}
	if request.Revision != currentRevision {
		writeError(w, http.StatusConflict, "revision_conflict", "Frames have changed; reload the latest revision.")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM panels WHERE page_id=?`, pageID); err == nil {
		for i := range request.Frames {
			request.Frames[i].ID = 0
			err = insertFrame(r.Context(), tx, pageID, &request.Frames[i])
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE pages SET revision=revision+1,frame_setup_complete=1 WHERE id=?`, pageID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not save frames.")
		return
	}
	writeJSON(w, http.StatusOK, framesResponse{Revision: currentRevision + 1, Frames: request.Frames})
}

func validateFrames(frames []Panel) string {
	if len(frames) > 500 {
		return "At most 500 frames may be submitted."
	}
	enabledOrders := []int{}
	orders := map[int]bool{}
	for i := range frames {
		f := &frames[i]
		if f.Order < 1 || orders[f.Order] || len(f.Name) > 200 || !finite(f.X, f.Y, f.Width, f.Height, f.PaddingPercent, f.MaskOpacity) {
			return "Frame orders must be positive and unique and numeric values must be finite."
		}
		orders[f.Order] = true
		if f.IsEnabled {
			enabledOrders = append(enabledOrders, f.Order)
		}
		if f.Source != "detected" && f.Source != "manual" && f.Source != "manual_edited" {
			return "Frame source must be detected, manual, or manual_edited."
		}
		if f.FrameType != "panel" && f.FrameType != "full_page" && f.FrameType != "focus" && f.FrameType != "speech" && f.FrameType != "object" {
			return "Frame type must be full_page, panel, focus, speech, or object."
		}
		if f.FitMode != "contain" && f.FitMode != "cover" && f.FitMode != "width" && f.FitMode != "height" {
			return "Fit mode must be contain, cover, width, or height."
		}
		if f.PaddingPercent < 0 || f.PaddingPercent > 50 || f.MaskOpacity < 0 || f.MaskOpacity > 1 || f.TransitionDurationMS < 0 || f.TransitionDurationMS > 10000 || len(f.Easing) > 100 {
			return "Frame presentation values are outside their allowed limits."
		}
		if f.ShapeType == "rectangle" {
			if f.Width <= 0 || f.Height <= 0 || f.X < 0 || f.Y < 0 || f.X+f.Width > 1 || f.Y+f.Height > 1 || len(f.Polygon) != 0 {
				return "Rectangle geometry must be positive and within normalized bounds."
			}
		} else if f.ShapeType == "polygon" {
			if len(f.Polygon) < 3 || len(f.Polygon) > 100 {
				return "Polygons must contain between 3 and 100 points."
			}
			for _, point := range f.Polygon {
				if !finite(point.X, point.Y) || point.X < 0 || point.X > 1 || point.Y < 0 || point.Y > 1 {
					return "Polygon points must be finite and within normalized bounds."
				}
			}
		} else {
			return "Shape type must be rectangle or polygon."
		}
	}
	sort.Ints(enabledOrders)
	for i, order := range enabledOrders {
		if order != i+1 {
			return "Enabled frame orders must be consecutive starting at 1."
		}
	}
	return ""
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertFrame(ctx context.Context, db execer, pageID int64, frame *Panel) error {
	now := time.Now().UTC()
	if frame.CreatedAt.IsZero() {
		frame.CreatedAt = now
	}
	frame.UpdatedAt = now
	polygon, err := json.Marshal(frame.Polygon)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `INSERT INTO panels(page_id,name,"order",shape_type,frame_type,x,y,width,height,polygon_json,fit_mode,padding_percent,mask_opacity,transition_duration_ms,easing,is_enabled,source,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, pageID, frame.Name, frame.Order, frame.ShapeType, frame.FrameType, frame.X, frame.Y, frame.Width, frame.Height, string(polygon), frame.FitMode, frame.PaddingPercent, frame.MaskOpacity, frame.TransitionDurationMS, frame.Easing, frame.IsEnabled, frame.Source, frame.CreatedAt.Format(time.RFC3339Nano), frame.UpdatedAt.Format(time.RFC3339Nano))
	if err == nil {
		frame.ID, err = result.LastInsertId()
	}
	return err
}

func generatedPanels(width, height int) []Panel {
	landscape := width > height
	ratio := float64(height) / float64(width)
	if landscape {
		ratio = float64(width) / float64(height)
	}
	count := max(2, int(math.Ceil(ratio)))
	size := math.Min(0.75, 1/ratio*1.15)
	step := (1 - size) / float64(count-1)
	panels := make([]Panel, count)
	for i := range panels {
		panels[i] = defaultFrame(i + 1)
		panels[i].Width, panels[i].Height, panels[i].Y = 1, size, float64(i)*step
		if landscape {
			panels[i].X, panels[i].Y = float64(i)*step, 0
			panels[i].Width, panels[i].Height = size, 1
		}
	}
	return panels
}

func defaultFrame(order int) Panel {
	now := time.Now().UTC()
	return Panel{Name: fmt.Sprintf("Frame %d", order), Order: order, ShapeType: "rectangle", FrameType: "panel", Polygon: []Point{}, FitMode: "contain", MaskOpacity: 0.65, TransitionDurationMS: 300, Easing: "ease", IsEnabled: true, Source: "detected", CreatedAt: now, UpdatedAt: now}
}

func (a *App) addFullPageFrame(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(chi.URLParam(r, "pageNumber"))
	if err != nil || number < 1 {
		writeError(w, 400, "invalid_page", "Page number is invalid.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "database_error", "Could not add frame.")
		return
	}
	defer tx.Rollback()
	var pageID int64
	var revision, order int
	err = tx.QueryRowContext(r.Context(), `SELECT p.id,p.revision,COALESCE(MAX(f."order"),0)+1 FROM pages p LEFT JOIN panels f ON f.page_id=p.id WHERE p.comic_id=? AND p.number=? GROUP BY p.id`, chi.URLParam(r, "comicID"), number).Scan(&pageID, &revision, &order)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Page not found.")
		return
	}
	frame := defaultFrame(order)
	frame.Name, frame.FrameType, frame.Width, frame.Height, frame.Source = "Full page", "full_page", 1, 1, "manual"
	if err == nil {
		err = insertFrame(r.Context(), tx, pageID, &frame)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE pages SET revision=revision+1,frame_setup_complete=1 WHERE id=?`, pageID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not add frame.")
		return
	}
	frames, err := loadFrames(r.Context(), a.db, pageID)
	if err != nil {
		writeError(w, 500, "database_error", "Could not load frames.")
		return
	}
	writeJSON(w, http.StatusCreated, framesResponse{Revision: revision + 1, Frames: frames})
}

func (a *App) getProgress(w http.ResponseWriter, r *http.Request) {
	var progress ReadingProgress
	var updated string
	err := a.db.QueryRowContext(r.Context(), `SELECT page,frame,mode,direction,updated_at FROM reading_progress WHERE comic_id=?`, chi.URLParam(r, "comicID")).Scan(&progress.Page, &progress.Frame, &progress.Mode, &progress.Direction, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		if _, comicErr := a.comic(r.Context(), chi.URLParam(r, "comicID")); errors.Is(comicErr, sql.ErrNoRows) {
			writeError(w, 404, "not_found", "Comic not found.")
			return
		}
		writeJSON(w, http.StatusOK, ReadingProgress{Page: 1, Frame: 1, Mode: "panel", Direction: "ltr"})
		return
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not load reading progress.")
		return
	}
	progress.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	writeJSON(w, http.StatusOK, progress)
}

func (a *App) putProgress(w http.ResponseWriter, r *http.Request) {
	var progress ReadingProgress
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&progress); err != nil || decoder.Decode(&struct{}{}) != io.EOF || progress.Page < 1 || progress.Frame < 0 || (progress.Mode != "panel" && progress.Mode != "page" && progress.Mode != "vertical") || (progress.Direction != "ltr" && progress.Direction != "rtl") {
		writeError(w, 400, "invalid_progress", "page, frame, mode (panel/page/vertical), and direction (ltr/rtl) are required.")
		return
	}
	var pageCount int
	if err := a.db.QueryRowContext(r.Context(), `SELECT page_count FROM comics WHERE id=?`, chi.URLParam(r, "comicID")).Scan(&pageCount); errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Comic not found.")
		return
	} else if err != nil {
		writeError(w, 500, "database_error", "Could not save reading progress.")
		return
	}
	if progress.Page > pageCount {
		writeError(w, 400, "invalid_progress", "Page is outside the comic page range.")
		return
	}
	progress.UpdatedAt = time.Now().UTC()
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO reading_progress(comic_id,page,frame,mode,direction,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(comic_id) DO UPDATE SET page=excluded.page,frame=excluded.frame,mode=excluded.mode,direction=excluded.direction,updated_at=excluded.updated_at`, chi.URLParam(r, "comicID"), progress.Page, progress.Frame, progress.Mode, progress.Direction, progress.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		writeError(w, 500, "database_error", "Could not save reading progress.")
		return
	}
	writeJSON(w, http.StatusOK, progress)
}

func (a *App) replacePanels(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(chi.URLParam(r, "pageNumber"))
	if err != nil || number < 1 {
		writeError(w, http.StatusBadRequest, "invalid_page", "Page number is invalid.")
		return
	}
	var panels []Panel
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&panels); err != nil || len(panels) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_panels", "A non-empty JSON array of panels is required.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_panels", "A non-empty JSON array of panels is required.")
		return
	}
	orders := make(map[int]bool, len(panels))
	for i := range panels {
		p := &panels[i]
		if orders[p.Order] || !finite(p.X, p.Y, p.Width, p.Height) || p.Width <= 0 || p.Height <= 0 || p.X < 0 || p.Y < 0 || p.X+p.Width > 1 || p.Y+p.Height > 1 {
			writeError(w, http.StatusBadRequest, "invalid_panels", "Panel orders must be unique and coordinates must be finite, positive, and within normalized bounds.")
			return
		}
		orders[p.Order] = true
		p.ID = 0
		if strings.TrimSpace(p.Source) == "" {
			p.Source = "manual"
		}
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "database_error", "Could not save panels.")
		return
	}
	defer tx.Rollback()
	var pageID int64
	err = tx.QueryRowContext(r.Context(), `SELECT id FROM pages WHERE comic_id=? AND number=?`, chi.URLParam(r, "comicID"), number).Scan(&pageID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Page not found.")
		return
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM panels WHERE page_id=?`, pageID)
	}
	for i := range panels {
		if err != nil {
			break
		}
		result, insertErr := tx.ExecContext(r.Context(), `INSERT INTO panels(page_id,"order",x,y,width,height,source) VALUES(?,?,?,?,?,?,?)`, pageID, panels[i].Order, panels[i].X, panels[i].Y, panels[i].Width, panels[i].Height, panels[i].Source)
		err = insertErr
		if err == nil {
			panels[i].ID, err = result.LastInsertId()
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not save panels.")
		return
	}
	sort.Slice(panels, func(i, j int) bool { return panels[i].Order < panels[j].Order })
	writeJSON(w, http.StatusOK, panels)
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func (a *App) pageImage(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(chi.URLParam(r, "pageNumber"))
	if err != nil || number < 1 {
		writeError(w, 400, "invalid_page", "Page number is invalid.")
		return
	}
	var relative, mediaType string
	err = a.db.QueryRowContext(r.Context(), `SELECT image_path,media_type FROM pages WHERE comic_id=? AND number=?`, chi.URLParam(r, "comicID"), number).Scan(&relative, &mediaType)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Page not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "database_error", "Could not load page.")
		return
	}
	path := filepath.Join(a.config.StorageRoot, relative)
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}

func safeArchivePath(name string) (string, bool) {
	if name == "" || strings.ContainsRune(name, '\x00') || filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return "", false
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

func ignoredEntry(name string) bool {
	base := filepath.Base(name)
	return strings.HasPrefix(name, "__MACOSX/") || base == ".DS_Store" || strings.HasPrefix(base, "._")
}
func supportedExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif":
		return true
	}
	return false
}

func naturalLess(a, b string) bool {
	a, b = strings.ToLower(filepath.ToSlash(a)), strings.ToLower(filepath.ToSlash(b))
	ar, br := []rune(a), []rune(b)
	for i, j := 0, 0; i < len(ar) && j < len(br); {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			ii, jj := i, j
			for ii < len(ar) && unicode.IsDigit(ar[ii]) {
				ii++
			}
			for jj < len(br) && unicode.IsDigit(br[jj]) {
				jj++
			}
			an := strings.TrimLeft(string(ar[i:ii]), "0")
			bn := strings.TrimLeft(string(br[j:jj]), "0")
			if an == "" {
				an = "0"
			}
			if bn == "" {
				bn = "0"
			}
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			i, j = ii, jj
			continue
		}
		if ar[i] != br[j] {
			return ar[i] < br[j]
		}
		i++
		j++
	}
	return len(ar) < len(br)
}

func randomID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "comic_" + hex.EncodeToString(b), nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (a *App) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		a.logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}
func (a *App) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.Error("panic", "value", recovered)
				writeError(w, 500, "internal_error", "An internal error occurred.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
