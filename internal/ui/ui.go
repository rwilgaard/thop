package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/bubbles/v2/textinput"
	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/git"
	"github.com/rwilgaard/thop/internal/tmux"
	"github.com/sahilm/fuzzy"
)

const activeLabel = "● open "
const leftPad = " "

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func normalizeScores(scores map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(scores))
	max := 0.0
	for _, v := range scores {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return out
	}
	for k, v := range scores {
		out[k] = v / max
	}
	return out
}

// combineScore weights fuzzy 60%, frecency 40%.
func combineScore(normFuzzyScore, normFrecency float64) float64 {
	return normFuzzyScore*0.6 + normFrecency*0.4
}

type viewMode int

const (
	viewAll viewMode = iota
	viewProject
	viewRepo
)

type inputMode int

const (
	modeNormal     inputMode = iota
	modeURLInput             // Ctrl-G
	modeDestPicker           // pick clone destination directory
	modeCloneName            // rename on conflict
)

type Result struct {
	Candidate candidates.Candidate
	Clone     *CloneRequest
}

type CloneRequest struct {
	URL  string
	Dest string
}

type baseItem struct {
	candidate candidates.Candidate
	active    bool
	display   string
}

type scoredItem struct {
	base  baseItem
	score float64
}

type model struct {
	all      []baseItem
	normFrec map[string]float64
	filtered []scoredItem
	tiQuery  textinput.Model // modeNormal search
	cursor   int
	view       viewMode
	switchOnly bool
	width      int
	height     int
	result     Result
	ready      bool
	inputMode    inputMode
	tiURL        textinput.Model // modeURLInput
	tiDest       textinput.Model // modeDestPicker
	tiCloneName  textinput.Model // modeCloneName
	destFiltered []scoredItem
	destCursor   int
	cloneDestDir string // chosen parent dir (set when conflict detected)
}

var (
	styleSep            lipgloss.Style
	stylePrompt         lipgloss.Style
	styleSelected       lipgloss.Style
	styleSelectedActive lipgloss.Style
	styleStatusActive   lipgloss.Style
	styleDimActive      lipgloss.Style
)

func initStyles(cfg config.Config) {
	c := cfg.Colors
	styleSep = lipgloss.NewStyle().Faint(true)
	stylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color(c.PromptColor)).Bold(true)
	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color(c.SelectionBg)).Foreground(lipgloss.Color(c.SelectionFg)).Bold(true)
	styleSelectedActive = lipgloss.NewStyle().Background(lipgloss.Color(c.SelectionBg)).Foreground(lipgloss.Color(c.ActiveColor)).Bold(true)
	styleStatusActive = lipgloss.NewStyle().Foreground(lipgloss.Color(c.StatusActiveColor)).Bold(true)
	styleDimActive = lipgloss.NewStyle().Foreground(lipgloss.Color(c.ActiveColor))
}

func newTextInput(prompt string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.CharLimit = 0
	return ti
}

func newModel(cs []candidates.Candidate, scores map[string]float64, ts tmux.TmuxState, switchOnly bool) model {
	all := make([]baseItem, 0, len(cs))
	for _, c := range cs {
		all = append(all, makeBaseItem(c, ts))
	}
	m := model{
		all:         all,
		normFrec:    normalizeScores(scores),
		switchOnly:  switchOnly,
		tiQuery:     newTextInput(""),
		tiURL:       newTextInput(""),
		tiDest:      newTextInput(""),
		tiCloneName: newTextInput(""),
	}
	_ = m.tiQuery.Focus()
	m.rebuildFiltered()
	return m
}

func makeBaseItem(c candidates.Candidate, ts tmux.TmuxState) baseItem {
	active := candidates.CandidateActive(c, ts.Sessions, ts.Windows)
	return baseItem{
		candidate: c,
		active:    active,
		display:   candidates.FormatDisplay(c, false),
	}
}

func (m *model) rebuildFiltered() {
	query := m.tiQuery.Value()
	var result []scoredItem

	if query == "" {
		for _, item := range m.all {
			if m.switchOnly && !item.active {
				continue
			}
			if !m.matchesView(item) {
				continue
			}
			result = append(result, scoredItem{
				base:  item,
				score: m.normFrec[item.candidate.AbsPath],
			})
		}
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].score > result[j].score
		})
	} else {
		// fuzzy match on RelPath — avoids ANSI escape codes in display strings
		keys := make([]string, len(m.all))
		for i, item := range m.all {
			keys[i] = item.candidate.RelPath
		}
		matches := fuzzy.Find(query, keys)

		type pendingItem struct {
			base     baseItem
			rawScore float64
		}
		var pending []pendingItem
		for _, match := range matches {
			item := m.all[match.Index]
			if m.switchOnly && !item.active {
				continue
			}
			if !m.matchesView(item) {
				continue
			}
			pending = append(pending, pendingItem{base: item, rawScore: float64(match.Score)})
		}

		maxRaw := 0.0
		for _, p := range pending {
			if p.rawScore > maxRaw {
				maxRaw = p.rawScore
			}
		}

		for _, p := range pending {
			normF := 0.0
			if maxRaw > 0 {
				normF = p.rawScore / maxRaw
			}
			result = append(result, scoredItem{
				base:  p.base,
				score: combineScore(normF, m.normFrec[p.base.candidate.AbsPath]),
			})
		}
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].score > result[j].score
		})
	}

	m.filtered = result
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

func (m *model) rebuildDestFiltered() {
	destQuery := m.tiDest.Value()
	var result []scoredItem
	if destQuery == "" {
		for _, item := range m.all {
			if item.candidate.IsRepo {
				continue
			}
			result = append(result, scoredItem{base: item, score: m.normFrec[item.candidate.AbsPath]})
		}
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].score > result[j].score
		})
	} else {
		keys := make([]string, len(m.all))
		for i, item := range m.all {
			keys[i] = item.candidate.RelPath
		}
		matches := fuzzy.Find(destQuery, keys)
		maxRaw := 0.0
		type pending struct {
			base     baseItem
			rawScore float64
		}
		var pend []pending
		for _, match := range matches {
			item := m.all[match.Index]
			if item.candidate.IsRepo {
				continue
			}
			raw := float64(match.Score)
			pend = append(pend, pending{base: item, rawScore: raw})
			if raw > maxRaw {
				maxRaw = raw
			}
		}
		for _, p := range pend {
			normF := 0.0
			if maxRaw > 0 {
				normF = p.rawScore / maxRaw
			}
			result = append(result, scoredItem{
				base:  p.base,
				score: combineScore(normF, m.normFrec[p.base.candidate.AbsPath]),
			})
		}
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].score > result[j].score
		})
	}
	m.destFiltered = result
	if m.destCursor >= len(m.destFiltered) {
		m.destCursor = 0
	}
}

func (m *model) matchesView(item baseItem) bool {
	switch m.view {
	case viewProject:
		return !item.candidate.IsRepo
	case viewRepo:
		return item.candidate.IsRepo
	default: // viewAll
		return true
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
	case tea.KeyPressMsg:
		switch m.inputMode {
		case modeURLInput:
			return m.updateURLInput(msg)
		case modeDestPicker:
			return m.updateDestPicker(msg)
		case modeCloneName:
			return m.updateCloneName(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	// Forward all messages to the active textinput so paste, blink, etc. work.
	var cmd tea.Cmd
	switch m.inputMode {
	case modeNormal:
		prev := m.tiQuery.Value()
		m.tiQuery, cmd = m.tiQuery.Update(msg)
		if m.tiQuery.Value() != prev {
			m.cursor = 0
			m.rebuildFiltered()
		}
	case modeURLInput:
		m.tiURL, cmd = m.tiURL.Update(msg)
	case modeDestPicker:
		prev := m.tiDest.Value()
		m.tiDest, cmd = m.tiDest.Update(msg)
		if m.tiDest.Value() != prev {
			m.destCursor = 0
			m.rebuildDestFiltered()
		}
	case modeCloneName:
		m.tiCloneName, cmd = m.tiCloneName.Update(msg)
	}
	return m, cmd
}

func (m model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		if len(m.filtered) > 0 {
			m.result.Candidate = m.filtered[m.cursor].base.candidate
		}
		return m, tea.Quit
	case "up", "ctrl+k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "ctrl+j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "ctrl+a":
		m.view, m.cursor = viewAll, 0
		m.rebuildFiltered()
	case "ctrl+p":
		m.view, m.cursor = viewProject, 0
		m.rebuildFiltered()
	case "ctrl+r":
		m.view, m.cursor = viewRepo, 0
		m.rebuildFiltered()
	case "ctrl+g":
		m.tiQuery.Blur()
		m.inputMode = modeURLInput
		m.tiURL.SetValue("")
		return m, m.tiURL.Focus()
	default:
		prev := m.tiQuery.Value()
		var cmd tea.Cmd
		m.tiQuery, cmd = m.tiQuery.Update(msg)
		if m.tiQuery.Value() != prev {
			m.cursor = 0
			m.rebuildFiltered()
		}
		return m, cmd
	}
	return m, nil
}

func (m model) updateURLInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.tiURL.Blur()
		m.tiURL.SetValue("")
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	case "enter":
		if m.tiURL.Value() != "" {
			m.tiURL.Blur()
			m.tiDest.SetValue("")
			m.destCursor = 0
			m.rebuildDestFiltered()
			m.inputMode = modeDestPicker
			return m, m.tiDest.Focus()
		}
	default:
		var cmd tea.Cmd
		m.tiURL, cmd = m.tiURL.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateDestPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.tiDest.Blur()
		m.inputMode = modeURLInput
		return m, m.tiURL.Focus()
	case "enter":
		if m.destCursor < len(m.destFiltered) {
			chosen := m.destFiltered[m.destCursor].base.candidate.AbsPath
			name := git.RepoNameFromURL(m.tiURL.Value())
			fullDest := filepath.Join(chosen, name)
			if _, err := os.Stat(fullDest); err == nil {
				m.tiDest.Blur()
				m.cloneDestDir = chosen
				m.tiCloneName.SetValue(name)
				m.inputMode = modeCloneName
				return m, m.tiCloneName.Focus()
			}
			m.result.Clone = &CloneRequest{URL: m.tiURL.Value(), Dest: fullDest}
			return m, tea.Quit
		}
		return m, tea.Quit
	case "up", "ctrl+k":
		if m.destCursor > 0 {
			m.destCursor--
		}
	case "down", "ctrl+j":
		if m.destCursor < len(m.destFiltered)-1 {
			m.destCursor++
		}
	default:
		prev := m.tiDest.Value()
		var cmd tea.Cmd
		m.tiDest, cmd = m.tiDest.Update(msg)
		if m.tiDest.Value() != prev {
			m.destCursor = 0
			m.rebuildDestFiltered()
		}
		return m, cmd
	}
	return m, nil
}

func (m model) updateCloneName(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.tiCloneName.Blur()
		m.inputMode = modeDestPicker
		return m, m.tiDest.Focus()
	case "enter":
		if m.tiCloneName.Value() != "" {
			m.result.Clone = &CloneRequest{
				URL:  m.tiURL.Value(),
				Dest: filepath.Join(m.cloneDestDir, m.tiCloneName.Value()),
			}
			return m, tea.Quit
		}
	default:
		var cmd tea.Cmd
		m.tiCloneName, cmd = m.tiCloneName.Update(msg)
		return m, cmd
	}
	return m, nil
}

func keyHints(pairs [][2]string) (rendered string, width int) {
	var parts []string
	for _, p := range pairs {
		key := stylePrompt.Render("<" + p[0] + ">")
		action := styleSep.Render(p[1])
		parts = append(parts, key+" "+action)
	}
	rendered = strings.Join(parts, "  ")
	width = lipgloss.Width(rendered)
	return
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

	sep := strings.Repeat("─", width-2)

	var sb strings.Builder
	var searchLine string
	switch m.inputMode {
	case modeURLInput:
		hints, hintsW := keyHints([][2]string{{"enter", "Confirm"}, {"esc", "Cancel"}})
		label := stylePrompt.Render("clone url: ")
		tiView := m.tiURL.View()
		tiW := lipgloss.Width(tiView)
		pad := max(1, width-2-lipgloss.Width(label)-tiW-hintsW)
		searchLine = leftPad + label + tiView + strings.Repeat(" ", pad) + hints
	case modeDestPicker:
		hints, hintsW := keyHints([][2]string{{"enter", "Pick"}, {"esc", "Back"}})
		label := stylePrompt.Render("clone into: ")
		tiView := m.tiDest.View()
		tiW := lipgloss.Width(tiView)
		pad := max(1, width-2-lipgloss.Width(label)-tiW-hintsW)
		searchLine = leftPad + label + tiView + strings.Repeat(" ", pad) + hints
	case modeCloneName:
		hints, hintsW := keyHints([][2]string{{"enter", "Confirm"}, {"esc", "Back"}})
		label := stylePrompt.Render("name conflict — rename: ")
		tiView := m.tiCloneName.View()
		tiW := lipgloss.Width(tiView)
		pad := max(1, width-2-lipgloss.Width(label)-tiW-hintsW)
		searchLine = leftPad + label + tiView + strings.Repeat(" ", pad) + hints
	default:
		hints, hintsW := keyHints([][2]string{
			{"ctrl-g", "Clone"}, {"ctrl-a", "All"}, {"ctrl-p", "Proj"}, {"ctrl-r", "Repo"},
		})
		label := stylePrompt.Render("❯ ")
		tiView := m.tiQuery.View()
		pad := max(1, width-2-lipgloss.Width(label)-lipgloss.Width(tiView)-hintsW)
		searchLine = leftPad + label + tiView + strings.Repeat(" ", pad) + hints
	}
	sb.WriteString(searchLine)
	sb.WriteByte('\n')

	sb.WriteString(leftPad)
	sb.WriteString(styleSep.Render(sep))
	sb.WriteByte('\n')

	// height budget: search + top-sep + bottom-sep + status = 4
	maxRows := max(5, height-4)

	switch m.inputMode {
	case modeCloneName:
		conflict := filepath.Join(m.cloneDestDir, git.RepoNameFromURL(m.tiURL.Value()))
		msg := styleSep.Render("⚠ already exists: " + conflict)
		sb.WriteString(leftPad + msg + "\n")
		for range maxRows - 1 {
			sb.WriteByte('\n')
		}
	case modeDestPicker:
		start := 0
		if m.destCursor >= maxRows {
			start = m.destCursor - maxRows + 1
		}
		end := min(start+maxRows, len(m.destFiltered))
		for i := start; i < end; i++ {
			item := m.destFiltered[i]
			if i == m.destCursor {
				plain := ansiEscape.ReplaceAllString(item.base.display, "")
				pad := max(1, width-1-lipgloss.Width(plain)-1)
				sb.WriteString(styleSelected.Render(leftPad + plain + strings.Repeat(" ", pad) + " "))
				sb.WriteByte('\n')
			} else {
				sb.WriteString(leftPad + item.base.display + "\n")
			}
		}
		for range maxRows - (end - start) {
			sb.WriteByte('\n')
		}
	default:
		start := 0
		if m.cursor >= maxRows {
			start = m.cursor - maxRows + 1
		}
		end := min(start+maxRows, len(m.filtered))

		for i := start; i < end; i++ {
			item := m.filtered[i]
			if i == m.cursor {
				// strip ANSI so background color fills the full row
				plain := ansiEscape.ReplaceAllString(item.base.display, "")
				plainW := lipgloss.Width(plain)
				if item.base.active {
					activeW := lipgloss.Width(activeLabel)
					pad := max(1, width-1-plainW-activeW)
					sb.WriteString(styleSelected.Render(leftPad + plain + strings.Repeat(" ", pad)))
					sb.WriteString(styleSelectedActive.Render(activeLabel))
					sb.WriteByte('\n')
				} else {
					pad := max(1, width-1-plainW-1)
					sb.WriteString(styleSelected.Render(leftPad + plain + strings.Repeat(" ", pad) + " "))
					sb.WriteByte('\n')
				}
			} else {
				sb.WriteString(leftPad)
				sb.WriteString(item.base.display)
				if item.base.active {
					activeStr := styleDimActive.Render(activeLabel)
					displayW := lipgloss.Width(item.base.display)
					activeW := lipgloss.Width(activeStr)
					pad := max(1, width-1-displayW-activeW)
					sb.WriteString(strings.Repeat(" ", pad))
					sb.WriteString(activeStr)
				}
				sb.WriteByte('\n')
			}
		}

		rendered := end - start
		for range maxRows - rendered {
			sb.WriteString("\n")
		}
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

func Run(cs []candidates.Candidate, scores map[string]float64, ts tmux.TmuxState, switchOnly bool, cfg config.Config) (Result, error) {
	initStyles(cfg)
	m := newModel(cs, scores, ts, switchOnly)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	if m, ok := final.(model); ok {
		return m.result, nil
	}
	return Result{}, nil
}

func RunDestPicker(cs []candidates.Candidate, cfg config.Config) (string, error) {
	initStyles(cfg)
	m := newModel(cs, map[string]float64{}, tmux.TmuxState{}, false)
	m.inputMode = modeDestPicker
	m.rebuildDestFiltered()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if fm, ok := final.(model); ok && fm.result.Clone != nil {
		return fm.result.Clone.Dest, nil
	}
	return "", nil
}
