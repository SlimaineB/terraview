package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "plans.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func TestLoadResolvesPlansAndLabels(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, `{
		"plans": [
			{"project": "storage", "env": "prod", "workspace": "default", "file": "plans/storage-prod.json"},
			{"project": "compute", "env": "prod", "workspace": "default", "file": "plans/compute-prod.json"},
			{"label": "IAM production", "file": "plans/iam-prod.json"}
		]
	}`)

	plans, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("plans = %d, want 3", len(plans))
	}

	first := plans[0]
	if first.ID != "storage/prod/default" {
		t.Errorf("id = %q, want storage/prod/default", first.ID)
	}
	if first.DisplayLabel() != "storage / prod / default" {
		t.Errorf("label = %q, want storage / prod / default", first.DisplayLabel())
	}
	if !strings.HasSuffix(first.Path, filepath.Join("plans", "storage-prod.json")) {
		t.Errorf("path = %q, want resolved relative to the manifest", first.Path)
	}
	if first.Source != filepath.Join("plans", "storage-prod.json") {
		t.Errorf("source = %q, want plans/storage-prod.json", first.Source)
	}

	third := plans[2]
	if third.ID != "iam-prod" {
		t.Errorf("id = %q, want iam-prod", third.ID)
	}
	if third.DisplayLabel() != "IAM production" {
		t.Errorf("label = %q, want IAM production", third.DisplayLabel())
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, `{"plans":[{"project":"storage","file":""}]}`)

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "file is required") {
		t.Fatalf("Load() error = %v, want file is required", err)
	}
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, `{
		"plans": [
			{"id": "storage", "file": "a.json"},
			{"id": "storage", "file": "b.json"}
		]
	}`)

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("Load() error = %v, want duplicate id", err)
	}
}
