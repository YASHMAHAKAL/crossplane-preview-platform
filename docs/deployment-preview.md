# Deployment changes in PR previews

Edit [`deploy/incident-tracker/preview.json`](../deploy/incident-tracker/preview.json) on a branch and open a normal source PR. A change to this file automatically selects a vCluster preview; the developer does not choose the preview mode. The Go evaluator reads the file from the exact PR head, validates every field, and writes only the normalized settings into the trusted `PreviewEnvironment` XR. The Crossplane Function puts them on the Incident Tracker Deployment **inside** the vCluster.

The current contract supports:

| Setting | Accepted values |
| --- | --- |
| `spec.replicas` | `1`, `2` |
| `spec.resources.requests.cpu` | `100m`, `250m` |
| `spec.resources.requests.memory` | `128Mi`, `256Mi` |
| `spec.resources.limits.cpu` | `500m`, `1000m` |
| `spec.resources.limits.memory` | `512Mi`, `1024Mi` |

All fields are required. The evaluator rejects unknown keys, extra JSON documents, deleted config, values outside these bounds, and unrelated deployment files. For example, a PR can change `replicas` from `1` to `2` and increase requests to `250m`/`256Mi`. The current file retains a single replica and the original resource settings as its baseline. The Function validates the XR values again before rendering. Service type changes and arbitrary Kubernetes manifests are not yet supported.

The `Preview image` CI run tests the checked-in config with the same Go parser and publishes an immutable image artifact for the PR head. The watcher waits for that artifact, records `vcluster` with reason `deployment-stack-change` and changed-file evidence, then Argo CD and Crossplane create the virtual cluster and app route. A PR that changes only Incident Tracker app files still uses a namespace. A PR containing both app and this deployment config uses a vCluster.

To check a live PR, enter its number on Backstage **PR Previews** or ask Codex for `preview.get-status`, then run:

```sh
deploy/local/verify-preview.sh <PR number> vcluster
```

Inspect the virtual Deployment through the vCluster kubeconfig Secret and a temporary API port forward; [vCluster documents the default `vc-NAME` Secret and localhost:8443 endpoint](https://www.vcluster.com/docs/vcluster/manage/accessing-vcluster). Confirm the replica and resource values in the virtual API, along with the live app route. Close the PR and run `deploy/local/verify-preview.sh <PR number> deleted` after status reports `cleanup-verified`.

Pulseboard currently writes incidents to an `emptyDir` in each app pod. Two replicas demonstrate deployment behavior and serve the UI, but their incident records are independent. A shared data store is needed before treating two replicas as consistent application storage.
