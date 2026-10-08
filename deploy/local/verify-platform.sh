#!/usr/bin/env bash
set -euo pipefail

context=kind-preview-platform
portal=false
if [[ ${1:-} == --portal && $# -eq 1 ]]; then
  portal=true
elif (( $# != 0 )); then
  echo 'usage: deploy/local/verify-platform.sh [--portal]' >&2
  exit 2
fi

fail() { echo "verify: $*" >&2; exit 1; }
k() { kubectl --context "$context" "$@"; }
ready() { k wait --for="condition=$1" "$2" --timeout=30s >/dev/null || fail "$2 is not $1"; }

ready Ready node/preview-platform-control-plane
for item in crossplane-system/crossplane argocd/argocd-applicationset-controller argocd/argocd-repo-server argocd/argocd-server nginx-ingress/nginx-ingress-controller; do
  namespace=${item%%/*}
  deployment=${item#*/}
  k rollout status "deployment/$deployment" -n "$namespace" --timeout=30s >/dev/null || fail "$deployment is not ready"
done
ready Healthy provider.pkg.crossplane.io/provider-helm
ready Healthy function.pkg.crossplane.io/function-preview-resources
ready Established xrd/previewenvironments.preview.platform.example.org
for composition in preview-namespace preview-vcluster; do
  k get composition "$composition" >/dev/null || fail "missing Composition $composition"
done
k -n argocd get appproject preview-environments >/dev/null || fail 'missing Argo CD AppProject'
repo_secrets=$(k -n argocd get secrets -l argocd.argoproj.io/secret-type=repository -o name)
printf '%s\n' "$repo_secrets" | grep -Fxq 'secret/preview-gitops-repo' || fail 'Argo CD GitOps repository Secret is missing its label'
configured_url=$(k -n argocd get secret preview-gitops-repo -o jsonpath='{.data.url}' | python3 -c 'import base64,sys; print(base64.b64decode(sys.stdin.read()).decode())')
[[ "$configured_url" == git@github.com:YASHMAHAKAL/preview-gitops.git ]] || fail 'Argo CD GitOps repository URL is incorrect'
appset_health=
for attempt in {1..30}; do
  appset_health=$(k -n argocd get applicationset preview-environments -o jsonpath='{.status.health.status}')
  [[ "$appset_health" == Healthy ]] && break
  sleep 10
done
[[ "$appset_health" == Healthy ]] || fail "ApplicationSet is $appset_health; inspect its conditions and GitOps access"

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
"$repo_root/deploy/local/preview-service-health.sh" >/dev/null || fail 'watcher/status API is unhealthy'
route_code=$(curl --noproxy '*' --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 5 http://preview-route-check.localhost:8088/healthz)
[[ "$route_code" == 404 ]] || fail "ingress probe returned HTTP $route_code instead of 404"
echo 'verify: kind node, Crossplane/provider/function, Argo CD, GitOps access, ingress, watcher, and status API are ready'

if "$portal"; then
  curl --noproxy '*' --fail --silent --show-error --output /dev/null --max-time 10 http://127.0.0.1:3000/ || fail 'Backstage frontend is unavailable'
  portal_token=${MCP_TOKEN:-${PREVIEW_MCP_TOKEN:-}}
  if [[ -z "$portal_token" && -r "$HOME/.local/state/crossplane-preview-platform/mcp-token" ]]; then
    portal_token=$(< "$HOME/.local/state/crossplane-preview-platform/mcp-token")
  fi
  [[ -n "$portal_token" ]] || fail 'set PREVIEW_MCP_TOKEN or use the local mcp-token file for authenticated catalog checks'
  for entity in component/default/incident-tracker template/default/request-incident-tracker-preview; do
    catalog_code=
    for attempt in {1..12}; do
      catalog_code=$(printf 'header = "Authorization: Bearer %s"\n' "$portal_token" |
        curl --config - --noproxy '*' --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 10 "http://127.0.0.1:7007/api/catalog/entities/by-name/$entity") || fail "Backstage catalog request failed for $entity"
      [[ "$catalog_code" == 200 ]] && break
      [[ "$catalog_code" == 401 || "$catalog_code" == 403 ]] && fail 'Backstage catalog token was rejected'
      sleep 5
    done
    [[ "$catalog_code" == 200 ]] || fail "Backstage catalog is unavailable for $entity (HTTP $catalog_code)"
  done
  echo 'verify: Backstage frontend and Incident Tracker catalog/template are ready'
fi
