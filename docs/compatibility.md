# Version and feasibility record

| Component | Chosen release | Evidence / status |
| --- | --- | --- |
| Crossplane | 2.4.0 | Helm release deployed in `kind-preview-platform`; core and RBAC manager pods are Ready. No live XR reconciliation yet. |
| Crossplane Go function SDK | 0.7.1 | Go unit tests and both official Composition renders pass. Runtime Docker image and `/tmp/function-preview-resources.xpkg` built locally. The `function-v0.1.0` GitHub Actions release published the package to GHCR; in-cluster Function health remains unverified. |
| Crossplane CLI | 2.5.0 | Verified published checksum, rendered both fixtures against v2.4.0 engine. |
| provider-helm | 1.2.0 | `Release` and in-cluster ProviderConfig authored; live reconcile unverified. |
| vCluster OSS chart | 0.36.0 | Rendered `Release` includes app manifests and host ingress sync; installation and URL unverified. |
| Argo CD | 3.5.2 | ApplicationSet Git directory generator authored; pruning unverified. |
| NGINX Ingress Controller OSS | chart 2.7.3 | Controller pod Ready; service uses NodePort 30080 and host port 8088. Pulseboard dashboard and `/healthz` returned HTTP 200 through its ingress. |
| Backstage | Choose one release when portal is created | Catalog, template, and status action sources authored; no running portal yet. |
| kind | 0.31.0 CLI; Kubernetes 1.35.0 node | Created `preview-platform` from the pinned config; `kind-preview-platform` control-plane is Ready. Host port 8088 maps to node port 30080. Crossplane and ingress are installed. |

The local environment has `kubectl`, Helm, and an accessible Docker daemon. At the operator's request, the previous `agent-guard` and `incidentpilot` kind clusters and their resources were deleted. Minikube was stopped but its profile and data were preserved. The new `preview-platform` cluster was created with a checksum-verified kind v0.31.0 CLI from `/tmp`. The official Crossplane renderer was run with a checksum-verified v2.5.0 CLI, the v2.4.0 render engine, and the local Go function. The direct app smoke test proves local routing; it does not prove Crossplane-managed namespace or vCluster previews, ApplicationSet pruning, or Backstage MCP parity.

The source repository is `YASHMAHAKAL/crossplane-preview-platform`; the separate trusted GitOps repository has not been created. The custom Function package is published at `ghcr.io/yashmahakal/function-preview-resources:v0.1.0`. The Function package and Pulseboard image currently require pull credentials while their GHCR packages are private; making the repository public alone does not change package visibility. A plain HTTP local registry is not configured: Crossplane's package manager fetches OCI packages itself, so a [kind containerd mirror](https://kind.sigs.k8s.io/docs/user/local-registry/) alone would not solve package retrieval; see the [Crossplane registry discussion](https://github.com/crossplane/crossplane/issues/6159).

References: [Crossplane v2](https://docs.crossplane.io/latest/composition/composite-resources/), [Crossplane composition RBAC](https://docs.crossplane.io/latest/composition/compositions/), [Argo CD ApplicationSet Git generator](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Git/), [NGINX OSS Helm install](https://docs.nginx.com/nginx-ingress-controller/install/helm/open-source/).
