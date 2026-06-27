package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
	"github.com/sahilm/fuzzy"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// ── Scoring ──────────────────────────────────────────────────────────────────

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

// combineScore merges a normalized fuzzy score and a normalized frecency score
// using a 60/40 weighting. Both inputs must be in [0,1].
func combineScore(normFuzzyScore, normFrecency float64) float64 {
	return normFuzzyScore*0.6 + normFrecency*0.4
}

// ── Types ─────────────────────────────────────────────────────────────────────

type viewMode int

const (
	viewAll viewMode = iota
	viewProject
	viewRepo
)

type baseItem struct {
	candidate candidates.Candidate
	active    bool
	display   string // ANSI-colored display string
}

type scoredItem struct {
	base  baseItem
	score float64
}

type model struct {
	all        []baseItem
	normFrec   map[string]float64
	filtered   []scoredItem
	query      string
	cursor     int
	view       viewMode
	switchOnly bool
	width      int
	height     int
	chosen     candidates.Candidate
	ready      bool
}

// ── Styles ────────────────────────────────────────────────────────────────────

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

// ── Constructor ───────────────────────────────────────────────────────────────

func newModel(cs []candidates.Candidate, scores map[string]float64, ts tmux.TmuxState, switchOnly bool) model {
	all := make([]baseItem, 0, len(cs))
	for _, c := range cs {
		all = append(all, makeBaseItem(c, ts))
	}

	m := model{
		all:        all,
		normFrec:   normalizeScores(scores),
		switchOnly: switchOnly,
	}
	m.rebuildFiltered()
	return m
}

func makeBaseItem(c candidates.Candidate, ts tmux.TmuxState) baseItem {
	active := candidates.CandidateActive(c, ts.Sessions, ts.Windows)
	return baseItem{
		candidate: c,
		active:    active,
		display:   candidates.FormatDisplay(c.RelPath, c.IsRepo, false),
	}
}

// ── Filtering & scoring ───────────────────────────────────────────────────────

func (m *model) rebuildFiltered() {
	var result []scoredItem

	if m.query == "" {
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
		// Fuzzy match on RelPath (no ANSI codes to trip up matching).
		keys := make([]string, len(m.all))
		for i, item := range m.all {
			keys[i] = item.candidate.RelPath
		}
		matches := fuzzy.Find(m.query, keys)

		// Step 1: filter to only visible matches so normalization is over the
		// displayed set, not over hidden items.
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

		// Step 2: normalize raw fuzzy scores across the visible set so that the
		// fuzzy contribution is bounded to [0,1] — matching the frecency scale.
		maxRaw := 0.0
		for _, p := range pending {
			if p.rawScore > maxRaw {
				maxRaw = p.rawScore
			}
		}

		// Step 3: combine normalized fuzzy + frecency.
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

func (m *model) matchesView(item baseItem) bool {
	switch m.view {
	case viewProject:
		return !item.candidate.IsRepo
	case viewRepo:
		return item.candidate.IsRepo
	default:
		return true
	}
}

// ── Bubbletea interface ───────────────────────────────────────────────────────

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.chosen = m.filtered[m.cursor].base.candidate
			}
			return m, tea.Quit

		case tea.KeyUp, tea.KeyCtrlK:
			if m.cursor > 0 {
				m.cursor--
			}

		case tea.KeyDown, tea.KeyCtrlJ:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}

		case tea.KeyBackspace, tea.KeyCtrlH:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.cursor = 0
				m.rebuildFiltered()
			}

		case tea.KeyCtrlA:
			m.view = viewAll
			m.cursor = 0
			m.rebuildFiltered()

		case tea.KeyCtrlP:
			m.view = viewProject
			m.cursor = 0
			m.rebuildFiltered()

		case tea.KeyCtrlR:
			m.view = viewRepo
			m.cursor = 0
			m.rebuildFiltered()

		case tea.KeyRunes:
			m.query += string(msg.Runes)
			m.cursor = 0
			m.rebuildFiltered()
		}
	}
	return m, nil
}

func (m model) View() string {
	if !m.ready {
		return ""
	}
	width := m.width
	if width == 0 {
		width = 80
	}
	height := m.height
	if height == 0 {
		height = 24
	}

	sep := strings.Repeat("─", width-1)

	var sb strings.Builder

	// Search row: ❯ query█                  ctrl-a · ctrl-p · ctrl-r
	hints := styleSep.Render("ctrl-a · ctrl-p · ctrl-r")
	promptStr := stylePrompt.Render("❯ ") + m.query + "█"
	promptW := lipgloss.Width(stylePrompt.Render("❯ ")) + len([]rune(m.query)) + 1 // + "█"
	hintsW := lipgloss.Width(hints)
	searchPad := max(1, width-promptW-hintsW-1)
	sb.WriteString(promptStr)
	sb.WriteString(strings.Repeat(" ", searchPad))
	sb.WriteString(hints)
	sb.WriteByte('\n')

	// Top separator
	sb.WriteString(styleSep.Render(sep))
	sb.WriteByte('\n')

	// List (scrolling window around cursor)
	// height budget: search(1) + top-sep(1) + bottom-sep(1) + status(1) = 4
	maxRows := max(5, height-4)
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := min(start+maxRows, len(m.filtered))

	for i := start; i < end; i++ {
		item := m.filtered[i]
		if i == m.cursor {
			// Strip ANSI so background color covers the full row cleanly.
			plain := ansiEscape.ReplaceAllString(item.base.display, "")
			plainW := lipgloss.Width(plain)
			if item.base.active {
				activeW := lipgloss.Width("● open")
				pad := max(1, width-1-plainW-activeW-3)
				sb.WriteString(styleSelected.Render(" " + plain + strings.Repeat(" ", pad)))
				sb.WriteString(styleSelectedActive.Render(" ● open "))
				sb.WriteByte('\n')
			} else {
				pad := max(1, width-1-plainW-1)
				sb.WriteString(styleSelected.Render(" " + plain + strings.Repeat(" ", pad) + " "))
				sb.WriteByte('\n')
			}
		} else {
			sb.WriteByte(' ')
			sb.WriteString(item.base.display)
			if item.base.active {
				activeStr := styleDimActive.Render("● open")
				displayW := lipgloss.Width(item.base.display)
				activeW := lipgloss.Width(activeStr)
				pad := max(1, width-1-displayW-activeW-1)
				sb.WriteString(strings.Repeat(" ", pad))
				sb.WriteString(activeStr)
			}
			sb.WriteByte('\n')
		}
	}

	// Pad remaining rows so the status bar is bottom-anchored.
	rendered := end - start
	for range maxRows - rendered {
		sb.WriteString("\n")
	}

	// Bottom separator
	sb.WriteString(styleSep.Render(sep))
	sb.WriteByte('\n')

	// Status bar: ● all · projects · repos              N items
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
	// No trailing newline — the last line must not advance the cursor to the next
	// row or the terminal scrolls, pushing the search bar (row 0) off screen.
	sb.WriteByte(' ')
	sb.WriteString(statusLeft)
	sb.WriteString(strings.Repeat(" ", statusPad))
	sb.WriteString(count)

	return sb.String()
}

// ── Entry point ───────────────────────────────────────────────────────────────

func Run(cs []candidates.Candidate, scores map[string]float64, ts tmux.TmuxState, switchOnly bool, cfg config.Config) (candidates.Candidate, error) {
	initStyles(cfg)
	m := newModel(cs, scores, ts, switchOnly)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return candidates.Candidate{}, err
	}
	if m, ok := final.(model); ok {
		return m.chosen, nil
	}
	return candidates.Candidate{}, nil
}
