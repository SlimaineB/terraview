package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"

	"terraview/internal/gitcontext"
	"terraview/internal/graph"
	"terraview/internal/manifest"
)

var errUnknownPlan = errors.New("unknown plan")

type planInfo struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Project   string `json:"project,omitempty"`
	Env       string `json:"env,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Source    string `json:"source,omitempty"`
}

type plansResponse struct {
	Plans []planInfo `json:"plans"`
}

type planStore struct {
	plans []manifest.Plan
	byID  map[string]manifest.Plan
	repo  gitcontext.Context

	mu    sync.Mutex
	cache map[string]*planCacheEntry
}

type planCacheEntry struct {
	once sync.Once
	data *graph.Data
	err  error
}

func newPlanStore(plans []manifest.Plan, repo gitcontext.Context) *planStore {
	store := &planStore{
		plans: plans,
		byID:  make(map[string]manifest.Plan, len(plans)),
		repo:  repo,
		cache: make(map[string]*planCacheEntry, len(plans)),
	}
	for _, plan := range plans {
		store.byID[plan.ID] = plan
	}
	return store
}

func (s *planStore) hasPlans() bool {
	return s != nil && len(s.plans) > 0
}

func (s *planStore) list() []planInfo {
	infos := make([]planInfo, 0, len(s.plans))
	for _, plan := range s.plans {
		infos = append(infos, planInfo{
			ID:        plan.ID,
			Label:     plan.DisplayLabel(),
			Project:   plan.Project,
			Env:       plan.Env,
			Workspace: plan.Workspace,
			Source:    plan.Source,
		})
	}
	return infos
}

func (s *planStore) get(id string) (*graph.Data, error) {
	if len(s.plans) == 0 {
		return nil, fmt.Errorf("%w: no plans configured", errUnknownPlan)
	}
	if id == "" {
		id = s.plans[0].ID
	}

	plan, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownPlan, id)
	}

	s.mu.Lock()
	entry := s.cache[id]
	if entry == nil {
		entry = &planCacheEntry{}
		s.cache[id] = entry
	}
	s.mu.Unlock()

	entry.once.Do(func() {
		raw, err := os.ReadFile(plan.Path)
		if err != nil {
			entry.err = fmt.Errorf("read plan %s: %w", plan.ID, err)
			return
		}
		data, err := graph.Parse(raw)
		if err != nil {
			entry.err = fmt.Errorf("parse plan %s: %w", plan.ID, err)
			return
		}
		data.Source = plan.Source
		data.PlanID = plan.ID
		data.PlanLabel = plan.DisplayLabel()
		data.Project = plan.Project
		data.Env = plan.Env
		data.Workspace = plan.Workspace
		data.SetRepoContext(s.repo)
		entry.data = data
	})
	return entry.data, entry.err
}

func handlePlans(store *planStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(plansResponse{Plans: store.list()}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
