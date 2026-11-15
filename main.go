package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"terraview/internal/gitcontext"
	"terraview/internal/graph"
	"terraview/internal/report"
	"terraview/internal/server"
)

var version = "dev"

//go:embed demo/demo-plan.json
var demoPlanJSON []byte

//go:embed static/index.html
var embeddedIndexHTML []byte

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

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	staticMode := flag.Bool("static", false, "Export a standalone HTML report instead of starting the HTTP server")
	inputFile := flag.String("in", "", "Input Terraform plan/state JSON file (or pipe JSON on stdin)")
	outputFile := flag.String("out", "terraview-report.html", "Output HTML file path (for -static mode)")
	portFlag := flag.String("port", "", "HTTP port (default $PORT or 8080)")
	openFlag := flag.Bool("open", false, "Open the browser after starting the server")
	planName := flag.String("name", "", "Plan name shown in the browser title")
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

	var plan *graph.Data
	if len(planData) > 0 {
		parsed, err := graph.Parse(planData)
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
	repo := gitcontext.Detect(startDir)
	plan.SetRepoContext(repo.Path, repo.Branch)

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

	handler := server.New(server.Config{
		Plan:     plan,
		Repo:     repo,
		DemoJSON: demoPlanJSON,
		Index:    loadIndexHTML,
	})

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
	if err := http.Serve(listener, handler); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}

func runStatic(plan *graph.Data, outputFile string) {
	template, err := loadIndexHTML()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading static template: %v\n", err)
		os.Exit(1)
	}
	if err := report.Write(outputFile, plan, template); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Static report written to %s (%d resources, %d dependencies)\n", outputFile, len(plan.Nodes), len(plan.Edges))
}
