#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

git_desc="$(git describe --always --dirty 2>/dev/null || true)"
if [ -n "$git_desc" ]; then
  version="${git_desc}-$(date -u +%Y%m%d%H%M%S)"
else
  version="dev-$(date -u +%Y%m%d%H%M%S)"
fi

echo "building forge version ${version}"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${version}" -o forge ./cmd/forge
