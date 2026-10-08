# Product and workflow

A source PR is the only preview trigger. A developer changes app or platform files in an IDE, opens a PR, and receives a preview decision automatically. Backstage and Codex expose service discovery and the same status; neither asks the developer to choose namespace or vCluster. The local operator config supplies bounded size and TTL defaults. A future Backstage template may edit supported source files and open a normal PR, which then follows the same path.

## Current implemented slice

1. A trusted, same-repository PR changes `app/incident-tracker/` files. GitHub Actions builds the app and publishes an image digest artifact for that exact head SHA.
2. The Go watcher reads PR metadata, the changed-file list, and the current-head CI artifact. It selects `namespace` with `namespaced-app-change` evidence, writes a normalized XR to the private trusted GitOps repository, and records status.
3. Argo CD syncs the evaluator-owned path. Crossplane renders a namespace, bounded app resources, and ingress. The status API reports the local URL only after current-head XR readiness and app health.
4. Backstage PR Previews and Codex `preview.get-status` show phase, reason, evidence, URL, and cleanup. Closing or merging the PR removes the GitOps path and verifies resource and route deletion.

A changed `deploy/incident-tracker/preview.json` selects `vcluster` after strict content validation. The Function applies its bounded replicas and resource settings to the Incident Tracker Deployment inside the virtual cluster. A direct PR with the exact allowlisted `deploy/cluster/incident-policy.json` CRD can also select `vcluster`. This is a narrow transitional demonstration. Other deployment, Crossplane, workflow, or source changes are rejected. Documentation-only and legacy request-file-only PRs are skipped. A mixed app and unsupported platform PR is rejected. No UI choice can override these decisions.

## Next phases

- Extend content-aware evaluation beyond the bounded Incident Tracker config to other supported deployment and Crossplane changes, with a tested virtual cluster path and explicit rejection for credentials, cluster privileges, and host controller mutation. A candidate Function source edit needs its own exact-head build and test artifact before it can affect a preview.
- Add Backstage templates that propose actual app or supported platform file edits as a source PR. Their outputs are ordinary PRs; the watcher remains the sole decision maker.
- Show the local kind cluster and its preview resources in Backstage with scoped credentials. Keep service catalog and preview status useful from Codex MCP.

## Acceptance

For the current slice, demonstrate a real app source edit becoming a namespace preview, visible app change, same-PR update, and verified deletion after closure. Demonstrate skip and reject decisions without host resource creation. Do not describe a deployment or Crossplane PR as supported until its corresponding evaluation and live run have passed.
