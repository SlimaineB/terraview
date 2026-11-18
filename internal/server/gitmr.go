package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"terraview/internal/gitcontext"
	"terraview/internal/gitops"
)

type commitMRRequest struct {
	Message string `json:"message"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Draft   bool   `json:"draft"`
	Base    string `json:"base"`
}

type commitMRResponse struct {
	OK        bool   `json:"ok"`
	URL       string `json:"url,omitempty"`
	Fallback  string `json:"fallback,omitempty"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Committed bool   `json:"committed"`
	Pushed    bool   `json:"pushed"`
	Message   string `json:"message,omitempty"`
}

// handleCommitMR implements POST /api/git/commit-mr: commit the working tree
// of the repository shown in the UI, push the current branch, and open a
// PR/MR on the origin remote (GitHub or GitLab).
func handleCommitMR(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			writeMRError(w, http.StatusMethodNotAllowed, "method not allowed", commitMRResponse{})
			return
		}
		if cfg.Repo.Path == "" {
			writeMRError(w, http.StatusBadRequest, "TerraView is not running inside a git repository", commitMRResponse{})
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		if err != nil {
			writeMRError(w, http.StatusBadRequest, "failed to read body", commitMRResponse{})
			return
		}
		var req commitMRRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeMRError(w, http.StatusBadRequest, "invalid JSON body", commitMRResponse{})
			return
		}
		req.Message = strings.TrimSpace(req.Message)
		if req.Message == "" {
			writeMRError(w, http.StatusBadRequest, "commit message is required", commitMRResponse{})
			return
		}

		// Re-detect at request time: the state may have changed since startup.
		repo := gitcontext.Detect(cfg.Repo.Path)
		if repo.Path == "" {
			writeMRError(w, http.StatusBadRequest, "repository context lost", commitMRResponse{})
			return
		}
		branch := repo.Branch
		base := strings.TrimSpace(req.Base)
		if base == "" {
			base = gitops.DefaultBase(repo.Path)
		}
		if gitops.IsBaseBranch(branch, base) {
			writeMRError(w, http.StatusBadRequest,
				"refusing to commit on the base branch "+branch+" — switch to a feature branch first",
				commitMRResponse{Branch: branch, Base: base})
			return
		}

		origin, err := gitops.Origin(repo.Path)
		if err != nil {
			writeMRError(w, http.StatusBadRequest, err.Error(), commitMRResponse{Branch: branch, Base: base})
			return
		}
		rem, err := gitops.ParseRemoteURL(origin)
		if err != nil {
			writeMRError(w, http.StatusBadRequest, "unsupported origin remote "+origin+": "+err.Error(), commitMRResponse{Branch: branch, Base: base})
			return
		}
		token := gitops.TokenFor(rem.Provider, rem.Host)
		if token == "" {
			writeMRError(w, http.StatusBadRequest,
				"no API token found — set GH_TOKEN/GITHUB_TOKEN (GitHub) or GITLAB_TOKEN/GLAB_TOKEN (GitLab), or configure a git credential helper",
				commitMRResponse{Branch: branch, Base: base})
			return
		}

		resp := commitMRResponse{Branch: branch, Base: base}

		committed, err := gitops.CommitAll(repo.Path, req.Message)
		if err != nil {
			resp.Message = "commit failed: " + err.Error()
			writeMRError(w, http.StatusInternalServerError, resp.Message, resp)
			return
		}
		resp.Committed = committed

		// Push only when needed: no upstream yet, or local commits ahead.
		if repo.Upstream == "" || repo.Ahead > 0 {
			if err := gitops.Push(repo.Path, branch); err != nil {
				resp.Message = "push failed: " + err.Error()
				resp.Fallback = gitops.CompareURL(rem, branch, base)
				writeMRError(w, http.StatusBadGateway, resp.Message, resp)
				return
			}
			resp.Pushed = true
		}

		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = strings.SplitN(req.Message, "\n", 2)[0]
		}
		mrURL, err := gitops.OpenMR(rem, gitops.MRRequest{
			Title:  title,
			Body:   req.Body,
			Branch: branch,
			Base:   base,
			Draft:  req.Draft,
		}, token)
		if err != nil {
			resp.Message = "could not open the MR via API: " + err.Error()
			resp.Fallback = gitops.CompareURL(rem, branch, base)
			writeMRError(w, http.StatusBadGateway, resp.Message, resp)
			return
		}

		resp.OK = true
		resp.URL = mrURL
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}
}

func writeMRError(w http.ResponseWriter, status int, msg string, resp commitMRResponse) {
	resp.OK = false
	resp.Message = msg
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
