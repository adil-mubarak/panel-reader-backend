from __future__ import annotations

import os
from pathlib import Path
from threading import Lock
from typing import Literal

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field


class DetectionRequest(BaseModel):
    comicId: str = Field(min_length=1, max_length=200)
    page: int = Field(ge=1)
    imagePath: str = Field(min_length=1)
    readingDirection: Literal["ltr", "rtl", "vertical"] = "ltr"


class Point(BaseModel):
    x: float
    y: float


class BoundingBox(BaseModel):
    x: float
    y: float
    width: float
    height: float


class DetectedPanel(BaseModel):
    confidence: float
    polygon: list[Point]
    boundingBox: BoundingBox


class DetectionResponse(BaseModel):
    width: int
    height: int
    modelVersion: str
    panels: list[DetectedPanel]


class ModelRuntime:
    def __init__(self) -> None:
        self._model = None
        self._roboflow_client = None
        self._lock = Lock()

    @property
    def provider(self) -> str:
        return os.environ.get("PANEL_AI_PROVIDER", "local").strip().lower()

    @property
    def model_path(self) -> Path:
        return Path(os.environ.get("PANEL_MODEL_PATH", "/models/comic-panel-seg.pt"))

    def model(self):
        if self._model is not None:
            return self._model
        with self._lock:
            if self._model is None:
                if not self.model_path.is_file():
                    raise RuntimeError(f"model checkpoint not found: {self.model_path}")
                from ultralytics import YOLO

                self._model = YOLO(str(self.model_path))
        return self._model

    def detect(self, image_path: Path) -> DetectionResponse:
        if self.provider == "roboflow":
            if os.environ.get("ROBOFLOW_WORKFLOW_ID", "").strip():
                return self.detect_roboflow_hybrid(image_path)
            return self.detect_roboflow(image_path)
        if self.provider == "roboflow_workflow":
            return self.detect_roboflow_workflow(image_path)
        if self.provider == "roboflow_hybrid":
            return self.detect_roboflow_hybrid(image_path)
        if self.provider != "local":
            raise RuntimeError(f"unsupported panel AI provider: {self.provider}")
        return self.detect_local(image_path)

    def detect_local(self, image_path: Path) -> DetectionResponse:
        from PIL import Image

        with Image.open(image_path) as image:
            width, height = image.size

        confidence_threshold = float(os.environ.get("PANEL_CONFIDENCE", "0.25"))
        image_size = int(os.environ.get("PANEL_IMAGE_SIZE", "1280"))
        device = os.environ.get("PANEL_DEVICE", "") or None
        results = self.model().predict(
            source=str(image_path),
            conf=confidence_threshold,
            imgsz=image_size,
            device=device,
            verbose=False,
        )

        panels: list[DetectedPanel] = []
        if results:
            result = results[0]
            boxes = result.boxes
            masks = result.masks
            count = len(boxes) if boxes is not None else 0
            for index in range(count):
                confidence = float(boxes.conf[index].item())
                x0, y0, x1, y1 = [float(value) for value in boxes.xyxy[index].tolist()]
                polygon: list[Point] = []
                if masks is not None and index < len(masks.xy):
                    polygon = simplify_polygon(masks.xy[index], width, height)
                if len(polygon) < 3:
                    polygon = [
                        Point(x=x0 / width, y=y0 / height),
                        Point(x=x1 / width, y=y0 / height),
                        Point(x=x1 / width, y=y1 / height),
                        Point(x=x0 / width, y=y1 / height),
                    ]
                panels.append(
                    DetectedPanel(
                        confidence=confidence,
                        polygon=polygon,
                        boundingBox=BoundingBox(
                            x=x0 / width,
                            y=y0 / height,
                            width=(x1 - x0) / width,
                            height=(y1 - y0) / height,
                        ),
                    )
                )

        return DetectionResponse(
            width=width,
            height=height,
            modelVersion=os.environ.get("PANEL_MODEL_VERSION", self.model_path.stem),
            panels=panels,
        )

    def roboflow_client(self):
        if self._roboflow_client is not None:
            return self._roboflow_client
        with self._lock:
            if self._roboflow_client is None:
                api_key = os.environ.get("ROBOFLOW_API_KEY", "").strip()
                if not api_key:
                    raise RuntimeError("ROBOFLOW_API_KEY is not configured")
                from inference_sdk import InferenceHTTPClient

                self._roboflow_client = InferenceHTTPClient(
                    api_url=os.environ.get("ROBOFLOW_API_URL", "https://serverless.roboflow.com"),
                    api_key=api_key,
                )
        return self._roboflow_client

    def detect_roboflow(self, image_path: Path) -> DetectionResponse:
        from PIL import Image

        with Image.open(image_path) as image:
            width, height = image.size
        model_id = os.environ.get("ROBOFLOW_MODEL_ID", "comic-panel-detectors/7").strip()
        if not model_id:
            raise RuntimeError("ROBOFLOW_MODEL_ID is not configured")
        result = self.roboflow_client().infer(str(image_path), model_id=model_id)
        return convert_roboflow_result(result, width, height, model_id)

    def detect_roboflow_workflow(self, image_path: Path) -> DetectionResponse:
        from PIL import Image

        with Image.open(image_path) as image:
            width, height = image.size
        workspace = os.environ.get("ROBOFLOW_WORKSPACE", "").strip()
        workflow_id = os.environ.get("ROBOFLOW_WORKFLOW_ID", "").strip()
        if not workspace or not workflow_id:
            raise RuntimeError("ROBOFLOW_WORKSPACE and ROBOFLOW_WORKFLOW_ID are required")
        result = self.roboflow_client().run_workflow(
            workspace_name=workspace,
            workflow_id=workflow_id,
            images={"image": str(image_path)},
            parameters={"classes": os.environ.get("ROBOFLOW_WORKFLOW_CLASSES", "Cover, Panels")},
            use_cache=True,
        )
        return convert_roboflow_workflow_result(result, width, height, workspace, workflow_id)

    def detect_roboflow_hybrid(self, image_path: Path) -> DetectionResponse:
        boxes: DetectionResponse | None = None
        masks: DetectionResponse | None = None
        box_error: Exception | None = None
        mask_error: Exception | None = None
        try:
            boxes = self.detect_roboflow(image_path)
        except Exception as error:
            box_error = error
        try:
            masks = self.detect_roboflow_workflow(image_path)
        except Exception as error:
            mask_error = error
        if boxes and masks:
            return refine_detections(boxes, masks)
        if masks:
            return masks
        if boxes:
            return boxes
        raise RuntimeError(f"Roboflow detector and segmentation workflow failed: {box_error}; {mask_error}")


def simplify_polygon(points, width: int, height: int) -> list[Point]:
    if len(points) < 3:
        return []
    maximum_points = int(os.environ.get("PANEL_MAX_POLYGON_POINTS", "64"))
    step = max(1, len(points) // maximum_points)
    selected = points[::step][:maximum_points]
    return [
        Point(
            x=min(1.0, max(0.0, float(point[0]) / width)),
            y=min(1.0, max(0.0, float(point[1]) / height)),
        )
        for point in selected
    ]


def convert_roboflow_result(result: object, width: int, height: int, model_id: str) -> DetectionResponse:
    if not isinstance(result, dict):
        raise RuntimeError("Roboflow returned an invalid response")
    image = result.get("image")
    if isinstance(image, dict):
        width = positive_int(image.get("width"), width)
        height = positive_int(image.get("height"), height)
    predictions = result.get("predictions", [])
    if not isinstance(predictions, list):
        raise RuntimeError("Roboflow predictions are invalid")

    configured_classes = {
        value.strip().casefold()
        for value in os.environ.get("ROBOFLOW_PANEL_CLASSES", "panel,panels").split(",")
        if value.strip()
    }
    threshold = float(os.environ.get("PANEL_CONFIDENCE", "0.25"))
    panels: list[DetectedPanel] = []
    for prediction in predictions:
        if not isinstance(prediction, dict):
            continue
        class_name = str(prediction.get("class", prediction.get("class_name", ""))).strip().casefold()
        if configured_classes and class_name not in configured_classes:
            continue
        try:
            confidence = float(prediction.get("confidence", prediction.get("score", 0)))
        except (TypeError, ValueError):
            continue
        if confidence < threshold or confidence > 1:
            continue
        polygon = roboflow_polygon(prediction_points(prediction), width, height)
        box = prediction_box(prediction, polygon, width, height)
        if box is None:
            continue
        panels.append(
            DetectedPanel(
                confidence=confidence,
                polygon=polygon,
                boundingBox=box,
            )
        )
    if not panels:
        raise RuntimeError("Roboflow returned no valid panel predictions")
    return DetectionResponse(
        width=width,
        height=height,
        modelVersion=f"roboflow/{model_id}",
        panels=panels,
    )


def convert_roboflow_workflow_result(result: object, width: int, height: int, workspace: str, workflow_id: str) -> DetectionResponse:
    predictions, image = find_workflow_predictions(result)
    payload: dict[str, object] = {"predictions": predictions}
    if image:
        payload["image"] = image
    return convert_roboflow_result(payload, width, height, f"workflow/{workspace}/{workflow_id}")


def find_workflow_predictions(value: object) -> tuple[list[dict], dict | None]:
    candidates: list[tuple[list[dict], dict | None]] = []

    def visit(node: object, inherited_image: dict | None = None) -> None:
        if isinstance(node, list):
            if node and all(isinstance(item, dict) and prediction_like(item) for item in node):
                candidates.append((node, inherited_image))
                return
            for item in node:
                visit(item, inherited_image)
            return
        if not isinstance(node, dict):
            return
        image = node.get("image") if isinstance(node.get("image"), dict) else inherited_image
        predictions = node.get("predictions")
        if isinstance(predictions, list) and (not predictions or all(isinstance(item, dict) for item in predictions)):
            candidates.append((predictions, image))
        for child in node.values():
            if child is not predictions and child is not image:
                visit(child, image)

    visit(value)
    if not candidates:
        raise RuntimeError("Roboflow workflow response contains no predictions")
    return max(candidates, key=lambda candidate: len(candidate[0]))


def prediction_like(value: dict) -> bool:
    return any(key in value for key in ("x", "points", "polygon", "mask"))


def prediction_points(prediction: dict) -> object:
    for key in ("points", "polygon", "segmentation"):
        value = prediction.get(key)
        if isinstance(value, list):
            return value
    mask = prediction.get("mask")
    if isinstance(mask, dict):
        for key in ("points", "polygon"):
            value = mask.get(key)
            if isinstance(value, list):
                return value
    return []


def prediction_box(prediction: dict, polygon: list[Point], width: int, height: int) -> BoundingBox | None:
    try:
        box_width = float(prediction["width"])
        box_height = float(prediction["height"])
        center_x = float(prediction["x"])
        center_y = float(prediction["y"])
        if box_width > 0 and box_height > 0:
            x0 = max(0.0, center_x - box_width / 2)
            y0 = max(0.0, center_y - box_height / 2)
            x1 = min(float(width), center_x + box_width / 2)
            y1 = min(float(height), center_y + box_height / 2)
            if x1 > x0 and y1 > y0:
                return BoundingBox(x=x0 / width, y=y0 / height, width=(x1 - x0) / width, height=(y1 - y0) / height)
    except (KeyError, TypeError, ValueError):
        pass
    if len(polygon) >= 3:
        xs, ys = [point.x for point in polygon], [point.y for point in polygon]
        x0, y0, x1, y1 = min(xs), min(ys), max(xs), max(ys)
        if x1 > x0 and y1 > y0:
            return BoundingBox(x=x0, y=y0, width=x1 - x0, height=y1 - y0)
    return None


def refine_detections(boxes: DetectionResponse, masks: DetectionResponse) -> DetectionResponse:
    remaining = list(masks.panels)
    refined: list[DetectedPanel] = []
    for detected in boxes.panels:
        matches = [(box_iou(detected.boundingBox, candidate.boundingBox), candidate) for candidate in remaining if len(candidate.polygon) >= 3]
        score, match = max(matches, default=(0.0, None), key=lambda item: item[0])
        if match is not None and score >= float(os.environ.get("ROBOFLOW_MASK_MATCH_IOU", "0.30")):
            refined.append(DetectedPanel(confidence=min(detected.confidence, match.confidence), polygon=match.polygon, boundingBox=match.boundingBox))
            remaining.remove(match)
        else:
            refined.append(detected)
    for candidate in remaining:
        if len(candidate.polygon) >= 3 and not any(box_iou(candidate.boundingBox, current.boundingBox) >= 0.60 for current in refined):
            refined.append(candidate)
    return DetectionResponse(width=boxes.width, height=boxes.height, modelVersion=f"{boxes.modelVersion}+{masks.modelVersion}", panels=refined)


def box_iou(left: BoundingBox, right: BoundingBox) -> float:
    x0, y0 = max(left.x, right.x), max(left.y, right.y)
    x1 = min(left.x + left.width, right.x + right.width)
    y1 = min(left.y + left.height, right.y + right.height)
    intersection = max(0.0, x1 - x0) * max(0.0, y1 - y0)
    union = left.width * left.height + right.width * right.height - intersection
    return intersection / union if union > 0 else 0.0


def roboflow_polygon(raw_points: object, width: int, height: int) -> list[Point]:
    if not isinstance(raw_points, list) or len(raw_points) < 3:
        return []
    points: list[Point] = []
    maximum_points = int(os.environ.get("PANEL_MAX_POLYGON_POINTS", "64"))
    step = max(1, len(raw_points) // maximum_points)
    for raw in raw_points[::step][:maximum_points]:
        try:
            if isinstance(raw, dict):
                x, y = float(raw["x"]), float(raw["y"])
            elif isinstance(raw, (list, tuple)) and len(raw) >= 2:
                x, y = float(raw[0]), float(raw[1])
            else:
                return []
        except (KeyError, TypeError, ValueError, IndexError):
            return []
        # Roboflow usually returns pixels, but accept already-normalized masks.
        if x > 1 or y > 1:
            x, y = x / width, y / height
        points.append(Point(x=min(1.0, max(0.0, x)), y=min(1.0, max(0.0, y))))
    return points if len(points) >= 3 else []


def positive_int(value: object, fallback: int) -> int:
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        return fallback
    return parsed if parsed > 0 else fallback


def safe_image_path(raw_path: str) -> Path:
    try:
        path = Path(raw_path).resolve(strict=True)
    except FileNotFoundError as error:
        raise HTTPException(status_code=404, detail="page image not found") from error
    except PermissionError as error:
        raise HTTPException(status_code=403, detail="page image is not accessible") from error
    except OSError as error:
        raise HTTPException(status_code=422, detail="page image path is invalid") from error
    roots = os.environ.get("PANEL_ALLOWED_IMAGE_ROOTS", "/data").split(os.pathsep)
    allowed = [Path(root).resolve() for root in roots if root]
    if not any(path == root or root in path.parents for root in allowed):
        raise HTTPException(status_code=403, detail="image path is outside allowed storage roots")
    if not path.is_file():
        raise HTTPException(status_code=404, detail="page image not found")
    return path


runtime = ModelRuntime()
app = FastAPI(title="Panel Reader AI", version="1.0.0")


@app.get("/health")
def health() -> dict[str, str | bool]:
    if runtime.provider in {"roboflow", "roboflow_workflow", "roboflow_hybrid"}:
        configured = bool(os.environ.get("ROBOFLOW_API_KEY", "").strip())
        return {
            "status": "ok" if configured else "credentials_missing",
            "provider": runtime.provider,
            "modelConfigured": configured,
            "modelVersion": f"roboflow/{os.environ.get('ROBOFLOW_MODEL_ID', 'comic-panel-detectors/7')}+workflow/{os.environ.get('ROBOFLOW_WORKFLOW_ID', '')}",
        }
    return {
        "status": "ok" if runtime.model_path.is_file() else "model_missing",
        "provider": "local",
        "modelConfigured": runtime.model_path.is_file(),
        "modelVersion": os.environ.get("PANEL_MODEL_VERSION", runtime.model_path.stem),
    }


@app.post("/internal/v1/panel-detection", response_model=DetectionResponse)
def panel_detection(request: DetectionRequest) -> DetectionResponse:
    path = safe_image_path(request.imagePath)
    try:
        return runtime.detect(path)
    except RuntimeError as error:
        raise HTTPException(status_code=503, detail=str(error)) from error
    except Exception as error:
        raise HTTPException(status_code=422, detail="panel inference failed") from error
