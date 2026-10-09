# IncidentPolicy schema preview

The source baseline is [`deploy/cluster/incident-policy.json`](../deploy/cluster/incident-policy.json). Open a normal PR that adds `"critical"` after `"high"` in `spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.severity.enum`. This is a real CRD schema edit: the resulting virtual API accepts `IncidentPolicy` objects with `spec.severity: critical`.

The Go evaluator fetches this file at the exact PR head and accepts only the baseline or that one enum extension. It compares every other canonical field to the operator's trusted baseline hash, then writes only `spec.incidentPolicy.allowCritical` to the trusted `PreviewEnvironment` XR. The Crossplane Function independently checks that the setting accompanies the `incident-policy` capability in vCluster mode and builds the CRD inside the virtual cluster. The source manifest itself is never applied to the host or copied into trusted GitOps. Other CRD schema edits, arbitrary manifests, and Crossplane source edits are rejected with a reason.

The same PR may change the bounded [Incident Tracker deployment settings](deployment-preview.md). The CRD edit takes precedence, so both the changed CRD and the validated app settings are previewed in a vCluster. An app or deployment PR without the CRD edit uses a namespace.

To verify a live PR, run `deploy/local/verify-preview.sh <PR number> vcluster`, inspect the virtual API with the [temporary kubeconfig procedure](deployment-preview.md), and check:

```sh
kubectl --kubeconfig="/tmp/incident-tracker-pr-<PR number>-kubeconfig" get crd incidentpolicies.incidents.demo.local -o json
kubectl --context kind-preview-platform get crd incidentpolicies.incidents.demo.local --ignore-not-found -o name
```

The first command should show `low`, `high`, and `critical` in the severity enum; the host command should return no CRD. A virtual `IncidentPolicy` with `spec.severity: critical` should be admitted. Close the PR and run `deploy/local/verify-preview.sh <PR number> deleted` after cleanup completes.
