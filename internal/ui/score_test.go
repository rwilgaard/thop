package ui

import (
	"slices"
	"strings"
	"testing"

	cand "github.com/rwilgaard/thop/internal/candidates"
	"github.com/rwilgaard/thop/internal/config"
	"github.com/rwilgaard/thop/internal/tmux"
)

func TestNormalizeScores(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]float64
		expect map[string]float64
	}{
		{
			name:   "empty input",
			input:  map[string]float64{},
			expect: map[string]float64{},
		},
		{
			name:   "all zero returns empty (map miss = 0.0)",
			input:  map[string]float64{"a": 0, "b": 0},
			expect: map[string]float64{},
		},
		{
			name:  "normalizes to 0-1 range",
			input: map[string]float64{"a": 2.0, "b": 1.0, "c": 4.0},
			expect: map[string]float64{
				"a": 0.5,
				"b": 0.25,
				"c": 1.0,
			},
		},
		{
			name:   "single item becomes 1.0",
			input:  map[string]float64{"a": 5.0},
			expect: map[string]float64{"a": 1.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeScores(tt.input)
			if len(got) != len(tt.expect) {
				t.Fatalf("len mismatch: got %d want %d", len(got), len(tt.expect))
			}
			for k, want := range tt.expect {
				if got[k] != want {
					t.Errorf("key %q: got %v want %v", k, got[k], want)
				}
			}
		})
	}
}

func TestRebuildFiltered(t *testing.T) {
	t.Run("empty query orders by frecency descending", func(t *testing.T) {
		cs := []cand.Candidate{
			{AbsPath: "/p/low", RelPath: "low"},
			{AbsPath: "/p/high", RelPath: "high"},
			{AbsPath: "/p/mid", RelPath: "mid"},
		}
		scores := map[string]float64{"/p/low": 1, "/p/high": 3, "/p/mid": 2}
		m := newModel(cs, scores, tmux.State{}, false, config.Config{}, false)
		if got, want := rowNames(m), []string{"high", "mid", "low"}; !slices.Equal(got, want) {
			t.Errorf("rows = %v, want %v", got, want)
		}
	})

	t.Run("equal fuzzy match: frecency decides, offsets are kept", func(t *testing.T) {
		// Same "abc" prefix, so both normalize to fuzzy 1.0 and the 40%
		// frecency weight sets the order.
		cs := []cand.Candidate{
			{AbsPath: "/p/abc-omega", RelPath: "abc-omega"},
			{AbsPath: "/p/abc-alpha", RelPath: "abc-alpha"},
		}
		scores := map[string]float64{"/p/abc-alpha": 10, "/p/abc-omega": 1}
		m := newModel(cs, scores, tmux.State{}, false, config.Config{}, false)
		m.tiQuery.SetValue("abc")
		m.rebuildFiltered()
		if got, want := rowNames(m), []string{"abc-alpha", "abc-omega"}; !slices.Equal(got, want) {
			t.Errorf("rows = %v, want %v", got, want)
		}
		if len(m.filtered[0].matches) != 3 {
			t.Errorf("matches = %v, want the three matched offsets", m.filtered[0].matches)
		}
	})

	cs := []cand.Candidate{
		{AbsPath: "/p/proj", RelPath: "proj"},
		{AbsPath: "/p/proj-live", RelPath: "proj-live"},
		{AbsPath: "/p/repo", RelPath: "repo", IsRepo: true},
		{AbsPath: "/t/scratch", RelPath: "scratch", IsTmp: true},
	}
	ts := tmux.State{Sessions: map[string]bool{"proj-live": true}}
	views := []struct {
		name  string
		view  viewMode
		query string
		want  []string
	}{
		{"all", viewAll, "", []string{"proj", "proj-live", "repo", "scratch"}},
		{"projects exclude repos and tmp", viewProject, "", []string{"proj", "proj-live"}},
		{"repos", viewRepo, "", []string{"repo"}},
		{"tmp", viewTmp, "", []string{"scratch"}},
		{"open", viewOpen, "", []string{"proj-live"}},
		{"open with query", viewOpen, "proj", []string{"proj-live"}},
	}
	for _, tt := range views {
		t.Run("view "+tt.name, func(t *testing.T) {
			m := newModel(cs, map[string]float64{}, ts, false, config.Config{}, false)
			m.view = tt.view
			m.tiQuery.SetValue(tt.query)
			m.rebuildFiltered()
			if got := rowNames(m); !slices.Equal(got, tt.want) {
				t.Errorf("rows = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRebuildFiltered_sessionOrder(t *testing.T) {
	ts := tmux.State{
		Sessions:      map[string]bool{"here": true, "before": true, "group": true},
		Windows:       map[string]bool{"group/repo": true},
		Current:       "here",
		CurrentWindow: "zsh",
		Last:          "before",
	}
	cs := []cand.Candidate{
		{AbsPath: "/p/here", RelPath: "here"},
		{AbsPath: "/p/top", RelPath: "top"},
		{AbsPath: "/p/before", RelPath: "before"},
		{AbsPath: "/p/group/repo", RelPath: "group/repo", IsRepo: true},
	}
	scores := map[string]float64{"/p/here": 9, "/p/top": 5, "/p/group/repo": 2, "/p/before": 1}
	m := newModel(cs, scores, ts, false, config.Config{}, false)

	order := func() []string {
		var out []string
		for _, it := range m.filtered {
			out = append(out, it.base.candidate.RelPath)
		}
		return out
	}

	want := []string{"before", "top", "group/repo", "here"}
	if got := order(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("empty query: got %v, want %v", got, want)
	}

	m = sized(m, 80, 12)
	if out := m.View().Content; !strings.Contains(out, "current") {
		t.Error("current row should be labelled")
	}

	m.tiQuery.SetValue("e")
	m.rebuildFiltered()
	if got := order(); got[0] != "here" {
		t.Errorf("query: got %v, want here first", got)
	}
}
