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
	"strconv"
	"strings"

	"terraview/internal/gitcontext"
	"terraview/internal/graph"
	"terraview/internal/manifest"
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
	var lastErr error
	for _, argv := range browserCandidates(os.Getenv("BROWSER"), url, runtime.GOOS) {
		cmd := exec.Command(argv[0], argv[1:]...)
		if err := cmd.Start(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		return fmt.Errorf("no browser command found")
	}
	return fmt.Errorf("open browser: %w", lastErr)
}

func browserCandidates(browser, url, goos string) [][]string {
	var candidates [][]string
	separator := ":"
	if goos == "windows" {
		separator = ";"
	}
	if strings.TrimSpace(browser) != "" {
		for _, entry := range strings.Split(browser, separator) {
			args, err := splitBrowserCommand(strings.TrimSpace(entry), url)
			if err != nil || len(args) == 0 {
				continue
			}
			candidates = append(candidates, args)
		}
	}
	switch goos {
	case "darwin":
		candidates = append(candidates, []string{"open", url})
	case "windows":
		candidates = append(candidates, []string{"rundll32", "url.dll,FileProtocolHandler", url})
	default:
		candidates = append(candidates, []string{"xdg-open", url})
	}
	return candidates
}

func splitBrowserCommand(command, url string) ([]string, error) {
	args, err := splitCommandLine(command)
	if err != nil {
		return nil, err
	}
	hasPlaceholder := false
	for i, arg := range args {
		if strings.Contains(arg, "%s") {
			hasPlaceholder = true
			args[i] = strings.ReplaceAll(arg, "%s", url)
		}
	}
	if !hasPlaceholder {
		args = append(args, url)
	}
	return args, nil
}

func splitCommandLine(command string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote byte
	inArg := false
	flush := func() {
		if inArg {
			args = append(args, current.String())
			current.Reset()
			inArg = false
		}
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				current.WriteByte(c)
			}
		case quote == '"':
			if c == '"' {
				quote = 0
			} else if c == '\\' && i+1 < len(command) && (command[i+1] == '"' || command[i+1] == '\\') {
				i++
				current.WriteByte(command[i])
			} else {
				current.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inArg = true
		case c == '\\':
			if i+1 < len(command) {
				i++
				current.WriteByte(command[i])
			} else {
				current.WriteByte(c)
			}
			inArg = true
		case c == ' ' || c == '\t' || c == '\n':
			flush()
		default:
			current.WriteByte(c)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in browser command")
	}
	flush()
	return args, nil
}

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	staticMode := flag.Bool("static", false, "Export a standalone HTML report instead of starting the HTTP server")
	inputFile := flag.String("in", "", "Input Terraform plan/state JSON file (or pipe JSON on stdin)")
	manifestFile := flag.String("manifest", "", "JSON manifest listing multiple plans")
	outputFile := flag.String("out", "terraview-report.html", "Output HTML file path (for -static mode)")
	portFlag := flag.String("port", "", "HTTP port (default $PORT or a free port)")
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

	var plans []manifest.Plan
	var plan *graph.Data
	source := ""
	fromStdin := false
	startDir := ""

	if *manifestFile != "" {
		if *inputFile != "" {
			fmt.Fprintln(os.Stderr, "Error: use either -in or -manifest, not both")
			os.Exit(1)
		}
		if *staticMode {
			fmt.Fprintln(os.Stderr, "Error: -manifest is only available in UI mode")
			os.Exit(1)
		}
		loaded, err := manifest.Load(*manifestFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading manifest: %v\n", err)
			os.Exit(1)
		}
		plans = loaded
		source = *manifestFile
		if abs, err := filepath.Abs(*manifestFile); err == nil {
			startDir = filepath.Dir(abs)
		}
	} else {
		var planData []byte
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

		if len(planData) > 0 {
			parsed, err := graph.Parse(planData)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing Terraform JSON: %v\n", err)
				os.Exit(1)
			}
			parsed.Source = source
			plan = parsed
		}

		if *inputFile != "" {
			if abs, err := filepath.Abs(*inputFile); err == nil {
				startDir = filepath.Dir(abs)
			}
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

	handler := server.New(server.Config{
		Plan:     plan,
		Plans:    plans,
		Repo:     repo,
		DemoJSON: demoPlanJSON,
		Index:    loadIndexHTML,
	})

	listener, actualPort, err := listenOn(port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://localhost:%s", actualPort)
	fmt.Printf("TerraView running at %s\n", url)
	if plan != nil {
		fmt.Printf("Loaded %s: %d resources, %d dependencies\n", source, len(plan.Nodes), len(plan.Edges))
	}
	if len(plans) > 0 {
		fmt.Printf("Loaded %d plans from %s\n", len(plans), source)
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

func listenOn(port string) (net.Listener, string, error) {
	addr := ":" + port
	if port == "" {
		addr = ":0"
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, "", err
	}

	if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok {
		port = strconv.Itoa(tcpAddr.Port)
	}
	return listener, port, nil
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
