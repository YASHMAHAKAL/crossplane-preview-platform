# PR-driven namespace Composition preview

This phase supports one bounded Crossplane Composition source edit: change `spec.pipeline[0].input.podBudget` from `4` to `5` in `platform/crossplane/compositions/namespace.yaml`. The evaluator reads the changed YAML at the exact PR head, requires the existing Composition identity, Function reference, pipeline shape, and input fields, and rejects unknown fields or extra documents. The candidate image and Function package workflows must both succeed for that head.

The trusted host XR provisions a vCluster with the released host Function and Composition. The local installer builds the approved namespace Composition from its trusted checked-in baseline, sets only `podBudget: 5`, and applies it inside the virtual API alongside the candidate Function package and normalized child XR. The candidate Function renders a `ResourceQuota` with `pods: "5"` and the Incident Tracker app at the candidate `-c.localhost` route. The status API exposes the route after child reconciliation and health checks. No PR YAML is published to host GitOps or applied to the host API.

To inspect a live approved PR, run `deploy/local/verify-crossplane-candidate-preview.sh <PR number> --expect-pod-budget-5`. This checks the candidate package and child XR at the current head, the guest Composition input, the guest ResourceQuota, the app URL, and host API isolation. Closing the PR uses the same verified cleanup path as Go Function candidate previews.

Other namespace Composition changes, `preview-vcluster` Composition changes, XRD edits, Function dependencies, and arbitrary Kubernetes manifests remain unsupported by automatic policy. The vCluster still shares host nodes, so only configured trusted same-repository authors may run candidate code.
