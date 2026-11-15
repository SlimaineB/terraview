package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
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
	Nodes             []GraphNode `json:"nodes"`
	Edges             []GraphEdge `json:"edges"`
	Stats             Stats       `json:"stats"`
	RawSummary        string      `json:"raw_summary"`
	HasDependencyData bool        `json:"has_dependency_data"`
	Source            string      `json:"source,omitempty"`
	GitPath           string      `json:"git_path,omitempty"`
	GitBranch         string      `json:"git_branch,omitempty"`
}

type repoContext struct {
	Path   string
	Branch string
}

func (rc repoContext) apply(data *GraphData) {
	if data == nil {
		return
	}
	data.GitPath = rc.Path
	data.GitBranch = rc.Branch
}

type Stats struct {
	TotalResources int            `json:"total_resources"`
	Actions        map[string]int `json:"actions"`
	Providers      map[string]int `json:"providers"`
}

// Demo plan embedded so the binary ships with a realistic example (any provider).
//
//go:embed demo/demo-plan.json
var sampleTerraformJSON string

//go:embed static/index.html
var embeddedIndexHTML []byte

func parseTerraformJSON(data []byte) (*GraphData, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	nodesMap := make(map[string]GraphNode)
	var edges []GraphEdge
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
		Nodes:             nodes,
		Edges:             edges,
		Stats:             stats,
		RawSummary:        fmt.Sprintf("Parsed %d resources and %d dependencies successfully.", len(nodes), len(edges)),
		HasDependencyData: hasDependencyData,
	}, nil
}

func parseModuleDependencies(mod map[string]interface{}, prefix string, nodesMap map[string]GraphNode, edges *[]GraphEdge, seen map[string]bool) {
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
func moduleInstancePrefixes(parentPrefix, moduleName string, nodesMap map[string]GraphNode) []string {
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

func appendEdge(edges *[]GraphEdge, seen map[string]bool, source, target string, nodesMap map[string]GraphNode) {
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
	*edges = append(*edges, GraphEdge{ID: edgeID, Source: source, Target: target})
}

// parsePriorStateDependencies supplements configuration references with the
// resolved depends_on values that Terraform recorded in prior_state.
func parsePriorStateDependencies(raw map[string]interface{}, nodesMap map[string]GraphNode, edges *[]GraphEdge, seen map[string]bool) bool {
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

func parseStateDependencies(mod map[string]interface{}, nodesMap map[string]GraphNode, edges *[]GraphEdge, seen map[string]bool) {
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

func parseStateJSON(raw map[string]interface{}, nodesMap map[string]GraphNode, edges *[]GraphEdge, actionsCount map[string]int, providersCount map[string]int, seen map[string]bool) {
	values, _ := raw["values"].(map[string]interface{})
	rootModule, _ := values["root_module"].(map[string]interface{})
	parseStateModule(rootModule, nodesMap, edges, actionsCount, providersCount, seen)
}

func parseStateModule(mod map[string]interface{}, nodesMap map[string]GraphNode, edges *[]GraphEdge, actionsCount map[string]int, providersCount map[string]int, seen map[string]bool) {
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

func writeGraphJSON(w http.ResponseWriter, data *GraphData) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func demoGraph() (*GraphData, error) {
	data, err := parseTerraformJSON([]byte(sampleTerraformJSON))
	if err != nil {
		return nil, err
	}
	data.Source = "demo"
	return data, nil
}

func handleGraph(plan *GraphData, repo repoContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if plan != nil {
				repo.apply(plan)
				writeGraphJSON(w, plan)
				return
			}
			data, err := demoGraph()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			repo.apply(data)
			writeGraphJSON(w, data)
		case http.MethodPost:
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
			data.Source = "upload"
			repo.apply(data)
			writeGraphJSON(w, data)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleDemoGraph(repo repoContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := demoGraph()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		repo.apply(data)
		writeGraphJSON(w, data)
	}
}

func loadIndexHTML() ([]byte, error) {
	// Prefer the on-disk file during development so UI edits are picked up.
	if data, err := os.ReadFile("static/index.html"); err == nil {
		return data, nil
	}
	if len(embeddedIndexHTML) > 0 {
		return embeddedIndexHTML, nil
	}
	return nil, fmt.Errorf("static/index.html not found")
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	data, err := loadIndexHTML()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func readPipedPlan() ([]byte, bool) {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 100*1024*1024))
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return nil, false
	}
	return data, true
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func gitOutput(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectRepoContext(startDir string) repoContext {
	if startDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			startDir = cwd
		}
	}
	if startDir == "" {
		return repoContext{}
	}

	root := gitOutput(startDir, "rev-parse", "--show-toplevel")
	if root == "" {
		// A plan can live outside the work tree (for example /tmp/plan.json)
		// while TerraView is started from a repository. Prefer that context.
		if cwd, err := os.Getwd(); err == nil && cwd != startDir {
			root = gitOutput(cwd, "rev-parse", "--show-toplevel")
		}
	}
	if root == "" {
		return repoContext{}
	}

	branch := gitOutput(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if branch == "" {
		if sha := gitOutput(root, "rev-parse", "--short", "HEAD"); sha != "" {
			branch = "detached @ " + sha
		}
	}
	return repoContext{Path: root, Branch: branch}
}

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	staticMode := flag.Bool("static", false, "Export a standalone HTML report instead of starting the HTTP server")
	inputFile := flag.String("in", "", "Input Terraform plan/state JSON file (or pipe JSON on stdin)")
	outputFile := flag.String("out", "terraview-report.html", "Output HTML file path (for -static mode)")
	portFlag := flag.String("port", "", "HTTP port (default $PORT or 8080)")
	openFlag := flag.Bool("open", false, "Open the plan in the browser after starting the server (automatic when piping a plan)")
	planName := flag.String("name", "", "Label for the loaded plan (default: input filename or \"stdin\")")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	openExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "open" {
			openExplicit = true
		}
	})

	var planData []byte
	source := ""
	fromStdin := false
	if *inputFile != "" {
		data, err := os.ReadFile(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input file: %v\n", err)
			os.Exit(1)
		}
		planData = data
		source = filepath.Base(*inputFile)
	} else if data, ok := readPipedPlan(); ok {
		planData = data
		source = "stdin"
		fromStdin = true
	}
	if *planName != "" {
		source = *planName
	}

	var plan *GraphData
	if len(planData) > 0 {
		parsed, err := parseTerraformJSON(planData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing Terraform JSON: %v\n", err)
			os.Exit(1)
		}
		parsed.Source = source
		plan = parsed
	}

	startDir := ""
	if *inputFile != "" {
		if abs, err := filepath.Abs(*inputFile); err == nil {
			startDir = filepath.Dir(abs)
		}
	}
	repo := detectRepoContext(startDir)
	repo.apply(plan)

	if *staticMode {
		if plan == nil {
			fmt.Fprintf(os.Stderr, "Error: provide -in <plan.json> or pipe Terraform JSON on stdin\n")
			os.Exit(1)
		}
		runStatic(plan, *outputFile)
		return
	}

	port := *portFlag
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/api/graph", handleGraph(plan, repo))
	http.HandleFunc("/api/demo", handleDemoGraph(repo))
	http.HandleFunc("/", handleIndex)

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://localhost:%s", port)
	fmt.Printf("TerraView running at %s\n", url)
	if plan != nil {
		fmt.Printf("Loaded %s: %d resources, %d dependencies\n", source, len(plan.Nodes), len(plan.Edges))
	}
	if *openFlag || (fromStdin && !openExplicit) {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "Could not open browser automatically: %v\n", err)
		}
	}
	if err := http.Serve(listener, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}

func runStatic(graphData *GraphData, outputFile string) {
	htmlTemplate, err := loadIndexHTML()
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
