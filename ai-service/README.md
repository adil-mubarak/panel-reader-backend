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

For local checkpoint inference, build `Dockerfile.local`, which installs the substantially larger PyTorch and Ultralytics stack. If a selected provider is unavailable, Go safely uses its pure-Go gutter detector.

## Train

Prepare a YOLO segmentation dataset with one class named `panel`, then copy and edit the example dataset configuration:

```sh
cd ai-service
python -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
cp dataset.example.yaml dataset.yaml
python train.py --data dataset.yaml --base yolo11n-seg.pt --epochs 100 --image-size 1280
```

Copy the best checkpoint from the generated run into `../models/comic-panel-seg.pt`.

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

Roboflow center-based boxes are converted to normalized top-left rectangles. Segmentation `points`, when returned, are preserved as normalized polygon frames. Predictions outside `ROBOFLOW_PANEL_CLASSES` are ignored.

Dataset attribution:

```text
Comic Panel Detectors Dataset by Personal
https://universe.roboflow.com/personal-ov9jg/comic-panel-detectors
Licensed under CC BY 4.0
https://creativecommons.org/licenses/by/4.0/
```

The dataset licence does not independently prove ownership of every underlying comic image. Review provenance and Roboflow's hosted-inference terms before commercial use.
