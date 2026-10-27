package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Record is one finished run in history.jsonl.
type Record struct {
	ID     int       `json:"id"`
	Time   time.Time `json:"time"`
	Server string    `json:"server"`
	Host   string    `json:"host,omitempty"`
	Ping   float64   `json:"ping_ms"`
	Jitter float64   `json:"jitter_ms"`
	Down   float64   `json:"down_mbps"`
	Up     float64   `json:"up_mbps"`
	Bloat  float64   `json:"bloat_ms"`
	Loss   float64   `json:"loss_pct"`
	Grade  string    `json:"grade"`
	MB     float64   `json:"data_mb"`

	fresh bool // the run of this session
}

func historyPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "velo", "history.jsonl")
}

// displayPath replaces the home directory with ~.
func displayPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// loadHistory reads all records, newest first. Lines that do not parse are skipped.
func loadHistory(path string) ([]Record, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.ID > 0 {
			out = append(out, r)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, sc.Err()
}

func appendHistory(path string, r Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(r)
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// when formats a run time the way the history table shows it.
func when(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "today " + t.Format("15:04")
	case t.Add(24*time.Hour).Format("2006-01-02") == now.Format("2006-01-02"):
		return "yday " + t.Format("15:04")
	}
	return strings.ToLower(t.Format("Jan 02 15:04"))
}
