#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
gitops_root=$HOME/.local/share/crossplane-preview-platform/preview-gitops
state_root=$HOME/.local/state/crossplane-preview-platform
binary_root=$HOME/.local/libexec/crossplane-preview-platform
unit_root=$HOME/.config/systemd/user

if [[ "$repo_root" == *[[:space:]]* || "$HOME" == *[[:space:]]* ]]; then
  echo 'Repository and home paths with spaces are unsupported by the generated systemd units.' >&2
  exit 1
fi
command -v systemctl >/dev/null
command -v gh >/dev/null
command -v git >/dev/null
command -v go >/dev/null
command -v kubectl >/dev/null
command -v python3 >/dev/null
gh auth token >/dev/null
kubectl --context kind-preview-platform get namespace argocd >/dev/null

mkdir -p "$(dirname "$gitops_root")" "$state_root" "$binary_root" "$unit_root"
chmod 700 "$state_root"
if [[ ! -d "$gitops_root/.git" ]]; then
  if [[ -e "$gitops_root" ]]; then
    echo "GitOps path exists but is not a checkout: $gitops_root" >&2
    exit 1
  fi
  git clone git@github.com:YASHMAHAKAL/preview-gitops.git "$gitops_root"
fi
remote=$(git -C "$gitops_root" remote get-url origin)
if [[ "$remote" != 'git@github.com:YASHMAHAKAL/preview-gitops.git' ]]; then
  echo "Unexpected GitOps origin: $remote" >&2
  exit 1
fi
if [[ -n $(git -C "$gitops_root" status --porcelain) ]]; then
  echo 'GitOps checkout is dirty; resolve it before installing services.' >&2
  exit 1
fi
git -C "$gitops_root" fetch origin main
if [[ $(git -C "$gitops_root" rev-parse HEAD) != $(git -C "$gitops_root" rev-parse origin/main) ]]; then
  echo 'GitOps checkout differs from origin/main; resolve it before installing services.' >&2
  exit 1
fi

(cd "$repo_root/platform/evaluator" && go build -o "$binary_root/watcher.new" ./cmd/watcher && go build -o "$binary_root/status-api.new" ./cmd/status-api)
mv -f "$binary_root/watcher.new" "$binary_root/watcher"
mv -f "$binary_root/status-api.new" "$binary_root/status-api"

cat > "$unit_root/preview-watcher.service" <<EOF
[Unit]
Description=Crossplane preview GitHub watcher
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$repo_root
Environment=PATH=$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin:/snap/bin:/snap/go/current/bin
ExecStart=/bin/bash $repo_root/deploy/local/run-preview-watcher.sh
Restart=always
RestartSec=10
NoNewPrivileges=yes

[Install]
WantedBy=default.target
EOF

cat > "$unit_root/preview-status-api.service" <<EOF
[Unit]
Description=Crossplane preview status API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$repo_root
Environment=PATH=$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin:/snap/bin:/snap/go/current/bin
ExecStart=$binary_root/status-api -gitops $gitops_root -listen 127.0.0.1:8090 -kube-context kind-preview-platform -preview-port 8088 -watcher-health-file $state_root/watcher-health.json
Restart=always
RestartSec=10
NoNewPrivileges=yes

[Install]
WantedBy=default.target
EOF

systemd-analyze --user verify "$unit_root/preview-watcher.service" "$unit_root/preview-status-api.service"
previous_success=$(python3 - "$state_root/watcher-health.json" <<'PY'
import json, sys
try:
    with open(sys.argv[1], encoding="utf-8") as heartbeat:
        print(json.load(heartbeat).get("lastSuccessAt", ""))
except FileNotFoundError:
    print("")
PY
)
systemctl --user daemon-reload
systemctl --user enable --now preview-watcher.service preview-status-api.service
systemctl --user restart preview-watcher.service preview-status-api.service
for attempt in {1..30}; do
  if "$repo_root/deploy/local/preview-service-health.sh"; then
    latest_success=$(python3 - "$state_root/watcher-health.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as heartbeat:
    print(json.load(heartbeat).get("lastSuccessAt", ""))
PY
)
    if [[ -n "$latest_success" && "$latest_success" != "$previous_success" ]]; then
      exit 0
    fi
  fi
  sleep 5
done
echo 'Preview services did not become healthy; inspect journalctl --user -u preview-watcher -u preview-status-api.' >&2
exit 1
