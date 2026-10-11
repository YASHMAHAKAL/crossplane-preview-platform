# PR-driven Crossplane Function preview

## Supported workflow

A trusted same-repository PR changes one or more direct Go files under `platform/crossplane/function/`. The Go evaluator fetches each changed file at the PR's exact head, parses it, and waits for both the `Preview image` and `Preview Crossplane candidate` push workflows for that head. It rejects changed Function Dockerfiles, dependencies, XRD or Composition files, and mixed IncidentPolicy CRD edits. The candidate workflow publishes an immutable Function package digest and renders both Compositions. A successful candidate CI record alone does not authorize raw PR YAML in the host cluster.

After both artifacts pass, policy version 6 selects `vcluster` with reason `crossplane-function-change`. The evaluator writes a normalized host `PreviewEnvironment` XR and fixed installer grant to the trusted GitOps checkout. The host's released Function creates the vCluster; its package is never replaced. The local watcher then installs Crossplane 2.4.0, the candidate Function digest, and the trusted baseline XRD and Compositions **inside** that virtual API. It applies a normalized child namespace XR there with the exact PR head and app image digest. The candidate Function renders the child app and its ingress at `<service>-pr-<number>-c.localhost`. The status API exposes that URL only after the host XR, candidate child XR, and HTTP health check pass. A missing or unhealthy candidate route causes the watcher to retry the virtual install.

On PR close or merge, the evaluator removes the host XR from trusted GitOps. Argo CD prunes it; Crossplane removes the vCluster Release and host namespace. The watcher checks the Application, XR, namespace, persistent volumes, and both the baseline and candidate HTTP routes before recording `deleted`.

## Live semantic test: PR #21

The disposable [PR #21](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/21) changed `resources.go` to add `preview.platform.example.org/candidate-head` to a composed Namespace. This is a visible Function behavior change, not a comment-only package rebuild.

| Check | Observed result |
| --- | --- |
| Exact source head | `9c43976bd0007e5df3b3f939b0287ac1d7639f96` |
| Image CI | [Run 38051197913](https://github.com/YASHMAHAKAL/crossplane-preview-platform/actions/runs/38051197913) succeeded |
| Candidate CI | [Run 38051197954](https://github.com/YASHMAHAKAL/crossplane-preview-platform/actions/runs/38051197954) succeeded |
| Candidate package | `ghcr.io/yashmahakal/function-preview-resources@sha256:fcb4fc4b75d7a4350f0d33b987d2a0b716e80299396a3bc00f4e68c7ff542f82` |
| Decision | `approved`, `vcluster`, `crossplane-function-change`, with the changed Go path as evidence |
| Host Function | Remained `ghcr.io/yashmahakal/function-preview-resources:v0.1.5` |
| Candidate guest Function | Its package and current identifier matched the candidate digest |
| Candidate child XR | `incident-tracker-pr-21-c`, `Synced=True` and `Ready=True` at the exact head |
| Semantic effect | The virtual Namespace's `candidate-head` annotation equalled the PR head; that child Namespace did not exist in the host API |
| Preview URL | Status API reported `ready` at `http://incident-tracker-pr-21-c.localhost:8088`; `/healthz` passed |
| Close cleanup | Host Namespace and persistent volumes absent; both routes returned HTTP 404; status recovered to `deleted`, `pr-closed`, `cleanup-verified` |

The test used `deploy/local/verify-crossplane-candidate-preview.sh 21 --expect-smoke-annotation` while the PR was open. The optional annotation check is specific to this smoke change. The general command omits that option.

The first virtual install failed because the machine's Snap Helm launcher cannot run under the watcher service's `NoNewPrivileges=yes`. The trusted installer now invokes the mounted static Helm binary when Snap is the available Helm command; the corrected retry installed the guest control plane. Cleanup briefly reported `cleanup-failed` after an internal HTTP probe timed out, then recovered. Internal probes now connect to loopback while preserving the preview hostname in the Host header. The final status and direct route checks confirmed deletion.

## Current limits

Automatic source previews cover Go Function files only. Candidate XRD and Composition edits still have a manual isolated gate and CI render, but no automatic live preview. The trusted host vCluster Composition also deploys its released baseline app at the ordinary PR hostname; the status API deliberately exposes the candidate-rendered app at the `-c` hostname. The candidate Function executes in a vCluster with shared host nodes, so the default trust boundary remains the configured same-repository author allowlist. A fresh-machine bootstrap was not repeated during this verification; it used the existing local kind installation.
