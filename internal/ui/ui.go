package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/git"
	"github.com/rwilgaard/thop/internal/tmux"
	"github.com/sahilm/fuzzy"
)

const activeLabel = "● open "
const leftPad = " "

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Quit     key.Binding
	Help     key.Binding
	Clone    key.Binding
	NewTmp   key.Binding
	CleanTmp key.Binding
	All      key.Binding
	Projects key.Binding
	Repos    key.Binding
	Tmp      key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Quit},
		{k.Clone, k.NewTmp, k.CleanTmp},
		{k.All, k.Projects, k.Repos, k.Tmp},
	}
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "ctrl+k"), key.WithHelp("↑/ctrl-k", "move up")),
	Down:     key.NewBinding(key.WithKeys("down", "ctrl+j"), key.WithHelp("↓/ctrl-j", "move down")),
	Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open selected")),
	Quit:     key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Clone:    key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("ctrl-g", "clone repo")),
	NewTmp:   key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl-n", "new tmp project")),
	CleanTmp: key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl-x", "delete tmp")),
	All:      key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl-a", "show all")),
	Projects: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl-p", "projects only")),
	Repos:    key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl-r", "repos only")),
	Tmp:      key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl-t", "tmp only")),
}

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
	viewTmp
)

type inputMode int

const (
	modeNormal       inputMode = iota
	modeURLInput               // Ctrl-G
	modeDestPicker             // pick clone destination directory
	modeCloneName              // rename on conflict
	modeNameInput              // Ctrl-N: typing tmp name
	modeCleanTmp               // Ctrl-X: search/select tmp projects
	modeConfirmClean           // y/N confirmation before delete
	modeLoading                // async op in progress
	modeError                  // show error, any key quits
)

type Result struct {
	Candidate candidates.Candidate
	Clone     *CloneRequest
	Tmp       *TmpRequest
}

type CloneRequest struct {
	URL    string
	Dest   string   // target path chosen before clone
	Cloned string   // actual cloned path, set after clone succeeds
}

type TmpRequest struct {
	Name string
	Path string   // actual created path, set after mkdir succeeds
}

// async operation result messages
type selectionDoneMsg struct{ err error }
type cloneDoneMsg struct {
	path string
	err  error
}
type tmpCreatedMsg struct {
	path string
	err  error
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
	tiName         textinput.Model // modeNameInput
	nameConflict   bool            // modeNameInput: typed name already exists
	tmpPath        string
	helpModel      help.Model
	selected       map[string]bool // AbsPath of selected tmp candidates (modeCleanTmp)
	cleanAll       []baseItem      // all tmp candidates for clean mode
	cleanFiltered  []baseItem      // search-filtered view of cleanAll
	cleanCursor    int
	tiClean        textinput.Model // modeCleanTmp search
	inTmux         bool
	loadingText    string
	errMsg         string
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

func newHelpModel(c config.Colors) help.Model {
	h := help.New()
	keyStyle := lipgloss.NewStyle().Bold(true)
	if c.HelpKeyColor != "" {
		keyStyle = keyStyle.Foreground(lipgloss.Color(c.HelpKeyColor))
	}
	descStyle := lipgloss.NewStyle().Faint(true)
	if c.HelpDescColor != "" {
		descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(c.HelpDescColor))
	}
	h.Styles.FullKey = keyStyle
	h.Styles.ShortKey = keyStyle
	h.Styles.FullDesc = descStyle
	h.Styles.ShortDesc = descStyle
	return h
}

func newTextInput(prompt string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.CharLimit = 0
	return ti
}

func cmdRunSelection(path, root string) tea.Cmd {
	return func() tea.Msg {
		return selectionDoneMsg{tmux.HandleSelection(path, root)}
	}
}

func cmdClone(url, dest string) tea.Cmd {
	return func() tea.Msg {
		cloned, err := git.Clone(url, dest)
		return cloneDoneMsg{path: cloned, err: err}
	}
}

func cmdCreateTmp(tmpPath, name string) tea.Cmd {
	return func() tea.Msg {
		if strings.Contains(name, "/") || strings.Contains(name, "..") {
			return tmpCreatedMsg{err: fmt.Errorf("name must not contain path separators or '..'")}
		}
		dest := filepath.Join(tmpPath, name)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return tmpCreatedMsg{err: err}
		}
		return tmpCreatedMsg{path: dest}
	}
}


func newModel(cs []candidates.Candidate, scores map[string]float64, ts tmux.TmuxState, switchOnly bool, tmpPath string, colors config.Colors) model {
	all := make([]baseItem, 0, len(cs))
	for _, c := range cs {
		all = append(all, makeBaseItem(c, ts))
	}
	m := model{
		all:         all,
		normFrec:    normalizeScores(scores),
		switchOnly:  switchOnly,
		tmpPath:     tmpPath,
		helpModel:   newHelpModel(colors),
		selected:    make(map[string]bool),
		tiQuery:     newTextInput(""),
		tiURL:       newTextInput(""),
		tiDest:      newTextInput(""),
		tiCloneName: newTextInput(""),
		tiClean:     newTextInput(""),
		tiName:      newTextInput(""),
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
			if item.candidate.IsTmp || item.candidate.IsRepo {
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
			if item.candidate.IsTmp || item.candidate.IsRepo {
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

func (m *model) rebuildCleanFiltered() {
	query := m.tiClean.Value()
	if query == "" {
		m.cleanFiltered = m.cleanAll
	} else {
		keys := make([]string, len(m.cleanAll))
		for i, item := range m.cleanAll {
			keys[i] = item.candidate.RelPath
		}
		matches := fuzzy.Find(query, keys)
		m.cleanFiltered = nil
		for _, match := range matches {
			m.cleanFiltered = append(m.cleanFiltered, m.cleanAll[match.Index])
		}
	}
	if m.cleanCursor >= len(m.cleanFiltered) {
		m.cleanCursor = 0
	}
}

func (m *model) matchesView(item baseItem) bool {
	switch m.view {
	case viewProject:
		return !item.candidate.IsRepo && !item.candidate.IsTmp
	case viewRepo:
		return item.candidate.IsRepo
	case viewTmp:
		return item.candidate.IsTmp
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
		m.helpModel.SetWidth(msg.Width)
	case selectionDoneMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.inputMode = modeError
			return m, nil
		}
		return m, tea.Quit
	case cloneDoneMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.inputMode = modeError
			return m, nil
		}
		m.result.Clone.Cloned = msg.path
		if m.inTmux {
			m.loadingText = "opening…"
			return m, cmdRunSelection(msg.path, "")
		}
		return m, tea.Quit
	case tmpCreatedMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.inputMode = modeError
			return m, nil
		}
		m.result.Tmp.Path = msg.path
		if m.inTmux {
			m.loadingText = "opening…"
			return m, cmdRunSelection(msg.path, m.tmpPath)
		}
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch m.inputMode {
		case modeURLInput:
			return m.updateURLInput(msg)
		case modeDestPicker:
			return m.updateDestPicker(msg)
		case modeCloneName:
			return m.updateCloneName(msg)
		case modeNameInput:
			return m.updateNameInput(msg)
		case modeCleanTmp:
			return m.updateCleanTmp(msg)
		case modeConfirmClean:
			return m.updateConfirmClean(msg)
		case modeLoading:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		case modeError:
			return m.updateError(msg)
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
	case modeNameInput:
		m.tiName, cmd = m.tiName.Update(msg)
	case modeCleanTmp:
		m.tiClean, cmd = m.tiClean.Update(msg)
	case modeLoading, modeError:
		// no active textinput
	}
	return m, cmd
}

func (m model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.helpModel.ShowAll {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "?", "esc":
			m.helpModel.ShowAll = false
			return m, m.tiQuery.Focus()
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Enter):
		if len(m.filtered) > 0 {
			c := m.filtered[m.cursor].base.candidate
			m.result.Candidate = c
			if m.inTmux {
				m.loadingText = "opening…"
				m.inputMode = modeLoading
				return m, cmdRunSelection(c.AbsPath, c.Root)
			}
		}
		return m, tea.Quit
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case key.Matches(msg, keys.All):
		m.view, m.cursor = viewAll, 0
		m.rebuildFiltered()
	case key.Matches(msg, keys.Projects):
		m.view, m.cursor = viewProject, 0
		m.rebuildFiltered()
	case key.Matches(msg, keys.Repos):
		m.view, m.cursor = viewRepo, 0
		m.rebuildFiltered()
	case key.Matches(msg, keys.Tmp):
		m.view, m.cursor = viewTmp, 0
		m.rebuildFiltered()
	case key.Matches(msg, keys.Clone):
		m.tiQuery.Blur()
		m.inputMode = modeURLInput
		m.tiURL.SetValue("")
		return m, m.tiURL.Focus()
	case key.Matches(msg, keys.NewTmp):
		m.tiQuery.Blur()
		m.tiName.SetValue("")
		m.inputMode = modeNameInput
		return m, m.tiName.Focus()
	case key.Matches(msg, keys.CleanTmp):
		m.cleanAll = nil
		for _, item := range m.all {
			if item.candidate.IsTmp {
				m.cleanAll = append(m.cleanAll, item)
			}
		}
		m.cleanCursor = 0
		m.selected = make(map[string]bool)
		m.tiClean.SetValue("")
		m.rebuildCleanFiltered()
		m.tiQuery.Blur()
		m.inputMode = modeCleanTmp
		return m, m.tiClean.Focus()
	case key.Matches(msg, keys.Help):
		m.tiQuery.Blur()
		m.helpModel.ShowAll = true
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
			m.tiDest.Blur()
			m.loadingText = "cloning…"
			m.inputMode = modeLoading
			return m, cmdClone(m.tiURL.Value(), fullDest)
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
			dest := filepath.Join(m.cloneDestDir, m.tiCloneName.Value())
			m.result.Clone = &CloneRequest{
				URL:  m.tiURL.Value(),
				Dest: dest,
			}
			m.tiCloneName.Blur()
			m.loadingText = "cloning…"
			m.inputMode = modeLoading
			return m, cmdClone(m.tiURL.Value(), dest)
		}
	default:
		var cmd tea.Cmd
		m.tiCloneName, cmd = m.tiCloneName.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateNameInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.tiName.Blur()
		m.nameConflict = false
		m.inputMode = modeNormal
		return m, m.tiQuery.Focus()
	case "enter":
		name := m.tiName.Value()
		if strings.Contains(name, "/") || strings.Contains(name, "..") {
			m.nameConflict = true
			return m, nil
		}
		if name != "" && m.tmpPath != "" && !m.nameConflict {
			if _, err := os.Stat(filepath.Join(m.tmpPath, name)); err == nil {
				m.nameConflict = true
				return m, nil
			}
		}
		if name == "" {
			name = "tmp-" + time.Now().Format("20060102-150405")
		}
		m.result.Tmp = &TmpRequest{Name: name}
		m.tiName.Blur()
		m.loadingText = "creating…"
		m.inputMode = modeLoading
		return m, cmdCreateTmp(m.tmpPath, name)
	default:
		prev := m.tiName.Value()
		var cmd tea.Cmd
		m.tiName, cmd = m.tiName.Update(msg)
		if m.tiName.Value() != prev {
			m.nameConflict = false
		}
		return m, cmd
	}
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

func (m model) updateCleanTmp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.tiClean.Blur()
		m.inputMode = modeNormal
		m.selected = make(map[string]bool)
		return m, m.tiQuery.Focus()
	case "up", "ctrl+k":
		if m.cleanCursor > 0 {
			m.cleanCursor--
		}
	case "down", "ctrl+j":
		if m.cleanCursor < len(m.cleanFiltered)-1 {
			m.cleanCursor++
		}
	case "space":
		if m.cleanCursor < len(m.cleanFiltered) {
			path := m.cleanFiltered[m.cleanCursor].candidate.AbsPath
			if m.selected[path] {
				delete(m.selected, path)
			} else {
				m.selected[path] = true
			}
		}
	case "enter":
		if len(m.cleanAll) > 0 {
			m.tiClean.Blur()
			m.inputMode = modeConfirmClean
		}
	default:
		prev := m.tiClean.Value()
		var cmd tea.Cmd
		m.tiClean, cmd = m.tiClean.Update(msg)
		if m.tiClean.Value() != prev {
			m.cleanCursor = 0
			m.rebuildCleanFiltered()
		}
		return m, cmd
	}
	return m, nil
}

func (m model) updateConfirmClean(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.inputMode = modeCleanTmp
		return m, m.tiClean.Focus()
	default:
		if msg.Key().Text == "y" {
			toDelete := m.selected
			if len(toDelete) == 0 {
				toDelete = make(map[string]bool, len(m.cleanFiltered))
				for _, item := range m.cleanFiltered {
					toDelete[item.candidate.AbsPath] = true
				}
			}
			var kept []baseItem
			var errMsgs []string
			for _, item := range m.all {
				c := item.candidate
				if c.IsTmp && toDelete[c.AbsPath] {
					if err := os.RemoveAll(c.AbsPath); err != nil {
						kept = append(kept, item)
						errMsgs = append(errMsgs, err.Error())
					}
				} else {
					kept = append(kept, item)
				}
			}
			m.all = kept
			m.selected = make(map[string]bool)
			m.rebuildFiltered()
			if len(errMsgs) > 0 {
				m.errMsg = strings.Join(errMsgs, "\n")
				m.inputMode = modeError
				return m, nil
			}
			m.inputMode = modeNormal
			return m, m.tiQuery.Focus()
		}
		m.inputMode = modeCleanTmp
		return m, m.tiClean.Focus()
	}
}

func (m model) updateError(_ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m, tea.Quit
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
	case modeLoading:
		searchLine = leftPad + styleSep.Render(m.loadingText)
	case modeError:
		hints, hintsW := keyHints([][2]string{{"any key", "quit"}})
		label := styleSep.Render("⚠  ")
		labelW := lipgloss.Width(label)
		lines := strings.SplitN(m.errMsg, "\n", 2)
		first := lines[0]
		firstW := lipgloss.Width(first)
		pad := max(1, width-2-labelW-firstW-hintsW)
		searchLine = leftPad + label + first + strings.Repeat(" ", pad) + hints
	case modeURLInput:
		hints, hintsW := keyHints([][2]string{{"enter", "Confirm"}, {"esc", "Cancel"}})
		label := stylePrompt.Render("clone url: ")
		tiView := m.tiURL.View()
		tiW := lipgloss.Width(tiView)
		pad := max(1, width-2-lipgloss.Width(label)-tiW-hintsW)
		searchLine = leftPad + label + tiView + strings.Repeat(" ", pad) + hints
	case modeNameInput:
		hints, hintsW := keyHints([][2]string{{"enter", "create"}, {"esc", "cancel"}})
		label := stylePrompt.Render("tmp name: ")
		tiView := m.tiName.View()
		tiW := lipgloss.Width(tiView)
		var hint string
		var hintW int
		switch {
		case m.nameConflict && (strings.Contains(m.tiName.Value(), "/") || strings.Contains(m.tiName.Value(), "..")):
			hint = styleSep.Render(" (invalid name)")
			hintW = lipgloss.Width(hint)
		case m.nameConflict:
			hint = styleSep.Render(" (exists — enter to open)")
			hintW = lipgloss.Width(hint)
		case m.tiName.Value() == "":
			hint = styleSep.Render(" (auto)")
			hintW = lipgloss.Width(hint)
		}
		pad := max(1, width-2-lipgloss.Width(label)-tiW-hintW-hintsW)
		searchLine = leftPad + label + tiView + hint + strings.Repeat(" ", pad) + hints
	case modeCleanTmp:
		hints, hintsW := keyHints([][2]string{{"space", "toggle"}, {"enter", "confirm"}, {"esc", "cancel"}})
		label := stylePrompt.Render("delete tmp: ")
		tiView := m.tiClean.View()
		tiW := lipgloss.Width(tiView)
		pad := max(1, width-2-lipgloss.Width(label)-tiW-hintsW)
		searchLine = leftPad + label + tiView + strings.Repeat(" ", pad) + hints
	case modeConfirmClean:
		nSel := len(m.selected)
		var promptText string
		if nSel > 0 {
			promptText = fmt.Sprintf("delete %d tmp project(s)?", nSel)
		} else {
			promptText = fmt.Sprintf("delete %d tmp project(s)?", len(m.cleanFiltered))
		}
		yn := styleSep.Render(" [y/N]")
		searchLine = leftPad + stylePrompt.Render(promptText) + yn
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
		label := stylePrompt.Render("❯ ")
		tiView := m.tiQuery.View()
		if m.helpModel.ShowAll {
			searchLine = leftPad + label + tiView
		} else {
			helpHint := m.helpModel.View(keys)
			helpHintW := lipgloss.Width(helpHint)
			pad := max(1, width-2-lipgloss.Width(label)-lipgloss.Width(tiView)-helpHintW)
			searchLine = leftPad + label + tiView + strings.Repeat(" ", pad) + helpHint
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
		start := 0
		if m.cleanCursor >= maxRows {
			start = m.cleanCursor - maxRows + 1
		}
		end := min(start+maxRows, len(m.cleanFiltered))
		for i := start; i < end; i++ {
			item := m.cleanFiltered[i]
			prefix := leftPad
			if m.selected[item.candidate.AbsPath] {
				prefix = "✓"
			}
			if i == m.cleanCursor {
				plain := ansiEscape.ReplaceAllString(item.display, "")
				pad := max(1, width-1-lipgloss.Width(plain)-1)
				sb.WriteString(styleSelected.Render(prefix + plain + strings.Repeat(" ", pad) + " "))
				sb.WriteByte('\n')
			} else {
				sb.WriteString(prefix + item.display + "\n")
			}
		}
		for range maxRows - (end - start) {
			sb.WriteByte('\n')
		}
	case m.inputMode == modeConfirmClean:
		var toDelete []baseItem
		if len(m.selected) > 0 {
			for _, item := range m.cleanAll {
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
			sb.WriteString(leftPad + toDelete[rows-1].display + "\n")
			rows++
		}
		for rows < maxRows {
			sb.WriteByte('\n')
			rows++
		}
	case m.inputMode == modeDestPicker:
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

func Run(cs []candidates.Candidate, scores map[string]float64, ts tmux.TmuxState, switchOnly bool, cfg config.Config, inTmux bool) (Result, error) {
	initStyles(cfg)
	m := newModel(cs, scores, ts, switchOnly, cfg.TmpPath, cfg.Colors)
	m.inTmux = inTmux
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

func RunDestPicker(cs []candidates.Candidate, cfg config.Config, inTmux bool, cloneURL string) (string, error) {
	initStyles(cfg)
	m := newModel(cs, map[string]float64{}, tmux.TmuxState{}, false, "", cfg.Colors)
	m.inTmux = inTmux
	m.tiURL.SetValue(cloneURL)
	m.inputMode = modeDestPicker
	m.rebuildDestFiltered()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if fm, ok := final.(model); ok && fm.result.Clone != nil {
		return fm.result.Clone.Cloned, nil
	}
	return "", nil
}
