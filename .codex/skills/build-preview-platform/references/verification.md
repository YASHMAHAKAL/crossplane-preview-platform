# Verification and evidence

Run checks that prove user-visible behavior and controller ownership. The filenames and exact commands below are targets to adapt once the repo and pinned versions exist.

## Static and offline checks

| Check | What it must prove |
| --- | --- |
| Go unit tests for the Composition Function | Stable resource names, correct namespace/host/image-digest propagation, bounded settings, error on invalid XR, and both mode outputs. |
| Go evaluator table tests | Deterministic decision, reason code and evidence for namespace/vCluster/reject, stale CI, untrusted PR, quota, idempotent retry. |
| Incident Tracker interaction checks | Create incident with service/severity/owner, filter and inspect seeded and new incidents, change status, and preserve accessible focus and responsive layout. |
| `crossplane composition render` fixtures | Render both XRs against the matching Composition and function; inspect expected resource kinds, ownership, labels, and image digest. |
| `crossplane resource validate` | Validate XRs and rendered resources against available schemas when the pinned CLI/CRDs support it. Supplement with API-server dry-run or integration checks for schemas the CLI cannot resolve. |
| Backstage status and catalog checks | Catalog exposes the service and PR status page; MCP catalog/status actions agree with the Go status API. Future source-edit templates open ordinary PRs without a mode selector. |
| GitOps output checks | Only normalized XR/metadata appears in trusted paths; no raw PR manifests, secret values, or mutable image tags. |

The render command generally needs XR, Composition, and Function package definitions, for example `crossplane composition render <xr> <composition> <functions>`. Pin the CLI and consult its matching command reference before scripting flags. Render output proves function logic, not provider behavior or local routing.

## Local integration scenarios

1. **Bootstrap:** One documented command brings up `kind` and controllers. Check Crossplane function/provider health, Argo CD readiness, Backstage catalog entry, and ingress resolution. A fresh operator can identify missing prerequisites.

For this repository, `deploy/local/preflight.sh` checks prerequisites, `deploy/local/bootstrap.sh` converges the pinned local control plane and supervised watcher, `deploy/local/start-portal.sh` starts Backstage, and `deploy/local/verify-platform.sh --portal` checks the installed path. Keep the private Argo deploy key and MCP token outside Git. An idempotent rerun on an existing cluster is evidence for reuse; do not call it a fresh-machine validation unless the absent-cluster path has actually run.
2. **Namespace PR:** Open a trusted code-only PR. Required CI succeeds for the current SHA. Evaluator explains `namespace`; GitOps contains one XR; Argo CD syncs; Crossplane reports ready; app health and create/update incident work at the local URL.
3. **vCluster PR:** Change the baseline IncidentPolicy CRD by adding `critical` to its severity enum, optionally together with bounded deployment settings. Evaluator explains `vcluster`; the changed enum and app exist in the virtual API server; a critical IncidentPolicy instance is admitted there; the host cluster does not gain the demo CRD; ingress reaches the app. Crossplane source changes require a separate isolated evaluation path.
4. **Rejection:** Ask for an unsupported privileged change or exceed a configured quota. Backstage and Codex show the same explanation; no XR or preview resources appear.
5. **Update:** Push a new PR head. After its own CI succeeds, the same preview serves the new digest. Old check results cannot make it ready.
6. **Merge/close:** Test each event. GitOps path and Argo CD Application disappear; XR, composed resources, URL route, and credentials disappear. If deletion stalls, status says `cleanup-failed` with the remaining resource and reason.
7. **Recovery:** Restart evaluator or replay a PR event. It reconstructs the desired state without duplicate previews or losing a deletion request.

Collect logs and resource snapshots only as needed to diagnose a failed scenario. Do not mark an integration complete from mocked status alone.

## Measurement record

For each measured PR, record source SHA, mode, start time (PR/CI success), decision time, URL-ready time, cleanup start/end, peak pod/memory use where measurable, and outcome. Publish the sample size and environment hardware with p50/p95; if there are too few samples, report raw timings instead. Keep a short table of expected versus observed behavior and a list of known limits. Resume claims should match this evidence.
