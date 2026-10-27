package main

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"
)

const (
	sampleStep = 0.05 // seconds between throughput samples
	dlDur      = 8.0  // download phase, seconds
	ulDur      = 7.0  // upload phase, seconds
	pingDur    = 2.4  // nominal latency phase, seconds
	pingProbes = 24
	streams    = 16
)

type Phase int

const (
	phaseIdle Phase = iota
	phasePing
	phaseDown
	phaseUp
	phaseDone
	phaseFailed
)

func (p Phase) running() bool { return p == phasePing || p == phaseDown || p == phaseUp }

// Result is the outcome of a finished run.
type Result struct {
	LoadedDown, LoadedUp float64
	Bloat, Loss          float64
	Grade                string
}

// Snapshot is a copy of the run state that the UI can read without a lock.
// The sample slices only grow, so sharing their backing arrays is safe.
type Snapshot struct {
	Phase    Phase
	T, Total float64 // seconds in this phase, seconds since start

	Pings        []float64
	Ping, Jitter float64
	HasPing      bool

	DL, UL   []float64 // Mbps, one sample per sampleStep
	LDL, LUL []float64 // round-trip time under load, ms
	Disp     float64   // smoothed current throughput
	MB       float64   // data moved, megabytes

	Down, Up       float64
	PeakD, PeakU   float64
	HasDown, HasUp bool
	Fin            *Result
	Err            string
	Server         Server
}

type logEvent struct {
	ts       time.Time
	tag, msg string
}

// Run is one test. An engine goroutine writes to it.
type Run struct {
	mu         sync.Mutex
	s          Snapshot
	ctx        context.Context
	cancel     context.CancelFunc
	events     chan<- logEvent
	start      time.Time
	phaseStart time.Time
	end        time.Time
	sent, lost int
	saturated  bool
}

func newRun(sv Server, events chan<- logEvent) *Run {
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()
	return &Run{
		s:   Snapshot{Phase: phasePing, Server: sv},
		ctx: ctx, cancel: cancel, events: events,
		start: now, phaseStart: now,
	}
}

func (r *Run) snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.s
	now := time.Now()
	if !s.Phase.running() {
		now = r.end
	}
	s.T = now.Sub(r.phaseStart).Seconds()
	s.Total = now.Sub(r.start).Seconds()
	return s
}

// log sends an event to the UI. Events from an aborted run are dropped.
func (r *Run) log(tag, format string, args ...any) {
	if r.ctx.Err() != nil {
		return
	}
	select {
	case r.events <- logEvent{time.Now(), tag, fmt.Sprintf(format, args...)}:
	default:
	}
}

func (r *Run) setPhase(p Phase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.Phase = p
	r.s.Disp = 0
	r.phaseStart = time.Now()
	r.saturated = false
}

func (r *Run) addPing(ms float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.Pings = append(r.s.Pings, ms)
	r.sent++
}

func (r *Run) probeLost() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent++
	r.lost++
}

// endPing computes latency and jitter. It returns false when no probe came back.
func (r *Run) endPing() bool {
	r.mu.Lock()
	p := r.s.Pings
	if len(p) == 0 {
		r.mu.Unlock()
		return false
	}
	r.s.Ping = avg(p)
	var j float64
	for i := 1; i < len(p); i++ {
		j += math.Abs(p[i] - p[i-1])
	}
	if len(p) > 1 {
		j /= float64(len(p) - 1)
	}
	r.s.Jitter, r.s.HasPing = j, true
	ping := r.s.Ping
	r.mu.Unlock()
	r.log("ping", "latency %.1f ms · jitter %.1f ms", ping, j)
	return true
}

// addSample records one throughput sample and the data moved since the last one.
func (r *Run) addSample(down bool, mbps, mb float64) {
	r.mu.Lock()
	if down {
		r.s.DL = append(r.s.DL, mbps)
	} else {
		r.s.UL = append(r.s.UL, mbps)
	}
	r.s.Disp += (mbps - r.s.Disp) * .3
	r.s.MB += mb
	flag := !r.saturated && time.Since(r.phaseStart).Seconds() > 2.5
	if flag {
		r.saturated = true
	}
	disp := r.s.Disp
	r.mu.Unlock()
	if flag {
		r.log(dirTag(down), "%s saturated ≈ %.0f Mbps", dirName(down), disp)
	}
}

func (r *Run) addMB(mb float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.MB += mb
}

func (r *Run) addLoaded(down bool, ms float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent++
	if down {
		r.s.LDL = append(r.s.LDL, ms)
	} else {
		r.s.LUL = append(r.s.LUL, ms)
	}
}

// endTransfer sets the result of one direction: the mean of the samples
// after the first 30 %, which skips the TCP ramp.
func (r *Run) endTransfer(down bool) {
	r.mu.Lock()
	arr := r.s.UL
	if down {
		arr = r.s.DL
	}
	res, pk := avg(arr[len(arr)*3/10:]), slices.Max(arr)
	if down {
		r.s.Down, r.s.PeakD, r.s.HasDown = res, pk, true
	} else {
		r.s.Up, r.s.PeakU, r.s.HasUp = res, pk, true
	}
	r.mu.Unlock()
	r.log(dirTag(down), "%s %.1f Mbps · peak %.0f", dirName(down), res, pk)
}

// finish grades the run. loss < 0 means: use the share of lost probes.
func (r *Run) finish(loss float64) {
	r.mu.Lock()
	s := &r.s
	ld, lu := avg(s.LDL), avg(s.LUL)
	bloat := math.Max(0, math.Max(ld, lu)-s.Ping)
	if loss < 0 {
		loss = 0
		if r.sent > 0 {
			loss = float64(r.lost) / float64(r.sent) * 100
		}
	}
	s.Fin = &Result{LoadedDown: ld, LoadedUp: lu, Bloat: bloat, Loss: loss, Grade: gradeOf(bloat)}
	s.Phase = phaseDone
	r.end = time.Now()
	fin := *s.Fin
	r.mu.Unlock()
	r.log("ok", "grade %s · bufferbloat +%.1f ms · loss %.1f%%", fin.Grade, fin.Bloat, fin.Loss)
}

func (r *Run) fail(format string, args ...any) {
	if r.ctx.Err() != nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	r.mu.Lock()
	r.s.Phase = phaseFailed
	r.s.Err = msg
	r.end = time.Now()
	r.mu.Unlock()
	r.log("warn", "%s", msg)
}

func dirTag(down bool) string {
	if down {
		return "dl"
	}
	return "ul"
}

func dirName(down bool) string {
	if down {
		return "download"
	}
	return "upload"
}

func avg(a []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	var s float64
	for _, v := range a {
		s += v
	}
	return s / float64(len(a))
}

// gradeOf grades bufferbloat in ms. The thresholds follow the design.
func gradeOf(bloat float64) string {
	switch {
	case bloat < 5:
		return "A+"
	case bloat < 30:
		return "A"
	case bloat < 60:
		return "B"
	case bloat < 200:
		return "C"
	}
	return "D"
}

var gradeNote = map[string]string{
	"A+": "no queueing under load",
	"A":  "minimal queueing",
	"B":  "noticeable on calls",
	"C":  "lag under load",
	"D":  "severe bufferbloat",
}
