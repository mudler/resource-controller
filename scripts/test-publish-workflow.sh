#!/usr/bin/env bash
set -euo pipefail

workflow=.github/workflows/publish.yml

worker_build=$(
  awk '
    /^      - id: build-worker$/ { in_worker_build = 1 }
    in_worker_build && seen && /^      - / { exit }
    in_worker_build { print; seen = 1 }
  ' "$workflow"
)

if ! grep -Fq '          build-args:' <<<"$worker_build"; then
  echo "worker build does not declare build-args" >&2
  exit 1
fi

if ! grep -Fq 'RC_IMAGE=ghcr.io/${{ github.repository }}@${{ steps.build.outputs.digest }}' <<<"$worker_build"; then
  echo "worker build does not pin RC_IMAGE to the controller build digest" >&2
  exit 1
fi

echo "publish workflow pins the worker to the controller build digest"
