# Version and feasibility record

| Component | Chosen release | Observed result |
| --- | --- | --- |
| Crossplane | 2.4.0 | Healthy in `kind-preview-platform`. Namespace and vCluster XRs reconciled during live synthetic runs. |
| Crossplane Go function SDK | 0.7.1 | Unit tests and both official Composition renders passed. `ghcr.io/yashmahakal/function-preview-resources:v0.1.2` is healthy in-cluster. |
| Crossplane CLI | 2.5.0 | Checksum-verified CLI rendered both fixtures against the v2.4.0 engine. |
| provider-helm | 1.2.0 | Healthy. A vCluster `Release` reached Ready and was deleted on PR-close simulation. The provider uses `crossplane-system:preview-provider-helm`; its installer Role is granted only in each approved vCluster namespace. |
| vCluster OSS chart | 0.36.0 | The pinned OSS image ran in a host namespace. Pulseboard returned HTTP 200 through host ingress, and the fixed IncidentPolicy CRD existed only in the virtual API. |
| Argo CD | 3.5.2 | Private GitOps repository directory generation created Applications; removing the directory pruned Applications and XRs in both modes. |
| NGINX Ingress Controller OSS | chart 2.7.3 | NodePort 30080 is mapped to host port 8088. Both preview URLs returned HTTP 200. |
| Backstage | Not yet pinned | Catalog, template, and status action sources are authored. No running portal or MCP action path has been verified. |
| kind | 0.31.0 CLI; Kubernetes 1.35.0 node | Dedicated `preview-platform` cluster; `kind-preview-platform` context. |

The source repository `YASHMAHAKAL/crossplane-preview-platform` and its GHCR app and Function packages are public. The evaluator-owned `YASHMAHAKAL/preview-gitops` repository is private. Argo CD has read-only repository access through a deploy key; the key is not committed. The local evaluator checkout uses the operator's Git credentials to publish trusted output. Synthetic downstream runs used PR numbers 4243 and 4244. Later, [real draft PRs #1 and #2](live-pr-verification.md) proved GitHub artifact verification, watcher decisions, GitOps publishing, preview readiness, and close cleanup in both modes.

For the vCluster run, the operator applied [the fixed namespaced installer Role](../deploy/local/vcluster-helm-installer.yaml) after Crossplane created the preview namespace. The provider could install and delete the chart in that namespace, while an authorization check denied it access in `default`. This per-preview bootstrap remains manual. Crossplane was not granted cluster-wide Role/RoleBinding creation or privilege escalation. A fully automated vCluster request path must preserve that boundary and still needs design and verification.

Both real PR close paths removed their GitOps preview directory, Application, XR, host namespace, and route. The vCluster run also removed its Helm Release and persistent volume; its namespaced Role, PVC, and temporary secrets disappeared with the namespace. The temporary direct `pulseboard-smoke` deployment was also removed. The fixed CRD was absent from the host API during the vCluster run. Backstage UI, Codex via Backstage MCP, live update/replay, and a merged PR are still unverified. The status API currently reports `cleaning` without confirming final deletion.

The local environment has `kubectl`, Helm, and Docker. The earlier `agent-guard` and `incidentpilot` kind clusters were deleted at the operator's request; Minikube was stopped and retained. The official Crossplane CLI v2.4.1 Linux amd64 download did not match its published SHA256 in this environment, so it was not executed; the v2.5.0 bundle checksum matched and was used.

References: [Crossplane v2](https://docs.crossplane.io/latest/composition/composite-resources/), [Crossplane composition RBAC](https://docs.crossplane.io/latest/composition/compositions/), [Argo CD ApplicationSet Git generator](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Git/), [NGINX OSS Helm install](https://docs.nginx.com/nginx-ingress-controller/install/helm/open-source/).
