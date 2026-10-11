# Product and workflow

A source PR is the only preview trigger. A developer changes app or platform files in an IDE, opens a PR, and receives a preview decision automatically. Backstage and Codex expose service discovery and the same status; neither asks the developer to choose namespace or vCluster. The local operator config supplies bounded size and TTL defaults. Backstage templates can edit supported source files and open a normal PR, which then follows the same path.

## Current implemented slice

1. A trusted, same-repository PR changes `app/incident-tracker/` files. GitHub Actions builds the app and publishes an image digest artifact for that exact head SHA.
2. The Go watcher reads PR metadata, the changed-file list, and the current-head CI artifact. It selects `namespace` with `namespaced-app-change` evidence, writes a normalized XR to the private trusted GitOps repository, and records status.
3. Argo CD syncs the evaluator-owned path. Crossplane renders a namespace, bounded app resources, and ingress. The status API reports the local URL only after current-head XR readiness and app health.
4. Backstage PR Previews and Codex `preview.get-status` show phase, reason, evidence, URL, and cleanup. Closing or merging the PR removes the GitOps path and verifies resource and route deletion.

A changed `deploy/incident-tracker/preview.json` selects `namespace` after strict content validation. The Function applies its bounded replicas and resource settings to the Incident Tracker Deployment in that namespace. A PR that changes the source baseline `deploy/cluster/incident-policy.json` can add only `critical` to the severity enum. The evaluator normalizes that choice to `incidentPolicy.allowCritical` and selects `vcluster`, including when mixed with app or bounded deployment changes; the Function reconstructs the CRD inside the virtual API. Direct Go Function source edits, the namespace Composition pod budget change from 4 to 5, and the XRD `decision.reasonCodes.minItems: 1` change select a vCluster after exact-head candidate CI. Other CRD edits, external service types, other deployment files, unsupported Crossplane source changes, and workflow edits are rejected. Documentation-only and legacy request-file-only PRs are skipped. A mixed app and unsupported platform PR is rejected. No UI choice can override these decisions.

## Next phases

- Keep the Backstage source-change templates within evaluator policy. PRs #25–#27 verified that their ordinary source diffs select namespace, namespace, and vCluster previews, then clean up after closure.
- Show the local kind cluster and its preview resources in Backstage with scoped credentials. Keep service catalog and preview status useful from Codex MCP.

## Acceptance

For the current slice, demonstrate a real app source edit becoming a namespace preview, visible app change, same-PR update, and verified deletion after closure. Demonstrate skip and reject decisions without host resource creation. Do not describe a deployment or Crossplane PR as supported until its corresponding evaluation and live run have passed.
