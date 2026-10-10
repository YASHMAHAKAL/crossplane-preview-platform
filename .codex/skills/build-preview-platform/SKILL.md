---
name: build-preview-platform
description: Build and evolve this repository's local, explainable PR preview platform using Crossplane v2, Backstage, Argo CD, and vCluster. Use for implementation, architecture, integration, and verification work on this project.
---

# Build the PR preview platform

Use this skill when implementing or changing the project rooted at `crossplane-preview-platform`. It captures the product contract and boundaries between source PRs, the evaluator, GitOps, Crossplane, and preview workloads. For a narrow edit, read only the reference relevant to that edit.

## Product contract

A developer changes source files and opens or updates a GitHub PR. The platform automatically evaluates the changed files, publishes an explained decision, and serves a live preview for supported changes. The developer never selects an isolation mode or submits a separate preview request. Backstage UI and Codex through Backstage MCP discover the service and show the same PR status. A trusted **Go evaluator** chooses a namespace or vCluster from the PR's actual needs. Argo CD syncs only evaluator-authored GitOps state. Crossplane v2 reconciles one `PreviewEnvironment` cluster-scoped XR through one of two Compositions, both calling a custom Go Composition Function. Merging or closing a PR removes the preview. Use the Incident Tracker app as the demonstration service.

Current migration stage: Incident Tracker app changes and bounded `deploy/incident-tracker/preview.json` replica/resource changes automatically get namespace previews. A PR editing the `deploy/cluster/incident-policy.json` baseline may add only `critical` to the severity enum; the normalized schema choice selects a vCluster, including when mixed with app and bounded deployment changes. Other CRD edits, external service types, other deployment files, and Crossplane source changes fail closed. An exact-head Crossplane candidate workflow builds the Function package, renders both Compositions, and records an immutable digest. The local candidate gate verified PR #20 at head `22af02243b1c158a8f659160288349a9119a7eb2`: a separate Crossplane 2.4 control plane installed the candidate Function, XRD, and Compositions, reconciled and deleted a namespace XR, and left the host Function unchanged. The vCluster Composition was rendered in CI but has not been live-reconciled by the candidate gate. Backstage templates may later create ordinary source changes, but must not offer a preview-mode selector.

The target is a local `kind` environment. A merged PR does not trigger permanent staging or production deployment. Keep the platform runnable without paid cloud infrastructure or a paid vCluster tier; external GitHub/GHCR use may still require network access and an account. The default demonstration accepts only trusted PRs from repositories and authors configured by the local operator. vCluster shared nodes are not an isolation boundary for untrusted code.

## Essential implementation rules

1. Inspect the current repository and user request before changing files. Honor later user decisions over this baseline, and update the reference that owns any changed contract.
2. Treat the source PR diff as the preview trigger for both IDE and future Backstage-created changes. Codex discovers catalog entities and PR status through Backstage actions; it does not bypass the evaluator to create an XR or mutate the host cluster.
3. Keep policy decisions outside the Composition Function. Implement the evaluator in Go: it decides `namespace`, `vcluster`, or `rejected` with evidence. The function renders deterministic resources for the selected Composition; it does not inspect GitHub or apply policy to raw PR data.
4. Never sync unreviewed PR manifests directly to the host cluster. Verify the PR source and CI artifact, normalize the allowed inputs, write a validated XR to a separate trusted GitOps path, and let Argo CD apply that path.
5. Use Crossplane v2 semantics deliberately: a modern cluster-scoped XRD (`scope: Cluster`) can compose namespaced resources. Keep the XRD, Compositions, function input, evaluator output, and fixtures aligned. Prefer pinned compatible releases; check current upstream documentation before relying on version-specific fields.
6. Make both preview modes serve the Incident Tracker and surface a reachable local URL. In vCluster mode, explicitly solve how the workload is deployed **inside** the virtual cluster and how its ingress reaches the host; a vCluster object alone is not a complete preview.
7. Design deletion as a first-class flow. A closed or merged PR removes the evaluator's GitOps directory; Argo CD prunes the XR; Crossplane and provider finalizers remove composed resources. Verify the namespace, vCluster release, routing, and secrets are gone. Treat failures as visible states, not silent success.
8. Keep costs, permissions, and reproducibility visible. Do not commit credentials, request broad host privileges, or claim measured performance without data. Keep a deterministic offline policy test path and a local end-to-end path.
9. Treat the Incident Tracker as a portfolio-quality product surface. Give it a coherent operations theme, responsive layout, clear status and severity hierarchy, useful filters and detail views, and accessible interaction states. Keep its features tied to the preview demonstration.

## Where to read next

- [Product and workflow](references/product-workflow.md): read for user journeys, Backstage UI and Codex parity, Incident Tracker behavior, and acceptance criteria.
- [Architecture and contracts](references/architecture-contracts.md): read when changing APIs, repository layout, evaluator rules, composition boundaries, GitOps, status, or cleanup.
- [Build sequence](references/build-sequence.md): read when planning implementation, choosing an order, or crossing subsystem boundaries. Complete the feasibility gates before building around uncertain integrations.
- [Verification](references/verification.md): read when adding fixtures, tests, local runs, metrics, or the final demo.
- [Upstream references](references/upstream.md): read when implementing a version-sensitive Backstage, Crossplane, Argo CD, or vCluster integration. Follow primary documentation and record the versions actually chosen in project files.

## How to work

Work in phases. The app-diff and bounded deployment-config namespace paths are verified. A real bounded IncidentPolicy schema edit mixed with deployment settings was verified in a live vCluster PR. Exact-head candidate builds, both Composition renders, and namespace XR reconciliation inside a disposable Crossplane control plane are verified. Next add content-aware evaluation and trusted GitOps for supported Crossplane source changes, including live verification of the vCluster Composition where needed. Then add Backstage source-change templates and cluster visibility. Make each slice reviewable: implementation, focused verification, and a short note about what works and what remains. Keep names and schemas stable once external components consume them.

The revised project is done when supported app, deployment, and Crossplane source changes trigger appropriate previews directly from PRs; Backstage can create ordinary source changes and show the local cluster; both isolation paths, rejection, preview update, URL and reason reporting, and merge/close cleanup work in a documented local demonstration; and render and policy checks pass.
