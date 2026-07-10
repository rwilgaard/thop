package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Colors struct {
	SelectionBg       string `yaml:"selection_bg"`
	SelectionFg       string `yaml:"selection_fg"`
	ActiveColor       string `yaml:"active_color"`
	PromptColor       string `yaml:"prompt_color"`
	MatchColor        string `yaml:"match_color"`
	StatusActiveColor string `yaml:"status_active_color"`
	HelpKeyColor      string `yaml:"help_key_color"`
	HelpDescColor     string `yaml:"help_desc_color"`
}

type Config struct {
	Paths   []string `yaml:"paths"`
	TmpPath string   `yaml:"tmp_path"`
	Layout  string   `yaml:"layout"`
	Colors  Colors   `yaml:"colors"`
}

func defaultConfig() Config {
	return Config{
		Colors: Colors{
			SelectionBg:       "8",
			SelectionFg:       "15",
			ActiveColor:       "11",
			PromptColor:       "11",
			StatusActiveColor: "11",
		},
	}
}

const exampleConfig = `# thop configuration
# https://github.com/rwilgaard/thop

paths:
  # - ~/projects
  # - ~/work

# Directory for disposable tmp projects (ctrl-n). Defaults to XDG_CACHE_HOME/thop/tmp.
# tmp_path: ~/scratch

# Search bar position: "top" (default) or "bottom" (status bar moves to top,
# best match sits next to the search bar).
# layout: "bottom"

# Override default UI colors.
# Values can be terminal color numbers (0-255) or hex codes (#rrggbb).
# colors:
#   selection_bg: "8"      # selected item background
#   selection_fg: "15"     # selected item foreground
#   active_color: "11"     # active session indicator
#   prompt_color: "11"     # search prompt glyph
#   match_color: "11"           # fuzzy match highlight (default: prompt_color)
#   status_active_color: "11" # active view label in status bar
#   help_key_color: ""         # help key text (default: terminal bold)
#   help_desc_color: ""        # help description text (default: terminal faint)
`

// Load reads config.yaml, falling back to defaults. A non-nil error means the
// file existed but could not be read or parsed — defaults are still returned,
// so callers can warn and continue.
func Load(xdgConfig, xdgCache, home string) (Config, error) {
	tmpDefault := filepath.Join(xdgCache, "thop", "tmp")
	cfg := defaultConfig()
	dir := filepath.Join(xdgConfig, "thop")
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(path, []byte(exampleConfig), 0o644)
		cfg.TmpPath = tmpDefault
		return cfg, nil
	}
	if err != nil {
		cfg.TmpPath = tmpDefault
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		cfg = defaultConfig()
		cfg.TmpPath = tmpDefault
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	for i, p := range cfg.Paths {
		cfg.Paths[i] = expandHome(p, home)
	}
	if cfg.TmpPath == "" {
		cfg.TmpPath = tmpDefault
	} else {
		cfg.TmpPath = expandHome(cfg.TmpPath, home)
	}
	return cfg, nil
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
