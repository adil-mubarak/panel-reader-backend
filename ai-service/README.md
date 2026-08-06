# Panel Segmentation Service

FastAPI inference service for a custom one-class comic-panel instance-segmentation model.

## Model

Place a trained Ultralytics segmentation checkpoint at:

```text
models/comic-panel-seg.pt
```

No model weights or training comic pages are included in this repository. Train only with material you own or are authorised to use.

Ultralytics code and default model distribution use AGPL-3.0 terms. Review those terms before distribution. A proprietary product should obtain the appropriate Ultralytics licence or replace this service implementation with an Apache-2.0-compatible detector such as Detectron2 Mask R-CNN. The Go API contract does not depend on Ultralytics.

## Run Hosted Roboflow With Docker

From the repository root:

```sh
cp .env.example .env
# Put a newly rotated key in .env. Never reuse the exposed key.
make ai-build
make run ai
```

Health check:

```sh
curl http://127.0.0.1:8090/health
```

Run the Go backend against it:

```sh
make run backend-ai
```

The Compose service runs with `PANEL_AI_UID` and `PANEL_AI_GID` so it can read private page directories through the read-only `/data` mount. Set these to the output of `id -u` and `id -g`; `make run ai` supplies them automatically.

If logs report `PermissionError: /data/comics`, recreate the container rather than changing storage to world-writable permissions:

```sh
PANEL_AI_UID=$(id -u) PANEL_AI_GID=$(id -g) \
  docker compose up -d --force-recreate panel-ai
```

The default Docker image is lightweight and uses hosted Roboflow. It does not install PyTorch or Ultralytics. Configure `PANEL_AI_PROVIDER=roboflow`, `ROBOFLOW_API_KEY`, and `ROBOFLOW_MODEL_ID=comic-panel-detectors/7` in `.env`.

For local checkpoint inference, `compose.local.yaml` builds `Dockerfile.local`, installs the substantially larger PyTorch and Ultralytics stack, and forces the local provider. If a selected provider is unavailable, Go safely uses its pure-Go gutter detector.

```sh
make ai-local-build
make ai-local
make ai-health
```

With a checkpoint at `models/comic-panel-seg.pt`, health reports JSON containing `"status":"ok"`, `"provider":"local"`, and `"modelConfigured":true`. Without it, the HTTP health endpoint remains reachable but reports `"status":"model_missing"`; inference is unavailable until the checkpoint is installed.

## Train Locally

Training requires an authorised comic-page dataset and suitable compute, normally an NVIDIA GPU. This repository does not include or download a dataset, does not run training automatically, and cannot guarantee perfect panel detection. Review predictions in the creator editor.

In Roboflow, create an **Instance Segmentation** project with exactly one class named `panel`. Draw one polygon around each panel that should become a guided-reading frame. Include conventional, borderless, irregular, inset, and overlapping panels when they are intended reading units. Exclude covers, full-page art without distinct panels, speech bubbles, characters, page furniture, and ambiguous regions unless they genuinely represent a panel. Keep every page from the same comic in one split to prevent train/validation leakage. Export a generated version as **YOLOv8 Oriented Bounding Boxes** is not valid; choose **YOLOv8 Instance Segmentation** (YOLO segmentation), download the ZIP, and extract it without rearranging files.

A typical extraction is:

```text
comic-panels/
|-- train/images/ and train/labels/
|-- valid/images/ and valid/labels/
|-- test/images/ and test/labels/
`-- data.yaml
```

Labels must contain class ID plus polygon coordinates, not five-token object-detection boxes. Images with no panels are useful negatives and may have an absent or empty label file. Copy `dataset.example.yaml` to a convenient location and edit its `path`; relative paths resolve from the YAML file. The optional `test` split may be omitted, while `train` and `val` are required.

Set up the host environment and validate without reading image pixels:

```sh
make ai-venv
make dataset-validate DATASET=/absolute/path/to/data.yaml
```

Train only after validation succeeds:

```sh
make train-ai DATASET=/absolute/path/to/data.yaml BASE=yolo11n-seg.pt EPOCHS=100 IMAGE_SIZE=1280 BATCH=8 DEVICE=0
```

Equivalent direct usage is `ai-service/.venv/bin/python ai-service/train.py --data /absolute/path/to/data.yaml --base yolo11n-seg.pt --epochs 100 --image-size 1280`. The script also accepts `--workers`, `--patience`, `--seed`, `--project`, and `--name`, and prints the resolved best-checkpoint path. By default output is under `ai-service/runs/panel-segmentation/comic-panel-seg/weights/best.pt`, regardless of the current working directory. Copy it into the runtime mount:

```sh
cp ai-service/runs/panel-segmentation/comic-panel-seg/weights/best.pt models/comic-panel-seg.pt
make ai-local-build
make ai-local
```

The `BASE` checkpoint may be downloaded by Ultralytics when training starts; no weights are downloaded during validation or tests. Ultralytics and its model distribution are subject to the AGPL-3.0 licensing terms noted above.

For a Roboflow export containing extra classes, normalize it before validation. The preparation command hard-links images where possible, removes non-panel annotations, remaps `Panels` to `panel`, cleans duplicate polygon vertices, preserves empty pages as negatives, and writes attribution:

```sh
make dataset-prepare \
  DATASET_SOURCE=/path/to/roboflow-export \
  DATASET_OUTPUT=/path/to/normalized-dataset
make dataset-validate DATASET=/path/to/normalized-dataset/data.yaml
```

The local Docker image is pinned to official CPU-only PyTorch wheels so it remains usable on hosts without NVIDIA hardware. Full-resolution, multi-epoch training on thousands of pages still requires substantially more time than GPU training.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `PANEL_MODEL_PATH` | `/models/comic-panel-seg.pt` | Segmentation checkpoint |
| `PANEL_MODEL_VERSION` | Checkpoint filename | Persisted model identifier |
| `PANEL_ALLOWED_IMAGE_ROOTS` | `/data` | Allowed shared image roots |
| `PANEL_CONFIDENCE` | `0.25` | Inference confidence threshold |
| `PANEL_IMAGE_SIZE` | `1280` | YOLO inference image size |
| `PANEL_MAX_POLYGON_POINTS` | `64` | Maximum returned mask points |
| `PANEL_DEVICE` | automatic | `cpu`, CUDA device, or other supported device |

## API

```text
POST /internal/v1/panel-detection
```

The service accepts a page path under an allowed shared root and returns normalized masks, bounding boxes, confidence scores, and a model version. Go validates the complete response before saving any frame.

## Roboflow Model

The hosted provider defaults to:

```text
comic-panel-detectors/7
```

Detector selection follows the comic's explicit content type:

```text
comic   -> ROBOFLOW_COMIC_MODEL_ID (comic-panel-detectors/7)
manga   -> ROBOFLOW_MANGA_MODEL_ID (manga-test/2)
webtoon -> ROBOFLOW_WEBTOON_MODEL_ID, or the comic model when unset
```

The Manga specialist is an object-detection model, so its boxes still pass through the configured segmentation workflow for polygon refinement and then through the Go structural completeness pass. Manga reading order remains RTL; model selection does not reverse Previous/Next controls. `manga-test/2` is published under CC BY 4.0, but underlying image provenance should still be reviewed before commercial use. Version 1 is intentionally not combined with version 2 because their source images overlap.

Roboflow center-based boxes are converted to normalized top-left rectangles. Segmentation `points`, when returned, are preserved as normalized polygon frames. Predictions outside `ROBOFLOW_PANEL_CLASSES` are ignored.

### Hybrid Polygon Refinement

Set:

```text
PANEL_AI_PROVIDER=roboflow_hybrid
ROBOFLOW_WORKSPACE=adil-mubarak
ROBOFLOW_WORKFLOW_ID=general-segmentation-api-12
ROBOFLOW_WORKFLOW_CLASSES=Cover, Panels
ROBOFLOW_MASK_MATCH_IOU=0.30
```

Hybrid mode runs `comic-panel-detectors/7` for stable candidate boxes and the segmentation workflow for masks. A valid polygon replaces a rectangle only when their bounding boxes overlap by at least the configured IoU. Invalid or unmatched masks retain the detector rectangle. Valid unmatched panel masks may be added when they do not duplicate an accepted frame.

The workflow parser supports nested Roboflow output objects, center-based boxes, polygon-only predictions, dictionary points, and `[x, y]` point pairs. Cover predictions are filtered from the reading sequence.

Dataset attribution:

```text
Comic Panel Detectors Dataset by Personal
https://universe.roboflow.com/personal-ov9jg/comic-panel-detectors
Licensed under CC BY 4.0
https://creativecommons.org/licenses/by/4.0/
```

The dataset licence does not independently prove ownership of every underlying comic image. Review provenance and Roboflow's hosted-inference terms before commercial use.

An optional public instance-segmentation source evaluated by this project is `Comic Book Panel Detection 2` version 2, published as CC BY 4.0. Its export contains `Cover` and `Panels`; use `dataset-prepare` to retain only panel polygons. The published version also contains rotation augmentation, so a model trained from it must be evaluated against unrotated held-out pages before deployment.
