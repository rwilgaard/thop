package tmux

import "testing"

func TestSessionize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"foo.bar", "foo_bar"},
		{"foo", "foo"},
		{"a.b.c", "a_b_c"},
		{"nodots", "nodots"},
	}
	for _, tt := range tests {
		if got := Sessionize(tt.in); got != tt.want {
			t.Errorf("Sessionize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTargetWindow(t *testing.T) {
	tests := []struct {
		session, window, want string
	}{
		{"my-session", "kustomize-sync.nvim", "=my-session:kustomize-sync?nvim"},
		{"my-session", "foo", "=my-session:=foo"},
		{"my-session", "foo.bar.baz", "=my-session:foo?bar?baz"},
		{"another_session", "dot.", "=another_session:dot?"},
		{"sess", ".dot", "=sess:?dot"},
	}
	for _, tt := range tests {
		t.Run(tt.window, func(t *testing.T) {
			if got := targetWindow(tt.session, tt.window); got != tt.want {
				t.Errorf("targetWindow(%q, %q) = %q, want %q", tt.session, tt.window, got, tt.want)
			}
		})
	}
}

func TestParseClient(t *testing.T) {
	tests := []struct {
		name, in                  string
		session, window, lastSess string
	}{
		{"full", "work\tapi\thome\n", "work", "api", "home"},
		{"no last session", "work\tapi\t\n", "work", "api", ""},
		{"tab in window name stays in last field", "work\tapi\thome\textra", "work", "api", "home\textra"},
		{"malformed", "work\n", "", "", ""},
		{"empty", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, w, l := parseClient(tt.in)
			if s != tt.session || w != tt.window || l != tt.lastSess {
				t.Errorf("parseClient(%q) = %q, %q, %q; want %q, %q, %q", tt.in, s, w, l, tt.session, tt.window, tt.lastSess)
			}
		})
	}
}
