# Architecture and contracts

This file owns subsystem boundaries and suggested contracts. The current entry point is a source PR diff; the old request-template wording in historical details below must not be used to restore mode selection or `preview.request.json` parsing. Exact API group, package versions, and field spellings should be checked against pinned upstream versions.

## Control flow

```text
Developer IDE or future Backstage source-edit template ─> GitHub PR
                                                     │
                                                     ▼
                                         CI image + PR file metadata
                                                     │
                                                     ▼
                                      Evaluator: validate + decide
                                                     │
                             approved: trusted GitOps XR / rejected: status
                                                     │
                                                     ▼
                              Argo CD ApplicationSet Git generator
                                                     │
                                                     ▼
                          Crossplane PreviewEnvironment cluster XR
                                   ├─ namespace Composition
                                   └─ vCluster Composition
                                                     │
                                                     ▼
                             health, decision, URL, cleanup status
                                      └─> Backstage UI + Codex MCP
```

The source repository and the trusted GitOps repository are distinct trust domains. The evaluator is the only component allowed to author preview XRs. Argo CD watches evaluator output, never arbitrary PR branches. Backstage currently displays state; future edit templates may create ordinary source PRs, but remain outside reconciliation.

## Suggested repository areas

Use this as a navigational target, not a demand to create empty folders at once:

```text
app/incident-tracker/        usable app and deployment package
platform/backstage/          portal, template, catalog, preview actions
platform/evaluator/          GitHub watcher, policy, GitOps writer, status API
platform/crossplane/         XRD, Compositions, function source/package, fixtures
platform/argocd/             AppProject/ApplicationSet and sync settings
platform/vcluster/           pinned OSS chart values and integration notes
deploy/local/                kind, ingress, bootstrap, local registry if used
tests/                       policy, render, integration fixtures
docs/                        setup, architecture, runbook, demo, measurements
```

Prefer one command or Make target for local bootstrap and another for teardown after the underlying pieces are independently inspectable. Avoid scripts that hide setup errors or erase unrelated clusters.

## `PreviewEnvironment` API

Define one modern cluster-scoped XRD (for example `preview.platform.example.org/v1alpha1`, kind `PreviewEnvironment`). Keep its schema narrow and structural. The evaluator writes one XR per service/PR. Suggested `spec` fields:

| Field | Purpose |
| --- | --- |
| `serviceRef` | Stable Backstage/service identifier or repository coordinates from the allowlist. |
| `pr.number`, `pr.headSHA` | Traceability and stale-build protection. |
| `image.digest` | Immutable artifact that passed required checks; reject mutable `latest`. |
| `request.size`, `request.ttl` | Bounded defaults from trusted local operator config, never PR-controlled. |
| `preview.host` | Normalized, collision-free local hostname assigned by the evaluator. |
| `decision.mode`, `decision.reasonCodes` | Selected `namespace` or `vcluster` and machine-readable explanation. |
| `compositionRef` or supported v2 selector | Explicitly chooses the matching Composition; confirm exact supported field in the pinned release. |

Do not place arbitrary Helm values, Kubernetes YAML, service account permissions, GitHub credentials, or secret bytes in the request or XR. Derive resource names from stable service and PR identifiers with DNS-safe normalization and collision checks. Include labels/annotations for service, PR, head SHA, managed-by, and decision revision on composed resources.

Use XR conditions plus a status aggregator for observed state. Avoid writing evaluator-owned decision fields back and forth with Crossplane-owned conditions. The status API should distinguish policy rejection, CI wait/failure, reconciliation failure, app health failure, and cleanup failure. Store enough data to explain each decision after the GitOps path has been deleted.

## Evaluator policy

Policy input is a typed, testable snapshot: source repository/author trust, PR number/head SHA, changed files or declared requirements, preview contract, required check results, image digest, configured limits, and current preview inventory. Unknown or malformed data fails closed with a reason code.

Implement the evaluator as a Go service and CLI sharing one typed policy package. Its decision is deterministic for the same snapshot and policy version. Suggested precedence:

1. Reject untrusted repository/author, unsupported fork PR, missing or stale required CI, missing immutable image, invalid contract, disallowed secret/privilege request, or exhausted quota.
2. Choose vCluster when the allowed change requires cluster-scoped APIs, CRDs, RBAC, or an operator in the preview environment. Record the evidence, such as changed file/contract capability and policy rule.
3. Choose namespace for allowed namespaced app resources and code-only changes.
4. Reject requirements the platform cannot safely or functionally render. Do not silently fall back to namespace when vCluster prerequisites fail.

Changed-file paths alone are weak evidence: inspect the supported deployment/contract content or use a typed declaration whose consistency is checked. Do not generalize to exactly two hard-coded PR examples. Define extensible rule categories and table-driven cases; the two demonstration PRs exercise categories, not special-case branches.

The evaluator writes idempotently to `previews/<service>-pr-<number>/` (or an equivalent unique path). Include a small metadata record with the decision and source SHA if useful, but keep the Argo CD path free of untrusted raw files. Guard concurrent updates and retries with head-SHA checks. On close/merge, remove the path and retain an audit/status record; a repeated close should be harmless.

## Crossplane Compositions and function

Use two `mode: Pipeline` Compositions matching the same XRD, with explicit selection after evaluation. Both call a custom Go Composition Function package. The function accepts a small versioned input selecting the resource pattern and fixed platform defaults; it reads the observed XR, validates invariants, emits desired composed resources with stable composition resource names, and returns useful errors/conditions. The function does not call GitHub, create PRs, or decide isolation mode.

Namespace mode should own a namespace, ResourceQuota/LimitRange or equivalent bounded defaults, app Deployment/Service/Ingress, and any needed ConfigMap/Secret references. vCluster mode should own a host namespace and an OSS vCluster Helm Release through provider-helm or another justified supported installation path; it also needs a supported deployment mechanism **inside** the virtual cluster plus host ingress sync/routing. Pin the vCluster chart and image to the OSS build. Prototype the workload and routing path before treating the mode as complete. If the chart's `experimental.deploy.vcluster.helm` mechanism proves unsuitable, use a small documented alternative with narrow virtual-cluster credentials; preserve GitOps ownership and cleanup.

Crossplane v2 can compose ordinary Kubernetes resources, but package RBAC must permit the specific kinds the function emits. Manage Crossplane/provider permissions deliberately. The pinned vCluster chart needs a fixed installer Role and RoleBinding in its host preview namespace. The trusted Go evaluator writes these fixed namespaced grants beside an approved vCluster XR; Argo CD applies them after Crossplane creates the namespace, with bounded sync retries. The AppProject permits only preview namespace destinations, the XR, and these two RBAC kinds. The grant's `Delete=false` annotation keeps it available while provider-helm uninstalls the Release; deletion of the Crossplane-owned namespace then removes it. Provider-helm uses a stable ServiceAccount from a DeploymentRuntimeConfig. Do not grant Crossplane cluster-wide Role/RoleBinding creation or privilege escalation to automate this step. For resources created inside vCluster, document which controller owns and deletes them. A rendered resource list alone does not prove the vCluster app is running.

## Argo CD and deletion

An ApplicationSet Git directory generator watches only the evaluator's `previews/*` directories. Configure an AppProject with the needed destination and resource allowlists. Set sync, pruning, and ApplicationSet deletion behavior so removing a directory deletes its Application and its XR. Verify the relevant finalizers and propagation with the pinned Argo CD version. The watcher first publishes GitOps removal, then checks the Argo Application, XR, host namespace (which owns the Helm Release, ingress, PVC, and secrets), persistent volumes referencing that namespace, and HTTP route. It reports `deleted` only after all are absent and a 30-second settle period has passed. If anything remains or an observation fails for ten minutes, report `cleanup-failed` with evidence and keep retrying. Preserve `pr-merged` separately from `pr-closed` in the status reason.

TTL expiry uses the same verified deletion path while the PR remains open. Preserve the first approval time and expiry across watcher restarts; an expired saved preview must be removed before re-evaluating a changed head or CI result. The status API must stop presenting its URL at the deadline even if the watcher is unavailable, and show cleanup pending until reconciliation resumes. Run the local watcher and status API under supervised user services with a persistent trusted GitOps checkout and a heartbeat outside Git. A failed PR read must not prevent publishing cleanup for other PRs. These services depend on the user's systemd manager and cluster remaining available; a stale heartbeat or failed reconciliation must be visible.

Use local ingress hostnames such as `<service>-pr-<n>.localhost` only after confirming host resolution for the chosen OS. Document port mapping, TLS choice, and DNS/hosts setup. Surface a URL only after the XR reports current-generation `Synced=True` and `Ready=True` for the same PR head **and** app health succeeds. Keep host-level exposure scoped to trusted demo traffic.

## Backstage action boundary

Register the service in Catalog with owner, source link, API entity where useful, and an explicit preview capability. The Scaffolder template has a JSON Schema input contract and outputs links to the created repository/PR. For Codex, expose stock catalog and scaffolder actions through Backstage MCP Actions Backend, plus a custom `preview.list-capabilities`, `preview.request`, or `preview.get-status` action only where stock actions do not supply a needed capability. Make custom action input/output schemas explicit. Both UI and MCP call the same backend logic and status store. Scope MCP exposure and authenticate the client; a static local token may be acceptable for the demo if stored outside Git.
