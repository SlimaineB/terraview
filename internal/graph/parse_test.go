package graph

import (
	"os"
	"testing"
)

func edgeIDs(g *Data) map[string]bool {
	ids := make(map[string]bool, len(g.Edges))
	for _, e := range g.Edges {
		ids[e.ID] = true
	}
	return ids
}

func parseTestPlan(t *testing.T, plan string) *Data {
	t.Helper()
	g, err := Parse([]byte(plan))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return g
}

func TestParseDemoPlanDependencies(t *testing.T) {
	data, err := os.ReadFile("../../demo/demo-plan.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	g, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
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

func TestParseRealTerraformPlan(t *testing.T) {
	data, err := os.ReadFile("testdata/real-plan/plan.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	g, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3", len(g.Nodes))
	}
	if g.Stats.Actions["create"] != 3 {
		t.Errorf("create actions = %d, want 3", g.Stats.Actions["create"])
	}

	ids := edgeIDs(g)
	want := []string{
		"terraform_data.network->terraform_data.subnet",
		"terraform_data.subnet->terraform_data.cluster",
	}
	for _, id := range want {
		if !ids[id] {
			t.Errorf("missing edge %q", id)
		}
	}
	if len(g.Edges) != len(want) {
		t.Errorf("edges = %d, want %d (%v)", len(g.Edges), len(want), ids)
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

func TestParseReplaceActions(t *testing.T) {
	g := parseTestPlan(t, `{
		"format_version": "1.2",
		"terraform_version": "1.11.4",
		"resource_changes": [
			{"address": "aws_instance.delete_create", "type": "aws_instance", "name": "delete_create", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["delete", "create"], "before": {}, "after": {}}},
			{"address": "aws_instance.create_delete", "type": "aws_instance", "name": "create_delete", "provider_name": "registry.terraform.io/hashicorp/aws", "change": {"actions": ["create", "delete"], "before": {}, "after": {}}}
		]
	}`)

	for _, node := range g.Nodes {
		if node.Action != "replace" {
			t.Errorf("node %s action = %q, want replace", node.ID, node.Action)
		}
	}
	if g.Stats.Actions["replace"] != 2 {
		t.Errorf("replace actions = %d, want 2", g.Stats.Actions["replace"])
	}
	if g.Stats.Actions["delete"] != 0 || g.Stats.Actions["create"] != 0 {
		t.Errorf("create/delete actions = %d/%d, want 0/0", g.Stats.Actions["create"], g.Stats.Actions["delete"])
	}
}

func TestParsePlanWithoutResourceChanges(t *testing.T) {
	g := parseTestPlan(t, `{
		"format_version": "1.1",
		"terraform_version": "1.6.6",
		"planned_values": {
			"root_module": {
				"resources": [
					{"address": "aws_vpc.main", "mode": "managed", "type": "aws_vpc", "name": "main", "provider_name": "registry.terraform.io/hashicorp/aws", "values": {}},
					{"address": "aws_subnet.private", "mode": "managed", "type": "aws_subnet", "name": "private", "provider_name": "registry.terraform.io/hashicorp/aws", "values": {}}
				]
			}
		},
		"configuration": {
			"root_module": {
				"resources": [
					{"address": "aws_vpc.main", "expressions": {}},
					{"address": "aws_subnet.private", "expressions": {"vpc_id": {"references": ["aws_vpc.main"]}}}
				]
			}
		}
	}`)

	if len(g.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(g.Nodes))
	}
	for _, node := range g.Nodes {
		if node.Action != "no-op" {
			t.Errorf("node %s action = %q, want no-op", node.ID, node.Action)
		}
	}
	if g.Stats.Actions["no-op"] != 2 {
		t.Errorf("no-op actions = %d, want 2", g.Stats.Actions["no-op"])
	}
	if ids := edgeIDs(g); !ids["aws_vpc.main->aws_subnet.private"] {
		t.Errorf("missing edge aws_vpc.main->aws_subnet.private (%v)", ids)
	}
}

func TestParseEmptyPlan(t *testing.T) {
	g := parseTestPlan(t, `{"format_version": "1.1", "terraform_version": "1.6.6", "planned_values": {"root_module": {}}}`)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("graph = %d nodes, %d edges, want empty", len(g.Nodes), len(g.Edges))
	}
}
