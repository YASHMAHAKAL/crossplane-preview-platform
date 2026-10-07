# Live GitHub PR verification — 2026-10-07

Two draft PRs in the public [source repository](https://github.com/YASHMAHAKAL/crossplane-preview-platform) exercised the real GitHub → CI artifact → Go watcher → private trusted GitOps repository → Argo CD → Crossplane path. Both PRs were closed after verification. The local `kind-preview-platform` cluster had no preview XR, Helm Release, preview namespace, or persistent volume after cleanup.

| PR | Source head | Decision | Live result | Close result |
| --- | --- | --- | --- | --- |
| [#1](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/1) | `755a441d3716c7cf240f706ea12e15cab7326c77` | `namespace`, `namespaced-app-change` | XR Ready; Argo Application Synced/Healthy; Pulseboard and `/healthz` HTTP 200; HTML included the PR-only welcome copy; status API returned `ready` and `http://incident-tracker-pr-1.localhost:8088`. | Watcher removed the trusted XR directory; Argo pruned its Application; Crossplane removed the XR and namespace; URL returned HTTP 404. |
| [#2](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/2) | `96486b99c9d9ef5fca4a5bd5324741facc9a5274` | `vcluster`, `cluster-api-required` | Scoped installer Role applied after namespace creation; Helm Release and XR Ready; Pulseboard and `/healthz` HTTP 200; Deployment, Service, Ingress, and fixed IncidentPolicy CRD present in virtual API; CRD absent from host API. | Watcher removed the trusted XR directory; Argo pruned its Application; Crossplane/provider-helm removed the Release, XR, host namespace, and persistent volume; URL returned HTTP 404. |

The watcher accepted only a successful `Preview image` push run and an artifact matching the exact PR head. The published image digests were:

- PR #1: `ghcr.io/yashmahakal/crossplane-preview-platform@sha256:3b1d59c977e39280dc4f64ed8a8b94991f54678e3d81c258547a26d305b0a825`
- PR #2: `ghcr.io/yashmahakal/crossplane-preview-platform@sha256:954060366fb407bd5b254b736831cbc99871c07f1479203869a41821f0d51c88`

The ApplicationSet Git directory generator uses a 180 second requeue interval. In these runs, a close could wait nearly one full interval before Argo began pruning. This is an observed latency source, not a measured percentile. The sample has two PRs; no p50/p95 latency claim is made.

PR #2 still required the explicit [operator bootstrap](../deploy/local/bootstrap-vcluster-helm-installer.sh) for a fixed namespaced Role and RoleBinding. The script checked the approved `vcluster` XR and its Crossplane-managed namespace before applying the grant. The provider did not receive host-wide installer permissions. GitHub artifact metadata was publicly readable, but anonymous download of an existing artifact returned HTTP 401; the watcher needs a token with access to Actions artifacts even though the source repository and GHCR packages are public.

The status API reported `cleaning` with no URL after PR #1 was closed. It does not yet confirm that Argo and Crossplane finished deletion, so an operator must inspect those controllers for a final cleanup result. Backstage UI and Codex through Backstage MCP were not exercised in these runs. PR update/replay, rejection through the live watcher, and a merged PR remain separate verification scenarios.
