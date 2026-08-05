# Panel Reader

## 1. Product Goal

Build a local-first web application for reading digital comics with a guided panel-to-panel experience comparable to a modern interactive comic reader.

The backend is written in Go. The browser application provides:

- Normal full-page reading.
- Vertical-scroll reading.
- Guided panel-to-panel reading.
- A creator/editor workspace for defining the exact camera path.
- Smooth pan, zoom, masking, page transitions, and progress restoration.

The defining experience is:

```text
Open comic
  -> first configured frame
  -> smooth camera movement
  -> next frame
  -> object or dialogue focus when configured
  -> final frame
  -> first frame of the next page
```

A frame is not limited to a physical comic panel. It may represent:

- A full page.
- A normal rectangular panel.
- An irregular panel.
- A speech bubble.
- A character reaction.
- An important object.
- A subsection of a large splash panel.
- A return to the full panel or full page after close-up frames.

The first release must prove that this guided experience feels fast, stable, comfortable, and intentionally directed by the comic creator. Format count, accounts, cloud storage, and advanced automatic detection remain secondary.

The system is not merely a PDF or archive viewer. It is a camera system driven by creator-defined frame metadata.

---

## 2. Product Principles

1. Build a reliable page reader before guided panel mode.
2. Validate guided reading with manually defined frames before automatic detection.
3. Treat frame order and framing as part of comic storytelling, not only image segmentation.
4. Keep the reader independent of CBZ, CBR, PDF, and future source formats.
5. Preserve high-resolution source images so text remains sharp while zoomed.
6. Treat uploaded PDFs and archives as untrusted data.
7. Keep processing, storage, panel detection, reading-order logic, and reader animation replaceable.
8. Use one shared reader engine for creator preview and public reading.
9. Store normalized geometry so the same frames work on mobile, tablet, and desktop.
10. Never require a cropped image per frame; move one high-resolution page image behind the viewport.
11. Every publishable page must contain at least one enabled frame.
12. Automatic detection may suggest frames, but creators must be able to correct every result.
13. Reprocessing must never silently overwrite manually edited frames.
14. Prefer a small, well-tested Go application over premature distributed services.
15. Provide safe full-page fallback behavior whenever frame metadata is missing or invalid.

---

## 3. Main Product Areas

The application consists of two connected experiences.

### 3.1 Creator Experience

```text
Create comic
  -> upload PDF or CBZ
  -> process pages
  -> review page list
  -> define frames and reading order
  -> preview the exact reader behavior
  -> validate all pages
  -> publish comic
```

### 3.2 Reader Experience

```text
Open comic
  -> restore reading settings and progress
  -> load current page and nearby pages
  -> display the active frame
  -> navigate by tap, swipe, keyboard, or mouse
  -> move smoothly between frames
  -> continue across pages
  -> save the exact page and frame position
```

The creator preview and public reader must use the same camera calculations, mask implementation, frame sequence, and page transition logic.

---

## 4. Scope

### MVP 1: Import and Normal Page Reader

- Import one `.pdf` or `.cbz` comic.
- Stream uploads without loading the complete file into memory.
- Validate uploaded content and enforce configurable limits.
- Safely extract CBZ page images.
- Render PDF pages to high-resolution images.
- Sort CBZ pages naturally.
- Store comic and page metadata.
- Generate reader images, thumbnails, and a cover image.
- Display pages in the browser.
- Navigate to the previous or next page.
- Support fit page and fit width.
- Support manual zoom and fullscreen.
- Preload adjacent pages.
- Provide loading, failure, and retry states.
- Support page mode and vertical-scroll mode.

### MVP 2: Creator Frame Editor

- Display the complete page in an editor canvas.
- Add rectangular frames.
- Add polygon or irregular frames.
- Move and resize rectangular frames.
- Move polygon vertices.
- Add and remove polygon vertices.
- Duplicate and delete frames.
- Create a one-click full-page frame.
- Reorder frames using drag and drop.
- Show frame order numbers.
- Configure frame type, fit mode, padding, mask opacity, and transition duration.
- Save normalized coordinates.
- Auto-save completed edit operations.
- Support undo and redo.
- Warn before discarding unsaved changes.
- Support keyboard selection, deletion, and ordering.
- Preview the exact guided reader experience.
- Show page-level completion status.

### MVP 3: Guided Panel Reader

- Read the creator-defined frame sequence.
- Support rectangle and polygon frames.
- Support full-page, panel, focus, speech, and object frames.
- Keep one high-resolution page image mounted during same-page movement.
- Animate pan and zoom as one camera movement.
- Darken or hide unrelated page areas using a configurable mask.
- Move forward and backward through the exact sequence.
- Transition from the final frame of one page to the first frame of the next page.
- Support keyboard, mouse, tap zones, swipe, and basic pinch zoom.
- Support left-to-right and right-to-left reading.
- Support panel, page, and vertical reading modes.
- Handle fullscreen, viewport resize, and orientation changes.
- Respect `prefers-reduced-motion`.
- Save exact page, frame, reading mode, and reading direction progress.
- Restore the reader at the exact previously viewed frame.
- Fall back to full-page display when frame data is invalid.

### MVP 4: Publishing and Validation

- Add explicit comic publishing states.
- Validate that every page contains at least one enabled frame.
- Validate geometry, frame order, and page assets.
- Show missing or invalid pages to the creator.
- Prevent incomplete comics from being published.
- Allow unpublishing and republishing.
- Keep draft assets inaccessible through the public reader.
- Support creator preview without publishing.

### MVP 5: Automatic Frame Suggestions

- Detect conventional rectangular bordered panels.
- Show detection results in a numbered debug overlay.
- Suggest an initial reading order.
- Allow creators to correct every result.
- Keep detected and manually edited frame data distinguishable.
- Never publish automatically generated frames without creator review.
- Preserve manual frames during detection reruns unless explicitly reset.

### Later

- Comic library search, filtering, and sorting.
- CBR support.
- Double-page spreads.
- More advanced borderless and overlapping panel detection.
- Better automatic reading-order suggestions.
- PWA and offline reading.
- Advanced touch behavior.
- Accounts, cloud storage, and synchronization.
- Optional ML-assisted frame suggestions.
- Creator analytics and reader completion analytics.
- Paid content and entitlement checks if the application becomes hosted.

### Explicitly Out of Scope for the First Release

- Payments and social features.
- Recommendations.
- Native mobile applications.
- PostgreSQL or object storage without a demonstrated need.
- Fully automatic publication of detected frames.
- Perfect automatic handling of splash pages, overlapping artwork, borderless panels, and artwork crossing panel boundaries.
- A separate cropped image for every frame.
- Distributed job infrastructure before a durable queue is actually required.

---

## 5. Technology

### Backend

- Go.
- Chi for HTTP routing.
- Go standard library where practical.
- SQLite for metadata, editor data, publishing state, and reading progress.
- Local filesystem for original and generated files.
- Structured logging with `log/slog`.
- Versioned SQL migrations.
- Bounded in-process worker pool when asynchronous processing becomes necessary.

### Frontend

- React.
- TypeScript.
- Vite.
- CSS transforms for camera movement.
- SVG masks for polygon and rectangle focus effects.
- Pointer Events for mouse, touch, and pen input.
- A small animation or gesture library only when browser APIs and CSS transitions are insufficient.
- IndexedDB or local storage for anonymous progress and settings.

### Comic Processing

- CBZ: Go `archive/zip`.
- PDF: render every page to an image behind an importer abstraction.
- CBR later: maintained RAR library or isolated extraction tool.
- Preserve supported JPEG, PNG, AVIF, and WebP source images where practical.
- Convert animated GIF content to an explicitly supported static representation unless animation support is intentionally added.
- Generate:
  - High-resolution original or master page.
  - Reader-optimized page.
  - Thumbnail.
  - Cover image.
- Re-encode only when the source format is unsupported, PDF rendering is required, or optimization is explicitly configured.

The interactive browser reader still requires HTML, CSS, and TypeScript even though the backend is written in Go.

---

## 6. Architecture

```text
Browser
  |
  | HTTP/JSON, page assets, frame metadata
  v
Go application
  |-- HTTP handlers
  |-- comic import service
  |-- PDF renderer adapter
  |-- CBZ importer
  |-- page asset service
  |-- frame editor service
  |-- publishing validation service
  |-- reader metadata service
  |-- reading progress service
  |-- processing worker
  |-- optional frame detector
  |-- SQLite repository
  `-- filesystem storage
          |
          v
    normalized page assets and metadata
          |
          v
       shared reader engine
          |
          |-- creator preview
          `-- public reader
```

After import, the editor and reader must not care whether pages originated from CBZ, CBR, PDF, or a future format.

### Component Responsibilities

- HTTP handlers parse and validate requests, call services, and encode responses.
- Import services validate source files and create page records.
- The PDF adapter renders pages but does not own HTTP behavior.
- Repositories own SQLite queries and transactions.
- Filesystem storage owns paths, temporary files, atomic moves, and retryable deletion.
- Frame services validate normalized geometry and ordered frame collections.
- Publishing validation decides whether a comic is complete enough to publish.
- Frame detection returns suggestions but does not publish or overwrite manual work.
- Reading-order logic remains separate from rectangle detection.
- The frontend owns camera calculations, gestures, masking, and animation.
- Creator preview and public reading share the same reader package.

Do not create interfaces merely for abstraction. Add an interface when tests, adapters, or multiple implementations require one.

---

## 7. Suggested Repository Structure

```text
panel-reader/
|-- backend/
|   |-- cmd/server/main.go
|   |-- internal/
|   |   |-- comic/
|   |   |-- importer/
|   |   |   |-- cbz/
|   |   |   `-- pdf/
|   |   |-- pageasset/
|   |   |-- frame/
|   |   |-- publishing/
|   |   |-- reader/
|   |   |-- progress/
|   |   |-- detection/
|   |   |-- storage/
|   |   |-- repository/
|   |   |-- processing/
|   |   `-- web/
|   |-- migrations/
|   `-- go.mod
|-- frontend/
|   |-- src/
|   |   |-- api/
|   |   |-- features/
|   |   |   |-- comics/
|   |   |   |-- frame-editor/
|   |   |   `-- comic-reader/
|   |   |-- shared/
|   |   |-- pages/
|   |   `-- types/
|   `-- package.json
|-- storage/                  # ignored by Git
|-- testdata/
|-- compose.yaml              # add only when useful
`-- README.md
```

Suggested frontend feature structure:

```text
frontend/src/features/
|-- frame-editor/
|   |-- components/
|   |   |-- EditorCanvas.tsx
|   |   |-- RectangleFrame.tsx
|   |   |-- PolygonFrame.tsx
|   |   |-- FrameToolbar.tsx
|   |   |-- FrameList.tsx
|   |   |-- PageNavigator.tsx
|   |   `-- ReaderPreview.tsx
|   |-- hooks/
|   |-- state/
|   `-- geometry/
`-- comic-reader/
    |-- components/
    |   |-- ComicReader.tsx
    |   |-- ReaderViewport.tsx
    |   |-- PageLayer.tsx
    |   |-- FrameMask.tsx
    |   |-- GestureLayer.tsx
    |   |-- ReaderControls.tsx
    |   `-- ReaderSettings.tsx
    |-- camera/
    |-- hooks/
    |-- state/
    `-- preloading/
```

Packages should be introduced as required. Avoid empty placeholder packages.

---

## 8. Domain Model

Use database entities internally and dedicated request and response types at the HTTP boundary. Do not return every page and frame in comic list responses.

```go
type Comic struct {
	ID                 string
	Title              string
	Slug               string
	SourceFormat       string
	Status             ComicStatus
	PublishStatus      PublishStatus
	PageCount          int
	FrameCount         int
	ReadingDirection   ReadingDirection
	DefaultReadingMode ReadingMode
	ErrorMessage       string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	PublishedAt        *time.Time
}

type Page struct {
	ID                 string
	ComicID            string
	Number             int
	OriginalImagePath  string
	ReaderImagePath    string
	ThumbnailPath      string
	Width              int
	Height             int
	MediaType          string
	FrameCount         int
	FrameSetupComplete bool
	Revision           int64
}

type Frame struct {
	ID                   string
	PageID               string
	Order                int
	Name                 string
	ShapeType            FrameShape
	FrameType            FrameType
	X                    *float64
	Y                    *float64
	Width                *float64
	Height               *float64
	PolygonPoints        []Point
	FitMode              FrameFitMode
	PaddingPercent       float64
	MaskOpacity          float64
	TransitionDurationMS int
	Easing               string
	Source               FrameSource
	IsEnabled            bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
```

### Frame Shape

- `rectangle`
- `polygon`

### Frame Type

- `full_page`
- `panel`
- `focus`
- `speech`
- `object`

Frame type is descriptive metadata. It allows the editor and future analytics to distinguish a normal comic panel from a close-up or full-page storytelling step.

### Frame Fit Mode

- `contain`: show the complete frame bounds with configured padding.
- `cover`: fill more of the viewport and allow limited edge cropping.

Use `contain` as the default.

### Reading Mode

- `panel`
- `page`
- `vertical`

### Reading Direction

- `ltr`
- `rtl`

### Frame Source

- `detected`: generated by the detector and not manually changed.
- `manual`: created by a user.
- `manual_edited`: originally detected but later changed by a user.

Reprocessing must not overwrite `manual` or `manual_edited` frames unless the creator explicitly requests a full reset.

### Normalized Frame Geometry

Frame geometry uses values from `0` to `1` relative to the original page.

Rectangle example:

```json
{
  "id": "frame_01",
  "order": 1,
  "name": "Top-left panel",
  "shapeType": "rectangle",
  "frameType": "panel",
  "x": 0.02,
  "y": 0.03,
  "width": 0.46,
  "height": 0.24,
  "fitMode": "contain",
  "paddingPercent": 4,
  "maskOpacity": 0.7,
  "transitionDurationMs": 350,
  "source": "manual",
  "isEnabled": true
}
```

Polygon example:

```json
{
  "id": "frame_02",
  "order": 2,
  "name": "Diagonal reaction panel",
  "shapeType": "polygon",
  "frameType": "focus",
  "polygonPoints": [
    { "x": 0.52, "y": 0.03 },
    { "x": 0.96, "y": 0.03 },
    { "x": 0.91, "y": 0.32 },
    { "x": 0.50, "y": 0.29 }
  ],
  "fitMode": "contain",
  "paddingPercent": 3,
  "maskOpacity": 0.78,
  "transitionDurationMs": 380,
  "source": "manual",
  "isEnabled": true
}
```

The API must reject:

- Negative coordinates.
- Values greater than `1`.
- Zero-sized rectangles.
- Rectangles extending outside page bounds.
- Polygons with fewer than three distinct points.
- Polygon points outside page bounds.
- Duplicate frame orders.
- Unsupported frame types, shapes, fit modes, or source values.

Clamp harmless floating-point overflow such as `1.0000001` within a small configured tolerance before validation.

---

## 9. Comic and Publishing Status

Processing and publishing are separate concerns.

### Processing Status

```text
queued -> processing -> panel_setup -> ready
                    `-> failed
```

Meaning:

- `queued`: upload accepted but processing has not started.
- `processing`: source pages are being extracted or rendered.
- `panel_setup`: page assets exist, but creator framing is incomplete.
- `ready`: processing completed and all required metadata is available.
- `failed`: processing failed.

### Publishing Status

```text
draft -> ready_to_publish -> published -> archived
```

A comic becomes `ready_to_publish` only when:

- Every page asset exists.
- Every page contains at least one enabled frame.
- Every frame passes geometry validation.
- Frame orders are consecutive and unique.
- Reader preview metadata can be generated.
- The comic has no unresolved processing failure.

A server restart must not leave a comic permanently marked as processing. Startup recovery must detect stale processing states and either resume, retry, or mark them failed with a safe message.

---

## 10. SQLite Schema

Store binary files on disk and relational metadata in SQLite.

Recommended tables:

- `comics`
- `pages`
- `frames`
- `reading_progress`
- `processing_jobs`
- `schema_migrations`

### Core Schema

```sql
CREATE TABLE comics (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    source_format TEXT NOT NULL,
    processing_status TEXT NOT NULL,
    publish_status TEXT NOT NULL DEFAULT 'draft',
    page_count INTEGER NOT NULL DEFAULT 0,
    frame_count INTEGER NOT NULL DEFAULT 0,
    reading_direction TEXT NOT NULL DEFAULT 'ltr',
    default_reading_mode TEXT NOT NULL DEFAULT 'panel',
    original_source_path TEXT NOT NULL,
    cover_path TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    published_at TEXT,
    CHECK (source_format IN ('cbz', 'pdf')),
    CHECK (reading_direction IN ('ltr', 'rtl')),
    CHECK (default_reading_mode IN ('panel', 'page', 'vertical'))
);

CREATE TABLE pages (
    id TEXT PRIMARY KEY,
    comic_id TEXT NOT NULL,
    number INTEGER NOT NULL,
    original_image_path TEXT NOT NULL,
    reader_image_path TEXT NOT NULL,
    thumbnail_path TEXT,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    media_type TEXT NOT NULL,
    frame_count INTEGER NOT NULL DEFAULT 0,
    frame_setup_complete INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (comic_id, number),
    FOREIGN KEY (comic_id) REFERENCES comics(id) ON DELETE CASCADE
);

CREATE TABLE frames (
    id TEXT PRIMARY KEY,
    page_id TEXT NOT NULL,
    frame_order INTEGER NOT NULL,
    name TEXT,
    shape_type TEXT NOT NULL,
    frame_type TEXT NOT NULL DEFAULT 'panel',
    x REAL,
    y REAL,
    width REAL,
    height REAL,
    polygon_json TEXT,
    fit_mode TEXT NOT NULL DEFAULT 'contain',
    padding_percent REAL NOT NULL DEFAULT 4,
    mask_opacity REAL NOT NULL DEFAULT 0.70,
    transition_duration_ms INTEGER NOT NULL DEFAULT 350,
    easing TEXT NOT NULL DEFAULT 'cubic-bezier(.22,.61,.36,1)',
    source TEXT NOT NULL,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (page_id, frame_order),
    FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE CASCADE,
    CHECK (shape_type IN ('rectangle', 'polygon')),
    CHECK (frame_type IN ('full_page', 'panel', 'focus', 'speech', 'object')),
    CHECK (fit_mode IN ('contain', 'cover')),
    CHECK (source IN ('detected', 'manual', 'manual_edited')),
    CHECK (mask_opacity >= 0 AND mask_opacity <= 1),
    CHECK (padding_percent >= 0 AND padding_percent <= 50),
    CHECK (transition_duration_ms >= 0 AND transition_duration_ms <= 5000)
);

CREATE TABLE reading_progress (
    id TEXT PRIMARY KEY,
    reader_key TEXT NOT NULL,
    comic_id TEXT NOT NULL,
    page_id TEXT NOT NULL,
    frame_id TEXT,
    page_number INTEGER NOT NULL,
    frame_order INTEGER,
    reading_mode TEXT NOT NULL,
    reading_direction TEXT NOT NULL,
    progress_percent REAL NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    UNIQUE (reader_key, comic_id),
    FOREIGN KEY (comic_id) REFERENCES comics(id) ON DELETE CASCADE,
    FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE CASCADE,
    FOREIGN KEY (frame_id) REFERENCES frames(id) ON DELETE SET NULL,
    CHECK (reading_mode IN ('panel', 'page', 'vertical')),
    CHECK (reading_direction IN ('ltr', 'rtl'))
);

CREATE TABLE processing_jobs (
    id TEXT PRIMARY KEY,
    comic_id TEXT NOT NULL,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL,
    current_page INTEGER NOT NULL DEFAULT 0,
    total_pages INTEGER NOT NULL DEFAULT 0,
    progress_percent REAL NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    FOREIGN KEY (comic_id) REFERENCES comics(id) ON DELETE CASCADE
);
```

### Required Database Behavior

- Enable SQLite foreign keys.
- Use transactions for complete page-frame replacement.
- Keep `(page_id, frame_order)` unique.
- Keep frame order consecutive after every save.
- Delete comic records with cascading metadata deletion.
- Perform filesystem deletion only after the database operation is accepted.
- Make filesystem cleanup retryable.
- Apply versioned migrations at startup.
- Update page and comic frame counts transactionally.
- Increment `pages.revision` after a successful frame update.
- Use the revision for optimistic concurrency control.

---

## 11. Storage Layout

```text
storage/
|-- tmp/
`-- comics/
    `-- <comic-id>/
        |-- original/
        |   |-- comic.cbz
        |   `-- comic.pdf
        |-- pages/
        |   |-- 0001-original.webp
        |   |-- 0001-reader.webp
        |   |-- 0002-original.webp
        |   `-- 0002-reader.webp
        |-- thumbnails/
        |   |-- 0001.webp
        |   `-- 0002.webp
        `-- cover.webp
```

For supported CBZ image formats, the original page asset may retain its source extension. PDF pages must be rendered to an image format such as WebP or JPEG.

### Recommended Image Variants

- Master/original page: approximately 2400–3600 pixels wide where practical.
- Reader page: approximately 1600–2400 pixels wide.
- Thumbnail: approximately 250–400 pixels wide.
- Optional high-density reader variant for large displays.

Exact dimensions must preserve aspect ratio and remain configurable.

### Atomic Import

1. Stream the upload to a randomly named file under `storage/tmp`.
2. Enforce the compressed upload limit while streaming.
3. Detect and validate the actual source format.
4. Inspect archive or PDF structure.
5. Extract or render into a temporary comic directory.
6. Decode page dimensions and build metadata.
7. Generate reader images, thumbnails, and cover.
8. Validate the complete temporary result.
9. Atomically rename the temporary directory into its final location.
10. Commit database records and move the comic to `panel_setup`.
11. Remove the final directory if the database commit fails.
12. Clean orphaned temporary and final directories during startup maintenance.

Never expose a partially imported comic as ready or published.

---

## 12. Import Rules

### 12.1 CBZ Rules

A CBZ is a ZIP archive containing page images.

#### Accepted Entries

- Regular files with supported image content.
- Images inside nested directories.
- Case-insensitive supported extensions.
- Actual image content verified by decoding configuration.

#### Ignored Entries

- Directories.
- `__MACOSX` content.
- `.DS_Store`.
- Common hidden metadata files.
- Non-image files.

#### Rejected Archives

- Empty archives or archives containing no valid pages.
- Absolute paths.
- Path traversal components.
- Symlinks and non-regular entries.
- Encrypted or malformed entries.
- Files violating configured limits.
- Duplicate normalized destination paths.
- Extreme compression ratios that exceed configured safety limits.

#### Natural Sorting

Sort by complete normalized relative path using case-insensitive natural ordering, with the original path as a deterministic tie-breaker.

```text
1.jpg
2.jpg
3.jpg
10.jpg
```

must not become:

```text
1.jpg
10.jpg
2.jpg
3.jpg
```

### 12.2 PDF Rules

- Verify the PDF signature and parseability.
- Reject encrypted PDFs unless password support is intentionally implemented.
- Reject malformed or zero-page PDFs.
- Enforce page-count and processing-time limits.
- Render pages in source order.
- Preserve sufficient resolution for panel zooming.
- Generate page assets behind the same `Page` abstraction used by CBZ.
- Never serve the original PDF directly as the guided reader surface.
- Store the original PDF privately for reprocessing or download only when authorized.

---

## 13. Security Limits

All limits must be configurable and use safe defaults.

| Limit | Initial default |
|---|---:|
| Upload size | 1 GiB |
| Archive entries | 2,000 |
| PDF or comic pages | 2,000 |
| Extracted total size | 4 GiB |
| Single extracted file | 100 MiB |
| Image dimension | 30,000 px per side |
| Decoded pixels | 200 megapixels per image |
| Processing time | 15 minutes |
| Polygon points per frame | 100 |
| Frames per page | 500 |

Additional requirements:

- Use `http.MaxBytesReader`.
- Do not hold the complete upload or archive in memory.
- Check extracted totals and compression ratios.
- Validate every archive path before creating a file.
- Ensure final paths remain inside the intended comic directory.
- Use generated storage names.
- Apply read-header, read, write, and idle timeouts.
- Shut down gracefully and cancel processing through `context.Context`.
- Do not serve original source files publicly.
- Use `X-Content-Type-Options: nosniff`.
- Return accurate content types.
- Add authentication, CSRF protection, entitlement checks, and rate limits before public multi-user hosting.
- Never expose filesystem paths, stack traces, SQL errors, renderer command output, or internal process details.

---

## 14. HTTP API

Version the API from the beginning:

```text
/api/v1
```

### 14.1 Health

```text
GET /api/v1/health
```

```json
{
  "status": "ok"
}
```

### 14.2 Comics

```text
POST   /api/v1/comics
GET    /api/v1/comics?limit=20&cursor=<cursor>
GET    /api/v1/comics/{comicID}
PUT    /api/v1/comics/{comicID}
DELETE /api/v1/comics/{comicID}
POST   /api/v1/comics/{comicID}/reprocess
```

`POST /comics` accepts `multipart/form-data` with a field named `file`.

Example response:

```json
{
  "id": "comic_123",
  "title": "Example Comic",
  "sourceFormat": "pdf",
  "processingStatus": "processing",
  "publishStatus": "draft",
  "processingJobId": "job_123"
}
```

### 14.3 Processing Status

```text
GET /api/v1/comics/{comicID}/processing-status
```

```json
{
  "status": "processing",
  "currentPage": 18,
  "totalPages": 42,
  "progressPercent": 42.86,
  "error": null
}
```

### 14.4 Pages

```text
GET /api/v1/comics/{comicID}/pages
GET /api/v1/comics/{comicID}/pages/{pageNumber}
GET /api/v1/comics/{comicID}/pages/{pageNumber}/image
GET /api/v1/comics/{comicID}/pages/{pageNumber}/thumbnail
```

The comic detail response must not embed every page and frame.

Page list response:

```json
{
  "items": [
    {
      "id": "page_001",
      "number": 1,
      "thumbnailUrl": "/api/v1/comics/comic_123/pages/1/thumbnail",
      "width": 2400,
      "height": 3600,
      "frameCount": 5,
      "frameSetupComplete": true,
      "revision": 7
    }
  ]
}
```

Page detail response includes the complete ordered frame collection for only that page.

### 14.5 Frames

```text
GET  /api/v1/comics/{comicID}/pages/{pageNumber}/frames
PUT  /api/v1/comics/{comicID}/pages/{pageNumber}/frames
POST /api/v1/comics/{comicID}/pages/{pageNumber}/frames/full-page
POST /api/v1/comics/{comicID}/pages/{pageNumber}/frames/duplicate
POST /api/v1/comics/{comicID}/pages/{pageNumber}/frames/reset
POST /api/v1/comics/{comicID}/pages/{pageNumber}/detect
```

`PUT` replaces the complete ordered frame collection in one transaction.

Use `If-Match` or a request revision:

```json
{
  "revision": 7,
  "frames": [
    {
      "id": "frame_01",
      "order": 1,
      "name": "Opening full page",
      "shapeType": "rectangle",
      "frameType": "full_page",
      "x": 0,
      "y": 0,
      "width": 1,
      "height": 1,
      "fitMode": "contain",
      "paddingPercent": 2,
      "maskOpacity": 0,
      "transitionDurationMs": 350,
      "easing": "cubic-bezier(.22,.61,.36,1)",
      "source": "manual",
      "isEnabled": true
    }
  ]
}
```

Return `409 Conflict` when the page revision has changed.

### 14.6 Validation and Publishing

```text
POST /api/v1/comics/{comicID}/validate
POST /api/v1/comics/{comicID}/publish
POST /api/v1/comics/{comicID}/unpublish
```

Validation response:

```json
{
  "valid": false,
  "totalPages": 42,
  "configuredPages": 39,
  "missingPages": [17, 30, 41],
  "invalidFrames": [
    {
      "page": 12,
      "frameId": "frame_44",
      "code": "polygon_out_of_bounds"
    }
  ]
}
```

### 14.7 Reader Delivery

```text
GET /api/v1/reader/comics/{slug}
GET /api/v1/reader/comics/{slug}/pages?around=<pageNumber>
```

The initial reader response should return:

- Comic metadata.
- Reader settings.
- Current page.
- Previous page where available.
- Next one or two pages.
- Ordered frame metadata.
- Progress information where available.

Do not return all high-resolution pages or all frame data for a very large comic in the initial response.

### 14.8 Progress

```text
GET /api/v1/comics/{comicID}/progress
PUT /api/v1/comics/{comicID}/progress
```

```json
{
  "pageId": "page_047",
  "page": 47,
  "frameId": "frame_03",
  "frame": 3,
  "mode": "panel",
  "direction": "ltr"
}
```

Debounce progress updates. Save after navigation settles, on page change, on mode change, and when leaving the reader.

### 14.9 Error Format

```json
{
  "error": {
    "code": "invalid_archive",
    "message": "The uploaded file is not a valid supported comic.",
    "request_id": "req_123"
  }
}
```

---

## 15. Normal Page Reader

Build and stabilize this before guided mode.

### Features

- Previous and next page.
- Fit page.
- Fit width.
- Manual zoom.
- Fullscreen.
- Hide and show controls.
- Page counter.
- Loading, failure, and retry states.
- Adjacent-page preloading.
- Page mode.
- Vertical-scroll mode.
- Left-to-right and right-to-left navigation.
- Browser resize and orientation handling.

Keep a small neighboring page window ready:

```text
Page N-2 | Page N-1 | Page N | Page N+1 | Page N+2
```

The exact window may be reduced on memory-constrained devices.

Decode the next page before displaying it where browser support allows. Do not load a complete large comic into browser memory.

### Initial Keyboard Controls

| Key | Action |
|---|---|
| Left arrow | Previous frame/page in LTR; configurable for RTL |
| Right arrow | Next frame/page in LTR; configurable for RTL |
| Space | Next frame or page |
| Shift + Space | Previous frame or page |
| `F` | Toggle fullscreen |
| `M` | Cycle reading mode |
| `Escape` | Exit fullscreen or hide controls |
| `+` / `-` | Zoom in/out in page mode |

Ignore shortcuts while focus is inside an input, text area, select, editable region, or control that owns the key event.

---

## 16. Creator Frame Editor

Create a dedicated creator route:

```text
/creator/comics/:comicId/frame-editor/:pageNumber
```

### Recommended Layout

```text
+------------------------------------------------------------------+
| Back | Page 3/42 | Saved | Preview | Validate | Publish          |
+----------------+--------------------------------+----------------+
| Page thumbnails|                                | Ordered frames |
|                |        Editing canvas          |                |
| Page 1  ✓      |                                | 1. Full page   |
| Page 2  ✓      |                                | 2. Panel       |
| Page 3  •      |                                | 3. Reaction    |
| Page 4  !      |                                |                |
+----------------+--------------------------------+----------------+
| Select | Rectangle | Polygon | Full Page | Duplicate | Delete    |
+------------------------------------------------------------------+
```

### Required Editor Features

- Select frame.
- Draw rectangle.
- Draw polygon.
- Move and resize rectangle.
- Move polygon vertices.
- Add and remove polygon vertices.
- Duplicate frame.
- Delete frame.
- Create full-page frame.
- Reorder frames by drag and drop.
- Display frame order labels on the page.
- Rename frames.
- Set frame type.
- Set fit mode.
- Set frame padding.
- Set mask opacity.
- Set transition duration.
- Enable or disable a frame.
- Undo and redo.
- Auto-save.
- Explicit save status.
- Warn before leaving with unsaved changes.
- Preview using the shared reader engine.
- Previous and next page navigation.
- Mobile, tablet, and desktop preview sizes.
- Portrait and landscape preview.
- LTR and RTL preview.
- Keyboard-accessible selection, deletion, and reordering.

### Editor State

```ts
interface EditorState {
  pageId: string;
  pageNumber: number;
  revision: number;
  selectedFrameId: string | null;
  tool: "select" | "rectangle" | "polygon";
  canvasZoom: number;
  canvasPanX: number;
  canvasPanY: number;
  frames: ComicFrame[];
  undoStack: ComicFrame[][];
  redoStack: ComicFrame[][];
  hasUnsavedChanges: boolean;
  saveStatus: "idle" | "dirty" | "saving" | "saved" | "error";
}
```

### Auto-Save Rules

```text
User completes drag, resize, draw, reorder, or property change
  -> mark editor dirty
  -> wait 700–1000 ms
  -> send complete ordered frame collection with page revision
  -> update revision
  -> show Saved
```

Do not save on every pointer movement.

### Coordinate Conversion

The editor displays a resized page image. Save normalized values rather than rendered pixels.

```text
normalizedX =
  (pointerX - displayedImageLeft) / displayedImageWidth

normalizedY =
  (pointerY - displayedImageTop) / displayedImageHeight
```

Rectangle values:

```text
x      = renderedX / displayedImageWidth
y      = renderedY / displayedImageHeight
width  = renderedWidth / displayedImageWidth
height = renderedHeight / displayedImageHeight
```

### Ordering Rules

- Frame order starts at `1`.
- Enabled frames have unique consecutive order values.
- Reordering renumbers the complete collection before saving.
- Disabled frames may remain in the editor but are excluded from the reader sequence.
- Backward navigation must be the exact reverse of forward navigation.

---

## 17. Shared Reader Engine

The same reader package must power:

- Creator preview.
- Draft preview.
- Published public reading.

Recommended component tree:

```text
ComicReader
|-- ReaderViewport
|-- PageLayer
|-- FrameMask
|-- GestureLayer
|-- ReaderControls
|-- ReaderProgress
|-- ReaderSettings
`-- PagePreloader
```

### Reader State

```ts
interface ReaderState {
  mode: "panel" | "page" | "vertical";
  currentPageIndex: number;
  currentFrameIndex: number | null;
  direction: "ltr" | "rtl";
  fullscreen: boolean;
  controlsVisible: boolean;
  isTransitioning: boolean;
  manualZoomActive: boolean;
  reducedMotion: boolean;
}
```

A `null` frame may be used only when intentionally showing an unconfigured page overview or a safe fallback. In a configured panel sequence, a full-page frame should be represented as normal frame metadata.

---

## 18. Guided Frame Sequence

The creator controls the sequence.

Examples:

### Conventional Page

```text
Full-page overview
  -> top-left panel
  -> top-right panel
  -> lower-left panel
  -> lower-right panel
```

### Splash Page

```text
Full page
  -> character close-up
  -> important object
  -> lower dialogue
  -> full page again
```

### Dramatic Reveal

```text
Speech bubble
  -> character reaction
  -> complete panel
```

### Large Wide Panel

```text
Complete panel
  -> left section
  -> center section
  -> right section
  -> complete panel
```

The reader must not infer or replace this sequence after publication.

---

## 19. Camera Calculation

The reader displays the complete high-resolution page and moves a virtual camera around it.

### 19.1 Rectangle Bounds

```text
frameX      = normalizedX * pageWidth
frameY      = normalizedY * pageHeight
frameWidth  = normalizedWidth * pageWidth
frameHeight = normalizedHeight * pageHeight
```

### 19.2 Polygon Bounds

For a polygon:

```text
minX = minimum(point.x)
minY = minimum(point.y)
maxX = maximum(point.x)
maxY = maximum(point.y)

frameX      = minX * pageWidth
frameY      = minY * pageHeight
frameWidth  = (maxX - minX) * pageWidth
frameHeight = (maxY - minY) * pageHeight
```

Use the polygon bounding box for camera positioning and the original polygon points for masking.

### 19.3 Padding

```text
paddedWidth  = frameWidth  * (1 + paddingPercent / 100)
paddedHeight = frameHeight * (1 + paddingPercent / 100)
```

### 19.4 Scale

```text
scaleX = usableViewportWidth / paddedWidth
scaleY = usableViewportHeight / paddedHeight
```

For `contain`:

```text
scale = min(scaleX, scaleY)
```

For `cover`:

```text
scale = max(scaleX, scaleY)
```

Clamp scale to configured minimum and maximum values.

### 19.5 Translation

```text
frameCenterX = frameX + frameWidth / 2
frameCenterY = frameY + frameHeight / 2

translateX = viewportWidth / 2 - frameCenterX * scale
translateY = viewportHeight / 2 - frameCenterY * scale
```

Clamp translation so unnecessary empty canvas is not exposed.

### 19.6 Transform

```css
.comic-page-layer {
  transform-origin: 0 0;
  transform:
    translate3d(var(--translate-x), var(--translate-y), 0)
    scale(var(--panel-scale));
  transition:
    transform var(--transition-duration)
    var(--transition-easing);
}

@media (prefers-reduced-motion: reduce) {
  .comic-page-layer {
    transition-duration: 0ms;
  }
}
```

Apply `will-change: transform` only during active animation or when profiling demonstrates a benefit.

### 19.7 Recalculation

Recalculate the transform when:

- Viewport size changes.
- Browser orientation changes.
- Fullscreen changes.
- Reader controls change the usable viewport.
- Reading mode changes.
- Device safe-area values change.

Disable transitions during geometry recalculation so resizing does not create accidental camera travel.

---

## 20. Active Frame Mask

The reader should optionally darken everything outside the current frame.

### Rectangle Frames

Use an SVG mask or overlay with a transparent rectangular cutout.

### Polygon Frames

Use an SVG polygon cutout based on normalized polygon points.

Conceptual structure:

```html
<svg class="frame-mask" aria-hidden="true">
  <defs>
    <mask id="active-frame-mask">
      <rect width="100%" height="100%" fill="white" />
      <polygon points="..." fill="black" />
    </mask>
  </defs>

  <rect
    width="100%"
    height="100%"
    fill="rgba(0,0,0,var(--mask-opacity))"
    mask="url(#active-frame-mask)"
  />
</svg>
```

Mask requirements:

- Animate opacity independently from camera movement.
- Support opacity from `0` to `1`.
- Disable masking for full-page frames when configured.
- Recalculate correctly on viewport and page transform changes.
- Never block navigation gestures or accessibility controls.
- Provide a reader setting to reduce or disable masking.

---

## 21. Reader Navigation

### Desktop

- Right arrow: next in LTR mode.
- Left arrow: previous in LTR mode.
- Reverse configurable directional behavior for RTL mode.
- Space: next.
- Shift + Space: previous.
- `F`: fullscreen.
- `M`: change reading mode.
- Escape: exit fullscreen or hide controls.
- Click right or left navigation zones.

### Mobile and Tablet

- Tap forward zone: next frame or page.
- Tap backward zone: previous frame or page.
- Tap center: show or hide controls.
- Swipe in reading direction: next.
- Swipe opposite reading direction: previous.
- Pinch: temporary manual zoom.
- Double tap: temporary zoom.
- Orientation change: recalculate active frame without losing state.

Tap zones:

```text
+-------------+----------------------+-------------+
|  Previous   | Show/hide controls   |    Next     |
+-------------+----------------------+-------------+
```

Reverse forward and backward zones when required by RTL reading settings.

### Transition Locking

Rapid input must not corrupt reader state.

```ts
if (state.isTransitioning) {
  queueOrIgnoreNavigation();
  return;
}
```

Recommended behavior:

- Keep at most one queued navigation action.
- Cancel stale transitions when the viewport changes.
- Use `transitionend` with a timeout fallback.
- Never allow a late event from an old page to overwrite the current state.

---

## 22. Same-Page and Cross-Page Transitions

### Same Page

```text
Current frame
  -> calculate next transform
  -> set transition state
  -> apply pan and zoom together
  -> update mask
  -> wait for completion
  -> commit frame state
```

Recommended duration:

```text
300–450 ms
```

### Next Page

```text
Final frame of current page
  -> verify next page is decoded
  -> mount current and next page layers
  -> position next page at its first-frame transform
  -> crossfade or controlled slide
  -> activate next page
  -> remove distant page layer
```

Recommended duration:

```text
200–350 ms
```

Requirements:

- No white flash.
- No same-page image reload.
- No visible jump to the full page before the next frame.
- Previous navigation across pages produces the exact reverse behavior.
- If the next image fails, show a retry surface rather than a blank viewport.

---

## 23. Page Preloading and Memory Management

Preload:

- Current page.
- Previous page.
- Next page.
- Optionally the second next page after the first next page is decoded.

Example window:

```text
Page N-2 | Page N-1 | Page N | Page N+1 | Page N+2
```

Memory requirements:

- Remove distant page images from the DOM.
- Release object URLs when no longer needed.
- Avoid preloading an entire long comic.
- Reduce cache size on memory-constrained devices.
- Cache frame metadata separately from large image assets.
- Prefer immutable image URLs or strong validators such as `ETag`.

---

## 24. Reader Modes and Switching

### Panel Mode

Follow creator-defined frames.

```text
Frame 1 -> Frame 2 -> Frame 3 -> next page frame 1
```

### Page Mode

Display one complete page at a time.

```text
Page 1 -> Page 2 -> Page 3
```

### Vertical Mode

Display pages in a lazy-loaded vertical stream.

```text
Page 1
Page 2
Page 3
```

Switching behavior:

- Panel to page: remain on the current page.
- Page to panel: return to the last active frame on that page, or the first frame if none exists.
- Vertical to panel: choose the frame associated with the most visible page.
- Preserve mode-specific zoom state separately where practical.
- Do not reset reading progress when mode changes.

---

## 25. Reader Settings

Support:

- Reading mode.
- Reading direction.
- Transition speed.
- Frame-mask opacity.
- Show or hide page number.
- Fullscreen.
- Reduced motion.
- Tap-zone direction behavior.
- Keep screen awake where supported.
- Resume confirmation behavior.

Store settings:

- In browser storage for anonymous users.
- In the user profile when authentication is later added.

The reader must continue to obey operating-system reduced-motion preferences even if a stored transition preference exists.

---

## 26. Special Page Handling

Every publishable page must contain at least one enabled frame.

### Cover

Create one full-page frame.

### Credits

Create one full-page frame.

### Advertisement or Text Page

Create one full-page frame unless the creator intentionally adds focused steps.

### Empty Page

Use a full-page frame or mark it explicitly skippable through a future page setting.

### Splash Page

A creator may use:

```text
Full page
  -> detail
  -> character
  -> dialogue
  -> full page
```

### Borderless or Overlapping Art

Use manually defined polygon or focus frames.

### Missing or Invalid Metadata

```text
Invalid frame
  -> log safe diagnostic
  -> display complete page
  -> allow next and previous navigation
  -> show creator validation warning in editor
```

Never leave the reader blank.

---

## 27. Reading Progress

Save:

- Comic.
- Page ID and page number.
- Frame ID and frame order.
- Reading mode.
- Reading direction.
- Updated time.
- Optional progress percentage.

Save after:

- Frame change settles.
- Page changes.
- Reading mode changes.
- Reader closes or page visibility changes.
- A short debounce period after rapid navigation.

Anonymous readers:

- Store progress locally using a stable local reader key.

Authenticated readers later:

- Synchronize local and server progress.
- Resolve conflicts using updated timestamps and explicit user choice where necessary.

Resume behavior:

```text
Continue from Page 8, Frame 4
Start from beginning
```

The resume target must be the exact frame, not only the page.

---

## 28. Publishing Validation

Before publishing, validate:

### Comic-Level Rules

- Comic has at least one page.
- Processing completed successfully.
- Cover and reader assets exist.
- Slug is valid and unique.
- Reading direction and default mode are supported.

### Page-Level Rules

- Every page has at least one enabled frame.
- Page dimensions are valid.
- Reader image exists.
- Frame setup is marked complete only after successful validation.

### Frame-Level Rules

- Order starts at `1`.
- Order is unique and consecutive.
- Rectangle geometry is inside page bounds.
- Polygon has at least three valid points.
- Fit mode is valid.
- Padding is within range.
- Mask opacity is within range.
- Transition duration is within range.
- At least one frame remains enabled.

Publishing must be a transactionally safe operation. A validation failure returns the complete actionable problem list.

---

## 29. Automatic Frame Suggestions

Automatic detection begins only after manual editing and guided reading work.

### Version 1 Scope

Support conventional rectangular panels with visible borders or gutters.

```text
page image
  -> create downscaled analysis copy
  -> grayscale
  -> threshold or edge detection
  -> identify rectangle candidates
  -> remove noise
  -> merge duplicates and heavy overlaps
  -> map to normalized source coordinates
  -> suggest reading order
  -> save as detected draft frames
```

### Detection Rules

- Detection never publishes.
- Detection never overwrites manual frames without explicit reset.
- Detection results are visibly marked as suggestions.
- Creator may accept, edit, reorder, delete, or replace every frame.
- Editing a detected frame changes its source to `manual_edited`.
- Reading order is a separate service from rectangle detection.
- LTR and RTL ordering are configurable.
- Row grouping must tolerate vertical misalignment.

Suggested interface:

```go
type Detector interface {
	Detect(ctx context.Context, imagePath string) ([]FrameCandidate, error)
}
```

Do not select ML until failure cases have been collected and categorized.

---

## 30. Background Processing

HTTP handlers must not contain extraction, PDF rendering, thumbnail generation, or detection logic.

### Initial Implementation

- Save comic metadata and set status.
- Run processing through a service that accepts `context.Context`.
- Synchronous execution is acceptable only for the earliest development milestone.
- Persist processing status.
- Clean up correctly on failure.

### Next Step

- Add an in-process bounded worker pool.
- Persist jobs before claiming crash recovery.
- Limit concurrent extraction, PDF rendering, and image decoding.
- Track per-page progress.
- Retry safe failures.
- Recover stale jobs during startup.

Do not add Redis or a separate queue service until multiple processes or durable distributed execution are required.

---

## 31. Error Handling

### Import Errors

Handle:

- Unsupported format.
- Encrypted PDF.
- Malformed PDF.
- Empty archive.
- No valid pages.
- Zip Slip path.
- Symlink entry.
- Size-limit violation.
- Pixel-limit violation.
- Rendering timeout.
- Storage failure.
- Database failure.
- Thumbnail-generation failure.

### Editor Errors

Handle:

- Revision conflict.
- Invalid geometry.
- Lost network connection.
- Failed auto-save.
- Invalid polygon.
- Page asset unavailable.
- Unsaved changes on navigation.

### Reader Errors

Handle:

- Page image load failure.
- Missing frame metadata.
- Invalid transform.
- Next-page preload failure.
- Expired or missing asset.
- Progress-save failure.
- Unsupported fullscreen or wake-lock API.

Reader failures must degrade to a usable full-page experience where possible.

---

## 32. Performance Requirements

Initial targets:

- First visible reader page under two seconds on a normal connection after metadata is available.
- Smooth frame transition without white flashes.
- No image reload between frames on the same page.
- Next page decoded before the user reaches the final frame where practical.
- No API request for every tap.
- Stable memory use across long comics.
- Responsive editing during drag and resize.
- Camera transform calculation fast enough for resize and orientation changes.

Use GPU-friendly animation properties:

```text
transform
opacity
```

Avoid animating:

```text
top
left
width
height
```

during guided reading.

Optimization must be measurement-driven, but safe upload, archive, PDF, and image limits are required from the beginning.

---

## 33. Accessibility

### Reader

- All navigation actions available from keyboard.
- Controls have accessible names.
- Focus is visible.
- Reduced motion is respected.
- Mask opacity can be reduced or disabled.
- Reader controls remain operable during transitions.
- Do not trap keyboard focus unexpectedly.
- Provide screen-reader status such as page and frame position.
- Use semantic buttons rather than clickable decorative elements.

### Editor

- Keyboard frame selection.
- Keyboard deletion.
- Keyboard order movement.
- Numeric geometry editing as an alternative to pointer dragging.
- Visible selected-frame state.
- Accessible form labels for frame properties.
- Confirmation for destructive reset actions.

---

## 34. Observability and Configuration

### Configuration

Support environment variables or flags for:

- HTTP address.
- SQLite path.
- Storage root.
- Upload limits.
- Archive limits.
- PDF page limits.
- Image and pixel limits.
- Processing timeout.
- Worker concurrency.
- Reader image dimensions.
- Allowed frontend origin in development.
- Log level.

Validate configuration at startup and fail with a clear message when invalid.

### Observability

Record:

- Request ID.
- Import duration.
- Source format.
- Page count.
- Extracted or rendered bytes.
- Thumbnail duration.
- Frame save conflicts.
- Publishing validation failures.
- Detection duration and candidate count.
- Reader image failures.
- Processing retry count.
- Graceful shutdown state.

Never log uploaded file contents, authorization headers, local progress identifiers, or raw internal errors in API responses.

---

## 35. Testing Strategy

### Go Unit Tests

- Natural sorting.
- Archive path validation.
- Upload and extraction limits.
- PDF validation.
- Image type detection.
- Normalized rectangle validation.
- Polygon validation.
- Frame-order validation.
- Reading-order logic.
- Publishing state transitions.
- Processing state recovery.

### Go Integration Tests

- Import a valid CBZ.
- Import a valid PDF.
- Retrieve page metadata and images.
- Reject empty or malformed sources.
- Reject Zip Slip paths and symlinks.
- Reject oversized content.
- Clean files and database records after failed imports.
- Replace complete frame collections transactionally.
- Detect revision conflicts.
- Validate publishable and non-publishable comics.
- Delete a comic and generated files.
- Recover stale processing state after restart.

### Frontend Reader Tests

- Forward and backward frame sequence.
- Cross-page forward and backward sequence.
- Rectangle camera calculations.
- Polygon bounding-box calculations.
- Contain and cover fit.
- Mask positioning.
- Viewport resize.
- Fullscreen changes.
- Orientation changes.
- Rapid repeated navigation.
- Reduced-motion behavior.
- Reader-mode switching.
- RTL navigation.
- Exact progress restoration.
- Missing frame fallback.
- Image-load failure handling.

### Frontend Editor Tests

- Rectangle creation.
- Polygon creation.
- Move and resize.
- Vertex editing.
- Duplicate and delete.
- Reordering and renumbering.
- Undo and redo.
- Auto-save debounce.
- Revision conflict.
- Unsaved-change warning.
- Full-page frame creation.
- Preview parity with public reader.

### Detection Tests

Maintain a small licensed test corpus with expected frame rectangles. Use visual debug overlays and tolerance-based comparisons rather than requiring exact pixels across every algorithm revision.

---

## 36. Development Milestones

### Milestone 1: Foundation

- Initialize Go and React projects.
- Add configuration, logging, graceful shutdown, and health endpoint.
- Add SQLite migrations.
- Establish frontend-to-backend communication.

Done when the frontend displays a successful health response.

### Milestone 2: Secure PDF and CBZ Import

- Stream upload to temporary storage.
- Detect source format.
- Validate configured limits.
- Safely extract CBZ pages.
- Render PDF pages.
- Generate page variants, thumbnails, and cover.
- Store comic and page metadata.
- Recover stale processing status.

Done when valid PDF and CBZ comics import consistently and malicious fixtures are rejected without leaving partial data.

### Milestone 3: Normal Reader

- Serve comic and page metadata.
- Display every page.
- Add previous and next navigation.
- Add fit modes, manual zoom, fullscreen, and vertical reading.
- Add adjacent-page preloading.
- Add LTR and RTL page navigation.

Done when a complete comic can be read comfortably without large memory growth or visible flashes.

### Milestone 4: Rectangle Frame Editor

- Add rectangle CRUD and ordering.
- Add normalized coordinate conversion.
- Add full-page frame creation.
- Add auto-save, undo, redo, and revision conflict handling.
- Add page completion indicators.

Done when an arbitrary conventional page can be accurately mapped without editing files manually.

### Milestone 5: Basic Guided Reader

- Add frame state.
- Add camera transform calculation.
- Add same-page pan and zoom.
- Add cross-page transitions.
- Add keyboard, mouse, and tap navigation.
- Add reduced-motion handling.
- Add progress saving.

Done when manually mapped rectangle frames feel smooth and predictable.

### Milestone 6: Polygon and Focus Frames

- Add polygon drawing and editing.
- Add polygon bounding-box calculation.
- Add SVG frame mask.
- Add frame type, padding, fit mode, opacity, duration, and easing settings.
- Add object, speech, focus, and full-page frame sequences.

Done when irregular panels and storytelling close-ups can be authored and read correctly.

### Milestone 7: Shared Preview and Publishing

- Use the same reader engine for creator preview and public reading.
- Add responsive preview sizes.
- Add validation report.
- Add publish and unpublish flows.
- Prevent incomplete publication.
- Protect draft content.

Done when a creator can configure, preview, validate, and publish a complete comic.

### Milestone 8: Mobile and RTL Polish

- Add swipe.
- Add pinch and double-tap zoom.
- Add orientation handling.
- Add responsive controls and safe-area support.
- Complete RTL behavior.
- Add reader settings.

Done when panel reading is comfortable on phones and tablets.

### Milestone 9: Automatic Suggestions

- Build rectangular frame detector.
- Add reading-order suggestion.
- Add numbered debug overlay.
- Preserve manual edits.
- Add explicit reset and rerun actions.

Done when most conventional pages produce useful editable starting frames.

### Milestone 10: Library and Production Readiness

- Covers and thumbnails.
- Continue reading.
- Search and sorting.
- Delete and reprocess actions.
- Deployment packaging.
- Monitoring.
- Backups.
- Durable processing jobs only when required.
- Authentication only when hosted for multiple users.
- Object storage or PostgreSQL only when scale demonstrates the need.

---

## 37. MVP Success Criteria

The MVP succeeds when this workflow is reliable:

```text
Open application
  -> upload comic PDF or CBZ
  -> comic pages are processed
  -> read normally
  -> open frame editor
  -> define rectangle, polygon, full-page, and focus frames
  -> arrange the exact reading sequence
  -> preview the shared reader
  -> validate every page
  -> publish
  -> open guided mode
  -> move smoothly through every frame
  -> continue to the next page
  -> switch between panel, page, and vertical modes
  -> reopen at the exact saved frame
```

Release criteria:

- Representative PDF and CBZ files import correctly.
- Malicious and oversized inputs are safely rejected.
- A user can read an entire comic in normal mode.
- A creator can configure rectangle and polygon frames.
- A creator can add full-page and object-focus steps.
- Forward and backward guided navigation are exact reverses.
- Camera movement has no same-page reload or visible white flash.
- Masking works for rectangles and polygons.
- LTR and RTL navigation work.
- Viewport and orientation changes preserve the active frame.
- Reduced-motion mode works immediately.
- Exact frame progress is restored.
- Incomplete comics cannot be published.
- Invalid frame data falls back to full-page reading.
- Core Go and frontend tests pass.

The main product question remains:

> Does the creator-controlled panel-to-panel reading feel cinematic, natural, and reliable?

That question must guide early technical and product decisions.

---

## 38. First Development Target

Build this vertical slice first:

```text
POST /api/v1/comics
  -> stream and validate one PDF or CBZ
  -> safely render or extract page images
  -> create reader image and thumbnail
  -> commit comic and page metadata atomically
  -> return processing status
  -> display page 1 in normal page mode
```

Then add:

```text
Previous page
  -> next page
  -> fit page
  -> fit width
  -> adjacent-page preload
```

Next build:

```text
Rectangle frame editor
  -> normalized frame save
  -> full-page frame
  -> ordered frame list
  -> basic guided camera
```

Do not begin automatic frame detection until:

- Normal reading is stable.
- Manual frame editing works.
- Guided camera movement feels good.
- Creator preview and public reading use the same engine.

---

## 39. Exact Build Order

Implement in this order:

```text
1. Go project, React project, configuration, migrations
2. Secure PDF and CBZ upload
3. Page extraction or rendering
4. Reader images, thumbnails, cover
5. Normal page reader
6. Rectangle frame editor
7. Normalized frame persistence
8. Ordered frame sequence
9. Guided camera transform
10. Same-page transition
11. Cross-page transition
12. Shared creator preview
13. Publishing validation
14. Progress restoration
15. Reader modes
16. Mobile gestures
17. RTL behavior
18. Polygon frames
19. SVG masking
20. Focus, speech, object, and splash-page sequences
21. Performance and memory optimization
22. Automatic frame suggestions
23. Library and hosted-user features
```

---

## 40. Final Vision

The finished application should feel like a cinematic interactive comic reader rather than an archive or PDF viewer:

```text
Comic library
  -> open comic
  -> choose panel, page, or vertical mode
  -> restore exact progress
  -> show creator-defined first frame
  -> smooth pan and zoom
  -> highlight dialogue, character, or object
  -> move through irregular and rectangular panels
  -> transition cleanly to the next page
  -> continue without reloading the page image
```

The architecture must remain simple enough for import formats, storage, frame detection, publishing, and frontend animation to evolve independently.

The core implementation rule is:

> Keep the complete high-resolution page loaded, and move a virtual camera through an ordered collection of creator-defined normalized frames.

---

## 41. Current Implementation Status

This section records behavior implemented in the current codebase. Earlier sections describe the complete product target; this section distinguishes working behavior from future work.

### 41.1 Supported Imports

The application currently imports:

- CBZ through Go's `archive/zip`.
- CBR through the pure-Go `rardecode` library.
- PDF through Poppler's `pdftocairo` and `pdfinfo` commands.

PDF support requires:

```sh
sudo apt install poppler-utils
```

Imports run asynchronously after the upload completes. The browser reports network-upload progress, then polls persisted backend processing progress through extraction, rendering, detection, and publishing.

The import control displays:

- Current phase.
- Combined percentage.
- A clockwise filling border.
- Processing errors returned by the backend.

### 41.2 Automatic Panel Detection

Newly imported pages are analyzed automatically before the comic becomes ready.

The current detector is implemented in pure Go and does not require OpenCV. It:

1. Decodes the page image.
2. Creates a bounded analysis image with a maximum dimension of 900 pixels.
3. Searches for full-span low-activity and low-variance gutter or border bands.
4. Supports white, black, and colored separators.
5. Recursively splits page regions horizontally and vertically.
6. Rejects tiny or unreliable regions.
7. Limits recursion depth and frames per page.
8. Orders results top-to-bottom and left-to-right.
9. Converts rectangles into normalized rich frame metadata.
10. Falls back to one full-page frame when no reliable split is found.

Detection progress is stored using the `detecting panels` processing phase.

Detection endpoints:

```text
POST /api/v1/comics/{comicID}/pages/{pageNumber}/detect
POST /api/v1/comics/{comicID}/pages/{pageNumber}/detect?reset=true
POST /api/v1/comics/{comicID}/detect
POST /api/v1/comics/{comicID}/detect?reset=true
```

Default behavior preserves pages containing `manual` or `manual_edited` frames. Page-level detection returns `409 Conflict` when manual work exists. Comic-wide detection skips those pages and reports their page numbers. The `reset=true` option explicitly permits replacement.

The frame editor provides an **Auto detect** action. It asks for confirmation before replacing manual work.

Current detector limitations:

- It works best when panels are separated by visible gutters or long borders.
- Borderless, overlapping, highly irregular, and artwork-crossing layouts may require manual correction.
- Detection generates a creator-editable starting sequence; it is not treated as infallible publication metadata.
- Future OpenCV or ML detectors may replace the implementation without changing the reader contract.

### 41.3 Rich Frame Metadata

Persisted frames currently support:

- Rectangle and polygon shapes.
- `full_page`, `panel`, `focus`, `speech`, and `object` frame types.
- Normalized coordinates and polygon points.
- `contain` and `cover` fit modes.
- Per-frame padding.
- Per-frame mask opacity.
- Per-frame transition duration and easing.
- Enabled and disabled states.
- `detected`, `manual`, and `manual_edited` sources.
- Consecutive one-based reading order.

Frame APIs use optimistic page revisions:

```text
GET  /api/v1/comics/{comicID}/pages/{pageNumber}/frames
PUT  /api/v1/comics/{comicID}/pages/{pageNumber}/frames
POST /api/v1/comics/{comicID}/pages/{pageNumber}/frames/full-page
```

Frame retrieval response:

```json
{
  "revision": 4,
  "frames": []
}
```

Frame replacement request:

```json
{
  "revision": 4,
  "frames": []
}
```

A stale revision returns `409 Conflict` and does not overwrite newer edits.

### 41.4 Creator Frame Editor

The current editor supports:

- Automatic detection for the current page.
- Rectangle creation.
- Polygon creation.
- Full-page frame creation.
- Frame duplication and deletion.
- Earlier and later ordering.
- Undo and redo history.
- Debounced auto-save after completed changes.
- Explicit save status and manual save.
- Unsaved-change warning.
- Frame name, type, fit, padding, mask, transition, and enabled properties.
- Numbered debug outlines.

Rectangle interaction includes:

- A center handle for moving the complete frame.
- Four corner resize handles.
- Top, right, bottom, and left edge resize handles.
- Page-boundary constraints.
- A minimum valid frame size.

Polygon interaction includes:

- Numbered draggable vertex handles on the canvas.
- A center diamond for moving the complete polygon.
- Adding and removing points.
- Numeric normalized point editing as an accessible fallback.
- Page-boundary constraints for vertices and complete-polygon movement.

### 41.5 Reader Behavior

The reader currently supports:

- Panel mode using enabled creator-defined frames.
- Page mode using one complete page at a time.
- Vertical mode using a continuous page stream.
- Rectangle and polygon camera bounds.
- `contain` and `cover` fitting.
- Per-frame padding, maximum zoom, duration, and easing.
- Rectangle and polygon SVG masks.
- Same-page pan and zoom without reloading the page image.
- Direct transition from the final frame to the next page's first frame.
- Keyboard, button, and tap-zone navigation.
- LTR and RTL directional controls.
- Reduced-motion behavior.
- Full-page fallback when enabled frame metadata is missing.

Exact progress stores and restores:

- Page number.
- Frame order.
- Reading mode.
- Reading direction.

Progress is persisted in SQLite and mirrored to browser storage as an anonymous fallback.

### 41.6 Library Covers

Comic list responses include:

```json
{
  "cover_url": "/api/v1/comics/comic_123/pages/1/image"
}
```

The first page currently serves as the comic cover. Library cards:

- Display the first page with cover cropping.
- Apply a gradient for readable title and page-count text.
- Limit long visible titles.
- Lazy-load cover images.
- Fall back to a text card when no page image is available.

Dedicated generated covers and thumbnails remain future work.

### 41.7 Comic Deletion

Each library card includes an × control in its top-right corner. The control is separate from the action that opens the comic.

Selecting × opens an accessible warning dialog containing:

- The comic title.
- A permanent-deletion warning.
- A **Close** action.
- A destructive **Delete** action.

Deletion endpoint:

```text
DELETE /api/v1/comics/{comicID}
```

Successful deletion removes:

- Comic database metadata.
- Page and frame records through foreign-key cascades.
- Reading progress.
- Stored comic files.
- Local browser progress for that comic.

The delete control remains visible on touch-only devices.

### 41.8 Missing Asset Behavior

SQLite metadata does not contain page-image bytes. A comic cannot be detected or read when its database records remain but its files under `storage/comics/{comicID}` are missing.

Symptoms include:

- Page-image API returning `404 Not Found`.
- Browser cache temporarily displaying an image that no longer exists on the backend.
- Automatic detection returning a page-asset or detection failure.

Missing page assets cannot be reconstructed from SQLite. The source CBZ, CBR, or PDF must be imported again.

Future hardening should:

- Validate page assets during startup.
- Mark inconsistent comics as failed.
- Return a specific `page_asset_missing` error.
- Offer a direct re-import action.
- Clean orphaned metadata and filesystem directories safely.

### 41.9 Current Verification

The implemented backend and frontend are verified with:

```sh
cd backend
go test -race ./...
go vet ./...

cd ../frontend
npm run build
```

Backend tests currently cover import behavior, archive safety, asynchronous progress, rich frame validation, polygon validation, revision conflicts, automatic detection, full-page fallback, and preservation of manual frames.

### 41.10 Optional AI Instance Segmentation

The codebase includes an optional Python FastAPI worker under `ai-service/`. It loads a custom one-class comic-panel segmentation checkpoint and implements:

```text
POST /internal/v1/panel-detection
GET  /health
```

Go enables the worker with:

```text
PANEL_READER_AI_URL=http://127.0.0.1:8090
PANEL_READER_AI_STORAGE_ROOT=/data
PANEL_READER_AI_TIMEOUT=30s
```

Detection behavior is:

```text
Page image
  -> call configured AI worker
  -> validate complete response in Go
  -> normalize and order rich frames
  -> persist confidence and model version
  -> use pure-Go detector if AI is unavailable or invalid
```

The worker returns polygon masks, bounding boxes, confidence scores, and model version metadata. Go rejects non-finite values, out-of-bounds coordinates, invalid polygons, invalid confidence, excessive panel counts, oversized responses, and malformed metadata before persistence.

The editor displays confidence using initial review bands:

- `>= 0.85`: likely correct.
- `0.50–0.84`: review recommended.
- `< 0.50`: low confidence.

These thresholds are presentation defaults and must be tuned against a representative validation set.

No trained checkpoint is included. A custom checkpoint must be trained from licensed polygon annotations and placed at `models/comic-panel-seg.pt`. The included `ai-service/train.py` and `dataset.example.yaml` provide a one-class YOLO segmentation training entry point.

Ultralytics' default licensing is AGPL-3.0. A proprietary deployment must comply with those terms, obtain an appropriate commercial licence, or replace the Python implementation with a commercially compatible detector such as Detectron2 Mask R-CNN. The Go integration depends only on the documented HTTP contract and can use either implementation.

#### Hosted Roboflow Provider

The AI adapter also supports hosted inference using:

```text
PANEL_AI_PROVIDER=roboflow
ROBOFLOW_API_URL=https://serverless.roboflow.com
ROBOFLOW_API_KEY=<secret environment value>
ROBOFLOW_MODEL_ID=comic-panel-detectors/7
ROBOFLOW_PANEL_CLASSES=panel,panels
```

The API key must never be stored in source code, React, committed environment files, documentation, logs, or Docker image layers. Exposed keys must be revoked and replaced.

Roboflow predictions are converted as follows:

```text
left = centerX - width / 2
top  = centerY - height / 2

normalizedX      = left / imageWidth
normalizedY      = top / imageHeight
normalizedWidth  = width / imageWidth
normalizedHeight = height / imageHeight
```

Segmentation points are preserved as normalized polygons when the provider returns them. Otherwise, the prediction becomes a rectangle frame. Cover and unrelated classes are filtered out. Empty, malformed, unauthorised, rate-limited, or unavailable hosted responses trigger the existing Go detector fallback.

Roboflow dataset attribution:

```text
Comic Panel Detectors Dataset by Personal
https://universe.roboflow.com/personal-ov9jg/comic-panel-detectors
CC BY 4.0: https://creativecommons.org/licenses/by/4.0/
```

Hosted inference sends page images to Roboflow and therefore is not local-first. Review provider pricing, retention, privacy, platform terms, dataset provenance, and underlying comic-image rights before production use.

The AI container mounts comic storage read-only and must run with a UID/GID able to traverse the host's private storage directories. Compose uses `PANEL_AI_UID` and `PANEL_AI_GID`; `make run ai` sets them from `id -u` and `id -g`. Missing files return `404`, inaccessible files return `403`, and malformed paths return `422` instead of leaking internal exceptions.
