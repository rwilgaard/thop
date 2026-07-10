package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/git"
)

type listRow struct {
	item    baseItem
	matches []int
}

type listOpts struct {
	cursor     int
	maxRows    int
	width      int
	showActive bool
	selected   map[string]bool // non-nil: render ✓ prefix for selected AbsPaths
	emptyMsg   string
	reversed   bool
}

// emptyMsg returns an empty-state message: "Nothing here" if pool is empty or
// query is empty, "No matches" otherwise.
func emptyMsg(query string, pool int) string {
	if pool == 0 || query == "" {
		return "Nothing here"
	}
	return "No matches"
}

// nonRepoCount returns the count of non-repo items in the slice.
func nonRepoCount(items []baseItem) int {
	count := 0
	for _, item := range items {
		if !item.candidate.IsRepo {
			count++
		}
	}
	return count
}

// renderRows renders the visible scroll window, or emptyMsg when there are no
// rows. reversed flips the window so index start renders last (bottom layout).
func renderRows(rows []listRow, o listOpts) []string {
	if len(rows) == 0 {
		if o.emptyMsg == "" {
			return nil
		}
		return []string{leftPad + styleSep.Render(o.emptyMsg)}
	}
	start, end := scrollWindow(o.cursor, o.maxRows, len(rows))
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, renderRow(rows[i], i == o.cursor, o))
	}
	if o.reversed {
		slices.Reverse(out)
	}
	return out
}

// fillRows pads lines with blanks to exactly maxRows, anchoring content at the
// top (default) or bottom (bottom layout). Overflow keeps the leading lines.
func fillRows(lines []string, maxRows int, bottom bool) []string {
	if len(lines) > maxRows {
		lines = lines[:maxRows]
	}
	blanks := make([]string, maxRows-len(lines))
	if bottom {
		return append(blanks, lines...)
	}
	return append(lines, blanks...)
}

// renderName styles name, highlighting matched byte offsets in styleMatch runs.
// matches must be rune-start byte offsets as produced by fuzzy.Find; mid-rune offsets are ignored.
func renderName(name string, matches []int, base, match lipgloss.Style) string {
	if len(matches) == 0 {
		return base.Render(name)
	}
	set := make(map[int]bool, len(matches))
	for _, i := range matches {
		set[i] = true
	}
	var sb strings.Builder
	var run []rune
	var matched bool
	flush := func() {
		if len(run) == 0 {
			return
		}
		if matched {
			sb.WriteString(match.Render(string(run)))
		} else {
			sb.WriteString(base.Render(string(run)))
		}
		run = run[:0]
	}
	for i, r := range name {
		if set[i] != matched {
			flush()
			matched = set[i]
		}
		run = append(run, r)
	}
	flush()
	return sb.String()
}

func renderRow(row listRow, isCursor bool, o listOpts) string {
	c := row.item.candidate
	glyph, glyphColor := candidates.Icon(c)

	prefix := leftPad
	if o.selected != nil && o.selected[c.AbsPath] {
		prefix = "✓"
	}

	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(glyphColor))
	nameStyle := lipgloss.NewStyle()
	if c.IsTmp {
		nameStyle = styleTmpName
	}
	sp := " "
	if isCursor {
		bg := styleSelected.GetBackground()
		iconStyle = iconStyle.Background(bg).Bold(true)
		nameStyle = styleSelected
		prefix = styleSelected.Render(prefix)
		sp = styleSelected.Render(sp)
	}

	matchStyle := styleMatch
	if isCursor {
		matchStyle = matchStyle.Background(styleSelected.GetBackground())
	}
	name := renderName(c.RelPath, row.matches, nameStyle, matchStyle)

	showActive := o.showActive && row.item.active
	rightW := 0
	var right string
	if showActive {
		if isCursor {
			right = styleSelectedActive.Render(activeLabel)
		} else {
			right = styleDimActive.Render(activeLabel)
		}
		rightW = lipgloss.Width(activeLabel)
	}
	contentW := 1 + lipgloss.Width(glyph) + 1 + lipgloss.Width(c.RelPath)
	pad := max(1, o.width-1-contentW-rightW)
	padStr := strings.Repeat(" ", pad)
	if isCursor {
		padStr = styleSelected.Render(padStr)
	}
	return prefix + iconStyle.Render(glyph) + sp + name + padStr + right
}

// joinCols joins column blocks horizontally with the given gap, interleaving
// gap strings between every pair of columns.
func joinCols(cols []string, gap string) string {
	parts := make([]string, 0, len(cols)*2-1)
	for i, c := range cols {
		if i > 0 {
			parts = append(parts, gap)
		}
		parts = append(parts, c)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func renderHelpOverlay(width int) string {
	cols := make([]string, 0, len(helpGroups))
	for _, g := range helpGroups {
		maxKey := 0
		for _, b := range g.keys {
			maxKey = max(maxKey, lipgloss.Width(b.Help().Key))
		}
		lines := []string{styleSep.Render(g.title)}
		for _, b := range g.keys {
			h := b.Help()
			pad := strings.Repeat(" ", maxKey-lipgloss.Width(h.Key)+2)
			lines = append(lines, styleHelpKey.Render(h.Key)+pad+styleHelpDesc.Render(h.Desc))
		}
		cols = append(cols, strings.Join(lines, "\n"))
	}

	limit := max(0, width-1)
	if joined := joinCols(cols, "    "); lipgloss.Width(joined) <= limit {
		return joined
	}
	if joined := joinCols(cols, "  "); lipgloss.Width(joined) <= limit {
		return joined
	}
	return strings.Join(cols, "\n\n")
}

func (m model) searchLine(width int) string {
	switch m.inputMode {
	case modeLoading:
		return leftPad + m.spin.View() + " " + styleSep.Render(m.loadingText)
	case modeError:
		hints := keyHints([][2]string{{"any key", "Dismiss"}, {"ctrl-c", "Quit"}})
		label := styleSep.Render("⚠  ")
		return inputRow(label, strings.SplitN(m.errMsg, "\n", 2)[0], hints, width)
	case modeURLInput:
		hints := keyHints([][2]string{{"enter", "Clone"}, {"esc", "Cancel"}})
		return inputRow(stylePrompt.Render("Clone repository ❯ "), m.tiURL.View(), hints, width)
	case modeNameInput:
		hints := keyHints([][2]string{{"enter", "Create"}, {"esc", "Cancel"}})
		label := stylePrompt.Render("New tmp project ❯ ")
		tiView := m.tiName.View()
		var hint string
		switch {
		case m.nameConflict && invalidTmpName(m.tiName.Value()):
			hint = styleSep.Render(" (Invalid name)")
		case m.nameConflict:
			hint = styleSep.Render(" (Already exists — enter opens it)")
		}
		return inputRow(label, tiView+hint, hints, width)
	case modeCleanTmp:
		hints := keyHints([][2]string{{"space", "Select"}, {"enter", "Delete"}, {"esc", "Cancel"}})
		return inputRow(stylePrompt.Render("Delete tmp projects ❯ "), m.tiClean.View(), hints, width)
	case modeConfirmClean:
		n := len(m.selected)
		if n == 0 {
			n = len(m.cleanFiltered)
		}
		noun := "projects"
		if n == 1 {
			noun = "project"
		}
		yn := styleSep.Render(" [y/N]")
		return leftPad + stylePrompt.Render(fmt.Sprintf("Delete %d tmp %s?", n, noun)) + yn
	case modeDestPicker:
		hints := keyHints([][2]string{{"enter", "Select"}, {"esc", "Back"}})
		return inputRow(stylePrompt.Render("Clone › Destination ❯ "), m.tiDest.View(), hints, width)
	case modeCloneName:
		hints := keyHints([][2]string{{"enter", "Clone as"}, {"esc", "Back"}})
		return inputRow(stylePrompt.Render("Clone › Name conflict ❯ "), m.tiCloneName.View(), hints, width)
	default:
		label := stylePrompt.Render("❯ ")
		tiView := m.tiQuery.View()
		if m.showHelp {
			return leftPad + label + tiView
		}
		hints := keyHints([][2]string{{"enter", "Open"}, {"ctrl-g", "Clone"}, {"?", "Help"}})
		return inputRow(label, tiView, hints, width)
	}
}

func (m model) bodyLines(width, maxRows int) []string {
	switch {
	case m.inputMode == modeLoading:
		return nil
	case m.inputMode == modeError:
		parts := strings.SplitN(m.errMsg, "\n", 2)
		if len(parts) < 2 {
			return nil
		}
		var lines []string
		for _, line := range strings.Split(parts[1], "\n") {
			lines = append(lines, leftPad+styleSep.Render(line))
		}
		return lines
	case m.showHelp:
		var lines []string
		for _, line := range strings.Split(renderHelpOverlay(width), "\n") {
			lines = append(lines, leftPad+line)
		}
		return lines
	case m.inputMode == modeCloneName:
		conflict := filepath.Join(m.cloneDestDir, git.RepoNameFromURL(m.tiURL.Value()))
		return []string{leftPad + styleSep.Render("⚠ Already exists: "+conflict)}
	case m.inputMode == modeCleanTmp:
		rows := make([]listRow, len(m.cleanFiltered))
		for i, it := range m.cleanFiltered {
			rows[i] = listRow{item: it.base, matches: it.matches}
		}
		return renderRows(rows, listOpts{
			cursor: m.cleanCursor, maxRows: maxRows, width: width,
			selected: m.selected,
			emptyMsg: emptyMsg(m.tiClean.Value(), len(m.tmpItems())),
			reversed: m.layoutBottom,
		})
	case m.inputMode == modeConfirmClean:
		var toDelete []baseItem
		if len(m.selected) > 0 {
			for _, item := range m.tmpItems() {
				if m.selected[item.candidate.AbsPath] {
					toDelete = append(toDelete, item)
				}
			}
		} else {
			for _, it := range m.cleanFiltered {
				toDelete = append(toDelete, it.base)
			}
		}
		lines := []string{leftPad + styleSep.Render("Will delete:")}
		for _, item := range toDelete {
			lines = append(lines, renderRow(listRow{item: item}, false, listOpts{width: width}))
		}
		return lines
	case m.inputMode == modeDestPicker:
		rows := make([]listRow, len(m.destFiltered))
		for i, it := range m.destFiltered {
			rows[i] = listRow{item: it.base, matches: it.matches}
		}
		return renderRows(rows, listOpts{
			cursor: m.destCursor, maxRows: maxRows, width: width,
			emptyMsg: emptyMsg(m.tiDest.Value(), nonRepoCount(m.all)),
			reversed: m.layoutBottom,
		})
	default:
		rows := make([]listRow, len(m.filtered))
		for i, it := range m.filtered {
			rows[i] = listRow{item: it.base, matches: it.matches}
		}
		return renderRows(rows, listOpts{
			cursor: m.cursor, maxRows: maxRows, width: width,
			showActive: true,
			emptyMsg:   emptyMsg(m.tiQuery.Value(), len(m.all)),
			reversed:   m.layoutBottom,
		})
	}
}

func (m model) View() tea.View {
	if !m.ready {
		return tea.NewView("")
	}
	width := m.width
	if width == 0 {
		width = 80
	}
	height := m.height
	if height == 0 {
		height = 24
	}

	// height budget: search + top-sep + bottom-sep + status = 4
	maxRows := max(5, height-4)
	body := fillRows(m.bodyLines(width, maxRows), maxRows, m.layoutBottom)
	sepLine := leftPad + styleSep.Render(strings.Repeat("─", max(0, width-2)))

	var sb strings.Builder
	writeLine := func(l string) {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	if m.layoutBottom {
		writeLine(m.statusBar(width))
		writeLine(sepLine)
		for _, l := range body {
			writeLine(l)
		}
		writeLine(sepLine)
		// no trailing newline: would scroll the terminal, shifting the frame
		sb.WriteString(m.searchLine(width))
	} else {
		writeLine(m.searchLine(width))
		writeLine(sepLine)
		for _, l := range body {
			writeLine(l)
		}
		writeLine(sepLine)
		// no trailing newline: would scroll the terminal, shifting the frame
		sb.WriteString(m.statusBar(width))
	}
	return tea.NewView(sb.String())
}

func (m model) statusBar(width int) string {
	var left, right string
	switch m.inputMode {
	case modeDestPicker:
		left = styleSep.Render("Clone: " + m.tiURL.Value())
		right = styleSep.Render(fmt.Sprintf("%d items", len(m.destFiltered)))
	case modeCleanTmp, modeConfirmClean:
		left = styleSep.Render(fmt.Sprintf("%d selected", len(m.selected)))
		right = styleSep.Render(fmt.Sprintf("%d items", len(m.cleanFiltered)))
	case modeURLInput, modeCloneName:
		left = styleSep.Render("Clone")
	case modeNameInput:
		left = styleSep.Render("New tmp")
	case modeLoading:
		left = styleSep.Render(m.loadingText)
	case modeError:
		left = styleSep.Render("Error")
	default:
		var sb strings.Builder
		viewLabels := []struct {
			key   string
			label string
			mode  viewMode
		}{
			{"^A", "All", viewAll},
			{"^P", "Projects", viewProject},
			{"^R", "Repos", viewRepo},
			{"^T", "Tmp", viewTmp},
		}
		for i, v := range viewLabels {
			if i > 0 {
				sb.WriteString(styleSep.Render(" · "))
			}
			sb.WriteString(styleSep.Render(v.key + " "))
			if m.view == v.mode {
				sb.WriteString(styleStatusActive.Render("● " + v.label))
			} else {
				sb.WriteString(styleSep.Render(v.label))
			}
		}
		left = sb.String()
		right = styleSep.Render(fmt.Sprintf("%d items", len(m.filtered)))
	}
	pad := max(1, width-2-lipgloss.Width(left)-lipgloss.Width(right))
	// no trailing newline: would scroll the terminal, shifting the search bar off screen
	return leftPad + left + strings.Repeat(" ", pad) + right
}
