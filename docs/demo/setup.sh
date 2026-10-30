#!/usr/bin/env bash
set -euo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dir="${K2_DEMO_DIR:-/tmp/k2-demo}"
mkdir -p "$dir/bin"
cd "$repo"
version="$(git describe --tags --abbrev=0 2>/dev/null || echo dev)"
go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$dir/bin/k2" ./cmd/k2
go build -trimpath -o "$dir/bin/fakekube" ./tools/demo/fakekube
ln -sf fakekube "$dir/bin/kubectl"
ln -sf fakekube "$dir/bin/helm"
echo "demo binaries in $dir/bin (k2 $version)"
