package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"terraview/internal/gitcontext"
	"terraview/internal/graph"
)

const handlerPlanJSON = `{
	"format_version": "1.2",
	"terraform_version": "1.11.4",
	"resource_changes": [
		{"address": "aws_instance.web", "type": "aws_instance", "name": "web", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}}
	]
}`

func demoJSON(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../demo/demo-plan.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return data
}

func testConfig(t *testing.T, plan *graph.Data) Config {
	return Config{
		Plan:     plan,
		DemoJSON: demoJSON(t),
		Index: func() ([]byte, error) {
			return []byte("<!doctype html><title>TerraView</title>"), nil
		},
	}
}

func decodeGraph(t *testing.T, body []byte) graph.Data {
	t.Helper()
	var got graph.Data
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return got
}

func TestHandleGraphGetServesLoadedPlan(t *testing.T) {
	plan := &graph.Data{
		Source: "stdin",
		Nodes:  []graph.Node{{ID: "aws_instance.web"}},
		Edges:  []graph.Edge{},
	}
	cfg := testConfig(t, plan)
	cfg.DemoJSON = nil

	rec := httptest.NewRecorder()
	New(cfg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	got := decodeGraph(t, rec.Body.Bytes())
	if got.Source != "stdin" {
		t.Errorf("source = %q, want stdin", got.Source)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].ID != "aws_instance.web" {
		t.Errorf("nodes = %+v, want the loaded plan", got.Nodes)
	}
}

func TestHandleGraphGetFallsBackToDemo(t *testing.T) {
	rec := httptest.NewRecorder()
	New(testConfig(t, nil)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	got := decodeGraph(t, rec.Body.Bytes())
	if got.Source != "demo" {
		t.Errorf("source = %q, want demo", got.Source)
	}
	if len(got.Nodes) != 15 {
		t.Errorf("nodes = %d, want 15", len(got.Nodes))
	}
}

func TestHandleGraphPostParsesUpload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/graph", strings.NewReader(handlerPlanJSON))
	rec := httptest.NewRecorder()
	New(testConfig(t, nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decodeGraph(t, rec.Body.Bytes())
	if got.Source != "upload" {
		t.Errorf("source = %q, want upload", got.Source)
	}
	if len(got.Nodes) != 1 {
		t.Errorf("nodes = %d, want 1", len(got.Nodes))
	}
}

func TestHandleGraphRejectsOtherMethods(t *testing.T) {
	rec := httptest.NewRecorder()
	New(testConfig(t, nil)).ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/graph", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDemoGraph(t *testing.T) {
	rec := httptest.NewRecorder()
	New(testConfig(t, nil)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/demo", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	got := decodeGraph(t, rec.Body.Bytes())
	if got.Source != "demo" {
		t.Errorf("source = %q, want demo", got.Source)
	}
	if len(got.Nodes) != 15 {
		t.Errorf("nodes = %d, want 15", len(got.Nodes))
	}
}

func TestHandleDemoGraphIncludesRepoContext(t *testing.T) {
	cfg := testConfig(t, nil)
	cfg.Repo = gitcontext.Context{Path: "/work/infra", Branch: "feature/plan-preview"}
	rec := httptest.NewRecorder()
	New(cfg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/demo", nil))

	got := decodeGraph(t, rec.Body.Bytes())
	if got.GitPath != cfg.Repo.Path {
		t.Errorf("git_path = %q, want %q", got.GitPath, cfg.Repo.Path)
	}
	if got.GitBranch != cfg.Repo.Branch {
		t.Errorf("git_branch = %q, want %q", got.GitBranch, cfg.Repo.Branch)
	}
}

func TestHandleIndexServesUI(t *testing.T) {
	rec := httptest.NewRecorder()
	New(testConfig(t, nil)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "TerraView") {
		t.Error("response does not contain the TerraView UI")
	}
}

func TestHandleIndexRejectsUnknownPath(t *testing.T) {
	rec := httptest.NewRecorder()
	New(testConfig(t, nil)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
