package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"testing"

	"terraview/internal/gitcontext"
	"terraview/internal/graph"
	"terraview/internal/server"
)

func TestRealTerraformPlanThroughServer(t *testing.T) {
	raw, err := os.ReadFile("internal/graph/testdata/real-plan/plan.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	plan, err := graph.Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	handler := server.New(server.Config{
		Plan: plan,
		Repo: gitcontext.Context{Path: "/workspace/infra", Branch: "main"},
		Index: func() ([]byte, error) {
			return []byte("<!doctype html><title>TerraView</title>"), nil
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got graph.Data
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3", len(got.Nodes))
	}
	if len(got.Edges) != 2 {
		t.Errorf("edges = %d, want 2", len(got.Edges))
	}
	if got.GitPath != "/workspace/infra" || got.GitBranch != "main" {
		t.Errorf("git context = %q/%q, want /workspace/infra/main", got.GitPath, got.GitBranch)
	}
}

func TestListenOnFreePort(t *testing.T) {
	listener, port, err := listenOn("")
	if err != nil {
		t.Fatalf("listenOn() error = %v", err)
	}
	defer listener.Close()

	number, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("port %q is not numeric: %v", port, err)
	}
	if number == 0 {
		t.Fatalf("port = %q, want an assigned port", port)
	}
}

func TestBrowserCandidates(t *testing.T) {
	const url = "http://localhost:8080"
	tests := []struct {
		name    string
		browser string
		goos    string
		want    [][]string
	}{
		{
			name: "unset falls back to xdg-open",
			goos: "linux",
			want: [][]string{{"xdg-open", url}},
		},
		{
			name:    "plain command appends url",
			browser: "firefox",
			goos:    "linux",
			want:    [][]string{{"firefox", url}, {"xdg-open", url}},
		},
		{
			name:    "placeholder replaces url",
			browser: "firefox --new-tab %s",
			goos:    "linux",
			want:    [][]string{{"firefox", "--new-tab", url}, {"xdg-open", url}},
		},
		{
			name:    "colon separated candidates",
			browser: "firefox:chromium --incognito %s",
			goos:    "linux",
			want: [][]string{
				{"firefox", url},
				{"chromium", "--incognito", url},
				{"xdg-open", url},
			},
		},
		{
			name:    "quoted arguments",
			browser: `open -a "Google Chrome" %s`,
			goos:    "darwin",
			want:    [][]string{{"open", "-a", "Google Chrome", url}, {"open", url}},
		},
		{
			name: "windows fallback",
			goos: "windows",
			want: [][]string{{"rundll32", "url.dll,FileProtocolHandler", url}},
		},
		{
			name:    "invalid command falls back",
			browser: `"firefox`,
			goos:    "linux",
			want:    [][]string{{"xdg-open", url}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := browserCandidates(tt.browser, url, tt.goos)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("browserCandidates() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSplitCommandLine(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{name: "simple", command: "firefox --new-tab", want: []string{"firefox", "--new-tab"}},
		{name: "double quotes", command: `open -a "Google Chrome"`, want: []string{"open", "-a", "Google Chrome"}},
		{name: "single quotes", command: `browser 'two words'`, want: []string{"browser", "two words"}},
		{name: "escaped space", command: `browser one\ two`, want: []string{"browser", "one two"}},
		{name: "windows path", command: `"C:\Program Files\Browser\browser.exe" %s`, want: []string{`C:\Program Files\Browser\browser.exe`, "%s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitCommandLine(tt.command)
			if err != nil {
				t.Fatalf("splitCommandLine() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitCommandLine() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
