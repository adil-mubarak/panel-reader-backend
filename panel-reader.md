Animated Comic Web Reader --- Development Plan

1. Project Goal

Build a web application with a Go backend that can read CBZ, CBR,and PDF comic files and provide a guided, panel-by-panel readingexperience inspired by modern digital comic readers.

The key experience is:

Full page → Panel 1 → smooth pan/zoom → Panel 2 → Panel 3 → nextpage

The first version should be local-first and focused on making thereader work well before adding accounts, cloud storage, or advanced AI.

2. Core Features

MVP

Upload/open .cbz comics

Extract comic pages

Display comics in normal page mode

Automatically detect rectangular comic panels

Determine panel reading order

Guided panel-by-panel reading

Smooth pan, zoom, and slide animations

Keyboard navigation

Fullscreen reading

Manual panel correction/editor

Later

.cbr support

.pdf support

Comic library with covers

Reading progress

Continue reading

Manga right-to-left mode

Double-page spreads

Better detection for irregular/borderless panels

User accounts

Cloud/object storage

Mobile/PWA support

3. Recommended Stack

Backend

Go

Chi for HTTP routing

Go standard library where possible

SQLite initially

PostgreSQL later if needed

Local filesystem storage initially

Frontend

React

TypeScript

Vite

CSS transforms for reader camera movement

Framer Motion for UI transitions where useful

Comic Processing

CBZ → Go archive/zip

CBR → RAR extraction library/tool

PDF → render pages to images

Processed page format → WebP/JPEG

Panel Detection

Start with traditional computer vision.

Later consider: - OpenCV - ONNX - ML-based panel detection

Do not start with AI unless traditional detection provesinsufficient.

4. High-Level Architecture

┌─────────────────────────────┐
│          Browser            │
│                             │
│ React + TypeScript          │
│ Animated Comic Reader       │
└──────────────┬──────────────┘
               │
               │ REST API
               ▼
┌─────────────────────────────┐
│          Go API             │
│                             │
│ Upload / Library            │
│ Comic Processing            │
│ Reader Metadata             │
│ Progress                    │
└──────────────┬──────────────┘
               │
     ┌─────────┼─────────┐
     ▼         ▼         ▼
    CBZ       CBR       PDF
     │         │         │
     └─────────┼─────────┘
               ▼
          Page Images
               │
               ▼
       Panel Detection
               │
               ▼
     Panel Coordinates
               │
               ▼
        Reader Frontend

5. Suggested Repository Structure

comic-reader/
├── backend/
│   ├── cmd/
│   │   └── api/
│   │       └── main.go
│   │
│   ├── internal/
│   │   ├── comic/
│   │   ├── archive/
│   │   ├── processor/
│   │   ├── panel/
│   │   ├── reader/
│   │   ├── storage/
│   │   └── http/
│   │
│   └── go.mod
│
├── frontend/
│   ├── src/
│   │   ├── api/
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── pages/
│   │   ├── reader/
│   │   └── types/
│   │
│   └── package.json
│
├── storage/
│   ├── comics/
│   ├── pages/
│   ├── covers/
│   └── metadata/
│
├── docker-compose.yml
└── README.md

6. Development Phases

Phase 1 --- Project Foundation

Goal

Create the Go API and React frontend and establish communication betweenthem.

Tasks

Initialize Go project

Add Chi router

Create health endpoint

Initialize React + TypeScript + Vite

Configure frontend API client

Add development configuration

Create storage directories

Initial endpoint

GET /api/health

Expected response:

{
  "status": "ok"
}

Done When

Opening the frontend successfully communicates with the Go backend.

Phase 2 --- CBZ Support

Start with CBZ because it is simply a ZIP archive containing images.

Processing Pipeline

comic.cbz
    ↓
validate file
    ↓
generate comic ID
    ↓
extract ZIP
    ↓
find image files
    ↓
sort pages naturally
    ↓
generate page metadata
    ↓
generate cover
    ↓
save comic

Important

Correct page sorting is necessary.

For example:

1.jpg
2.jpg
3.jpg
10.jpg

must not become:

1.jpg
10.jpg
2.jpg
3.jpg

Use natural/numeric sorting.

API

POST /api/comics
GET  /api/comics
GET  /api/comics/:id
GET  /api/comics/:id/pages
GET  /api/comics/:id/pages/:page

Suggested Models

type Comic struct {
    ID        string `json:"id"`
    Title     string `json:"title"`
    Cover     string `json:"cover"`
    PageCount int    `json:"page_count"`
    Pages     []Page `json:"pages"`
}

type Page struct {
    Number int     `json:"number"`
    Image  string  `json:"image"`
    Width  int     `json:"width"`
    Height int     `json:"height"`
    Panels []Panel `json:"panels"`
}

type Panel struct {
    Order  int `json:"order"`
    X      int `json:"x"`
    Y      int `json:"y"`
    Width  int `json:"width"`
    Height int `json:"height"`
}

Done When

You can upload a CBZ and retrieve every page through the API.

Phase 3 --- Normal Comic Reader

Before guided panels, build a reliable page reader.

Features

Previous page

Next page

Fit width

Fit page

Zoom

Fullscreen

Hide/show controls

Page counter

Preload adjacent pages

Keyboard Controls

←       Previous page
→       Next page
F       Fullscreen
Esc     Exit fullscreen / controls

Preloading

Keep these ready:

previous page
current page
next page

This reduces visible loading between pages.

Done When

A complete CBZ can comfortably be read in normal page mode.

Phase 4 --- Panel Detection

Goal

Convert a comic page into ordered panel coordinates.

Example:

┌──────────────────────────────┐
│           PANEL 1            │
├───────────────┬──────────────┤
│               │              │
│    PANEL 2    │   PANEL 3    │
│               │              │
├───────────────┴──────────────┤
│           PANEL 4            │
└──────────────────────────────┘

Store:

{
  "page": 1,
  "panels": [
    {
      "order": 1,
      "x": 20,
      "y": 20,
      "width": 960,
      "height": 400
    },
    {
      "order": 2,
      "x": 20,
      "y": 440,
      "width": 470,
      "height": 600
    },
    {
      "order": 3,
      "x": 510,
      "y": 440,
      "width": 470,
      "height": 600
    },
    {
      "order": 4,
      "x": 20,
      "y": 1060,
      "width": 960,
      "height": 600
    }
  ]
}

Detection Pipeline

page image
    ↓
resize for analysis
    ↓
grayscale
    ↓
threshold
    ↓
edge detection
    ↓
contour detection
    ↓
rectangle candidates
    ↓
filter noise
    ↓
merge/filter overlapping regions
    ↓
determine reading order
    ↓
save panel coordinates

Version 1 Scope

Support conventional rectangular panels first.

Do not attempt to perfectly handle: - Borderless panels - Charactersextending outside panel borders - Highly irregular panels - Overlappingpanels - Splash pages - Complex manga layouts

Those come later.

Done When

Most conventional comic pages produce usable panel rectangles in thecorrect order.

Phase 5 --- Dynamic Panel Reader

This is the project's core feature.

Important Design Decision

Do not create separate cropped images for every panel.

Display the original high-resolution page and move a virtual cameraaround it.

Original page

┌──────────────────────────────┐
│           PANEL 1            │
├───────────────┬──────────────┤
│ PANEL 2       │ PANEL 3      │
├───────────────┴──────────────┤
│           PANEL 4            │
└──────────────────────────────┘

                ↓

Viewport zooms into PANEL 1

                ↓

smooth pan/zoom

                ↓

PANEL 2

                ↓

PANEL 3

Frontend State

interface ReaderState {
    page: number
    panel: number
    mode: "page" | "panel"
    fullscreen: boolean
}

Camera Calculation

For each panel calculate:

scale
translateX
translateY

based on: - viewport width - viewport height - page width - pageheight - panel rectangle - desired padding

Apply the result using CSS transforms.

.comic-page {
    transform-origin: top left;
    will-change: transform;

    transition:
        transform 450ms cubic-bezier(.22, .61, .36, 1);
}

Navigation

Full Page
    ↓
Panel 1
    ↓
Panel 2
    ↓
Panel 3
    ↓
...
    ↓
Full Page / Next Page

Done When

Pressing the navigation key smoothly moves the camera between detectedpanels.

Phase 6 --- Animation Polish

The movement should feel intentional rather than like a basic imagezoom.

Horizontal Panels

[PANEL 1] [PANEL 2]

     ─────────→

Favor horizontal movement.

Vertical Panels

[PANEL 1]
    │
    ↓
[PANEL 2]

Favor vertical movement.

Different Panel Sizes

Move and zoom simultaneously.

large panel
     ↓
pan + zoom
     ↓
small panel

Animation Goals

Smooth

Fast enough not to feel slow

No flashing

No image reload during movement

No abrupt scale changes

Respect reduced-motion browser settings

Suggested starting duration:

350–500 ms

Tune later through testing.

Phase 7 --- Manual Panel Editor

Automatic detection will never be perfect.

A manual editor should therefore be a core feature rather than anafterthought.

Features

Display full page

Show detected rectangles

Drag panel

Resize panel

Delete panel

Add panel

Reorder panels

Save changes

Reset to automatic detection

Example:

┌─────────────────────────────────┐
│                                 │
│   ┌──────── PANEL 1 ────────┐   │
│   │                         │   │
│   └─────────────────────────┘   │
│                                 │
│   ┌── PANEL 2 ──┐ ┌ PANEL 3 ┐   │
│   │             │ │         │   │
│   │             │ │         │   │
│   └─────────────┘ └─────────┘   │
│                                 │
└─────────────────────────────────┘

API

PUT /api/comics/:comicId/pages/:page/panels

Done When

Incorrect panel detection can be fixed without modifying files manually.

Phase 8 --- CBR Support

Once CBZ is stable, add CBR.

Pipeline

comic.cbr
    ↓
RAR extraction
    ↓
images
    ↓
same pipeline used by CBZ

The important architectural rule is:

After extraction, the reader should not care whether the original filewas CBZ, CBR, or PDF.

Everything becomes pages.

Phase 9 --- PDF Support

Pipeline

comic.pdf
    ↓
PDF renderer
    ↓
page images
    ↓
page metadata
    ↓
panel detection
    ↓
reader

Avoid making PDF-specific logic leak into the reader.

Internally:

CBZ ─┐
CBR ─┼──→ Page Images ──→ Panels ──→ Reader
PDF ─┘

7. Unified Internal Comic Format

Every imported comic should eventually look like:

{
  "id": "comic_123",
  "title": "Example Comic",
  "cover": "/covers/comic_123.webp",
  "pageCount": 120,
  "readingDirection": "ltr",
  "pages": [
    {
      "number": 1,
      "image": "/pages/comic_123/001.webp",
      "width": 1600,
      "height": 2400,
      "panels": []
    }
  ]
}

This keeps the frontend independent of the source format.

8. Reader Modes

Support two main modes.

Page Mode

Traditional comic reading.

Page 1
  ↓
Page 2
  ↓
Page 3

Dynamic Panel Mode

Page overview
     ↓
Panel 1
     ↓
Panel 2
     ↓
Panel 3
     ↓
Next page

Later add:

Manga Mode

Right → Left

Double Page Mode

┌────────────┬────────────┐
│   PAGE 10  │   PAGE 11  │
└────────────┴────────────┘

9. Reader Controls

Suggested controls:

← / →        Previous / next panel or page
Space        Next panel
F            Fullscreen
M            Toggle page/panel mode
Esc          Exit fullscreen / show controls
+ / -        Zoom

Mouse/touch: - Click right side → next - Click left side → previous -Mouse wheel → zoom/scroll where appropriate - Swipe → navigation -Double click → fullscreen or zoom

10. Library

Add the library only after the reader itself is working.

Library Screen

My Comics

┌──────────┐ ┌──────────┐ ┌──────────┐
│          │ │          │ │          │
│  COVER   │ │  COVER   │ │  COVER   │
│          │ │          │ │          │
└──────────┘ └──────────┘ └──────────┘

  Comic A      Comic B      Comic C

   45%          New          82%

Features

Cover

Title

File format

Page count

Last opened

Reading percentage

Continue reading

Delete

Reprocess

Edit metadata

11. Reading Progress

Store:

comic_id
page
panel
reader_mode
updated_at

Example:

{
  "comic_id": "comic_123",
  "page": 47,
  "panel": 3,
  "mode": "panel"
}

When reopening the comic:

Continue reading?

Page 47 — Panel 3

12. Processing Strategy

Comic processing can become expensive.

Do not make the upload request wait for everything.

Eventually use:

Upload
   ↓
Comic created
   ↓
Background processing
   │
   ├── extract pages
   ├── dimensions
   ├── cover
   ├── thumbnails
   └── panel detection
   ↓
Ready

Possible statuses:

uploaded
extracting
processing
detecting_panels
ready
failed

For the first MVP, synchronous processing is acceptable.

Move to background jobs once processing becomes slow.

13. Storage Layout

Example:

storage/
└── comics/
    └── comic_123/
        ├── original/
        │   └── comic.cbz
        │
        ├── pages/
        │   ├── 001.webp
        │   ├── 002.webp
        │   └── 003.webp
        │
        ├── thumbnails/
        │   ├── 001.webp
        │   └── 002.webp
        │
        ├── cover.webp
        └── metadata.json

Later this can move to S3-compatible object storage without changing thereader architecture.

14. Performance

Comics can contain hundreds of high-resolution pages, so performanceneeds attention.

Do

Lazy-load pages

Preload adjacent pages

Generate thumbnails

Cache panel metadata

Serve optimized images

Use WebP where appropriate

Avoid loading the whole comic into browser memory

Use CSS transforms instead of generating images during navigation

Do Not

Load 200 full-resolution pages simultaneously

Prefer:

Page N-1
Page N
Page N+1

and dynamically load more as required.

15. Security

Uploaded archives are untrusted files.

Validate: - File type - Archive size - Number of extracted files -Extracted total size - Image formats - File paths - PDF size

Protect against: - ZIP bombs - Path traversal / Zip Slip - Extremelylarge images - Malformed archives - Malicious filenames

Never extract paths such as:

../../windows/system32/example

outside the comic storage directory.

16. Advanced Panel Detection

Only tackle this after the basic detector works.

Version 1

Rectangular bordered panels.

Version 2

Improve: - gutter detection - nested contours - overlapping rectangles -splash pages - unusual gutters

Version 3

Consider ML for: - borderless panels - irregular shapes - artworkcrossing boundaries - complicated layouts

Possible architecture:

Go API
   │
   ├── storage
   ├── comics
   ├── reader
   └── processing
          │
          ▼
    Panel Detection
          │
      ┌───┴────┐
      │        │
   OpenCV     ONNX

Keep panel detection behind an interface so the implementation canchange later.

17. Suggested Go Interfaces

type ComicExtractor interface {
    Extract(path string, destination string) ([]Page, error)
}

Implementations:

CBZExtractor
CBRExtractor
PDFExtractor

Panel detector:

type PanelDetector interface {
    Detect(imagePath string) ([]Panel, error)
}

Storage:

type ComicRepository interface {
    Create(comic Comic) error
    Get(id string) (Comic, error)
    List() ([]Comic, error)
    Update(comic Comic) error
    Delete(id string) error
}

This prevents file-format and detection logic from becoming tightlycoupled.

18. API Roadmap

Comics

POST   /api/comics
GET    /api/comics
GET    /api/comics/:id
DELETE /api/comics/:id

Pages

GET /api/comics/:id/pages
GET /api/comics/:id/pages/:page

Panels

GET /api/comics/:id/pages/:page/panels
PUT /api/comics/:id/pages/:page/panels
POST /api/comics/:id/pages/:page/detect

Progress

GET /api/comics/:id/progress
PUT /api/comics/:id/progress

19. Development Milestones

Milestone 1 --- File to Browser

Target:

CBZ
 ↓
Go
 ↓
Extract
 ↓
Browser
 ↓
Display page

Do not move forward until this is stable.

Milestone 2 --- Complete Page Reader

Target:

CBZ
 ↓
All pages
 ↓
Previous / Next
 ↓
Fullscreen
 ↓
Smooth normal reading

Milestone 3 --- Panel Detection

Target:

Page
 ↓
Detector
 ↓
Panel rectangles
 ↓
Display rectangles as debug overlay

Build a debug mode that shows:

[1]
[2]
[3]
[4]

over detected panels.

This will make detector development much easier.

Milestone 4 --- Dynamic Reader

Target:

Panel 1
   ↓
animated camera
   ↓
Panel 2
   ↓
animated camera
   ↓
Panel 3

This is the first major product milestone.

Milestone 5 --- Panel Editor

Allow users to correct detection.

At this point the application should already be genuinely usable.

Milestone 6 --- CBR + PDF

Add the remaining input formats while keeping the reader unchanged.

Milestone 7 --- Library

Add: - Covers - Metadata - Progress - Continue reading - Search/sort

Milestone 8 --- Advanced Reader

Add: - Manga RTL - Double-page spreads - Touch gestures - Bettertransitions - Reader preferences - Per-comic settings

Milestone 9 --- Advanced Detection

Improve detection only after collecting examples of where the existingdetector fails.

Milestone 10 --- Production

Add: - Docker - Production configuration - PostgreSQL if necessary -Object storage if necessary - Background jobs - Authentication ifhosting publicly - Monitoring/logging

20. Recommended MVP Scope

The first real release should contain only:

CBZ upload

CBZ extraction

Comic page reader

Automatic rectangular panel detection

Panel debug overlay

Dynamic panel mode

Smooth pan/zoom

Previous/next panel

Fullscreen

Manual panel correction

Local storage

Do not initially build: - Accounts - Payments - Social features -Recommendations - Cloud sync - AI panel detection - Mobile apps - CBR -PDF

Prove the core experience first.

21. MVP Success Criteria

The MVP is successful when this workflow feels good:

Open application
       ↓
Upload comic.cbz
       ↓
Comic appears
       ↓
Open comic
       ↓
Reader shows page
       ↓
Enable Dynamic Panel
       ↓
Panel 1
       ↓
smooth animation
       ↓
Panel 2
       ↓
smooth animation
       ↓
Panel 3
       ↓
next page

The most important question is not:

"How many formats does the application support?"

It is:

"Does panel-by-panel reading feel good?"

That should guide the early development decisions.

22. Suggested Build Order

1. Go API
        ↓
2. React frontend
        ↓
3. CBZ upload
        ↓
4. CBZ extraction
        ↓
5. Page API
        ↓
6. Normal reader
        ↓
7. Panel detector
        ↓
8. Detection debug overlay
        ↓
9. Panel reading state
        ↓
10. Camera transform calculation
        ↓
11. Smooth panel animations
        ↓
12. Manual panel editor
        ↓
13. Reading progress
        ↓
14. Comic library
        ↓
15. CBR support
        ↓
16. PDF support
        ↓
17. Better panel detection
        ↓
18. Manga / double-page modes
        ↓
19. Performance optimization
        ↓
20. Production deployment

23. First Development Target

For the first development session, build only this:

POST /api/comics
       ↓
receive .cbz
       ↓
save file
       ↓
extract images
       ↓
sort pages
       ↓
return comic metadata
       ↓
React displays page 1

Once that works, add previous/next page navigation.

Only after the normal reader works reliably should panel detectionbegin.

Final Product Vision

The finished application should feel like a cinematic comic reader,not merely an archive viewer.

Comic Library
      ↓
Open Comic
      ↓
Page Mode ───────────── Dynamic Panel Mode
                              ↓
                          Panel 1
                              ↓
                       smooth camera
                              ↓
                          Panel 2
                              ↓
                       smooth camera
                              ↓
                          Panel 3
                              ↓
                         Next Page

Keep the architecture simple enough that the panel detector, storagesystem, frontend animations, and supported comic formats can evolveindependently.