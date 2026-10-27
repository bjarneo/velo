package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const spinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
const sparkChars = "▁▂▃▄▅▆▇█"

// gaugeScale is the set of labels on the arc. Faster links use a wider set.
type gaugeScale struct {
	labels []string
	vals   []float64
}

var gaugeScales = []gaugeScale{
	{[]string{"0", "5", "10", "25", "50", "100", "250", "500", "1G"}, []float64{0, 5, 10, 25, 50, 100, 250, 500, 1000}},
	{[]string{"0", "10", "25", "50", "100", "250", "500", "1G", "2.5G"}, []float64{0, 10, 25, 50, 100, 250, 500, 1000, 2500}},
	{[]string{"0", "25", "50", "100", "250", "500", "1G", "2.5G", "10G"}, []float64{0, 25, 50, 100, 250, 500, 1000, 2500, 10000}},
}

func scaleFor(peak float64) gaugeScale {
	for _, s := range gaugeScales {
		if peak <= s.vals[len(s.vals)-1] {
			return s
		}
	}
	return gaugeScales[len(gaugeScales)-1]
}

// frac maps a value to its position on the arc, linear inside each segment.
func (s gaugeScale) frac(v float64) float64 {
	n := len(s.vals) - 1
	if v <= 0 {
		return 0
	}
	for i := 0; i < n; i++ {
		if v <= s.vals[i+1] {
			return (float64(i) + (v-s.vals[i])/(s.vals[i+1]-s.vals[i])) / float64(n)
		}
	}
	return 1
}

func fmt6(v float64) string {
	s := fmt.Sprintf("%.1f", math.Min(9999.9, math.Max(0, v)))
	return strings.Repeat("0", max(0, 6-len(s))) + s
}

// leadZeros counts the leading zeros that the readout shows as ghosts.
func leadZeros(s string) int {
	n := 0
	for n < len(s)-3 && s[n] == '0' {
		n++
	}
	return n
}

func fmtMB(mb float64) string {
	if mb < 1000 {
		return fmt.Sprintf("%.0f MB", mb)
	}
	return fmt.Sprintf("%.2f GB", mb/1000)
}

func fmtT(s float64) string {
	return fmt.Sprintf("%02d:%04.1f", int(s)/60, math.Mod(s, 60))
}

func niceMax(v float64) float64 {
	for _, n := range []float64{25, 50, 100, 250, 500, 1000, 1500, 2000, 2500, 5000, 10000} {
		if n >= v {
			return n
		}
	}
	return 25000
}

func maxOf(a []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	return slices.Max(a)
}

func (m *model) theme() Theme { return m.themes[m.themeIdx] }

func (m *model) phaseColor(p Phase) RGB {
	t := m.theme()
	switch p {
	case phasePing:
		return t.Yel
	case phaseDown:
		return t.Cyan
	case phaseUp:
		return t.Mag
	case phaseDone:
		return t.Grn
	case phaseFailed:
		return t.Red
	}
	return t.Acc
}

func (m *model) gradeColor(g string) RGB {
	t := m.theme()
	switch {
	case strings.HasPrefix(g, "A"):
		return t.Grn
	case g == "B":
		return t.Yel
	}
	return t.Red
}

func (m *model) spin() string {
	r := []rune(spinner)
	return string(r[int(m.now.UnixMilli()/90)%len(r)])
}

func (m *model) blink() bool { return (m.now.UnixMilli()/530)%2 == 0 }

func (m *model) render() *Canvas {
	t := m.theme()
	W, H := max(m.w, 1), max(m.h, 1)
	c := newCanvas(W, H, t.Fg, t.Bg)
	if W < 64 || H < 22 {
		msg := fmt.Sprintf("velo needs 64×22 · this terminal is %d×%d", W, H)
		c.textC(W/2, H/2, fit(msg, W), t.Dim, false)
		return c
	}
	padX, padY := 2, 1
	if W < 110 {
		padX = 1
	}
	if H < 40 {
		padY = 0
	}
	top, foot := padY, H-1-padY
	m.drawHeader(c, padX, top, W-2*padX)
	m.drawFooter(c, padX, foot, W-2*padX)
	gap := 1
	if H < 34 {
		gap = 0
	}
	y0 := top + 1 + gap
	m.drawCockpit(c, padX, y0, W-2*padX, foot-gap-y0)

	switch m.modal {
	case modalServers:
		m.drawServers(c)
	case modalHistory:
		m.drawHistory(c)
	case modalHelp:
		m.drawHelp(c)
	}
	return c
}

func (m *model) status() (string, RGB) {
	t := m.theme()
	dot := "○"
	if m.blink() {
		dot = "●"
	}
	switch m.snap.Phase {
	case phasePing:
		return dot + " PROBING", t.Yel
	case phaseDown:
		return dot + " DOWNLOADING", t.Cyan
	case phaseUp:
		return dot + " UPLOADING", t.Mag
	case phaseDone:
		return "✓ COMPLETE", t.Grn
	case phaseFailed:
		return "✗ FAILED", t.Red
	}
	if m.discovering {
		return m.spin() + " LOCATING", t.Dim
	}
	return "○ READY", t.Dim
}

func (m *model) drawHeader(c *Canvas, x, y, w int) {
	t := m.theme()
	end := c.textBg(x, y, " ◢ VELO ", t.Bg, t.Acc, true)
	if m.demo {
		c.text(end+2, y, "DEMO", t.Yel, true)
	}
	right := x + w
	right = c.textR(right, y, m.now.Format("15:04:05"), t.Fg, false) - 3
	if sv, ok := m.currentServer(); ok {
		s := fmt.Sprintf("%s · %s · %.0f km", lower(sv.Sponsor), lower(sv.City), sv.Km)
		right = c.textR(right, y, fit(s, max(0, right-end-20)), t.Dim, false) - 3
	}
	label, col := m.status()
	c.textR(right, y, label, col, true)
}

type keycap struct{ cap, label, key string }

func (m *model) drawFooter(c *Canvas, x, y, w int) {
	t := m.theme()
	keys := []keycap{{"⏎", "run", "enter"}, {"esc", "abort", "esc"}, {"s", "server", "s"}, {"h", "history", "h"}}
	if m.demo {
		keys = append(keys, keycap{"p", "profile", "p"})
	}
	keys = append(keys, keycap{"t", "theme", "t"}, keycap{"?", "help", "?"}, keycap{"q", "quit", "q"})

	mode := "live"
	if m.demo {
		mode = lower(demoProfiles[m.profile].Name)
	}
	right := lower(m.theme().Name) + " · " + mode

	// Narrow terminals first lose the right text, then the wide gaps,
	// then the key labels.
	gap, labels := 2, true
	width := func() int {
		n := 0
		for _, k := range keys {
			n += runeLen(k.cap) + 2 + gap
			if labels {
				n += 1 + runeLen(k.label)
			}
		}
		return n - gap
	}
	limit := x + w
	if width()+runeLen(right)+2 <= w {
		limit = c.textR(x+w, y, right, t.Dim, false) - 2
	} else if gap = 1; width() > w {
		labels = false
	}
	cx := x
	for _, k := range keys {
		need := runeLen(k.cap) + 2
		if labels {
			need += 1 + runeLen(k.label)
		}
		if cx+need > limit {
			break
		}
		start := cx
		cx = c.textBg(cx, y, " "+k.cap+" ", t.Fg, t.Sel, true)
		if labels {
			cx = c.text(cx+1, y, k.label, t.Dim, false)
		}
		c.hit(start, y, cx-start, 1, "key:"+k.key, 0)
		cx += gap
	}
}

// panel draws a rounded box with a title and an optional tag in the border.
func (m *model) panel(c *Canvas, x, y, w, h int, title, tag string, border RGB) {
	t := m.theme()
	c.put(x, y, '╭', border, false)
	c.put(x+w-1, y, '╮', border, false)
	c.put(x, y+h-1, '╰', border, false)
	c.put(x+w-1, y+h-1, '╯', border, false)
	c.hline(x+1, y, w-2, '─', border)
	c.hline(x+1, y+h-1, w-2, '─', border)
	for j := y + 1; j < y+h-1; j++ {
		c.put(x, j, '│', border, false)
		c.put(x+w-1, j, '│', border, false)
	}
	end := c.spans(x+2, y, span{" ", t.Fg, false}, span{"◆", t.Acc, false}, span{" " + title + " ", t.Fg, true})
	if tag = fit(tag, x+w-3-end-3); tag != "" {
		c.textR(x+w-2, y, " "+tag+" ", t.Dim, false)
	}
}

func (m *model) drawCockpit(c *Canvas, x, y, w, h int) {
	rightW := min(60, max(46, w*31/100))
	single := w-rightW-2 < 88
	leftW := w
	if !single {
		leftW = w - rightW - 2
	}
	gy := 0
	if h >= 40 {
		gy = 1
	}

	// left column: throughput, timeline, history
	tl := min(16, 10+max(0, (h-52)/2))
	th := min(22, h-tl-7-2*gy)
	hist := true
	if th < 14 {
		hist = false
		th = min(22, h-tl-gy)
		if th < 13 {
			tl = max(6, h-13-gy)
			th = h - tl - gy
		}
	}
	m.drawThroughput(c, x, y, leftW, th)
	m.drawTimeline(c, x, y+th+gy, leftW, tl)
	if hist {
		hy := y + th + tl + 2*gy
		m.drawHistoryPanel(c, x, hy, leftW, y+h-hy)
	}
	if single {
		return
	}

	// right column: results, latency, link, events
	rx := x + leftW + 2
	ry := y
	m.drawResults(c, rx, ry, rightW, 9)
	ry += 9 + gy
	m.drawLatency(c, rx, ry, rightW, 7)
	ry += 7 + gy
	left := y + h - ry
	if left >= 7+gy+4 {
		m.drawLink(c, rx, ry, rightW, 7)
		ry += 7 + gy
	} else if left >= 7 {
		m.drawLink(c, rx, ry, rightW, 7)
		return
	}
	if y+h-ry >= 4 {
		m.drawEvents(c, rx, ry, rightW, y+h-ry)
	}
}

type stepView struct {
	n, name, info, mark string
	markC, nameC, col   RGB
	p                   float64
}

func (m *model) steps() []stepView {
	t, s := m.theme(), m.snap
	idx := map[Phase]int{phasePing: 0, phaseDown: 1, phaseUp: 2, phaseDone: 3}
	cur, ok := idx[s.Phase]
	if !ok {
		cur = -1
	}
	names := []string{"LATENCY", "DOWNLOAD", "UPLOAD", "ANALYZE"}
	cols := []RGB{t.Yel, t.Cyan, t.Mag, t.Acc}
	has := []bool{s.HasPing, s.HasDown, s.HasUp, s.Fin != nil}
	out := make([]stepView, 4)
	for i := range names {
		st := "wait"
		switch {
		case s.Phase == phaseDone || has[i] || (cur >= 0 && i < cur):
			st = "done"
		case i == cur:
			st = "run"
		}
		v := stepView{n: fmt.Sprintf("%02d", i+1), name: names[i], col: cols[i], info: "—", mark: "○", markC: t.Ghost, nameC: t.Dim}
		switch st {
		case "done":
			v.p, v.mark, v.markC, v.nameC = 1, "✓", t.Grn, t.Fg
			v.info = [...]string{
				fmt.Sprintf("%.1f ms", s.Ping), fmt.Sprintf("%.1f Mbps", s.Down), fmt.Sprintf("%.1f Mbps", s.Up), "",
			}[i]
			if i == 3 && s.Fin != nil {
				v.info = "grade " + s.Fin.Grade
			}
		case "run":
			switch i {
			case 0:
				v.p = float64(len(s.Pings)) / pingProbes
			case 1:
				v.p = s.T / dlDur
			case 2:
				v.p = s.T / ulDur
			}
			v.p = math.Min(1, v.p)
			v.info = fmt.Sprintf("%d%%", int(math.Round(v.p*100)))
			v.mark, v.markC, v.nameC = m.spin(), cols[i], t.Fg
		}
		out[i] = v
	}
	return out
}

// bar draws n lit cells of ━ followed by unlit ─.
func bar(c *Canvas, x, y, w int, p float64, lit, unlit RGB) {
	n := int(math.Round(math.Max(0, math.Min(1, p)) * float64(w)))
	c.text(c.text(x, y, strings.Repeat("━", n), lit, false), y, strings.Repeat("─", w-n), unlit, false)
}

func (m *model) heroValue() (val float64, ghostAll bool, unit, label string, labelC, digitC RGB, frac, sweep float64) {
	t, s := m.theme(), m.snap
	sc := m.scale()
	sweep, unit, digitC = -1, "Mbps", m.phaseColor(s.Phase)
	switch s.Phase {
	case phasePing:
		val, unit, label, labelC = avg(s.Pings), "ms", "◆ LATENCY PROBE", t.Yel
		sweep = (1 - math.Cos(s.T*math.Pi/.6)) / 2
	case phaseDown, phaseUp:
		val, labelC = s.Disp, digitC
		label = "▼ DOWNLOAD"
		if s.Phase == phaseUp {
			label = "▲ UPLOAD"
		}
		frac = sc.frac(val)
	case phaseDone:
		val, label, labelC, digitC = s.Down, "✓ DOWNLOAD · FINAL", t.Grn, t.Cyan
		frac = sc.frac(val)
	case phaseFailed:
		ghostAll, label, labelC = true, "✗ TEST FAILED", t.Red
	default:
		ghostAll, label, labelC = true, "◇ STANDBY", t.Dim
	}
	return
}

func (m *model) scale() gaugeScale {
	return scaleFor(math.Max(maxOf(m.snap.DL), maxOf(m.snap.UL)))
}

func (m *model) drawThroughput(c *Canvas, x, y, w, h int) {
	t, s := m.theme(), m.snap
	tag := "16 streams · http"
	if m.demo {
		tag = lower(demoProfiles[m.profile].Name) + " · 16 streams"
	}
	m.panel(c, x, y, w, h, "THROUGHPUT", tag, t.Line)
	ix, iy, iw, ih := x+2, y+1, w-4, h-2

	gw := int(math.Ceil(2.16*float64(ih-1)*cellAspect/1.62 + 4))
	const seqMin, teleMin = 24, 22
	showSeq := iw >= gw+seqMin+3
	showTele := iw >= gw+seqMin+teleMin+6
	if !showSeq {
		gw = min(gw, iw)
	}
	side := 0
	var gx int
	switch {
	case showTele:
		side = min(34, (iw-gw-6)/2)
		gx = ix + (iw-gw)/2
	case showSeq:
		side = min(34, iw-gw-3)
		gx = ix + side + 3 + (iw-side-3-gw)/2
	default:
		gx = ix + (iw-gw)/2
	}

	// gauge
	val, ghostAll, unit, label, labelC, digitC, frac, sweep := m.heroValue()
	geo := newGaugeGeom(gw, ih)
	key := fmt.Sprintf("%d|%d|%.4f|%.3f", gw, ih, frac, sweep)
	if key != m.gaugeKey {
		m.gaugeKey, m.gaugeRows = key, gaugeGrid(geo, frac, sweep)
	}
	lit := m.phaseColor(s.Phase)
	if s.Phase == phaseDone {
		lit = t.Cyan
	}
	gcol := [...]RGB{t.Bg, t.Ghost, t.Dim, lit, t.Fg}
	for r, row := range m.gaugeRows {
		for col, g := range row {
			if g.cat != 0 {
				c.put(gx+col, iy+r, g.ch, gcol[g.cat], false)
			}
		}
	}
	sc := m.scale()
	for i, lab := range sc.labels {
		f := float64(i) / float64(len(sc.labels)-1)
		lx, ly := geo.labelAt(f)
		col := t.Dim
		if frac > 0 && frac >= f-1e-6 {
			col = t.Fg
		}
		c.textC(gx+lx, iy+ly, lab, col, false)
	}

	// readout inside the arc: dot-matrix digits when they fit, else one row
	cx := gx + gw/2
	str := fmt6(val)
	ghost := leadZeros(str)
	if ghostAll {
		ghost = len(str)
	}
	var labelY, statusY int
	if geo.r >= 18 {
		digitsTop := iy + int(math.Round(geo.cy/cellAspect-3.8))
		unitY := digitsTop + 5
		labelY, statusY = digitsTop-2, digitsTop+8
		if ih < 16 {
			labelY, unitY, statusY = digitsTop-1, digitsTop+4, digitsTop+6
		}
		drawLED(c, cx-ledWidth(str)/2, digitsTop, str, ghost, digitC, t.Ghost, t.Line, t.Bg)
		c.textC(cx, unitY, strings.ToUpper(unit), t.Dim, false)
	} else {
		valueY := iy + int(math.Round(geo.cy/cellAspect-1.5))
		labelY, statusY = valueY-1, valueY+2
		u := " " + strings.ToUpper(unit)
		c.spans(cx-(len(str)+len(u))/2, valueY, span{str[:ghost], t.Ghost, true}, span{str[ghost:], digitC, true}, span{u, t.Dim, false})
	}
	statusY = min(statusY, iy+ih-1)
	c.textC(cx, labelY, label, labelC, true)
	line, lineC := m.statusLine()
	c.textC(cx, statusY, line, lineC, false)

	if showSeq {
		m.drawSequence(c, ix, iy, side, ih)
	}
	if showTele {
		m.drawTelemetry(c, ix+iw-side, iy, side, ih)
	}
}

func (m *model) statusLine() (string, RGB) {
	t, s := m.theme(), m.snap
	switch s.Phase {
	case phasePing:
		return fmt.Sprintf("%s  probe %d/%d", m.spin(), len(s.Pings), pingProbes), t.Yel
	case phaseDown, phaseUp:
		return fmt.Sprintf("%s  %d streams · %s", m.spin(), streams, fmtMB(s.MB)), m.phaseColor(s.Phase)
	case phaseDone:
		return fmt.Sprintf("▲ %.1f   ◆ %.1f ms", s.Up, s.Ping), t.Fg
	case phaseFailed:
		return "✗  ⏎ to retry", t.Red
	}
	if m.discovering {
		return m.spin() + "  locating servers", t.Dim
	}
	if len(m.servers) == 0 {
		return "✗  no servers · ⏎ to retry", t.Red
	}
	if m.blink() {
		return "⏎  LAUNCH TEST", t.Acc
	}
	return "⏎  LAUNCH TEST", mix(t.Acc, t.Bg, .35)
}

func (m *model) drawSequence(c *Canvas, x, y, w, h int) {
	t := m.theme()
	gap := 1
	if h < 13 {
		gap = 0
	}
	blockH := 1 + gap + 4*2 + 3*gap
	cy := y + max(0, (h-blockH)/2)
	c.text(x, cy, "SEQUENCE", t.Dim, false)
	cy += 1 + gap
	for _, s := range m.steps() {
		c.spans(x, cy, span{s.n + " ", t.Dim, false}, span{s.name, s.nameC, true})
		c.textR(x+w, cy, s.info+" "+s.mark, s.markC, false)
		bar(c, x, cy+1, w, s.p, s.col, t.Ghost)
		cy += 2 + gap
	}
}

type kv struct {
	k, v string
}

func (m *model) telemetry() []kv {
	s := m.snap
	var cur []float64
	var lastL []float64
	switch s.Phase {
	case phaseDown, phaseDone:
		cur, lastL = s.DL, s.LDL
	case phaseUp:
		cur, lastL = s.UL, s.LUL
	}
	dash := "—"
	out := []kv{{"PEAK", dash}, {"AVERAGE", dash}, {"LOADED RTT", dash}, {"TRANSFERRED", dash}, {"STREAMS", fmt.Sprintf("%d × tcp", streams)}, {"ELAPSED", fmtT(0)}}
	if len(cur) > 0 {
		out[0].v = fmt.Sprintf("%.1f Mbps", slices.Max(cur))
		out[1].v = fmt.Sprintf("%.1f Mbps", avg(cur[len(cur)*3/10:]))
	}
	if len(lastL) > 0 && s.Phase.running() {
		out[2].v = fmt.Sprintf("%.1f ms", lastL[len(lastL)-1])
	}
	if m.run != nil {
		out[3].v = fmtMB(s.MB)
		out[5].v = fmtT(s.Total)
	}
	return out
}

func (m *model) drawTelemetry(c *Canvas, x, y, w, h int) {
	t := m.theme()
	rows := m.telemetry()
	gap := 1
	if h < 13 {
		gap = 0
	}
	blockH := 1 + gap + len(rows) + (len(rows)-1)*gap
	cy := y + max(0, (h-blockH)/2)
	c.text(x, cy, "TELEMETRY", t.Dim, false)
	cy += 1 + gap
	for _, r := range rows {
		col := t.Fg
		if r.v == "—" {
			col = t.Ghost
		}
		c.text(x, cy, r.k, t.Dim, false)
		c.textR(x+w, cy, r.v, col, false)
		cy += 1 + gap
	}
}

// yMax is the top of the timeline. It follows the samples and the last run.
func (m *model) yMax() float64 {
	peak := math.Max(maxOf(m.snap.DL), maxOf(m.snap.UL))
	if len(m.history) > 0 {
		peak = math.Max(peak, math.Max(m.history[0].Down, m.history[0].Up))
	}
	if peak == 0 {
		peak = 100
	}
	return niceMax(peak * 1.12)
}

func (m *model) drawTimeline(c *Canvas, x, y, w, h int) {
	t, s := m.theme(), m.snap
	yMax := m.yMax()
	m.panel(c, x, y, w, h, "TIMELINE", fmt.Sprintf("y %.0f Mbps", yMax), t.Line)
	ix, iy, iw, ih := x+2, y+1, w-4, h-2
	rows := ih - 1
	const lw = 5
	gx, gw := ix+lw+1, iw-lw-1
	c.textR(ix+lw, iy, fmt.Sprintf("%.0f", yMax), t.Dim, false)
	c.textR(ix+lw, iy+(rows-1)/2, fmt.Sprintf("%.0f", yMax/2), t.Dim, false)
	c.textR(ix+lw, iy+rows-1, "0", t.Dim, false)
	gc := [...]RGB{t.Bg, t.Ghost, mix(t.Cyan, t.Bg, .42), t.Cyan, mix(t.Mag, t.Bg, .42), t.Mag}
	for r, row := range areaGrid(gw, rows, s.DL, s.UL, yMax) {
		for col, g := range row {
			if g.cat != 0 {
				c.put(gx+col, iy+r, g.ch, gc[g.cat], false)
			}
		}
	}
	ly := iy + rows
	xd := gx + int(math.Round(dlDur/(dlDur+ulDur)*float64(gw)))
	c.text(gx, ly, "▼ DOWNLOAD", t.Cyan, false)
	c.textR(xd-1, ly, fmt.Sprintf("%.1fs", dlDur), t.Dim, false)
	c.put(xd, ly, '│', t.Line, false)
	c.text(xd+2, ly, "▲ UPLOAD", t.Mag, false)
	c.textR(gx+gw, ly, fmt.Sprintf("%.1fs", dlDur+ulDur), t.Dim, false)
}

func (m *model) drawHistoryPanel(c *Canvas, x, y, w, h int) {
	t := m.theme()
	m.panel(c, x, y, w, h, "HISTORY", fmt.Sprintf("%d runs · h for all", len(m.history)), t.Line)
	m.historyTable(c, x+2, y+1, w-4, h-3, m.history)
}

// historyTable draws a header row and up to rows records.
func (m *model) historyTable(c *Canvas, x, y, w, rows int, recs []Record) {
	t := m.theme()
	type col struct {
		title string
		w     int
		right bool
	}
	whenW, barW := 13, 8
	if w < 84 {
		whenW = 0
	}
	if w < 64 {
		barW = 0
	}
	downW := 8
	if barW > 0 {
		downW += barW + 1
	}
	cols := []col{{"RUN", 4, false}, {"WHEN", whenW, false}, {"SERVER", 0, false}, {"PING", 9, true}, {"DOWN", downW, false}, {"UP", 8, true}, {"GRADE", 5, true}}
	fixed, n := 0, 0
	for _, cl := range cols {
		if cl.w > 0 || cl.title == "SERVER" {
			fixed += cl.w
			n++
		}
	}
	cols[2].w = max(6, w-fixed-2*(n-1))

	cx := x
	for _, cl := range cols {
		if cl.w == 0 {
			continue
		}
		if cl.right {
			c.textR(cx+cl.w, y, cl.title, t.Dim, false)
		} else {
			c.text(cx, y, cl.title, t.Dim, false)
		}
		cx += cl.w + 2
	}
	if len(recs) == 0 {
		c.text(x, y+1, "no runs yet · press ⏎ to run the first test", t.Ghost, false)
		return
	}
	top := 0.0
	for _, r := range recs {
		top = math.Max(top, r.Down)
	}
	for i, r := range recs {
		if i >= rows {
			break
		}
		ry := y + 1 + i
		if r.fresh {
			c.tint(x-1, ry, w+2, 1, t.Sel)
		}
		cx := x
		for _, cl := range cols {
			if cl.w == 0 {
				continue
			}
			switch cl.title {
			case "RUN":
				c.text(cx, ry, fmt.Sprintf("#%02d", r.ID), t.Dim, false)
			case "WHEN":
				c.text(cx, ry, when(r.Time, m.now), t.Fg, false)
			case "SERVER":
				c.text(cx, ry, fit(r.Server, cl.w), t.Fg, false)
			case "PING":
				c.spans(cx+cl.w-runeLen(fmt.Sprintf("%.1f", r.Ping))-3, ry, span{fmt.Sprintf("%.1f", r.Ping), t.Fg, false}, span{" ms", t.Dim, false})
			case "DOWN":
				dx := cx
				if barW > 0 {
					k := int(math.Round(r.Down / math.Max(top, 1) * float64(barW)))
					dx = c.text(dx, ry, strings.Repeat("▰", k)+strings.Repeat("▱", barW-k), t.Cyan, false) + 1
				}
				c.text(dx, ry, fmt.Sprintf("%.1f", r.Down), t.Fg, false)
			case "UP":
				c.textR(cx+cl.w, ry, fmt.Sprintf("%.1f", r.Up), t.Fg, false)
			case "GRADE":
				c.textR(cx+cl.w, ry, r.Grade, m.gradeColor(r.Grade), true)
			}
			cx += cl.w + 2
		}
	}
}

// spark renders samples as block characters over n cells. total is the
// sample count of a full phase, so a partial phase fills part of the line.
func spark(arr []float64, total, n int) (string, string) {
	if len(arr) == 0 {
		return "", strings.Repeat("▁", n)
	}
	mx := slices.Max(arr) * 1.04
	if mx <= 0 {
		mx = 1
	}
	sp := []rune(sparkChars)
	var out []rune
	for i := 0; i < n; i++ {
		a := i * total / n
		b := max(a+1, (i+1)*total/n)
		if a >= len(arr) {
			break
		}
		v := avg(arr[a:min(b, len(arr))])
		out = append(out, sp[max(0, min(7, int(math.Round(v/mx*7))))])
	}
	return string(out), strings.Repeat("▁", n-len(out))
}

func (m *model) resultDown() string {
	s := m.snap
	switch {
	case s.HasDown:
		return fmt.Sprintf("%.1f", s.Down)
	case s.Phase == phaseDown:
		return fmt.Sprintf("%.1f", s.Disp)
	}
	return "—"
}

func (m *model) resultUp() string {
	s := m.snap
	switch {
	case s.HasUp:
		return fmt.Sprintf("%.1f", s.Up)
	case s.Phase == phaseUp:
		return fmt.Sprintf("%.1f", s.Disp)
	}
	return "—"
}

func (m *model) idlePing() (float64, bool) {
	s := m.snap
	if s.HasPing {
		return s.Ping, true
	}
	if len(s.Pings) > 0 {
		return avg(s.Pings), true
	}
	return 0, false
}

func (m *model) drawResults(c *Canvas, x, y, w, h int) {
	t, s := m.theme(), m.snap
	tag := "awaiting run"
	switch {
	case s.Phase == phaseDone && len(m.history) > 0:
		tag = fmt.Sprintf("run #%02d", m.history[0].ID)
	case s.Phase.running():
		tag = "live"
	}
	m.panel(c, x, y, w, h, "RESULTS", tag, t.Line)
	ix, iy, iw := x+2, y+1, w-4

	value := func(row int, label string, col RGB, v string) {
		c.text(ix, row, label, col, true)
		c.spans(ix+iw-runeLen(v)-5, row, span{v, t.Fg, true}, span{" Mbps", t.Dim, false})
	}
	sparkRow := func(row int, arr []float64, total int, col RGB) {
		lit, rest := spark(arr, total, iw)
		c.text(c.text(ix, row, lit, col, false), row, rest, t.Ghost, false)
	}
	value(iy, "▼ DOWNLOAD", t.Cyan, m.resultDown())
	sparkRow(iy+1, s.DL, int(dlDur/sampleStep), t.Cyan)
	value(iy+2, "▲ UPLOAD", t.Mag, m.resultUp())
	sparkRow(iy+3, s.UL, int(ulDur/sampleStep), t.Mag)
	c.hline(ix, iy+4, iw, '╌', t.Line)

	dash := "—"
	ping, jit, loss, data := dash, dash, dash, dash
	if p, ok := m.idlePing(); ok {
		ping = fmt.Sprintf("%.1f", p)
	}
	if s.HasPing {
		jit = fmt.Sprintf("%.1f", s.Jitter)
	}
	if s.Fin != nil {
		loss = fmt.Sprintf("%.1f", s.Fin.Loss)
	}
	if m.run != nil {
		data = fmtMB(s.MB)
	}
	cw := iw / 4
	stats := []struct {
		k    string
		kc   RGB
		v, u string
	}{{"PING", t.Yel, ping, " ms"}, {"JITTER", t.Dim, jit, " ms"}, {"LOSS", t.Dim, loss, " %"}, {"DATA", t.Dim, data, ""}}
	for i, st := range stats {
		sx := ix + i*cw
		c.text(sx, iy+5, st.k, st.kc, false)
		u := st.u
		if st.v == dash {
			u = ""
		}
		c.spans(sx, iy+6, span{st.v, t.Fg, true}, span{u, t.Dim, false})
	}
}

func (m *model) drawLatency(c *Canvas, x, y, w, h int) {
	t, s := m.theme(), m.snap
	m.panel(c, x, y, w, h, "LATENCY", "under load", t.Line)
	ix, iy, iw := x+2, y+1, w-4
	idle, hasIdle := m.idlePing()
	rows := []struct {
		k   string
		col RGB
		v   []float64
	}{{"IDLE", t.Yel, nil}, {"▼ LOADED", t.Cyan, s.LDL}, {"▲ LOADED", t.Mag, s.LUL}}
	vals := make([]float64, 3)
	has := []bool{hasIdle, len(s.LDL) > 0, len(s.LUL) > 0}
	vals[0], vals[1], vals[2] = idle, avg(s.LDL), avg(s.LUL)
	lmax := math.Max(40, math.Max(vals[0], math.Max(vals[1], vals[2]))) * 1.15
	const lw, vw = 9, 9
	bw := iw - lw - vw - 2
	for i, r := range rows {
		ry := iy + i
		c.text(ix, ry, r.k, r.col, false)
		q := 0.0
		v := "—"
		if has[i] {
			q = math.Max(1/float64(bw), vals[i]/lmax)
			v = fmt.Sprintf("%.1f ms", vals[i])
		}
		bar(c, ix+lw+1, ry, bw, q, r.col, t.Ghost)
		c.textR(ix+iw, ry, v, t.Fg, false)
	}
	c.hline(ix, iy+3, iw, '╌', t.Line)
	bloat, grade, gbg, gfg := "—", "··", t.Line, t.Dim
	if s.Fin != nil {
		bloat, grade = fmt.Sprintf("+%.1f ms", s.Fin.Bloat), s.Fin.Grade
		gbg, gfg = m.gradeColor(grade), t.Bg
	}
	c.spans(ix, iy+4, span{"bufferbloat ", t.Dim, false}, span{bloat, t.Fg, false})
	badge := " " + padR(grade, 2) + " "
	bx := ix + iw - runeLen(badge)
	c.textBg(bx, iy+4, badge, gfg, gbg, true)
	c.textR(bx-1, iy+4, "GRADE", t.Dim, false)
}

func (m *model) currentServer() (Server, bool) {
	if m.server < 0 || m.server >= len(m.servers) {
		return Server{}, false
	}
	return m.servers[m.server], true
}

func (m *model) drawLink(c *Canvas, x, y, w, h int) {
	t := m.theme()
	m.panel(c, x, y, w, h, "LINK", "s to change", t.Line)
	ix, iy, iw := x+2, y+1, w-4
	const lw = 8
	vw := iw - lw
	dash := "—"
	server, km, host := dash, "", dash
	if sv, ok := m.currentServer(); ok {
		server, km, host = sv.Sponsor+" · "+sv.City+" "+sv.CC, fmt.Sprintf("%.0f km", sv.Km), sv.Host
	}
	isp, client := dash, dash
	if m.client.IP != "" {
		isp, client = m.client.ISP, m.clientIP()+" · "+m.client.family()
	}
	rows := []kv{{"server", server}, {"host", host}, {"isp", isp}, {"client", client}, {"iface", m.iface}}
	for i, r := range rows {
		c.text(ix, iy+i, r.k, t.Dim, false)
		vwr := vw
		if i == 0 && km != "" {
			c.textR(ix+iw, iy, km, t.Dim, false)
			vwr -= runeLen(km) + 1
		}
		c.text(ix+lw, iy+i, fit(r.v, vwr), t.Fg, false)
	}
}

var tagColors = map[string]func(Theme) RGB{
	"sys":  func(t Theme) RGB { return t.Dim },
	"net":  func(t Theme) RGB { return t.Acc },
	"ok":   func(t Theme) RGB { return t.Grn },
	"warn": func(t Theme) RGB { return t.Red },
	"ping": func(t Theme) RGB { return t.Yel },
	"dl":   func(t Theme) RGB { return t.Cyan },
	"ul":   func(t Theme) RGB { return t.Mag },
}

func (m *model) drawEvents(c *Canvas, x, y, w, h int) {
	t := m.theme()
	m.panel(c, x, y, w, h, "EVENTS", "tail -f", t.Line)
	ix, iy, iw, ih := x+2, y+1, w-4, h-2
	prompt := iy + ih - 1
	c.text(ix, prompt, "❯", t.Acc, false)
	if m.blink() {
		c.put(ix+2, prompt, '█', t.Fg, false)
	}
	n := min(len(m.logs), ih-1)
	for i, l := range m.logs[len(m.logs)-n:] {
		ry := prompt - n + i
		col := t.Dim
		if f, ok := tagColors[l.tag]; ok {
			col = f(t)
		}
		cx := c.text(ix, ry, l.ts.Format("15:04:05"), t.Ghost, false)
		cx = c.text(cx+2, ry, padR(l.tag, 4), col, false)
		c.text(cx+2, ry, fit(l.msg, ix+iw-cx-2), t.Fg, false)
	}
}

// modalBox dims the screen and draws an empty box with an accent border.
func (m *model) modalBox(c *Canvas, w, h int, title, tag string) (int, int) {
	t := m.theme()
	c.fade(t.Bg, .3)
	c.hit(0, 0, c.W, c.H, "close", 0)
	w, h = min(w, c.W-4), min(h, c.H-2)
	x, y := (c.W-w)/2, (c.H-h)/2
	c.fill(x, y, w, h, t.Fg, t.Bg)
	m.panel(c, x, y, w, h, title, tag, t.Acc)
	c.hit(x, y, w, h, "noop", 0)
	return x, y
}

func (m *model) drawServers(c *Canvas) {
	t := m.theme()
	n := len(m.servers)
	w, h := 100, n+7
	x, y := m.modalBox(c, w, h, "SELECT SERVER", fmt.Sprintf("%d nearby · by distance", n))
	w = min(w, c.W-4)
	ix, iw := x+2, w-4
	const ptrW, locW, distW, rttW, curW = 2, 24, 9, 9, 2
	spW := iw - ptrW - locW - distW - rttW - curW - 10
	hy := y + 2
	cx := c.text(ix+ptrW+2, hy, padR("SPONSOR", spW), t.Dim, false)
	cx = c.text(cx+2, hy, padR("LOCATION", locW), t.Dim, false)
	cx = c.text(cx+2, hy, padL("DIST", distW), t.Dim, false)
	c.text(cx+2, hy, padL("EST RTT", rttW), t.Dim, false)
	for i, sv := range m.servers {
		ry := hy + 1 + i
		fg := t.Fg
		if i == m.server {
			fg = t.Acc
		}
		if i == m.sel {
			c.tint(ix-1, ry, iw+2, 1, t.Sel)
			c.text(ix, ry, "▶", t.Acc, false)
		}
		rtt := "—"
		if m.demo {
			rtt = fmt.Sprintf("%.1f ms", demoProfiles[m.profile].Ping+sv.Km*.011)
		} else if sv.RTT > 0 {
			rtt = fmt.Sprintf("%.1f ms", sv.RTT)
		}
		c.text(ix+ptrW+2, ry, padR(sv.Sponsor, spW), fg, true)
		if sv.ID != "" && runeLen(sv.Sponsor)+runeLen(sv.ID)+2 <= spW {
			c.textR(ix+ptrW+2+spW, ry, sv.ID, t.Dim, false)
		}
		cx := ix + ptrW + 2 + spW
		cx = c.text(cx+2, ry, padR(sv.City+", "+sv.CC, locW), fg, false)
		cx = c.text(cx+2, ry, padL(fmt.Sprintf("%.0f km", sv.Km), distW), fg, false)
		cx = c.text(cx+2, ry, padL(rtt, rttW), t.Yel, false)
		if i == m.server {
			c.text(cx+2, ry, "●", t.Grn, false)
		}
		c.hit(ix-1, ry, iw+2, 1, "server", i)
	}
	fy := hy + n + 2
	c.hline(ix, fy, iw, '╌', t.Line)
	c.spans(ix, fy+1, span{"j k ↑ ↓ move   ⏎ select   esc close", t.Dim, false})
	c.textR(ix+iw, fy+1, "● current", t.Dim, false)
}

func (m *model) drawHistory(c *Canvas) {
	t := m.theme()
	n := max(1, min(len(m.history), 12, c.H-14))
	tag := displayPath(m.histPath)
	if m.demo {
		tag = "demo · not saved"
	}
	x, y := m.modalBox(c, 124, n+11, "HISTORY", tag)
	w := min(124, c.W-4)
	ix, iw := x+2, w-4

	var downs, ups []float64
	for _, r := range m.history {
		downs, ups = append(downs, r.Down), append(ups, r.Up)
	}
	stats := []struct {
		k  string
		kc RGB
		v  string
	}{
		{"RUNS", t.Dim, fmt.Sprint(len(m.history))},
		{"AVG ▼", t.Cyan, fmt.Sprintf("%.1f", avg(downs))},
		{"AVG ▲", t.Mag, fmt.Sprintf("%.1f", avg(ups))},
		{"BEST ▼", t.Grn, fmt.Sprintf("%.1f", maxOf(downs))},
	}
	cw := iw / 4
	for i, st := range stats {
		c.text(ix+i*cw, y+2, st.k, st.kc, false)
		c.text(ix+i*cw, y+3, st.v, t.Fg, true)
	}
	c.hline(ix, y+5, iw, '╌', t.Line)
	m.historyTable(c, ix, y+6, iw, n, m.history)
	fy := y + 6 + n + 2
	c.hline(ix, fy, iw, '╌', t.Line)
	c.text(ix, fy+1, "esc close   ⏎ new run", t.Dim, false)
}

func (m *model) drawHelp(c *Canvas) {
	t := m.theme()
	binds := []kv{{"⏎ space", "run or rerun the test"}, {"esc", "abort test · close panel"}, {"s", "choose server"}, {"h", "run history"}}
	if m.demo {
		binds = append(binds, kv{"p", "cycle connection profile"})
	}
	binds = append(binds, kv{"t", "cycle theme"}, kv{"?", "this help"}, kv{"q", "quit to shell"})
	x, y := m.modalBox(c, 64, len(binds)+7, "KEYBINDS", "velo "+version)
	w := min(64, c.W-4)
	ix, iw := x+2, w-4
	for i, b := range binds {
		c.text(ix, y+2+i, b.k, t.Acc, true)
		c.text(ix+11, y+2+i, b.v, t.Fg, false)
	}
	fy := y + 2 + len(binds) + 1
	c.hline(ix, fy, iw, '╌', t.Line)
	note := "built-in palette · " + lower(t.Name)
	if m.omarchyDir != "" && m.themeIdx == 0 {
		note = "colors follow " + displayPath(m.omarchyDir)
	}
	c.text(ix, fy+1, fit(note, iw), t.Dim, false)
}
