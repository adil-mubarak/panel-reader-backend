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
	"unicode/utf8"
)

const externalDetectionMaxResponse = 4 << 20

type externalDetectionRequest struct {
	ComicID          string `json:"comicId,omitempty"`
	Page             int    `json:"page,omitempty"`
	ImagePath        string `json:"imagePath"`
	ReadingDirection string `json:"readingDirection"`
	ContentType      string `json:"contentType"`
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

func (a *App) detectPanelsFile(ctx context.Context, path, comicID string, page int, direction, contentType string) ([]Panel, error) {
	if strings.TrimSpace(a.config.PanelDetectorURL) != "" {
		frames, err := a.detectPanelsExternal(ctx, path, comicID, page, direction, contentType)
		if err == nil {
			ai := postprocessDetectionsWithConfig(frames, direction, a.config, true)
			structural, structuralErr := detectPanelsFile(ctx, path)
			if structuralErr != nil {
				a.logger.Warn("structural panel detection failed after external success", "comic_id", comicID, "page", page, "error", structuralErr)
				markDetectionProvenance(ai, "roboflow")
				return ai, nil
			}
			structural = postprocessDetectionsWithConfig(structural, direction, a.config, false)
			return fuseDetections(ai, structural, direction), nil
		}
		a.logger.Warn("external panel detection failed; using Go detector", "comic_id", comicID, "page", page, "error", err)
	}
	frames, err := detectPanelsFile(ctx, path)
	frames = postprocessDetectionsWithConfig(frames, direction, a.config, false)
	frames = conservativeStructuralDetections(frames)
	markDetectionProvenance(frames, "structural")
	return frames, err
}

func markDetectionProvenance(frames []Panel, detector string) {
	for i := range frames {
		if detector == "roboflow" && strings.HasPrefix(frames[i].ModelVersion, "roboflow/") {
			continue
		}
		if detector == "roboflow" && frames[i].ModelVersion != "" {
			frames[i].ModelVersion = "roboflow/" + frames[i].ModelVersion
		} else {
			frames[i].ModelVersion = detector + "/v1"
		}
	}
}

func fuseDetections(ai, structural []Panel, direction string) []Panel {
	aiCount, structuralCount := len(ai), len(structural)
	markDetectionProvenance(ai, "roboflow")
	markDetectionProvenance(structural, "structural")
	if len(ai) == 0 {
		filtered := conservativeStructuralDetections(structural)
		markDetectionProvenance(filtered, "structural")
		return finalizeHybridDetections(filtered, direction, aiCount, structuralCount)
	}
	if len(ai) >= 2 && len(structural) == 1 && structural[0].FrameType == "full_page" {
		return finalizeHybridDetections(ai, direction, aiCount, structuralCount)
	}

	result := append([]Panel(nil), ai...)
	used := make([]bool, len(structural))
	for aiIndex := len(result) - 1; aiIndex >= 0; aiIndex-- {
		parts := make([]int, 0, 4)
		covered := 0.0
		for si, candidate := range structural {
			inter := intersectionArea(candidate, result[aiIndex])
			if structuralRecoveryCandidate(candidate) && inter/frameArea(candidate) >= .9 {
				parts = append(parts, si)
				covered += inter
			}
		}
		if len(parts) < 2 || covered/frameArea(result[aiIndex]) < .55 || !nonOverlappingStructural(parts, structural) {
			continue
		}
		replacement := make([]Panel, 0, len(result)-1+len(parts))
		replacement = append(replacement, result[:aiIndex]...)
		for _, si := range parts {
			used[si] = true
			structural[si].ModelVersion = "hybrid/structural-v1"
			replacement = append(replacement, structural[si])
		}
		replacement = append(replacement, result[aiIndex+1:]...)
		result = replacement
	}

	for si, candidate := range structural {
		if used[si] || !structuralRecoveryCandidate(candidate) || frameArea(candidate) > .8 {
			continue
		}
		add := true
		for _, existing := range result {
			inter := intersectionArea(candidate, existing)
			union := frameArea(candidate) + frameArea(existing) - inter
			if inter/frameArea(candidate) >= .35 || union > 0 && inter/union >= .2 {
				add = false
				break
			}
		}
		if add {
			candidate.ModelVersion = "hybrid/structural-v1"
			result = append(result, candidate)
		}
	}
	return finalizeHybridDetections(result, direction, aiCount, structuralCount)
}

func structuralRecoveryCandidate(candidate Panel) bool {
	return candidate.FrameType != "full_page" && candidate.Width >= .08 && candidate.Height >= .055 &&
		frameArea(candidate) >= .005 && candidate.Confidence >= .35
}

func conservativeStructuralDetections(frames []Panel) []Panel {
	if len(frames) == 1 && frames[0].FrameType == "full_page" {
		return frames
	}
	filtered := frames[:0]
	for _, candidate := range frames {
		if structuralRecoveryCandidate(candidate) {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		return fullPageDetection()
	}
	return filtered
}

func finalizeHybridDetections(frames []Panel, direction string, aiCount, structuralCount int) []Panel {
	recovered := 0
	for i := range frames {
		if strings.HasPrefix(frames[i].ModelVersion, "hybrid/structural-") {
			recovered++
		}
	}
	metadata := fmt.Sprintf(";aiCandidates=%d;structuralCandidates=%d;recoveredPanels=%d", aiCount, structuralCount, recovered)
	for i := range frames {
		if !strings.HasPrefix(frames[i].ModelVersion, "hybrid/") {
			frames[i].ModelVersion = "hybrid/" + frames[i].ModelVersion
		}
		maxVersionLength := 200 - len(metadata)
		if len(frames[i].ModelVersion) > maxVersionLength {
			frames[i].ModelVersion = frames[i].ModelVersion[:maxVersionLength]
			for !utf8.ValidString(frames[i].ModelVersion) {
				frames[i].ModelVersion = frames[i].ModelVersion[:len(frames[i].ModelVersion)-1]
			}
		}
		frames[i].ModelVersion += metadata
	}
	sortDetectedPanels(frames, direction)
	return frames
}

func nonOverlappingStructural(indices []int, frames []Panel) bool {
	for i, index := range indices {
		for _, other := range indices[:i] {
			inter := intersectionArea(frames[index], frames[other])
			if inter/math.Min(frameArea(frames[index]), frameArea(frames[other])) > .15 {
				return false
			}
		}
	}
	return true
}

func (a *App) detectPanelsExternal(ctx context.Context, path, comicID string, page int, direction, contentType string) ([]Panel, error) {
	if direction != "ltr" && direction != "rtl" && direction != "vertical" {
		return nil, errors.New("invalid reading direction")
	}
	if contentType != "comic" && contentType != "manga" && contentType != "webtoon" {
		return nil, errors.New("invalid content type")
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
	payload, err := json.Marshal(externalDetectionRequest{ComicID: comicID, Page: page, ImagePath: requestPath, ReadingDirection: direction, ContentType: contentType})
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
	if direction == "vertical" {
		sort.SliceStable(frames, func(i, j int) bool {
			if frames[i].Y != frames[j].Y {
				return frames[i].Y < frames[j].Y
			}
			return frames[i].X < frames[j].X
		})
	} else {
		type panelRow struct {
			top       float64
			minHeight float64
			frames    []Panel
		}
		sort.SliceStable(frames, func(i, j int) bool {
			if frames[i].Y != frames[j].Y {
				return frames[i].Y < frames[j].Y
			}
			return frames[i].X < frames[j].X
		})
		rows := make([]panelRow, 0, len(frames))
		for _, frame := range frames {
			rowIndex := -1
			bestDistance := math.MaxFloat64
			for i := range rows {
				tolerance := math.Max(.015, math.Min(rows[i].minHeight, frame.Height)*.4)
				distance := math.Abs(frame.Y - rows[i].top)
				if distance <= tolerance && distance < bestDistance {
					rowIndex, bestDistance = i, distance
				}
			}
			if rowIndex < 0 {
				rows = append(rows, panelRow{top: frame.Y, minHeight: frame.Height, frames: []Panel{frame}})
				continue
			}
			rows[rowIndex].frames = append(rows[rowIndex].frames, frame)
			rows[rowIndex].minHeight = math.Min(rows[rowIndex].minHeight, frame.Height)
		}
		ordered := make([]Panel, 0, len(frames))
		for _, row := range rows {
			sort.SliceStable(row.frames, func(i, j int) bool {
				if direction == "rtl" {
					return row.frames[i].X > row.frames[j].X
				}
				return row.frames[i].X < row.frames[j].X
			})
			ordered = append(ordered, row.frames...)
		}
		copy(frames, ordered)
	}
	for i := range frames {
		frames[i].Order = i + 1
		frames[i].Name = fmt.Sprintf("Detected panel %d", i+1)
	}
}
