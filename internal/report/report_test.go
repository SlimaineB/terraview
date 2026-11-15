package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"terraview/internal/graph"
)

func TestWriteEmbedsGraphData(t *testing.T) {
	template := []byte(`<html><body>` + embeddedDataMarker + `</body></html>`)
	data := &graph.Data{
		Nodes:  []graph.Node{{ID: "terraform_data.network", Action: "create"}},
		Edges:  []graph.Edge{},
		Source: "plan.json",
	}
	path := filepath.Join(t.TempDir(), "report.html")

	if err := Write(path, data, template); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	html, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{`id="terraview-embedded-data"`, `"terraform_data.network"`, `"source":"plan.json"`} {
		if !strings.Contains(string(html), want) {
			t.Errorf("report does not contain %q", want)
		}
	}
}
