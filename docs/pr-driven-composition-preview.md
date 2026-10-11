# PR-driven namespace Composition preview

This phase supports one bounded Crossplane Composition source edit: change `spec.pipeline[0].input.podBudget` from `4` to `5` in `platform/crossplane/compositions/namespace.yaml`. The evaluator reads the changed YAML at the exact PR head, requires the existing Composition identity, Function reference, pipeline shape, and input fields, and rejects unknown fields or extra documents. The candidate image and Function package workflows must both succeed for that head.

The trusted host XR provisions a vCluster with the released host Function and Composition. The local installer builds the approved namespace Composition from its trusted checked-in baseline, sets only `podBudget: 5`, and applies it inside the virtual API alongside the candidate Function package and normalized child XR. The candidate Function renders a `ResourceQuota` with `pods: "5"` and the Incident Tracker app at the candidate `-c.localhost` route. The status API exposes the route after child reconciliation and health checks. No PR YAML is published to host GitOps or applied to the host API.

To inspect a live approved PR, run `deploy/local/verify-crossplane-candidate-preview.sh <PR number> --expect-pod-budget-5`. This checks the candidate package and child XR at the current head, the guest Composition input, the guest ResourceQuota, the app URL, and host API isolation. Closing the PR uses the same verified cleanup path as Go Function candidate previews.

Other namespace Composition changes, `preview-vcluster` Composition changes, XRD edits, Function dependencies, and arbitrary Kubernetes manifests remain unsupported by automatic policy. The vCluster still shares host nodes, so only configured trusted same-repository authors may run candidate code.

## Live semantic test: PR #22

The disposable [PR #22](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/22) changed the Composition input to `podBudget: 5` at head `b6d7078b9e51b77ce698978a78f80fcfdd32209e`. The [candidate workflow](https://github.com/YASHMAHAKAL/crossplane-preview-platform/actions/runs/38103310883) and [image workflow](https://github.com/YASHMAHAKAL/crossplane-preview-platform/actions/runs/38103310890) passed at that head. The candidate package was `ghcr.io/yashmahakal/function-preview-resources@sha256:a0323fe6efc065d68c1cb53d19e41b8c00ff0ebd8ad69412bfb6d627562dc8c1`.

The evaluator recorded `approved/vcluster/crossplane-composition-change` with `podBudget=5` evidence. Argo CD synced the trusted host XR. Guest Crossplane installed the candidate Function and Composition; the child XR became Ready and Synced, and the status API reported the healthy URL `http://incident-tracker-pr-22-c.localhost:8088`. `deploy/local/verify-crossplane-candidate-preview.sh 22 --expect-pod-budget-5` passed: the virtual Composition input and the composed ResourceQuota both had a five pod budget, the child namespace was absent from the host API, and the host Composition had no candidate budget.

After the PR closed, Argo CD pruned its Application and the host XR. Crossplane removed the vCluster namespace and persistent volume; both baseline and candidate routes returned HTTP 404. The status API reached `deleted` with `pr-closed` and `cleanup-verified` at `2026-10-11T02:05:14Z`.

The first image workflow on this test PR failed because a policy test fixture assumed the checked-out Composition always contained the baseline value. The fixture was corrected on main; both workflows passed on the final head. This test reused the existing local kind cluster rather than bootstrapping a fresh machine.
