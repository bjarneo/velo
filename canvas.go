package main

import (
	"strconv"
	"strings"
)

// Cell is one terminal cell.
type Cell struct {
	Ch     rune
	Fg, Bg RGB
	Bold   bool
}

// action identifies what a mouse click on a region does.
type action struct {
	kind string
	idx  int
}

type hitRegion struct {
	x, y, w, h int
	act        action
}

// Canvas is a grid of cells that serializes to ANSI text. Every glyph that
// velo draws occupies one column, so a rune maps to one cell.
type Canvas struct {
	W, H  int
	cells []Cell
	hits  []hitRegion
}

func newCanvas(w, h int, fg, bg RGB) *Canvas {
	c := &Canvas{W: w, H: h, cells: make([]Cell, w*h)}
	for i := range c.cells {
		c.cells[i] = Cell{Ch: ' ', Fg: fg, Bg: bg}
	}
	return c
}

func (c *Canvas) in(x, y int) bool { return x >= 0 && y >= 0 && x < c.W && y < c.H }

func (c *Canvas) at(x, y int) *Cell { return &c.cells[y*c.W+x] }

// put draws a glyph and keeps the cell background.
func (c *Canvas) put(x, y int, ch rune, fg RGB, bold bool) {
	if !c.in(x, y) {
		return
	}
	cl := c.at(x, y)
	cl.Ch, cl.Fg, cl.Bold = ch, fg, bold
}

// putBg draws a glyph with an explicit background.
func (c *Canvas) putBg(x, y int, ch rune, fg, bg RGB, bold bool) {
	if !c.in(x, y) {
		return
	}
	*c.at(x, y) = Cell{Ch: ch, Fg: fg, Bg: bg, Bold: bold}
}

// text draws s from x and returns the column after the last glyph.
func (c *Canvas) text(x, y int, s string, fg RGB, bold bool) int {
	for _, r := range s {
		c.put(x, y, r, fg, bold)
		x++
	}
	return x
}

// textBg draws s with an explicit background.
func (c *Canvas) textBg(x, y int, s string, fg, bg RGB, bold bool) int {
	for _, r := range s {
		c.putBg(x, y, r, fg, bg, bold)
		x++
	}
	return x
}

// textR draws s so that its last glyph ends at column right-1.
func (c *Canvas) textR(right, y int, s string, fg RGB, bold bool) int {
	x := right - runeLen(s)
	c.text(x, y, s, fg, bold)
	return x
}

// textC centers s on column cx.
func (c *Canvas) textC(cx, y int, s string, fg RGB, bold bool) {
	c.text(cx-runeLen(s)/2, y, s, fg, bold)
}

// span is one styled run of text.
type span struct {
	s    string
	fg   RGB
	bold bool
}

func (c *Canvas) spans(x, y int, ss ...span) int {
	for _, s := range ss {
		x = c.text(x, y, s.s, s.fg, s.bold)
	}
	return x
}

func spansLen(ss ...span) int {
	n := 0
	for _, s := range ss {
		n += runeLen(s.s)
	}
	return n
}

// fill clears a rectangle to background bg.
func (c *Canvas) fill(x, y, w, h int, fg, bg RGB) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			c.putBg(i, j, ' ', fg, bg, false)
		}
	}
}

// tint sets the background of a rectangle and keeps its glyphs.
func (c *Canvas) tint(x, y, w, h int, bg RGB) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			if c.in(i, j) {
				c.at(i, j).Bg = bg
			}
		}
	}
}

func (c *Canvas) hline(x, y, w int, ch rune, fg RGB) {
	for i := 0; i < w; i++ {
		c.put(x+i, y, ch, fg, false)
	}
}

// fade moves every cell toward bg. It dims the screen behind a modal.
func (c *Canvas) fade(bg RGB, keep float64) {
	for i := range c.cells {
		c.cells[i].Fg = mix(c.cells[i].Fg, bg, keep)
		c.cells[i].Bg = mix(c.cells[i].Bg, bg, keep)
		c.cells[i].Bold = false
	}
}

// hit registers a click region. A later region wins over an earlier one.
func (c *Canvas) hit(x, y, w, h int, kind string, idx int) {
	c.hits = append(c.hits, hitRegion{x, y, w, h, action{kind, idx}})
}

func (c *Canvas) hitTest(x, y int) (action, bool) {
	for i := len(c.hits) - 1; i >= 0; i-- {
		r := c.hits[i]
		if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
			return r.act, true
		}
	}
	return action{}, false
}

// String serializes the canvas with 24-bit SGR sequences. Bubble Tea
// downsamples the colors when the terminal has fewer.
func (c *Canvas) String() string {
	var b strings.Builder
	b.Grow(c.W * c.H * 4)
	for y := 0; y < c.H; y++ {
		var prev *Cell
		for x := 0; x < c.W; x++ {
			cl := c.at(x, y)
			if prev == nil || cl.Fg != prev.Fg || cl.Bg != prev.Bg || cl.Bold != prev.Bold {
				b.WriteString("\x1b[0")
				if cl.Bold {
					b.WriteString(";1")
				}
				writeRGB(&b, "38", cl.Fg)
				writeRGB(&b, "48", cl.Bg)
				b.WriteByte('m')
			}
			b.WriteRune(cl.Ch)
			prev = cl
		}
		b.WriteString("\x1b[0m")
		if y < c.H-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func writeRGB(b *strings.Builder, kind string, c RGB) {
	b.WriteString(";" + kind + ";2;")
	b.WriteString(strconv.Itoa(int(c.R)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(c.G)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(c.B)))
}

func runeLen(s string) int { return len([]rune(s)) }

// fit cuts s to w columns and marks a cut with an ellipsis.
func fit(s string, w int) string {
	r := []rune(s)
	if w <= 0 {
		return ""
	}
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

func padR(s string, w int) string {
	n := runeLen(s)
	if n >= w {
		return fit(s, w)
	}
	return s + strings.Repeat(" ", w-n)
}

func padL(s string, w int) string {
	n := runeLen(s)
	if n >= w {
		return fit(s, w)
	}
	return strings.Repeat(" ", w-n) + s
}
