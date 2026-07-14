package ui

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
)

func TestIconFor(t *testing.T) {
	ic := config.Icons{Project: "P", Repo: "R", Tmp: "T"}
	tests := []struct {
		name      string
		c         cand.Candidate
		wantGlyph string
		wantColor color.Color
	}{
		{"project", cand.Candidate{}, "P", lipgloss.Blue},
		{"repo", cand.Candidate{IsRepo: true}, "R", lipgloss.Green},
		{"tmp", cand.Candidate{IsTmp: true}, "T", lipgloss.Magenta},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			glyph, col := iconFor(tt.c, ic)
			if glyph != tt.wantGlyph || col != tt.wantColor {
				t.Errorf("iconFor() = %q,%v want %q,%v", glyph, col, tt.wantGlyph, tt.wantColor)
			}
		})
	}
}
