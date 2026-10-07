package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/git"
	"github.com/rwilgaard/thop/internal/tmux"
)

type viewMode int

const (
	viewAll viewMode = iota
	viewProject
	viewRepo
	viewTmp
	viewOpen
)

type inputMode int

const (
	modeNormal   inputMode = iota
	modeURLInput           // Ctrl-G
	modeDestPicker
	modeCloneName    // rename on conflict
	modeNameInput    // Ctrl-T: typing tmp name
	modeCleanTmp     // Ctrl-X: search/select tmp projects
	modeConfirmClean // y/N confirmation before delete
	modeConfirmClose // y/N confirmation before killing a session or window
	modeLoading
	modeCloning // git clone running; esc cancels
	modeHelp
	modeSetup // first run: no scan roots configured
	modeNewProjRoot
	modeNewProjName
	modeError
)

type Result struct {
	Candidate candidates.Candidate
	Clone     *CloneRequest
	Tmp       *TmpRequest
}

type CloneRequest struct {
	URL     string
	Dest    string // target path chosen before clone
	Cloned  string // actual cloned path, set after clone succeeds
	Session string // session of the directory cloned into
}

type TmpRequest struct {
	Name    string
	Path    string // actual created path, set after mkdir succeeds
	Session string
}

type (
	selectionDoneMsg struct{ err error }
	cloneDoneMsg     struct {
		path string
		err  error
	}
)

type tmpCreatedMsg struct {
	path string
	err  error
}

type baseItem struct {
	candidate candidates.Candidate
	active    bool
	current   bool
	previous  bool
}

type scoredItem struct {
	base    baseItem
	score   float64
	matches []int // matched byte offsets into RelPath
}

// cloneFlow holds Ctrl-G clone state: URL entry, destination picking, rename
// on conflict.
type cloneFlow struct {
	tiURL        textinput.Model
	tiDest       textinput.Model
	tiName       textinput.Model // rename-on-conflict input
	destFiltered []scoredItem
	destCursor   int
	destDir      string // chosen parent dir (set when conflict detected)
	destSession  string
	shorthand    string
	cancel       context.CancelFunc // non-nil while a clone is running
	cancelled    bool
}

// tmpFlow holds Ctrl-T new-tmp-project state.
type tmpFlow struct {
	tiName   textinput.Model
	conflict bool // typed name already exists
}

// cleanFlow holds Ctrl-X tmp-deletion state.
type cleanFlow struct {
	tiQuery  textinput.Model
	filtered []scoredItem // search-filtered view of tmp candidates
	cursor   int
	selected map[string]bool // AbsPath of selected tmp candidates
}

// Setup enables the first-run dialog shown when no scan roots are configured.
type Setup struct {
	// Add saves path as a scan root and returns the resolved root with its
	// candidates.
	Add func(path string) (root string, cs []candidates.Candidate, err error)
}

// setupFlow holds first-run state.
type setupFlow struct {
	tiPath textinput.Model
	add    func(path string) (string, []candidates.Candidate, error)
	err    string
}

// newProjFlow holds Ctrl-N new-project state.
type newProjFlow struct {
	tiRoot   textinput.Model
	filtered []scoredItem // scan roots matching tiRoot
	cursor   int
	picked   bool // root came from the picker, so esc returns there
	root     string
	tiName   textinput.Model
	err      string
}

type model struct {
	all       []baseItem
	normFrec  map[string]float64
	filtered  []scoredItem
	tiQuery   textinput.Model // modeNormal search
	cursor    int
	view      viewMode
	width     int
	height    int
	result    Result
	ready     bool
	inputMode inputMode

	clone cloneFlow
	tmp   tmpFlow
	clean cleanFlow
	setup setupFlow

	newProj    newProjFlow
	paths      []string // scan roots
	missing    []string // scan roots not found on disk
	configFile string   // as shown to the user

	closeTarget baseItem
	ts          tmux.State
	killSession func(ts tmux.State, session string) error
	killWindow  func(session, window string) error
	loadState   func() tmux.State

	tmpPath       string
	inTmux        bool
	layoutBottom  bool // layout: "bottom" — status bar top, search bar bottom, lists reversed
	keys          keyMap
	st            styles
	spin          spinner.Model
	loadingText   string
	errMsg        string
	errReturnMode inputMode       // mode to restore when the error banner is dismissed
	ctx           context.Context // cancelled when the program exits; kills in-flight clones
}

func newTextInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = placeholder
	ti.CharLimit = 0
	// bubbles textinput truncates the placeholder to 1 rune when Width is
	// unset (0) — see placeholderView(). Size it to the placeholder itself
	// so the full text renders; inputRow still pads the row externally.
	ti.SetWidth(len([]rune(placeholder)))
	return ti
}

func cmdRunSelection(path, root, session string) tea.Cmd {
	return func() tea.Msg {
		return selectionDoneMsg{tmux.HandleSelection(path, root, session)}
	}
}

// addCandidates adds cs to the picker and re-resolves sessions, since a new
// name can collide with an existing one.
func (m *model) addCandidates(cs ...candidates.Candidate) {
	all := make([]candidates.Candidate, 0, len(m.all)+len(cs))
	all = append(all, cs...)
	for _, it := range m.all {
		all = append(all, it.candidate)
	}
	m.all = m.all[:0]
	for _, c := range candidates.Resolve(all) {
		m.all = append(m.all, makeBaseItem(c, m.ts))
	}
}

// sessionOf returns the resolved session of the candidate at path.
func (m model) sessionOf(path string) string {
	for _, it := range m.all {
		if it.candidate.AbsPath == path {
			session, _ := candidates.Target(it.candidate)
			return session
		}
	}
	return ""
}

func cmdClone(ctx context.Context, url, dest string) tea.Cmd {
	return func() tea.Msg {
		cloned, err := git.Clone(ctx, url, dest)
		return cloneDoneMsg{path: cloned, err: err}
	}
}

func cmdCreateTmp(tmpPath, name string) tea.Cmd {
	return func() tea.Msg {
		dest := filepath.Join(tmpPath, name)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return tmpCreatedMsg{err: err}
		}
		return tmpCreatedMsg{path: dest}
	}
}

func newModel(cs []candidates.Candidate, scores map[string]float64, ts tmux.State, switchOnly bool, cfg config.Config, inTmux bool) model {
	all := make([]baseItem, 0, len(cs))
	for _, c := range cs {
		all = append(all, makeBaseItem(c, ts))
	}
	m := model{
		all:          all,
		normFrec:     normalizeScores(scores),
		ts:           ts,
		killSession:  tmux.State.KillSession,
		killWindow:   tmux.KillWindow,
		loadState:    tmux.LoadState,
		tmpPath:      cfg.TmpPath,
		paths:        cfg.Paths,
		configFile:   tilde(cfg.File),
		layoutBottom: cfg.Layout == "bottom",
		keys:         buildKeyMap(cfg),
		st:           newStyles(cfg),
		inTmux:       inTmux,
		spin:         spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		ctx:          context.Background(),
		tiQuery:      newTextInput("Search projects…"),
		clone: cloneFlow{
			tiURL:  newTextInput("https://github.com/owner/repo.git"),
			tiDest: newTextInput("Search folders…"),
			tiName: newTextInput(""),

			shorthand: cfg.CloneShorthand,
		},
		tmp: tmpFlow{
			tiName: newTextInput("Name (empty = auto)"),
		},
		newProj: newProjFlow{
			tiRoot: newTextInput("Search roots…"),
			tiName: newTextInput("Name"),
		},
		clean: cleanFlow{
			tiQuery:  newTextInput("Search…"),
			selected: make(map[string]bool),
		},
	}
	for _, p := range cfg.Paths {
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			m.missing = append(m.missing, p)
		}
	}
	if switchOnly {
		m.view = viewOpen
	}
	_ = m.tiQuery.Focus()
	m.rebuildFiltered()
	return m
}

// refreshTmux reloads tmux state after a kill so open/current labels and
// ordering are right.
func (m *model) refreshTmux() {
	m.ts = m.loadState()
	for i, item := range m.all {
		m.all[i] = makeBaseItem(item.candidate, m.ts)
	}
}

func makeBaseItem(c candidates.Candidate, ts tmux.State) baseItem {
	return baseItem{
		candidate: c,
		active:    candidates.Active(c, ts),
		current:   candidates.Current(c, ts),
		previous:  candidates.Previous(c, ts),
	}
}

// tmpItems returns all tmp candidates from m.all (derived, not stored).
func (m model) tmpItems() []baseItem {
	var out []baseItem
	for _, item := range m.all {
		if item.candidate.IsTmp {
			out = append(out, item)
		}
	}
	return out
}

// moveCursor returns cur stepped by delta, wrapping at the ends of [0, n).
func moveCursor(cur, delta, n int) int {
	if n == 0 {
		return 0
	}
	return ((cur+delta)%n + n) % n
}

func pageCursor(cur, delta, n int) int {
	return max(0, min(n-1, cur+delta))
}

// maxRows is the list height: frame minus search, two separators and status.
func (m model) maxRows() int {
	height := m.height
	if height == 0 {
		height = 24
	}
	rows := max(5, height-4)
	if len(m.missing) > 0 {
		rows-- // warning row
	}
	return rows
}

// tilde shortens a path under the home dir to "~/…" for display.
func tilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}

func (m model) pageStep(dir int) int {
	return m.visualStep(dir) * m.maxRows()
}

// visualStep maps a visual direction (-1 up, +1 down) to an index delta.
// Bottom layout renders lists reversed, so the mapping flips.
func (m model) visualStep(dir int) int {
	if m.layoutBottom {
		return -dir
	}
	return dir
}

func (m model) Init() tea.Cmd { return nil }

// runProgram runs m to completion and extracts its Result, wiring up the
// context that cancels in-flight clones when the program exits.
func runProgram(m model) (Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx = ctx
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return Result{}, err
	}
	if fm, ok := final.(model); ok {
		return fm.result, nil
	}
	return Result{}, nil
}

// Run shows the picker. A non-nil setup opens the first-run dialog first.
func Run(cs []candidates.Candidate, scores map[string]float64, ts tmux.State, switchOnly bool, cfg config.Config, inTmux bool, setup *Setup) (Result, error) {
	m := newModel(cs, scores, ts, switchOnly, cfg, inTmux)
	if setup != nil {
		_ = m.openSetup(*setup)
	}
	return runProgram(m)
}

// RunDestPicker shows the clone destination picker for cloneURL. A non-nil
// setup opens the first-run dialog first.
func RunDestPicker(cs []candidates.Candidate, cfg config.Config, inTmux bool, cloneURL string, setup *Setup) (Result, error) {
	m := newModel(cs, map[string]float64{}, tmux.State{}, false, cfg, inTmux)
	m.tiQuery.Blur()
	m.clone.tiURL.SetValue(cloneURL)
	if setup != nil {
		_ = m.openSetup(*setup)
	} else {
		_ = m.openDestPicker()
	}
	return runProgram(m)
}
