package graph

import "terraview/internal/gitcontext"

type Node struct {
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

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type Data struct {
	Nodes             []Node `json:"nodes"`
	Edges             []Edge `json:"edges"`
	Stats             Stats  `json:"stats"`
	RawSummary        string `json:"raw_summary"`
	HasDependencyData bool   `json:"has_dependency_data"`
	Source            string `json:"source,omitempty"`
	PlanID            string `json:"plan_id,omitempty"`
	PlanLabel         string `json:"plan_label,omitempty"`
	Project           string `json:"project,omitempty"`
	Env               string `json:"env,omitempty"`
	Workspace         string `json:"workspace,omitempty"`
	GitPath           string `json:"git_path,omitempty"`
	GitBranch         string `json:"git_branch,omitempty"`
	GitDirty          bool   `json:"git_dirty,omitempty"`
	GitUnpushed       bool   `json:"git_unpushed,omitempty"`
	GitAhead          int    `json:"git_ahead,omitempty"`
}

func (d *Data) SetRepoContext(ctx gitcontext.Context) {
	if d == nil {
		return
	}
	d.GitPath = ctx.Path
	d.GitBranch = ctx.Branch
	d.GitDirty = ctx.Dirty
	d.GitUnpushed = ctx.Unpushed
	d.GitAhead = ctx.Ahead
}

type Stats struct {
	TotalResources int            `json:"total_resources"`
	Actions        map[string]int `json:"actions"`
	Providers      map[string]int `json:"providers"`
}
