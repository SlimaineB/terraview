package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"terraview/internal/gitcontext"
	"terraview/internal/graph"
	"terraview/internal/manifest"
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

func planJSONWithAddress(address string) string {
	return fmt.Sprintf(`{
		"format_version": "1.2",
		"terraform_version": "1.11.4",
		"resource_changes": [
			{"address": %q, "type": "terraform_data", "name": "test", "provider_name": "terraform.io/builtin/terraform", "change": {"actions": ["create"], "before": null, "after": {}}}
		]
	}`, address)
}

func writePlanFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func TestHandlePlansListsManifestEntries(t *testing.T) {
	first := manifest.Plan{
		Entry: manifest.Entry{
			ID:        "storage/prod/default",
			Project:   "storage",
			Env:       "prod",
			Workspace: "default",
		},
		Path:   writePlanFile(t, planJSONWithAddress("terraform_data.storage")),
		Source: "plans/storage-prod.json",
	}
	second := manifest.Plan{
		Entry: manifest.Entry{
			ID:        "compute/prod/default",
			Project:   "compute",
			Env:       "prod",
			Workspace: "default",
		},
		Path:   writePlanFile(t, planJSONWithAddress("terraform_data.compute")),
		Source: "plans/compute-prod.json",
	}
	cfg := testConfig(t, nil)
	cfg.Plans = []manifest.Plan{first, second}

	rec := httptest.NewRecorder()
	New(cfg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/plans", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got plansResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got.Plans) != 2 {
		t.Fatalf("plans = %d, want 2", len(got.Plans))
	}
	if got.Plans[0].ID != first.ID || got.Plans[0].Label != "storage / prod / default" {
		t.Errorf("first plan = %+v, want storage/prod/default", got.Plans[0])
	}
	if got.Plans[1].Project != "compute" || got.Plans[1].Source != "plans/compute-prod.json" {
		t.Errorf("second plan = %+v, want compute plan metadata", got.Plans[1])
	}
}

func TestHandleGraphSelectsManifestPlan(t *testing.T) {
	first := manifest.Plan{
		Entry:  manifest.Entry{ID: "storage/prod/default", Project: "storage", Env: "prod", Workspace: "default"},
		Path:   writePlanFile(t, planJSONWithAddress("terraform_data.storage")),
		Source: "plans/storage-prod.json",
	}
	second := manifest.Plan{
		Entry:  manifest.Entry{ID: "compute/prod/default", Project: "compute", Env: "prod", Workspace: "default"},
		Path:   writePlanFile(t, planJSONWithAddress("terraform_data.compute")),
		Source: "plans/compute-prod.json",
	}
	cfg := testConfig(t, nil)
	cfg.Plans = []manifest.Plan{first, second}
	cfg.Repo = gitcontext.Context{Path: "/work/infra", Branch: "main"}

	rec := httptest.NewRecorder()
	New(cfg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph?plan=compute%2Fprod%2Fdefault", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decodeGraph(t, rec.Body.Bytes())
	if len(got.Nodes) != 1 || got.Nodes[0].ID != "terraform_data.compute" {
		t.Errorf("nodes = %+v, want compute plan", got.Nodes)
	}
	if got.PlanID != second.ID || got.PlanLabel != "compute / prod / default" {
		t.Errorf("plan metadata = %q/%q, want compute context", got.PlanID, got.PlanLabel)
	}
	if got.Project != "compute" || got.Env != "prod" || got.Workspace != "default" {
		t.Errorf("environment = %q/%q/%q, want compute/prod/default", got.Project, got.Env, got.Workspace)
	}
	if got.GitPath != cfg.Repo.Path || got.GitBranch != cfg.Repo.Branch {
		t.Errorf("git context = %q/%q, want %q/%q", got.GitPath, got.GitBranch, cfg.Repo.Path, cfg.Repo.Branch)
	}
}

func TestHandleGraphDefaultsToFirstManifestPlan(t *testing.T) {
	first := manifest.Plan{
		Entry:  manifest.Entry{ID: "storage/prod/default"},
		Path:   writePlanFile(t, planJSONWithAddress("terraform_data.storage")),
		Source: "storage-prod.json",
	}
	cfg := testConfig(t, nil)
	cfg.Plans = []manifest.Plan{first}

	rec := httptest.NewRecorder()
	New(cfg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decodeGraph(t, rec.Body.Bytes())
	if got.PlanID != first.ID {
		t.Errorf("plan_id = %q, want %q", got.PlanID, first.ID)
	}
}

func TestHandleGraphRejectsUnknownManifestPlan(t *testing.T) {
	cfg := testConfig(t, nil)
	cfg.Plans = []manifest.Plan{{
		Entry: manifest.Entry{ID: "storage/prod/default"},
		Path:  writePlanFile(t, planJSONWithAddress("terraform_data.storage")),
	}}
	rec := httptest.NewRecorder()
	New(cfg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph?plan=missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
