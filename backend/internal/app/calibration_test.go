package app

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const calibrationMatchIoU = 0.5

type calibrationFixture struct {
	Name      string           `json:"name"`
	Synthetic syntheticFixture `json:"synthetic"`
	Expected  []normalizedBox  `json:"expected"`
}

type syntheticFixture struct {
	Width             int          `json:"width"`
	Height            int          `json:"height"`
	VerticalGutters   []gutterBand `json:"verticalGutters"`
	HorizontalGutters []gutterBand `json:"horizontalGutters"`
}

type gutterBand struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type normalizedBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type calibrationMetrics struct {
	TruePositive int
	Predicted    int
	Expected     int
	IoUSum       float64
}

func TestDetectorCalibration(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "detector-calibration.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []calibrationFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}

	var total calibrationMetrics
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), fixture.Name+".png")
			writeSyntheticFixture(t, path, fixture.Synthetic)
			panels, err := detectPanelsFile(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			predicted := make([]normalizedBox, len(panels))
			for i, panel := range panels {
				predicted[i] = normalizedBox{panel.X, panel.Y, panel.Width, panel.Height}
			}
			metrics := scoreCalibration(predicted, fixture.Expected, calibrationMatchIoU)
			t.Logf("precision=%.3f recall=%.3f mean_iou=%.3f matches=%d predicted=%d expected=%d",
				metrics.precision(), metrics.recall(), metrics.meanIoU(), metrics.TruePositive, metrics.Predicted, metrics.Expected)
			total.TruePositive += metrics.TruePositive
			total.Predicted += metrics.Predicted
			total.Expected += metrics.Expected
			total.IoUSum += metrics.IoUSum
			if metrics.TruePositive != len(fixture.Expected) || metrics.Predicted != len(fixture.Expected) {
				t.Errorf("detector did not match all expected boxes: got %v, want %v", predicted, fixture.Expected)
			}
		})
	}
	t.Logf("aggregate precision=%.3f recall=%.3f mean_iou=%.3f matches=%d predicted=%d expected=%d",
		total.precision(), total.recall(), total.meanIoU(), total.TruePositive, total.Predicted, total.Expected)
}

func writeSyntheticFixture(t *testing.T, path string, fixture syntheticFixture) {
	t.Helper()
	if fixture.Width < 1 || fixture.Height < 1 {
		t.Fatalf("invalid synthetic fixture size %dx%d", fixture.Width, fixture.Height)
	}
	img := image.NewGray(image.Rect(0, 0, fixture.Width, fixture.Height))
	for y := 0; y < fixture.Height; y++ {
		for x := 0; x < fixture.Width; x++ {
			// High-frequency content prevents illustrated regions from being mistaken for gutters.
			img.SetGray(x, y, color.Gray{Y: uint8(35 + (x*17+y*31)%180)})
		}
	}
	for _, gutter := range fixture.VerticalGutters {
		for x := gutter.Start; x < gutter.End; x++ {
			for y := 0; y < fixture.Height; y++ {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	for _, gutter := range fixture.HorizontalGutters {
		for y := gutter.Start; y < gutter.End; y++ {
			for x := 0; x < fixture.Width; x++ {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func scoreCalibration(predicted, expected []normalizedBox, threshold float64) calibrationMetrics {
	metrics := calibrationMetrics{Predicted: len(predicted), Expected: len(expected)}
	usedPredicted, usedExpected := make([]bool, len(predicted)), make([]bool, len(expected))
	for {
		best, bestPredicted, bestExpected := threshold, -1, -1
		for pi, prediction := range predicted {
			if usedPredicted[pi] {
				continue
			}
			for ei, expectation := range expected {
				if !usedExpected[ei] {
					if overlap := boxIoU(prediction, expectation); overlap >= best {
						best, bestPredicted, bestExpected = overlap, pi, ei
					}
				}
			}
		}
		if bestPredicted < 0 {
			return metrics
		}
		usedPredicted[bestPredicted], usedExpected[bestExpected] = true, true
		metrics.TruePositive++
		metrics.IoUSum += best
	}
}

func boxIoU(a, b normalizedBox) float64 {
	w := max(0.0, min(a.X+a.Width, b.X+b.Width)-max(a.X, b.X))
	h := max(0.0, min(a.Y+a.Height, b.Y+b.Height)-max(a.Y, b.Y))
	intersection := w * h
	union := a.Width*a.Height + b.Width*b.Height - intersection
	if union <= 0 {
		return 0
	}
	return intersection / union
}

func (m calibrationMetrics) precision() float64 { return ratio(m.TruePositive, m.Predicted) }
func (m calibrationMetrics) recall() float64    { return ratio(m.TruePositive, m.Expected) }
func (m calibrationMetrics) meanIoU() float64   { return ratioFloat(m.IoUSum, m.TruePositive) }

func ratio(numerator, denominator int) float64 {
	return ratioFloat(float64(numerator), denominator)
}

func ratioFloat(numerator float64, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / float64(denominator)
}

func (b normalizedBox) String() string {
	return fmt.Sprintf("{x:%.3f y:%.3f width:%.3f height:%.3f}", b.X, b.Y, b.Width, b.Height)
}
