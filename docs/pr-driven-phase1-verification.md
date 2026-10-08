# PR-driven Phase 1 live verification

On 2026-10-08, [PR #16](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/16) tested the automatic app-change path against the local `kind-preview-platform` cluster. The PR changed only `app/incident-tracker/public/index.html`; it contained no `preview.request.json` and no environment selection.

| Check | Observed result |
| --- | --- |
| Initial head `cbd3b76629ba2a8c9a38abc45f05f9862cdf0f3f` | The `Preview image` build passed. The Go watcher selected `namespace` with `namespaced-app-change` and cited the changed HTML path. |
| First preview | `deploy/local/verify-preview.sh 16 namespace` passed: current-head Ready XR, matching mode, healthy app route and context. The served page contained `PR-DIFF PREVIEW LIVE`. |
| Updated head `b163c1cea8e88586986adc3b7a89357c1fb885c3` | CI passed. The watcher recorded the new head and preserved the original expiry. Argo CD synced the new trusted GitOps revision; the status API returned `ready` for that head, and the page contained `PR-DIFF PREVIEW UPDATED`. |
| Backstage | `deploy/local/verify-platform.sh --portal` passed. The catalog Component linked to `/previews`; the retired request Template returned HTTP 404 from the catalog. |
| Closed PR | The watcher removed the trusted XR path and reported `cleaning` while Argo still had resources. It later reported `deleted` with `pr-closed` and `cleanup-verified`. `deploy/local/verify-preview.sh 16 deleted` confirmed the Application, XR, namespace, persistent volume, and route were absent. |

The observed close-to-verified-deletion interval was about five minutes (`cleaning` at 10:49:02 UTC; `deleted` at 10:53:54 UTC). The ApplicationSet Git generator used a 180-second requeue and one pass still saw its cached old directory before pruning on the next pass. This local result is a single sample, not a latency percentile or availability claim.

Phase 1 covers normal Incident Tracker app PRs. General deployment and Crossplane file changes remain rejected until their virtual-cluster evaluation path is built. The exact allowlisted IncidentPolicy CRD path remains a transitional vCluster demonstration.
