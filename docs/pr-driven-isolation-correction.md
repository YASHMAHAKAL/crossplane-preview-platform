# Impact-based preview decision: live verification

On 2026-10-08, policy version 4 corrected the isolation choice for bounded Incident Tracker deployment settings. App edits and validated replica/resource settings select a namespace. The exact allowlisted IncidentPolicy CRD selects a vCluster, including a mixed CRD and deployment PR. Unsupported service types, other deployment files, and Crossplane source edits remain rejected. The policy table and both Composition renders verify these paths offline.

[PR #18](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/18) changed only `deploy/incident-tracker/preview.json`: replicas `1` to `2`, requests to `250m`/`256Mi`, and limits to `1000m`/`1024Mi`. It contained no environment selector. Its exact head was `742b0a98151ab936a1a89b1faa2ce8b4ec5d3c32`.

| Check | Observed result |
| --- | --- |
| CI | The `Preview image` build and GitGuardian checks passed for the PR head. |
| Go evaluator | Selected `namespace` with `namespaced-deployment-change`; evidence was `deploy/incident-tracker/preview.json:replicas=2`. |
| Trusted GitOps | Wrote only the normalized deployment fields into a `preview-namespace` XR. Its capabilities array was empty; no installer Role or vCluster Release was created. |
| Crossplane | Function package `ghcr.io/yashmahakal/function-preview-resources:v0.1.4` was Installed and Healthy. The namespace Composition rendered a Deployment, quota, Service, and Ingress; the vCluster fixture with CRD plus deployment settings rendered the virtual workload. |
| Live namespace | `deploy/local/verify-preview.sh 18 namespace` passed for the Ready XR, exact head, app route, and mode context. The Deployment had 2 desired, 2 ready, and 2 available replicas; two pods were Running. Requests were `250m`/`256Mi`; Kubernetes normalized limits to `1` CPU/`1Gi`. The Service type was `ClusterIP`. |
| Rollout quota | The namespace quota allowed `750m` CPU and `768Mi` memory in requests. The two pods used `500m`/`512Mi`, leaving one pod's maximum accepted requests for a rolling-update surge. |
| Closed PR | GitHub recorded closure at 12:06:23 UTC. The watcher entered `cleaning` at 12:06:44 UTC and removed the preview URL. It reported `deleted`/`pr-closed`/`cleanup-verified` at 12:12:02 UTC. `deploy/local/verify-preview.sh 18 deleted` independently confirmed the Application, XR, namespace, persistent volumes, trusted GitOps directory, and route were absent. |

[PR #17](pr-driven-phase2-verification.md) is historical evidence from policy version 3, which sent the same deployment edit to a vCluster. It does not represent the current isolation decision. This run verifies the new namespace decision and workload. The mixed CRD and deployment decision has offline policy and render checks; it has not been separately exercised as a live PR under policy version 4.

Closure to verified deletion took about 5 minutes 40 seconds in this one local run. No manual ApplicationSet refresh was used. This is a single observation, not a latency percentile.
