#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 || ! $1 =~ ^[1-9][0-9]*$ || ( "$2" != namespace && "$2" != vcluster && "$2" != deleted ) ]]; then
  echo 'usage: deploy/local/verify-preview.sh <PR number> namespace|vcluster|deleted' >&2
  exit 2
fi

number=$1
expected=$2
name=incident-tracker-pr-$number
context=kind-preview-platform
fail() { echo "preview $number: $*" >&2; exit 1; }
k() { kubectl --context "$context" "$@"; }

status=$(curl --noproxy '*' --fail --silent --show-error --max-time 10 "http://127.0.0.1:8090/api/previews/$name") || fail 'status API has no decision yet'
if [[ "$expected" == deleted ]]; then
  PREVIEW_STATUS_JSON=$status python3 - <<'PY' || fail 'cleanup status is not verified'
import json, os
status = json.loads(os.environ["PREVIEW_STATUS_JSON"])
assert status["phase"] == "deleted", status["phase"]
assert "cleanup-verified" in status["reasonCodes"]
assert not status.get("url")
PY
  [[ -z $(k -n argocd get application "preview-$name" --ignore-not-found -o name) ]] || fail 'Argo Application remains'
  [[ -z $(k get previewenvironment "$name" --ignore-not-found -o name) ]] || fail 'XR remains'
  [[ -z $(k get namespace "$name" --ignore-not-found -o name) ]] || fail 'namespace remains'
  [[ ! -e "$HOME/.local/share/crossplane-preview-platform/preview-gitops/previews/$name" ]] || fail 'trusted GitOps preview directory remains'
  k get persistentvolumes -o json | python3 -c 'import json,sys; namespace=sys.argv[1]; assert not [v["metadata"]["name"] for v in json.load(sys.stdin)["items"] if v.get("spec",{}).get("claimRef",{}).get("namespace")==namespace]' "$name" || fail 'persistent volume remains'
  route_code=$(curl --noproxy '*' --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 5 "http://$name.localhost:8088/healthz")
  [[ "$route_code" == 404 ]] || fail "route still returns HTTP $route_code"
  echo "preview $number: verified deletion of Application, XR, namespace, PV, and route"
  exit 0
fi

PREVIEW_STATUS_JSON=$status python3 - "$expected" "$name" <<'PY' || fail 'status is not ready for the expected mode'
import json, os, sys
status = json.loads(os.environ["PREVIEW_STATUS_JSON"])
mode, name = sys.argv[1:]
assert status["phase"] == "ready", status["phase"]
assert status["mode"] == mode, status["mode"]
assert status.get("url") == f"http://{name}.localhost:8088"
assert len(status["headSHA"]) == 40
PY
xr=$(k get previewenvironment "$name" -o json) || fail 'XR is absent'
PREVIEW_STATUS_JSON=$status PREVIEW_XR_JSON=$xr python3 - "$expected" <<'PY' || fail 'XR does not match the ready PR head and mode'
import json, os, sys
status = json.loads(os.environ["PREVIEW_STATUS_JSON"])
xr = json.loads(os.environ["PREVIEW_XR_JSON"])
assert xr["spec"]["pr"]["headSHA"] == status["headSHA"]
assert xr["spec"]["decision"]["mode"] == sys.argv[1]
assert all(any(c["type"] == condition and c["status"] == "True" for c in xr["status"]["conditions"]) for condition in ("Synced", "Ready"))
PY
curl --noproxy '*' --fail --silent --show-error --output /dev/null --max-time 5 "http://$name.localhost:8088/healthz" || fail 'app route is unhealthy'
context_json=$(curl --noproxy '*' --fail --silent --show-error --max-time 5 "http://$name.localhost:8088/api/context") || fail 'app context is unavailable'
PREVIEW_CONTEXT_JSON=$context_json python3 - "$expected" <<'PY' || fail 'app is running in the wrong preview mode'
import json, os, sys
assert json.loads(os.environ["PREVIEW_CONTEXT_JSON"])["mode"] == sys.argv[1]
PY
if [[ "$expected" == vcluster ]]; then
  [[ -z $(k get crd incidentpolicies.incidents.demo.local --ignore-not-found -o name) ]] || fail 'demo CRD leaked to the host cluster'
fi
echo "preview $number: verified ready $expected XR, matching PR head, app route, and context"
