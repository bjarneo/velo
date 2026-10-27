package main

import (
	"math"
	"math/rand/v2"
	"time"
)

// Profile is a simulated connection for --demo.
type Profile struct {
	Name                                string
	Down, Up, Ping, Jit, BloatD, BloatU float64
	Noise, Loss                         float64
	ISP, Iface                          string
}

var demoProfiles = []Profile{
	{"Gigabit fiber", 936, 884, 5.8, .7, 2.2, 3.4, .022, 0, "Nordlys Fiber A/S", "enp1s0 · 2.5 GbE"},
	{"Cable 500/50", 487, 47.5, 13.6, 2.4, 24, 88, .05, .1, "Kabelnet", "enp1s0 · 1 GbE"},
	{"5G home", 318, 41.2, 26.4, 7.2, 41, 66, .13, .4, "Aether Mobile", "wwan0 · 5G SA n78"},
	{"Rural DSL", 38.6, 8.9, 33.8, 3.9, 118, 254, .06, .8, "Landlinje Net", "enp1s0 · VDSL2 35b"},
}

func demoServer(sponsor, city, cc, code string, km float64) Server {
	return Server{
		Sponsor: sponsor, City: city, CC: cc, Km: km,
		Host: "speed-" + lower(code) + "." + lower(sponsor) + ".net:8080",
	}
}

var demoServers = []Server{
	demoServer("Fiberby", "Copenhagen", "DK", "CPH", 4),
	demoServer("GlobalConnect", "Copenhagen", "DK", "CPH", 7),
	demoServer("Bahnhof", "Malmö", "SE", "MMX", 29),
	demoServer("Telia", "Stockholm", "SE", "ARN", 522),
	demoServer("Leaseweb", "Amsterdam", "NL", "AMS", 622),
	demoServer("Hetzner", "Falkenstein", "DE", "FSN", 682),
	demoServer("Vultr", "London", "GB", "LHR", 955),
	demoServer("Scaleway", "Paris", "FR", "CDG", 1028),
}

// demoHistory is the seed history that --demo shows. It is never saved.
func demoHistory() []Record {
	now := time.Now()
	at := func(days int, h, m int) time.Time {
		d := now.AddDate(0, 0, -days)
		return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, d.Location())
	}
	return []Record{
		{ID: 6, Time: at(0, 9, 14), Server: "Fiberby · Copenhagen", Ping: 5.9, Down: 928.4, Up: 871.2, Grade: "A+"},
		{ID: 5, Time: at(1, 22, 41), Server: "Fiberby · Copenhagen", Ping: 7.6, Down: 611.0, Up: 802.5, Grade: "A"},
		{ID: 4, Time: at(2, 18, 3), Server: "Hetzner · Falkenstein", Ping: 13.4, Down: 884.7, Up: 845.9, Grade: "A"},
		{ID: 3, Time: at(3, 11, 27), Server: "Bahnhof · Malmö", Ping: 6.3, Down: 902.1, Up: 866.0, Grade: "A+"},
		{ID: 2, Time: at(5, 8, 55), Server: "Fiberby · Copenhagen", Ping: 6.1, Down: 415.8, Up: 398.3, Grade: "A"},
		{ID: 1, Time: at(8, 20, 12), Server: "Telia · Stockholm", Ping: 11.8, Down: 893.5, Up: 852.4, Grade: "A"},
	}
}

func gauss() float64 {
	u, v := 0.0, 0.0
	for u == 0 {
		u = rand.Float64()
	}
	for v == 0 {
		v = rand.Float64()
	}
	return math.Sqrt(-2*math.Log(u)) * math.Cos(2*math.Pi*v)
}

// runDemo simulates a test with the same timing as a live run.
func runDemo(r *Run, p Profile) {
	sv := r.s.Server
	far := math.Min(.2, sv.Km/5200)
	jiggle := func() float64 { return .965 + rand.Float64()*.05 }
	tD, tU := p.Down*(1-far)*jiggle(), p.Up*(1-far)*jiggle()
	base := p.Ping + sv.Km*.011

	r.log("ping", "probing latency · %d rtt probes", pingProbes)
	tick := time.NewTicker(time.Duration(sampleStep * float64(time.Second)))
	defer tick.Stop()
	phase, t, pings := phasePing, 0.0, 0
	var noise, dip float64
	var loaded [2]int
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-tick.C:
		}
		t += sampleStep
		if phase == phasePing {
			for want := min(pingProbes, int(t/.1+1e-6)); pings < want; pings++ {
				v := base + math.Abs(gauss())*p.Jit*.8
				if rand.Float64() < .05 {
					v += p.Jit * 2.5
				}
				r.addPing(math.Max(.4, v))
			}
			if t >= pingDur-1e-6 {
				r.endPing()
				phase, t = phaseDown, 0
				r.setPhase(phase)
				r.log("dl", "download · opening %d streams", streams)
			}
			continue
		}
		down := phase == phaseDown
		tgt, tau, dur, bloat, li := tU, 1.0, ulDur, p.BloatU, 1
		if down {
			tgt, tau, dur, bloat, li = tD, .7, dlDur, p.BloatD, 0
		}
		noise = noise*.9 + gauss()*p.Noise*.4
		if rand.Float64() < p.Noise*.1 {
			dip = .18 + rand.Float64()*.3
		}
		dip *= .88
		v := math.Max(0, tgt*(1-math.Exp(-t/tau))*(1+noise)*(1-dip))
		r.addSample(down, v, v*sampleStep/8)
		if int(t/.25+1e-6) > loaded[li] {
			loaded[li]++
			r.addLoaded(down, base+bloat*(1-math.Exp(-t/1.2))+math.Abs(gauss())*p.Jit*1.4)
		}
		if t >= dur-1e-6 {
			r.endTransfer(down)
			if !down {
				r.finish(math.Max(0, p.Loss+(rand.Float64()-.5)*p.Loss*.6))
				return
			}
			phase, t = phaseUp, 0
			r.setPhase(phase)
			r.log("ul", "upload · opening %d streams", streams)
		}
	}
}
