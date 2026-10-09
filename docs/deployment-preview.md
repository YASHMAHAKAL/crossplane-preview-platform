# Deployment changes in PR previews

Edit [`deploy/incident-tracker/preview.json`](../deploy/incident-tracker/preview.json) on a branch and open a normal source PR. A validated change to this file selects a namespace preview; the developer does not choose the preview mode. The Go evaluator reads the file from the exact PR head, validates every field, and writes only the normalized settings into the trusted `PreviewEnvironment` XR. The Crossplane Function puts them on the Incident Tracker Deployment in that preview namespace.

The current contract supports:

| Setting | Accepted values |
| --- | --- |
| `spec.replicas` | `1`, `2` |
| `spec.resources.requests.cpu` | `100m`, `250m` |
| `spec.resources.requests.memory` | `128Mi`, `256Mi` |
| `spec.resources.limits.cpu` | `500m`, `1000m` |
| `spec.resources.limits.memory` | `512Mi`, `1024Mi` |

All fields are required. The evaluator rejects unknown keys, extra JSON documents, deleted config, values outside these bounds, and unrelated deployment files. For example, a PR can change `replicas` from `1` to `2` and increase requests to `250m`/`256Mi`. The current file retains a single replica and the original resource settings as its baseline. The Function validates the XR values again before rendering. Service type changes and arbitrary Kubernetes manifests are not yet supported.

The `Preview image` CI run tests the checked-in config with the same Go parser and publishes an immutable image artifact for the PR head. The watcher waits for that artifact, records `namespace` with reason `namespaced-deployment-change` and changed-file evidence, then Argo CD and Crossplane create the preview namespace and app route. A PR containing both app and bounded deployment changes stays in a namespace. If the PR also makes the supported [IncidentPolicy schema edit](incident-policy-preview.md), the evaluator selects vCluster and applies the deployment settings inside it. Unsupported service types, arbitrary Kubernetes manifests, and Crossplane source edits are rejected. The rendered Service uses the Kubernetes default `ClusterIP` type; this contract does not accept a service-type override.

For a two-replica namespace preview, the small ResourceQuota permits a third pod during a rolling update at the maximum accepted request values. The medium quota already has enough room. These limits bound the generated preview, not the host's total capacity; the local cluster must still have schedulable CPU and memory.

To check a live PR, enter its number on Backstage **PR Previews** or ask Codex for `preview.get-status`, then run:

```sh
deploy/local/verify-preview.sh <PR number> namespace
```

Inspect the namespaced Deployment directly:

```sh
name=incident-tracker-pr-<PR number>
kubectl --context kind-preview-platform -n "$name" get deployment incident-tracker -o json
kubectl --context kind-preview-platform -n "$name" get pods
```

If the same PR also changes the supported CRD, the preview uses a vCluster. For the pinned chart, its host namespace contains `vc-incident-tracker-pr-N` and control-plane pod `incident-tracker-pr-N-0`; [vCluster documents the Secret and localhost:8443 access pattern](https://www.vcluster.com/docs/vcluster/manage/accessing-vcluster). Port-forward the pod in one terminal:

```sh
name=incident-tracker-pr-<PR number>
kubectl --context kind-preview-platform -n "$name" port-forward "pod/$name-0" 8443:8443
```

In another terminal, retrieve the temporary kubeconfig with private file permissions and inspect the virtual Deployment:

```sh
name=incident-tracker-pr-<PR number>
umask 077
kubectl --context kind-preview-platform -n "$name" get secret "vc-$name" -o jsonpath='{.data.config}' | base64 --decode > "/tmp/$name-kubeconfig"
kubectl --kubeconfig="/tmp/$name-kubeconfig" -n default get deployment incident-tracker -o json
rm -f "/tmp/$name-kubeconfig"
```

Confirm `spec.replicas`, `status.readyReplicas`, and container resource requests/limits. Kubernetes may normalize `1000m` to `1` CPU and `1024Mi` to `1Gi`. Close the PR and run `deploy/local/verify-preview.sh <PR number> deleted` after status reports `cleanup-verified`.

Pulseboard currently writes incidents to an `emptyDir` in each app pod. Two replicas demonstrate deployment behavior and serve the UI, but their incident records are independent. A shared data store is needed before treating two replicas as consistent application storage.
