package app

import (
	"context"
	"errors"
	"image"
	"io"
	"math"
	"os"
	"sort"
)

const (
	detectionMaxDimension = 900
	detectionMaxFrames    = 64
	detectionMaxDepth     = 8
)

type detectionRect struct{ x0, y0, x1, y1 int }

type analysisImage struct {
	w, h int
	pix  []float64
	bg   float64
}

func detectPanelsFile(ctx context.Context, path string) ([]Panel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return detectPanels(ctx, f)
}

// detectPanels intentionally uses only the standard image decoders registered by app.go.
func detectPanels(ctx context.Context, r io.Reader) ([]Panel, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a := downsampleImage(img)
	if a.w < 2 || a.h < 2 {
		return fullPageDetection(), nil
	}
	regions := make([]detectionRect, 0, 8)
	var split func(detectionRect, int) error
	split = func(rect detectionRect, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth >= detectionMaxDepth || len(regions) >= detectionMaxFrames {
			regions = append(regions, rect)
			return nil
		}
		axis, lo, hi, ok := a.bestGutter(rect)
		if !ok {
			regions = append(regions, rect)
			return nil
		}
		first, second := rect, rect
		if axis == 0 {
			first.y1, second.y0 = lo, hi
		} else {
			first.x1, second.x0 = lo, hi
		}
		if !a.reliableRegion(first) || !a.reliableRegion(second) {
			regions = append(regions, rect)
			return nil
		}
		if err := split(first, depth+1); err != nil {
			return err
		}
		return split(second, depth+1)
	}
	if err := split(detectionRect{0, 0, a.w, a.h}, 0); err != nil {
		return nil, err
	}
	if len(regions) < 2 || len(regions) > detectionMaxFrames {
		return fullPageDetection(), nil
	}
	sort.SliceStable(regions, func(i, j int) bool {
		cyi, cyj := regions[i].y0+regions[i].y1, regions[j].y0+regions[j].y1
		tolerance := max(2, min(regions[i].y1-regions[i].y0, regions[j].y1-regions[j].y0)/3)
		if abs(cyi-cyj) > tolerance*2 {
			return cyi < cyj
		}
		return regions[i].x0 < regions[j].x0
	})
	frames := make([]Panel, 0, len(regions))
	for i, rect := range regions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		frame := defaultFrame(i + 1)
		frame.Name = "Detected panel " + itoa(i+1)
		frame.X, frame.Y = float64(rect.x0)/float64(a.w), float64(rect.y0)/float64(a.h)
		frame.Width, frame.Height = float64(rect.x1-rect.x0)/float64(a.w), float64(rect.y1-rect.y0)/float64(a.h)
		frame.Confidence = a.distributedContentScore(rect)
		frames = append(frames, frame)
	}
	return frames, nil
}

// distributedContentScore rewards activity spread through a region rather than
// a small cluster of text-like marks on an otherwise uniform background.
func (a analysisImage) distributedContentScore(r detectionRect) float64 {
	const grid = 4
	w, h := r.x1-r.x0, r.y1-r.y0
	insetX, insetY := w/20, h/20
	r.x0, r.x1 = r.x0+insetX, r.x1-insetX
	r.y0, r.y1 = r.y0+insetY, r.y1-insetY
	if r.x1-r.x0 < grid || r.y1-r.y0 < grid {
		return 0
	}
	active := 0
	rows, cols := [grid]bool{}, [grid]bool{}
	for gy := 0; gy < grid; gy++ {
		for gx := 0; gx < grid; gx++ {
			x0 := r.x0 + (r.x1-r.x0)*gx/grid
			x1 := r.x0 + (r.x1-r.x0)*(gx+1)/grid
			y0 := r.y0 + (r.y1-r.y0)*gy/grid
			y1 := r.y0 + (r.y1-r.y0)*(gy+1)/grid
			var sum, sum2, activity float64
			count, edges := 0, 0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					v := a.pix[y*a.w+x]
					sum, sum2, count = sum+v, sum2+v*v, count+1
					if x > x0 {
						activity += math.Abs(v - a.pix[y*a.w+x-1])
						edges++
					}
					if y > y0 {
						activity += math.Abs(v - a.pix[(y-1)*a.w+x])
						edges++
					}
				}
			}
			mean := sum / float64(count)
			variance := math.Max(0, sum2/float64(count)-mean*mean)
			if variance >= .0015 || activity/float64(max(1, edges)) >= .03 {
				active++
				rows[gy], cols[gx] = true, true
			}
		}
	}
	activeRows, activeCols := 0, 0
	for i := 0; i < grid; i++ {
		if rows[i] {
			activeRows++
		}
		if cols[i] {
			activeCols++
		}
	}
	coverage := math.Min(float64(activeRows)/grid, float64(activeCols)/grid)
	return .7*float64(active)/(grid*grid) + .3*coverage
}

func downsampleImage(src image.Image) analysisImage {
	b := src.Bounds()
	scale := math.Max(float64(b.Dx())/detectionMaxDimension, float64(b.Dy())/detectionMaxDimension)
	if scale < 1 {
		scale = 1
	}
	w, h := max(1, int(math.Ceil(float64(b.Dx())/scale))), max(1, int(math.Ceil(float64(b.Dy())/scale)))
	a := analysisImage{w: w, h: h, pix: make([]float64, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx := b.Min.X + min(b.Dx()-1, int((float64(x)+.5)*scale))
			sy := b.Min.Y + min(b.Dy()-1, int((float64(y)+.5)*scale))
			r, g, bl, _ := src.At(sx, sy).RGBA()
			a.pix[y*w+x] = (.2126*float64(r) + .7152*float64(g) + .0722*float64(bl)) / 65535
		}
	}
	corners := []float64{a.pix[0], a.pix[w-1], a.pix[(h-1)*w], a.pix[h*w-1]}
	sort.Float64s(corners)
	a.bg = (corners[1] + corners[2]) / 2
	return a
}

// bestGutter returns the strongest full-span background-like band, horizontal first on ties.
func (a analysisImage) bestGutter(r detectionRect) (axis, lo, hi int, ok bool) {
	bestScore := -1.0
	for candidateAxis := 0; candidateAxis < 2; candidateAxis++ {
		length, span := r.y1-r.y0, r.x1-r.x0
		start := r.y0
		if candidateAxis == 1 {
			length, span, start = r.x1-r.x0, r.y1-r.y0, r.x0
		}
		margin := max(2, length/25)
		minimumBand := max(2, length/100)
		for p := start + margin; p < start+length-margin; {
			if !a.gutterLine(r, candidateAxis, p, span) {
				p++
				continue
			}
			end := p + 1
			for end < start+length-margin && a.gutterLine(r, candidateAxis, end, span) {
				end++
			}
			// Bands connected to a region edge are margins, not separators.
			if p > start+margin && end < start+length-margin && end-p >= minimumBand {
				score := float64(end-p) / float64(length)
				if score > bestScore {
					axis, lo, hi, ok, bestScore = candidateAxis, p, end, true, score
				}
			}
			p = end
		}
	}
	return
}

func (a analysisImage) gutterLine(r detectionRect, axis, p, span int) bool {
	var sum, sum2, activity float64
	var previous float64
	for i := 0; i < span; i++ {
		x, y := r.x0+i, p
		if axis == 1 {
			x, y = p, r.y0+i
		}
		v := a.pix[y*a.w+x]
		sum, sum2 = sum+v, sum2+v*v
		if i > 0 {
			activity += math.Abs(v - previous)
		}
		previous = v
	}
	mean := sum / float64(span)
	variance := math.Max(0, sum2/float64(span)-mean*mean)
	// Gutters and panel borders may be white, black, or colored. Their useful
	// property is consistency across the region, not similarity to page corners.
	return variance <= .004 && activity/float64(max(1, span-1)) <= .035
}

func (a analysisImage) reliableRegion(r detectionRect) bool {
	w, h := r.x1-r.x0, r.y1-r.y0
	return w >= max(4, a.w/40) && h >= max(4, a.h/40) && w*h >= a.w*a.h/500
}

func fullPageDetection() []Panel {
	f := defaultFrame(1)
	f.Name, f.FrameType, f.X, f.Y, f.Width, f.Height = "Full page", "full_page", 0, 0, 1, 1
	return []Panel{f}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func itoa(v int) string {
	if v < 1 || v > detectionMaxFrames {
		panic(errors.New("detector frame number outside bounds"))
	}
	if v < 10 {
		return string(rune('0' + v))
	}
	return string(rune('0'+v/10)) + string(rune('0'+v%10))
}
