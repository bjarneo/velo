package main

import (
	"io"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestThemeFromNamedColors(t *testing.T) {
	th, ok := themeFromColors("Tokyo Night", map[string]string{
		"background": "#1a1b26", "foreground": "#a9b1d6", "accent": "#7aa2f7",
		"dark_foreground": "#565f89", "selection": "#292e42", "cyan": "#449dab",
		"magenta": "#ad8ee6", "yellow": "#e0af68", "green": "#9ece6a", "red": "#f7768e",
	})
	if !ok {
		t.Fatal("theme not parsed")
	}
	if th.Acc != mustHex("#7aa2f7") || th.Dim != mustHex("#565f89") || th.Cyan != mustHex("#449dab") {
		t.Errorf("wrong colors: %+v", th)
	}
}

func TestThemeFromIndexedColors(t *testing.T) {
	th, ok := themeFromColors("x", map[string]string{
		"background": "#000000", "foreground": "#ffffff", "accent": "#ff0000",
		"color1": "#110000", "color2": "#001100", "color3": "#111100",
		"color5": "#110011", "color6": "#001111", "color8": "#777777",
	})
	if !ok {
		t.Fatal("theme not parsed")
	}
	if th.Red != mustHex("#110000") || th.Cyan != mustHex("#001111") || th.Dim != mustHex("#777777") {
		t.Errorf("wrong colors: %+v", th)
	}
	if th.Sel == th.Bg {
		t.Error("selection must differ from the background")
	}
}

func TestThemeNeedsBackgroundAndForeground(t *testing.T) {
	if _, ok := themeFromColors("x", map[string]string{"accent": "#ff0000"}); ok {
		t.Error("theme without background must fail")
	}
}

func TestGradeOf(t *testing.T) {
	for bloat, want := range map[float64]string{0: "A+", 4.9: "A+", 5: "A", 29: "A", 30: "B", 60: "C", 199: "C", 200: "D"} {
		if got := gradeOf(bloat); got != want {
			t.Errorf("gradeOf(%v) = %s, want %s", bloat, got, want)
		}
	}
}

func TestReadoutFormat(t *testing.T) {
	cases := []struct {
		v     float64
		s     string
		ghost int
	}{{5.8, "0005.8", 3}, {936, "0936.0", 1}, {0, "0000.0", 3}, {12345, "9999.9", 0}}
	for _, c := range cases {
		s := fmt6(c.v)
		if s != c.s || leadZeros(s) != c.ghost {
			t.Errorf("fmt6(%v) = %s ghost %d, want %s ghost %d", c.v, s, leadZeros(s), c.s, c.ghost)
		}
	}
}

func TestGaugeScale(t *testing.T) {
	s := gaugeScales[0]
	for v, want := range map[float64]float64{0: 0, 50: .5, 1000: 1, 5000: 1, 7.5: 1.5 / 8} {
		if got := s.frac(v); math.Abs(got-want) > 1e-9 {
			t.Errorf("frac(%v) = %v, want %v", v, got, want)
		}
	}
	if scaleFor(1500).labels[8] != "2.5G" || scaleFor(900).labels[8] != "1G" {
		t.Error("scaleFor picks the wrong scale")
	}
}

func TestPack(t *testing.T) {
	g := make([]uint8, 4*4)
	g[0] = 1 // top-left dot of cell 0
	for y := 0; y < 4; y++ {
		g[y*4+2], g[y*4+3] = 2, 2 // all dots of cell 1
	}
	rows := pack(g, 4, 4)
	if rows[0][0] != (glyph{'⠁', 1}) || rows[0][1] != (glyph{'⣿', 2}) {
		t.Errorf("pack = %v", rows)
	}
}

func TestSpark(t *testing.T) {
	lit, rest := spark([]float64{1, 1, 1, 1}, 8, 8)
	if runeLen(lit) != 4 || runeLen(rest) != 4 {
		t.Errorf("spark = %q + %q", lit, rest)
	}
	if lit, rest := spark(nil, 8, 5); lit != "" || runeLen(rest) != 5 {
		t.Errorf("empty spark = %q + %q", lit, rest)
	}
}

func TestHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "velo", "history.jsonl")
	if h, err := loadHistory(path); err != nil || h != nil {
		t.Fatalf("missing file: %v %v", h, err)
	}
	for id := 1; id <= 2; id++ {
		if err := appendHistory(path, Record{ID: id, Down: float64(id), Grade: "A"}); err != nil {
			t.Fatal(err)
		}
	}
	h, err := loadHistory(path)
	if err != nil || len(h) != 2 || h[0].ID != 2 {
		t.Fatalf("history = %+v, %v", h, err)
	}
}

func TestWhen(t *testing.T) {
	now := time.Date(2026, 10, 27, 18, 0, 0, 0, time.Local)
	for in, want := range map[time.Time]string{
		now.Add(-2 * time.Hour):                           "today 16:00",
		time.Date(2026, 10, 26, 22, 41, 0, 0, time.Local): "yday 22:41",
		time.Date(2026, 10, 19, 20, 12, 0, 0, time.Local): "oct 19 20:12",
	} {
		if got := when(in, now); got != want {
			t.Errorf("when(%v) = %s, want %s", in, got, want)
		}
	}
}

func TestCountReader(t *testing.T) {
	var n atomic.Int64
	b, err := io.ReadAll(&countReader{left: 3<<20 + 7, n: &n})
	if err != nil || len(b) != 3<<20+7 || n.Load() != int64(len(b)) {
		t.Errorf("read %d bytes, counted %d, err %v", len(b), n.Load(), err)
	}
}

// TestDemoRun runs the simulation while the UI side reads snapshots, so the
// race detector sees both sides.
func TestDemoRun(t *testing.T) {
	events := make(chan logEvent, 256)
	r := newRun(demoServers[0], events)
	done := make(chan struct{})
	go func() { runDemo(r, demoProfiles[0]); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	var s Snapshot
	for time.Now().Before(deadline) {
		s = r.snapshot()
		time.Sleep(20 * time.Millisecond)
	}
	r.cancel()
	<-done
	if !s.HasPing || s.Phase != phaseDown || len(s.DL) == 0 {
		t.Errorf("after 3 s: phase %d, ping %v, %d samples", s.Phase, s.HasPing, len(s.DL))
	}
}

// finishedSnapshot is a complete run without timing.
func finishedSnapshot() Snapshot {
	s := Snapshot{Phase: phaseDone, Server: demoServers[0], HasPing: true, Ping: 5.8, Jitter: .7, MB: 1500, Total: 17.4}
	for i := 0; i < pingProbes; i++ {
		s.Pings = append(s.Pings, 5.8)
	}
	for i := 0; i < int(dlDur/sampleStep); i++ {
		s.DL = append(s.DL, 900+float64(i%7))
	}
	for i := 0; i < int(ulDur/sampleStep); i++ {
		s.UL = append(s.UL, 850+float64(i%5))
	}
	s.LDL, s.LUL = []float64{8, 9}, []float64{9, 10}
	s.Down, s.Up, s.HasDown, s.HasUp = 903, 852, true, true
	s.Fin = &Result{LoadedDown: 8.5, LoadedUp: 9.5, Bloat: 3.7, Grade: "A+"}
	return s
}

// TestRenderSizes draws every phase and modal at many sizes. It fails on a
// panic, such as a slice index out of range.
func TestRenderSizes(t *testing.T) {
	snaps := []Snapshot{{}, finishedSnapshot()}
	mid := finishedSnapshot()
	mid.Phase, mid.UL, mid.HasUp, mid.Fin, mid.T = phaseUp, mid.UL[:40], false, nil, 2
	snaps = append(snaps, mid, Snapshot{Phase: phasePing, Pings: []float64{5}}, Snapshot{Phase: phaseFailed, Err: "x"})
	m := &model{
		themes: builtinThemes, demo: true, servers: demoServers, history: demoHistory(),
		now: time.Now(), iface: "eth0", events: make(chan logEvent, 1),
	}
	for i := 0; i < 50; i++ {
		m.log("sys", "line %d with a long message that does not fit in a narrow events panel", i)
	}
	for w := 40; w <= 240; w += 7 {
		for h := 12; h <= 80; h += 5 {
			for _, s := range snaps {
				for _, md := range []modalKind{modalNone, modalServers, modalHistory, modalHelp} {
					m.w, m.h, m.snap, m.modal = w, h, s, md
					m.run = &Run{}
					c := m.render()
					if c.W != w || c.H != h {
						t.Fatalf("canvas %dx%d for %dx%d", c.W, c.H, w, h)
					}
				}
			}
		}
	}
}

func TestPrivateMasksIP(t *testing.T) {
	m := &model{
		themes: builtinThemes, private: true, servers: demoServers, now: time.Now(),
		client: Client{IP: "46.15.13.36", ISP: "Example"}, w: 200, h: 54,
	}
	m.discovered(discoveredMsg{servers: slices.Clone(demoServers), client: m.client})
	c := m.render()
	var b strings.Builder
	for _, cl := range c.cells {
		b.WriteRune(cl.Ch)
	}
	if strings.Contains(b.String(), "46.15.13.36") {
		t.Error("screen shows the IP with --private")
	}
	for _, l := range m.logs {
		if strings.Contains(l.msg, "46.15.13.36") {
			t.Errorf("log shows the IP with --private: %q", l.msg)
		}
	}
	if !strings.Contains(b.String(), "•••.•••.•••.•••") {
		t.Error("masked IP is not on screen")
	}
}
