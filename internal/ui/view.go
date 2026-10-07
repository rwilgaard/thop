package ui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

func (m model) missingLine(width int) string {
	names := make([]string, len(m.missing))
	for i, p := range m.missing {
		names[i] = tilde(p)
	}
	msg := m.st.icons.Warning + " Not found: " + strings.Join(names, ", ")
	if m.configFile != "" {
		msg += " — check " + m.configFile
	}
	msg, _ = truncateName(msg, nil, width-2)
	return leftPad + m.st.dimActive.Render(msg)
}

func (m model) emptyMsg() string {
	if len(m.paths) == 0 && m.configFile != "" && m.tiQuery.Value() == "" && m.view == viewAll {
		return "No project roots. Add paths in " + m.configFile
	}
	return emptyMsg(m.tiQuery.Value(), len(m.all))
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
		msg, _ := truncateName(o.emptyMsg, nil, o.width-2)
		return []string{leftPad + st.sep.Render(msg)}
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

func truncateName(name string, matches []int, maxW int) (string, []int) {
	if lipgloss.Width(name) <= maxW {
		return name, matches
	}
	cut, w := 0, 0
	for i, r := range name {
		rw := lipgloss.Width(string(r))
		if w+rw > maxW-1 {
			break
		}
		w += rw
		cut = i + len(string(r))
	}
	kept := make([]int, 0, len(matches))
	for _, i := range matches {
		if i < cut {
			kept = append(kept, i)
		}
	}
	return name[:cut] + "…", kept
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
	rightW := 0
	var right string
	if o.showActive && row.item.active {
		label := st.activeLabel
		if row.item.current {
			label = st.currentLabel
		}
		if isCursor {
			right = st.selectedActive.Render(label)
		} else {
			right = st.dimActive.Render(label)
		}
		rightW = lipgloss.Width(label)
	}
	fixedW := 1 + lipgloss.Width(glyph) + 1
	nameW := o.width - 1 - fixedW - 1 - rightW
	text, matches := truncateName(c.RelPath, row.matches, nameW)
	name := renderName(text, matches, nameStyle, matchStyle)
	contentW := fixedW + lipgloss.Width(text)
	// rows that look alike say which root they are in, space permitting
	if room := nameW - lipgloss.Width(text) - 2; c.Collides && room >= 4 {
		root, _ := truncateName(tilde(c.Root), nil, room)
		rootStyle := st.sep
		if isCursor {
			rootStyle = st.selected.Bold(false).Faint(true)
		}
		name += sp + sp + rootStyle.Render(root)
		contentW += 2 + lipgloss.Width(root)
	}
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

// helpColumns lays the help groups out in the widest arrangement that fits
// limit: three columns, two (first group beside the rest), or stacked.
func (st styles) helpColumns(limit int, groups []helpGroup) string {
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

	stacked := func(c []string) string { return strings.Join(c, "\n\n") }
	if l := joinCols(cols, "    "); lipgloss.Width(l) <= limit {
		return l
	}
	if l := joinCols(cols, "  "); lipgloss.Width(l) <= limit {
		return l
	}
	if len(cols) > 2 {
		if l := joinCols([]string{cols[0], stacked(cols[1:])}, "  "); lipgloss.Width(l) <= limit {
			return l
		}
	}
	return stacked(cols)
}

func (m model) searchLine(width int) string {
	prompt := m.st.icons.Prompt
	switch m.inputMode {
	case modeLoading:
		return leftPad + m.spin.View() + " " + m.st.sep.Render(m.loadingText)
	case modeCleanTmp:
		return m.st.inputRow(m.st.prompt.Render("Delete tmp projects "+prompt+" "), m.clean.tiQuery.View(),
			[][2]string{{"enter", "Delete"}, {"space", "Select"}, {"esc", "Cancel"}}, width)
	case modeDestPicker:
		return m.st.inputRow(m.st.prompt.Render("Clone › Destination "+prompt+" "), m.clone.tiDest.View(),
			[][2]string{{"enter", "Select"}, {"esc", "Back"}}, width)
	case modeNewProjRoot:
		return m.st.inputRow(m.st.prompt.Render("New project › Root "+prompt+" "), m.newProj.tiRoot.View(),
			[][2]string{{"enter", "Select"}, {"esc", "Back"}}, width)
	default:
		hints := [][2]string{
			{m.keys.Enter.Help().Key, "Open"},
			{m.keys.Help.Help().Key, "Help"},
			{m.keys.NextFilter.Help().Key, "Filter"},
		}
		if m.cursor < len(m.filtered) && m.filtered[m.cursor].base.active {
			hints = append(hints, [2]string{m.keys.Close.Help().Key, "Close"})
		}
		hints = append(hints, [2]string{m.keys.Clone.Help().Key, "Clone"})
		return m.st.inputRow(m.st.prompt.Render(prompt+" "), m.tiQuery.View(), hints, width)
	}
}

func (m model) bodyLines(width, maxRows int) []string {
	switch m.inputMode {
	case modeLoading:
		return nil
	case modeCleanTmp:
		return m.st.renderRows(toListRows(m.clean.filtered), listOpts{
			cursor: m.clean.cursor, maxRows: maxRows, width: width,
			selected: m.clean.selected, showActive: true,
			emptyMsg: emptyMsg(m.clean.tiQuery.Value(), len(m.tmpItems())),
			reversed: m.layoutBottom,
		})
	case modeNewProjRoot:
		return m.st.renderRows(toListRows(m.newProj.filtered), listOpts{
			cursor: m.newProj.cursor, maxRows: maxRows, width: width,
			emptyMsg: "No matches",
			reversed: m.layoutBottom,
		})
	case modeDestPicker:
		return m.st.renderRows(toListRows(m.clone.destFiltered), listOpts{
			cursor: m.clone.destCursor, maxRows: maxRows, width: width,
			emptyMsg: emptyMsg(m.clone.tiDest.Value(), nonRepoCount(m.all)),
			reversed: m.layoutBottom,
		})
	default:
		return m.st.renderRows(toListRows(m.filtered), listOpts{
			cursor: m.cursor, maxRows: maxRows, width: width,
			showActive: true,
			emptyMsg:   m.emptyMsg(),
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
	frame := m.frame(width, m.maxRows())
	height := lipgloss.Height(frame)
	// dialog chrome: two borders, a blank line and up to two hint rows
	if d, ok := m.dialog(width, height-5); ok {
		frame = overlay(frame, m.st.renderDialog(d), width, height)
	}
	return tea.NewView(frame)
}

// frame renders the search line, list and status bar. Behind a dialog the
// search line and list are those of the backdrop mode, dimmed.
func (m model) frame(width, maxRows int) string {
	bg := m
	bg.inputMode = m.backdropMode()
	search := clampWidth(bg.searchLine(width), width)
	body := fillRows(bg.bodyLines(width, maxRows), maxRows, m.layoutBottom)
	if len(m.missing) > 0 {
		body = append(body, m.missingLine(width))
	}
	if bg.inputMode != m.inputMode {
		search = m.st.dim(search)
		for i, l := range body {
			body[i] = m.st.dim(l)
		}
	}
	status := clampWidth(m.statusBar(width), width)
	sepLine := leftPad + m.st.sep.Render(strings.Repeat(m.st.icons.Separator, max(0, width-2)))

	first, last := search, status
	if m.layoutBottom {
		first, last = status, search
	}
	lines := append([]string{first, sepLine}, body...)
	// no trailing newline: would scroll the terminal, shifting the frame
	return strings.Join(append(lines, sepLine, last), "\n")
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
		{m.keys.Open, "Open", viewOpen},
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
		if len(t.binding.Keys()) > 0 {
			sb.WriteString(m.st.prompt.Render(keyFn(t.binding)))
			sb.WriteString(" ")
		}
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

func position(cursor, n int) string {
	if n == 0 {
		return "0 items"
	}
	return fmt.Sprintf("%d/%d", cursor+1, n)
}

func (m model) statusBar(width int) string {
	var left, right string
	switch m.inputMode {
	case modeDestPicker:
		left = m.st.modePill("Clone") + "  " + m.st.sep.Render(m.clone.tiURL.Value())
		right = m.st.sep.Render(position(m.clone.destCursor, len(m.clone.destFiltered)))
	case modeCleanTmp, modeConfirmClean:
		left = m.st.modePill("Clean") + "  " + m.st.sep.Render(fmt.Sprintf("%d selected", len(m.clean.selected)))
		right = m.st.sep.Render(position(m.clean.cursor, len(m.clean.filtered)))
	case modeURLInput, modeCloneName, modeCloning:
		left = m.st.modePill("Clone")
	case modeConfirmClose:
		left = m.st.modePill("Close")
	case modeSetup:
		left = m.st.modePill("Setup")
	case modeNewProjRoot:
		left = m.st.modePill("New project")
		right = m.st.sep.Render(position(m.newProj.cursor, len(m.newProj.filtered)))
	case modeNewProjName:
		left = m.st.modePill("New project")
	case modeNameInput:
		left = m.st.modePill("New tmp")
	case modeLoading:
		left = m.st.sep.Render(m.loadingText)
	case modeError:
		left = m.st.modePill("Error")
	default:
		right = m.st.sep.Render(position(m.cursor, len(m.filtered)))
		badge := m.st.modePill("Filter") + "  "
		// Prefer spelled keys (<ctrl-a>) with bullet separators to match the
		// other hint rows; fall back to compact carets (^A) when the row
		// would overflow.
		left = badge + m.filterTabs(spelledKey, m.st.sep.Render(" • "))
		if lipgloss.Width(left)+lipgloss.Width(right) > width-2 {
			left = badge + m.filterTabs(caretLabel, "  ")
		}
	}
	pad := max(1, width-2-lipgloss.Width(left)-lipgloss.Width(right))
	// no trailing newline: would scroll the terminal, shifting the search bar off screen
	return leftPad + left + strings.Repeat(" ", pad) + right
}
