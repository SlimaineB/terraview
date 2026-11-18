// Package gitops performs the local git operations behind the "Commit & MR"
// action: read the origin remote, commit the working tree, push the branch,
// and open a pull request (GitHub) or merge request (GitLab) via the API.
//
// Safety rules: it never force-pushes, never creates tags, and refuses to act
// on a base branch (main/master) or a detached HEAD.
package gitops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Remote describes the parsed origin remote.
type Remote struct {
	Provider string // "github" or "gitlab"
	Host     string // e.g. github.com
	WebBase  string // e.g. https://github.com
	APIBase  string // e.g. https://api.github.com
	Owner    string // GitHub: owner; GitLab: full namespace path (group/sub)
	Repo     string // repository name
}

// MRRequest carries the fields needed to open a PR/MR.
type MRRequest struct {
	Title  string
	Body   string
	Branch string // source branch
	Base   string // target branch
	Draft  bool
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// IsBaseBranch reports whether branch must not be committed to directly.
func IsBaseBranch(branch, base string) bool {
	if branch == "" || strings.HasPrefix(branch, "detached") {
		return true
	}
	if base != "" && branch == base {
		return true
	}
	switch branch {
	case "main", "master":
		return true
	}
	return false
}

// ParseRemoteURL parses an origin URL (https, ssh, or scp-like) into a Remote.
func ParseRemoteURL(raw string) (Remote, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Remote{}, errors.New("empty remote URL")
	}

	var host, path string
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return Remote{}, fmt.Errorf("invalid remote URL %q", raw)
		}
		host = u.Hostname()
		path = strings.TrimPrefix(u.Path, "/")
	case strings.Contains(raw, "@") && strings.Contains(raw, ":"):
		// scp-like syntax: git@host:owner/repo.git
		at := strings.LastIndex(raw, "@")
		colon := strings.Index(raw[at:], ":")
		if colon < 0 {
			return Remote{}, fmt.Errorf("invalid remote URL %q", raw)
		}
		host = raw[at+1 : at+colon]
		path = raw[at+colon+1:]
	default:
		return Remote{}, fmt.Errorf("unsupported remote URL %q", raw)
	}

	path = strings.TrimSuffix(path, ".git")
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(segs) < 2 || segs[len(segs)-1] == "" {
		return Remote{}, fmt.Errorf("cannot parse owner/repo from %q", raw)
	}

	rem := Remote{
		Host:  host,
		Owner: strings.Join(segs[:len(segs)-1], "/"),
		Repo:  segs[len(segs)-1],
	}
	switch {
	case host == "github.com":
		rem.Provider = "github"
		rem.WebBase = "https://github.com"
		rem.APIBase = "https://api.github.com"
	case strings.Contains(host, "gitlab"):
		rem.Provider = "gitlab"
		rem.WebBase = "https://" + host
		rem.APIBase = rem.WebBase + "/api/v4"
	default:
		// Assume a GitHub Enterprise host.
		rem.Provider = "github"
		rem.WebBase = "https://" + host
		rem.APIBase = rem.WebBase + "/api/v3"
	}

	// API base override (tests, proxies).
	if rem.Provider == "github" {
		if v := strings.TrimSpace(os.Getenv("TERRAVIEW_GITHUB_API_BASE")); v != "" {
			rem.APIBase = v
		}
	} else if v := strings.TrimSpace(os.Getenv("TERRAVIEW_GITLAB_API_BASE")); v != "" {
		rem.APIBase = v
	}
	return rem, nil
}

// Origin returns the URL of the origin remote (falling back to the first remote).
func Origin(dir string) (string, error) {
	if out, err := runGit(dir, "remote", "get-url", "origin"); err == nil && out != "" {
		return out, nil
	}
	remotes, err := runGit(dir, "remote")
	if err != nil {
		return "", err
	}
	names := strings.Fields(remotes)
	if len(names) == 0 {
		return "", errors.New("no git remote configured")
	}
	return runGit(dir, "remote", "get-url", names[0])
}

// DefaultBase resolves the repository's default branch (origin/HEAD, then
// main/master if they exist locally).
func DefaultBase(dir string) string {
	if out, err := runGit(dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil && out != "" {
		return strings.TrimPrefix(out, "origin/")
	}
	for _, b := range []string{"main", "master"} {
		if _, err := runGit(dir, "rev-parse", "--verify", "refs/heads/"+b); err == nil {
			return b
		}
	}
	return "main"
}

// CommitAll stages everything and commits. It returns committed=false when
// the working tree was already clean.
func CommitAll(dir, message string) (bool, error) {
	if _, err := runGit(dir, "add", "-A"); err != nil {
		return false, err
	}
	out, err := runGit(dir, "commit", "-m", message)
	if err != nil {
		if strings.Contains(out, "nothing to commit") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Push pushes the branch to origin with -u. It never force-pushes.
func Push(dir, branch string) error {
	_, err := runGit(dir, "push", "-u", "origin", branch)
	return err
}

// OpenMR opens a PR (GitHub) or MR (GitLab) and returns its web URL.
func OpenMR(rem Remote, req MRRequest, token string) (string, error) {
	if rem.Provider == "gitlab" {
		return openGitLabMR(rem, req, token)
	}
	return openGitHubPR(rem, req, token)
}

// CompareURL returns a manual compare/new-MR URL, used as a fallback when the
// API call fails but the branch was pushed.
func CompareURL(rem Remote, branch, base string) string {
	if rem.Provider == "gitlab" {
		v := url.Values{}
		v.Set("merge_request[source_branch]", branch)
		v.Set("merge_request[target_branch]", base)
		return fmt.Sprintf("%s/%s/-/merge_requests/new?%s", rem.WebBase, rem.Owner+"/"+rem.Repo, v.Encode())
	}
	return fmt.Sprintf("%s/%s/compare/%s...%s", rem.WebBase, rem.Owner+"/"+rem.Repo, base, branch)
}

// TokenFor resolves an API token: GH_TOKEN/GITHUB_TOKEN (GitHub) or
// GITLAB_TOKEN/GLAB_TOKEN (GitLab), then the git credential helper.
// The token is never logged.
func TokenFor(provider, host string) string {
	var envNames []string
	if provider == "gitlab" {
		envNames = []string{"GITLAB_TOKEN", "GLAB_TOKEN"}
	} else {
		envNames = []string{"GH_TOKEN", "GITHUB_TOKEN"}
	}
	for _, name := range envNames {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "password=") {
			return strings.TrimPrefix(line, "password=")
		}
	}
	return ""
}

func openGitHubPR(rem Remote, req MRRequest, token string) (string, error) {
	payload := map[string]any{
		"title": req.Title,
		"head":  req.Branch,
		"base":  req.Base,
		"body":  req.Body,
	}
	if req.Draft {
		payload["draft"] = true
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequest(http.MethodPost,
		rem.APIBase+"/repos/"+rem.Owner+"/"+rem.Repo+"/pulls", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API %s: %s", resp.Status, truncate(string(respBody), 300))
	}
	var result struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decoding GitHub response: %w", err)
	}
	if result.HTMLURL == "" {
		return "", errors.New("GitHub API returned no PR URL")
	}
	return result.HTMLURL, nil
}

func openGitLabMR(rem Remote, req MRRequest, token string) (string, error) {
	title := req.Title
	if req.Draft && !strings.HasPrefix(title, "Draft: ") {
		title = "Draft: " + title
	}
	form := url.Values{}
	form.Set("source_branch", req.Branch)
	form.Set("target_branch", req.Base)
	form.Set("title", title)
	if req.Body != "" {
		form.Set("description", req.Body)
	}

	project := url.PathEscape(rem.Owner + "/" + rem.Repo)
	httpReq, err := http.NewRequest(http.MethodPost,
		rem.APIBase+"/projects/"+project+"/merge_requests", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitLab API %s: %s", resp.Status, truncate(string(respBody), 300))
	}
	var result struct {
		WebURL string `json:"web_url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decoding GitLab response: %w", err)
	}
	if result.WebURL == "" {
		return "", errors.New("GitLab API returned no MR URL")
	}
	return result.WebURL, nil
}

func runGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("git %s: %w", args[0], err)
	}
	return s, nil
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
