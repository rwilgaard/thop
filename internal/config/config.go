package config

import (
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
	StatusActiveColor string `yaml:"status_active_color"`
}

type Config struct {
	Paths  []string `yaml:"paths"`
	Colors Colors   `yaml:"colors"`
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

# Override default UI colors.
# Values can be terminal color numbers (0-255) or hex codes (#rrggbb).
# colors:
#   selection_bg: "8"      # selected item background
#   selection_fg: "15"     # selected item foreground
#   active_color: "11"     # active session indicator
#   prompt_color: "11"     # search prompt glyph
#   status_active_color: "11"  # active view label in status bar
`

func Load(xdgConfig, home string) Config {
	cfg := defaultConfig()
	dir := filepath.Join(xdgConfig, "thop")
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(path, []byte(exampleConfig), 0o644)
		return cfg
	}
	if err != nil {
		return cfg
	}
	_ = yaml.Unmarshal(data, &cfg)
	for i, p := range cfg.Paths {
		cfg.Paths[i] = expandHome(p, home)
	}
	return cfg
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
