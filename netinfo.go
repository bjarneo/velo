package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// defaultIface returns the interface of the IPv4 default route with the
// lowest metric, read from /proc/net/route.
func defaultIface() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()
	best, bestMetric := "", -1
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fl := strings.Fields(sc.Text())
		if len(fl) < 8 || fl[1] != "00000000" || fl[7] != "00000000" {
			continue
		}
		m, _ := strconv.Atoi(fl[6])
		if bestMetric < 0 || m < bestMetric {
			best, bestMetric = fl[0], m
		}
	}
	return best
}

// ifaceLabel describes an interface, for example "enp1s0 · 2.5 GbE".
func ifaceLabel(name string) string {
	if name == "" {
		return "—"
	}
	sys := filepath.Join("/sys/class/net", name)
	if _, err := os.Stat(filepath.Join(sys, "wireless")); err == nil {
		return name + " · wifi"
	}
	b, err := os.ReadFile(filepath.Join(sys, "speed"))
	if err != nil {
		return name
	}
	mbps, err := strconv.Atoi(strings.TrimSpace(string(b)))
	switch {
	case err != nil || mbps <= 0:
		return name
	case mbps >= 1000:
		return fmt.Sprintf("%s · %s GbE", name, strconv.FormatFloat(float64(mbps)/1000, 'f', -1, 64))
	}
	return fmt.Sprintf("%s · %d Mb/s", name, mbps)
}

func lower(s string) string { return strings.ToLower(s) }
