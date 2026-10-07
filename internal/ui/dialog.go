package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/git"
)

const dialogMaxWidth = 56

type dialog struct {
	title string
	lines []string
	hints [][2]string
}

func dialogWidth(frameW int) int {
	return min(frameW, max(16, min(dialogMaxWidth, frameW-4)))
}

// backdropMode is the mode whose frame shows behind the current dialog, or the
// current mode itself when it has no dialog.
func (m model) backdropMode() inputMode {
	mode := m.inputMode
	if mode == modeError {
		mode = m.errReturnMode
	}
	switch mode {
	case modeURLInput, modeNameInput:
		return modeNormal
	case modeCloneName:
		return modeDestPicker
	case modeConfirmClean:
		return modeCleanTmp
	case modeConfirmClose:
		return modeNormal
	default:
		return mode
	}
}

func (m model) dialog(innerW, maxLines int) (dialog, bool) {
	input := func(ti textinput.Model) string {
		return m.st.prompt.Render(m.st.icons.Prompt+" ") + ti.View()
	}
	switch m.inputMode {
	case modeURLInput:
		return dialog{
			title: "Clone repository",
			lines: []string{input(m.clone.tiURL)},
			hints: [][2]string{{"enter", "Next"}, {"esc", "Cancel"}},
		}, true
	case modeNameInput:
		lines := []string{input(m.tmp.tiName)}
		switch {
		case m.tmp.conflict && !candidates.ValidTmpName(m.tmp.tiName.Value()):
			lines = append(lines, m.st.sep.Render("Invalid name"))
		case m.tmp.conflict:
			lines = append(lines, m.st.sep.Render("Already exists — enter opens it"))
		}
		return dialog{
			title: "New tmp project",
			lines: lines,
			hints: [][2]string{{"enter", "Create"}, {"esc", "Cancel"}},
		}, true
	case modeCloneName:
		name := git.RepoNameFromURL(m.clone.tiURL.Value())
		return dialog{
			title: "Name conflict",
			lines: []string{
				m.st.sep.Render(m.st.icons.Warning + " " + name + " already exists"),
				input(m.clone.tiName),
			},
			hints: [][2]string{{"enter", "Clone as"}, {"esc", "Back"}},
		}, true
	case modeConfirmClean:
		targets := m.cleanTargets()
		open := 0
		for _, item := range targets {
			if item.active {
				open++
			}
		}
		noun := "projects"
		if len(targets) == 1 {
			noun = "project"
		}
		title := fmt.Sprintf("Delete %d tmp %s?", len(targets), noun)
		if open > 0 {
			title = fmt.Sprintf("Delete %d tmp %s (%d open)?", len(targets), noun, open)
		}
		shown := len(targets)
		if shown > maxLines {
			shown = maxLines - 1
		}
		var lines []string
		for _, item := range targets[:shown] {
			row := m.st.renderRow(listRow{item: item}, false, listOpts{width: innerW + 2, showActive: true})
			lines = append(lines, strings.TrimPrefix(row, leftPad))
		}
		if more := len(targets) - shown; more > 0 {
			lines = append(lines, m.st.sep.Render(fmt.Sprintf("… and %d more", more)))
		}
		return dialog{
			title: title,
			lines: lines,
			hints: [][2]string{{"y", "Delete"}, {"any key", "Cancel"}},
		}, true
	case modeConfirmClose:
		c := m.closeTarget.candidate
		session, window := candidates.Target(c)
		title := "Close session " + c.RelPath + "?"
		if window != "" {
			title = "Close window " + c.RelPath + "?"
		} else if n := m.windowCount(session); n > 1 {
			title = fmt.Sprintf("Close session %s (%d windows)?", c.RelPath, n)
		}
		row := m.st.renderRow(listRow{item: m.closeTarget}, false, listOpts{width: innerW + 2, showActive: true})
		return dialog{
			title: title,
			lines: []string{strings.TrimPrefix(row, leftPad)},
			hints: [][2]string{{"y", "Close"}, {"any key", "Cancel"}},
		}, true
	case modeError:
		lines := strings.Split(lipgloss.Wrap(m.errMsg, innerW, ""), "\n")
		if len(lines) > maxLines {
			lines = append(lines[:maxLines-1], "…")
		}
		return dialog{
			title: "Error",
			lines: lines,
			hints: [][2]string{{"any key", "Dismiss"}, {"ctrl-c", "Quit"}},
		}, true
	}
	return dialog{}, false
}

func (m model) windowCount(session string) int {
	n := 0
	for w := range m.ts.Windows {
		if strings.HasPrefix(w, session+"/") {
			n++
		}
	}
	return n
}

// hintLines lays the key hints out on as few lines as fit innerW.
func (st styles) hintLines(pairs [][2]string, innerW int) []string {
	var lines []string
	cur := ""
	for _, p := range pairs {
		h := st.keyHints([][2]string{p})
		switch {
		case cur == "":
			cur = h
		case lipgloss.Width(cur)+2+lipgloss.Width(h) <= innerW:
			cur += "  " + h
		default:
			lines = append(lines, cur)
			cur = h
		}
	}
	return append(lines, cur)
}

func (st styles) renderDialog(d dialog, boxW int) string {
	innerW := boxW - 4
	border := st.dialogBorder.Render
	title, _ := truncateName(d.title, nil, boxW-5)

	rows := append(append(d.lines, ""), st.hintLines(d.hints, innerW)...)
	out := make([]string, 0, len(rows)+2)
	out = append(out, border("╭─ ")+st.prompt.Render(title)+
		border(" "+strings.Repeat("─", boxW-5-lipgloss.Width(title))+"╮"))
	for _, r := range rows {
		r = clampWidth(r, innerW)
		pad := strings.Repeat(" ", innerW-lipgloss.Width(r))
		out = append(out, border("│")+" "+r+pad+" "+border("│"))
	}
	out = append(out, border("╰"+strings.Repeat("─", boxW-2)+"╯"))
	return strings.Join(out, "\n")
}

// overlay centers box over base in a width×height frame.
func overlay(base, box string, width, height int) string {
	x := max(0, (width-lipgloss.Width(box))/2)
	y := max(0, (height-lipgloss.Height(box))/2)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}

func (st styles) dim(line string) string {
	return st.sep.Render(ansi.Strip(line))
}

// sizeDialogInputs fits the dialog textinputs to the box; they scroll
// horizontally past that.
func (m *model) sizeDialogInputs() {
	w := max(1, dialogWidth(m.width)-4-lipgloss.Width(m.st.icons.Prompt)-2)
	for _, ti := range []*textinput.Model{&m.clone.tiURL, &m.clone.tiName, &m.tmp.tiName} {
		ti.SetWidth(w)
	}
}
