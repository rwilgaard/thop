package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/config"
)

const (
	activeLabel = "● open "
	leftPad     = " "
)

var (
	styleSep            lipgloss.Style
	stylePrompt         lipgloss.Style
	styleSelected       lipgloss.Style
	styleSelectedActive lipgloss.Style
	styleStatusActive   lipgloss.Style
	styleDimActive      lipgloss.Style
	styleTmpName        lipgloss.Style
	styleMatch          lipgloss.Style
)

func initStyles(cfg config.Config) {
	c := cfg.Colors
	styleSep = lipgloss.NewStyle().Faint(true)
	stylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color(c.PromptColor)).Bold(true)
	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color(c.SelectionBg)).Foreground(lipgloss.Color(c.SelectionFg)).Bold(true)
	styleSelectedActive = lipgloss.NewStyle().Background(lipgloss.Color(c.SelectionBg)).Foreground(lipgloss.Color(c.ActiveColor)).Bold(true)
	styleStatusActive = lipgloss.NewStyle().Foreground(lipgloss.Color(c.StatusActiveColor)).Bold(true)
	styleDimActive = lipgloss.NewStyle().Foreground(lipgloss.Color(c.ActiveColor))
	styleTmpName = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	match := c.MatchColor
	if match == "" {
		match = c.PromptColor
	}
	styleMatch = lipgloss.NewStyle().Foreground(lipgloss.Color(match)).Bold(true)
}

func keyHints(pairs [][2]string) string {
	var parts []string
	for _, p := range pairs {
		key := stylePrompt.Render("<" + p[0] + ">")
		action := styleSep.Render(p[1])
		parts = append(parts, key+" "+action)
	}
	return strings.Join(parts, "  ")
}

func inputRow(label, mid, hints string, width int) string {
	pad := max(1, width-2-lipgloss.Width(label)-lipgloss.Width(mid)-lipgloss.Width(hints))
	return leftPad + label + mid + strings.Repeat(" ", pad) + hints
}
