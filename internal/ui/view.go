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

// emptyMsg returns an empty-state message: "nothing here" if pool is empty or
// query is empty, "no matches" otherwise.
func emptyMsg(query string, pool int) string {
	if pool == 0 || query == "" {
		return "nothing here"
	}
	return "no matches"
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
		hints := keyHints([][2]string{{"any key", "dismiss"}, {"ctrl+c", "quit"}})
		label := styleSep.Render("⚠  ")
		searchLine = inputRow(label, strings.SplitN(m.errMsg, "\n", 2)[0], hints, width)
	case modeURLInput:
		hints := keyHints([][2]string{{"enter", "Confirm"}, {"esc", "Cancel"}})
		searchLine = inputRow(stylePrompt.Render("clone url: "), m.tiURL.View(), hints, width)
	case modeNameInput:
		hints := keyHints([][2]string{{"enter", "create"}, {"esc", "cancel"}})
		label := stylePrompt.Render("tmp name: ")
		tiView := m.tiName.View()
		var hint string
		switch {
		case m.nameConflict && invalidTmpName(m.tiName.Value()):
			hint = styleSep.Render(" (invalid name)")
		case m.nameConflict:
			hint = styleSep.Render(" (exists — enter to open)")
		case m.tiName.Value() == "":
			hint = styleSep.Render(" (auto)")
		}
		searchLine = inputRow(label, tiView+hint, hints, width)
	case modeCleanTmp:
		hints := keyHints([][2]string{{"space", "toggle"}, {"enter", "confirm"}, {"esc", "cancel"}})
		searchLine = inputRow(stylePrompt.Render("delete tmp: "), m.tiClean.View(), hints, width)
	case modeConfirmClean:
		n := len(m.selected)
		if n == 0 {
			n = len(m.cleanFiltered)
		}
		yn := styleSep.Render(" [y/N]")
		searchLine = leftPad + stylePrompt.Render(fmt.Sprintf("delete %d tmp project(s)?", n)) + yn
	case modeDestPicker:
		hints := keyHints([][2]string{{"enter", "Pick"}, {"esc", "Back"}})
		searchLine = inputRow(stylePrompt.Render("clone into: "), m.tiDest.View(), hints, width)
	case modeCloneName:
		hints := keyHints([][2]string{{"enter", "Confirm"}, {"esc", "Back"}})
		searchLine = inputRow(stylePrompt.Render("name conflict — rename: "), m.tiCloneName.View(), hints, width)
	default:
		label := stylePrompt.Render("❯ ")
		tiView := m.tiQuery.View()
		if m.helpModel.ShowAll {
			searchLine = leftPad + label + tiView
		} else {
			searchLine = inputRow(label, tiView, m.helpModel.View(keys), width)
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
	case m.helpModel.ShowAll:
		helpContent := m.helpModel.View(keys)
		rows := 0
		for _, line := range strings.Split(helpContent, "\n") {
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
		msg := styleSep.Render("⚠ already exists: " + conflict)
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
		sb.WriteString(leftPad + styleSep.Render("will delete:") + "\n")
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
			emptyMsg: emptyMsg(m.tiQuery.Value(), len(m.all)),
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
		left = styleSep.Render("clone: " + m.tiURL.Value())
		right = styleSep.Render(fmt.Sprintf("%d items", len(m.destFiltered)))
	case modeCleanTmp, modeConfirmClean:
		left = styleSep.Render(fmt.Sprintf("%d selected", len(m.selected)))
		right = styleSep.Render(fmt.Sprintf("%d items", len(m.cleanFiltered)))
	case modeURLInput, modeCloneName:
		left = styleSep.Render("clone")
	case modeNameInput:
		left = styleSep.Render("new tmp")
	case modeLoading:
		left = styleSep.Render(m.loadingText)
	case modeError:
		left = styleSep.Render("error")
	default:
		var sb strings.Builder
		viewLabels := []struct {
			label string
			mode  viewMode
		}{
			{"all", viewAll},
			{"projects", viewProject},
			{"repos", viewRepo},
			{"tmp", viewTmp},
		}
		for i, v := range viewLabels {
			if i > 0 {
				sb.WriteString(styleSep.Render(" · "))
			}
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
