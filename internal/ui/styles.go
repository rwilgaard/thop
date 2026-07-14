package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
)

const leftPad = " "

// styles bundles the configured glyphs and lipgloss styles used for rendering.
// Built once in newModel — no package-level render state, so any model
// (including test-constructed ones) renders correctly.
type styles struct {
	icons       config.Icons
	activeLabel string // icons.Active + " open "

	sep            lipgloss.Style
	prompt         lipgloss.Style
	selected       lipgloss.Style
	selectedActive lipgloss.Style
	statusPill     lipgloss.Style
	filterActive   lipgloss.Style
	dimActive      lipgloss.Style
	tmpName        lipgloss.Style
	match          lipgloss.Style
	helpKey        lipgloss.Style
	helpDesc       lipgloss.Style
}

func newStyles(cfg config.Config) styles {
	// A bare config.Config{} (tests, direct construction) has empty icons and
	// colors; backfill defaults so nothing renders blank. Load already does
	// this for the real path, making it a no-op there.
	c := cfg.Colors.OrDefaults()
	st := styles{icons: cfg.Icons.OrDefaults()}
	st.activeLabel = st.icons.Active + " open "
	st.sep = lipgloss.NewStyle().Faint(true)
	st.prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(c.PromptColor)).Bold(true)
	st.selected = lipgloss.NewStyle().Background(lipgloss.Color(c.SelectionBg)).Foreground(lipgloss.Color(c.SelectionFg)).Bold(true)
	st.selectedActive = lipgloss.NewStyle().Background(lipgloss.Color(c.SelectionBg)).Foreground(lipgloss.Color(c.ActiveColor)).Bold(true)
	st.statusPill = lipgloss.NewStyle().Background(lipgloss.Color(c.StatusActiveColor)).Foreground(lipgloss.Black).Bold(true)
	st.filterActive = lipgloss.NewStyle().Foreground(lipgloss.Color(c.SelectionFg)).Bold(true)
	st.dimActive = lipgloss.NewStyle().Foreground(lipgloss.Color(c.ActiveColor))
	st.tmpName = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	match := c.MatchColor
	if match == "" {
		match = c.PromptColor
	}
	st.match = lipgloss.NewStyle().Foreground(lipgloss.Color(match)).Bold(true)

	st.helpKey = lipgloss.NewStyle().Bold(true)
	if c.HelpKeyColor != "" {
		st.helpKey = st.helpKey.Foreground(lipgloss.Color(c.HelpKeyColor))
	}
	st.helpDesc = lipgloss.NewStyle().Faint(true)
	if c.HelpDescColor != "" {
		st.helpDesc = st.helpDesc.Foreground(lipgloss.Color(c.HelpDescColor))
	}
	return st
}

// iconFor returns the type glyph and its fixed per-type color for a candidate.
func iconFor(c candidates.Candidate, ic config.Icons) (string, color.Color) {
	switch {
	case c.IsTmp:
		return ic.Tmp, lipgloss.Magenta
	case c.IsRepo:
		return ic.Repo, lipgloss.Green
	default:
		return ic.Project, lipgloss.Blue
	}
}

// bracketKey wraps a key name in the "<key>" hint form used across the status
// and search rows. Single source of the bracket convention (see spelledKey).
func bracketKey(s string) string { return "<" + s + ">" }

func (st styles) keyHints(pairs [][2]string) string {
	var parts []string
	for _, p := range pairs {
		key := st.prompt.Render(bracketKey(p[0]))
		action := st.sep.Render(p[1])
		parts = append(parts, key+" "+action)
	}
	return strings.Join(parts, "  ")
}

func inputRow(label, mid, hints string, width int) string {
	pad := max(1, width-2-lipgloss.Width(label)-lipgloss.Width(mid)-lipgloss.Width(hints))
	return leftPad + label + mid + strings.Repeat(" ", pad) + hints
}
