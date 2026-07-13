package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/config"
)

const leftPad = " "

var (
	icons        config.Icons
	activeLabel  string // icons.Active + " open ", set in initStyles
	glyphPrompt  string
	glyphSelect  string
	glyphWarning string
	glyphSep     string
)

var (
	styleSep            lipgloss.Style
	stylePrompt         lipgloss.Style
	styleSelected       lipgloss.Style
	styleSelectedActive lipgloss.Style
	styleStatusPill     lipgloss.Style
	styleFilterActive   lipgloss.Style
	styleDimActive      lipgloss.Style
	styleTmpName        lipgloss.Style
	styleMatch          lipgloss.Style
	styleHelpKey        lipgloss.Style
	styleHelpDesc       lipgloss.Style
)

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func initStyles(cfg config.Config) {
	c := cfg.Colors
	// A bare config.Config{} (tests, direct construction) has empty icons;
	// backfill defaults so glyphs never render blank. Load already does this
	// for the real path, making these no-ops there.
	def := config.DefaultIcons()
	icons = config.Icons{
		Project:   orDefault(cfg.Icons.Project, def.Project),
		Repo:      orDefault(cfg.Icons.Repo, def.Repo),
		Tmp:       orDefault(cfg.Icons.Tmp, def.Tmp),
		Prompt:    orDefault(cfg.Icons.Prompt, def.Prompt),
		Active:    orDefault(cfg.Icons.Active, def.Active),
		Selected:  orDefault(cfg.Icons.Selected, def.Selected),
		Warning:   orDefault(cfg.Icons.Warning, def.Warning),
		Separator: orDefault(cfg.Icons.Separator, def.Separator),
	}
	activeLabel = icons.Active + " open "
	glyphPrompt = icons.Prompt
	glyphSelect = icons.Selected
	glyphWarning = icons.Warning
	glyphSep = icons.Separator
	styleSep = lipgloss.NewStyle().Faint(true)
	stylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color(c.PromptColor)).Bold(true)
	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color(c.SelectionBg)).Foreground(lipgloss.Color(c.SelectionFg)).Bold(true)
	styleSelectedActive = lipgloss.NewStyle().Background(lipgloss.Color(c.SelectionBg)).Foreground(lipgloss.Color(c.ActiveColor)).Bold(true)
	styleStatusPill = lipgloss.NewStyle().Background(lipgloss.Color(c.StatusActiveColor)).Foreground(lipgloss.Black).Bold(true)
	styleFilterActive = lipgloss.NewStyle().Foreground(lipgloss.Color(c.SelectionFg)).Bold(true)
	styleDimActive = lipgloss.NewStyle().Foreground(lipgloss.Color(c.ActiveColor))
	styleTmpName = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	match := c.MatchColor
	if match == "" {
		match = c.PromptColor
	}
	styleMatch = lipgloss.NewStyle().Foreground(lipgloss.Color(match)).Bold(true)

	styleHelpKey = lipgloss.NewStyle().Bold(true)
	if c.HelpKeyColor != "" {
		styleHelpKey = styleHelpKey.Foreground(lipgloss.Color(c.HelpKeyColor))
	}
	styleHelpDesc = lipgloss.NewStyle().Faint(true)
	if c.HelpDescColor != "" {
		styleHelpDesc = styleHelpDesc.Foreground(lipgloss.Color(c.HelpDescColor))
	}
}

// bracketKey wraps a key name in the "<key>" hint form used across the status
// and search rows. Single source of the bracket convention (see spelledKey).
func bracketKey(s string) string { return "<" + s + ">" }

func keyHints(pairs [][2]string) string {
	var parts []string
	for _, p := range pairs {
		key := stylePrompt.Render(bracketKey(p[0]))
		action := styleSep.Render(p[1])
		parts = append(parts, key+" "+action)
	}
	return strings.Join(parts, "  ")
}

func inputRow(label, mid, hints string, width int) string {
	pad := max(1, width-2-lipgloss.Width(label)-lipgloss.Width(mid)-lipgloss.Width(hints))
	return leftPad + label + mid + strings.Repeat(" ", pad) + hints
}
