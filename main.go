// Command velo is a terminal speed test with a cockpit dashboard. It
// measures latency, download, upload, and bufferbloat against
// speedtest.net servers.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const version = "0.1.0"

func main() {
	demo := flag.Bool("demo", false, "simulate the network instead of testing it")
	themeName := flag.String("theme", "", "start with this theme, for example \"nord\"")
	serverID := flag.String("server", "", "use the speedtest.net server with this id, as the server list shows it")
	manual := flag.Bool("no-autostart", false, "wait for ⏎ instead of starting a test")
	private := flag.Bool("private", false, "mask the client IP address, for screenshots and recordings")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: velo [flags]\n\n")
		flag.PrintDefaults()
		names := make([]string, len(builtinThemes))
		for i, t := range builtinThemes {
			names[i] = lower(strings.ReplaceAll(t.Name, " ", "-"))
		}
		fmt.Fprintf(os.Stderr, "\nthemes: omarchy, %s\n", strings.Join(names, ", "))
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("velo", version)
		return
	}

	m := &model{
		demo:      *demo,
		private:   *private,
		autostart: !*manual,
		serverID:  *serverID,
		histPath:  historyPath(),
		events:    make(chan logEvent, 256),
		iface:     ifaceLabel(defaultIface()),
	}

	m.themes = builtinThemes
	if t, dir, ok := loadOmarchyTheme(); ok {
		m.themes = append([]Theme{t}, builtinThemes...)
		m.omarchyDir = dir
	}
	if *themeName != "" && *themeName != "omarchy" {
		i := findTheme(m.themes, *themeName)
		if i < 0 {
			fmt.Fprintf(os.Stderr, "velo: unknown theme %q\n", *themeName)
			os.Exit(2)
		}
		m.themeIdx = i
	}

	if m.demo {
		m.servers = demoServers
		m.history = demoHistory()
		m.client = Client{IP: "203.0.113.9", ISP: demoProfiles[0].ISP}
		m.iface = demoProfiles[0].Iface
	} else {
		h, err := loadHistory(m.histPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "velo: %v\n", err)
		}
		m.history = h
	}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "velo: %v\n", err)
		os.Exit(1)
	}
	if m.quitLine != "" {
		fmt.Println(m.quitLine)
	}
}
