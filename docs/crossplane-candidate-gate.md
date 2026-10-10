# Crossplane source candidate gate

The source PR still triggers the platform automatically. This gate prepares the next policy expansion: it builds and tests a candidate Function without installing PR code in the host Crossplane control plane. The evaluator currently rejects Crossplane source edits, so a candidate CI result alone does not create a PR preview.

## Supported candidate source paths

A push to a non-`main` branch containing changes under `platform/crossplane/function/`, to `platform/crossplane/xrd.yaml`, or under `platform/crossplane/compositions/` starts the `Preview Crossplane candidate` workflow. The workflow checks out the push SHA, tests the Go Function, builds its runtime and xpkg, and publishes a unique semantic-version candidate tag to the existing public Function package in GHCR. It resolves the package's immutable digest, renders both Compositions with that digest, and uploads `crossplane-candidate-ci.json` only after those checks pass. The record contains the exact `headSHA`, digest reference, tag, and workflow run ID. The short-lived candidate tag is a lookup aid; the gate installs only the digest.

The workflow runs on same-repository pushes, not fork PR code. Its `GITHUB_TOKEN` has repository-scoped package write permission; no personal access token is stored in Git. The package includes executable Function code, so install candidates only for trusted authors.

## Run the isolated candidate check

With the local `kind-preview-platform` control plane healthy, Docker available, and `gh auth login` able to read this repository's Actions artifacts, open a same-repository PR from a trusted author. After its `Preview Crossplane candidate` run succeeds, run:

```sh
cd platform/evaluator
go run ./cmd/candidate-gate -pr <PR number>
```

The Go reader checks that the PR is open, same-repository, and authored by an allowlisted user. It selects a successful `push` run for the **current** PR head, checks the artifact's SHA, run ID, tag, and digest repository, and verifies that the tag still resolves to the recorded digest. It clones the PR branch and compares its checked-out commit with the recorded head SHA. The trusted local gate then installs the digest and the candidate XRD and Compositions inside a disposable vCluster, applies the trusted namespace XR fixture, checks composed resources and their deletion, and checks that the host Function and API did not receive the fixture. It rechecks the PR and artifact after the run so a head update cannot be reported as a current-head success.

The local gate retains trusted RBAC and XR fixture files from this checkout. It tests whether candidate code satisfies the current namespace contract. The fixture's fake app image means `Synced=True` and resource creation are checked; workload readiness and a preview URL are not. The vCluster Composition is rendered in CI but is not live-reconciled by this gate. A separate policy and GitOps phase is required before Crossplane source edits can produce automatic PR previews.

## Verified run

The disposable same-repository [PR #20](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/20) exercised this path at head `22af02243b1c158a8f659160288349a9119a7eb2`. [Workflow run 38050006712](https://github.com/YASHMAHAKAL/crossplane-preview-platform/actions/runs/38050006712) succeeded and published `ghcr.io/yashmahakal/function-preview-resources@sha256:7521585e7bbe77f8cf27231cd5374a44a671518e522f362496bf2a4163f1777f`. `go run ./cmd/candidate-gate -pr 20` passed: the candidate package installed in the disposable vCluster, the namespace XR reconciled and deleted its composed resources, and the host Function remained unchanged. The gate removed the temporary vCluster and `crossplane-candidate-spike` namespace before reporting success. This PR changed a Function comment to exercise the build path; it did not test a semantic Function change.
