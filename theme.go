package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RGB is a 24-bit color.
type RGB struct{ R, G, B uint8 }

// hexRGB parses "#rrggbb". It returns ok=false for any other form.
func hexRGB(s string) (RGB, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return RGB{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return RGB{}, false
	}
	return RGB{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
}

func mustHex(s string) RGB {
	c, _ := hexRGB(s)
	return c
}

// mix returns t parts of a and 1-t parts of b, like CSS color-mix in srgb.
func mix(a, b RGB, t float64) RGB {
	f := func(x, y uint8) uint8 { return uint8(float64(x)*t + float64(y)*(1-t) + 0.5) }
	return RGB{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B)}
}

// Theme holds the named colors of the design. Ghost is derived.
type Theme struct {
	Name                                                  string
	Bg, Line, Sel, Fg, Dim, Acc, Cyan, Mag, Yel, Grn, Red RGB
	Ghost                                                 RGB
}

func newTheme(name, bg, line, sel, fg, dim, acc, cyan, mag, yel, grn, red string) Theme {
	t := Theme{
		Name: name, Bg: mustHex(bg), Line: mustHex(line), Sel: mustHex(sel), Fg: mustHex(fg),
		Dim: mustHex(dim), Acc: mustHex(acc), Cyan: mustHex(cyan), Mag: mustHex(mag),
		Yel: mustHex(yel), Grn: mustHex(grn), Red: mustHex(red),
	}
	t.Ghost = mix(t.Dim, t.Bg, .5)
	return t
}

var builtinThemes = []Theme{
	newTheme("Tokyo Night", "#1a1b26", "#292e42", "#283457", "#c0caf5", "#565f89", "#7aa2f7", "#7dcfff", "#bb9af7", "#e0af68", "#9ece6a", "#f7768e"),
	newTheme("Catppuccin", "#1e1e2e", "#313244", "#45475a", "#cdd6f4", "#6c7086", "#89b4fa", "#89dceb", "#cba6f7", "#f9e2af", "#a6e3a1", "#f38ba8"),
	newTheme("Gruvbox", "#282828", "#3c3836", "#504945", "#ebdbb2", "#928374", "#fe8019", "#8ec07c", "#d3869b", "#fabd2f", "#b8bb26", "#fb4934"),
	newTheme("Nord", "#2e3440", "#3b4252", "#434c5e", "#d8dee9", "#6d7a96", "#88c0d0", "#8fbcbb", "#b48ead", "#ebcb8b", "#a3be8c", "#bf616a"),
	newTheme("Everforest", "#2d353b", "#3d484d", "#475258", "#d3c6aa", "#859289", "#a7c080", "#83c092", "#d699b6", "#dbbc7f", "#a7c080", "#e67e80"),
	newTheme("Kanagawa", "#1f1f28", "#2a2a37", "#2d4f67", "#dcd7ba", "#727169", "#7e9cd8", "#7fb4ca", "#957fb8", "#e6c384", "#98bb6c", "#e46876"),
	newTheme("Rosé Pine", "#191724", "#26233a", "#403d52", "#e0def4", "#6e6a86", "#ebbcba", "#9ccfd8", "#c4a7e7", "#f6c177", "#9ccfd8", "#eb6f92"),
	newTheme("Matte Black", "#121212", "#262626", "#333333", "#bebebe", "#6b6b6b", "#e0a64e", "#e8e8e8", "#e0a64e", "#d9b77e", "#a8c08a", "#d06a6a"),
}

// omarchyThemeDirs lists the places where Omarchy keeps the active theme.
func omarchyThemeDirs() []string {
	home, _ := os.UserHomeDir()
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	return []string{
		filepath.Join(state, "omarchy", "current", "theme"),
		filepath.Join(home, ".config", "omarchy", "current", "theme"),
	}
}

// loadOmarchyTheme reads colors.toml of the active Omarchy theme. It knows
// both the named-key form (accent, foreground, cyan, ...) and the
// color0..color15 form.
func loadOmarchyTheme() (Theme, string, bool) {
	for _, dir := range omarchyThemeDirs() {
		kv, err := readFlatTOML(filepath.Join(dir, "colors.toml"))
		if err != nil {
			continue
		}
		name := "Omarchy"
		if n := omarchyThemeName(dir); n != name {
			name += " · " + n
		}
		if t, ok := themeFromColors(name, kv); ok {
			return t, dir, true
		}
	}
	return Theme{}, "", false
}

func omarchyThemeName(dir string) string {
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "theme.name")); err == nil {
		if n := strings.TrimSpace(string(b)); n != "" {
			return prettyName(n)
		}
	}
	if target, err := filepath.EvalSymlinks(dir); err == nil && filepath.Base(target) != "theme" {
		return prettyName(filepath.Base(target))
	}
	return "Omarchy"
}

func prettyName(slug string) string {
	words := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func themeFromColors(name string, kv map[string]string) (Theme, bool) {
	// pick returns the first key that holds a color, else the fallback.
	pick := func(fallback RGB, keys ...string) RGB {
		for _, k := range keys {
			if c, ok := hexRGB(kv[k]); ok {
				return c
			}
		}
		return fallback
	}
	bg, ok1 := hexRGB(kv["background"])
	fg, ok2 := hexRGB(kv["foreground"])
	if !ok1 || !ok2 {
		return Theme{}, false
	}
	t := Theme{Name: name, Bg: bg, Fg: fg, Line: mix(fg, bg, .13)}
	t.Acc = pick(mix(fg, bg, .7), "accent", "blue", "color4")
	t.Dim = pick(mix(fg, bg, .45), "dark_foreground", "color8", "muted")
	t.Sel = pick(mix(t.Acc, bg, .25), "selection", "selection_background")
	if t.Sel == bg {
		t.Sel = mix(t.Acc, bg, .25)
	}
	t.Cyan = pick(t.Acc, "cyan", "color6")
	t.Mag = pick(t.Acc, "magenta", "color5")
	t.Yel = pick(t.Acc, "yellow", "color3")
	t.Grn = pick(t.Acc, "green", "color2")
	t.Red = pick(t.Acc, "red", "color1")
	t.Ghost = mix(t.Dim, t.Bg, .5)
	return t, true
}

// readFlatTOML reads top-level `key = "value"` pairs. Tables and arrays are
// skipped because colors.toml does not use them for colors.
func readFlatTOML(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	kv := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == '[' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		kv[strings.TrimSpace(k)] = strings.Trim(v, `"'`)
	}
	return kv, sc.Err()
}

// findTheme returns the index of the theme with this name, case-insensitive.
func findTheme(themes []Theme, name string) int {
	for i, t := range themes {
		if strings.EqualFold(t.Name, name) || strings.EqualFold(strings.ReplaceAll(t.Name, " ", "-"), name) {
			return i
		}
	}
	return -1
}
