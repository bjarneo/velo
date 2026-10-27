package main

import "math"

// cellAspect is the height of a terminal cell divided by its width. The
// gauge uses it to draw a round arc.
const cellAspect = 2.1

// brailleBit maps a dot at column i, row j of a cell to its bit.
var brailleBit = [2][4]uint8{{1, 2, 4, 64}, {8, 16, 32, 128}}

// glyph is one braille cell. Cat is the highest category of its dots and
// selects the color. Cat 0 means the cell is empty.
type glyph struct {
	ch  rune
	cat uint8
}

// pack turns a dot grid of w×h categories into rows of braille glyphs.
func pack(g []uint8, w, h int) [][]glyph {
	cols, rows := w/2, h/4
	out := make([][]glyph, rows)
	for r := 0; r < rows; r++ {
		row := make([]glyph, cols)
		for c := 0; c < cols; c++ {
			var bits, cat uint8
			for i := 0; i < 2; i++ {
				for j := 0; j < 4; j++ {
					if k := g[(r*4+j)*w+c*2+i]; k != 0 {
						bits |= brailleBit[i][j]
						cat = max(cat, k)
					}
				}
			}
			row[c] = glyph{rune(0x2800 + int(bits)), cat}
		}
		out[r] = row
	}
	return out
}

// gaugeGeom places a 240° arc in a box of cols×rows cells. Units are cell
// widths on both axes.
type gaugeGeom struct {
	cols, rows int
	r, cx, cy  float64
}

// newGaugeGeom fits the largest arc, and its scale labels, into the box.
func newGaugeGeom(cols, rows int) gaugeGeom {
	r := math.Min(float64(rows-1)*cellAspect/1.62, (float64(cols)-4)/2.16)
	return gaugeGeom{cols: cols, rows: rows, r: r, cx: float64(cols) / 2, cy: 1.08*r + 0.5*cellAspect}
}

// labelAt returns the cell for the scale label at fraction t of the arc.
func (g gaugeGeom) labelAt(t float64) (x, y int) {
	a := (210 - 240*t) * math.Pi / 180
	rr := g.r*1.08 + 1
	return int(math.Round(g.cx + rr*math.Cos(a))), int((g.cy - rr*math.Sin(a)) / cellAspect)
}

// gaugeGrid draws the arc. frac lights the arc up to that fraction. When
// sweep is in [0,1], a moving highlight replaces frac.
func gaugeGrid(g gaugeGeom, frac, sweep float64) [][]glyph {
	w, h := g.cols*2, g.rows*4
	dw, dh := 0.5, cellAspect/4
	grid := make([]uint8, w*h)
	band, tickOut, tickIn, tickMinor := g.r*0.89, g.r*0.86, g.r*0.78, g.r*0.82
	for y := 0; y < h; y++ {
		dy := g.cy - (float64(y)+.5)*dh
		for x := 0; x < w; x++ {
			dx := (float64(x)+.5)*dw - g.cx
			rr := math.Hypot(dx, dy)
			if rr > g.r || rr < tickIn {
				continue
			}
			a := math.Atan2(dy, dx) * 180 / math.Pi
			if a < -90 {
				a += 360
			}
			t := (210 - a) / 240
			if t < 0 || t > 1 {
				continue
			}
			var k uint8
			switch {
			case rr >= band:
				if math.Mod(t*240, 7.5) > 5.7 {
					continue
				}
				switch {
				case sweep >= 0:
					d := math.Abs(t - sweep)
					k = 1
					if d < .014 {
						k = 4
					} else if d < .09 {
						k = 3
					}
				case frac > 0 && t <= frac:
					k = 3
					if t > frac-.02 {
						k = 4
					}
				default:
					k = 1
				}
			case rr <= tickOut:
				q := t * 16
				m := math.Round(q)
				// distance from the dot to the tick ray, in cell widths
				if math.Abs(q-m)/16*240*math.Pi/180*rr > 0.3 {
					continue
				}
				if int(m)%2 == 1 && rr < tickMinor {
					continue
				}
				k = 2
			default:
				continue
			}
			grid[y*w+x] = k
		}
	}
	return pack(grid, w, h)
}

// sampleAt returns the throughput sample at t seconds into the test.
func sampleAt(dl, ul []float64, t float64) (float64, bool) {
	arr, i := dl, int(t/sampleStep)
	if t >= dlDur {
		arr, i = ul, int((t-dlDur)/sampleStep)
	}
	if i < 0 || i >= len(arr) {
		return 0, false
	}
	return arr[i], true
}

// areaGrid draws download then upload as one filled area over 15 s.
// Categories: 1 grid, 2 download fill, 3 download edge, 4 upload fill,
// 5 upload edge.
func areaGrid(cols, rows int, dl, ul []float64, yMax float64) [][]glyph {
	w, h := cols*2, rows*4
	total := dlDur + ulDur
	xd := int(math.Round(dlDur / total * float64(w)))
	g := make([]uint8, w*h)
	for x := 0; x < w; x++ {
		t := (float64(x) + .5) / float64(w) * total
		down := t < dlDur
		v, ok := sampleAt(dl, ul, t)
		top := -1
		if ok {
			top = max(1, min(h, int(math.Round(v/yMax*float64(h)))))
		}
		for y := 0; y < h; y++ {
			yb := h - y
			var k uint8
			if top >= yb {
				switch {
				case yb >= top-1 && down:
					k = 3
				case yb >= top-1:
					k = 5
				case (x+yb)%2 == 0 && down:
					k = 2
				case (x+yb)%2 == 0:
					k = 4
				}
			}
			if k == 0 && (((yb == h || yb == h/2 || yb == 1) && x%4 == 0) || (x == xd && yb%2 == 0)) {
				k = 1
			}
			g[y*w+x] = k
		}
	}
	return pack(g, w, h)
}

// ledFont is a 5×7 dot-matrix font for the hero readout.
var ledFont = map[rune][]string{
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
	'.': {"0", "0", "0", "0", "0", "0", "1"},
}

// ledWidth returns the columns that drawLED uses for s.
func ledWidth(s string) int {
	n := 0
	for i, r := range s {
		if i > 0 {
			n++
		}
		n += len(ledFont[r][0])
	}
	return n
}

// drawLED draws s as dot-matrix digits, two pixel rows per cell. The first
// ghost characters use the ghost color. Off pixels use the off color.
func drawLED(c *Canvas, x, y int, s string, ghost int, on, ghostC, off, bg RGB) {
	for i, r := range []rune(s) {
		bm := ledFont[r]
		lit := on
		if i < ghost {
			lit = ghostC
		}
		px := func(row, col int) RGB {
			if row >= len(bm) {
				return bg
			}
			if bm[row][col] == '1' {
				return lit
			}
			return off
		}
		for row := 0; row < 4; row++ {
			for col := 0; col < len(bm[0]); col++ {
				c.putBg(x+col, y+row, '▀', px(row*2, col), px(row*2+1, col), false)
			}
		}
		x += len(bm[0]) + 1
	}
}
