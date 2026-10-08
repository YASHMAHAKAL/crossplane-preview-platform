#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state_root=$HOME/.local/state/crossplane-preview-platform
gitops_root=$HOME/.local/share/crossplane-preview-platform/preview-gitops
binary_root=$HOME/.local/libexec/crossplane-preview-platform
config_path=${PREVIEW_CONFIG:-"$repo_root/platform/evaluator/config.example.json"}

# gh obtains the existing operator credential at service start. It never
# appears in the unit, checkout, or command line of the watcher process.
export GITHUB_TOKEN=$(gh auth token)
exec "$binary_root/watcher" \
  -config "$config_path" \
  -gitops "$gitops_root" \
  -kube-context kind-preview-platform \
  -preview-port 8088 \
  -health-file "$state_root/watcher-health.json"
