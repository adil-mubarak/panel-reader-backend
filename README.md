# Panel Reader

Local-first CBZ, CBR, and PDF reader with a Go backend and React frontend.

Reader modes include animated guided frames, single pages, and continuous vertical book scrolling. Imports are explicitly classified as Comic (LTR), Manga (RTL), or Webtoon (vertical), and the classification can be changed later without reordering manually edited frames.

The reader keeps a bounded decoded-page cache, crossfades already-decoded pages, supports direct page jumps and transition speed presets, and provides 25%-500% zoom with fit-page, fit-width, wheel, touch-drag, and pan controls.

Optional custom instance-segmentation is provided by `ai-service/`. See `ai-service/README.md` for model training, licensing, and runtime setup. Without a configured AI checkpoint, the Go backend uses its built-in gutter detector.

The frame editor includes conservative splash/no-panel fallback, adaptive rectangle/polygon output, and hybrid completeness checks. When hosted AI omits a bordered panel, the backend also runs its structural detector, recovers reliable missing candidates, and reports AI, structural, and recovered counts for creator review. Existing pages can be reprocessed with **Auto detect**.

The editor also includes detection quality warnings, split/merge tools, page approval, and approved-page YOLO/COCO exports for building an authorised correction dataset.

PDF import requires Poppler's `pdftocairo` command:

```sh
sudo apt install poppler-utils
```

CBR extraction is implemented in Go and does not require an external command.

## Development

Run the API:

```sh
make run backend
```

Run the API under Delve and attach a debugger to port `2345`:

```sh
make debug backend
```

For VS Code, select `Launch Panel Reader backend` and press `F5`. VS Code starts Delve itself, so `make debug backend` is not needed when using this launch configuration.

Run the frontend in another terminal:

```sh
make run frontend
```

Open `http://localhost:5173`. Runtime data is stored under `storage/` by default.

## Verification

```sh
cd backend && go test ./...
cd frontend && npm run build
```
