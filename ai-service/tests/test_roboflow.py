import os
import unittest
from unittest.mock import patch

from fastapi import HTTPException

from app.main import (
    BoundingBox,
    DetectedPanel,
    DetectionRequest,
    DetectionResponse,
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


if __name__ == "__main__":
    unittest.main()
