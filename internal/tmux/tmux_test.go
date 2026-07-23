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
