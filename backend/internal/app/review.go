package app

import (
	"math"
)

const postprocessMaxFrames = 64

type DetectionReport struct {
	Warnings          []string `json:"warnings"`
	PanelCount        int      `json:"panelCount"`
	Coverage          float64  `json:"coverage"`
	AverageConfidence float64  `json:"averageConfidence,omitempty"`
	Classification    string   `json:"classification,omitempty"`
}

func postprocessDetections(frames []Panel, direction string, confidenceThreshold float64, ai bool) []Panel {
	config := Config{DetectionConfidenceThreshold: confidenceThreshold, DetectionReliableConfidence: .55, DetectionMinCoverage: .35, DetectionMaxOverlap: .35, DetectionPolygonRectangularity: .9}
	return postprocessDetectionsWithConfig(frames, direction, config, ai)
}

func postprocessDetectionsWithConfig(frames []Panel, direction string, config Config, ai bool) []Panel {
	candidates := make([]Panel, 0, len(frames))
	for _, f := range frames {
		normalizeDetectedShape(&f, config.DetectionPolygonRectangularity)
		if ai && f.Confidence < config.DetectionConfidenceThreshold || frameArea(f) < .002 {
			continue
		}
		physical := f.FrameType == "panel" || f.FrameType == "full_page"
		duplicate := false
		for i := range candidates {
			candidatePhysical := candidates[i].FrameType == "panel" || candidates[i].FrameType == "full_page"
			if !physical || !candidatePhysical {
				continue
			}
			inter := intersectionArea(f, candidates[i])
			union := frameArea(f) + frameArea(candidates[i]) - inter
			contained := inter / math.Min(frameArea(f), frameArea(candidates[i]))
			if union > 0 && inter/union >= .75 || contained >= .92 {
				duplicate = true
				if f.Confidence > candidates[i].Confidence {
					candidates[i] = f
				}
				break
			}
		}
		if !duplicate {
			candidates = append(candidates, f)
		}
	}
	sortDetectedPanels(candidates, direction)
	if len(candidates) > postprocessMaxFrames {
		candidates = candidates[:postprocessMaxFrames]
		sortDetectedPanels(candidates, direction)
	}
	if ai && uncertainDetections(candidates, config) {
		return fullPageDetection()
	}
	return candidates
}

func uncertainDetections(frames []Panel, config Config) bool {
	physical := make([]Panel, 0, len(frames))
	for _, f := range frames {
		if f.FrameType == "panel" || f.FrameType == "full_page" {
			physical = append(physical, f)
		}
	}
	if len(physical) == 0 {
		return true
	}
	if len(physical) == 1 {
		return len(frames) == 1
	}
	reliable, confidence, overlap := 0, 0.0, 0.0
	for i, f := range physical {
		confidence += f.Confidence
		if f.Confidence >= config.DetectionReliableConfidence {
			reliable++
		}
		for j := 0; j < i; j++ {
			overlap += intersectionArea(f, physical[j])
		}
	}
	report := buildDetectionReport(physical)
	average := confidence / float64(len(physical))
	return reliable < 2 && (average < config.DetectionReliableConfidence || report.Coverage < config.DetectionMinCoverage || overlap > config.DetectionMaxOverlap)
}

func normalizeDetectedShape(f *Panel, rectangularity float64) {
	if f.ShapeType != "polygon" || len(f.Polygon) < 3 {
		return
	}
	points := simplifyPolygon(f.Polygon)
	valid := len(points) >= 3 && len(points) <= 32 && !polygonSelfIntersects(points)
	area := polygonArea(points)
	boxArea := f.Width * f.Height
	if !valid || boxArea <= 0 || area/boxArea >= rectangularity {
		f.ShapeType, f.Polygon = "rectangle", []Point{}
		return
	}
	f.Polygon = points
}

func simplifyPolygon(points []Point) []Point {
	result := make([]Point, 0, len(points))
	for _, p := range points {
		if len(result) == 0 || math.Hypot(p.X-result[len(result)-1].X, p.Y-result[len(result)-1].Y) > .002 {
			result = append(result, p)
		}
	}
	if len(result) > 1 && math.Hypot(result[0].X-result[len(result)-1].X, result[0].Y-result[len(result)-1].Y) <= .002 {
		result = result[:len(result)-1]
	}
	return result
}

func polygonArea(points []Point) float64 {
	area := 0.0
	for i, p := range points {
		q := points[(i+1)%len(points)]
		area += p.X*q.Y - q.X*p.Y
	}
	return math.Abs(area) / 2
}

func polygonSelfIntersects(points []Point) bool {
	for i := range points {
		a, b := points[i], points[(i+1)%len(points)]
		for j := i + 1; j < len(points); j++ {
			if j == i || j == (i+1)%len(points) || (j+1)%len(points) == i {
				continue
			}
			c, d := points[j], points[(j+1)%len(points)]
			if segmentsCross(a, b, c, d) {
				return true
			}
		}
	}
	return false
}

func segmentsCross(a, b, c, d Point) bool {
	cross := func(p, q, r Point) float64 { return (q.X-p.X)*(r.Y-p.Y) - (q.Y-p.Y)*(r.X-p.X) }
	return cross(a, b, c)*cross(a, b, d) < 0 && cross(c, d, a)*cross(c, d, b) < 0
}

func frameArea(f Panel) float64 {
	if f.ShapeType != "polygon" || len(f.Polygon) < 3 {
		return f.Width * f.Height
	}
	area := 0.0
	for i, p := range f.Polygon {
		q := f.Polygon[(i+1)%len(f.Polygon)]
		area += p.X*q.Y - q.X*p.Y
	}
	return math.Abs(area) / 2
}

func intersectionArea(a, b Panel) float64 {
	x0, y0 := math.Max(a.X, b.X), math.Max(a.Y, b.Y)
	x1, y1 := math.Min(a.X+a.Width, b.X+b.Width), math.Min(a.Y+a.Height, b.Y+b.Height)
	return math.Max(0, x1-x0) * math.Max(0, y1-y0)
}

func buildDetectionReport(frames []Panel) DetectionReport {
	report := DetectionReport{Warnings: []string{}, PanelCount: len(frames)}
	if len(frames) == 0 {
		report.Warnings = append(report.Warnings, "no_panels")
		report.Classification = "uncertain"
		return report
	}
	confidenceCount, overlapPairs := 0, 0
	for i, f := range frames {
		if f.Confidence > 0 {
			report.AverageConfidence += f.Confidence
			confidenceCount++
		}
		for j := 0; j < i; j++ {
			if intersectionArea(f, frames[j])/math.Min(frameArea(f), frameArea(frames[j])) > .35 {
				overlapPairs++
			}
		}
	}
	if confidenceCount > 0 {
		report.AverageConfidence /= float64(confidenceCount)
		if report.AverageConfidence < .5 {
			report.Warnings = append(report.Warnings, "low_confidence")
		}
	}
	if overlapPairs > max(1, len(frames)/3) {
		report.Warnings = append(report.Warnings, "heavy_overlap")
	}
	const grid = 64
	covered := 0
	for y := 0; y < grid; y++ {
		for x := 0; x < grid; x++ {
			px, py := (float64(x)+.5)/grid, (float64(y)+.5)/grid
			for _, f := range frames {
				if pointInFrame(px, py, f) {
					covered++
					break
				}
			}
		}
	}
	report.Coverage = float64(covered) / (grid * grid)
	report.Classification = "multi_panel"
	if len(frames) == 1 && frames[0].FrameType == "full_page" {
		report.Classification = "full_page"
	}
	if report.Coverage < .55 {
		report.Warnings = append(report.Warnings, "large_uncovered_area")
	}
	if len(frames) > 30 {
		report.Warnings = append(report.Warnings, "too_many_panels")
	}
	for i := 1; i < len(frames); i++ {
		if math.Abs((frames[i].Y+frames[i].Height/2)-(frames[i-1].Y+frames[i-1].Height/2)) < .03 && math.Abs(frames[i].X-frames[i-1].X) < .03 {
			report.Warnings = append(report.Warnings, "ambiguous_order")
			break
		}
	}
	return report
}

func pointInFrame(x, y float64, f Panel) bool {
	if f.ShapeType != "polygon" || len(f.Polygon) < 3 {
		return x >= f.X && x <= f.X+f.Width && y >= f.Y && y <= f.Y+f.Height
	}
	inside := false
	for i, j := 0, len(f.Polygon)-1; i < len(f.Polygon); j, i = i, i+1 {
		pi, pj := f.Polygon[i], f.Polygon[j]
		if (pi.Y > y) != (pj.Y > y) && x < (pj.X-pi.X)*(y-pi.Y)/(pj.Y-pi.Y)+pi.X {
			inside = !inside
		}
	}
	return inside
}

func reviewStatusForReport(report DetectionReport) string {
	if len(report.Warnings) > 0 {
		return "review_recommended"
	}
	return "unreviewed"
}
