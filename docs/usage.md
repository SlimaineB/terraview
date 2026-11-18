---
title: Usage
---

# Usage

## Pipe a plan (recommended)

```bash
terraform plan -out=tfplan
terraform show -json tfplan | terraview
```

TerraView listens on a free local port, prints the URL, loads the piped plan, and opens the browser automatically. Use `-open=false` to disable that, `-port 8080` for a fixed port, and `-name` to label the instance.

For `-open`, TerraView honors `$BROWSER` (including `%s` placeholders and colon-separated fallbacks), then falls back to `open` on macOS, `rundll32` on Windows, and `xdg-open` on Linux/Unix.

## Load a file or a state

`-in` accepts the JSON produced by `terraform show -json`:

```bash
# Plan
terraform show -json tfplan > plan.json
terraview -in plan.json -name "project-a/feature-x"

# State
terraform show -json > state.json
terraview -in state.json -name "project-a/state"
```

`-in` expects the JSON from `terraform show -json`, not a raw `.tfstate` file.

## Multiple instances

Give each project or branch its own port and label:

```bash
terraform show -json tfplan | terraview -port 8081 -name "project-a/feature-x"
terraform show -json tfplan | terraview -port 8082 -name "project-b/feature-y"
```

## Multi-plan manifest (projects, envs, workspaces)

For repositories with several Terraform roots, environments, or workspaces, describe the plans in a manifest:

```json
{
  "plans": [
    {"project": "storage", "env": "prod", "workspace": "default", "file": "plans/storage-prod.json"},
    {"project": "compute", "env": "prod", "workspace": "default", "file": "plans/compute-prod.json"},
    {"id": "iam-prod", "label": "IAM / production", "project": "iam", "env": "prod", "workspace": "default", "file": "plans/iam-prod.json"}
  ]
}
```

```bash
terraview -manifest plans.json -open
```

- `file` is required and resolved relative to the manifest file.
- `id` defaults to `project/env/workspace`; set it explicitly to customise the URL key.
- `label` overrides the display label.
- The plan switcher lists every plan with a colored dot and its action counts (`2 add · 1 mod`, `1 del`, …), so you can see which projects the branch impacts without opening each one. Colors: red = destroy, orange = replace, amber = update, green = create, gray = no changes.
- The selection is exposed as `?plan=<id>`.
- `-manifest` is UI-only; use `-in` or `-static` for a single report.

Generate one plan per stack/env/workspace:

```bash
terraform -chdir=stacks/storage workspace select -or-create prod
terraform -chdir=stacks/storage plan -out=tfplan
terraform -chdir=stacks/storage show -json tfplan > plans/storage-prod.json
```

## Git context

When TerraView runs inside a git work tree, the UI displays the repository root and current branch:

- With `-in`, the repository is detected from the plan file's directory.
- With stdin, the repository is detected from the current working directory.
- A plan outside the work tree falls back to TerraView's working directory.
- A detached `HEAD` is shown as `detached @ <short-sha>`.
- `-name` changes the plan label only; it does not override the git path or branch.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-static` | `false` | Export a standalone HTML report and exit |
| `-in` | stdin (when piped) | Read plan/state JSON from a file |
| `-manifest` | | Load multiple plans from a JSON manifest (UI mode only) |
| `-out` | `terraview-report.html` | Output path (with `-static`) |
| `-port` | `$PORT`, then a free port | HTTP port (`-port 8080` for a fixed port) |
| `-open` | `false` (`true` for piped plans unless set) | Open the browser after starting |
| `-name` | input filename or `stdin` | Label shown in the browser title |
| `-version` | | Print the version and exit |

`-out` is only used with `-static`. When a plan is piped, `-open` defaults to `true` unless explicitly set; pass `-open=false` to suppress it.