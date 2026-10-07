#!/usr/bin/env bash
# Manual recovery fallback; the evaluator and Argo CD now apply this grant.
set -euo pipefail

if [[ $# -ne 1 || ! $1 =~ ^incident-tracker-pr-[1-9][0-9]*$ ]]; then
  echo "usage: $0 incident-tracker-pr-<number>" >&2
  exit 2
fi

preview_namespace=$1
repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
context=kind-preview-platform

# The grant is appropriate only for an evaluator-approved vCluster XR and the
# namespace composed for that same XR. Do not apply it to arbitrary namespaces.
mode=$(kubectl --context "$context" get previewenvironments.preview.platform.example.org "$preview_namespace" -o jsonpath='{.spec.decision.mode}')
service=$(kubectl --context "$context" get previewenvironments.preview.platform.example.org "$preview_namespace" -o jsonpath='{.spec.serviceRef}')
namespace_service=$(kubectl --context "$context" get namespace "$preview_namespace" -o jsonpath='{.metadata.labels.preview\.platform\.example\.org/service}')
managed_by=$(kubectl --context "$context" get namespace "$preview_namespace" -o jsonpath='{.metadata.labels.app\.kubernetes\.io/managed-by}')

if [[ $mode != vcluster || $service != incident-tracker || $namespace_service != "$service" || $managed_by != crossplane ]]; then
  echo "refusing installer grant: XR and composed namespace do not match an approved Incident Tracker vCluster preview" >&2
  exit 1
fi

kubectl --context "$context" -n "$preview_namespace" apply -f "$repository_root/deploy/local/vcluster-helm-installer.yaml"
