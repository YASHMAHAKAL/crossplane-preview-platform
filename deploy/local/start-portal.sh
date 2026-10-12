#!/usr/bin/env bash
set -euo pipefail

if (( $# > 1 )) || { (( $# == 1 )) && [[ "$1" != --prepare-only ]]; }; then
  echo 'usage: deploy/local/start-portal.sh [--prepare-only]' >&2
  exit 2
fi
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
portal_root=$repo_root/platform/backstage/portal
state_root=$HOME/.local/state/crossplane-preview-platform
token_file=$state_root/mcp-token
[[ $(node --version) =~ ^v(22|24)\. ]] || { echo 'Backstage requires Node 22 or 24.' >&2; exit 1; }

umask 077
mkdir -p "$state_root"
chmod 700 "$state_root"
if [[ -z ${MCP_TOKEN:-} ]]; then
  if [[ -n ${PREVIEW_MCP_TOKEN:-} ]]; then
    export MCP_TOKEN=$PREVIEW_MCP_TOKEN
  else
    if [[ ! -f "$token_file" ]]; then
      python3 -c 'import secrets; print(secrets.token_urlsafe(48))' > "$token_file"
    fi
    chmod 600 "$token_file"
    export MCP_TOKEN=$(< "$token_file")
  fi
fi
if [[ -z ${GITHUB_TOKEN:-} ]]; then
  export GITHUB_TOKEN=$(gh auth token)
fi
[[ -n "$MCP_TOKEN" && -n "$GITHUB_TOKEN" ]] || { echo 'Backstage credentials are unavailable.' >&2; exit 1; }

cd "$portal_root"
node .yarn/releases/yarn-4.13.0.cjs install --immutable
node .yarn/releases/yarn-4.13.0.cjs tsc
if [[ ${1:-} == --prepare-only ]]; then
  echo 'portal: dependencies and TypeScript check passed'
  exit 0
fi
cluster_context=kind-preview-platform
reader_namespace=backstage-system
reader_service_account=backstage-preview-reader
kubectl --context "$cluster_context" -n "$reader_namespace" get serviceaccount "$reader_service_account" >/dev/null || {
  echo 'portal: Backstage Kubernetes reader is missing; run deploy/local/bootstrap.sh first' >&2
  exit 1
}
mapfile -t cluster_connection < <(kubectl config view --raw -o json | python3 -c '
import json, sys
config = json.load(sys.stdin)
cluster_name = next(item["context"]["cluster"] for item in config["contexts"] if item["name"] == "kind-preview-platform")
cluster = next(item["cluster"] for item in config["clusters"] if item["name"] == cluster_name)
print(cluster["server"])
print(cluster.get("certificate-authority-data", ""))
')
[[ ${#cluster_connection[@]} == 2 && ${cluster_connection[0]} == https://127.0.0.1:34433 && -n ${cluster_connection[1]} ]] || {
  echo 'portal: expected the pinned kind API URL https://127.0.0.1:34433 and CA in kubeconfig' >&2
  exit 1
}
export K8S_PREVIEW_URL=${cluster_connection[0]}
export K8S_PREVIEW_CA_DATA=${cluster_connection[1]}
# Kubernetes issues a bounded token. Restart the portal after this local demo
# session to renew it; no service account credential is written to disk.
export K8S_PREVIEW_TOKEN=$(kubectl --context "$cluster_context" -n "$reader_namespace" create token "$reader_service_account" --duration=8h)
[[ -n $K8S_PREVIEW_TOKEN ]] || { echo 'portal: could not request the Kubernetes reader token' >&2; exit 1; }
echo 'portal: starting Backstage on http://localhost:3000; run verify-platform.sh --portal in another terminal'
exec node .yarn/releases/yarn-4.13.0.cjs start
