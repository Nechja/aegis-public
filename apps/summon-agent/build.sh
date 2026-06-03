#!/usr/bin/env bash
# Build the summon-agent binary for linux/amd64 and (optionally) upload to MinIO.
#
# Usage:
#   ./build.sh              # just build -> ./bin/summon-agent
#   ./build.sh upload       # build + kubectl exec into MinIO and cp via mc
#
# Upload path assumes the cluster MinIO pod has `mc` available and an alias
# named `local` pointing at http://localhost:9000 with vm-images bucket readable.
# If you run this elsewhere, swap the upload block for your own mc / aws-cli call.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$HERE"

mkdir -p bin
echo "==> go build (linux/amd64)"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/summon-agent .
ls -lh bin/summon-agent

if [[ "${1:-}" == "upload" ]]; then
    echo "==> uploading to MinIO vm-images/summon-agent"
    POD="$(kubectl -n minio get pods -l app=minio -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
    POD="${POD:-$(kubectl -n minio get pods -o jsonpath='{.items[0].metadata.name}')}"

    # MinIO's official image is distroless — no `tar`, so `kubectl cp` fails. Stream the binary
    # through `mc pipe` over stdin instead. The `local` alias is not persisted between execs
    # (~/.mc lives on an ephemeral fs), so set it each time using the root creds secret.
    AK="$(kubectl -n minio get secret minio-creds -o jsonpath='{.data.root-user}' | base64 -d)"
    SK="$(kubectl -n minio get secret minio-creds -o jsonpath='{.data.root-password}' | base64 -d)"

    kubectl -n minio exec "$POD" -- mc alias set local http://localhost:9000 "$AK" "$SK" >/dev/null
    kubectl -n minio exec -i "$POD" -- mc pipe local/vm-images/summon-agent < bin/summon-agent
    kubectl -n minio exec "$POD" -- mc ls local/vm-images/summon-agent
    echo "==> uploaded"
fi
