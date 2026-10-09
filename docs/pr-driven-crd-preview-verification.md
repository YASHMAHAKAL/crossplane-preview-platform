# Bounded CRD edit: live vCluster verification

On 2026-10-09, [PR #19](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/19) changed two ordinary source files: it added `critical` to the IncidentPolicy CRD severity enum and changed the bounded Incident Tracker deployment config to two replicas with `250m`/`256Mi` requests and `1000m`/`1024Mi` limits. The PR had no environment selector. Its exact head was `3680502556bbe433bd564f92f71c3b6070386c15`.

| Check | Observed result |
| --- | --- |
| CI | The `Preview image` build and GitGuardian checks passed for this head. The Function v0.1.5 package release also passed; Crossplane reported the installed Function Healthy. |
| Go policy | Selected `vcluster` with `cluster-api-required`. Evidence recorded `allowCritical=true` for the CRD edit and `replicas=2` for the deployment edit. Other CRD schema edits, such as changing scope or adding an unrelated value, failed offline policy tests. |
| Trusted GitOps | The `preview-vcluster` XR carried only `incidentPolicy.allowCritical: true`, bounded deployment settings, the immutable image digest, and metadata. No raw PR CRD appeared in the trusted XR. |
| Composition | Crossplane CLI v2.5.0 rendered the vCluster Release with the `low`/`high`/`critical` CRD and a two-replica virtual Deployment. The namespace render still passed. The updated XRD passed API-server dry-run and was applied before the PR was evaluated. |
| Live preview | `deploy/local/verify-preview.sh 19 vcluster` passed for the Ready XR, exact head, route, and app context. The virtual Deployment had 2 desired, 2 ready, and 2 available replicas. The virtual API reported `250m`/`256Mi` requests and normalized limits of `1` CPU/`1Gi`. |
| CRD behavior | The virtual API exposed severity enum `low`, `high`, `critical`. It created and read an `IncidentPolicy` with `spec.severity: critical`; server dry-run rejected `urgent` with the supported enum listed. The host API had no `incidentpolicies.incidents.demo.local` CRD. |
| Closed PR | GitHub recorded closure at 12:33:54 UTC. The watcher entered `cleaning` at 12:34:36 UTC and reported `deleted`/`pr-closed`/`cleanup-verified` at 12:38:33 UTC. `deploy/local/verify-preview.sh 19 deleted` independently confirmed the Application, XR, host namespace, persistent volumes, trusted GitOps directory, and route were absent. |

The virtual API check used a temporary private kubeconfig and local port forward; both were removed. The created `critical-preview-check` object existed only inside the virtual cluster and is removed with that cluster. The Incident Tracker app does not consume IncidentPolicy objects; this check demonstrates an isolated Kubernetes API schema change and mixed workload configuration, not an application feature change.

Closure to verified deletion took about 4 minutes 39 seconds in this one local run, without a manual ApplicationSet refresh. This is a single observation, not a latency percentile.
