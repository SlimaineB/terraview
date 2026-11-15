package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func edgeIDs(g *GraphData) map[string]bool {
	ids := make(map[string]bool, len(g.Edges))
	for _, e := range g.Edges {
		ids[e.ID] = true
	}
	return ids
}

func parseTestPlan(t *testing.T, plan string) *GraphData {
	t.Helper()
	g, err := parseTerraformJSON([]byte(plan))
	if err != nil {
		t.Fatalf("parseTerraformJSON() error = %v", err)
	}
	return g
}

func TestParseDemoPlanDependencies(t *testing.T) {
	data, err := os.ReadFile("demo/demo-plan.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	g, err := parseTerraformJSON(data)
	if err != nil {
		t.Fatalf("parseTerraformJSON() error = %v", err)
	}
	if len(g.Nodes) != 15 {
		t.Errorf("nodes = %d, want 15", len(g.Nodes))
	}
	if len(g.Edges) != 16 {
		t.Errorf("edges = %d, want 16", len(g.Edges))
	}
	if !g.HasDependencyData {
		t.Error("HasDependencyData = false, want true")
	}
}

func TestParseForEachModuleDependencies(t *testing.T) {
	g := parseTestPlan(t, `{
		"format_version": "1.2",
		"terraform_version": "1.11.4",
		"resource_changes": [
			{"address": "aws_vpc.main", "type": "aws_vpc", "name": "main", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["no-op"], "before": {}, "after": {}}},
			{"address": "module.app[\"chrono\"].kubernetes_manifest.namespace", "type": "kubernetes_manifest", "name": "namespace", "provider_name": "registry.terraform.io/hashicorp/kubernetes", "change": {"actions": ["create"], "before": null, "after": {}}},
			{"address": "module.app[\"chrono\"].kubernetes_manifest.argocd_app", "type": "kubernetes_manifest", "name": "argocd_app", "provider_name": "registry.terraform.io/hashicorp/kubernetes", "change": {"actions": ["create"], "before": null, "after": {}}},
			{"address": "module.app[\"chrono\"].module.db.aws_db_instance.main", "type": "aws_db_instance", "name": "main", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}},
			{"address": "module.app[\"chrono\"].module.db.aws_db_instance.replica", "type": "aws_db_instance", "name": "replica", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}}
		],
		"configuration": {
			"root_module": {
				"module_calls": {
					"app": {
						"module": {
							"resources": [
								{"address": "kubernetes_manifest.namespace", "expressions": {}},
								{"address": "kubernetes_manifest.argocd_app", "expressions": {"namespace": {"references": ["kubernetes_manifest.namespace", "var.namespace"]}}}
							],
							"module_calls": {
								"db": {
									"module": {
										"resources": [
											{"address": "aws_db_instance.main", "expressions": {}},
											{"address": "aws_db_instance.replica", "expressions": {"source_id": {"references": ["aws_db_instance.main.id"]}}}
										]
									}
								}
							}
						}
					}
				}
			}
		},
		"prior_state": {
			"values": {
				"root_module": {
					"resources": [
						{"address": "module.app[\"chrono\"].kubernetes_manifest.namespace", "depends_on": ["aws_vpc.main"]}
					],
					"child_modules": [
						{
							"address": "module.app[\"chrono\"]",
							"child_modules": [
								{
									"address": "module.app[\"chrono\"].module.db",
									"resources": [
										{"address": "module.app[\"chrono\"].module.db.aws_db_instance.main", "depends_on": ["aws_vpc.main"]}
									]
								}
							]
						}
					]
				}
			}
		}
	}`)

	if len(g.Nodes) != 5 {
		t.Fatalf("nodes = %d, want 5", len(g.Nodes))
	}

	ids := edgeIDs(g)
	want := []string{
		"module.app[\"chrono\"].kubernetes_manifest.namespace->module.app[\"chrono\"].kubernetes_manifest.argocd_app",
		"module.app[\"chrono\"].module.db.aws_db_instance.main->module.app[\"chrono\"].module.db.aws_db_instance.replica",
		"aws_vpc.main->module.app[\"chrono\"].kubernetes_manifest.namespace",
		"aws_vpc.main->module.app[\"chrono\"].module.db.aws_db_instance.main",
	}
	for _, id := range want {
		if !ids[id] {
			t.Errorf("missing edge %q", id)
		}
	}
	if len(g.Edges) != len(want) {
		t.Errorf("edges = %d, want %d (%v)", len(g.Edges), len(want), ids)
	}
}

func TestParseCountModuleDependencies(t *testing.T) {
	g := parseTestPlan(t, `{
		"format_version": "1.2",
		"terraform_version": "1.11.4",
		"resource_changes": [
			{"address": "module.web[0].aws_security_group.sg", "type": "aws_security_group", "name": "sg", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}},
			{"address": "module.web[0].aws_instance.web", "type": "aws_instance", "name": "web", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}},
			{"address": "module.web[1].aws_security_group.sg", "type": "aws_security_group", "name": "sg", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}},
			{"address": "module.web[1].aws_instance.web", "type": "aws_instance", "name": "web", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}}
		],
		"configuration": {
			"root_module": {
				"module_calls": {
					"web": {
						"module": {
							"resources": [
								{"address": "aws_security_group.sg", "expressions": {}},
								{"address": "aws_instance.web", "expressions": {"security_groups": {"references": ["aws_security_group.sg.id"]}}}
							]
						}
					}
				}
			}
		}
	}`)

	ids := edgeIDs(g)
	want := []string{
		"module.web[0].aws_security_group.sg->module.web[0].aws_instance.web",
		"module.web[1].aws_security_group.sg->module.web[1].aws_instance.web",
	}
	for _, id := range want {
		if !ids[id] {
			t.Errorf("missing edge %q", id)
		}
	}
	if len(g.Edges) != len(want) {
		t.Errorf("edges = %d, want %d (%v)", len(g.Edges), len(want), ids)
	}
}

func TestParsePriorStateOnlyDependencies(t *testing.T) {
	g := parseTestPlan(t, `{
		"format_version": "1.2",
		"terraform_version": "1.11.4",
		"resource_changes": [
			{"address": "aws_vpc.main", "type": "aws_vpc", "name": "main", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["no-op"], "before": {}, "after": {}}},
			{"address": "aws_subnet.public", "type": "aws_subnet", "name": "public", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["no-op"], "before": {}, "after": {}}}
		],
		"prior_state": {
			"values": {
				"root_module": {
					"resources": [
						{"address": "aws_subnet.public", "depends_on": ["aws_vpc.main"]}
					]
				}
			}
		}
	}`)

	if len(g.Edges) != 1 {
		t.Fatalf("edges = %d, want 1 (%v)", len(g.Edges), edgeIDs(g))
	}
	if !g.HasDependencyData {
		t.Error("HasDependencyData = false, want true")
	}
}

func TestParseEdgesAreDeduplicated(t *testing.T) {
	g := parseTestPlan(t, `{
		"format_version": "1.2",
		"terraform_version": "1.11.4",
		"resource_changes": [
			{"address": "aws_vpc.main", "type": "aws_vpc", "name": "main", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["no-op"], "before": {}, "after": {}}},
			{"address": "aws_subnet.public", "type": "aws_subnet", "name": "public", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}}
		],
		"configuration": {
			"root_module": {
				"resources": [
					{"address": "aws_vpc.main", "expressions": {}},
					{"address": "aws_subnet.public", "expressions": {"vpc_id": {"references": ["aws_vpc.main.id", "aws_vpc.main.cidr_block"]}}}
				]
			}
		},
		"prior_state": {
			"values": {
				"root_module": {
					"resources": [
						{"address": "aws_subnet.public", "depends_on": ["aws_vpc.main"]}
					]
				}
			}
		}
	}`)

	if len(g.Edges) != 1 {
		t.Fatalf("edges = %d, want 1 (%v)", len(g.Edges), edgeIDs(g))
	}
	if g.Edges[0].ID != "aws_vpc.main->aws_subnet.public" {
		t.Errorf("edge = %q, want aws_vpc.main->aws_subnet.public", g.Edges[0].ID)
	}
}

const handlerPlanJSON = `{
	"format_version": "1.2",
	"terraform_version": "1.11.4",
	"resource_changes": [
		{"address": "aws_instance.web", "type": "aws_instance", "name": "web", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create"], "before": null, "after": {}}}
	]
}`

func decodeGraph(t *testing.T, body []byte) GraphData {
	t.Helper()
	var got GraphData
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return got
}

func TestHandleGraphGetServesLoadedPlan(t *testing.T) {
	plan := &GraphData{
		Source: "stdin",
		Nodes:  []GraphNode{{ID: "aws_instance.web"}},
		Edges:  []GraphEdge{},
	}

	rec := httptest.NewRecorder()
	handleGraph(plan, repoContext{})(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))

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
	handleGraph(nil, repoContext{})(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))

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
	handleGraph(nil, repoContext{})(rec, req)

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
	handleGraph(nil, repoContext{})(rec, httptest.NewRequest(http.MethodPut, "/api/graph", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDemoGraph(t *testing.T) {
	rec := httptest.NewRecorder()
	handleDemoGraph(repoContext{})(rec, httptest.NewRequest(http.MethodGet, "/api/demo", nil))

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
	repo := repoContext{Path: "/work/infra", Branch: "feature/plan-preview"}
	rec := httptest.NewRecorder()
	handleDemoGraph(repo)(rec, httptest.NewRequest(http.MethodGet, "/api/demo", nil))

	got := decodeGraph(t, rec.Body.Bytes())
	if got.GitPath != repo.Path {
		t.Errorf("git_path = %q, want %q", got.GitPath, repo.Path)
	}
	if got.GitBranch != repo.Branch {
		t.Errorf("git_branch = %q, want %q", got.GitBranch, repo.Branch)
	}
}

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

func TestDetectRepoContext(t *testing.T) {
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

	got := detectRepoContext(subdir)
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
	got = detectRepoContext(subdir)
	if !strings.HasPrefix(got.Branch, "detached @ ") {
		t.Errorf("branch = %q, want detached @ <sha>", got.Branch)
	}
}

func TestHandleIndexServesUI(t *testing.T) {
	rec := httptest.NewRecorder()
	handleIndex(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "TerraView") {
		t.Error("response does not contain the TerraView UI")
	}
}

func TestHandleIndexRejectsUnknownPath(t *testing.T) {
	rec := httptest.NewRecorder()
	handleIndex(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
