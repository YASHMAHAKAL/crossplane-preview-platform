# PR-driven Phase 2 live verification

On 2026-10-08, [PR #17](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/17) tested a real deployment configuration edit against the local `kind-preview-platform` cluster. Its only changed source file was `deploy/incident-tracker/preview.json`; the PR did not contain an environment request or selected preview mode. The edit changed replicas from `1` to `2`, requests from `100m`/`128Mi` to `250m`/`256Mi`, and limits from `500m`/`512Mi` to `1000m`/`1024Mi`.

| Check | Observed result |
| --- | --- |
| PR head `6828d6411eb3afae2c7952d21e5f170b524f99a4` | The `Preview image` build and GitGuardian checks passed. The Go watcher fetched the config at this exact head and selected `vcluster` with `deployment-stack-change`, citing `deploy/incident-tracker/preview.json:replicas=2`. |
| Trusted GitOps | The `PreviewEnvironment` XR contained normalized replicas and resource values. Its capabilities list was empty; the developer's JSON was not copied into Kubernetes as an arbitrary manifest. |
| Crossplane | The published Function package `v0.1.3` was Healthy and Installed. The updated XRD was Established. The namespace and vCluster render fixtures both passed with Crossplane CLI v2.5.0; the vCluster render contained the two-replica resource settings. |
| Live vCluster | `deploy/local/verify-preview.sh 17 vcluster` passed with a Ready XR, matching head, healthy route, and vCluster app context. The host cluster had no `incidentpolicies.incidents.demo.local` CRD. |
| Virtual Deployment | Through the virtual cluster's API, `incident-tracker` had 2 desired, 2 ready, and 2 available replicas. Its requests were `250m`/`256Mi`; Kubernetes reported the `1000m`/`1024Mi` limits as `1` CPU/`1Gi`. Two virtual app pods were present. |
| Backstage | `deploy/local/verify-platform.sh --portal` passed. The catalog Component linked to the PR Previews page and the deployment contract guide. |
| Closed PR | GitHub recorded closure at 11:27:12 UTC. The watcher published `cleaning` at 11:27:49 UTC and removed the preview URL. It reported `deleted`/`pr-closed`/`cleanup-verified` at 11:31:34 UTC. `deploy/local/verify-preview.sh 17 deleted` independently confirmed the Application, XR, namespace, persistent volume, trusted GitOps directory, and route were absent. |

The virtual Deployment check used a temporary kubeconfig with private file permissions and a short-lived port forward. Both were removed after the check. The exact commands for a future manual run are in [the deployment preview guide](deployment-preview.md).

Closure to verified deletion took about 4 minutes 22 seconds in this single local run, without a manual ApplicationSet refresh. The earlier cleanup status was accurate while Argo CD completed its normal reconcile and cascading deletion; this result is one observation, not a latency percentile.

This slice supports only the bounded Incident Tracker deployment config. Service type changes, arbitrary deployment manifests, and Crossplane source changes still require their own validated evaluation path. Pulseboard stores incidents in each pod's `emptyDir`; with two replicas, records are independent between pods.
