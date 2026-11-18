package gitcontext

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Context struct {
	Path     string
	Branch   string
	Dirty    bool
	Upstream string
	Ahead    int
	Behind   int
	Unpushed bool
}

// Detect inspects startDir, falling back to the process working directory
// when the path is not inside a work tree.
func Detect(startDir string) Context {
	return detect(startDir, true)
}

// DetectRepo inspects dir as a work tree without falling back to the process
// working directory. Used to refresh an already-known repository.
func DetectRepo(dir string) Context {
	return detect(dir, false)
}

func detect(startDir string, allowCwdFallback bool) Context {
	if startDir == "" {
		if !allowCwdFallback {
			return Context{}
		}
		if cwd, err := os.Getwd(); err == nil {
			startDir = cwd
		}
	}
	if startDir == "" {
		return Context{}
	}

	root := gitOutput(startDir, "rev-parse", "--show-toplevel")
	if root == "" && allowCwdFallback {
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

	ctx := Context{Path: root, Branch: branch}
	loadStatus(root, &ctx)

	if branch == "" || strings.HasPrefix(branch, "detached") {
		ctx.Unpushed = true
	} else if ctx.Upstream == "" {
		ctx.Unpushed = true
	} else if ctx.Ahead > 0 {
		ctx.Unpushed = true
	}

	return ctx
}

// loadStatus reads branch, ahead/behind, and working-tree state in one call.
func loadStatus(root string, ctx *Context) {
	status := gitOutput(root, "status", "--porcelain=v2", "--branch", "--untracked-files=normal")
	if status == "" {
		// Empty means the command failed or timed out; avoid false warnings.
		return
	}

	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "# branch.upstream "):
			ctx.Upstream = strings.TrimSpace(strings.TrimPrefix(line, "# branch.upstream "))
		case strings.HasPrefix(line, "# branch.ab "):
			for _, f := range strings.Fields(strings.TrimPrefix(line, "# branch.ab ")) {
				switch {
				case strings.HasPrefix(f, "+"):
					ctx.Ahead, _ = strconv.Atoi(f[1:])
				case strings.HasPrefix(f, "-"):
					ctx.Behind, _ = strconv.Atoi(f[1:])
				}
			}
		case strings.HasPrefix(line, "#") || line == "":
			// Branch header or blank line.
		default:
			ctx.Dirty = true
		}
	}
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
