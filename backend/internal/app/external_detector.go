package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

const externalDetectionMaxResponse = 4 << 20

type externalDetectionRequest struct {
	ComicID          string `json:"comicId,omitempty"`
	Page             int    `json:"page,omitempty"`
	ImagePath        string `json:"imagePath"`
	ReadingDirection string `json:"readingDirection"`
}

type externalBoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type externalPanel struct {
	Confidence  float64             `json:"confidence"`
	Polygon     []Point             `json:"polygon"`
	BoundingBox externalBoundingBox `json:"boundingBox"`
}

type externalDetectionResponse struct {
	Width        int             `json:"width"`
	Height       int             `json:"height"`
	ModelVersion string          `json:"modelVersion"`
	Panels       []externalPanel `json:"panels"`
}

func (a *App) detectPanelsFile(ctx context.Context, path, comicID string, page int, direction string) ([]Panel, error) {
	if strings.TrimSpace(a.config.PanelDetectorURL) != "" {
		frames, err := a.detectPanelsExternal(ctx, path, comicID, page, direction)
		if err == nil {
			return frames, nil
		}
		a.logger.Warn("external panel detection failed; using Go detector", "comic_id", comicID, "page", page, "error", err)
	}
	return detectPanelsFile(ctx, path)
}

func (a *App) detectPanelsExternal(ctx context.Context, path, comicID string, page int, direction string) ([]Panel, error) {
	if direction != "ltr" && direction != "rtl" {
		return nil, errors.New("invalid reading direction")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve image path: %w", err)
	}
	requestPath := absolutePath
	if strings.TrimSpace(a.config.PanelDetectorRoot) != "" {
		storageRoot, rootErr := filepath.Abs(a.config.StorageRoot)
		relative, relErr := filepath.Rel(storageRoot, absolutePath)
		if rootErr != nil || relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, errors.New("image path is outside configured storage root")
		}
		requestPath = filepath.Join(a.config.PanelDetectorRoot, relative)
	}
	payload, err := json.Marshal(externalDetectionRequest{ComicID: comicID, Page: page, ImagePath: requestPath, ReadingDirection: direction})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(a.config.PanelDetectorURL, "/") + "/internal/v1/panel-detection"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf("panel detector returned HTTP %d", response.StatusCode)
	}
	var result externalDetectionResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, externalDetectionMaxResponse+1))
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode panel detector response: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("panel detector response contains trailing data or exceeds the size limit")
	}
	return validateExternalDetection(result, direction)
}

func validateExternalDetection(result externalDetectionResponse, direction string) ([]Panel, error) {
	if result.Width <= 0 || result.Height <= 0 || len(result.ModelVersion) > 200 || len(result.Panels) > 500 {
		return nil, errors.New("invalid panel detector response metadata")
	}
	frames := make([]Panel, len(result.Panels))
	for i, detected := range result.Panels {
		box := detected.BoundingBox
		if !finite(box.X, box.Y, box.Width, box.Height, detected.Confidence) || box.X < 0 || box.Y < 0 || box.Width <= 0 || box.Height <= 0 || box.X+box.Width > 1 || box.Y+box.Height > 1 || detected.Confidence < 0 || detected.Confidence > 1 {
			return nil, fmt.Errorf("invalid panel %d bounding box or confidence", i)
		}
		if len(detected.Polygon) > 100 || len(detected.Polygon) == 1 || len(detected.Polygon) == 2 {
			return nil, fmt.Errorf("invalid panel %d polygon", i)
		}
		for _, point := range detected.Polygon {
			if !finite(point.X, point.Y) || point.X < 0 || point.X > 1 || point.Y < 0 || point.Y > 1 {
				return nil, fmt.Errorf("invalid panel %d polygon coordinate", i)
			}
		}
		frame := defaultFrame(0)
		frame.X, frame.Y, frame.Width, frame.Height = box.X, box.Y, box.Width, box.Height
		frame.Confidence, frame.ModelVersion = detected.Confidence, result.ModelVersion
		if len(detected.Polygon) >= 3 {
			frame.ShapeType, frame.Polygon = "polygon", detected.Polygon
		}
		frames[i] = frame
	}
	sortDetectedPanels(frames, direction)
	return frames, nil
}

func sortDetectedPanels(frames []Panel, direction string) {
	sort.SliceStable(frames, func(i, j int) bool {
		left, right := frames[i], frames[j]
		leftCenter, rightCenter := left.Y+left.Height/2, right.Y+right.Height/2
		tolerance := math.Min(left.Height, right.Height) / 3
		if math.Abs(leftCenter-rightCenter) > tolerance {
			return leftCenter < rightCenter
		}
		if direction == "rtl" {
			return left.X > right.X
		}
		return left.X < right.X
	})
	for i := range frames {
		frames[i].Order = i + 1
		frames[i].Name = fmt.Sprintf("Detected panel %d", i+1)
	}
}
