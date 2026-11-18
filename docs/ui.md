---
title: UI guide
---

# UI guide

## Top bar

- **Search** — filter by resource ID, type, or name
- **Action filters with counts** — All / Add / Modify / Replace / Destroy / Existing; each chip shows its count and filters the graph when clicked
- **Atlantis toggle** — switch copied commands between `terraform` and `atlantis` syntax
- **Theme toggle** — light or dark
- **Import Plan JSON** — paste `terraform show -json` output
- **Raw JSON** — open the full plan JSON in a bottom panel

## Repository context

A slim bar below the header shows the git repository root, current branch, and plan source (file name, `stdin`, uploaded plan, or demo) whenever that context is available. Long paths are truncated visually and shown in full on hover.

When the working tree has uncommitted changes, the branch has unpushed commits, is missing an upstream, or is a detached HEAD, an amber warning chip appears in the same bar. This reflects the repository's current state, not a guarantee about when the plan was generated.

### Commit & MR

On a named feature branch, a **Commit & MR** button opens a dialog that commits the working tree, pushes the branch when needed, and opens a pull request (GitHub) or merge request (GitLab) on the origin remote.

- The commit message is prefilled; the title defaults to its first line.
- Edit the target branch (default `main`) and tick **Draft** if needed.
- The action refuses `main`/`master`, detached HEADs, and empty messages, and never force-pushes.
- Tokens come from `GH_TOKEN`/`GITHUB_TOKEN` or `GITLAB_TOKEN`/`GLAB_TOKEN`, or `git credential fill`. They never leave the server.
- If the API call fails after the push, the dialog shows a manual compare/new-MR URL.

## Canvas

- Resources are grouped by deployment level, derived from the dependency graph.
- Each node exposes copy buttons for the targeted plan and apply command (or destroy for deletions), in `terraform` or `atlantis` syntax depending on the header toggle.
- Tick one or more node checkboxes to build a single multi-target command; the live command appears in the bottom **Bulk Command** panel.
- Drag the background to pan, scroll to zoom, and use the bottom-left controls to reset.

## Bulk Command panel

Selecting resources opens a bottom panel with a live command that targets every ticked resource in one shot:

```bash
atlantis plan -- -target='aws_vpc.main' -target='aws_subnet.public' -target='aws_db_instance.postgres'
```

- Switch between `plan`, `apply`, and `destroy`, and between `terraform`/`atlantis` with the header toggle.
- **Select all** picks all currently visible (filtered) resources; **Clear** empties the selection.
- **Copy** copies the live command. The panel hides when nothing is selected and yields to the Raw JSON panel when that opens.


## Resource inspector

The right panel is resizable (drag its edge, double-click to reset) and shows:

- Provider, plan action, and attributes to be added, modified, or destroyed
- Pretty-printed JSON diffs, including line-level changes for inline IAM policies and nested values
- Upstream and downstream dependencies
- Targeted commands for the selected resource, in `terraform` or `atlantis` syntax depending on the header toggle

Atlantis examples:

```bash
atlantis plan -- -target='aws_vpc.main'
atlantis plan -- -destroy -target='aws_s3_bucket.legacy_bucket'
atlantis apply
```

Add `-d <dir>` or `-p <project>` to `atlantis apply` when the repository has multiple Atlantis projects.

## Keyboard shortcuts

| Key | Action |
|-----|--------|
| `?` | Show/hide shortcuts help |
| `/` | Focus search |
| `Esc` | Close panels / clear selection |
| `←` / `→` | Pan left/right |
| `↑` / `↓` | Pan up/down |
| `=` / `-` | Zoom in/out |
| `0` | Reset view |