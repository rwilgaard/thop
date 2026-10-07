package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
)

func TestDialog_fitsFrame(t *testing.T) {
	longErr := strings.Repeat("fatal: could not read from remote repository ", 8)
	tests := []struct {
		name  string
		mode  inputMode
		wants []string
	}{
		{"url", modeURLInput, []string{"Clone repository", "Next", "Cancel"}},
		{"tmp name", modeNameInput, []string{"New tmp project", "Create", "Cancel"}},
		{"clone name", modeCloneName, []string{"Name conflict", "repo already exists", "Clone as", "Back"}},
		{"confirm", modeConfirmClean, []string{"Delete 1 tmp project?", "scratch", "Delete", "Cancel"}},
		{"error", modeError, []string{"Error", "fatal: could not", "Dismiss"}},
		{"cloning", modeCloning, []string{"Cloning", "→ proj/repo", "Cancel"}},
		{"close", modeConfirmClose, []string{"Close session", "scratch", "Cancel"}},
		{"help", modeHelp, []string{"Help", "Navigate", "Close"}},
		{"setup", modeSetup, []string{"Add a project root", "Save", "Skip"}},
		{"new project", modeNewProjName, []string{"New project", "Create", "Back"}},
	}
	sizes := []struct{ w, h int }{{30, 9}, {60, 16}, {70, 20}, {100, 24}}
	for _, tt := range tests {
		for _, sz := range sizes {
			t.Run(fmt.Sprintf("%s %dx%d", tt.name, sz.w, sz.h), func(t *testing.T) {
				m := testModel()
				m.all = []baseItem{{candidate: cand.Candidate{AbsPath: "/t/scratch", RelPath: "scratch", IsTmp: true}}}
				m.rebuildFiltered()
				m.rebuildCleanFiltered()
				m.clone.tiURL.SetValue("https://example.com/owner/repo.git")
				m.errMsg = longErr
				m.result.Clone = &CloneRequest{Dest: "/dest/proj/repo"}
				m.closeTarget = m.all[0]
				m = press(m, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
				m.inputMode = tt.mode

				out := m.View().Content
				lines := strings.Split(out, "\n")
				if want := max(5, sz.h-4) + 4; len(lines) != want {
					t.Errorf("frame has %d lines, want %d", len(lines), want)
				}
				for i, line := range lines {
					if w := lipgloss.Width(line); w > sz.w {
						t.Errorf("line %d is %d cells wide, frame is %d", i, w, sz.w)
					}
				}
				plain := ansi.Strip(out)
				for _, w := range tt.wants {
					if !strings.Contains(plain, w) {
						t.Errorf("missing %q in:\n%s", w, plain)
					}
				}
			})
		}
	}
}

func TestDialog_backdrop(t *testing.T) {
	tests := []struct {
		name   string
		mode   inputMode
		errRet inputMode
		want   inputMode
	}{
		{"url over picker", modeURLInput, modeNormal, modeNormal},
		{"tmp name over picker", modeNameInput, modeNormal, modeNormal},
		{"clone name over dest picker", modeCloneName, modeNormal, modeDestPicker},
		{"confirm over clean list", modeConfirmClean, modeNormal, modeCleanTmp},
		{"cloning over dest picker", modeCloning, modeNormal, modeDestPicker},
		{"help over picker", modeHelp, modeNormal, modeNormal},
		{"setup over picker", modeSetup, modeNormal, modeNormal},
		{"new project name over picker", modeNewProjName, modeNormal, modeNormal},
		{"close over picker", modeConfirmClose, modeNormal, modeNormal},
		{"error over picker", modeError, modeNormal, modeNormal},
		{"error from url input over picker", modeError, modeURLInput, modeNormal},
		{"no dialog", modeDestPicker, modeNormal, modeDestPicker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel()
			m.inputMode, m.errReturnMode = tt.mode, tt.errRet
			if got := m.backdropMode(); got != tt.want {
				t.Errorf("backdropMode = %v, want %v", got, tt.want)
			}
		})
	}

	m := newModel([]cand.Candidate{{AbsPath: "/p/visible", RelPath: "visible"}}, map[string]float64{}, tmux.State{}, false, config.Config{}, false)
	m = sized(m, 80, 24)
	m.inputMode = modeURLInput
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "visible") {
		t.Errorf("list should stay visible behind the dialog:\n%s", plain)
	}
}

func TestHelpOverlay(t *testing.T) {
	m := testModel()
	m = sized(m, 100, 24)

	if m.inputMode == modeHelp {
		t.Fatal("help should start hidden")
	}

	q := keyRune('?')
	m = press(m, q)
	if m.inputMode != modeHelp {
		t.Error("? should show help")
	}

	out := ansi.Strip(m.View().Content)
	for _, w := range []string{"Navigate", "Actions", "Filters", "Clone repository", "Move up", "Next filter", "Close session", "Page down"} {
		if !strings.Contains(out, w) {
			t.Errorf("help overlay missing %q", w)
		}
	}

	m = press(m, q)
	if m.inputMode != modeNormal {
		t.Error("second ? should hide help")
	}

	m.inputMode = modeHelp
	m = press(m, keyEsc)
	if m.inputMode != modeNormal {
		t.Error("esc should hide help")
	}
}

func TestHelpDialog_columns(t *testing.T) {
	tests := []struct {
		name    string
		w, h    int
		sameRow []string // titles that must share a line
	}{
		{"three columns", 100, 24, []string{"Navigate", "Actions", "Filters"}},
		{"two columns", 70, 20, []string{"Navigate", "Actions"}},
		{"stacked", 40, 30, []string{"Navigate"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel()
			m.width, m.height, m.ready = tt.w, tt.h, true
			m.inputMode = modeHelp
			plain := ansi.Strip(m.View().Content)
			for _, w := range []string{"Actions", "Filters", "Next filter", "Close session"} {
				if !strings.Contains(plain, w) {
					t.Errorf("missing %q in:\n%s", w, plain)
				}
			}
			for _, line := range strings.Split(plain, "\n") {
				if !strings.Contains(line, "Navigate") {
					continue
				}
				n := 0
				for _, title := range []string{"Navigate", "Actions", "Filters"} {
					if strings.Contains(line, title) {
						n++
					}
				}
				if n != len(tt.sameRow) {
					t.Errorf("title row has %d groups, want %d: %q", n, len(tt.sameRow), line)
				}
			}
		})
	}
}

func TestErrorRecovery(t *testing.T) {
	tests := []struct {
		name     string
		msg      tea.Msg
		wantMode inputMode
	}{
		{"clone fail returns to URL input", cloneDoneMsg{err: os.ErrPermission}, modeURLInput},
		{"open fail returns to picker", selectionDoneMsg{err: os.ErrPermission}, modeNormal},
		{"tmp create fail returns to name input", tmpCreatedMsg{err: os.ErrPermission}, modeNameInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel()
			m.clone.tiURL.SetValue("https://x/y.git")
			m.result.Clone = &CloneRequest{}
			m.result.Tmp = &TmpRequest{}
			m.inputMode = modeLoading

			m = press(m, tt.msg)
			if m.inputMode != modeError {
				t.Fatalf("expected modeError, got %v", m.inputMode)
			}

			// any key dismisses back to origin
			m = press(m, keyRune('x'))
			if m.inputMode != tt.wantMode {
				t.Errorf("expected recovery to %v, got %v", tt.wantMode, m.inputMode)
			}
			if m.errMsg != "" {
				t.Error("errMsg should be cleared on dismiss")
			}
			if tt.wantMode == modeURLInput && m.clone.tiURL.Value() != "https://x/y.git" {
				t.Error("URL should be preserved for retry")
			}
		})
	}

	// ctrl+c still quits
	m := testModel()
	m.inputMode = modeError
	_, cmd := m.Update(keyCtrl('c'))
	if cmd == nil {
		t.Error("ctrl+c in error mode should quit")
	}
}
