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
	matches []int //nolint:unused // wired up by match highlighting
}

type listOpts struct {
	cursor     int
	maxRows    int
	width      int
	showActive bool
	selected   map[string]bool // non-nil: render ✓ prefix for selected AbsPaths
	emptyMsg   string
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

	name := nameStyle.Render(c.RelPath)

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
		searchLine = leftPad + styleSep.Render(m.loadingText)
	case modeError:
		hints := keyHints([][2]string{{"any key", "quit"}})
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
		for i, item := range m.cleanFiltered {
			rows[i] = listRow{item: item}
		}
		renderRows(&sb, rows, listOpts{
			cursor: m.cleanCursor, maxRows: maxRows, width: width,
			selected: m.selected,
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
			toDelete = m.cleanFiltered
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
			rows[i] = listRow{item: it.base}
		}
		renderRows(&sb, rows, listOpts{
			cursor: m.destCursor, maxRows: maxRows, width: width,
		})
	default:
		rows := make([]listRow, len(m.filtered))
		for i, it := range m.filtered {
			rows[i] = listRow{item: it.base}
		}
		renderRows(&sb, rows, listOpts{
			cursor: m.cursor, maxRows: maxRows, width: width,
			showActive: true,
		})
	}

	sb.WriteString(leftPad)
	sb.WriteString(styleSep.Render(sep))
	sb.WriteByte('\n')

	type viewLabel struct {
		label string
		mode  viewMode
	}
	viewLabels := []viewLabel{
		{"all", viewAll},
		{"projects", viewProject},
		{"repos", viewRepo},
		{"tmp", viewTmp},
	}
	var statusSb strings.Builder
	for i, v := range viewLabels {
		if i > 0 {
			statusSb.WriteString(styleSep.Render(" · "))
		}
		if m.view == v.mode {
			statusSb.WriteString(styleStatusActive.Render("● " + v.label))
		} else {
			statusSb.WriteString(styleSep.Render(v.label))
		}
	}
	statusLeft := statusSb.String()
	count := styleSep.Render(fmt.Sprintf("%d items", len(m.filtered)))
	statusLeftW := lipgloss.Width(statusLeft)
	countW := lipgloss.Width(count)
	statusPad := max(1, width-1-statusLeftW-countW-1)
	// no trailing newline: would scroll the terminal, shifting the search bar off screen
	sb.WriteString(leftPad)
	sb.WriteString(statusLeft)
	sb.WriteString(strings.Repeat(" ", statusPad))
	sb.WriteString(count)

	return tea.NewView(sb.String())
}
