package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

type modalKind int

const (
	modalNone modalKind = iota
	modalServers
	modalHistory
	modalHelp
)

type model struct {
	w, h int
	now  time.Time

	themes       []Theme
	themeIdx     int
	omarchyDir   string
	omarchyStamp time.Time
	themeChecked time.Time

	demo      bool
	private   bool
	profile   int
	autostart bool
	serverID  string

	servers     []Server
	server      int
	client      Client
	iface       string
	discovering bool

	run      *Run
	snap     Snapshot
	recorded bool

	history  []Record
	histPath string

	logs   []logEvent
	events chan logEvent

	modal modalKind
	sel   int

	gaugeKey  string
	gaugeRows [][]glyph
	canvas    *Canvas
	quitLine  string
}

type tickMsg time.Time

type autostartMsg struct{}

type discoveredMsg struct {
	servers   []Server
	client    Client
	err       error
	clientErr error
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func listen(ch <-chan logEvent) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func discover() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var msg discoveredMsg
	var wg sync.WaitGroup
	wg.Go(func() { msg.client, msg.clientErr = fetchClient(ctx) })
	msg.servers, msg.err = fetchServers(ctx)
	if msg.err == nil {
		estimateRTT(ctx, msg.servers)
	}
	wg.Wait()
	return msg
}

func (m *model) log(tag, format string, args ...any) {
	m.logs = append(m.logs, logEvent{time.Now(), tag, fmt.Sprintf(format, args...)})
	if len(m.logs) > 60 {
		m.logs = m.logs[len(m.logs)-60:]
	}
}

func (m *model) Init() tea.Cmd {
	m.log("sys", "velo %s · theme %s", version, lower(m.theme().Name))
	cmds := []tea.Cmd{tick(0), listen(m.events)}
	if m.demo {
		p := demoProfiles[m.profile]
		m.log("net", "link up · %s", p.Iface)
		m.log("net", "client %s · %s", m.clientIP(), lower(p.ISP))
		m.log("net", "%d servers discovered · nearest %s (%.0f km)", len(m.servers), lower(m.servers[0].Sponsor), m.servers[0].Km)
		m.log("ok", "demo mode · results are simulated")
		if m.autostart {
			cmds = append(cmds, tea.Tick(1600*time.Millisecond, func(time.Time) tea.Msg { return autostartMsg{} }))
		}
		return tea.Batch(cmds...)
	}
	m.log("net", "link up · %s", m.iface)
	m.log("net", "locating servers · speedtest.net")
	m.discovering = true
	return tea.Batch(append(cmds, discover)...)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		m.now = time.Time(msg)
		m.followOmarchy()
		if m.run != nil {
			m.snap = m.run.snapshot()
			if m.snap.Phase == phaseDone && !m.recorded {
				m.record()
			}
		}
		if m.snap.Phase.running() {
			return m, tick(50 * time.Millisecond)
		}
		return m, tick(110 * time.Millisecond)
	case logEvent:
		m.logs = append(m.logs, msg)
		return m, listen(m.events)
	case discoveredMsg:
		m.discovered(msg)
		if msg.err == nil && m.autostart {
			return m, tea.Tick(600*time.Millisecond, func(time.Time) tea.Msg { return autostartMsg{} })
		}
	case autostartMsg:
		if m.autostart && m.run == nil && m.modal == modalNone {
			m.autostart = false
			return m, m.start()
		}
	case tea.KeyPressMsg:
		return m, m.key(msg.String())
	case tea.MouseClickMsg:
		if m.canvas == nil {
			break
		}
		mo := tea.Mouse(msg)
		if mo.Button != tea.MouseLeft {
			break
		}
		if act, ok := m.canvas.hitTest(mo.X, mo.Y); ok {
			return m, m.click(act)
		}
	}
	return m, nil
}

func (m *model) discovered(msg discoveredMsg) {
	m.discovering = false
	if msg.err != nil {
		m.log("warn", "server discovery failed · %s", shortErr(msg.err))
		m.log("sys", "press ⏎ to retry")
		return
	}
	m.servers = msg.servers
	m.server = bestServer(m.servers)
	for i, s := range m.servers {
		if m.serverID != "" && s.ID == m.serverID {
			m.server = i
		}
	}
	if msg.clientErr == nil {
		m.client = msg.client
		m.log("net", "client %s · %s", m.clientIP(), lower(m.client.ISP))
	}
	sv := m.servers[m.server]
	m.log("net", "%d servers discovered · %s %s (%.0f km · %.1f ms)", len(m.servers), lower(sv.Sponsor), lower(sv.City), sv.Km, sv.RTT)
	m.log("ok", "ready · press ⏎ to launch")
}

// clientIP returns the client address, masked when --private is set.
func (m *model) clientIP() string {
	if !m.private {
		return m.client.IP
	}
	if m.client.family() == "ipv6" {
		return "••••:••••:••••::••••"
	}
	return "•••.•••.•••.•••"
}

func (m *model) nextID() int {
	id := 0
	for _, r := range m.history {
		id = max(id, r.ID)
	}
	return id + 1
}

func (m *model) start() tea.Cmd {
	if m.discovering || m.snap.Phase.running() {
		return nil
	}
	m.autostart = false
	m.modal = modalNone
	if len(m.servers) == 0 {
		m.discovering = true
		m.log("net", "locating servers · speedtest.net")
		return discover
	}
	sv := m.servers[m.server]
	r := newRun(sv, m.events)
	m.run, m.recorded = r, false
	m.log("sys", "session #%02d → %s", m.nextID(), sv.Host)
	if m.demo {
		go runDemo(r, demoProfiles[m.profile])
	} else {
		go runLive(r)
	}
	m.snap = r.snapshot()
	return nil
}

func (m *model) stopRun() {
	if m.run != nil {
		m.run.cancel()
	}
	m.run, m.snap = nil, Snapshot{}
}

func (m *model) abort() {
	if m.modal != modalNone {
		m.modal = modalNone
		return
	}
	if m.snap.Phase.running() {
		m.stopRun()
		m.log("warn", "aborted by user")
	}
}

func (m *model) record() {
	m.recorded = true
	s := m.snap
	r2 := func(v float64) float64 { return math.Round(v*100) / 100 }
	rec := Record{
		ID: m.nextID(), Time: time.Now().Truncate(time.Second), Server: s.Server.Sponsor + " · " + s.Server.City, Host: s.Server.Host,
		Ping: r2(s.Ping), Jitter: r2(s.Jitter), Down: r2(s.Down), Up: r2(s.Up), MB: r2(s.MB),
		Bloat: r2(s.Fin.Bloat), Loss: r2(s.Fin.Loss), Grade: s.Fin.Grade,
	}
	for i := range m.history {
		m.history[i].fresh = false
	}
	if m.demo {
		m.log("sys", "demo run · not saved")
	} else if err := appendHistory(m.histPath, rec); err != nil {
		m.log("warn", "history not saved · %s", shortErr(err))
	} else {
		m.log("sys", "saved → %s", displayPath(m.histPath))
	}
	rec.fresh = true
	m.history = append([]Record{rec}, m.history...)
}

func (m *model) openServers() {
	if m.snap.Phase.running() {
		m.log("warn", "test in progress · esc to abort")
		return
	}
	if len(m.servers) == 0 {
		m.log("warn", "no servers yet")
		return
	}
	m.modal, m.sel = modalServers, m.server
}

func (m *model) pickServer(i int) {
	sv := m.servers[i]
	m.stopRun()
	m.server, m.modal = i, modalNone
	m.log("net", "server → %s %s (%.0f km)", lower(sv.Sponsor), lower(sv.City), sv.Km)
}

// followOmarchy reloads the Omarchy theme every 2 s when its colors.toml
// changes, for example after omarchy-theme-set.
func (m *model) followOmarchy() {
	if m.omarchyDir == "" || m.now.Sub(m.themeChecked) < 2*time.Second {
		return
	}
	m.themeChecked = m.now
	fi, err := os.Stat(filepath.Join(m.omarchyDir, "colors.toml"))
	if err != nil || fi.ModTime().Equal(m.omarchyStamp) {
		return
	}
	first := m.omarchyStamp.IsZero()
	m.omarchyStamp = fi.ModTime()
	if t, _, ok := loadOmarchyTheme(); ok && !first {
		m.themes[0], m.gaugeKey = t, ""
		m.log("sys", "omarchy theme → %s", lower(t.Name))
	}
}

func (m *model) cycleTheme() {
	m.themeIdx = (m.themeIdx + 1) % len(m.themes)
	m.gaugeKey = ""
	m.log("sys", "theme → %s", lower(m.theme().Name))
}

func (m *model) cycleProfile() {
	if !m.demo {
		return
	}
	if m.snap.Phase.running() {
		m.log("warn", "aborted · profile changed")
	}
	m.stopRun()
	m.profile = (m.profile + 1) % len(demoProfiles)
	m.iface = demoProfiles[m.profile].Iface
	m.client.ISP = demoProfiles[m.profile].ISP
	m.log("sys", "profile → %s", lower(demoProfiles[m.profile].Name))
}

func (m *model) quit() tea.Cmd {
	m.stopRun()
	m.quitLine = "velo: session closed"
	if len(m.history) > 0 && m.history[0].fresh {
		r := m.history[0]
		m.quitLine += fmt.Sprintf(" · last run ↓ %.1f ↑ %.1f Mbps · %.1f ms · %s", r.Down, r.Up, r.Ping, r.Grade)
	}
	return tea.Quit
}

func (m *model) key(k string) tea.Cmd {
	if k == "ctrl+c" {
		return m.quit()
	}
	n := len(m.servers)
	switch m.modal {
	case modalServers:
		switch k {
		case "j", "down":
			m.sel = (m.sel + 1) % n
		case "k", "up":
			m.sel = (m.sel + n - 1) % n
		case "enter":
			m.pickServer(m.sel)
		case "esc", "s", "q":
			m.modal = modalNone
		}
		return nil
	case modalHistory, modalHelp:
		switch k {
		case "esc", "q", "?", "h":
			m.modal = modalNone
		case "enter":
			if m.modal == modalHistory {
				return m.start()
			}
		}
		return nil
	}
	switch k {
	case "enter", "space":
		return m.start()
	case "esc":
		m.abort()
	case "s":
		m.openServers()
	case "h":
		m.modal = modalHistory
	case "t":
		m.cycleTheme()
	case "p":
		m.cycleProfile()
	case "?":
		m.modal = modalHelp
	case "q":
		return m.quit()
	}
	return nil
}

func (m *model) click(a action) tea.Cmd {
	switch {
	case a.kind == "close":
		m.modal = modalNone
	case a.kind == "server":
		m.pickServer(a.idx)
	case strings.HasPrefix(a.kind, "key:"):
		return m.key(strings.TrimPrefix(a.kind, "key:"))
	}
	return nil
}

func (m *model) View() tea.View {
	m.canvas = m.render()
	v := tea.NewView(m.canvas.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "velo"
	return v
}
