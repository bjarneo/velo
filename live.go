package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	userAgent   = "velo/" + version
	serversURL  = "https://www.speedtest.net/api/js/servers?engine=js&https_functional=true&limit=10"
	configURL   = "https://www.speedtest.net/speedtest-config.php"
	uploadChunk = 8 << 20
)

// Server is one speedtest.net test server.
type Server struct {
	ID, Sponsor, City, Country, CC, Host string
	URL                                  string // upload endpoint
	Base                                 string // directory that holds the test files
	Km                                   float64
	RTT                                  float64 // estimated ms, 0 when not measured
}

// Client is what speedtest.net knows about this machine.
type Client struct {
	IP, ISP, Country string
}

func (c Client) family() string {
	if ip := net.ParseIP(c.IP); ip != nil && ip.To4() == nil {
		return "ipv6"
	}
	return "ipv4"
}

func get(ctx context.Context, cl *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return resp, nil
}

// fetchServers returns the nearest servers, sorted by distance.
func fetchServers(ctx context.Context) ([]Server, error) {
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := get(ctx, cl, serversURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw []struct {
		URL, Name, Country, CC, Sponsor, ID, Host string
		Distance                                  float64
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("server list: %w", err)
	}
	out := make([]Server, 0, len(raw))
	for _, s := range raw {
		if !strings.HasSuffix(s.URL, "upload.php") {
			continue
		}
		out = append(out, Server{
			ID: s.ID, Sponsor: s.Sponsor, City: s.Name, Country: s.Country, CC: s.CC, Host: s.Host,
			URL: s.URL, Base: strings.TrimSuffix(s.URL, "upload.php"), Km: s.Distance,
		})
	}
	if len(out) == 0 {
		return nil, errors.New("server list is empty")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Km < out[j].Km })
	return out, nil
}

func fetchClient(ctx context.Context) (Client, error) {
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := get(ctx, cl, configURL)
	if err != nil {
		return Client{}, err
	}
	defer resp.Body.Close()
	var cfg struct {
		Client struct {
			IP      string `xml:"ip,attr"`
			ISP     string `xml:"isp,attr"`
			Country string `xml:"country,attr"`
		} `xml:"client"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return Client{}, fmt.Errorf("client config: %w", err)
	}
	return Client{IP: cfg.Client.IP, ISP: cfg.Client.ISP, Country: cfg.Client.Country}, nil
}

func newProbeClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxConnsPerHost:     1,
		MaxIdleConnsPerHost: 1,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
	}}
}

// probe times one request for latency.txt in milliseconds.
func probe(ctx context.Context, cl *http.Client, sv Server, timeout time.Duration) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := fmt.Sprintf("%slatency.txt?x=%d", sv.Base, rand.Int64())
	t0 := time.Now()
	resp, err := get(ctx, cl, url)
	if err != nil {
		return 0, err
	}
	_, err = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if err != nil {
		return 0, err
	}
	return float64(time.Since(t0).Microseconds()) / 1000, nil
}

// estimateRTT measures each server once on a warm connection. Servers that
// do not answer keep RTT 0.
func estimateRTT(ctx context.Context, servers []Server) {
	var wg sync.WaitGroup
	for i := range servers {
		wg.Add(1)
		go func(sv *Server) {
			defer wg.Done()
			cl := newProbeClient()
			defer cl.CloseIdleConnections()
			if _, err := probe(ctx, cl, *sv, 2*time.Second); err != nil {
				return
			}
			best := 0.0
			for range 2 {
				if ms, err := probe(ctx, cl, *sv, 2*time.Second); err == nil && (best == 0 || ms < best) {
					best = ms
				}
			}
			sv.RTT = best
		}(&servers[i])
	}
	wg.Wait()
}

// bestServer returns the index of the server with the lowest RTT. It falls
// back to the nearest server when none answered.
func bestServer(servers []Server) int {
	best := 0
	for i, s := range servers {
		if s.RTT > 0 && (servers[best].RTT == 0 || s.RTT < servers[best].RTT) {
			best = i
		}
	}
	return best
}

// runLive measures latency, download, and upload against one server.
func runLive(r *Run) {
	ctx, sv := r.ctx, r.s.Server
	pc := newProbeClient()
	defer pc.CloseIdleConnections()
	if _, err := probe(ctx, pc, sv, 4*time.Second); err != nil {
		r.fail("server unreachable · %s", shortErr(err))
		return
	}
	r.log("ping", "probing latency · %d rtt probes", pingProbes)
	next := time.Now()
	for range pingProbes {
		ms, err := probe(ctx, pc, sv, 2*time.Second)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			r.probeLost()
		} else {
			r.addPing(ms)
		}
		next = next.Add(100 * time.Millisecond)
		if !sleepUntil(ctx, next) {
			return
		}
	}
	if !r.endPing() {
		r.fail("no latency replies from %s", sv.Host)
		return
	}

	for _, down := range []bool{true, false} {
		if err := transfer(r, pc, down); err != nil {
			if ctx.Err() == nil {
				r.fail("%s failed · %s", dirName(down), shortErr(err))
			}
			return
		}
	}
	r.finish(-1)
}

// transfer runs one direction with parallel streams on fresh connections
// and samples the byte counter every sampleStep. Download counts the bytes
// read. Upload counts the bytes the server acknowledged, when the system
// reports them.
func transfer(r *Run, pc *http.Client, down bool) error {
	sv, dur := r.s.Server, ulDur
	phase := phaseUp
	if down {
		dur, phase = dlDur, phaseDown
	}
	r.setPhase(phase)
	r.log(dirTag(down), "%s · opening %d streams", dirName(down), streams)

	ack := newAckTracker()
	tc := &http.Client{Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         ack.dial,
		MaxIdleConnsPerHost: streams * 2,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		ReadBufferSize:      128 << 10,
		WriteBufferSize:     128 << 10,
	}}
	defer tc.CloseIdleConnections()
	var moved atomic.Int64
	count := moved.Load
	if _, ok := ack.acked(); ok && !down {
		count = func() int64 {
			if b, ok := ack.acked(); ok {
				return b
			}
			return moved.Load()
		}
	}
	ctx, cancel := context.WithTimeout(r.ctx, time.Duration((dur+2)*float64(time.Second)))
	defer cancel()
	var wg sync.WaitGroup
	for range streams {
		wg.Go(func() {
			if down {
				downloadStream(ctx, tc, sv.Base+"random4000x4000.jpg", &moved)
			} else {
				uploadStream(ctx, tc, sv.URL, &moved)
			}
		})
	}
	wg.Go(func() { loadedProbes(ctx, r, pc, down) })

	type mark struct {
		t time.Time
		b int64
	}
	tick := time.NewTicker(time.Duration(sampleStep * float64(time.Second)))
	defer tick.Stop()
	win := []mark{{time.Now(), 0}}
	var last int64
	for n := int(dur / sampleStep); n > 0; n-- {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case now := <-tick.C:
			b := count()
			// the rate covers the last 250 ms to smooth packet bursts
			win = append(win, mark{now, b})
			if len(win) > 6 {
				win = win[1:]
			}
			rate := float64(b-win[0].b) * 8 / 1e6 / now.Sub(win[0].t).Seconds()
			r.addSample(down, rate, float64(b-last)/1e6)
			last = b
		}
	}
	cancel()
	wg.Wait()
	if r.ctx.Err() != nil {
		return r.ctx.Err()
	}
	total := count()
	if total == 0 {
		return errors.New("no data moved")
	}
	r.addMB(float64(total-last) / 1e6)
	r.endTransfer(down)
	return nil
}

func downloadStream(ctx context.Context, cl *http.Client, url string, n *atomic.Int64) {
	buf := make([]byte, 128<<10)
	for ctx.Err() == nil {
		resp, err := get(ctx, cl, fmt.Sprintf("%s?x=%d", url, rand.Int64()))
		if err != nil {
			pause(ctx, 200*time.Millisecond)
			continue
		}
		for {
			k, err := resp.Body.Read(buf)
			n.Add(int64(k))
			if err != nil {
				break
			}
		}
		resp.Body.Close()
	}
}

// uploadBlock is the payload that every upload stream repeats.
var uploadBlock = func() []byte {
	b := make([]byte, 1<<20)
	for i := range b {
		b[i] = byte(rand.Uint32())
	}
	return b
}()

// countReader serves left bytes of uploadBlock and counts what the
// transport reads.
type countReader struct {
	off, left int
	n         *atomic.Int64
}

func (c *countReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, io.EOF
	}
	k := copy(p[:min(len(p), c.left)], uploadBlock[c.off:])
	c.off = (c.off + k) % len(uploadBlock)
	c.left -= k
	c.n.Add(int64(k))
	return k, nil
}

func uploadStream(ctx context.Context, cl *http.Client, url string, n *atomic.Int64) {
	for ctx.Err() == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &countReader{left: uploadChunk, n: n})
		if err != nil {
			return
		}
		req.ContentLength = uploadChunk
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := cl.Do(req)
		if err != nil {
			pause(ctx, 200*time.Millisecond)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// loadedProbes measures latency every 250 ms while the streams fill the link.
func loadedProbes(ctx context.Context, r *Run, pc *http.Client, down bool) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		ms, err := probe(ctx, pc, r.s.Server, 3*time.Second)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			r.probeLost()
			continue
		}
		r.addLoaded(down, ms)
	}
}

func sleepUntil(ctx context.Context, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err() == nil
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func pause(ctx context.Context, d time.Duration) { sleepUntil(ctx, time.Now().Add(d)) }

// shortErr keeps the last part of a wrapped network error.
func shortErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	return s
}
