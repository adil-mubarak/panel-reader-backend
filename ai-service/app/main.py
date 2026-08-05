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
    readingDirection: Literal["ltr", "rtl"] = "ltr"


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
            return self.detect_roboflow(image_path)
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
            confidence = float(prediction.get("confidence", 0))
            box_width = float(prediction["width"])
            box_height = float(prediction["height"])
            center_x = float(prediction["x"])
            center_y = float(prediction["y"])
        except (KeyError, TypeError, ValueError):
            continue
        if confidence < threshold or confidence > 1 or box_width <= 0 or box_height <= 0:
            continue
        x0 = max(0.0, center_x - box_width / 2)
        y0 = max(0.0, center_y - box_height / 2)
        x1 = min(float(width), center_x + box_width / 2)
        y1 = min(float(height), center_y + box_height / 2)
        if x1 <= x0 or y1 <= y0:
            continue
        polygon = roboflow_polygon(prediction.get("points"), width, height)
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
    if not panels:
        raise RuntimeError("Roboflow returned no valid panel predictions")
    return DetectionResponse(
        width=width,
        height=height,
        modelVersion=f"roboflow/{model_id}",
        panels=panels,
    )


def roboflow_polygon(raw_points: object, width: int, height: int) -> list[Point]:
    if not isinstance(raw_points, list) or len(raw_points) < 3:
        return []
    points: list[Point] = []
    maximum_points = int(os.environ.get("PANEL_MAX_POLYGON_POINTS", "64"))
    step = max(1, len(raw_points) // maximum_points)
    for raw in raw_points[::step][:maximum_points]:
        if not isinstance(raw, dict):
            return []
        try:
            x, y = float(raw["x"]), float(raw["y"])
        except (KeyError, TypeError, ValueError):
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
    if runtime.provider == "roboflow":
        configured = bool(os.environ.get("ROBOFLOW_API_KEY", "").strip())
        return {
            "status": "ok" if configured else "credentials_missing",
            "provider": "roboflow",
            "modelConfigured": configured,
            "modelVersion": f"roboflow/{os.environ.get('ROBOFLOW_MODEL_ID', 'comic-panel-detectors/7')}",
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
