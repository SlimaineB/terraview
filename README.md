# TerraView — Terraform DAG & Plan Diff Visualizer

TerraView is a lightweight, dependency-free Go application that visualizes Terraform execution plans (`terraform show -json`) and state files (`.tfstate`) as an interactive **Directed Acyclic Graph (DAG)**.

It highlights exactly what will be **added**, **modified**, or **destroyed**, shows upstream/downstream dependencies for every resource, and provides copy-paste ready **Atlantis / `terraform -target`** commands for your merge requests. Works with **any Terraform provider** — AWS, Azure, GCP, Kubernetes, Docker, Helm, and more.

---

## Features

| Feature | Description |
|---------|-------------|
| **DAG layout** | Resources arranged by dependency level (topological sort: roots → dependents) |
| **Zoom & pan** | Mouse wheel zoom, drag-to-pan, and reset controls |
| **Plan action badges** | `ADD (+)` green, `MODIFY (~)` amber, `DESTROY (-)` red, `EXISTING` slate |
| **Inline attribute diff** | Click any resource to see `before` → `after` changes side-by-side |
| **Provider-agnostic icons** | Lucide icons mapped by resource type prefix (VPC, subnet, database, VM, K8s, etc.) |
| **Atlantis commands** | Per-resource `plan -target`, `apply -target`, `destroy -target`, `import` copy buttons |
| **Dark / Light theme** | Light mode by default, toggle in header |
| **Raw JSON viewer** | Inspect the full plan JSON in a bottom panel |
| **Static HTML export** | Generate a single self-contained `.html` report — no server needed (ideal for CI artifacts) |

---

## Quick Start

### Run the interactive server

```bash
git clone https://github.com/SlimaineB/terraview.git
cd terraview
go run .
```

Open your browser at **http://localhost:8080**.

Click **Multi-Cloud Demo** to load a built-in enterprise plan (IAM, VPC, EKS, RDS, S3, Security Groups, with create/update/no-op/delete actions).

Or click **Import Plan JSON** and paste the output of:

```bash
terraform show -json plan.json > plan.json
```

---

## Static HTML Export (GitLab CI / GitHub CI friendly)

Generate a standalone HTML report that requires **no running server**. Perfect for publishing as a CI artifact.

### Usage

```bash
# Build the binary
make build

# Generate a standalone HTML report from a Terraform plan JSON
./terraview -static -in plan.json -out terraview-report.html
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-static` | `false` | Enable static export mode |
| `-in` | (required with `-static`) | Input Terraform plan JSON file |
| `-out` | `terraview-report.html` | Output HTML file path |

### GitLab CI Example

```yaml
stages:
  - plan
  - report

terraform_plan:
  stage: plan
  image: hashicorp/terraform:1.5
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
  script:
    - make build
    - ./terraview -static -in plan.json -out terraview-report.html
  artifacts:
    paths:
      - terraview-report.html
    expire_in: 1 week
  needs:
    - terraform_plan
```

Open `terraview-report.html` in any browser — it is fully self-contained (CSS, icons, and plan data embedded).

---

## UI Walkthrough

### Top Bar
- **Resource stats** — count of Existing, Add (+), Modify (~)
- **Theme toggle** — Light / Dark mode
- **Import Plan JSON** — paste `terraform show -json` output
- **Multi-Cloud Demo** — load the built-in example plan
- **`</>` Raw JSON** — open a bottom panel showing the complete plan JSON

### Canvas
- Resources are grouped by **Deployment Level** (topological sort by dependencies)
- Click a node to open the **Resource Diff Inspector**
- Drag the canvas background to pan; scroll to zoom; use bottom-left controls for zoom in/out/reset

### Resource Diff Inspector (right panel, resizable)
- **Provider** — the actual provider name from the plan
- **Plan Action** — colour-coded badge (ADD / MODIFY / DESTROY / EXISTING)
- **Attribute diff** — side-by-side `before` (`-`) / `after` (`+`) for modified resources; full attribute list for existing or new resources
- **Upstream / Downstream** — dependency chain
- **Atlantis / Targeted Plan** — copy-paste commands for MRs:
  - `terraform plan -target=aws_vpc.main`
  - `terraform apply -target=aws_vpc.main`
  - `terraform destroy -target=aws_s3_bucket.legacy_bucket`
  - `terraform import <addr> <RESOURCE_ID>`

### Filters (top bar)
- **Search** — filter by resource ID, type, or name
- **Action chips** — All / Add (+) / Modify (~) / Existing

---

## Makefile

```
make build     # go build -o terraview .
make test      # go test ./...
make vet       # go vet ./...
make fmt       # gofmt -w .
make lint      # fmt + vet
make run       # go run . (HTTP server on :8080)
make static    # build binary + export terraview-report.html from plan.json
make image     # docker build
make clean     # remove binary and report
```

Variables: `REGISTRY` (default: `ghcr.io/slimaineb`), `IMAGE_TAG` (default: git short SHA)

---

## Dockerfile

Multi-stage build, distroless runtime, CGO disabled:

```dockerfile
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /terraview .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /terraview /terraview
EXPOSE 8080
ENTRYPOINT ["/terraview"]
```

Build and run:

```bash
docker build -t terraview .
docker run -p 8080:8080 terraview
```

---

## CI & Release

GitHub Actions workflows live in `.github/workflows/`:

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `ci.yml` | push / PR to `main` | gofmt check, `go vet`, `go test`, `go build` |
| `release.yml` | push tag `v*` | Build binaries (linux/darwin/windows), create GitHub Release, build & push multi-arch Docker image to GHCR |

Published images: `ghcr.io/<owner>/terraview`

---

## Repository Layout

```
terraview/
├── main.go               # Go HTTP server, Terraform JSON parser, static HTML exporter
├── go.mod                # Go module (standard library only)
├── Makefile              # build, test, vet, fmt, lint, static, image, run, clean
├── Dockerfile            # Multi-stage distroless container image
├── README.md             # This file
├── static/
│   └── index.html        # Single-file frontend (Tailwind, Lucide, custom SVG DAG)
└── .github/workflows/
    ├── ci.yml            # CI pipeline
    └── release.yml       # Release + Docker publish
```

---

## Requirements

- Go 1.25+
- Modern browser (Chrome, Firefox, Edge, Safari)

No external Go dependencies — only the standard library is used.

---

## License

MIT