package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"terraview/internal/gitcontext"
	"terraview/internal/graph"
	"terraview/internal/server"
)

func TestRealTerraformPlanThroughServer(t *testing.T) {
	raw, err := os.ReadFile("internal/graph/testdata/real-plan/plan.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	plan, err := graph.Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	handler := server.New(server.Config{
		Plan: plan,
		Repo: gitcontext.Context{Path: "/workspace/infra", Branch: "main"},
		Index: func() ([]byte, error) {
			return []byte("<!doctype html><title>TerraView</title>"), nil
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got graph.Data
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3", len(got.Nodes))
	}
	if len(got.Edges) != 2 {
		t.Errorf("edges = %d, want 2", len(got.Edges))
	}
	if got.GitPath != "/workspace/infra" || got.GitBranch != "main" {
		t.Errorf("git context = %q/%q, want /workspace/infra/main", got.GitPath, got.GitBranch)
	}
}
