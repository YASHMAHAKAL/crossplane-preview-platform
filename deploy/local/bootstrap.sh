#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cluster=preview-platform
context=kind-preview-platform
argo_repo=git@github.com:YASHMAHAKAL/preview-gitops.git
argo_manifest=https://raw.githubusercontent.com/argoproj/argo-cd/v3.5.2/manifests/install.yaml

note() { echo "bootstrap: $*"; }
fail() { echo "bootstrap: $*" >&2; exit 1; }
k() { kubectl --context "$context" "$@"; }

"$repo_root/deploy/local/preflight.sh"

if ! kind get clusters | grep -Fxq -- "$cluster"; then
  note "creating $cluster from the pinned kind config"
  kind create cluster --config "$repo_root/deploy/local/kind.yaml"
fi
k wait --for=condition=Ready node --all --timeout=5m

chart_version() {
  helm list -n "$2" -o json | python3 -c 'import json,sys; name=sys.argv[1]; charts=[item["chart"] for item in json.load(sys.stdin) if item["name"]==name]; print(charts[0] if charts else "")' "$1"
}

installed=$(chart_version crossplane crossplane-system)
if [[ -z "$installed" ]]; then
  note 'installing Crossplane 2.4.0'
  helm repo add crossplane-stable https://charts.crossplane.io/stable
  helm repo update crossplane-stable
  helm install crossplane crossplane-stable/crossplane --version 2.4.0 --namespace crossplane-system --create-namespace --kube-context "$context" --wait --timeout 10m
elif [[ "$installed" != crossplane-2.4.0 ]]; then
  fail "Crossplane chart is $installed; expected crossplane-2.4.0. Resolve the version manually."
else
  note 'Crossplane 2.4.0 already installed'
fi
k rollout status deployment/crossplane -n crossplane-system --timeout=5m

if ! k get namespace argocd >/dev/null 2>&1; then
  k create namespace argocd
fi
if ! k get deployment argocd-applicationset-controller -n argocd >/dev/null 2>&1; then
  note 'installing Argo CD v3.5.2'
  k apply -n argocd --server-side --force-conflicts -f "$argo_manifest"
else
  image=$(k get deployment argocd-server -n argocd -o jsonpath='{.spec.template.spec.containers[0].image}')
  [[ "$image" == *:v3.5.2 ]] || fail "Argo CD image is $image; expected v3.5.2. Resolve the version manually."
  note 'Argo CD v3.5.2 already installed'
fi
for deployment in argocd-applicationset-controller argocd-repo-server argocd-server; do
  k rollout status "deployment/$deployment" -n argocd --timeout=10m
done

if k -n argocd get secret preview-gitops-repo >/dev/null 2>&1; then
  configured_url=$(k -n argocd get secret preview-gitops-repo -o jsonpath='{.data.url}' | python3 -c 'import base64,sys; print(base64.b64decode(sys.stdin.read()).decode())')
  [[ "$configured_url" == "$argo_repo" ]] || fail 'existing Argo repository Secret points to a different URL'
  note 'Argo CD read-only GitOps credential already configured'
else
  key_file=${PREVIEW_ARGO_REPO_KEY_FILE:-}
  [[ -r "$key_file" ]] || fail 'PREVIEW_ARGO_REPO_KEY_FILE is required for Argo CD GitOps access'
  note 'installing the provided read-only Argo CD deploy key'
  k -n argocd create secret generic preview-gitops-repo \
    --from-literal=type=git --from-literal="url=$argo_repo" \
    --from-file="sshPrivateKey=$key_file" --dry-run=client -o yaml | k apply -f - >/dev/null
  k -n argocd label secret preview-gitops-repo argocd.argoproj.io/secret-type=repository
fi

installed=$(chart_version nginx-ingress nginx-ingress)
if [[ -z "$installed" ]]; then
  note 'installing NGINX ingress 2.7.3'
  helm install nginx-ingress oci://ghcr.io/nginx/charts/nginx-ingress --version 2.7.3 --namespace nginx-ingress --create-namespace --values "$repo_root/deploy/local/nginx-ingress-values.yaml" --kube-context "$context" --wait --timeout 10m
elif [[ "$installed" != nginx-ingress-2.7.3 ]]; then
  fail "NGINX ingress chart is $installed; expected nginx-ingress-2.7.3. Resolve the version manually."
else
  note 'NGINX ingress 2.7.3 already installed'
fi
k rollout status deployment/nginx-ingress-controller -n nginx-ingress --timeout=5m

note 'applying Crossplane RBAC, provider, function, API, and Compositions'
k apply -f "$repo_root/platform/crossplane/rbac/composed-resources.yaml"
k apply -f "$repo_root/platform/crossplane/provider-helm-runtime.yaml"
k apply -f "$repo_root/platform/crossplane/provider-helm.yaml"
k wait --for=condition=healthy provider.pkg.crossplane.io/provider-helm --timeout=10m
k apply -f "$repo_root/platform/crossplane/provider-helm-config.yaml"
k apply -f "$repo_root/platform/crossplane/functions.yaml"
k wait --for=condition=healthy function.pkg.crossplane.io/function-preview-resources --timeout=10m
k apply -f "$repo_root/platform/crossplane/xrd.yaml"
k wait --for=condition=established xrd/previewenvironments.preview.platform.example.org --timeout=5m
k apply -f "$repo_root/platform/crossplane/compositions/"
k apply -f "$repo_root/platform/argocd/preview-appset.yaml"

note 'installing persistent watcher and status API services'
"$repo_root/deploy/local/install-preview-services.sh"
"$repo_root/deploy/local/verify-platform.sh"
note 'platform control plane is ready; run deploy/local/start-portal.sh in another terminal'
