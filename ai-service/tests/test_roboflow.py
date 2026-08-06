import os
import unittest
from unittest.mock import MagicMock, patch

from fastapi import HTTPException

from app.main import (
    BoundingBox,
    DetectedPanel,
    DetectionRequest,
    DetectionResponse,
    ModelRuntime,
    Point,
    convert_roboflow_result,
    convert_roboflow_workflow_result,
    refine_detections,
    safe_image_path,
)


class RoboflowConversionTests(unittest.TestCase):
    def test_detection_request_accepts_vertical_reading_direction(self):
        request = DetectionRequest(
            comicId="comic-1",
            page=1,
            imagePath="/data/comics/page.jpg",
            readingDirection="vertical",
        )
        self.assertEqual(request.readingDirection, "vertical")

    def test_detection_request_defaults_content_type_to_comic(self):
        request = DetectionRequest(comicId="comic-1", page=1, imagePath="/data/page.jpg")
        self.assertEqual(request.contentType, "comic")

    def test_converts_center_box_and_filters_cover(self):
        result = {
            "image": {"width": 1000, "height": 2000},
            "predictions": [
                {"class": "Panels", "confidence": 0.9, "x": 300, "y": 400, "width": 400, "height": 500},
                {"class": "Cover", "confidence": 0.99, "x": 500, "y": 1000, "width": 1000, "height": 2000},
            ],
        }
        with patch.dict(os.environ, {"ROBOFLOW_PANEL_CLASSES": "panel,panels", "PANEL_CONFIDENCE": "0.25"}):
            response = convert_roboflow_result(result, 1000, 2000, "comic-panel-detectors/7")
        self.assertEqual(len(response.panels), 1)
        self.assertAlmostEqual(response.panels[0].boundingBox.x, 0.1)
        self.assertAlmostEqual(response.panels[0].boundingBox.y, 0.075)
        self.assertEqual(response.modelVersion, "roboflow/comic-panel-detectors/7")

    def test_preserves_segmentation_points(self):
        result = {
            "predictions": [{
                "class": "panel",
                "confidence": 0.8,
                "x": 50,
                "y": 50,
                "width": 80,
                "height": 80,
                "points": [{"x": 10, "y": 10}, {"x": 90, "y": 10}, {"x": 50, "y": 90}],
            }],
        }
        response = convert_roboflow_result(result, 100, 100, "model/1")
        self.assertEqual(len(response.panels[0].polygon), 3)
        self.assertAlmostEqual(response.panels[0].polygon[0].x, 0.1)

    def test_rejects_empty_valid_predictions(self):
        with self.assertRaises(RuntimeError):
            convert_roboflow_result({"predictions": []}, 100, 100, "model/1")

    def test_inaccessible_image_returns_forbidden(self):
        with patch("app.main.Path.resolve", side_effect=PermissionError("denied")):
            with self.assertRaises(HTTPException) as raised:
                safe_image_path("/data/comics/page.jpg")
        self.assertEqual(raised.exception.status_code, 403)

    def test_converts_nested_workflow_polygon(self):
        result = [{
            "segmentation": {
                "image": {"width": 200, "height": 400},
                "predictions": [{
                    "class_name": "Panels",
                    "score": 0.88,
                    "polygon": [[20, 40], [180, 40], [170, 200], [30, 200]],
                }],
            },
        }]
        response = convert_roboflow_workflow_result(result, 200, 400, "adil-mubarak", "general-segmentation-api-12")
        self.assertEqual(len(response.panels), 1)
        self.assertEqual(len(response.panels[0].polygon), 4)
        self.assertAlmostEqual(response.panels[0].boundingBox.x, 0.1)
        self.assertAlmostEqual(response.panels[0].boundingBox.height, 0.4)

    def test_hybrid_refines_box_with_matching_mask(self):
        box = DetectedPanel(confidence=.95, polygon=[], boundingBox=BoundingBox(x=.1, y=.1, width=.4, height=.4))
        mask = DetectedPanel(confidence=.9, polygon=[Point(x=.1, y=.1), Point(x=.5, y=.12), Point(x=.48, y=.5)], boundingBox=BoundingBox(x=.1, y=.1, width=.4, height=.4))
        refined = refine_detections(
            DetectionResponse(width=100, height=100, modelVersion="boxes", panels=[box]),
            DetectionResponse(width=100, height=100, modelVersion="masks", panels=[mask]),
        )
        self.assertEqual(len(refined.panels[0].polygon), 3)
        self.assertEqual(refined.modelVersion, "boxes+masks")

    def test_hybrid_includes_unmatched_valid_mask(self):
        box = DetectedPanel(confidence=.95, polygon=[], boundingBox=BoundingBox(x=.1, y=.1, width=.2, height=.2))
        mask = DetectedPanel(confidence=.9, polygon=[Point(x=.6, y=.6), Point(x=.8, y=.6), Point(x=.8, y=.8)], boundingBox=BoundingBox(x=.6, y=.6, width=.2, height=.2))
        refined = refine_detections(
            DetectionResponse(width=100, height=100, modelVersion="boxes", panels=[box]),
            DetectionResponse(width=100, height=100, modelVersion="masks", panels=[mask]),
        )
        self.assertEqual(len(refined.panels), 2)
        self.assertEqual(refined.panels[1], mask)

    def test_hybrid_suppresses_duplicate_unmatched_masks(self):
        first = DetectedPanel(confidence=.9, polygon=[Point(x=.5, y=.5), Point(x=.8, y=.5), Point(x=.8, y=.8)], boundingBox=BoundingBox(x=.5, y=.5, width=.3, height=.3))
        duplicate = DetectedPanel(confidence=.8, polygon=[Point(x=.51, y=.51), Point(x=.81, y=.51), Point(x=.81, y=.81)], boundingBox=BoundingBox(x=.51, y=.51, width=.3, height=.3))
        refined = refine_detections(
            DetectionResponse(width=100, height=100, modelVersion="boxes", panels=[]),
            DetectionResponse(width=100, height=100, modelVersion="masks", panels=[first, duplicate]),
        )
        self.assertEqual(refined.panels, [first])

    def test_manga_uses_specialist_model_and_classes(self):
        runtime = ModelRuntime()
        runtime._roboflow_client = MagicMock()
        runtime._roboflow_client.infer.return_value = {
            "predictions": [{"class": "manga-panel", "confidence": .9, "x": 50, "y": 50, "width": 80, "height": 80}]
        }
        image = MagicMock()
        image.__enter__.return_value.size = (100, 100)
        with patch.dict(os.environ, {"ROBOFLOW_MANGA_MODEL_ID": "manga/custom", "ROBOFLOW_MANGA_PANEL_CLASSES": "manga-panel"}, clear=True), patch("PIL.Image.open", return_value=image):
            response = runtime.detect_roboflow(MagicMock(), "manga")
        runtime._roboflow_client.infer.assert_called_once_with(str(runtime._roboflow_client.infer.call_args.args[0]), model_id="manga/custom")
        self.assertEqual(response.modelVersion, "roboflow/manga/custom")
        self.assertEqual(len(response.panels), 1)

    def test_comic_model_uses_legacy_fallback(self):
        runtime = ModelRuntime()
        runtime._roboflow_client = MagicMock()
        runtime._roboflow_client.infer.return_value = {
            "predictions": [{"class": "panel", "confidence": .9, "x": 50, "y": 50, "width": 80, "height": 80}]
        }
        image = MagicMock()
        image.__enter__.return_value.size = (100, 100)
        with patch.dict(os.environ, {"ROBOFLOW_MODEL_ID": "legacy/3"}, clear=True), patch("PIL.Image.open", return_value=image):
            response = runtime.detect_roboflow(MagicMock(), "comic")
        self.assertEqual(runtime._roboflow_client.infer.call_args.kwargs["model_id"], "legacy/3")
        self.assertEqual(response.modelVersion, "roboflow/legacy/3")

    def test_hybrid_calls_content_specialist_and_workflow(self):
        runtime = ModelRuntime()
        boxes = DetectionResponse(width=100, height=100, modelVersion="boxes", panels=[])
        masks = DetectionResponse(width=100, height=100, modelVersion="masks", panels=[])
        with patch.object(runtime, "detect_roboflow", return_value=boxes) as detect_boxes, patch.object(runtime, "detect_roboflow_workflow", return_value=masks) as detect_masks, patch("app.main.refine_detections", return_value=boxes) as refine:
            runtime.detect_roboflow_hybrid(MagicMock(), "manga")
        detect_boxes.assert_called_once_with(unittest.mock.ANY, "manga")
        detect_masks.assert_called_once_with(unittest.mock.ANY, "manga")
        refine.assert_called_once_with(boxes, masks)

    def test_webtoon_falls_back_to_comic_model_and_classes(self):
        runtime = ModelRuntime()
        runtime._roboflow_client = MagicMock()
        runtime._roboflow_client.infer.return_value = {
            "predictions": [{"class": "comic-frame", "confidence": .9, "x": 50, "y": 50, "width": 80, "height": 80}]
        }
        image = MagicMock()
        image.__enter__.return_value.size = (100, 100)
        environment = {"ROBOFLOW_COMIC_MODEL_ID": "comic/9", "ROBOFLOW_COMIC_PANEL_CLASSES": "comic-frame"}
        with patch.dict(os.environ, environment, clear=True), patch("PIL.Image.open", return_value=image):
            response = runtime.detect_roboflow(MagicMock(), "webtoon")
        self.assertEqual(runtime._roboflow_client.infer.call_args.kwargs["model_id"], "comic/9")
        self.assertEqual(len(response.panels), 1)


if __name__ == "__main__":
    unittest.main()
