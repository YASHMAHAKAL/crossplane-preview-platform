#!/usr/bin/env bash
set -euo pipefail

systemctl --user is-active preview-watcher.service preview-status-api.service
curl --fail --silent --show-error --max-time 5 http://127.0.0.1:8090/healthz
echo
