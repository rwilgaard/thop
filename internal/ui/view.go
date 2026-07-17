package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/git"
)

type listRow struct {
	item    baseItem
	matches []int
}

func toListRows(items []scoredItem) []listRow {
	rows := make([]listRow, len(items))
	for i, it := range items {
		rows[i] = listRow{item: it.base, matches: it.matches}
	}
	return rows
}

func scrollWindow(cursor, maxRows, total int) (start, end int) {
	if cursor >= maxRows {
		start = cursor - maxRows + 1
	}
	end = min(start+maxRows, total)
	return
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
func (st styles) renderRows(rows []listRow, o listOpts) []string {
	if len(rows) == 0 {
		if o.emptyMsg == "" {
			return nil
		}
		return []string{leftPad + st.sep.Render(o.emptyMsg)}
	}
	start, end := scrollWindow(o.cursor, o.maxRows, len(rows))
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, st.renderRow(rows[i], i == o.cursor, o))
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

// renderName styles name, highlighting matched byte offsets in match-style runs.
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

func (st styles) renderRow(row listRow, isCursor bool, o listOpts) string {
	c := row.item.candidate
	glyph, glyphColor := iconFor(c, st.icons)

	prefix := leftPad
	if o.selected != nil && o.selected[c.AbsPath] {
		prefix = st.icons.Selected
	}

	iconStyle := lipgloss.NewStyle().Foreground(glyphColor)
	nameStyle := lipgloss.NewStyle()
	if c.IsTmp {
		nameStyle = st.tmpName
	}
	sp := " "
	if isCursor {
		bg := st.selected.GetBackground()
		iconStyle = iconStyle.Background(bg).Bold(true)
		nameStyle = st.selected
		prefix = st.selected.Render(prefix)
		sp = st.selected.Render(sp)
	}

	matchStyle := st.match
	if isCursor {
		matchStyle = matchStyle.Background(st.selected.GetBackground())
	}
	name := renderName(c.RelPath, row.matches, nameStyle, matchStyle)

	showActive := o.showActive && row.item.active
	rightW := 0
	var right string
	if showActive {
		if isCursor {
			right = st.selectedActive.Render(st.activeLabel)
		} else {
			right = st.dimActive.Render(st.activeLabel)
		}
		rightW = lipgloss.Width(st.activeLabel)
	}
	contentW := 1 + lipgloss.Width(glyph) + 1 + lipgloss.Width(c.RelPath)
	pad := max(1, o.width-1-contentW-rightW)
	padStr := strings.Repeat(" ", pad)
	if isCursor {
		padStr = st.selected.Render(padStr)
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

func (st styles) renderHelpOverlay(width int, groups []helpGroup) string {
	cols := make([]string, 0, len(groups))
	for _, g := range groups {
		maxKey := 0
		for _, b := range g.keys {
			maxKey = max(maxKey, lipgloss.Width(b.Help().Key))
		}
		lines := []string{st.sep.Render(g.title)}
		for _, b := range g.keys {
			h := b.Help()
			pad := strings.Repeat(" ", maxKey-lipgloss.Width(h.Key)+2)
			lines = append(lines, st.helpKey.Render(h.Key)+pad+st.helpDesc.Render(h.Desc))
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
	prompt := m.st.icons.Prompt
	switch m.inputMode {
	case modeLoading:
		return leftPad + m.spin.View() + " " + m.st.sep.Render(m.loadingText)
	case modeError:
		hints := m.st.keyHints([][2]string{{"any key", "Dismiss"}, {"ctrl-c", "Quit"}})
		label := m.st.sep.Render(m.st.icons.Warning + "  ")
		return inputRow(label, strings.SplitN(m.errMsg, "\n", 2)[0], hints, width)
	case modeURLInput:
		hints := m.st.keyHints([][2]string{{"enter", "Clone"}, {"esc", "Cancel"}})
		return inputRow(m.st.prompt.Render("Clone repository "+prompt+" "), m.clone.tiURL.View(), hints, width)
	case modeNameInput:
		hints := m.st.keyHints([][2]string{{"enter", "Create"}, {"esc", "Cancel"}})
		label := m.st.prompt.Render("New tmp project " + prompt + " ")
		tiView := m.tmp.tiName.View()
		var hint string
		switch {
		case m.tmp.conflict && !candidates.ValidTmpName(m.tmp.tiName.Value()):
			hint = m.st.sep.Render(" (Invalid name)")
		case m.tmp.conflict:
			hint = m.st.sep.Render(" (Already exists — enter opens it)")
		}
		return inputRow(label, tiView+hint, hints, width)
	case modeCleanTmp:
		hints := m.st.keyHints([][2]string{{"space", "Select"}, {"enter", "Delete"}, {"esc", "Cancel"}})
		return inputRow(m.st.prompt.Render("Delete tmp projects "+prompt+" "), m.clean.tiQuery.View(), hints, width)
	case modeConfirmClean:
		n := len(m.clean.selected)
		if n == 0 {
			n = len(m.clean.filtered)
		}
		noun := "projects"
		if n == 1 {
			noun = "project"
		}
		yn := m.st.sep.Render(" [y/N]")
		return leftPad + m.st.prompt.Render(fmt.Sprintf("Delete %d tmp %s?", n, noun)) + yn
	case modeDestPicker:
		hints := m.st.keyHints([][2]string{{"enter", "Select"}, {"esc", "Back"}})
		return inputRow(m.st.prompt.Render("Clone › Destination "+prompt+" "), m.clone.tiDest.View(), hints, width)
	case modeCloneName:
		hints := m.st.keyHints([][2]string{{"enter", "Clone as"}, {"esc", "Back"}})
		return inputRow(m.st.prompt.Render("Clone › Name conflict "+prompt+" "), m.clone.tiName.View(), hints, width)
	default:
		label := m.st.prompt.Render(prompt + " ")
		tiView := m.tiQuery.View()
		if m.showHelp {
			return leftPad + label + tiView
		}
		hints := m.st.keyHints([][2]string{
			{m.keys.Enter.Help().Key, "Open"},
			{m.keys.Clone.Help().Key, "Clone"},
			{m.keys.Help.Help().Key, "Help"},
		})
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
			lines = append(lines, leftPad+m.st.sep.Render(line))
		}
		return lines
	case m.showHelp:
		var lines []string
		for _, line := range strings.Split(m.st.renderHelpOverlay(width, buildHelpGroups(m.keys)), "\n") {
			lines = append(lines, leftPad+line)
		}
		return lines
	case m.inputMode == modeCloneName:
		conflict := filepath.Join(m.clone.destDir, git.RepoNameFromURL(m.clone.tiURL.Value()))
		return []string{leftPad + m.st.sep.Render(m.st.icons.Warning+" Already exists: "+conflict)}
	case m.inputMode == modeCleanTmp:
		return m.st.renderRows(toListRows(m.clean.filtered), listOpts{
			cursor: m.clean.cursor, maxRows: maxRows, width: width,
			selected: m.clean.selected,
			emptyMsg: emptyMsg(m.clean.tiQuery.Value(), len(m.tmpItems())),
			reversed: m.layoutBottom,
		})
	case m.inputMode == modeConfirmClean:
		var toDelete []baseItem
		if len(m.clean.selected) > 0 {
			for _, item := range m.tmpItems() {
				if m.clean.selected[item.candidate.AbsPath] {
					toDelete = append(toDelete, item)
				}
			}
		} else {
			for _, it := range m.clean.filtered {
				toDelete = append(toDelete, it.base)
			}
		}
		lines := []string{leftPad + m.st.sep.Render("Will delete:")}
		for _, item := range toDelete {
			lines = append(lines, m.st.renderRow(listRow{item: item}, false, listOpts{width: width}))
		}
		return lines
	case m.inputMode == modeDestPicker:
		return m.st.renderRows(toListRows(m.clone.destFiltered), listOpts{
			cursor: m.clone.destCursor, maxRows: maxRows, width: width,
			emptyMsg: emptyMsg(m.clone.tiDest.Value(), nonRepoCount(m.all)),
			reversed: m.layoutBottom,
		})
	default:
		return m.st.renderRows(toListRows(m.filtered), listOpts{
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
	sepLine := leftPad + m.st.sep.Render(strings.Repeat(m.st.icons.Separator, max(0, width-2)))

	var sb strings.Builder
	writeLine := func(l string) {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	if m.layoutBottom {
		writeLine(clampWidth(m.statusBar(width), width))
		writeLine(sepLine)
		for _, l := range body {
			writeLine(l)
		}
		writeLine(sepLine)
		// no trailing newline: would scroll the terminal, shifting the frame
		sb.WriteString(clampWidth(m.searchLine(width), width))
	} else {
		writeLine(clampWidth(m.searchLine(width), width))
		writeLine(sepLine)
		for _, l := range body {
			writeLine(l)
		}
		writeLine(sepLine)
		// no trailing newline: would scroll the terminal, shifting the frame
		sb.WriteString(clampWidth(m.statusBar(width), width))
	}
	return tea.NewView(sb.String())
}

// modePill renders the current mode name as a filled badge for the status bar.
func (st styles) modePill(label string) string {
	return st.statusPill.Render(" " + label + " ")
}

// clampWidth truncates a status/search line so it can never exceed the frame
// width and wrap to a second row, which would scroll the layout.
func clampWidth(line string, width int) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// spelledKey returns a binding's bracketed help-key token ("<ctrl-a>"),
// matching the other hint rows. caretLabel (keymap.go) is the bare compact
// fallback ("^A") used when the spelled form would overflow the bar.
func spelledKey(b key.Binding) string { return bracketKey(b.Help().Key) }

type filterTab struct {
	binding key.Binding
	label   string
	mode    viewMode
}

func (m model) filterTabList() []filterTab {
	return []filterTab{
		{m.keys.All, "All", viewAll},
		{m.keys.Projects, "Projects", viewProject},
		{m.keys.Repos, "Repos", viewRepo},
		{m.keys.Tmp, "Tmp", viewTmp},
	}
}

// filterTabs renders the view-filter segments, formatting each key token via
// keyFn and joining them with sep. The active filter's label is colored text.
func (m model) filterTabs(keyFn func(key.Binding) string, sep string) string {
	var sb strings.Builder
	for i, t := range m.filterTabList() {
		if i > 0 {
			sb.WriteString(sep)
		}
		sb.WriteString(m.st.prompt.Render(keyFn(t.binding)))
		sb.WriteString(" ")
		// Active filter is just colored text — no background pill. Bare labels
		// keep active and inactive the same width, so nothing shifts on switch.
		if m.view == t.mode {
			sb.WriteString(m.st.filterActive.Render(t.label))
		} else {
			sb.WriteString(m.st.sep.Render(t.label))
		}
	}
	return sb.String()
}

func (m model) statusBar(width int) string {
	var left, right string
	switch m.inputMode {
	case modeDestPicker:
		left = m.st.modePill("Clone") + "  " + m.st.sep.Render(m.clone.tiURL.Value())
		right = m.st.sep.Render(fmt.Sprintf("%d items", len(m.clone.destFiltered)))
	case modeCleanTmp, modeConfirmClean:
		left = m.st.modePill("Clean") + "  " + m.st.sep.Render(fmt.Sprintf("%d selected", len(m.clean.selected)))
		right = m.st.sep.Render(fmt.Sprintf("%d items", len(m.clean.filtered)))
	case modeURLInput, modeCloneName:
		left = m.st.modePill("Clone")
	case modeNameInput:
		left = m.st.modePill("New tmp")
	case modeLoading:
		left = m.st.sep.Render(m.loadingText)
	case modeError:
		left = m.st.modePill("Error")
	default:
		count := fmt.Sprintf("%d items", len(m.filtered))
		timeStr := ""
		if len(m.filtered) > 0 {
			if ts, ok := m.lastOpened[m.filtered[m.cursor].base.candidate.AbsPath]; ok {
				timeStr = "opened " + humanizeSince(ts) + " · "
			}
		}
		right = m.st.sep.Render(timeStr + count)
		badge := m.st.modePill("Filter") + "  "
		// Prefer spelled keys (<ctrl-a>) with bullet separators to match the
		// other hint rows; fall back to compact carets (^A) when the row
		// would overflow.
		left = badge + m.filterTabs(spelledKey, m.st.sep.Render(" • "))
		if lipgloss.Width(left)+lipgloss.Width(right) > width-2 {
			left = badge + m.filterTabs(caretLabel, "  ")
		}
		// Still overflowing → drop the time hint, keep the count.
		if timeStr != "" && lipgloss.Width(left)+lipgloss.Width(right) > width-2 {
			right = m.st.sep.Render(count)
		}
	}
	pad := max(1, width-2-lipgloss.Width(left)-lipgloss.Width(right))
	// no trailing newline: would scroll the terminal, shifting the search bar off screen
	return leftPad + left + strings.Repeat(" ", pad) + right
}

// humanizeSince formats the gap between now and ts as a terse relative string.
func humanizeSince(ts int64) string {
	age := time.Now().Unix() - ts
	switch {
	case age < 60:
		return "just now"
	case age < 3600:
		return fmt.Sprintf("%dm ago", age/60)
	case age < 86400:
		return fmt.Sprintf("%dh ago", age/3600)
	case age < 604800:
		return fmt.Sprintf("%dd ago", age/86400)
	case age < 2592000:
		return fmt.Sprintf("%dw ago", age/604800)
	default:
		return fmt.Sprintf("%dmo ago", age/2592000)
	}
}
