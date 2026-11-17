---
title: CI reports
---

# CI integration

## Static HTML report

Generate a report that needs no running server:

```bash
terraview -static -in plan.json -out terraview-report.html
terraform show -json tfplan | terraview -static -out terraview-report.html
```

The plan data and application HTML are embedded in the output file. Tailwind, Lucide, and Google Fonts are loaded from CDNs, so an internet connection is required for styling and icons. This is a good fit for CI artifacts and review attachments.

## GitHub Actions

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

## GitLab CI

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

## Prebuilt binary in CI

Download the release binary directly (faster than `go build`):

```bash
# Linux amd64
VERSION=v0.2.0
curl -sSL "https://github.com/SlimaineB/terraview/releases/download/${VERSION}/terraview-0.2.0-linux-amd64.tar.gz" | tar -xz
./terraview -static -in plan.json -out terraview-report.html
```

## Docker in CI

```yaml
# GitHub Actions
- name: Generate report
  run: |
    docker run --rm -v ${{ github.workspace }}:/workspace \
      ghcr.io/slimaineB/terraview:latest \
      -static -in /workspace/plan.json -out /workspace/terraview-report.html

# GitLab CI
terraview_report:
  stage: report
  image: docker:latest
  services:
    - docker:dind
  needs:
    - terraform_plan
  script:
    - docker pull ghcr.io/slimaineB/terraview:latest
    - docker run --rm -v $(pwd):/workspace \
      ghcr.io/slimaineB/terraview:latest \
      -static -in /workspace/plan.json -out /workspace/terraview-report.html
  artifacts:
    paths:
      - terraview-report.html
    expire_in: 1 week
```

## Multi-plan manifest in CI

For repos with multiple stacks, generate a manifest and serve all plans:

```bash
# Generate plans for each stack/env
for stack in storage compute iam; do
  terraform -chdir=stacks/$stack workspace select -or-create prod
  terraform -chdir=stacks/$stack plan -out=tfplan
  terraform -chdir=stacks/$stack show -json tfplan > plans/$stack-prod.json
done

# Create manifest
cat > plans.json <<'EOF'
{
  "plans": [
    {"project": "storage", "env": "prod", "workspace": "default", "file": "plans/storage-prod.json"},
    {"project": "compute", "env": "prod", "workspace": "default", "file": "plans/compute-prod.json"},
    {"project": "iam", "env": "prod", "workspace": "default", "file": "plans/iam-prod.json"}
  ]
}
EOF

# Serve (headless for CI artifact)
terraview -manifest plans.json -port 8080 -open=false &
sleep 2
curl http://localhost:8080 > terraview-manifest.html
```

The single-file manifest HTML contains all plans and can be uploaded as one artifact.