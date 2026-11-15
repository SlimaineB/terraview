# TerraView — Terraform DAG & Plan Diff Visualizer

[![CI](https://github.com/SlimaineB/terraview/actions/workflows/ci.yml/badge.svg)](https://github.com/SlimaineB/terraview/actions/workflows/ci.yml)
[![Release](https://github.com/SlimaineB/terraview/actions/workflows/release.yml/badge.svg)](https://github.com/SlimaineB/terraview/actions/workflows/release.yml)
[![Landing page](https://img.shields.io/badge/website-slimaineb.github.io%2Fterraview-22d3ee)](https://slimaineb.github.io/terraview/)
![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)

TerraView is a single-binary Go app that turns Terraform plan and state JSON (`terraform show -json`) into an interactive **Directed Acyclic Graph (DAG)**.

It shows exactly what will be **added**, **modified**, or **destroyed**, exposes upstream/downstream dependencies, and provides copy-paste **Atlantis / `terraform -target`** commands. It works with any provider — AWS, Azure, GCP, Kubernetes, Docker, Helm, and more — without an Atlantis or merge-request round trip.

The frontend and demo plan are embedded in the binary. There are no external Go dependencies, no provider plugins, and no cloud SDK.

Project site: **[slimaineb.github.io/terraview](https://slimaineb.github.io/terraview/)**.

---

## Quick start

```bash
terraform plan -out=tfplan
terraform show -json tfplan | terraview
```

TerraView prints its local URL and opens the graph. When launched inside a git work tree, the UI also shows the repository root and current branch.

---

## Features

| Feature | Description |
|---------|-------------|
| **DAG layout** | Resources arranged by dependency level (topological sort: roots → dependents) |
| **Dependency sources** | `configuration` expression references, `depends_on`, and `prior_state`; warns when dependency data is missing |
| **Plan actions** | `ADD (+)` green, `MODIFY (~)` amber, `REPLACE (±)` violet, `DESTROY (-)` red, `EXISTING` slate |
| **Diff inspector** | Resizable side panel with formatted `before` → `after` JSON, including line-level diffs for inline policies and other structured values |
| **Atlantis commands** | Per-resource `plan -target`, `apply -target`, `destroy -target`, and `import` copy buttons |
| **Node quick actions** | Copy `terraform plan` / `apply` / `destroy` commands directly from a node |
| **Search & filters** | Filter by resource ID/type/name and by action, including Replace and Destroy |
| **Zoom & pan** | Mouse-wheel zoom, drag-to-pan, and reset controls |
| **Themes & raw JSON** | Light/dark mode and a bottom panel with the full plan JSON |
| **Git context** | Repository root and current branch shown in the UI when the plan comes from a git work tree |
| **Multi-plan manifest** | Load project/env/workspace plans from one JSON manifest and switch between them in a single UI |
| **Static report** | Export a single HTML file — no server required (ideal for CI artifacts) |
| **Pipe & view** | `terraform show -json tfplan \| terraview` loads the plan and opens the browser |

---

## Install

### Prebuilt binaries

Download from [GitHub Releases](https://github.com/SlimaineB/terraview/releases). Builds are published for:

| OS | Architectures |
|----|---------------|
| Linux | `amd64`, `arm64` |
| macOS | `amd64`, `arm64` |
| Windows | `amd64` |

Each platform gets a `.tar.gz`/`.zip` archive and a raw binary. A `checksums.txt` is included for verification.

```bash
# Example: latest macOS Apple Silicon build via the GitHub CLI
VERSION="$(gh release view --repo SlimaineB/terraview --json tagName -q .tagName)"
mkdir -p terraview-bin
gh release download "$VERSION" --repo SlimaineB/terraview --dir terraview-bin \
  --pattern "terraview-*-darwin-arm64.tar.gz"
tar -xzf terraview-bin/terraview-*-darwin-arm64.tar.gz -C terraview-bin
./terraview-bin/terraview -version
```

On Windows, download the `.zip` and extract it with `Expand-Archive`.

Every CI run also uploads a `terraview-linux-amd64` binary as a GitHub Actions artifact.

### From source

```bash
git clone https://github.com/SlimaineB/terraview.git
cd terraview
go build -o terraview .
```

Requires Go 1.25 or newer.

---

## Usage

### Pipe a plan (recommended)

```bash
terraform plan -out=tfplan
terraform show -json tfplan | terraview
```

TerraView listens on a free local port, prints the URL, loads the piped plan, and opens the browser automatically. Use `-open=false` to disable that, `-port 8080` for a fixed port, and `-name` to label the instance.

### Load a file or a state

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

### Run with the demo

```bash
terraview          # prebuilt binary
go run .           # from source
```

Open the URL printed by TerraView and click **Multi-Cloud Demo** to load the built-in example (15 resources, create/update/no-op/delete actions).

### Multiple instances

Give each project or branch its own port and label:

```bash
terraform show -json tfplan | terraview -port 8081 -name "project-a/feature-x"
terraform show -json tfplan | terraview -port 8082 -name "project-b/feature-y"
```

### Multi-plan manifest (projects, envs, workspaces)

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
- The UI groups plans by `project` and exposes the selection as `?plan=<id>`.
- `-manifest` is UI-only; use `-in` or `-static` for a single report.

Generate one plan per stack/env/workspace:

```bash
terraform -chdir=stacks/storage workspace select -or-create prod
terraform -chdir=stacks/storage plan -out=tfplan
terraform -chdir=stacks/storage show -json tfplan > plans/storage-prod.json
```

### Git context

When TerraView runs inside a git work tree, the UI displays the repository root and current branch:

- With `-in`, the repository is detected from the plan file’s directory.
- With stdin, the repository is detected from the current working directory.
- A plan outside the work tree falls back to TerraView’s working directory.
- A detached `HEAD` is shown as `detached @ <short-sha>`.
- `-name` changes the plan label only; it does not override the git path or branch.

### Example: VPC → subnet → EKS

```hcl
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "private" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.2.0/24"
}

resource "aws_eks_cluster" "app" {
  vpc_config {
    subnet_ids = [aws_subnet.private.id]
  }
}
```

```bash
terraform init
terraform plan -out=tfplan
terraform show -json tfplan | terraview
```

TerraView places the VPC before the subnet and the subnet before the EKS cluster. Selecting the subnet shows its CIDR diff and a ready-to-copy target command:

```bash
atlantis plan -- -target=aws_subnet.private
```

---

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

---

## Static HTML report

Generate a report that needs no running server:

```bash
terraview -static -in plan.json -out terraview-report.html
terraform show -json tfplan | terraview -static -out terraview-report.html
```

The plan data and application HTML are embedded in the output file. Tailwind, Lucide, and Google Fonts are loaded from CDNs, so an internet connection is required for styling and icons. This is a good fit for CI artifacts and review attachments.

### GitHub Actions

```yaml
name: Terraform report

on: [pull_request]

permissions:
  contents: read

jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: hashicorp/setup-terraform@v3

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Build report
        run: |
          terraform init
          terraform plan -out=tfplan
          terraform show -json tfplan > plan.json
          go run . -static -in plan.json -out terraview-report.html

      - uses: actions/upload-artifact@v4
        with:
          name: terraview-report
          path: terraview-report.html
```

Configure the backend, credentials, and variables required by your Terraform configuration.

### GitLab CI

```yaml
stages:
  - plan
  - report

terraform_plan:
  stage: plan
  image:
    name: hashicorp/terraform:latest  # pin a version in production
    entrypoint: [""]
  script:
    - terraform init
    - terraform plan -out=tfplan
    - terraform show -json tfplan > plan.json
  artifacts:
    paths:
      - plan.json

terraview_report:
  stage: report
  image: golang:1.25
  needs:
    - terraform_plan
  script:
    - go build -o terraview .
    - ./terraview -static -in plan.json -out terraview-report.html
  artifacts:
    paths:
      - terraview-report.html
    expire_in: 1 week
```

---

## UI guide

### Top bar

- **Stats** — Existing, Add (+), Modify (~), Replace (±), Destroy (-)
- **Search** — filter by resource ID, type, or name
- **Action filters** — All / Add (+) / Modify (~) / Replace (±) / Destroy (-) / Existing
- **Theme toggle** — light or dark
- **Import Plan JSON** — paste `terraform show -json` output
- **Multi-Cloud Demo** — load the embedded example
- **Raw JSON** — open the full plan JSON in a bottom panel

### Repository context

A slim bar below the header shows the git repository root, current branch, and plan source (file name, `stdin`, uploaded plan, or demo) whenever that context is available. Long paths are truncated visually and shown in full on hover.

### Canvas

- Resources are grouped by deployment level, derived from the dependency graph.
- Each node exposes copy buttons for `terraform plan -target=...` and `terraform apply -target=...` (or `terraform destroy -target=...` for deletions).
- Drag the background to pan, scroll to zoom, and use the bottom-left controls to reset.

### Resource inspector

The right panel is resizable (drag its edge, double-click to reset) and shows:

- Provider, plan action, and attributes to be added, modified, or destroyed
- Pretty-printed JSON diffs, including line-level changes for inline IAM policies and nested values
- Upstream and downstream dependencies
- Targeted commands and optional Atlantis commands for the selected resource

Atlantis examples:

```bash
atlantis plan -- -target=aws_vpc.main
atlantis plan -- -destroy -target=aws_s3_bucket.legacy_bucket
atlantis apply
```

Add `-d <dir>` or `-p <project>` to `atlantis apply` when the repository has multiple Atlantis projects.

---

## HTTP API

The server exposes a small JSON API:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/plans` | Plans declared in `-manifest` (empty without a manifest) |
| `GET` | `/api/graph` | Loaded plan, manifest plan via `?plan=<id>`, or the demo when no plan is loaded |
| `POST` | `/api/graph` | Parse a plan/state JSON body and return the graph (10 MB limit) |
| `GET` | `/api/demo` | Built-in demo plan |
| `GET` | `/` | Web UI |

Graph responses include `source` plus, when available, `git_path` and `git_branch`. `POST /api/graph` parses the request body without changing the plan loaded at startup.

Pass `-port 8080` if you need a predictable URL for these examples.

```bash
curl -s http://localhost:8080/api/graph | jq '{source, git_path, git_branch, stats}'
curl -s -X POST --data-binary @plan.json http://localhost:8080/api/graph > graph.json
```

---

## Docker

```bash
docker build -t terraview .
docker run --rm -p 8080:8080 terraview
```

The image is built with `CGO_ENABLED=0`, runs on a distroless base, and embeds the UI and demo. To view a local plan without waiting for an MR:

```bash
terraform show -json tfplan | docker run --rm -i -p 8080:8080 terraview -open=false
```

Published tag images are available at `ghcr.io/<owner>/terraview:<version>`.

---

## Development

### Make targets

```bash
make build     # go build -o terraview .
make test      # go test ./...
make vet       # go vet ./...
make fmt       # gofmt -w .
make lint      # fmt + vet
make run       # go run . (server on a free port)
make plan      # terraform init/plan, write plan.json, open the UI
make ui        # reopen plan.json in the UI
make manifest  # open all plans from plans.json in the multi-plan UI
make static    # build + export terraview-report.html from plan.json
make image     # docker build
make clean     # remove binary and report
```

`make image` accepts `REGISTRY` and `IMAGE_TAG` overrides; see the Makefile for defaults.

`make plan` supports `TF_DIR`, `TF_WORKSPACE`, `TF_VAR_FILE`, `TF_PLAN_ARGS`, `TF_PLAN`, and `PLAN_JSON`:

```bash
make plan TF_DIR=stacks/storage TF_WORKSPACE=prod TF_VAR_FILE=prod.tfvars \
  PLAN_JSON=plans/storage-prod.json
make manifest MANIFEST=plans.json
```

`TF_WORKSPACE` is exported to Terraform, so `terraform plan` runs against that workspace.

The parser test suite includes a real plan generated by Terraform in `internal/graph/testdata/real-plan/`:

```bash
go test ./internal/graph -run TestParseRealTerraformPlan -v
```

### Requirements

- Go 1.25+
- A modern browser (Chrome, Firefox, Edge, Safari)
- Internet access for the UI’s CDN assets (Tailwind, Lucide, Google Fonts)
- Docker (optional) for the container image

No external Go dependencies are used.

### Repository layout

```
terraview/
├── main.go                    # CLI wiring, flags, embedded assets
├── internal/
│   ├── graph/
│   │   ├── graph.go           # Graph types and repo annotations
│   │   ├── parse.go           # Terraform plan/state parser
│   │   ├── parse_test.go      # Parser and real-plan fixture tests
│   │   └── testdata/          # Generated by `terraform show -json`
│   ├── gitcontext/
│   │   └── context.go         # Repository root and branch detection
│   ├── manifest/
│   │   └── manifest.go        # Multi-plan manifest parsing
│   ├── server/
│   │   ├── server.go          # HTTP API and UI handler
│   │   └── plans.go           # Plan selector API and lazy plan loading
│   └── report/
│       └── report.go          # Standalone HTML report writer
├── go.mod                     # Go module (standard library only)
├── Makefile                   # build, test, plan, manifest, static, image, run, clean
├── Dockerfile                 # Multi-stage distroless image
├── README.md
├── demo/
│   └── demo-plan.json         # Embedded demo plan (served by /api/demo)
├── docs/
│   └── index.html             # GitHub Pages landing page
├── static/
│   └── index.html             # Single-file frontend (embedded; on-disk copy wins in dev)
└── .github/workflows/
    ├── ci.yml                 # Lint, test, build, upload binary artifact
    ├── pages.yml              # Deploy docs/ to GitHub Pages
    └── release.yml            # Tests, binaries, GitHub Release, GHCR image
```

During development, an on-disk `static/index.html` takes precedence over the embedded copy, so UI edits are reflected on refresh without rebuilding.

---

## CI & Release

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `ci.yml` | push / PR to `main` or `master`, manual | gofmt check, inline HTML script validation, `go vet`, `go test`, build, upload `terraview-linux-amd64` artifact |
| `release.yml` | tag `v*`, manual | test, build `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, publish GitHub Release and multi-arch GHCR image |
| `pages.yml` | push to `main` changing `docs/**` or the workflow, manual | deploy `docs/` to GitHub Pages |

On a tag, the release contains:

- `.tar.gz` archives for Linux/macOS and `.zip` for Windows
- Raw binaries for each platform
- `checksums.txt`

A manual `release.yml` run builds the binaries and uploads them as workflow artifacts, without creating a release or pushing images.

### Landing page

The project site lives in `docs/index.html` and is deployed by `pages.yml` to:

**[https://slimaineb.github.io/terraview/](https://slimaineb.github.io/terraview/)**

To enable deployment, open **Settings → Pages → Build and deployment**, set **Source** to **GitHub Actions**, then push to `main` or run the Pages workflow manually.

---

## Troubleshooting

- **The browser did not open** — visit the printed URL, or pass `-open=false` in headless/container environments.
- **Repository context is missing** — start TerraView inside a git work tree, or pass a plan file located in one. A detached `HEAD` is shown as `detached @ <short-sha>`.
- **No dependency arrows** — the plan JSON has no dependency data. Regenerate it with `terraform show -json tfplan`.
- **Port already in use** — choose another port with `-port`; use `-name` to distinguish parallel instances.
- **The report looks unstyled offline** — Tailwind, Lucide, and Google Fonts come from CDNs.
- **A `.tfstate` file is rejected** — export it with `terraform show -json` and pass that JSON instead.

---

## License

MIT
