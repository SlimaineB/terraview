package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

var version = "dev"

type GraphNode struct {
	ID       string                 `json:"id"`
	Label    string                 `json:"label"`
	Type     string                 `json:"type"`
	Provider string                 `json:"provider"`
	Name     string                 `json:"name"`
	Action   string                 `json:"action"`
	Group    string                 `json:"group"`
	Before   map[string]interface{} `json:"before"`
	After    map[string]interface{} `json:"after"`
}

type GraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphData struct {
	Nodes      []GraphNode `json:"nodes"`
	Edges      []GraphEdge `json:"edges"`
	Stats      Stats       `json:"stats"`
	RawSummary string      `json:"raw_summary"`
}

type Stats struct {
	TotalResources int            `json:"total_resources"`
	Actions        map[string]int `json:"actions"`
	Providers      map[string]int `json:"providers"`
}

// Comprehensive Full AWS Enterprise Multi-Module Terraform Plan JSON with explicit before/after changes
const sampleTerraformJSON = `{
  "format_version": "1.1",
  "terraform_version": "1.5.0",
  "resource_changes": [
    {
      "address": "aws_iam_role.cluster_role",
      "mode": "managed",
      "type": "aws_iam_role",
      "name": "cluster_role",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["no-op"], "before": {"name": "cluster_role", "arn": "arn:aws:iam::123456789012:role/cluster"}, "after": {"name": "cluster_role", "arn": "arn:aws:iam::123456789012:role/cluster"}}
    },
    {
      "address": "aws_vpc.main",
      "mode": "managed",
      "type": "aws_vpc",
      "name": "main",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["no-op"], "before": {"cidr_block": "10.0.0.0/16", "enable_dns_hostnames": true}, "after": {"cidr_block": "10.0.0.0/16", "enable_dns_hostnames": true}}
    },
    {
      "address": "module.vpc.aws_subnet.public_1",
      "mode": "managed",
      "type": "aws_subnet",
      "name": "public_1",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["create"], "before": null, "after": {"cidr_block": "10.0.1.0/24", "map_public_ip_on_launch": true}}
    },
    {
      "address": "module.vpc.aws_subnet.public_2",
      "mode": "managed",
      "type": "aws_subnet",
      "name": "public_2",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["create"], "before": null, "after": {"cidr_block": "10.0.2.0/24", "map_public_ip_on_launch": true}}
    },
    {
      "address": "module.vpc.aws_subnet.private_1",
      "mode": "managed",
      "type": "aws_subnet",
      "name": "private_1",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["create"], "before": null, "after": {"cidr_block": "10.0.10.0/24", "map_public_ip_on_launch": false}}
    },
    {
      "address": "module.vpc.aws_internet_gateway.igw",
      "mode": "managed",
      "type": "aws_internet_gateway",
      "name": "igw",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["no-op"], "before": {"tags": {"Env": "prod"}}, "after": {"tags": {"Env": "prod"}}}
    },
    {
      "address": "module.vpc.aws_route_table.public",
      "mode": "managed",
      "type": "aws_route_table",
      "name": "public",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {
        "actions": ["update"],
        "before": {"propagating_vgws": [], "tags": {"Name": "public-rt-old"}} ,
        "after": {"propagating_vgws": ["vgw-12345"], "tags": {"Name": "public-rt-new"}}
      }
    },
    {
      "address": "module.security.aws_security_group.lb_sg",
      "mode": "managed",
      "type": "aws_security_group",
      "name": "lb_sg",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["create"], "before": null, "after": {"name": "lb-security-group", "ingress_ports": [80, 443]}}
    },
    {
      "address": "module.security.aws_security_group.eks_nodes_sg",
      "mode": "managed",
      "type": "aws_security_group",
      "name": "eks_nodes_sg",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["create"], "before": null, "after": {"name": "eks-nodes-sg", "ingress_ports": [443, 10250]}}
    },
    {
      "address": "module.database.aws_db_subnet_group.db_subnet",
      "mode": "managed",
      "type": "aws_db_subnet_group",
      "name": "db_subnet",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["create"], "before": null, "after": {"name": "main-db-subnet"}}
    },
    {
      "address": "module.database.aws_db_instance.postgres",
      "mode": "managed",
      "type": "aws_db_instance",
      "name": "postgres",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {
        "actions": ["update"],
        "before": {"allocated_storage": 20, "instance_class": "db.t3.micro", "engine_version": "14.5"},
        "after": {"allocated_storage": 50, "instance_class": "db.t3.medium", "engine_version": "14.7"}
      }
    },
    {
      "address": "module.eks.aws_eks_cluster.main",
      "mode": "managed",
      "type": "aws_eks_cluster",
      "name": "main",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["create"], "before": null, "after": {"name": "production-eks", "version": "1.28"}}
    },
    {
      "address": "module.eks.aws_eks_node_group.nodes",
      "mode": "managed",
      "type": "aws_eks_node_group",
      "name": "nodes",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {"actions": ["create"], "before": null, "after": {"instance_types": ["t3.xlarge"], "scaling_config": {"desired_size": 3}}}
    },
    {
      "address": "aws_s3_bucket.legacy_archive_bucket",
      "mode": "managed",
      "type": "aws_s3_bucket",
      "name": "legacy_archive_bucket",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {
        "actions": ["delete"],
        "before": {"bucket": "legacy-archive-2023", "acl": "private", "versioning": [{"enabled": true}], "tags": {"Env": "archive", "ManagedBy": "terraform"}, "force_destroy": false},
        "after": null
      }
    },
    {
      "address": "aws_security_group_rule.old_ingress_rule",
      "mode": "managed",
      "type": "aws_security_group_rule",
      "name": "old_ingress_rule",
      "provider_name": "registry.terraform.io/hashicorp/aws",
      "change": {
        "actions": ["delete"],
        "before": {"type": "ingress", "from_port": 3389, "to_port": 3389, "protocol": "tcp", "cidr_blocks": ["0.0.0.0/0"], "description": "RDP from everywhere"},
        "after": null
      }
    }
  ],
  "configuration": {
    "root_module": {
      "resources": [
        {"address": "aws_iam_role.cluster_role", "expressions": {}},
        {"address": "aws_vpc.main", "expressions": {}}
      ],
      "module_calls": {
        "vpc": {
          "module": {
            "resources": [
              {
                "address": "module.vpc.aws_subnet.public_1",
                "expressions": {"vpc_id": {"references": ["aws_vpc.main"]}}
              },
              {
                "address": "module.vpc.aws_subnet.public_2",
                "expressions": {"vpc_id": {"references": ["aws_vpc.main"]}}
              },
              {
                "address": "module.vpc.aws_subnet.private_1",
                "expressions": {"vpc_id": {"references": ["aws_vpc.main"]}}
              },
              {
                "address": "module.vpc.aws_internet_gateway.igw",
                "expressions": {"vpc_id": {"references": ["aws_vpc.main"]}}
              },
              {
                "address": "module.vpc.aws_route_table.public",
                "expressions": {
                  "vpc_id": {"references": ["aws_vpc.main"]},
                  "gateway_id": {"references": ["module.vpc.aws_internet_gateway.igw"]}
                }
              }
            ]
          }
        },
        "security": {
          "module": {
            "resources": [
              {
                "address": "module.security.aws_security_group.lb_sg",
                "expressions": {"vpc_id": {"references": ["aws_vpc.main"]}}
              },
              {
                "address": "module.security.aws_security_group.eks_nodes_sg",
                "expressions": {"vpc_id": {"references": ["aws_vpc.main"]}}
              }
            ]
          }
        },
        "database": {
          "module": {
            "resources": [
              {
                "address": "module.database.aws_db_subnet_group.db_subnet",
                "expressions": {"subnet_ids": {"references": ["module.vpc.aws_subnet.private_1"]}}
              },
              {
                "address": "module.database.aws_db_instance.postgres",
                "expressions": {
                  "db_subnet_group_name": {"references": ["module.database.aws_db_subnet_group.db_subnet"]},
                  "vpc_security_group_ids": {"references": ["module.security.aws_security_group.eks_nodes_sg"]}
                }
              }
            ]
          }
        },
        "eks": {
          "module": {
            "resources": [
              {
                "address": "module.eks.aws_eks_cluster.main",
                "expressions": {
                  "role_arn": {"references": ["aws_iam_role.cluster_role"]},
                  "subnet_ids": {"references": ["module.vpc.aws_subnet.public_1"]}
                }
              },
              {
                "address": "module.eks.aws_eks_node_group.nodes",
                "expressions": {
                  "cluster_name": {"references": ["module.eks.aws_eks_cluster.main"]},
                  "node_role_arn": {"references": ["aws_iam_role.cluster_role"]},
                  "subnet_ids": {"references": ["module.vpc.aws_subnet.private_1"]}
                }
              }
            ]
          }
        }
      }
    }
  }
}`

func parseTerraformJSON(data []byte) (*GraphData, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	nodesMap := make(map[string]GraphNode)
	var edges []GraphEdge
	actionsCount := make(map[string]int)
	providersCount := make(map[string]int)

	if rc, ok := raw["resource_changes"].([]interface{}); ok {
		for _, item := range rc {
			res, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			addr, _ := res["address"].(string)
			if addr == "" {
				continue
			}
			resType, _ := res["type"].(string)
			resName, _ := res["name"].(string)
			providerName, _ := res["provider_name"].(string)

			action := "no-op"
			var beforeMap, afterMap map[string]interface{}

			if change, ok := res["change"].(map[string]interface{}); ok {
				if acts, ok := change["actions"].([]interface{}); ok && len(acts) > 0 {
					if actStr, ok := acts[0].(string); ok {
						action = actStr
					}
				}
				if b, ok := change["before"].(map[string]interface{}); ok {
					beforeMap = b
				}
				if a, ok := change["after"].(map[string]interface{}); ok {
					afterMap = a
				}
			}

			providerShort := extractProviderShort(providerName)
			group := extractGroup(resType, providerName)

			actionsCount[action]++
			providersCount[providerShort]++

			nodesMap[addr] = GraphNode{
				ID:       addr,
				Label:    addr,
				Type:     resType,
				Provider: providerShort,
				Name:     resName,
				Action:   action,
				Group:    group,
				Before:   beforeMap,
				After:    afterMap,
			}
		}

		if config, ok := raw["configuration"].(map[string]interface{}); ok {
			if rootMod, ok := config["root_module"].(map[string]interface{}); ok {
				parseModuleDependencies(rootMod, nodesMap, &edges)
			}
		}
	} else if _, ok := raw["values"].(map[string]interface{}); ok {
		parseStateJSON(raw, nodesMap, &edges, actionsCount, providersCount)
	} else {
		return nil, fmt.Errorf("unrecognized terraform JSON format")
	}

	var nodes []GraphNode
	for _, n := range nodesMap {
		nodes = append(nodes, n)
	}

	stats := Stats{
		TotalResources: len(nodes),
		Actions:        actionsCount,
		Providers:      providersCount,
	}

	return &GraphData{
		Nodes:      nodes,
		Edges:      edges,
		Stats:      stats,
		RawSummary: fmt.Sprintf("Parsed %d resources and %d dependencies successfully.", len(nodes), len(edges)),
	}, nil
}

func parseModuleDependencies(mod map[string]interface{}, nodesMap map[string]GraphNode, edges *[]GraphEdge) {
	if resources, ok := mod["resources"].([]interface{}); ok {
		for _, r := range resources {
			resMap, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			addr, _ := resMap["address"].(string)
			if _, exists := nodesMap[addr]; !exists {
				continue
			}

			if exprs, ok := resMap["expressions"].(map[string]interface{}); ok {
				for _, exprVal := range exprs {
					exprMap, ok := exprVal.(map[string]interface{})
					if !ok {
						continue
					}
					if refs, ok := exprMap["references"].([]interface{}); ok {
						for _, ref := range refs {
							refStr, _ := ref.(string)
							targetAddr := cleanResourceAddress(refStr)
							if targetAddr != "" && targetAddr != addr {
								if _, targetExists := nodesMap[targetAddr]; targetExists {
									edgeID := fmt.Sprintf("%s->%s", targetAddr, addr)
									*edges = append(*edges, GraphEdge{
										ID:     edgeID,
										Source: targetAddr,
										Target: addr,
									})
								}
							}
						}
					}
				}
			}
		}
	}

	if childMods, ok := mod["module_calls"].(map[string]interface{}); ok {
		for _, cm := range childMods {
			cmMap, ok := cm.(map[string]interface{})
			if !ok {
				continue
			}
			if submodule, ok := cmMap["module"].(map[string]interface{}); ok {
				parseModuleDependencies(submodule, nodesMap, edges)
			}
		}
	}
}

func parseStateJSON(raw map[string]interface{}, nodesMap map[string]GraphNode, edges *[]GraphEdge, actionsCount map[string]int, providersCount map[string]int) {
	values, _ := raw["values"].(map[string]interface{})
	rootModule, _ := values["root_module"].(map[string]interface{})
	parseStateModule(rootModule, nodesMap, edges, actionsCount, providersCount)
}

func parseStateModule(mod map[string]interface{}, nodesMap map[string]GraphNode, edges *[]GraphEdge, actionsCount map[string]int, providersCount map[string]int) {
	if resources, ok := mod["resources"].([]interface{}); ok {
		for _, r := range resources {
			resMap, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			addr, _ := resMap["address"].(string)
			resType, _ := resMap["type"].(string)
			resName, _ := resMap["name"].(string)
			providerName, _ := resMap["provider_name"].(string)

			if addr == "" {
				continue
			}

			providerShort := extractProviderShort(providerName)
			group := extractGroup(resType, providerName)
			actionsCount["no-op"]++
			providersCount[providerShort]++

			nodesMap[addr] = GraphNode{
				ID:       addr,
				Label:    addr,
				Type:     resType,
				Provider: providerShort,
				Name:     resName,
				Action:   "no-op",
				Group:    group,
			}
		}
	}

	if childModules, ok := mod["child_modules"].([]interface{}); ok {
		for _, cm := range childModules {
			if cmMap, ok := cm.(map[string]interface{}); ok {
				parseStateModule(cmMap, nodesMap, edges, actionsCount, providersCount)
			}
		}
	}
}

func cleanResourceAddress(ref string) string {
	parts := strings.Split(ref, ".")
	if len(parts) >= 4 && parts[0] == "module" {
		return fmt.Sprintf("module.%s.%s.%s", parts[1], parts[2], parts[3])
	}
	if len(parts) >= 2 {
		return fmt.Sprintf("%s.%s", parts[0], parts[1])
	}
	return ""
}

func extractProviderShort(prov string) string {
	parts := strings.Split(prov, "/")
	name := parts[len(parts)-1]
	name = strings.TrimPrefix(name, "terraform-provider-")
	if name == "" {
		return "aws"
	}
	return name
}

func extractGroup(resType, providerName string) string {
	// Derive group from provider name first (most reliable)
	providerLower := strings.ToLower(providerName)
	resTypeLower := strings.ToLower(resType)

	if strings.Contains(providerLower, "aws") || strings.HasPrefix(resTypeLower, "aws_") {
		return "aws"
	}
	if strings.Contains(providerLower, "azurerm") || strings.Contains(providerLower, "azure") || strings.HasPrefix(resTypeLower, "azurerm_") || strings.HasPrefix(resTypeLower, "azure_") {
		return "azure"
	}
	if strings.Contains(providerLower, "google") || strings.Contains(providerLower, "gcp") || strings.HasPrefix(resTypeLower, "google_") {
		return "gcp"
	}
	if strings.Contains(providerLower, "kubernetes") || strings.Contains(providerLower, "k8s") || strings.HasPrefix(resTypeLower, "kubernetes_") || strings.HasPrefix(resTypeLower, "helm_") {
		return "k8s"
	}
	if strings.Contains(providerLower, "docker") || strings.HasPrefix(resTypeLower, "docker_") {
		return "docker"
	}
	if strings.Contains(providerLower, "helm") {
		return "helm"
	}
	return "other"
}

func handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data, err := parseTerraformJSON([]byte(sampleTerraformJSON))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
		return
	}

	if r.Method == http.MethodPost {
		body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		data, err := parseTerraformJSON(body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error parsing Terraform JSON: %v", err), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func main() {
	staticMode := flag.Bool("static", false, "Export a standalone HTML report instead of starting the HTTP server")
	inputFile := flag.String("in", "", "Input Terraform plan JSON file (for -static mode)")
	outputFile := flag.String("out", "terraview-report.html", "Output HTML file path (for -static mode)")
	flag.Parse()

	if *staticMode {
		if *inputFile == "" {
			fmt.Fprintf(os.Stderr, "Error: -in is required when using -static\n")
			os.Exit(1)
		}
		runStatic(*inputFile, *outputFile)
		return
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/api/graph", handleGraph)
	http.Handle("/", http.FileServer(http.Dir("./static")))

	fmt.Printf("TerraView running at http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func runStatic(inputFile, outputFile string) {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input file: %v\n", err)
		os.Exit(1)
	}

	graphData, err := parseTerraformJSON(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing Terraform JSON: %v\n", err)
		os.Exit(1)
	}

	htmlTemplate, err := os.ReadFile("static/index.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading static template: %v\n", err)
		os.Exit(1)
	}

	graphJSON, err := json.Marshal(graphData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling graph data: %v\n", err)
		os.Exit(1)
	}

	embedded := fmt.Sprintf(`<script id="terraview-embedded-data" type="application/json" style="display:none;">%s</script>`, string(graphJSON))

	html := bytes.Replace(htmlTemplate, []byte(`<script id="terraview-embedded-data" type="application/json" style="display:none;"></script>`), []byte(embedded), 1)

	if err := os.WriteFile(outputFile, html, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Static report written to %s (%d resources, %d dependencies)\n", outputFile, len(graphData.Nodes), len(graphData.Edges))
}
