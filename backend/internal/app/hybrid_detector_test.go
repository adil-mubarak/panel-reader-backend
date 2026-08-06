package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestFuseDetectionsRecoversOmittedPanels(t *testing.T) {
	structural := sevenPanelFrames()
	ai := []Panel{structural[0], structural[1], structural[3], structural[5]}
	for i := range ai {
		ai[i].Confidence, ai[i].ModelVersion = .9, "seg-v2"
	}
	got := fuseDetections(ai, structural, "ltr")
	if len(got) != 7 {
		t.Fatalf("got %d panels, want 7: %#v", len(got), got)
	}
	report := buildDetectionReport(got)
	if report.AICandidateCount != 4 || report.StructuralCandidateCount != 7 || report.RecoveredPanelCount != 3 || !containsWarning(report.Warnings, "detector_disagreement") {
		t.Fatalf("report = %#v", report)
	}
	for i := range got {
		if got[i].Order != i+1 {
			t.Fatalf("orders = %#v", got)
		}
	}
}

func TestFuseDetectionsDoesNotInflateDuplicates(t *testing.T) {
	structural := sevenPanelFrames()
	ai := append([]Panel(nil), structural...)
	for i := range ai {
		ai[i].Confidence, ai[i].ModelVersion = .9, "seg-v2"
	}
	if got := fuseDetections(ai, structural, "ltr"); len(got) != 7 {
		t.Fatalf("got %d duplicate-inflated panels", len(got))
	}
}

func TestFuseDetectionsSplitsOversizedOnlyForPartition(t *testing.T) {
	oversized := panelBox(0, 0, 1, .3)
	oversized.Confidence, oversized.ModelVersion = .9, "seg-v2"
	parts := []Panel{panelBox(0, 0, .32, .3), panelBox(.34, 0, .32, .3), panelBox(.68, 0, .32, .3)}
	if got := fuseDetections([]Panel{oversized}, parts, "ltr"); len(got) != 3 {
		t.Fatalf("reliable partition produced %d panels: %#v", len(got), got)
	}
	if got := fuseDetections([]Panel{oversized}, parts[:1], "ltr"); len(got) != 1 || got[0].Width != 1 {
		t.Fatalf("single nested candidate replaced oversized AI: %#v", got)
	}
}

func TestFuseDetectionsIgnoresStructuralFullPage(t *testing.T) {
	ai := []Panel{panelBox(0, 0, .48, 1), panelBox(.52, 0, .48, 1)}
	for i := range ai {
		ai[i].Confidence, ai[i].ModelVersion = .9, "seg-v2"
	}
	if got := fuseDetections(ai, fullPageDetection(), "ltr"); len(got) != 2 {
		t.Fatalf("structural full-page changed AI result: %#v", got)
	}
}

func TestFuseDetectionsUsesDistributedVisualContent(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 800, 1000))
	for i := range img.Pix {
		img.Pix[i] = 220
	}
	real := []Panel{
		panelBox(.04, .04, .27, .22), panelBox(.365, .04, .27, .22), panelBox(.69, .04, .27, .22),
		panelBox(.08, .32, .13, .07), panelBox(.05, .64, .9, .3),
	}
	falseCandidates := []Panel{
		panelBox(.38, .33, .24, .073), panelBox(.7, .34, .07, .07), panelBox(.8, .3, .05, .12),
		panelBox(.08, .48, .68, .038), panelBox(.3, .55, .39, .028),
	}
	for _, panel := range real {
		paintPanelTexture(img, panel)
	}
	// Dense caption-like marks occupy only one part of an otherwise flat box.
	caption := falseCandidates[0]
	x0, y0 := int(caption.X*800), int(caption.Y*1000)
	for y := y0 + 8; y < y0+30; y += 4 {
		for x := x0 + 8; x < x0+70; x++ {
			if x%3 != 0 {
				img.SetGray(x, y, color.Gray{Y: 25})
			}
		}
	}

	a := downsampleImage(img)
	structural := append(append([]Panel(nil), real...), falseCandidates...)
	for i := range structural {
		structural[i].Confidence = contentScoreForPanel(a, structural[i])
	}
	ai := append([]Panel(nil), real[:2]...)
	for i := range ai {
		ai[i].Confidence, ai[i].ModelVersion = .9, "seg-v2"
	}
	got := fuseDetections(ai, structural, "ltr")
	if len(got) != len(real) {
		t.Fatalf("got %d panels, want only %d illustrated panels: %#v", len(got), len(real), got)
	}
	for _, expected := range real {
		found := false
		for _, panel := range got {
			if intersectionArea(expected, panel)/frameArea(expected) > .95 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("illustrated panel was not recovered: %#v; got %#v", expected, got)
		}
	}
}

func TestStructuralDetectorSyntheticSevenPanels(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 600, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 600; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(35 + (x*17+y*31)%180)})
		}
	}
	paintBlack(img, 0, 280, 600, 300)
	paintBlack(img, 0, 590, 600, 610)
	paintBlack(img, 194, 300, 210, 590)
	paintBlack(img, 397, 300, 413, 590)
	paintBlack(img, 187, 610, 203, 900)
	paintBlack(img, 390, 610, 406, 900)
	panels := detectPNG(t, img)
	if len(panels) != 7 {
		t.Fatalf("got %d panels, want 7: %#v", len(panels), panels)
	}
}

func TestStructuralDetectorSplashStaysFullPage(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 400, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 400; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8((x*19 + y*37) % 256)})
		}
	}
	panels := detectPNG(t, img)
	if len(panels) != 1 || panels[0].FrameType != "full_page" {
		t.Fatalf("splash = %#v", panels)
	}
}

func sevenPanelFrames() []Panel {
	frames := []Panel{
		panelBox(0, 0, 1, .3),
		panelBox(0, .32, .32, .32), panelBox(.34, .32, .32, .32), panelBox(.68, .32, .32, .32),
		panelBox(0, .66, .32, .34), panelBox(.34, .66, .32, .34), panelBox(.68, .66, .32, .34),
	}
	for i := range frames {
		frames[i].Confidence = .8
	}
	return frames
}

func panelBox(x, y, width, height float64) Panel {
	f := defaultFrame(0)
	f.X, f.Y, f.Width, f.Height = x, y, width, height
	f.Confidence = .8
	return f
}

func paintBlack(img *image.Gray, x0, y0, x1, y1 int) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetGray(x, y, color.Gray{})
		}
	}
}

func paintPanelTexture(img *image.Gray, panel Panel) {
	x0, y0 := int(panel.X*float64(img.Rect.Dx())), int(panel.Y*float64(img.Rect.Dy()))
	x1 := int((panel.X + panel.Width) * float64(img.Rect.Dx()))
	y1 := int((panel.Y + panel.Height) * float64(img.Rect.Dy()))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(25 + (x*19+y*31)%190)})
		}
	}
}

func contentScoreForPanel(a analysisImage, panel Panel) float64 {
	return a.distributedContentScore(detectionRect{
		x0: int(panel.X * float64(a.w)), y0: int(panel.Y * float64(a.h)),
		x1: int((panel.X + panel.Width) * float64(a.w)), y1: int((panel.Y + panel.Height) * float64(a.h)),
	})
}

func detectPNG(t *testing.T, img image.Image) []Panel {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	panels, err := detectPanels(t.Context(), &encoded)
	if err != nil {
		t.Fatal(err)
	}
	return panels
}
