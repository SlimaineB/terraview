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