package git

import (
	"os"
	"os/exec"
	"path"
	"strings"
)

// RepoNameFromURL returns the repository name from any git URL.
// Handles https://host/user/repo, https://host/user/repo.git,
// and git@host:user/repo.git forms.
func RepoNameFromURL(url string) string {
	// Normalize git SSH URLs (git@host:user/repo) to slash form.
	if i := strings.LastIndex(url, ":"); i >= 0 && !strings.HasPrefix(url, "http") {
		url = url[i+1:]
	}
	base := path.Base(url)
	return strings.TrimSuffix(base, ".git")
}

// Clone runs git clone into destPath and returns destPath.
// The caller is responsible for constructing the full destination path
// (typically filepath.Join(parentDir, RepoNameFromURL(url))).
func Clone(url, destPath string) (string, error) {
	cmd := exec.Command("git", "clone", url, destPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return destPath, cmd.Run()
}
