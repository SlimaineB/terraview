package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"terraview/internal/gitcontext"
	"terraview/internal/graph"
	"terraview/internal/manifest"
)

type IndexLoader func() ([]byte, error)

type Config struct {
	Plan     *graph.Data
	Plans    []manifest.Plan
	Repo     gitcontext.Context
	DemoJSON []byte
	Index    IndexLoader
}

func New(cfg Config) http.Handler {
	store := newPlanStore(cfg.Plans, cfg.Repo)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/plans", handlePlans(store))
	mux.HandleFunc("/api/graph", handleGraph(cfg, store))
	mux.HandleFunc("/api/demo", handleDemoGraph(cfg))
	mux.HandleFunc("/", handleIndex(cfg))
	return mux
}

func handleGraph(cfg Config, store *planStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if store.hasPlans() {
				data, err := store.get(r.URL.Query().Get("plan"))
				if err != nil {
					if errors.Is(err, errUnknownPlan) {
						http.Error(w, err.Error(), http.StatusNotFound)
						return
					}
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeGraphJSON(w, data)
				return
			}
			if cfg.Plan != nil {
				cfg.Plan.SetRepoContext(cfg.Repo)
				writeGraphJSON(w, cfg.Plan)
				return
			}
			data, err := demoGraph(cfg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeGraphJSON(w, data)
		case http.MethodPost:
			body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			data, err := graph.Parse(body)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error parsing Terraform JSON: %v", err), http.StatusBadRequest)
				return
			}
			data.Source = "upload"
			data.SetRepoContext(cfg.Repo)
			writeGraphJSON(w, data)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleDemoGraph(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := demoGraph(cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeGraphJSON(w, data)
	}
}

func demoGraph(cfg Config) (*graph.Data, error) {
	data, err := graph.Parse(cfg.DemoJSON)
	if err != nil {
		return nil, err
	}
	data.Source = "demo"
	data.SetRepoContext(cfg.Repo)
	return data, nil
}

func handleIndex(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		if cfg.Index == nil {
			http.Error(w, "UI not configured", http.StatusInternalServerError)
			return
		}
		data, err := cfg.Index()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}
}

func writeGraphJSON(w http.ResponseWriter, data *graph.Data) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
