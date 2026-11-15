package gitcontext

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Context struct {
	Path   string
	Branch string
}

func Detect(startDir string) Context {
	if startDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			startDir = cwd
		}
	}
	if startDir == "" {
		return Context{}
	}

	root := gitOutput(startDir, "rev-parse", "--show-toplevel")
	if root == "" {
		// A plan can live outside the work tree (for example /tmp/plan.json)
		// while TerraView is started from a repository. Prefer that context.
		if cwd, err := os.Getwd(); err == nil && cwd != startDir {
			root = gitOutput(cwd, "rev-parse", "--show-toplevel")
		}
	}
	if root == "" {
		return Context{}
	}

	branch := gitOutput(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if branch == "" {
		if sha := gitOutput(root, "rev-parse", "--short", "HEAD"); sha != "" {
			branch = "detached @ " + sha
		}
	}
	return Context{Path: root, Branch: branch}
}

func gitOutput(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
