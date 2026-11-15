package graph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func Parse(data []byte) (*Data, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	nodesMap := make(map[string]Node)
	var edges []Edge
	seenEdges := make(map[string]bool)
	hasDependencyData := false
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

			nodesMap[addr] = Node{
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
				hasDependencyData = true
				parseModuleDependencies(rootMod, "", nodesMap, &edges, seenEdges)
			}
		}
	} else if _, ok := raw["values"].(map[string]interface{}); ok {
		hasDependencyData = true
		parseStateJSON(raw, nodesMap, &edges, actionsCount, providersCount, seenEdges)
	} else {
		return nil, fmt.Errorf("unrecognized terraform JSON format")
	}

	if parsePriorStateDependencies(raw, nodesMap, &edges, seenEdges) {
		hasDependencyData = true
	}

	var nodes []Node
	for _, n := range nodesMap {
		nodes = append(nodes, n)
	}

	stats := Stats{
		TotalResources: len(nodes),
		Actions:        actionsCount,
		Providers:      providersCount,
	}

	return &Data{
		Nodes:             nodes,
		Edges:             edges,
		Stats:             stats,
		RawSummary:        fmt.Sprintf("Parsed %d resources and %d dependencies successfully.", len(nodes), len(edges)),
		HasDependencyData: hasDependencyData,
	}, nil
}

func parseModuleDependencies(mod map[string]interface{}, prefix string, nodesMap map[string]Node, edges *[]Edge, seen map[string]bool) {
	// Configuration addresses are relative to the module for instances created
	// with count/for_each, but root-relative for plain modules. Resolve both.
	resolveAddress := func(addr string) string {
		if addr == "" {
			return ""
		}
		if prefix != "" {
			candidate := prefix + "." + addr
			if _, ok := nodesMap[candidate]; ok {
				return candidate
			}
		}
		if _, ok := nodesMap[addr]; ok {
			return addr
		}
		return ""
	}

	if resources, ok := mod["resources"].([]interface{}); ok {
		for _, r := range resources {
			resMap, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			localAddr, _ := resMap["address"].(string)
			addr := resolveAddress(localAddr)
			if addr == "" {
				continue
			}

			if exprs, ok := resMap["expressions"].(map[string]interface{}); ok {
				collectReferences(exprs, func(ref string) {
					appendEdge(edges, seen, resolveAddress(cleanResourceAddress(ref)), addr, nodesMap)
				})
			}

			if deps, ok := resMap["depends_on"].([]interface{}); ok {
				for _, d := range deps {
					depStr, _ := d.(string)
					appendEdge(edges, seen, resolveAddress(cleanResourceAddress(depStr)), addr, nodesMap)
				}
			}
		}
	}

	if childMods, ok := mod["module_calls"].(map[string]interface{}); ok {
		for name, cm := range childMods {
			cmMap, ok := cm.(map[string]interface{})
			if !ok {
				continue
			}
			submodule, ok := cmMap["module"].(map[string]interface{})
			if !ok {
				continue
			}
			for _, childPrefix := range moduleInstancePrefixes(prefix, name, nodesMap) {
				parseModuleDependencies(submodule, childPrefix, nodesMap, edges, seen)
			}
		}
	}
}

func collectReferences(value interface{}, visit func(string)) {
	switch v := value.(type) {
	case map[string]interface{}:
		if refs, ok := v["references"].([]interface{}); ok {
			for _, ref := range refs {
				if refStr, ok := ref.(string); ok {
					visit(refStr)
				}
			}
		}
		for _, child := range v {
			collectReferences(child, visit)
		}
	case []interface{}:
		for _, child := range v {
			collectReferences(child, visit)
		}
	}
}

// moduleInstancePrefixes finds the concrete addresses of a module call,
// e.g. module.app["prod"] or module.vpc[0], by inspecting resource addresses.
func moduleInstancePrefixes(parentPrefix, moduleName string, nodesMap map[string]Node) []string {
	base := "module." + moduleName
	if parentPrefix != "" {
		base = parentPrefix + "." + base
	}

	prefixes := make(map[string]bool)
	for addr := range nodesMap {
		if !strings.HasPrefix(addr, base) {
			continue
		}
		rest := addr[len(base):]
		if rest == "" || (rest[0] != '.' && rest[0] != '[') {
			continue
		}
		instance := base
		if rest[0] == '[' {
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				continue
			}
			instance = base + rest[:end+1]
		}
		prefixes[instance] = true
	}

	result := make([]string, 0, len(prefixes))
	for p := range prefixes {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}

func appendEdge(edges *[]Edge, seen map[string]bool, source, target string, nodesMap map[string]Node) {
	if source == "" || target == "" || source == target {
		return
	}
	if _, ok := nodesMap[source]; !ok {
		return
	}
	if _, ok := nodesMap[target]; !ok {
		return
	}
	edgeID := fmt.Sprintf("%s->%s", source, target)
	if seen[edgeID] {
		return
	}
	seen[edgeID] = true
	*edges = append(*edges, Edge{ID: edgeID, Source: source, Target: target})
}

// parsePriorStateDependencies supplements configuration references with the
// resolved depends_on values that Terraform recorded in prior_state.
func parsePriorStateDependencies(raw map[string]interface{}, nodesMap map[string]Node, edges *[]Edge, seen map[string]bool) bool {
	priorState, ok := raw["prior_state"].(map[string]interface{})
	if !ok {
		return false
	}
	values, ok := priorState["values"].(map[string]interface{})
	if !ok {
		return false
	}
	rootModule, ok := values["root_module"].(map[string]interface{})
	if !ok {
		return false
	}

	before := len(*edges)
	parseStateDependencies(rootModule, nodesMap, edges, seen)
	return len(*edges) > before
}

func parseStateDependencies(mod map[string]interface{}, nodesMap map[string]Node, edges *[]Edge, seen map[string]bool) {
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
			if deps, ok := resMap["depends_on"].([]interface{}); ok {
				for _, d := range deps {
					depStr, _ := d.(string)
					appendEdge(edges, seen, cleanResourceAddress(depStr), addr, nodesMap)
				}
			}
		}
	}

	if childModules, ok := mod["child_modules"].([]interface{}); ok {
		for _, cm := range childModules {
			if cmMap, ok := cm.(map[string]interface{}); ok {
				parseStateDependencies(cmMap, nodesMap, edges, seen)
			}
		}
	}
}

func parseStateJSON(raw map[string]interface{}, nodesMap map[string]Node, edges *[]Edge, actionsCount map[string]int, providersCount map[string]int, seen map[string]bool) {
	values, _ := raw["values"].(map[string]interface{})
	rootModule, _ := values["root_module"].(map[string]interface{})
	parseStateModule(rootModule, nodesMap, edges, actionsCount, providersCount, seen)
}

func parseStateModule(mod map[string]interface{}, nodesMap map[string]Node, edges *[]Edge, actionsCount map[string]int, providersCount map[string]int, seen map[string]bool) {
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

			nodesMap[addr] = Node{
				ID:       addr,
				Label:    addr,
				Type:     resType,
				Provider: providerShort,
				Name:     resName,
				Action:   "no-op",
				Group:    group,
			}

			if deps, ok := resMap["depends_on"].([]interface{}); ok {
				for _, d := range deps {
					depStr, _ := d.(string)
					appendEdge(edges, seen, cleanResourceAddress(depStr), addr, nodesMap)
				}
			}
		}
	}

	if childModules, ok := mod["child_modules"].([]interface{}); ok {
		for _, cm := range childModules {
			if cmMap, ok := cm.(map[string]interface{}); ok {
				parseStateModule(cmMap, nodesMap, edges, actionsCount, providersCount, seen)
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
