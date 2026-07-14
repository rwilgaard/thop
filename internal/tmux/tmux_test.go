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
