---
title: Install
---

# Install TerraView

## Prebuilt binaries

Download from [GitHub Releases](https://github.com/SlimaineB/terraview/releases). Builds are published for:

| OS | Architectures |
|----|---------------|
| Linux | `amd64`, `arm64` |
| macOS | `amd64`, `arm64` |
| Windows | `amd64` |

Each platform gets a `.tar.gz`/`.zip` archive and a raw binary. A `checksums.txt` is included for verification.

```bash
# Linux amd64 (no GitHub CLI needed)
VERSION=v0.3.1
mkdir -p bin
curl -sSL -o bin/terraview.tar.gz \
  "https://github.com/SlimaineB/terraview/releases/download/${VERSION}/terraview-0.3.1-linux-amd64.tar.gz"
tar -xzf bin/terraview.tar.gz -C bin
./bin/terraview -version
```

Other assets follow the same pattern: `terraview-0.3.1-linux-arm64.tar.gz`, `terraview-0.3.1-darwin-arm64.tar.gz`, `terraview-0.3.1-windows-amd64.zip`. Verify downloads with `checksums.txt` from the same release.

On Windows, download the `.zip` and extract it with `Expand-Archive`.

Every CI run also uploads a `terraview-linux-amd64` binary as a GitHub Actions artifact.

## From source

```bash
git clone https://github.com/SlimaineB/terraview.git
cd terraview
go build -o terraview .
```

Requires Go 1.25 or newer.

## Docker

```bash
docker pull ghcr.io/slimaineb/terraview:latest
docker run --rm -v $(pwd):/workspace ghcr.io/slimaineb/terraview:latest \
  -in /workspace/plan.json
```

The image is multi-arch (linux/amd64, linux/arm64) and distroless.

## Requirements

- Go 1.25+ (for building from source)
- A modern browser (Chrome, Firefox, Edge, Safari)
- Internet access for the UI's CDN assets (Tailwind, Lucide, Google Fonts)
- Docker (optional) for the container image
- Python 3 (optional) for `make pr`

No external Go dependencies are used.