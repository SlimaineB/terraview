package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"terraview/internal/gitcontext"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func setupRepo(t *testing.T, branch string) string {
	return setupRepoWithRemote(t, branch, "https://github.com/acme/infra.git")
}

func setupRepoWithRemote(t *testing.T, branch, remote string) string {
	t.Helper()
	// Commit identity via env so the handler's subprocesses work in CI too.
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	dir := t.TempDir()
	gitT(t, dir, "init", "-b", "main")
	gitT(t, dir, "commit", "--allow-empty", "-m", "init")
	gitT(t, dir, "remote", "add", "origin", remote)
	if branch != "main" {
		gitT(t, dir, "checkout", "-b", branch)
		// Fake an upstream so the handler skips the push (ahead == 0).
		gitT(t, dir, "update-ref", "refs/remotes/origin/"+branch, "HEAD")
		gitT(t, dir, "branch", "--set-upstream-to=origin/"+branch, branch)
	}
	return dir
}

func postCMR(t *testing.T, h http.Handler, body string) (int, commitMRResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/git/commit-mr", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp commitMRResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	return rec.Code, resp
}

func TestCommitMRNoRepo(t *testing.T) {
	h := New(Config{})
	code, resp := postCMR(t, h, `{"message":"x"}`)
	if code != http.StatusBadRequest || resp.OK {
		t.Fatalf("code=%d resp=%+v", code, resp)
	}
}

func TestCommitMRRefusesBaseBranch(t *testing.T) {
	dir := setupRepo(t, "main")
	cfg := Config{Repo: gitcontext.Detect(dir)}
	h := New(cfg)
	code, resp := postCMR(t, h, `{"message":"x"}`)
	if code != http.StatusBadRequest || !strings.Contains(resp.Message, "base branch") {
		t.Fatalf("code=%d resp=%+v", code, resp)
	}
}

func TestCommitMRRequiresMessage(t *testing.T) {
	dir := setupRepo(t, "feature/x")
	cfg := Config{Repo: gitcontext.Detect(dir)}
	h := New(cfg)
	code, _ := postCMR(t, h, `{"message":"  "}`)
	if code != http.StatusBadRequest {
		t.Fatalf("code=%d", code)
	}
}

func TestCommitMRFullFlow(t *testing.T) {
	var gotBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/infra/pulls" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"html_url": "https://github.com/acme/infra/pull/42"})
	}))
	defer api.Close()

	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("TERRAVIEW_GITHUB_API_BASE", api.URL)

	dir := setupRepo(t, "feature/x")
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("resource \"null_resource\" \"x\" {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{Repo: gitcontext.Detect(dir)}
	h := New(cfg)
	code, resp := postCMR(t, h, `{"message":"add null resource","title":"Add null resource","base":"main"}`)
	if code != http.StatusOK || !resp.OK {
		t.Fatalf("code=%d resp=%+v", code, resp)
	}
	if resp.URL != "https://github.com/acme/infra/pull/42" {
		t.Errorf("url = %q", resp.URL)
	}
	if !resp.Committed || resp.Pushed {
		t.Errorf("committed=%v pushed=%v, want committed=true pushed=false (upstream up to date)", resp.Committed, resp.Pushed)
	}
	if resp.Branch != "feature/x" || resp.Base != "main" {
		t.Errorf("branch=%q base=%q", resp.Branch, resp.Base)
	}
	if gotBody["head"] != "feature/x" || gotBody["base"] != "main" || gotBody["title"] != "Add null resource" {
		t.Errorf("api body = %v", gotBody)
	}

	// Working tree must be clean now.
	status := gitT(t, dir, "status", "--porcelain")
	if status != "" {
		t.Errorf("working tree still dirty after commit: %q", status)
	}
}

func TestCommitMRAPIFailureFallback(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed"}`))
	}))
	defer api.Close()

	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("TERRAVIEW_GITHUB_API_BASE", api.URL)

	dir := setupRepo(t, "feature/x")
	cfg := Config{Repo: gitcontext.Detect(dir)}
	h := New(cfg)
	code, resp := postCMR(t, h, `{"message":"x"}`)
	if code != http.StatusBadGateway || resp.OK {
		t.Fatalf("code=%d resp=%+v", code, resp)
	}
	if !strings.Contains(resp.Fallback, "/compare/main...feature/x") {
		t.Errorf("fallback = %q", resp.Fallback)
	}
	if !strings.Contains(resp.Message, "could not open the MR") {
		t.Errorf("message = %q", resp.Message)
	}
}

func TestCommitMRNoToken(t *testing.T) {
	// Isolate from real credentials: temp HOME, no config, no askpass helper,
	// and a host the credential store knows nothing about.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_ASKPASS", "/bin/false")
	t.Setenv("SSH_ASKPASS", "/bin/false")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	dir := setupRepoWithRemote(t, "feature/x", "https://ghe.invalid/acme/infra.git")
	cfg := Config{Repo: gitcontext.Detect(dir)}
	h := New(cfg)
	code, resp := postCMR(t, h, `{"message":"x"}`)
	if code != http.StatusBadRequest || !strings.Contains(resp.Message, "no API token") {
		t.Fatalf("code=%d resp=%+v", code, resp)
	}
}
