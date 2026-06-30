package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"path"
	"strings"
)

// RepoNameFromURL returns the repository name from any git URL.
func RepoNameFromURL(url string) string {
	// Normalize git SSH URLs (git@host:user/repo) to slash form.
	if i := strings.LastIndex(url, ":"); i >= 0 && !strings.HasPrefix(url, "http") {
		url = url[i+1:]
	}
	base := path.Base(url)
	return strings.TrimSuffix(base, ".git")
}

// Clone runs git clone into destPath and returns destPath.
// Stderr is captured and returned as part of any error so callers can display it.
func Clone(url, destPath string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("git", "clone", url, destPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", err
	}
	return destPath, nil
}
