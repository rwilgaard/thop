package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/atomicfile"
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

type Popup struct {
	Width  string `yaml:"width"`
	Height string `yaml:"height"`
}

type Icons struct {
	Project   string `yaml:"project"`
	Repo      string `yaml:"repo"`
	Tmp       string `yaml:"tmp"`
	Prompt    string `yaml:"prompt"`
	Active    string `yaml:"active"`
	Selected  string `yaml:"selected"`
	Warning   string `yaml:"warning"`
	Separator string `yaml:"separator"`
}

type Config struct {
	File    string   `yaml:"-"` // where the config was loaded from
	Paths   []string `yaml:"paths"`
	TmpPath string   `yaml:"tmp_path"`
	Layout  string   `yaml:"layout"`
	// CloneShorthand expands "owner/repo" clone input; "{repo}" is replaced.
	CloneShorthand string              `yaml:"clone_shorthand"`
	Popup          Popup               `yaml:"popup"`
	Keymap         map[string][]string `yaml:"keymap"`
	Colors         Colors              `yaml:"colors"`
	Icons          Icons               `yaml:"icons"`
}

const (
	defIconProject   = "󰉋"
	defIconRepo      = ""
	defIconTmp       = "~"
	defIconPrompt    = "❯"
	defIconActive    = "●"
	defIconSelected  = "✓"
	defIconWarning   = "⚠"
	defIconSeparator = "─"
)

func defaultConfig() Config {
	return Config{
		CloneShorthand: "https://github.com/{repo}.git",
		Popup: Popup{
			Width:  "60%",
			Height: "50%",
		},
		Colors: Colors{
			SelectionBg:       ansiCode(lipgloss.BrightBlack),
			SelectionFg:       ansiCode(lipgloss.BrightWhite),
			ActiveColor:       ansiCode(lipgloss.BrightYellow),
			PromptColor:       ansiCode(lipgloss.BrightYellow),
			StatusActiveColor: ansiCode(lipgloss.BrightYellow),
		},
		Icons: Icons{
			Project:   defIconProject,
			Repo:      defIconRepo,
			Tmp:       defIconTmp,
			Prompt:    defIconPrompt,
			Active:    defIconActive,
			Selected:  defIconSelected,
			Warning:   defIconWarning,
			Separator: defIconSeparator,
		},
	}
}

const exampleConfig = `# thop configuration
# https://github.com/rwilgaard/thop

paths:
  # - ~/projects
  # - ~/work

# Directory for disposable tmp projects (ctrl-t). Defaults to XDG_CACHE_HOME/thop/tmp.
# tmp_path: ~/scratch

# Search bar position: "top" (default) or "bottom" (status bar moves to top,
# best match sits next to the search bar).
# layout: "bottom"

# Typing "owner/repo" as a clone URL expands through this template.
# Set to "" to turn that off.
# clone_shorthand: "https://github.com/{repo}.git"   # or "git@github.com:{repo}.git"

# Popup size when thop re-execs itself inside a tmux popup.
# Any tmux -w/-h value works (percent or fixed rows/cols).
# popup:
#   width: "60%"
#   height: "50%"

# Override default keybindings. Omit any binding to keep its default.
# Binding a plain character (like "k") makes it untypeable in the search
# field. A key you bind is taken from the action that had it by default;
# binding one key to two actions yourself is rejected at startup.
# keymap:
#   up: ["up", "ctrl+k"]
#   down: ["down", "ctrl+j"]
#   enter: ["enter"]
#   quit: ["esc", "ctrl+c"]
#   help: ["?"]
#   clone: ["ctrl+g"]
#   newtmp: ["ctrl+t"]
#   newproject: ["ctrl+n"]
#   cleantmp: ["ctrl+x"]
#   close: ["ctrl+q"]
#   pageup: ["pgup", "ctrl+u"]
#   pagedown: ["pgdown", "ctrl+d"]
#   nextfilter: ["tab"]
#   prevfilter: ["shift+tab"]
#   all: []          # direct filter jumps, unbound by default
#   projects: []
#   repos: []
#   tmp: []
#   open: []

# Override default UI colors.
# Values can be terminal color numbers (0-255) or hex codes (#rrggbb).
# colors:
#   selection_bg: "8"      # selected item background
#   selection_fg: "15"     # selected item foreground
#   active_color: "11"     # active session indicator
#   prompt_color: "11"     # search prompt glyph
#   match_color: "11"           # fuzzy match highlight (default: prompt_color)
#   status_active_color: "11" # mode badge background (Filter/Clone/…)
#   help_key_color: ""         # help key text (default: terminal bold)
#   help_desc_color: ""        # help description text (default: terminal faint)

# Override UI glyphs. Omit any to keep its default (Nerd Font icons).
# icons:
#   project: ""    # project dir icon
#   repo: ""       # git repo icon
#   tmp: "~"       # tmp project icon
#   prompt: ">"    # search/input prompt glyph
#   active: "*"    # open-session indicator
#   selected: "x"  # multi-select check mark
#   warning: "!"   # warning glyph
#   separator: "-" # horizontal rule rune
`

func configFile(xdgConfig string) string {
	return filepath.Join(xdgConfig, "thop", "config.yaml")
}

var pathsKeyRe = regexp.MustCompile(`(?m)^paths:`)

var emptyPathsRe = regexp.MustCompile(`(?m)^paths:[ \t]*(\[[ \t]*\])?[ \t]*(#.*)?$`)

// AddPath adds path as a scan root in the config file, keeping the rest of
// the file (comments included) as it is. It expects a file with no roots yet.
func AddPath(file, path string) error {
	data, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	entry := "paths:\n  - " + strconv.Quote(path)
	content := string(data)
	if loc := emptyPathsRe.FindStringIndex(content); loc != nil {
		content = content[:loc[0]] + entry + content[loc[1]:]
	} else if pathsKeyRe.MatchString(content) {
		// a second paths key would make the file unparseable
		return fmt.Errorf("%s already has a paths entry", file)
	} else {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += entry + "\n"
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return atomicfile.Write(file, func(w io.Writer) error {
		_, err := io.WriteString(w, content)
		return err
	})
}

// Load reads config.yaml, falling back to defaults. A non-nil error means the
// file existed but could not be read or parsed — defaults are still returned,
// so callers can warn and continue.
func Load(xdgConfig, xdgCache, home string) (Config, error) {
	tmpDefault := filepath.Join(xdgCache, "thop", "tmp")
	cfg := defaultConfig()
	path := configFile(xdgConfig)
	cfg.File = path
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
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
		cfg.File = path
		cfg.TmpPath = tmpDefault
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	for i, p := range cfg.Paths {
		cfg.Paths[i] = ExpandHome(p, home)
	}
	if cfg.TmpPath == "" {
		cfg.TmpPath = tmpDefault
	} else {
		cfg.TmpPath = ExpandHome(cfg.TmpPath, home)
	}
	// yaml.Unmarshal over defaults keeps defaults for absent keys, but an
	// explicit empty scalar ("") overwrites them. Reapply every default so a
	// blank override falls back — an empty popup size fails tmux display-popup,
	// an empty color renders with no attribute. Fields whose default is itself
	// empty (match_color, help_*_color) are no-ops.
	def := defaultConfig()
	cfg.Popup.Width = orDefault(cfg.Popup.Width, def.Popup.Width)
	cfg.Popup.Height = orDefault(cfg.Popup.Height, def.Popup.Height)
	cfg.Colors = cfg.Colors.OrDefaults()
	cfg.Icons = cfg.Icons.OrDefaults()
	return cfg, nil
}

// OrDefaults returns c with every empty field replaced by its built-in
// default. Fields whose default is itself empty stay empty.
func (c Colors) OrDefaults() Colors {
	def := defaultConfig().Colors
	return Colors{
		SelectionBg:       orDefault(c.SelectionBg, def.SelectionBg),
		SelectionFg:       orDefault(c.SelectionFg, def.SelectionFg),
		ActiveColor:       orDefault(c.ActiveColor, def.ActiveColor),
		PromptColor:       orDefault(c.PromptColor, def.PromptColor),
		MatchColor:        orDefault(c.MatchColor, def.MatchColor),
		StatusActiveColor: orDefault(c.StatusActiveColor, def.StatusActiveColor),
		HelpKeyColor:      orDefault(c.HelpKeyColor, def.HelpKeyColor),
		HelpDescColor:     orDefault(c.HelpDescColor, def.HelpDescColor),
	}
}

// OrDefaults returns ic with every empty glyph replaced by its built-in default.
func (ic Icons) OrDefaults() Icons {
	def := defaultConfig().Icons
	return Icons{
		Project:   orDefault(ic.Project, def.Project),
		Repo:      orDefault(ic.Repo, def.Repo),
		Tmp:       orDefault(ic.Tmp, def.Tmp),
		Prompt:    orDefault(ic.Prompt, def.Prompt),
		Active:    orDefault(ic.Active, def.Active),
		Selected:  orDefault(ic.Selected, def.Selected),
		Warning:   orDefault(ic.Warning, def.Warning),
		Separator: orDefault(ic.Separator, def.Separator),
	}
}

// ansiCode renders a lipgloss 4-bit color constant (ansi.BasicColor, ~uint8) as
// the string form the Colors fields expect: lipgloss.BrightBlack -> "8".
func ansiCode[T ~uint8](c T) string { return fmt.Sprint(uint8(c)) }

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func ExpandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
