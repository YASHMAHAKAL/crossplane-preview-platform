#!/usr/bin/env bash
set -euo pipefail

# Exercise a Crossplane package and Composition in a disposable virtual API.
# The package reference can point at a candidate built from a PR's exact head.
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
context=kind-preview-platform
namespace=crossplane-candidate-spike
release=crossplane-candidate-vc
port=18443
package=${1:-ghcr.io/yashmahakal/function-preview-resources:v0.1.5}

fail() { echo "isolation-gate: $*" >&2; exit 1; }
note() { echo "isolation-gate: $*"; }
host() { kubectl --context "$context" "$@"; }
guest() { kubectl --kubeconfig "$kubeconfig" "$@"; }

[[ $package =~ ^[a-z0-9][a-z0-9./_-]+(:[a-zA-Z0-9._-]+|@sha256:[a-f0-9]{64})$ ]] || fail 'pass a valid OCI Function package reference'
(( $# <= 1 )) || fail 'pass at most one OCI Function package reference'
if (( $# == 1 )); then
  [[ $package =~ @sha256:[a-f0-9]{64}$ ]] || fail 'candidate Function package must use an immutable sha256 digest'
fi
for binary in base64 helm kubectl python3; do
  command -v "$binary" >/dev/null || fail "install $binary"
done
host --request-timeout=5s get node >/dev/null || fail "$context is unavailable"
if host --request-timeout=5s get namespace "$namespace" >/dev/null 2>&1; then
  fail "$namespace already exists; inspect and remove it before starting this gate"
fi
host_package=$(host --request-timeout=5s get function.pkg.crossplane.io/function-preview-resources -o jsonpath='{.spec.package}')
[[ -n $host_package ]] || fail 'host Function is missing'

umask 077
kubeconfig=$(mktemp /tmp/preview-candidate-kubeconfig.XXXXXXXX)
forward_log=$(mktemp /tmp/preview-candidate-forward.XXXXXXXX)
forward_pid=
created=false
cleanup() {
  result=$?
  trap - EXIT
  if [[ -n $forward_pid ]]; then
    kill "$forward_pid" 2>/dev/null || true
    wait "$forward_pid" 2>/dev/null || true
  fi
  rm -f "$kubeconfig" "$forward_log"
  if [[ $created == true ]]; then
    note "removing temporary vCluster and $namespace"
    helm uninstall "$release" --namespace "$namespace" --kube-context "$context" --wait --timeout 2m >/dev/null 2>&1 || true
    host delete namespace "$namespace" --wait=true --timeout=2m >/dev/null 2>&1 || true
    if host --request-timeout=5s get namespace "$namespace" >/dev/null 2>&1; then
      echo "isolation-gate: cleanup incomplete; namespace $namespace remains" >&2
      result=1
    fi
    remaining_pvs=
    for ((attempt=0; attempt<15; attempt++)); do
      remaining_pvs=$(host --request-timeout=5s get pv -o json | python3 -c 'import json,sys; ns=sys.argv[1]; print(" ".join(p["metadata"]["name"] for p in json.load(sys.stdin)["items"] if p.get("spec",{}).get("claimRef",{}).get("namespace")==ns))' "$namespace") || { result=1; break; }
      [[ -z $remaining_pvs ]] && break
      sleep 2
    done
    if [[ -n ${remaining_pvs:-} ]]; then
      echo "isolation-gate: cleanup incomplete; persistent volumes remain: $remaining_pvs" >&2
      result=1
    fi
  fi
  exit "$result"
}
trap cleanup EXIT

note 'installing temporary vCluster OSS 0.36.0'
created=true
helm install "$release" vcluster --repo https://charts.loft.sh --version 0.36.0 \
  --namespace "$namespace" --create-namespace --kube-context "$context" \
  --set controlPlane.statefulSet.image.registry=ghcr.io \
  --set controlPlane.statefulSet.image.repository=loft-sh/vcluster-oss \
  --wait --timeout 5m >/dev/null
host -n "$namespace" wait --for=condition=Ready "pod/$release-0" --timeout=2m >/dev/null

for ((attempt=0; attempt<30; attempt++)); do
  if host -n "$namespace" get secret "vc-$release" -o jsonpath='{.data.config}' 2>/dev/null | base64 --decode > "$kubeconfig"; then
    [[ -s $kubeconfig ]] && break
  fi
  sleep 2
done
[[ -s $kubeconfig ]] || fail 'vCluster kubeconfig was not created'
python3 - "$kubeconfig" "$port" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
config = path.read_text()
old = "https://localhost:8443"
if config.count(old) != 1:
    raise SystemExit("unexpected vCluster kubeconfig server")
path.write_text(config.replace(old, f"https://localhost:{sys.argv[2]}"))
PY
host -n "$namespace" port-forward "pod/$release-0" "$port:8443" > "$forward_log" 2>&1 &
forward_pid=$!
for ((attempt=0; attempt<30; attempt++)); do
  if guest --request-timeout=3s get node >/dev/null 2>&1; then break; fi
  kill -0 "$forward_pid" 2>/dev/null || fail 'vCluster port forward exited'
  sleep 2
done
guest --request-timeout=5s get node >/dev/null || fail 'vCluster API is unavailable'

note 'installing Crossplane 2.4.0 inside the virtual API'
helm --kubeconfig "$kubeconfig" install crossplane crossplane \
  --repo https://charts.crossplane.io/stable --version 2.4.0 \
  --namespace crossplane-system --create-namespace --wait --timeout 5m >/dev/null
guest wait --for=create deploymentruntimeconfig.pkg.crossplane.io/default --timeout=2m >/dev/null

note "installing virtual Function package $package"
cat <<EOF | guest apply -f - >/dev/null
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-preview-resources
spec:
  package: $package
EOF
guest wait --for=condition=Healthy function.pkg.crossplane.io/function-preview-resources --timeout=5m >/dev/null
guest apply -f "$repo_root/platform/crossplane/rbac/composed-resources.yaml" \
  -f "$repo_root/platform/crossplane/xrd.yaml" \
  -f "$repo_root/platform/crossplane/compositions/" >/dev/null
guest wait --for=condition=Established xrd/previewenvironments.preview.platform.example.org --timeout=2m >/dev/null

note 'checking virtual Crossplane API and package health'
guest get composition preview-namespace preview-vcluster >/dev/null
guest get crd previewenvironments.preview.platform.example.org >/dev/null
guest_package=$(guest get function.pkg.crossplane.io/function-preview-resources -o jsonpath='{.spec.package}')
[[ $guest_package == "$package" ]] || fail 'virtual Function package changed unexpectedly'
[[ $(host --request-timeout=5s get function.pkg.crossplane.io/function-preview-resources -o jsonpath='{.spec.package}') == "$host_package" ]] \
  || fail 'host Function package changed'
note 'PASS: virtual Crossplane, Function, XRD, and Compositions are installed; host Function stayed unchanged'
