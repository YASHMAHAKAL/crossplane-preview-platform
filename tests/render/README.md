# Crossplane Composition render checks

`namespace-xr.json` and `vcluster-xr.json` are cluster-scoped v2 XR fixtures. `functions.yaml` marks the custom function as a local development runtime. The two Make targets use the actual Compositions, the v2.4.0 render engine, and the XRD schema. The custom function reports readiness from observed Namespace, Deployment, Service, Ingress, ResourceQuota, and vCluster Release state.

Run from a machine with Docker and a Crossplane CLI:

```sh
cd platform/crossplane/function
go build -buildvcs=false -o /tmp/function-preview-resources .
/tmp/function-preview-resources --insecure
```

In another terminal at the repository root:

```sh
make render-namespace
make render-vcluster
```

Observed here with Crossplane CLI v2.5.0 and Crossplane v2.4.0:

- Namespace render: XR references a Namespace, ResourceQuota, Deployment, Service, and Ingress. The Deployment carries the immutable fixture image, `PREVIEW_MODE=namespace`, and two replicas with bounded resource requests/limits. The quota has room for a rolling-update surge pod; the Ingress host is `incident-tracker-pr-42.localhost`.
- vCluster render: XR references a host Namespace and provider-helm Release. The Release pins vCluster chart 0.36.0, enables ingress sync, and includes the normalized IncidentPolicy CRD with `critical` plus Incident Tracker Deployment, Service, and Ingress as virtual-cluster manifests. The app has `PREVIEW_MODE=vcluster`; the mixed CRD/deployment fixture renders two replicas with bounded resources inside the virtual cluster.
- Both XRs had `Synced=True` and `Ready=False` in the renderer because no real resources existed; live readiness and reachability require the cluster check.

The official CLI v2.4.1 Linux amd64 download did not match its published SHA256 (`ad0661...` published versus `963ae0...` downloaded), including the checksum inside its official bundle. We did not execute it. The v2.5.0 bundle checksum matched and was used for the checks above.
