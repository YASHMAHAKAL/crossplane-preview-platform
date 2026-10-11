#!/usr/bin/env bash
set -euo pipefail

number=${1:?PR number required}
[[ $number =~ ^[1-9][0-9]*$ ]] || { echo 'expected a positive PR number' >&2; exit 2; }
name=incident-tracker-pr-$number
child=$name-c
context=${PREVIEW_KUBE_CONTEXT:-kind-preview-platform}
host() { kubectl --context "$context" "$@"; }
guest() { kubectl --kubeconfig "$kubeconfig" "$@"; }

status=$(curl -fsS "http://127.0.0.1:8090/api/previews/$name")
read -r head url < <(python3 -c '
import json,sys
s=json.load(sys.stdin)
assert s["phase"]=="ready" and s["mode"]=="vcluster"
assert s["reasonCodes"][0]=="crossplane-function-change"
print(s["headSHA"],s["url"])
' <<< "$status")
[[ $url == "http://$child.localhost:8088" ]] || { echo "wrong candidate URL: $url" >&2; exit 1; }
curl -fsS "$url/healthz" >/dev/null
package=$(host get previewenvironment "$name" -o jsonpath='{.metadata.annotations.preview\.platform\.example\.org/candidate-package}')
[[ $package =~ ^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$ ]] || { echo 'host XR has no immutable candidate package' >&2; exit 1; }
host_package=$(host get function.pkg.crossplane.io/function-preview-resources -o jsonpath='{.spec.package}')
[[ $host_package != "$package" ]] || { echo 'host Function was replaced with candidate' >&2; exit 1; }

umask 077
kubeconfig=$(mktemp /tmp/preview-candidate-verify.XXXXXXXX)
forward_log=$(mktemp /tmp/preview-candidate-verify-forward.XXXXXXXX)
forward_pid=
cleanup() {
  if [[ -n $forward_pid ]]; then
    kill "$forward_pid" 2>/dev/null || true
    wait "$forward_pid" 2>/dev/null || true
  fi
  rm -f "$kubeconfig" "$forward_log"
}
trap cleanup EXIT
host -n "$name" get secret "vc-$name" -o jsonpath='{.data.config}' | base64 --decode > "$kubeconfig"
port=$(python3 - <<'PY'
import socket
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    print(listener.getsockname()[1])
PY
)
python3 - "$kubeconfig" "$port" <<'PY'
from pathlib import Path
import sys
path=Path(sys.argv[1])
config=path.read_text()
old="https://localhost:8443"
assert config.count(old)==1
path.write_text(config.replace(old,f"https://localhost:{sys.argv[2]}"))
PY
host -n "$name" port-forward "pod/$name-0" "$port:8443" > "$forward_log" 2>&1 &
forward_pid=$!
for ((attempt=0; attempt<30; attempt++)); do
  if guest --request-timeout=3s get node >/dev/null 2>&1; then break; fi
  sleep 2
done
guest --request-timeout=5s get node >/dev/null
guest get function.pkg.crossplane.io/function-preview-resources -o json | python3 -c '
import json,sys
obj=json.load(sys.stdin)
expected=sys.argv[1]
assert obj["spec"]["package"]==expected
assert obj["status"]["currentIdentifier"]==expected
' "$package"
guest get previewenvironment "$child" -o json | python3 -c '
import json,sys
obj=json.load(sys.stdin)
assert obj["spec"]["pr"]["headSHA"]==sys.argv[1]
generation=obj["metadata"]["generation"]
for kind in ("Synced","Ready"):
    assert any(c.get("type")==kind and c.get("status")=="True" and c.get("observedGeneration",0)>=generation for c in obj["status"]["conditions"])
' "$head"
guest get namespace "$child" -o json | python3 -c '
import json,sys
obj=json.load(sys.stdin)
expected=sys.argv[1]
annotation=obj["metadata"].get("annotations",{}).get("preview.platform.example.org/candidate-head","")
if len(sys.argv)>2 and sys.argv[2]=="--expect-smoke-annotation":
    assert annotation==expected, (annotation,expected)
print("virtual namespace annotation:", annotation or "(none)")
' "$head" "${2:-}"
if host get namespace "$child" >/dev/null 2>&1; then
  echo 'candidate child namespace leaked to host API' >&2
  exit 1
fi
echo "candidate-preview: PASS PR #$number head=$head package=$package URL=$url"
