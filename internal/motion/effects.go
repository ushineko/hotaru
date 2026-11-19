package motion

import (
	"image"
	"math"
)

/*
shape is one region of a text layer, measured: each pixel's coverage, the
colour of the nearest glyph pixel (unpremultiplied), and, for a morph, the
signed distance to the glyphs' edge, negative inside.
*/
type shape struct {
	w, h   int
	alpha  []float32
	colour [][3]float32
	sdf    []float32
}

// measure takes a region's shape. signed adds the distance field a morph
// needs; a melt needs only the coverage and the colours.
func measure(img *image.RGBA, r image.Rectangle, signed bool) shape {
	w, h := r.Dx(), r.Dy()
	s := shape{w: w, h: h, alpha: make([]float32, w*h), colour: make([][3]float32, w*h)}
	inside := make([]bool, w*h)
	for y := range h {
		for x := range w {
			a := img.Pix[img.PixOffset(r.Min.X+x, r.Min.Y+y)+3]
			s.alpha[y*w+x] = float32(a) / 255
			inside[y*w+x] = a >= 128
		}
	}
	outside, nearest := edt(inside, w, h)
	for i, j := range nearest {
		if j < 0 {
			continue
		}
		o := img.PixOffset(r.Min.X+int(j)%w, r.Min.Y+int(j)/w)
		a := float32(img.Pix[o+3])
		if a > 0 {
			s.colour[i] = [3]float32{
				float32(img.Pix[o]) * 255 / a, float32(img.Pix[o+1]) * 255 / a, float32(img.Pix[o+2]) * 255 / a,
			}
		}
	}
	if signed {
		in, _ := edt(not(inside), w, h)
		s.sdf = make([]float32, w*h)
		for i := range s.sdf {
			s.sdf[i] = outside[i] - in[i]
		}
	}
	return s
}

func not(b []bool) []bool {
	out := make([]bool, len(b))
	for i, v := range b {
		out[i] = !v
	}
	return out
}

/*
edt is the exact Euclidean distance from every pixel to the nearest set
pixel, and that pixel's index (-1 where none is set): Felzenszwalb and
Huttenlocher's lower envelope, down the columns and then along the rows.
*/
func edt(set []bool, w, h int) ([]float32, []int32) {
	const inf = 1e20
	n := max(w, h)
	f := make([]float64, w*h)
	rowOf := make([]int32, w*h)
	g, d := make([]float64, n), make([]float64, n)
	arg := make([]int, n)
	v, z := make([]int, n), make([]float64, n+1)
	for x := range w {
		for y := range h {
			g[y] = inf
			if set[y*w+x] {
				g[y] = 0
			}
		}
		envelope(g[:h], d, arg, v, z)
		for y := range h {
			f[y*w+x] = d[y]
			rowOf[y*w+x] = int32(arg[y]) //nolint:gosec // a region is far below 2^31
		}
	}
	dist := make([]float32, w*h)
	near := make([]int32, w*h)
	for y := range h {
		copy(g[:w], f[y*w:(y+1)*w])
		envelope(g[:w], d, arg, v, z)
		for x := range w {
			dist[y*w+x] = float32(math.Sqrt(d[x]))
			if d[x] >= inf/2 {
				near[y*w+x] = -1
				continue
			}
			sx := arg[x]
			near[y*w+x] = rowOf[y*w+sx]*int32(w) + int32(sx) //nolint:gosec // as above
		}
	}
	return dist, near
}

// envelope is the 1D squared distance transform of f into d, with the index
// that gives each minimum in arg. v and z are scratch.
func envelope(f, d []float64, arg, v []int, z []float64) {
	n := len(f)
	k := 0
	v[0], z[0], z[1] = 0, math.Inf(-1), math.Inf(1)
	for q := 1; q < n; q++ {
		var s float64
		for {
			p := v[k]
			s = ((f[q] + float64(q*q)) - (f[p] + float64(p*p))) / float64(2*(q-p))
			if s > z[k] || k == 0 {
				break
			}
			k--
		}
		if s <= z[k] {
			k = 0
			v[0], z[0] = q, math.Inf(-1)
		} else {
			k++
			v[k], z[k] = q, s
		}
		z[k+1] = math.Inf(1)
	}
	k = 0
	for q := range n {
		for z[k+1] < float64(q) {
			k++
		}
		p := v[k]
		d[q] = float64((q-p)*(q-p)) + f[p]
		arg[q] = p
	}
}

// put writes an unpremultiplied colour at coverage a, or clears the pixel.
func put(out *image.RGBA, o int, c [3]float32, a float32) {
	if a <= 0 {
		out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = 0, 0, 0, 0
		return
	}
	a = min(a, 1)
	out.Pix[o] = uint8(min(255, c[0]*a))
	out.Pix[o+1] = uint8(min(255, c[1]*a))
	out.Pix[o+2] = uint8(min(255, c[2]*a))
	out.Pix[o+3] = uint8(a * 255)
}

func mix(a, b [3]float32, t float32) [3]float32 {
	return [3]float32{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

/*
melt blurs the region's old and new text, cross-fades them and cuts the
result back to a hard edge. The blur is widest halfway and scaled to the
region's height, so small numbers melt gently and the big one runs.
*/
func melt(out *image.RGBA, r image.Rectangle, p pair, t float64) {
	e := float32(Ease(t))
	w, h := p.to.w, p.to.h
	field := make([]float32, w*h)
	for i := range field {
		field[i] = p.from.alpha[i]*(1-e) + p.to.alpha[i]*e
	}
	radius := int(float64(h) * 0.07 * math.Sin(math.Pi*t))
	for range 3 {
		blur(field, w, h, radius)
	}
	for y := range h {
		for x := range w {
			i := y*w + x
			cov := min(1, max(0, (field[i]-0.42)/0.16))
			put(out, out.PixOffset(r.Min.X+x, r.Min.Y+y), mix(p.from.colour[i], p.to.colour[i], e), cov)
		}
	}
}

// morph blends the old and new text's distance fields and cuts at zero, with
// a one-pixel soft edge: the digits flow from one shape into the next.
func morph(out *image.RGBA, r image.Rectangle, p pair, t float64) {
	e := float32(Ease(t))
	w, h := p.to.w, p.to.h
	for y := range h {
		for x := range w {
			i := y*w + x
			d := p.from.sdf[i]*(1-e) + p.to.sdf[i]*e
			put(out, out.PixOffset(r.Min.X+x, r.Min.Y+y), mix(p.from.colour[i], p.to.colour[i], e), 0.5-d)
		}
	}
}

/*
roll moves the region's old text up and out while the new rolls in from below,
an odometer's wheel, smeared along the motion in proportion to its speed.
*/
func roll(out, from, to *image.RGBA, r image.Rectangle, t float64) {
	const samples = 12
	e := Ease(t)
	h := float64(r.Dy())
	smear := math.Sin(math.Pi*t) * h * 0.10
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			var acc [4]float64
			for s := range samples {
				off := smear * (float64(s)/(samples-1) - 0.5)
				for _, src := range []struct {
					img   *image.RGBA
					shift float64
				}{{from, e * h}, {to, -(1 - e) * h}} {
					sy := int(math.Round(float64(y) + src.shift + off))
					if sy < r.Min.Y || sy >= r.Max.Y {
						continue
					}
					o := src.img.PixOffset(x, sy)
					for c := range 4 {
						acc[c] += float64(src.img.Pix[o+c])
					}
				}
			}
			o := out.PixOffset(x, y)
			for c := range 4 {
				out.Pix[o+c] = uint8(min(255, acc[c]/samples))
			}
		}
	}
}

// blur is a box blur of a w by h field in place, rows then columns, with a
// window of 2r+1 and the edges held.
func blur(f []float32, w, h, r int) {
	if r < 1 {
		return
	}
	win := float32(2*r + 1)
	tmp := make([]float32, max(w, h))
	for y := range h {
		row := f[y*w : (y+1)*w]
		var acc float32
		for x := -r; x <= r; x++ {
			acc += row[min(max(x, 0), w-1)]
		}
		for x := range w {
			tmp[x] = acc / win
			acc += row[min(x+r+1, w-1)] - row[max(x-r, 0)]
		}
		copy(row, tmp[:w])
	}
	for x := range w {
		var acc float32
		for y := -r; y <= r; y++ {
			acc += f[min(max(y, 0), h-1)*w+x]
		}
		for y := range h {
			tmp[y] = acc / win
			acc += f[min(y+r+1, h-1)*w+x] - f[max(y-r, 0)*w+x]
		}
		for y := range h {
			f[y*w+x] = tmp[y]
		}
	}
}

/*
Glow lights the comet's head: the pixels where the rings now differ from the
rings a moment ago, which is where the arcs have just swept, brightened
towards white with a soft halo, by strength (1 at the start, fading to 0).
*/
func Glow(now, before *image.RGBA, strength float64) {
	if strength <= 0 {
		return
	}
	// Only around what changed: the sweep is a strip of the ring, and
	// blurring the whole frame twice was most of a comet frame's cost.
	b := now.Bounds()
	area := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if differs(now, before, now.PixOffset(x, y)) {
				area = area.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if area.Empty() {
		return
	}
	area = area.Inset(-halo).Intersect(b)
	w, h := area.Dx(), area.Dy()
	field := make([]float32, w*h)
	for y := range h {
		for x := range w {
			if differs(now, before, now.PixOffset(area.Min.X+x, area.Min.Y+y)) {
				field[y*w+x] = 1
			}
		}
	}
	soft := append([]float32(nil), field...)
	blur(soft, w, h, 6)
	blur(soft, w, h, 6)
	s := float32(strength)
	for y := range h {
		for x := range w {
			g := min(1, field[y*w+x]*s+soft[y*w+x]*s*1.5)
			if g <= 0 {
				continue
			}
			o := now.PixOffset(area.Min.X+x, area.Min.Y+y)
			a := max(float32(now.Pix[o+3]), g*255)
			for c := range 3 {
				v := float32(now.Pix[o+c])
				now.Pix[o+c] = uint8(min(a, v+(a-v)*g))
			}
			now.Pix[o+3] = uint8(a)
		}
	}
}

// halo is how far the glow reaches past the sweep: two blurs of 6.
const halo = 12

// differs says whether two layers differ at a pixel by more than rounding.
func differs(a, b *image.RGBA, o int) bool {
	for c := range 4 {
		if d := int(a.Pix[o+c]) - int(b.Pix[o+c]); d > 16 || d < -16 {
			return true
		}
	}
	return false
}
