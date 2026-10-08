#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cluster=preview-platform
context=kind-preview-platform
gitops_url=git@github.com:YASHMAHAKAL/preview-gitops.git

fail() { echo "preflight: $*" >&2; exit 1; }
check() { command -v "$1" >/dev/null || fail "install $1 and put it on PATH"; }

for command in bash curl docker gh git go helm kind kubectl node python3 systemctl; do
  check "$command"
done
[[ -f "$repo_root/deploy/local/kind.yaml" ]] || fail 'run from a complete project checkout'
[[ $(node --version) =~ ^v(22|24)\. ]] || fail 'Backstage requires Node 22 or 24'
go_version=$(go version | awk '{print $3}')
python3 - "$go_version" <<'PY' || fail 'evaluator and Function modules require Go 1.25.10 or newer'
import re, sys
match = re.fullmatch(r"go(\d+)\.(\d+)(?:\.(\d+))?", sys.argv[1])
assert match and tuple(int(part or 0) for part in match.groups()) >= (1, 25, 10)
PY
systemctl --user show-environment >/dev/null 2>&1 || fail 'the user systemd manager is unavailable'
docker info >/dev/null 2>&1 || fail 'Docker daemon is unavailable to this user'
gh auth token >/dev/null 2>&1 || fail 'run gh auth login with access to GitHub Actions artifacts'
GIT_TERMINAL_PROMPT=0 git ls-remote "$gitops_url" HEAD >/dev/null 2>&1 || fail "SSH read access to $gitops_url is unavailable"

if kind get clusters | grep -Fxq -- "$cluster"; then
  kubectl --context "$context" get node >/dev/null || fail "$context is unavailable"
  if ! kubectl --context "$context" -n argocd get secret preview-gitops-repo >/dev/null 2>&1; then
    [[ -r ${PREVIEW_ARGO_REPO_KEY_FILE:-} ]] || fail 'set PREVIEW_ARGO_REPO_KEY_FILE to an existing read-only GitOps deploy key'
  fi
  echo "preflight: reusing $context"
else
  [[ -r ${PREVIEW_ARGO_REPO_KEY_FILE:-} ]] || fail 'set PREVIEW_ARGO_REPO_KEY_FILE to a read-only GitOps deploy key before creating the cluster'
  python3 - <<'PY' || fail 'host port 8088 is already in use'
import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 8088))
PY
  echo "preflight: ready to create kind cluster $cluster"
fi

echo "preflight: Node $(node --version), Go $go_version, $(kind version | awk '{print $2}')"
echo 'preflight: GitHub credential, private GitOps SSH access, and Docker are available'
