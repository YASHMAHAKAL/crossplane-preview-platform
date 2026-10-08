#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || ( "$1" != --services-only && "$1" != --delete-cluster ) ]]; then
  echo 'usage: deploy/local/teardown.sh --services-only|--delete-cluster' >&2
  exit 2
fi

if [[ "$1" == --delete-cluster ]]; then
  open_prs=$(gh pr list --repo YASHMAHAKAL/crossplane-preview-platform --state open --json number --jq 'length')
  [[ "$open_prs" == 0 ]] || { echo "Refusing cluster deletion: $open_prs source PR(s) are open." >&2; exit 1; }
  if kind get clusters | rg -qx preview-platform; then
    active=$(kubectl --context kind-preview-platform get previewenvironments.preview.platform.example.org -o name)
    [[ -z "$active" ]] || { echo 'Refusing cluster deletion: preview XRs remain.' >&2; exit 1; }
    active=$(kubectl --context kind-preview-platform -n argocd get applications -o name | rg '^application.argoproj.io/preview-' || true)
    [[ -z "$active" ]] || { echo 'Refusing cluster deletion: preview Argo Applications remain.' >&2; exit 1; }
  fi
  gitops_root=$HOME/.local/share/crossplane-preview-platform/preview-gitops
  if [[ -d "$gitops_root/previews" ]]; then
    active=$(find "$gitops_root/previews" -mindepth 1 -maxdepth 1 -type d -print -quit)
    [[ -z "$active" ]] || { echo 'Refusing cluster deletion: trusted GitOps preview paths remain.' >&2; exit 1; }
  fi
fi

for service in preview-watcher.service preview-status-api.service; do
  if systemctl --user cat "$service" >/dev/null 2>&1; then
    systemctl --user disable --now "$service"
  fi
done
if [[ "$1" == --delete-cluster ]] && kind get clusters | rg -qx preview-platform; then
  kind delete cluster --name preview-platform
fi
echo 'teardown: project services stopped; private GitOps checkout and credentials retained'
