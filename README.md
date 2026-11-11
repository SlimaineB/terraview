# TerraView — Terraform DAG & Plan Diff Visualizer

TerraView is a lightweight Go application that visualizes Terraform execution plans (`terraform show -json`) and state files (`.tfstate`) as an interactive Directed Acyclic Graph (DAG). It highlights exactly what will be **added**, **modified**, or **destroyed**, shows upstream/downstream dependencies, and provides copy-paste ready Atlantis / `terraform -target` commands for your merge requests.

---

## Features

- **DAG layout** — resources arranged by dependency level (roots → dependents)
- **Smooth zoom & pan** — mouse wheel zoom, drag-to-pan, reset button
- **Plan action badges** — `ADD (+)` green, `MODIFY (~)` amber, `DESTROY (-)` red, `EXISTING` slate
- **Inline attribute diff** — click any resource to see `before` → `after` changes
- **AWS-focused expressive icons** — VPC, Subnet, IGW, Security Group, IAM, RDS, EKS, S3, etc.
- **Atlantis / targeted commands** — per-resource `plan -target`, `apply -target`, `destroy -target`, `import` copy buttons
- **Dark / Light theme** — light mode by default, toggle in header
- **Static HTML export** — generate a single self-contained `.html` report for GitLab CI artifacts (no server needed)

---

## Quick Start

### Run the HTTP server

```bash
go run main.go
```

Open your browser at **http://localhost:8080**.

Click **AWS Demo** to load a built-in multi-module enterprise plan (IAM, VPC, EKS, RDS, S3, Security Groups, etc.).

Or click **Import Plan JSON** and paste the output of:

```bash
terraform show -json plan.json > plan.json
```

---

## Static HTML Export (GitLab CI friendly)

Generate a standalone HTML report that requires **no running server**. Perfect for GitLab CI artifacts.

### Usage

```bash
# Build the binary
go build -o terraview main.go

# Generate a static report from a Terraform plan JSON
./terraview -static -in plan.json -out terraview-report.html
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-static` | `false` | Enable static export mode |
| `-in` | (required with `-static`) | Input Terraform plan JSON file |
| `-out` | `terraview-report.html` | Output HTML file path |

### GitLab CI Example

Add this to your `.gitlab-ci.yml`:

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
    - go build -o terraview main.go
    - ./terraview -static -in plan.json -out terraview-report.html
  artifacts:
    paths:
      - terraview-report.html
    expire_in: 1 week
  needs:
    - terraform_plan
```

Publish the `terraview-report.html` artifact and open it in any browser — it is fully self-contained (CSS, icons, and plan data embedded).

---

## UI Walkthrough

### Top Bar
- **Resource stats** — count of Existing, Add (+), Modify (~)
- **Theme toggle** — Light / Dark mode
- **Import JSON Plan** — paste `terraform show -json` output
- **AWS Demo** — load the built-in enterprise multi-module example
- **`</>` Raw JSON** — open a bottom panel showing the complete plan JSON

### Canvas
- Resources are grouped by **Deployment Level** (topological sort by dependencies)
- Click a node to open the **Resource Diff Inspector**
- Drag the canvas background to pan; scroll to zoom; use bottom-left controls for zoom in/out/reset

### Resource Diff Inspector (right panel)
- **Provider** — always AWS in the demo
- **Plan Action** — colour-coded badge (ADD / MODIFY / DESTROY / EXISTING)
- **Attribute diff** — side-by-side `before` (`-`) / `after` (`+`) for modified resources; full attribute list for existing or new resources
- **Upstream / Downstream** — dependency chain
- **Atlantis / Targeted Plan** — copy-paste commands for MRs:
  - `terraform plan -target=aws_vpc.main`
  - `terraform apply -target=aws_vpc.main`
  - `terraform destroy -target=aws_s3_bucket.legacy_archive_bucket`
  - `terraform import <addr> <RESOURCE_ID>`

### Filters (top bar)
- Search box — filter by resource ID, type, or name
- Action chips — All / Add (+) / Modify (~) / Existing

---

## Project Structure

```
terraview/
├── main.go               # Go HTTP server + JSON parser + static export
├── go.mod                # Go module
├── static/
│   └── index.html        # Single-file frontend (Tailwind, Lucide, Vis-free SVG DAG)
└── README.md             # This file
```

---

## Requirements

- Go 1.25+
- Modern browser (Chrome, Firefox, Edge, Safari)

No external Go dependencies — only the standard library is used.

---

## License

Internal tool — free to use and modify.