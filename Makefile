# Makefile for terraview: build, test, container image, and static HTML report.
#
# Requires: go, docker (or podman)
# Override: make REGISTRY=ghcr.io/youruser IMAGE_TAG=v1.0.0 build

GO          ?= go
DOCKER      ?= docker
REGISTRY    ?= ghcr.io/slimaineb
IMAGE       ?= $(REGISTRY)/terraview
IMAGE_TAG   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)

.PHONY: all build test vet fmt lint static image run clean help

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

# Start the interactive HTTP server on :8080
run:
	$(GO) run .

# Generate a standalone HTML report from a Terraform plan JSON (CI-friendly)
static:
	$(GO) build -o terraview .
	./terraview -static -in plan.json -out terraview-report.html

# Build OCI container image
image:
	$(DOCKER) build -t $(IMAGE):$(IMAGE_TAG) .

clean:
	rm -f terraview terraview-report.html

help:
	@echo "terraview Makefile targets:"
	@echo "  make build     - go build -o terraview ."
	@echo "  make test      - go test ./..."
	@echo "  make vet       - go vet ./..."
	@echo "  make fmt       - gofmt -w ."
	@echo "  make lint      - fmt + vet"
	@echo "  make run       - go run . (HTTP server on :8080)"
	@echo "  make static    - build binary + export terraview-report.html from plan.json"
	@echo "  make image     - docker build -t $(IMAGE):$(IMAGE_TAG)"
	@echo "  make clean     - remove binary and report"
	@echo ""
	@echo "Variables: REGISTRY (default: ghcr.io/slimaineb), IMAGE_TAG (default: git short SHA)"
