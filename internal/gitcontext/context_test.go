package gitcontext

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=TerraView Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=TerraView Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestDetect(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	runGitTestCommand(t, dir, "init", "-q")
	runGitTestCommand(t, dir, "checkout", "-q", "-b", "feature/terraview")

	subdir := filepath.Join(dir, "modules", "network")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	got := Detect(subdir)
	if got.Branch != "feature/terraview" {
		t.Errorf("branch = %q, want feature/terraview", got.Branch)
	}
	wantRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	gotRoot, err := filepath.EvalSymlinks(got.Path)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if gotRoot != wantRoot {
		t.Errorf("path = %q, want %q", gotRoot, wantRoot)
	}

	runGitTestCommand(t, dir, "commit", "-q", "--allow-empty", "-m", "initial")
	runGitTestCommand(t, dir, "checkout", "-q", "--detach")
	got = Detect(subdir)
	if !strings.HasPrefix(got.Branch, "detached @ ") {
		t.Errorf("branch = %q, want detached @ <sha>", got.Branch)
	}
}

func TestDetectRepoDoesNotFallBackToCwd(t *testing.T) {
	if got := DetectRepo(t.TempDir()); got.Path != "" {
		t.Fatalf("DetectRepo on a non-repo dir = %+v, want empty", got)
	}
}
