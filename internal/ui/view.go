package ui

import (
	"fmt"
	"path/filepath"
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

// renderRows writes exactly o.maxRows lines: the visible scroll window
// padded with blanks, or emptyMsg when there are no rows.
func renderRows(sb *strings.Builder, rows []listRow, o listOpts) {
	if len(rows) == 0 {
		if o.emptyMsg != "" {
			sb.WriteString(leftPad + styleSep.Render(o.emptyMsg))
		}
		sb.WriteByte('\n')
		for range o.maxRows - 1 {
			sb.WriteByte('\n')
		}
		return
	}
	start, end := scrollWindow(o.cursor, o.maxRows, len(rows))
	for i := start; i < end; i++ {
		sb.WriteString(renderRow(rows[i], i == o.cursor, o))
		sb.WriteByte('\n')
	}
	for range o.maxRows - (end - start) {
		sb.WriteByte('\n')
	}
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

	sep := strings.Repeat("─", max(0, width-2))

	var sb strings.Builder
	var searchLine string
	switch m.inputMode {
	case modeLoading:
		searchLine = leftPad + m.spin.View() + " " + styleSep.Render(m.loadingText)
	case modeError:
		hints := keyHints([][2]string{{"any key", "Dismiss"}, {"ctrl-c", "Quit"}})
		label := styleSep.Render("⚠  ")
		searchLine = inputRow(label, strings.SplitN(m.errMsg, "\n", 2)[0], hints, width)
	case modeURLInput:
		hints := keyHints([][2]string{{"enter", "Clone"}, {"esc", "Cancel"}})
		searchLine = inputRow(stylePrompt.Render("Clone repository ❯ "), m.tiURL.View(), hints, width)
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
		searchLine = inputRow(label, tiView+hint, hints, width)
	case modeCleanTmp:
		hints := keyHints([][2]string{{"space", "Select"}, {"enter", "Delete"}, {"esc", "Cancel"}})
		searchLine = inputRow(stylePrompt.Render("Delete tmp projects ❯ "), m.tiClean.View(), hints, width)
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
		searchLine = leftPad + stylePrompt.Render(fmt.Sprintf("Delete %d tmp %s?", n, noun)) + yn
	case modeDestPicker:
		hints := keyHints([][2]string{{"enter", "Select"}, {"esc", "Back"}})
		searchLine = inputRow(stylePrompt.Render("Clone › Destination ❯ "), m.tiDest.View(), hints, width)
	case modeCloneName:
		hints := keyHints([][2]string{{"enter", "Clone as"}, {"esc", "Back"}})
		searchLine = inputRow(stylePrompt.Render("Clone › Name conflict ❯ "), m.tiCloneName.View(), hints, width)
	default:
		label := stylePrompt.Render("❯ ")
		tiView := m.tiQuery.View()
		if m.showHelp {
			searchLine = leftPad + label + tiView
		} else {
			hints := keyHints([][2]string{{"enter", "Open"}, {"ctrl-g", "Clone"}, {"?", "Help"}})
			searchLine = inputRow(label, tiView, hints, width)
		}
	}
	sb.WriteString(searchLine)
	sb.WriteByte('\n')

	sb.WriteString(leftPad)
	sb.WriteString(styleSep.Render(sep))
	sb.WriteByte('\n')

	// height budget: search + top-sep + bottom-sep + status = 4
	maxRows := max(5, height-4)

	switch {
	case m.inputMode == modeLoading:
		for range maxRows {
			sb.WriteByte('\n')
		}
	case m.inputMode == modeError:
		lines := strings.SplitN(m.errMsg, "\n", 2)
		rows := 0
		if len(lines) > 1 {
			for _, line := range strings.Split(lines[1], "\n") {
				if rows >= maxRows {
					break
				}
				sb.WriteString(leftPad + styleSep.Render(line) + "\n")
				rows++
			}
		}
		for rows < maxRows {
			sb.WriteByte('\n')
			rows++
		}
	case m.showHelp:
		rows := 0
		for _, line := range strings.Split(renderHelpOverlay(width), "\n") {
			if rows >= maxRows {
				break
			}
			sb.WriteString(leftPad + line + "\n")
			rows++
		}
		for rows < maxRows {
			sb.WriteByte('\n')
			rows++
		}
	case m.inputMode == modeCloneName:
		conflict := filepath.Join(m.cloneDestDir, git.RepoNameFromURL(m.tiURL.Value()))
		msg := styleSep.Render("⚠ Already exists: " + conflict)
		sb.WriteString(leftPad + msg + "\n")
		for range maxRows - 1 {
			sb.WriteByte('\n')
		}
	case m.inputMode == modeCleanTmp:
		rows := make([]listRow, len(m.cleanFiltered))
		for i, it := range m.cleanFiltered {
			rows[i] = listRow{item: it.base, matches: it.matches}
		}
		renderRows(&sb, rows, listOpts{
			cursor: m.cleanCursor, maxRows: maxRows, width: width,
			selected: m.selected,
			emptyMsg: emptyMsg(m.tiClean.Value(), len(m.tmpItems())),
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
		sb.WriteString(leftPad + styleSep.Render("Will delete:") + "\n")
		rows := 1
		for rows < maxRows && rows-1 < len(toDelete) {
			sb.WriteString(renderRow(listRow{item: toDelete[rows-1]}, false, listOpts{width: width}) + "\n")
			rows++
		}
		for rows < maxRows {
			sb.WriteByte('\n')
			rows++
		}
	case m.inputMode == modeDestPicker:
		rows := make([]listRow, len(m.destFiltered))
		for i, it := range m.destFiltered {
			rows[i] = listRow{item: it.base, matches: it.matches}
		}
		renderRows(&sb, rows, listOpts{
			cursor: m.destCursor, maxRows: maxRows, width: width,
			emptyMsg: emptyMsg(m.tiDest.Value(), nonRepoCount(m.all)),
		})
	default:
		rows := make([]listRow, len(m.filtered))
		for i, it := range m.filtered {
			rows[i] = listRow{item: it.base, matches: it.matches}
		}
		renderRows(&sb, rows, listOpts{
			cursor: m.cursor, maxRows: maxRows, width: width,
			showActive: true,
			emptyMsg:   emptyMsg(m.tiQuery.Value(), len(m.all)),
		})
	}

	sb.WriteString(leftPad)
	sb.WriteString(styleSep.Render(sep))
	sb.WriteByte('\n')

	// no trailing newline: would scroll the terminal, shifting the search bar off screen
	sb.WriteString(m.statusBar(width))

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
