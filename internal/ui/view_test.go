package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
)

func TestView(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/golang/foo", RelPath: "golang/foo", IsRepo: true},
		{AbsPath: "/p/work", RelPath: "work", IsRepo: false},
	}
	scores := map[string]float64{"/p/golang/foo": 1.0}
	ts := tmux.State{
		Sessions: map[string]bool{"golang": true},
		Windows:  map[string]bool{"golang/foo": true},
	}
	m := newModel(cs, scores, ts, false, config.Config{}, false)
	m.width = 80
	m.height = 24
	m.ready = true

	out := m.View().Content

	checks := []struct {
		want string
		desc string
	}{
		{"❯", "prompt glyph"},
		{"Help", "help hint"},
		{"Open", "open hint"},
		// full placeholder can't match: textinput renders the first rune as a
		// separate cursor-styled ANSI run
		{"earch projects…", "search placeholder"},
		{"golang/foo", "first item"},
		{"work", "second item"},
		{"", "repo icon"},
		{"󰉋", "project icon"},
		{"● open", "active indicator"},
		{"Filter", "status bar filter heading"},
		{"All", "status bar active view"},
		{"1/2", "cursor position"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("View() missing %s: %q not found", c.desc, c.want)
		}
	}
}

func TestView_emptyState(t *testing.T) {
	m := testModel()
	m = sized(m, 80, 24)

	out := m.View().Content
	if !strings.Contains(out, "Nothing here") {
		t.Errorf("empty pool should say 'Nothing here': %q", out)
	}

	m.all = []baseItem{{candidate: cand.Candidate{AbsPath: "/p/foo", RelPath: "foo"}}}
	m.tiQuery.SetValue("zzz")
	m.rebuildFiltered()
	out = m.View().Content
	if !strings.Contains(out, "No matches") {
		t.Errorf("query without hits should say 'No matches': %q", out)
	}
}

func TestView_longRowsFitWidth(t *testing.T) {
	long := strings.Repeat("very-long-name-", 10)
	m := testModel()
	m.all = []baseItem{
		{candidate: cand.Candidate{AbsPath: "/p/a", RelPath: long}, active: true, current: true},
		{candidate: cand.Candidate{AbsPath: "/p/b", RelPath: long + "b"}},
	}
	m.rebuildFiltered()
	m = sized(m, 40, 12)

	out := m.View().Content
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("line %d is %d cells wide, frame is %d", i, w, m.width)
		}
	}
	if !strings.Contains(out, "current") {
		t.Error("truncated row lost its label")
	}
}

func TestRenderName_highlights(t *testing.T) {
	base := lipgloss.NewStyle()
	match := lipgloss.NewStyle().Bold(true)
	out := renderName("abc", []int{0, 2}, base, match)
	// matched runes wrapped in bold, unmatched not. lipgloss v2 resets with
	// "\x1b[m" rather than "\x1b[0m", so match on the bold-open + reset pair.
	if !strings.Contains(out, "\x1b[1ma\x1b[m") || !strings.Contains(out, "\x1b[1mc\x1b[m") {
		t.Errorf("matched runes not bold: %q", out)
	}
	if strings.Contains(out, "\x1b[1mb") {
		t.Errorf("unmatched rune styled: %q", out)
	}
}

func TestTruncateName(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		matches     []int
		maxW        int
		want        string
		wantMatches []int
	}{
		{"fits", "abc", []int{0, 2}, 3, "abc", []int{0, 2}},
		{"cut", "abcdef", []int{0, 4}, 4, "abc…", []int{0}},
		{"multibyte", "æøåabc", []int{0, 2}, 3, "æø…", []int{0, 2}},
		{"no room", "abc", nil, 0, "…", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotMatches := truncateName(tt.in, tt.matches, tt.maxW)
			if got != tt.want {
				t.Errorf("name = %q, want %q", got, tt.want)
			}
			if len(gotMatches) != len(tt.wantMatches) {
				t.Fatalf("matches = %v, want %v", gotMatches, tt.wantMatches)
			}
			for i := range gotMatches {
				if gotMatches[i] != tt.wantMatches[i] {
					t.Errorf("matches = %v, want %v", gotMatches, tt.wantMatches)
				}
			}
		})
	}
}

func TestStatusBar_modes(t *testing.T) {
	m := testModel()
	m = sized(m, 100, 24)

	// normal, wide: spelled+bracketed keys with dot separators, matching the
	// other hint rows
	out := m.View().Content
	for _, w := range []string{"Filter", "All", "Projects", "Repos", "Tmp", "Open", "•", "0 items"} {
		if !strings.Contains(out, w) {
			t.Errorf("wide normal status missing %q: %q", w, out)
		}
	}
	if status := strings.Split(out, "\n")[m.maxRows()+3]; strings.Contains(status, "<") {
		t.Errorf("unbound filter keys should not render: %q", status)
	}

	// bound direct-jump keys show up; narrow falls back to compact carets, no
	// dots, so it fits
	m.keys = buildKeyMap(config.Config{Keymap: map[string][]string{"all": {"ctrl+a"}}})
	if wide := m.View().Content; !strings.Contains(wide, "<ctrl-a>") {
		t.Errorf("bound filter key should render: %q", wide)
	}
	m.width = 60
	narrow := m.View().Content
	if !strings.Contains(narrow, "^A") || strings.Contains(narrow, "<ctrl-a>") || strings.Contains(narrow, "•") {
		t.Errorf("narrow normal status should use compact caret fallback: %q", narrow)
	}
	for _, line := range strings.Split(narrow, "\n") {
		if lipgloss.Width(line) > 60 {
			t.Errorf("narrow status line exceeds width 60 (got %d): %q", lipgloss.Width(line), line)
		}
	}
	m.width = 100

	// the active filter must render distinctly from inactive ones: switching
	// the active view changes the bar. Guards against every label rendering
	// the same (dropping the m.view==t.mode branch).
	m.view = viewAll
	allActive := m.View().Content
	m.view = viewProject
	projActive := m.View().Content
	if allActive == projActive {
		t.Error("active filter not styled distinctly: switching active view produced identical output")
	}
	m.view = viewAll

	// dest picker: clone URL + own count, no tabs
	m.inputMode = modeDestPicker
	m.clone.tiURL.SetValue("https://x/y.git")
	m.rebuildDestFiltered()
	out = m.View().Content
	if !strings.Contains(out, "https://x/y.git") {
		t.Errorf("dest picker should show clone URL: %q", out)
	}
	if strings.Contains(out, "Projects") {
		t.Errorf("dest picker should not show view tabs: %q", out)
	}

	// clean tmp: N selected + own count
	m.inputMode = modeCleanTmp
	m.all = []baseItem{{candidate: cand.Candidate{AbsPath: "/t/a", RelPath: "a", IsTmp: true}}}
	m.rebuildCleanFiltered()
	m.clean.selected = map[string]bool{"/t/a": true}
	out = m.View().Content
	if !strings.Contains(out, "1 selected") || !strings.Contains(out, "1/1") {
		t.Errorf("clean mode should show selection and count: %q", out)
	}
}

func TestTextInputPrompts(t *testing.T) {
	// textinput's own "> " prompt would render after ours
	m := testModel()
	inputs := map[string]string{
		"tiQuery":        m.tiQuery.Prompt,
		"clone.tiURL":    m.clone.tiURL.Prompt,
		"clone.tiDest":   m.clone.tiDest.Prompt,
		"clone.tiName":   m.clone.tiName.Prompt,
		"clean.tiQuery":  m.clean.tiQuery.Prompt,
		"tmp.tiName":     m.tmp.tiName.Prompt,
		"setup.tiPath":   m.setup.tiPath.Prompt,
		"newProj.tiRoot": m.newProj.tiRoot.Prompt,
		"newProj.tiName": m.newProj.tiName.Prompt,
	}
	for name, prompt := range inputs {
		if prompt != "" {
			t.Errorf("%s.Prompt = %q, want empty", name, prompt)
		}
	}
}

func TestInlinePrompts(t *testing.T) {
	tests := []struct {
		mode inputMode
		want []string
	}{
		{modeDestPicker, []string{"Clone › Destination ❯", "Select", "Back"}},
		{modeCleanTmp, []string{"Delete tmp projects ❯", "Select", "Delete", "Cancel"}},
		{modeNewProjRoot, []string{"New project › Root ❯", "Select", "Back"}},
	}
	for _, tt := range tests {
		m := sized(testModel(), 100, 24)
		m.inputMode = tt.mode
		out := plain(m)
		for _, w := range tt.want {
			if !strings.Contains(out, w) {
				t.Errorf("mode %v: missing %q", tt.mode, w)
			}
		}
	}
}

func TestInputRow_hintsFitWhole(t *testing.T) {
	cs := []cand.Candidate{
		{AbsPath: "/p/open", RelPath: "open"},
		{AbsPath: "/t/scratch", RelPath: "scratch", IsTmp: true},
	}
	ts := tmux.State{Sessions: map[string]bool{"open": true}}
	actions := []string{"Open", "Help", "Filter", "Close", "Clone", "Delete", "Select", "Cancel", "Back"}
	for _, mode := range []inputMode{modeNormal, modeCleanTmp, modeDestPicker} {
		for _, width := range []int{30, 40, 60, 100} {
			t.Run(fmt.Sprintf("mode %d width %d", mode, width), func(t *testing.T) {
				m := newModel(cs, map[string]float64{"/p/open": 1}, ts, false, config.Config{}, false)
				m.rebuildCleanFiltered()
				m.rebuildDestFiltered()
				m.inputMode = mode
				m.width, m.height, m.ready = width, 12, true
				line := ansi.Strip(strings.Split(m.View().Content, "\n")[0])
				if w := lipgloss.Width(line); w > width {
					t.Errorf("row is %d cells, frame is %d: %q", w, width, line)
				}
				// every "<key>" must be followed by its whole action word
				for _, part := range strings.Split(line, "<")[1:] {
					_, action, _ := strings.Cut(part, "> ")
					action = strings.TrimSpace(action)
					if !slices.Contains(actions, action) {
						t.Errorf("clipped hint %q in %q", action, line)
					}
				}
			})
		}
	}

	m := newModel(cs, map[string]float64{"/p/open": 1}, ts, false, config.Config{}, false)
	if line := ansi.Strip(m.searchLine(120)); !strings.Contains(line, "Close") {
		t.Errorf("open row should offer Close: %q", line)
	}
	m.cursor = 1
	if line := ansi.Strip(m.searchLine(120)); strings.Contains(line, "Close") {
		t.Errorf("row that isn't open should not offer Close: %q", line)
	}
}

func TestCollidingRows(t *testing.T) {
	cs := cand.Resolve([]cand.Candidate{
		{AbsPath: "/code/alpha", Root: "/code", RelPath: "alpha"},
		{AbsPath: "/work/alpha", Root: "/work", RelPath: "alpha"},
		{AbsPath: "/code/beta", Root: "/code", RelPath: "beta"},
	})
	ts := tmux.State{Sessions: map[string]bool{"alpha@work": true}}
	m := newModel(cs, map[string]float64{}, ts, false, config.Config{}, false)
	m = sized(m, 60, 12)

	rows := map[string]string{}
	for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		for _, root := range []string{"/code", "/work"} {
			if strings.Contains(line, "alpha") && strings.Contains(line, root) {
				rows[root] = line
			}
		}
		if strings.Contains(line, "beta") && strings.Contains(line, "/code") {
			t.Errorf("unique name should not show its root: %q", line)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("both alpha rows should name their root, got %v", rows)
	}
	if strings.Contains(rows["/code"], "open") || !strings.Contains(rows["/work"], "open") {
		t.Errorf("only the /work alpha is open: %v", rows)
	}

	m.width = 20
	for i, line := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(line); w > 20 {
			t.Errorf("line %d is %d cells wide, frame is 20", i, w)
		}
	}
}

func TestMissingRoots(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	ok, gone := filepath.Join(root, "ok"), filepath.Join(root, "wrok")
	var cs []cand.Candidate
	for i := range 30 {
		name := fmt.Sprintf("p%02d", i)
		cs = append(cs, cand.Candidate{AbsPath: filepath.Join(ok, name), Root: ok, RelPath: name})
	}

	t.Run("all roots present", func(t *testing.T) {
		m := newModel(cs, map[string]float64{}, tmux.State{}, false, config.Config{Paths: []string{ok}}, false)
		m = sized(m, 80, 12)
		if out := m.View().Content; strings.Contains(out, "Not found") {
			t.Errorf("unexpected warning: %q", out)
		}
		if got := m.maxRows(); got != 8 {
			t.Errorf("maxRows = %d, want 8", got)
		}
	})

	for _, width := range []int{30, 80, 200} {
		t.Run(fmt.Sprintf("one missing, width %d", width), func(t *testing.T) {
			cfg := config.Config{Paths: []string{ok, gone}, File: "/etc/thop/config.yaml"}
			m := newModel(cs, map[string]float64{}, tmux.State{}, false, cfg, false)
			m.width, m.height, m.ready = width, 12, true
			if got := m.maxRows(); got != 7 {
				t.Errorf("maxRows = %d, want 7 (one row given to the warning)", got)
			}
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != 12 {
				t.Errorf("frame has %d lines, want 12", len(lines))
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w > width {
					t.Errorf("line %d is %d cells wide, frame is %d", i, w, width)
				}
			}
			warn := ansi.Strip(lines[9])
			if !strings.Contains(warn, "Not found") {
				t.Errorf("warning row missing above the bottom separator: %q", warn)
			}
			if width == 200 && !strings.Contains(warn, gone+" — check /etc/thop/config.yaml") {
				t.Errorf("warning should name the path and config file: %q", warn)
			}
		})
	}
}

func TestMissingRoots_bottomLayout(t *testing.T) {
	root := t.TempDir()
	cs := []cand.Candidate{{AbsPath: filepath.Join(root, "a"), Root: root, RelPath: "a"}}
	cfg := config.Config{Paths: []string{root, filepath.Join(root, "gone")}, Layout: "bottom"}
	m := newModel(cs, map[string]float64{}, tmux.State{}, false, cfg, false)
	m = sized(m, 80, 12)

	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	// status, separator, body…, separator, search
	if !strings.Contains(lines[2], "Not found") {
		t.Errorf("warning should sit at the top of the list, away from the search bar: %q", lines[2])
	}
	if best := lines[len(lines)-3]; !strings.Contains(best, "a") || strings.Contains(best, "Not found") {
		t.Errorf("best match should hug the search bar: %q", best)
	}
}

func TestLayoutBottom(t *testing.T) {
	for _, layout := range []string{"", "top", "sideways"} {
		if newModel(nil, map[string]float64{}, tmux.State{}, false, config.Config{Layout: layout}, false).layoutBottom {
			t.Errorf("layout %q should render top", layout)
		}
	}

	cs := []cand.Candidate{
		{AbsPath: "/p/best", RelPath: "best"},
		{AbsPath: "/p/worse", RelPath: "worse"},
	}
	scores := map[string]float64{"/p/best": 5, "/p/worse": 1}
	bottom := config.Config{Layout: "bottom"}
	frame := func(cs []cand.Candidate) []string {
		return strings.Split(plain(sized(newModel(cs, scores, tmux.State{}, false, bottom, false), 80, 24)), "\n")
	}

	lines := frame(cs)
	last := len(lines) - 1
	if !strings.Contains(lines[0], "Filter") || !strings.Contains(lines[last], "❯") {
		t.Errorf("want status bar first and search bar last, got %q … %q", lines[0], lines[last])
	}
	// best match hugs the search bar: search, separator, then the list
	if !strings.Contains(lines[last-2], "best") || !strings.Contains(lines[last-3], "worse") {
		t.Errorf("list should be reversed above the search bar, got %q then %q", lines[last-3], lines[last-2])
	}

	if lines := frame(nil); !strings.Contains(lines[last-2], "Nothing here") {
		t.Errorf("empty state should sit next to the search bar, got %q", lines[last-2])
	}
}
