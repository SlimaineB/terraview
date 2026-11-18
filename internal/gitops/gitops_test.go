package gitops

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		raw      string
		provider string
		owner    string
		repo     string
		apiBase  string
		wantErr  bool
	}{
		{"https://github.com/acme/infra.git", "github", "acme", "infra", "https://api.github.com", false},
		{"https://github.com/acme/infra", "github", "acme", "infra", "https://api.github.com", false},
		{"git@github.com:acme/infra.git", "github", "acme", "infra", "https://api.github.com", false},
		{"ssh://git@github.com/acme/infra.git", "github", "acme", "infra", "https://api.github.com", false},
		{"https://gitlab.com/group/sub/infra.git", "gitlab", "group/sub", "infra", "https://gitlab.com/api/v4", false},
		{"git@gitlab.example.com:team/app.git", "gitlab", "team", "app", "https://gitlab.example.com/api/v4", false},
		{"https://ghe.corp.local/acme/infra.git", "github", "acme", "infra", "https://ghe.corp.local/api/v3", false},
		{"", "", "", "", "", true},
		{"/local/path/repo.git", "", "", "", "", true},
		{"https://github.com/onlyowner", "", "", "", "", true},
	}
	for _, tc := range cases {
		rem, err := ParseRemoteURL(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseRemoteURL(%q): expected error, got %+v", tc.raw, rem)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRemoteURL(%q): unexpected error: %v", tc.raw, err)
			continue
		}
		if rem.Provider != tc.provider || rem.Owner != tc.owner || rem.Repo != tc.repo || rem.APIBase != tc.apiBase {
			t.Errorf("ParseRemoteURL(%q) = %+v, want provider=%s owner=%s repo=%s api=%s",
				tc.raw, rem, tc.provider, tc.owner, tc.repo, tc.apiBase)
		}
	}
}

func TestIsBaseBranch(t *testing.T) {
	for _, branch := range []string{"", "main", "master", "detached @ abc1234"} {
		if !IsBaseBranch(branch, "") {
			t.Errorf("IsBaseBranch(%q, \"\") = false, want true", branch)
		}
	}
	if !IsBaseBranch("develop", "develop") {
		t.Error("branch equal to explicit base must be refused")
	}
	if IsBaseBranch("feature-x", "main") {
		t.Error("feature-x must not be refused")
	}
}

func TestTokenForEnv(t *testing.T) {
	t.Setenv("GH_TOKEN", "gh-secret")
	if got := TokenFor("github", "github.com"); got != "gh-secret" {
		t.Errorf("TokenFor github = %q, want gh-secret", got)
	}
	t.Setenv("GITLAB_TOKEN", "gl-secret")
	if got := TokenFor("gitlab", "gitlab.com"); got != "gl-secret" {
		t.Errorf("TokenFor gitlab = %q, want gl-secret", got)
	}
}

func TestOpenMRGitHub(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"html_url": "https://github.com/acme/infra/pull/42"})
	}))
	defer srv.Close()

	rem := Remote{Provider: "github", APIBase: srv.URL, Owner: "acme", Repo: "infra", WebBase: "https://github.com"}
	url, err := OpenMR(rem, MRRequest{Title: "Add VPC", Body: "desc", Branch: "feature/x", Base: "main", Draft: true}, "tok123")
	if err != nil {
		t.Fatalf("OpenMR: %v", err)
	}
	if url != "https://github.com/acme/infra/pull/42" {
		t.Errorf("url = %q", url)
	}
	if gotPath != "/repos/acme/infra/pulls" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotBody["head"] != "feature/x" || gotBody["base"] != "main" || gotBody["title"] != "Add VPC" || gotBody["draft"] != true {
		t.Errorf("body = %v", gotBody)
	}
}

func TestOpenMRGitLab(t *testing.T) {
	var gotPath string
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		body, _ := io.ReadAll(r.Body)
		form, _ = url.ParseQuery(string(body))
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"web_url": "https://gitlab.com/group/sub/-/merge_requests/7"})
	}))
	defer srv.Close()

	rem := Remote{Provider: "gitlab", APIBase: srv.URL, Owner: "group/sub", Repo: "infra", WebBase: "https://gitlab.com"}
	url, err := OpenMR(rem, MRRequest{Title: "Add VPC", Branch: "feature/x", Base: "main", Draft: true}, "tok123")
	if err != nil {
		t.Fatalf("OpenMR: %v", err)
	}
	if url != "https://gitlab.com/group/sub/-/merge_requests/7" {
		t.Errorf("url = %q", url)
	}
	if gotPath != "/projects/group%2Fsub%2Finfra/merge_requests" {
		t.Errorf("path = %q", gotPath)
	}
	if form.Get("source_branch") != "feature/x" || form.Get("target_branch") != "main" {
		t.Errorf("form = %v", form)
	}
	if form.Get("title") != "Draft: Add VPC" {
		t.Errorf("draft title = %q", form.Get("title"))
	}
}

func TestOpenMRAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed"}`))
	}))
	defer srv.Close()

	rem := Remote{Provider: "github", APIBase: srv.URL, Owner: "acme", Repo: "infra"}
	_, err := OpenMR(rem, MRRequest{Title: "x", Branch: "b", Base: "main"}, "tok")
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("expected 422 error, got %v", err)
	}
}

func TestCompareURL(t *testing.T) {
	gh := Remote{Provider: "github", WebBase: "https://github.com", Owner: "acme", Repo: "infra"}
	if got := CompareURL(gh, "feature", "main"); got != "https://github.com/acme/infra/compare/main...feature" {
		t.Errorf("github compare = %q", got)
	}
	gl := Remote{Provider: "gitlab", WebBase: "https://gitlab.com", Owner: "group", Repo: "app"}
	got := CompareURL(gl, "feature", "main")
	if !strings.HasPrefix(got, "https://gitlab.com/group/app/-/merge_requests/new?") {
		t.Fatalf("gitlab compare = %q", got)
	}
	q, err := url.ParseQuery(strings.SplitN(got, "?", 2)[1])
	if err != nil {
		t.Fatalf("bad query: %v", err)
	}
	if q.Get("merge_request[source_branch]") != "feature" || q.Get("merge_request[target_branch]") != "main" {
		t.Errorf("gitlab compare query = %v", q)
	}
}

// --- git operations on temp repos ---

func git(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, nil, "init", "-b", "main")
	git(t, dir, nil, "commit", "--allow-empty", "-m", "init")
	return dir
}

func TestCommitAllAndPush(t *testing.T) {
	dir := initRepo(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, bare, nil, "init", "--bare", "-b", "main")
	git(t, dir, nil, "remote", "add", "origin", bare)

	git(t, dir, nil, "checkout", "-b", "feature/x")
	if IsBaseBranch("feature/x", "main") {
		t.Fatal("feature/x must be allowed")
	}

	// Nothing to commit yet.
	committed, err := CommitAll(dir, "msg")
	if err != nil || committed {
		t.Fatalf("clean CommitAll = %v, %v", committed, err)
	}

	// Dirty the tree, then commit.
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("resource {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	committed, err = CommitAll(dir, "add main.tf")
	if err != nil || !committed {
		t.Fatalf("dirty CommitAll = %v, %v", committed, err)
	}

	if err := Push(dir, "feature/x"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	out := git(t, dir, nil, "ls-remote", "origin", "refs/heads/feature/x")
	if !strings.Contains(out, "refs/heads/feature/x") {
		t.Errorf("branch not pushed: %q", out)
	}
}

func TestDefaultBase(t *testing.T) {
	dir := initRepo(t)
	if got := DefaultBase(dir); got != "main" {
		t.Errorf("DefaultBase = %q, want main", got)
	}
}
