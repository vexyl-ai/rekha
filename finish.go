package rekha

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/colour"
	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// canvasWhite is the OKLab distance within which a depth-0 shape counts as
// the white page and is left out: SVG's transparent background renders
// white, and a logo traced from a transparent PNG stays transparent. 0.02
// is about one just-noticeable difference, the drift an anti-aliased
// background gives its centroid.
const canvasWhite = 0.02

// layer is one shape of one colour queued for output.
type layer struct {
	colour int
	shape  contour.Shape
	depth  int
}

// Finish traces every colour of s, orders the shapes (stacked: nesting
// depth, outer contours only, consecutive paths of one colour sharing a
// <path>; cutout: colour then scan order, holes cut out, one path per
// shape), fits each contour under o.Mode and returns the paths with their
// palette fills. White shapes at depth 0 are left out.
func (s *Segmentation) Finish(o Options) (*Result, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	res := &Result{Width: s.Width, Height: s.Height}
	if s.Width*s.Height == 0 || len(s.Palette) == 0 {
		return res, nil
	}
	if len(s.Labels) != s.Width*s.Height {
		return nil, fmt.Errorf("rekha: %d labels for a %d×%d segmentation", len(s.Labels), s.Width, s.Height)
	}

	layers := s.layers(o)
	stacked := o.Hierarchical == HierarchicalStacked && o.Preset != PresetBW
	shapes := make([]contour.Shape, len(layers))
	for i, l := range layers {
		shapes[i] = l.shape
	}
	parents := contour.Nest(shapes)
	for i := range layers {
		layers[i].depth = depthOf(parents, i)
	}
	if stacked {
		sort.SliceStable(layers, func(a, b int) bool {
			if layers[a].depth != layers[b].depth {
				return layers[a].depth < layers[b].depth
			}
			return pointLess(layers[a].shape.Outer.Pts[0], layers[b].shape.Outer.Pts[0])
		})
	}

	// Drop the white page: depth-0 shapes sit straight on the background,
	// which renders white, so leaving the white ones out changes nothing
	// and keeps a transparent input transparent.
	kept := layers[:0]
	for _, l := range layers {
		if l.depth == 0 && s.nearWhite(l.colour) {
			continue
		}
		kept = append(kept, l)
	}
	layers = kept

	// Fit every contour in parallel, indexed by layer and subpath.
	subpaths := make([][]svg.Subpath, len(layers))
	var jobs [][2]int
	for i, l := range layers {
		n := 1
		if !stacked {
			n += len(l.shape.Holes)
		}
		subpaths[i] = make([]svg.Subpath, n)
		for k := 0; k < n; k++ {
			jobs = append(jobs, [2]int{i, k})
		}
	}
	parallel(len(jobs), func(j int) {
		i, k := jobs[j][0], jobs[j][1]
		c := layers[i].shape.Outer
		if k > 0 {
			c = layers[i].shape.Holes[k-1]
		}
		subpaths[i][k] = svg.Subpath{Segs: segments(c.Pts, o)}
	})

	for i, l := range layers {
		fill := colour.FromSRGB8(s.Palette[l.colour].R, s.Palette[l.colour].G, s.Palette[l.colour].B).Hex()
		// Stacked outers of one colour share a <path>; cutout keeps one
		// path per shape, as Phase 1 did.
		if n := len(res.Paths); stacked && n > 0 && res.Paths[n-1].Fill == fill {
			res.Paths[n-1].Subpaths = append(res.Paths[n-1].Subpaths, subpaths[i]...)
			continue
		}
		res.Paths = append(res.Paths, svg.Path{Fill: fill, Subpaths: subpaths[i]})
	}
	return res, nil
}

// layers traces each colour's label mask into shapes, colours in a worker
// pool each owning one bitmap, and returns them in colour then scan order.
// The bw preset traces foreground only, with Phase 1's despeckling.
func (s *Segmentation) layers(o Options) []layer {
	colours := make([]int, 0, len(s.Palette))
	minArea := 0
	if o.Preset == PresetBW {
		colours, minArea = append(colours, 1), o.FilterSpeckle
	} else {
		for c := range s.Palette {
			colours = append(colours, c)
		}
	}
	perColour := make([][]contour.Shape, len(colours))
	var bufs sync.Pool
	parallel(len(colours), func(i int) {
		bm, _ := bufs.Get().(*bitmap.Bitmap)
		if bm == nil {
			bm = bitmap.New(s.Width, s.Height)
		} else {
			bm.Clear()
		}
		label := uint8(colours[i])
		for y := 0; y < s.Height; y++ {
			row := s.Labels[y*s.Width : (y+1)*s.Width]
			for x, l := range row {
				if l == label {
					bm.Set(x, y, true)
				}
			}
		}
		perColour[i] = contour.BuildShapes(contour.Trace(bm, o.TurnPolicy), minArea)
		bufs.Put(bm)
	})
	var out []layer
	for i, shapes := range perColour {
		for _, sh := range shapes {
			out = append(out, layer{colour: colours[i], shape: sh})
		}
	}
	return out
}

// depthOf counts the ancestors of shape i.
func depthOf(parents []int, i int) int {
	d := 0
	for p := parents[i]; p >= 0; p = parents[p] {
		d++
	}
	return d
}

// nearWhite reports whether palette entry i is within canvasWhite of white.
func (s *Segmentation) nearWhite(i int) bool {
	c := s.Palette[i]
	return colour.Dist(colour.FromSRGB8(c.R, c.G, c.B), colour.FromSRGB8(255, 255, 255)) <= canvasWhite
}

func pointLess(a, b geom.Point) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}

// parallel runs fn(0..n-1) on GOMAXPROCS workers and waits. Callers index
// results by i, so output never depends on scheduling.
func parallel(n int, fn func(i int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	if workers <= 1 {
		for i := 0; i < n; i++ {
			fn(i)
		}
		return
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				fn(i)
			}
		}()
	}
	wg.Wait()
}
