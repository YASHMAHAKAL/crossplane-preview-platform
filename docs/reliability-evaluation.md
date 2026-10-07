# Preview reliability evaluation — 2026-10-07

## Method and environment

The [Go observer](../platform/evaluator/cmd/preview-eval/main.go) polls the same `Store.Status` logic as the Backstage status API. Start it **before** the watcher first approves a PR for `-target ready`, or before closing the PR for `-target deleted`. It records each phase transition, the first observed target time, the evaluator's first approval or cleanup-start timestamp, the source head, and the poll interval. A target already present on the first poll is marked `late-start` and receives no latency. The measured duration is an **upper bound within the five-second polling resolution**, plus request time. The cleanup duration starts when the watcher records `cleaning`; GitHub's `closedAt` is reported separately.

The local cluster was one `kind-preview-platform` Kubernetes v1.35.0 node with allocatable 12 CPUs and 15,583,516 KiB memory. Crossplane v2.4.0, Argo CD v3.5.2, provider-helm v1.2.0, vCluster OSS chart 0.36.0, and NGINX ingress were installed. The ApplicationSet generator requeues every 180 seconds. No manual ApplicationSet refresh was used for PR #11. These are individual local samples, **not** p50 or p95 estimates; pod memory peaks were not collected.

## Real PR results

| Trial | Source and CI | Observed result | Cleanup |
| --- | --- | --- | --- |
| [PR #11](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/11), namespace | Head `00aee93b5b5e615ccbcb9fad0d87ae7dd9b86d97`; `Preview image` passed at `11:45:06Z` | Evaluator approved at `11:45:15.816Z`; route-ready status first observed at `11:45:47.703Z`: **31.887 s**. Argo was Synced/Healthy and `/healthz` returned 200. This sample used the original route-only readiness rule. | GitHub closed at `11:46:16Z`; watcher started cleanup at `11:46:49.910Z`; verified `deleted` first observed at `11:52:33.320Z`: **343.409 s** from watcher cleanup start, **377.320 s** from GitHub close. Argo Application, XR, namespace, persistent volumes, and route were absent. |
| [PR #12](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/12), vCluster | Head `afb759242ba571bdd0b47e1589cdcefc3ff5d23b`; `Preview image` passed at `11:53:40Z` | Evaluator approved at `11:54:15.636Z`; automatic installer Role existed, Argo synced, and the route returned 200. At one check, the status API said `ready` while the XR still said `Ready=False`; it later became Ready. The observer's `/tmp` output was lost across a session pause, so **no valid vCluster latency** is claimed. | The watcher and status API were stopped across the pause. The preview still existed after its `13:54:15Z` expiry. PR closed at `16:49:32Z`; the restored watcher removed its GitOps directory and ultimately published `deleted`/`cleanup-verified`. Cleanup used a manual ApplicationSet refresh and is excluded from the normal-polling timing comparison. |

The two successful PR #11 observer JSON outputs were captured before the `/tmp` reset and transcribed unchanged into [readiness](evaluation/samples/pr11-ready.json) and [deletion](evaluation/samples/pr11-deleted.json). The observer now writes directly to a chosen output path so later runs can retain their raw files.

A later observer run against already-deleted PR #12 returned `outcome: late-start` with no `durationMs`, confirming that it does not manufacture a cleanup latency from a terminal status record.

## Failures found and changes made

1. **Premature ready status.** A vCluster route returned HTTP 200 before the XR had `Ready=True`. The status API and observer now require the XR's current-generation `Synced=True` and `Ready=True` for the same PR head, followed by `/healthz` HTTP 200. Missing Kubernetes access is reported as `degraded` with evidence. Unit tests cover false Ready, stale generation, stale head, and an unhealthy route. The corrected rule passed local tests, but a new live timing sample could not be collected because GitHub writes failed later in the session.
2. **Repeated cleanup commits.** During PR #11 cleanup, every watcher pass rewrote `cleaning` without its evidence and then wrote the evidence back, causing two unnecessary GitOps commits. The evaluator now preserves an unchanged cleaning record. A regression test checks byte-for-byte status stability on replay. PR #11's normal Argo cleanup took longer than one 180-second generator interval: the generator saw a cached preview directory at `11:48:35Z` and removed it at `11:51:35Z`.
3. **Watcher availability controls TTL.** PR #12 remained active past its requested expiry while the local watcher process was stopped. TTL is enforced only when the watcher reconciles; it is not a Kubernetes-enforced deadline. A supervised watcher or independent expiry controller is needed before claiming unattended lifecycle reliability.
4. **External GitHub write failures.** Three SSH pushes to the source repository returned HTTP 500. A Contents API update also returned HTTP 500; a synthetic XR publish to the separate trusted GitOps repository returned HTTP 500. The empty temporary source branch was removed, the unpushed synthetic GitOps commit was reset to `origin/main`, and no synthetic XR reached the cluster. Read APIs still worked. No corrected-rule live timing is claimed from these failed attempts.

Deterministic Go checks also cover failed current-head CI (`rejected`/`ci-failed`), stale CI (`waiting-for-ci`), unsupported cluster resources, a route stuck at HTTP 503 (no ready timing), delayed cleanup (`cleanup-failed` after ten minutes), and recovery once resources disappear. These are **offline failure tests**, not claims of a live injected CI or controller outage.

## Repeat the measurement

Start the status API and watcher from [the local runbook](../README.md). In a separate terminal, before watcher approval, run:

```sh
cd platform/evaluator
go run ./cmd/preview-eval -gitops /path/to/preview-gitops \
  -kube-context kind-preview-platform -name incident-tracker-pr-<n> \
  -target ready -expect-mode vcluster -interval 5s -timeout 20m \
  -out ../../docs/evaluation/samples/pr<n>-ready.json
```

For cleanup, start a second observer with `-target deleted` and an output filename ending in `-deleted.json` **before** closing the PR. Record GitHub's `closedAt` separately. A finished sample should say `outcome: observed`; `late-start`, `timeout`, `wrong-mode`, and `terminal-phase` do not support a latency claim. Keep the watcher running through deletion and confirm no preview XR or open test PR remains.
