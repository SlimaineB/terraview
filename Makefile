# Makefile for terraview: build, test, container image, static HTML report, and
# an opt-in commit + PR/MR helper.
#
# Requires: go, terraform (for plan), docker (or podman), python3 (for `make pr`)
# Override: make REGISTRY=ghcr.io/youruser IMAGE_TAG=v1.0.0 build

GO          ?= go
TERRAFORM   ?= terraform
DOCKER      ?= docker
REGISTRY    ?= ghcr.io/slimaineb
IMAGE       ?= $(REGISTRY)/terraview
IMAGE_TAG   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)
TF_DIR      ?= internal/graph/testdata/real-plan
TF_WORKSPACE ?=
TF_VAR_FILE ?=
TF_PLAN_ARGS ?=
TF_PLAN     ?= tfplan
PLAN_JSON   ?= plan.json
MANIFEST    ?= plans.json
BASE_BRANCH ?=
GIT_FETCH   ?= 0
GIT_STRICT  ?= 0
BASE_CHECK_ARGS = $(if $(BASE_BRANCH),--base "$(BASE_BRANCH)",) $(if $(filter 1,$(GIT_FETCH)),--fetch,) $(if $(filter 1,$(GIT_STRICT)),--strict,)

export TF_WORKSPACE

.PHONY: all build test vet fmt lint static image run plan ui manifest pr clean help docs docs-build docs-serve

all: build test vet

build:
	$(GO) build -o terraview .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

lint: fmt vet

# Start the interactive HTTP server on a free port
run:
	$(GO) run .

# Run terraform plan in TF_DIR and open the resulting plan in the UI
plan:
	@test -n "$(wildcard $(TF_DIR)/*.tf)" || { \
		echo "No Terraform configuration found in $(TF_DIR)."; \
		echo "Run: make plan TF_DIR=path/to/terraform"; \
		exit 1; \
	}
	@bash ./scripts/check-base.sh $(BASE_CHECK_ARGS)
	$(TERRAFORM) -chdir="$(TF_DIR)" init -input=false
	$(TERRAFORM) -chdir="$(TF_DIR)" plan $(if $(TF_VAR_FILE),-var-file="$(TF_VAR_FILE)") $(TF_PLAN_ARGS) -out="$(TF_PLAN)"
	$(TERRAFORM) -chdir="$(TF_DIR)" show -json "$(TF_PLAN)" > "$(PLAN_JSON)"
	$(MAKE) ui

# Open the plan JSON in the UI
ui:
	$(GO) run . -in "$(PLAN_JSON)" -open

# Open all plans listed in MANIFEST in the multi-plan UI
manifest:
	$(GO) run . -manifest "$(MANIFEST)" -open

# Commit current changes on a branch and open a PR/MR (disabled by default).
# Usage: TERRAVIEW_ALLOW_PR=1 make pr MSG="Add feature" [TITLE="Add feature"]
pr:
	@TERRAVIEW_ALLOW_PR="$${TERRAVIEW_ALLOW_PR:-0}" ./scripts/open-pr.sh \
		$(if $(MSG),-m "$(MSG)",) \
		$(if $(TITLE),-t "$(TITLE)",) \
		$(if $(BODY),-b "$(BODY)",) \
		$(if $(BASE),--base "$(BASE)",) \
		$(if $(BRANCH),--branch "$(BRANCH)",) \
		$(PR_ARGS)

# Generate a standalone HTML report from a Terraform plan JSON (CI-friendly)
static:
	$(GO) build -o terraview .
	./terraview -static -in plan.json -out terraview-report.html

# Build OCI container image
image:
	$(DOCKER) build -t $(IMAGE):$(IMAGE_TAG) .

clean:
	rm -f terraview terraview-report.html "$(PLAN_JSON)" "$(TF_DIR)/$(TF_PLAN)"

help:
	@echo "terraview Makefile targets:"
	@echo "  make build     - go build -o terraview ."
	@echo "  make test      - go test ./..."
	@echo "  make vet       - go vet ./..."
	@echo "  make fmt       - gofmt -w ."
	@echo "  make lint      - fmt + vet"
	@echo "  make run       - go run . (HTTP server on a free port)"
	@echo "  make plan      - terraform plan in TF_DIR (checks the base branch first), export PLAN_JSON, open the UI"
	@echo "  make ui        - open PLAN_JSON in the UI"
	@echo "  make manifest  - open all plans from MANIFEST in the multi-plan UI"
	@echo "  make pr        - commit + open a PR/MR (checks the base branch; requires TERRAVIEW_ALLOW_PR=1)"
	@echo "  make static    - build binary + export terraview-report.html from plan.json"
	@echo "  make image     - docker build -t $(IMAGE):$(IMAGE_TAG)"
	@echo "  make clean     - remove binary, report, plan JSON, and tfplan"
	@echo ""
	@echo "Variables: REGISTRY, IMAGE_TAG, TF_DIR (default: internal/graph/testdata/real-plan), TF_WORKSPACE, TF_VAR_FILE, TF_PLAN_ARGS, TF_PLAN (default: tfplan), PLAN_JSON (default: plan.json), MANIFEST (default: plans.json), BASE_BRANCH, GIT_FETCH (1 = fetch base), GIT_STRICT (1 = fail when behind)"
	@echo "make pr: MSG (required for uncommitted changes), TITLE, BODY, BASE, BRANCH, PR_ARGS"
	@echo ""
	@echo "Docs targets (require Node 22.23.4+):"
	@echo "  make docs       - install deps + build Astro site"
	@echo "  make docs-build - build Astro site (run npm install first)"
	@echo "  make docs-serve - preview the built site on port 8099"

# Docs site targets (require Node 22.23.4+)
# Override NODE/NPM to point at a specific toolchain, e.g. a nvm install.
NODE  ?= node
NPM   ?= npm
ASTRO := $(NODE) ./node_modules/.bin/astro
SYNC  := $(NODE) scripts/sync-docs.mjs

docs:
	$(NPM) ci
	$(SYNC)
	$(ASTRO) build

docs-build:
	$(SYNC)
	$(ASTRO) build

docs-serve:
	$(SYNC)
	$(ASTRO) build
	$(ASTRO) preview --port 8099
