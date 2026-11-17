---
title: Overview
---

# TerraView

TerraView is a single-binary Go app that turns Terraform plan and state JSON (`terraform show -json`) into an interactive **Directed Acyclic Graph (DAG)**.

It shows exactly what will be **added**, **modified**, or **destroyed**, exposes upstream/downstream dependencies, and provides copy-paste **Atlantis / `terraform -target`** commands. It works with any provider — AWS, Azure, GCP, Kubernetes, Docker, Helm, and more — without an Atlantis or merge-request round trip.

The frontend and demo plan are embedded in the binary. There are no external Go dependencies, no provider plugins, and no cloud SDK.

Project site: **[slimaineb.github.io/terraview](https://slimaineb.github.io/terraview/)**.

## Quick start

```bash
terraform plan -out=tfplan
terraform show -json tfplan | terraview
```

TerraView prints its local URL and opens the graph. When launched inside a git work tree, the UI also shows the repository root and current branch.

## Features

| Feature | Description |
|---------|-------------|
| **DAG layout** | Resources arranged by dependency level (topological sort: roots → dependents) |
| **Dependency sources** | `configuration` expression references, `depends_on`, and `prior_state`; warns when dependency data is missing |
| **Plan actions** | `ADD (+)` green, `MODIFY (~)` amber, `REPLACE (±)` violet, `DESTROY (-)` red, `EXISTING` slate |
| **Diff inspector** | Resizable side panel with formatted `before` → `after` JSON, including line-level diffs for inline policies and other structured values |
| **Atlantis commands** | Per-resource `plan -target`, `apply -target`, `destroy -target`, and `import` copy buttons |
| **Node quick actions** | Copy `terraform plan` / `apply` / `destroy` commands directly from a node |
| **Bulk selection** | Tick resources and get one multi-target command, shown live in a bottom panel |
| **Search & filters** | Filter by resource ID/type/name and by action, including Replace and Destroy; counts merged into the filter chips |
| **Zoom & pan** | Mouse-wheel zoom, drag-to-pan, and reset controls |
| **Themes & raw JSON** | Light/dark mode and a bottom panel with the full plan JSON |
| **Git context** | Repository root and branch shown in the UI, with a warning for uncommitted changes or unpushed commits |
| **Multi-plan manifest** | Load project/env/workspace plans from one JSON manifest and switch between them in a single UI |
| **Static report** | Export a single HTML file — no server required (ideal for CI artifacts) |
| **Pipe & view** | `terraform show -json tfplan \| terraview` loads the plan and opens the browser |

## Next steps

- [Install TerraView](/install/)
- [Usage guide](/usage/)
- [UI walkthrough](/ui/)
- [CI integration](/ci/)