package main

import (
	"image"
	"math"
)

// THROWAWAY prototype effects for spec 081: how a change of reading could be
// drawn on the text and the rings. Every function works on 640x640 RGBA
// layers with premultiplied alpha, as image.RGBA stores them.

const side = 640

// shape is a text layer prepared for morphing: its signed distance field
// (negative inside the glyphs), and for every pixel the colour of the nearest
// glyph pixel, unpremultiplied.
type shape struct {
	sdf    []float32
	colour [][3]float32
	alpha  []float32
}

// prepare measures a layer once per change.
func prepare(img *image.RGBA) shape {
	n := side * side
	inside := make([]bool, n)
	alpha := make([]float32, n)
	for i := range n {
		a := img.Pix[i*4+3]
		alpha[i] = float32(a) / 255
		inside[i] = a >= 128
	}
	outsideDist, nearest := edt(inside)
	insideDist, _ := edt(not(inside))
	s := shape{sdf: make([]float32, n), colour: make([][3]float32, n), alpha: alpha}
	for i := range n {
		s.sdf[i] = outsideDist[i] - insideDist[i]
		j := nearest[i]
		if j < 0 {
			continue
		}
		a := float32(img.Pix[j*4+3])
		if a == 0 {
			continue
		}
		s.colour[i] = [3]float32{float32(img.Pix[j*4]) * 255 / a, float32(img.Pix[j*4+1]) * 255 / a, float32(img.Pix[j*4+2]) * 255 / a}
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
edt is the exact Euclidean distance from every pixel to the nearest true
pixel, and that pixel's index (-1 where there is none): Felzenszwalb and
Huttenlocher's two passes of the 1D lower envelope.
*/
func edt(set []bool) ([]float32, []int32) {
	const inf = 1e20
	n := side
	f := make([]float64, n*n)
	colOf := make([]int32, n*n) // nearest set row in the column, per pixel
	// columns
	g := make([]float64, n)
	d := make([]float64, n)
	arg := make([]int, n)
	for x := range n {
		for y := range n {
			if set[y*n+x] {
				g[y] = 0
			} else {
				g[y] = inf
			}
		}
		envelope(g, d, arg)
		for y := range n {
			f[y*n+x] = d[y]
			colOf[y*n+x] = int32(arg[y]) //nolint:gosec // < 640
		}
	}
	dist := make([]float32, n*n)
	near := make([]int32, n*n)
	for y := range n {
		for x := range n {
			g[x] = f[y*n+x]
		}
		envelope(g, d, arg)
		for x := range n {
			dist[y*n+x] = float32(math.Sqrt(d[x]))
			if d[x] >= inf/2 {
				near[y*n+x] = -1
				continue
			}
			sx := arg[x]
			near[y*n+x] = colOf[y*n+sx]*int32(n) + int32(sx) //nolint:gosec // < 640*640
		}
	}
	return dist, near
}

// envelope is the 1D squared distance transform of f into d, with the
// minimising index in arg.
func envelope(f, d []float64, arg []int) {
	n := len(f)
	v := make([]int, n)
	z := make([]float64, n+1)
	k := 0
	v[0], z[0], z[1] = 0, math.Inf(-1), math.Inf(1)
	for q := 1; q < n; q++ {
		var s float64
		for {
			p := v[k]
			s = ((f[q] + float64(q*q)) - (f[p] + float64(p*p))) / float64(2*q-2*p)
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

func ease(t float64) float64 { return t * t * (3 - 2*t) }

func clamp01(v float32) float32 { return min(1, max(0, v)) }

// put writes an unpremultiplied colour at coverage a.
func put(out *image.RGBA, i int, c [3]float32, a float32) {
	if a <= 0 {
		return
	}
	out.Pix[i*4] = uint8(c[0] * a)
	out.Pix[i*4+1] = uint8(c[1] * a)
	out.Pix[i*4+2] = uint8(c[2] * a)
	out.Pix[i*4+3] = uint8(a * 255)
}

func mix3(a, b [3]float32, t float32) [3]float32 {
	return [3]float32{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

// morph is the liquid morph: the two distance fields blended, cut at zero,
// with a one-pixel soft edge.
func morph(a, b shape, t float64) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, side, side))
	e := float32(ease(t))
	for i := range a.sdf {
		d := a.sdf[i]*(1-e) + b.sdf[i]*e
		if d > 1 {
			continue
		}
		put(out, i, mix3(a.colour[i], b.colour[i], e), clamp01(0.5-d))
	}
	return out
}

// gooey is the melt: inside each changed region, both layers blurred and
// cross-faded, then cut back to a hard edge, the blur widest halfway and
// scaled to the region's height. Text that did not change stays as it is.
func gooey(a, b shape, textB *image.RGBA, boxes []box, t float64) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, side, side))
	copy(out.Pix, textB.Pix)
	e := float32(ease(t))
	for _, bx := range boxes {
		w, h := bx.x1-bx.x0, bx.y1-bx.y0
		radius := int(float64(h) * 0.07 * math.Sin(math.Pi*t))
		field := make([]float32, w*h)
		for y := range h {
			for x := range w {
				i := (bx.y0+y)*side + bx.x0 + x
				field[y*w+x] = a.alpha[i]*(1-e) + b.alpha[i]*e
			}
		}
		for range 3 {
			blurRegion(field, w, h, radius)
		}
		for y := range h {
			for x := range w {
				i := (bx.y0+y)*side + bx.x0 + x
				for c := range 4 {
					out.Pix[i*4+c] = 0
				}
				cov := clamp01((field[y*w+x] - 0.42) / 0.16)
				put(out, i, mix3(a.colour[i], b.colour[i], e), cov)
			}
		}
	}
	return out
}

// blurRegion is boxBlur on a w by h field.
func blurRegion(f []float32, w, h, r int) {
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

// boxBlur blurs a field in place, rows then columns, with a window of 2r+1.
func boxBlur(f []float32, r int) {
	if r < 1 {
		return
	}
	tmp := make([]float32, side)
	w := float32(2*r + 1)
	for y := range side {
		row := f[y*side : (y+1)*side]
		var acc float32
		for x := -r; x <= r; x++ {
			acc += row[min(max(x, 0), side-1)]
		}
		for x := range side {
			tmp[x] = acc / w
			acc += row[min(x+r+1, side-1)] - row[max(x-r, 0)]
		}
		copy(row, tmp)
	}
	for x := range side {
		var acc float32
		for y := -r; y <= r; y++ {
			acc += f[min(max(y, 0), side-1)*side+x]
		}
		for y := range side {
			tmp[y] = acc / w
			acc += f[min(y+r+1, side-1)*side+x] - f[max(y-r, 0)*side+x]
		}
		for y := range side {
			f[y*side+x] = tmp[y]
		}
	}
}

// box is a region of the text that changed.
type box struct{ x0, y0, x1, y1 int }

// changedBoxes finds the regions where two text layers differ: the differing
// pixels, grown a little, in connected pieces.
func changedBoxes(a, b *image.RGBA) []box {
	n := side * side
	diff := make([]bool, n)
	for i := range n {
		for c := range 4 {
			if d := int(a.Pix[i*4+c]) - int(b.Pix[i*4+c]); d > 24 || d < -24 {
				diff[i] = true
				break
			}
		}
	}
	const grow = 6
	grown := make([]bool, n)
	for y := range side {
		for x := range side {
			if !diff[y*side+x] {
				continue
			}
			for dy := -grow; dy <= grow; dy++ {
				for dx := -grow; dx <= grow; dx++ {
					if xx, yy := x+dx, y+dy; xx >= 0 && yy >= 0 && xx < side && yy < side {
						grown[yy*side+xx] = true
					}
				}
			}
		}
	}
	seen := make([]bool, n)
	var out []box
	for i := range n {
		if !grown[i] || seen[i] {
			continue
		}
		bx := box{side, side, 0, 0}
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := j%side, j/side
			bx.x0, bx.y0, bx.x1, bx.y1 = min(bx.x0, x), min(bx.y0, y), max(bx.x1, x+1), max(bx.y1, y+1)
			for _, k := range []int{j - 1, j + 1, j - side, j + side} {
				if k >= 0 && k < n && grown[k] && !seen[k] && (k%side-x)*(k%side-x) <= 1 {
					seen[k] = true
					stack = append(stack, k)
				}
			}
		}
		out = append(out, bx)
	}
	return out
}

// roll is the odometer: in each changed region the old text rolls up and
// out while the new rolls in from below, smeared along the motion.
func roll(a, b *image.RGBA, boxes []box, t float64) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, side, side))
	copy(out.Pix, b.Pix)
	e := ease(t)
	speed := math.Sin(math.Pi * t) // fastest halfway
	const samples = 12
	for _, bx := range boxes {
		h := float64(bx.y1 - bx.y0)
		smear := speed * h * 0.10
		for y := bx.y0; y < bx.y1; y++ {
			for x := bx.x0; x < bx.x1; x++ {
				var acc [4]float64
				for s := range samples {
					off := smear * (float64(s)/(samples-1) - 0.5)
					add := func(img *image.RGBA, shift float64) {
						sy := int(math.Round(float64(y) + shift + off))
						if sy < bx.y0 || sy >= bx.y1 {
							return
						}
						j := (sy*side + x) * 4
						for c := range 4 {
							acc[c] += float64(img.Pix[j+c])
						}
					}
					add(a, e*h)       // old: moved up by e*h, so sample below
					add(b, -(1-e)*h) // new: coming up from below
				}
				i := (y*side + x) * 4
				for c := range 4 {
					out.Pix[i+c] = uint8(min(255, acc[c]/samples))
				}
			}
		}
	}
	return out
}

// over lays src over dst, both premultiplied.
func over(dst, src *image.RGBA) {
	for i := 0; i < len(dst.Pix); i += 4 {
		a := uint32(src.Pix[i+3])
		if a == 0 {
			continue
		}
		for c := range 4 {
			dst.Pix[i+c] = uint8(uint32(src.Pix[i+c]) + uint32(dst.Pix[i+c])*(255-a)/255)
		}
	}
}

// glow brightens the pixels where two ring layers differ (the arc's sweep
// over the last moment) towards white, with a soft halo.
func glow(now, before *image.RGBA, strength float64) {
	field := make([]float32, side*side)
	for i := range field {
		for c := range 4 {
			if d := int(now.Pix[i*4+c]) - int(before.Pix[i*4+c]); d > 16 || d < -16 {
				field[i] = 1
				break
			}
		}
	}
	halo := append([]float32(nil), field...)
	boxBlur(halo, 6)
	boxBlur(halo, 6)
	for i := range field {
		w := clamp01(field[i]*float32(strength) + halo[i]*float32(strength)*1.5)
		if w == 0 {
			continue
		}
		a := max(float32(now.Pix[i*4+3]), w*255)
		for c := range 3 {
			v := float32(now.Pix[i*4+c])
			now.Pix[i*4+c] = uint8(min(a, v+(a-v)*w))
		}
		now.Pix[i*4+3] = uint8(a)
	}
}

// spring is a sweep that overshoots and settles.
func spring(t float64) float64 {
	return 1 - math.Exp(-5*t)*math.Cos(9*t)
}
