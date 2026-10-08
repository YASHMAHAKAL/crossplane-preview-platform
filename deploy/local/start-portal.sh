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
echo 'portal: starting Backstage on http://localhost:3000; run verify-platform.sh --portal in another terminal'
exec node .yarn/releases/yarn-4.13.0.cjs start
