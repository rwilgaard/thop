package main

import (
	"testing"

	"github.com/rwilgaard/thop/internal/candidates"
)

func TestSessionFor(t *testing.T) {
	cs := candidates.Resolve([]candidates.Candidate{
		{AbsPath: "/code/alpha", Root: "/code", RelPath: "alpha"},
		{AbsPath: "/work/alpha", Root: "/work", RelPath: "alpha"},
		{AbsPath: "/code/group", Root: "/code", RelPath: "group"},
		{AbsPath: "/code/group/repo", Root: "/code", RelPath: "group/repo", IsRepo: true},
	})
	tests := []struct {
		name, path, want string
	}{
		{"colliding project", "/work/alpha", "alpha@work"},
		{"unlisted dir inside a project", "/work/alpha/notes", "alpha@work"},
		{"repo window", "/code/group/repo", "group"},
		{"dir inside a repo keeps the repo-named session", "/code/group/repo/sub", ""},
		{"unknown", "/elsewhere/x", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionFor(tt.path, cs); got != tt.want {
				t.Errorf("sessionFor(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
