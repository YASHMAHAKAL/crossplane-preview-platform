# Build sequence and decision gates

## Current migration order

1. Verified: real Incident Tracker app file changes from trusted source PRs select namespace automatically; `preview.request.json` has no authority.
2. Verified: the bounded `deploy/incident-tracker/preview.json` diff selects namespace, with live replicas/resources, rollout capacity, and close cleanup.
3. Verified: a real IncidentPolicy severity enum extension selected vCluster; PR #19 proved the changed CRD and mixed deployment settings inside its virtual API, host isolation, and close cleanup.
4. Verified: PR #20 built and published an exact-head candidate Function package; CI rendered both Compositions with its digest, and the disposable vCluster gate installed candidate source, reconciled and deleted a namespace XR, and left the host Function unchanged. Next add content-aware evaluation and trusted GitOps before Crossplane source edits can trigger automatic previews. Live-reconcile the vCluster Composition where policy requires it. Reject host-impacting edits that cannot be safely tested.
5. Add Backstage templates that propose real source file changes and open ordinary PRs, then show local cluster resources with scoped access. Keep catalog/status available to Codex MCP.

The original sequence below describes how the initial request-template prototype was built. It is historical and must not reintroduce manual preview-mode selection.

Use the sequence to reduce integration risk. It is an implementation guide, not a reason to stop after a mock or a single demo branch. Build the smallest complete vertical slice before expanding features.

## 0. Pin versions and prove uncertain integrations

Create a short compatibility matrix covering Kubernetes/kind, Crossplane v2, Crossplane CLI and Go SDK, Argo CD, Backstage, provider-helm, and vCluster OSS. Pin actual images/charts/packages in code and document why each is compatible. Use primary documentation in [upstream.md](upstream.md).

Run focused feasibility spikes with observable outcomes:

- Render a cluster-scoped v2 XR into at least one namespaced Kubernetes resource using a locally built Go Composition Function.
- Install an OSS vCluster in `kind`; deploy the Incident Tracker container inside it; make its ingress reachable on the host; delete it cleanly.
- Verify the chosen Backstage MCP actions expose catalog discovery, schema details, and a controlled request/status action to a Codex client.
- Verify ApplicationSet directory removal prunes a minimal XR in the pinned Argo CD release.

Record any unsupported setting and chosen replacement. Do not continue building a full integration around an unverified experimental field.

## 1. Build the Incident Tracker and service template

Implement the Incident Tracker's polished dashboard, incident queue, filters, details/activity, create flow, status changes, and health. Give it a coherent responsive visual theme and seed data. Make its image reproducible, with a deployment package that accepts the image digest, host, and bounded resource settings. Add CI that tests/builds and publishes or records an immutable artifact for the PR head. The local-only path may use a registry attached to `kind`; the GitHub PR path must prove how the cluster retrieves its image.

Build the Backstage template that generates this service repository, registers `catalog-info.yaml`, and publishes the preview contract. Ensure the UI presents accepted fields and output links. Seed one reference Incident Tracker service so evaluation can begin even before all scaffolding polish is finished.

## 2. Make the policy decision runnable without Kubernetes

Implement the evaluator in Go with typed inputs and outputs. Start with table-driven fixtures for: code-only app change → namespace; allowed CRD/cluster capability → vCluster; unsupported privileged request → rejected; malformed contract → rejected; stale/missing CI artifact → wait/reject; quota exceeded → rejected; same PR head repeated → same decision. Include reason codes, evidence, policy version, and source SHA. Keep tests offline and deterministic.

Then connect the evaluator to GitHub PR metadata and required check/artifact data. Prefer a GitHub App or narrowly scoped token. Polling is acceptable for a local demo; webhook delivery is optional if it simplifies latency. Support PR opened, synchronized, merged, and closed. Protect against duplicate events, stale head updates, and concurrent PRs.

## 3. Provision the namespace preview end to end

Implement XRD, namespace Composition, custom Go function, function package, provider permissions, and render fixtures. Add Argo CD ApplicationSet and a trusted GitOps writer. Get a real code PR through evaluator → GitOps → Argo CD → Crossplane → running Incident Tracker → local URL. Verify status propagation and deletion before adding vCluster complexity.

## 4. Provision the vCluster preview end to end

Add the second Composition and the allowed cluster-scoped demonstration change. Provision a distinct host namespace and OSS vCluster; install the app and CRD capability in the virtual cluster; prove the app URL is reachable and that the CRD is isolated from the host cluster. Inspect owner references/finalizers and verify cleanup removes virtual cluster, host namespace, ingress, and any temporary credentials. Keep resource requests small enough for the documented local machine profile.

## 5. Complete Backstage UI and Codex parity

Expose catalog capability discovery, request creation, and status lookup through actions with typed schemas. Use Backstage's MCP Actions Backend for Codex; make authentication and action filtering explicit. Check that asking Codex “What can I request for Incident Tracker?” returns service details and accepted inputs, then that requesting a preview produces the same PR and status that a UI request would. Do not claim MCP parity based on a direct HTTP call that bypasses Backstage actions.

## 6. Reliability, documentation, and presentation

Add bounded retries, stale PR reconciliation, status refresh, TTL expiry if included in the contract, and cleanup failure visibility. Capture measurements from actual runs: decision latency, PR-to-URL p50/p95 over a stated sample, namespace/vCluster resource footprint, cleanup time, and failure counts. Document setup, required tools/accounts, local URL resolution, demo steps, teardown, and known limits. Include an architecture diagram and a short explanation of the evaluator's novelty: policy-driven isolation with evidence, one request surface across UI/Codex, and GitOps/Crossplane lifecycle control.

## Scope controls

The core release has one reference app and two isolation choices, yet policy should be reusable for other compositions and service contracts. Avoid introducing cloud accounts, production deployment, multi-tenant billing, a general CI platform, or an autonomous infrastructure agent just to make the demo look larger. A real, complete local workflow carries more value than many incomplete integrations.
