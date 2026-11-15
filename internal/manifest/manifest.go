package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Entry struct {
	ID        string `json:"id,omitempty"`
	Project   string `json:"project,omitempty"`
	Env       string `json:"env,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Label     string `json:"label,omitempty"`
	File      string `json:"file"`
}

type File struct {
	Plans []Entry `json:"plans"`
}

type Plan struct {
	Entry
	Path   string `json:"-"`
	Source string `json:"-"`
}

func Load(path string) ([]Plan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var file File
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if len(file.Plans) == 0 {
		return nil, fmt.Errorf("manifest %s has no plans", path)
	}

	baseDir := filepath.Dir(path)
	plans := make([]Plan, 0, len(file.Plans))
	seen := make(map[string]bool, len(file.Plans))
	for i, entry := range file.Plans {
		if strings.TrimSpace(entry.File) == "" {
			return nil, fmt.Errorf("manifest plan %d: file is required", i+1)
		}

		source := sourceName(entry.File)
		planPath := entry.File
		if !filepath.IsAbs(planPath) {
			planPath = filepath.Join(baseDir, planPath)
		}
		planPath = filepath.Clean(planPath)

		id := strings.TrimSpace(entry.ID)
		if id == "" {
			id = defaultID(entry, planPath)
		}
		if seen[id] {
			return nil, fmt.Errorf("manifest plan %d: duplicate id %q", i+1, id)
		}
		seen[id] = true

		entry.ID = id
		entry.File = planPath
		plans = append(plans, Plan{
			Entry:  entry,
			Path:   planPath,
			Source: source,
		})
	}
	return plans, nil
}

func (p Plan) DisplayLabel() string {
	if p.Label != "" {
		return p.Label
	}
	parts := make([]string, 0, 3)
	for _, part := range []string{p.Project, p.Env, p.Workspace} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return p.Source
	}
	return strings.Join(parts, " / ")
}

func defaultID(entry Entry, planPath string) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{entry.Project, entry.Env, entry.Workspace} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return strings.TrimSuffix(filepath.Base(planPath), filepath.Ext(planPath))
	}
	return strings.Join(parts, "/")
}

func sourceName(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Base(path)
	}
	return filepath.ToSlash(path)
}
