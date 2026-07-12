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
)

type inputMode int

const (
	modeNormal   inputMode = iota
	modeURLInput           // Ctrl-G
	modeDestPicker
	modeCloneName    // rename on conflict
	modeNameInput    // Ctrl-N: typing tmp name
	modeCleanTmp     // Ctrl-X: search/select tmp projects
	modeConfirmClean // y/N confirmation before delete
	modeLoading
	modeError
)

type Result struct {
	Candidate candidates.Candidate
	Clone     *CloneRequest
	Tmp       *TmpRequest
}

type CloneRequest struct {
	URL    string
	Dest   string // target path chosen before clone
	Cloned string // actual cloned path, set after clone succeeds
}

type TmpRequest struct {
	Name string
	Path string // actual created path, set after mkdir succeeds
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
}

type scoredItem struct {
	base    baseItem
	score   float64
	matches []int // matched byte offsets into RelPath
}

type pendingItem struct {
	base     baseItem
	rawScore float64
	matches  []int
}

type model struct {
	all           []baseItem
	normFrec      map[string]float64
	filtered      []scoredItem
	tiQuery       textinput.Model // modeNormal search
	cursor        int
	view          viewMode
	switchOnly    bool
	width         int
	height        int
	result        Result
	ready         bool
	inputMode     inputMode
	tiURL         textinput.Model
	tiDest        textinput.Model
	tiCloneName   textinput.Model
	destFiltered  []scoredItem
	destCursor    int
	cloneDestDir  string // chosen parent dir (set when conflict detected)
	tiName        textinput.Model
	nameConflict  bool // modeNameInput: typed name already exists
	tmpPath       string
	showHelp      bool
	selected      map[string]bool // AbsPath of selected tmp candidates (modeCleanTmp)
	cleanFiltered []scoredItem    // search-filtered view of tmp candidates
	cleanCursor   int
	tiClean       textinput.Model
	inTmux        bool
	layoutBottom  bool // layout: "bottom" — status bar top, search bar bottom, lists reversed
	keys          keyMap
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

func cmdRunSelection(path, root string) tea.Cmd {
	return func() tea.Msg {
		return selectionDoneMsg{tmux.HandleSelection(path, root)}
	}
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

func newModel(cs []candidates.Candidate, scores map[string]float64, ts tmux.TmuxState, switchOnly bool, cfg config.Config, inTmux bool) model {
	all := make([]baseItem, 0, len(cs))
	for _, c := range cs {
		all = append(all, makeBaseItem(c, ts))
	}
	m := model{
		all:          all,
		normFrec:     normalizeScores(scores),
		switchOnly:   switchOnly,
		tmpPath:      cfg.TmpPath,
		layoutBottom: cfg.Layout == "bottom",
		keys:         buildKeyMap(cfg),
		selected:     make(map[string]bool),
		inTmux:       inTmux,
		spin:         spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		ctx:          context.Background(),
		tiQuery:      newTextInput("Search projects…"),
		tiURL:        newTextInput("https://github.com/owner/repo.git"),
		tiDest:       newTextInput("Search folders…"),
		tiCloneName:  newTextInput(""),
		tiClean:      newTextInput("Search…"),
		tiName:       newTextInput("Name (empty = auto)"),
	}
	_ = m.tiQuery.Focus()
	m.rebuildFiltered()
	return m
}

func makeBaseItem(c candidates.Candidate, ts tmux.TmuxState) baseItem {
	return baseItem{
		candidate: c,
		active:    candidates.CandidateActive(c, ts.Sessions, ts.Windows),
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

func invalidTmpName(s string) bool {
	return strings.Contains(s, "/") || strings.Contains(s, "..")
}

// moveCursor returns cur stepped by delta, clamped to [0, n).
func moveCursor(cur, delta, n int) int {
	next := cur + delta
	if next < 0 || next >= n {
		return cur
	}
	return next
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

func Run(cs []candidates.Candidate, scores map[string]float64, ts tmux.TmuxState, switchOnly bool, cfg config.Config, inTmux bool) (Result, error) {
	initStyles(cfg)
	m := newModel(cs, scores, ts, switchOnly, cfg, inTmux)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx = ctx
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

func RunDestPicker(cs []candidates.Candidate, cfg config.Config, inTmux bool, cloneURL string) (Result, error) {
	initStyles(cfg)
	m := newModel(cs, map[string]float64{}, tmux.TmuxState{}, false, cfg, inTmux)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx = ctx
	m.tiURL.SetValue(cloneURL)
	m.inputMode = modeDestPicker
	m.rebuildDestFiltered()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	if fm, ok := final.(model); ok {
		return fm.result, nil
	}
	return Result{}, nil
}
