# Crossplane PR Preview Platform

A local portfolio project that turns a trusted GitHub PR into an explained preview environment. The Go evaluator chooses a Kubernetes namespace for app-only changes, a vCluster for a fixed cluster API capability, or a rejection with a reason. Crossplane v2 renders the resources; Argo CD consumes only evaluator-authored GitOps state. Pulseboard, the Incident Tracker app, is the preview workload.

```text
Backstage UI or Codex via Backstage MCP
    -> Scaffolder opens/updates a GitHub PR
    -> GitHub Actions tests and publishes a GHCR image + head-bound artifact
    -> Go watcher validates PR, request, allowed capability, CI, capacity
    -> trusted GitOps repo -> Argo CD ApplicationSet -> Crossplane v2 XR
    -> namespace or vCluster -> Pulseboard URL after health check
    -> merged/closed PR removes XR path -> Argo CD prunes
```

## Current implementation

- Pulseboard has a responsive operations dashboard, seeded incidents, severity and status filters, service health, details, updates, notes, and a JSON API.
- The Go policy is deterministic and tested for namespace, vCluster, rejection, stale CI, invalid image, capacity, TTL expiry, and lifecycle cleanup. The GitHub reader binds the artifact to the exact PR head. The GitOps publisher accepts only evaluator-owned files and can initialize an empty trusted checkout. A watcher and status API are included.
- Crossplane v2 XRD, two Compositions, Go function, provider-helm resources, Argo CD ApplicationSet, Backstage catalog/template and a read-only status action are authored. Both Compositions passed the official CLI renderer with the local Go function and Crossplane v2.4.0 runtime.
- The custom Function runtime image and xpkg package built locally. GitHub Actions published `ghcr.io/yashmahakal/function-preview-resources:v0.1.0`; in-cluster Function health is the next integration check.
- The dedicated `kind-preview-platform` cluster has a Ready Kubernetes v1.35.0 node, Crossplane v2.4.0, and NGINX Ingress Controller. A temporary direct Pulseboard deployment is reachable at [http://pulseboard-smoke.localhost:8088](http://pulseboard-smoke.localhost:8088). **A Crossplane-managed preview, Argo CD pruning, vCluster routing, and Backstage MCP execution have not been verified yet.** See [compatibility and feasibility](docs/compatibility.md).

![Pulseboard desktop dashboard](docs/screenshots/pulseboard-dashboard.png)

[Mobile preview](docs/screenshots/pulseboard-mobile.png)

## Verify what works offline

```sh
make verify
cd platform/evaluator
go run ./cmd/evaluator -snapshot ../../tests/fixtures/namespace.json -config config.example.json -gitops /tmp/preview-gitops-test
go run ./cmd/evaluator -snapshot ../../tests/fixtures/vcluster.json -config config.example.json -gitops /tmp/preview-gitops-test
go run ./cmd/evaluator -snapshot ../../tests/fixtures/rejected.json -config config.example.json -gitops /tmp/preview-gitops-test
go run ./cmd/evaluator -snapshot ../../tests/fixtures/closed.json -config config.example.json -gitops /tmp/preview-gitops-test
```

Each decision is JSON on stdout. Approved decisions create `previews/<service>-pr-<n>/previewenvironment.json`; the closed fixture removes its XR directory. `status/<name>.json` retains the decision record.

The Composition render fixtures and local function instructions are in [tests/render/README.md](tests/render/README.md). Both render targets passed here; rendering proves the desired resource graph, while live controller behavior still needs a cluster run.

## Live app and ingress smoke check

The local cluster currently runs [a temporary Pulseboard deployment](deploy/local/pulseboard-smoke.yaml) in `pulseboard-smoke`. Its dashboard and `/healthz` both returned HTTP 200 through NGINX at `http://pulseboard-smoke.localhost:8088`. This proves the app image and host routing; the deployment is not a PR preview or a Crossplane-managed XR. To repeat the check after rebuilding the image:

```sh
docker build -t pulseboard:smoke app/incident-tracker
kind load docker-image pulseboard:smoke --name preview-platform
kubectl --context kind-preview-platform apply -f deploy/local/pulseboard-smoke.yaml
kubectl --context kind-preview-platform rollout status deployment/pulseboard -n pulseboard-smoke
curl --noproxy '*' http://pulseboard-smoke.localhost:8088/healthz
```

Remove the temporary app with `kubectl --context kind-preview-platform delete namespace pulseboard-smoke` after the Crossplane-managed preview is available.

## Local cluster setup

This path needs Docker, kind v0.31.0, kubectl, Helm, the Crossplane CLI, Go, Node 22, a public GitHub repository, and a separate trusted GitOps repository. Allow enough RAM for Crossplane, Argo CD, the ingress controller, and a vCluster. The commands below are a runbook for a fresh machine; skip `kind create cluster` when `kind-preview-platform` already exists.

This workspace has the dedicated `kind-preview-platform` context and a Ready node. Crossplane and NGINX are already installed; Argo CD is pending. The two earlier kind clusters were deleted at the operator's request; the Minikube profile was stopped and retained. The checksum-verified kind CLI used to create this cluster is at `/tmp/kind-v0.31.0-linux-amd64` and is not on this shell's PATH. Select `kind-preview-platform` explicitly when installing the remaining controllers.

1. Create the cluster and install controllers:

   ```sh
   kind create cluster --config deploy/local/kind.yaml
   helm repo add crossplane-stable https://charts.crossplane.io/stable
   helm repo update
   helm install crossplane crossplane-stable/crossplane --version 2.4.0 --namespace crossplane-system --create-namespace
   kubectl create namespace argocd
   kubectl apply -n argocd --server-side --force-conflicts -f https://raw.githubusercontent.com/argoproj/argo-cd/v3.5.2/manifests/install.yaml
   helm install nginx-ingress oci://ghcr.io/nginx/charts/nginx-ingress --version 2.7.3 --namespace nginx-ingress --create-namespace --values deploy/local/nginx-ingress-values.yaml
   ```

2. The [Function release workflow](.github/workflows/preview-function.yaml) builds and publishes the package when a `function-v<semver>` tag is pushed, for example `function-v0.1.0`. Keep the version in [functions.yaml](platform/crossplane/functions.yaml) aligned with that tag. The equivalent local build is:

   ```sh
   cd platform/crossplane/function
   docker build --platform linux/amd64 -t function-preview-runtime:v0.1.0 .
   crossplane xpkg build --package-root=package --embed-runtime-image=function-preview-runtime:v0.1.0 --package-file=/tmp/function-preview-resources.xpkg
   cd ../../..
   ```

   GHCR package visibility is managed separately from repository visibility. The Function package and Pulseboard image must each be made public for anonymous cluster pulls. If either remains private, the cluster needs pull credentials for that package; keep credentials outside Git. The release workflow uses the built-in `GITHUB_TOKEN` to publish.

3. Apply Crossplane prerequisites in order. Check that Providers and Functions are healthy before applying Compositions:

   ```sh
   kubectl apply -f platform/crossplane/rbac/composed-resources.yaml
   kubectl apply -f platform/crossplane/provider-helm.yaml
   kubectl wait --for=condition=healthy provider.pkg.crossplane.io/provider-helm --timeout=5m
   kubectl apply -f platform/crossplane/provider-helm-config.yaml
   kubectl apply -f platform/crossplane/functions.yaml
   kubectl wait --for=condition=healthy function.pkg.crossplane.io/function-preview-resources --timeout=5m
   kubectl wait --for=condition=healthy function.pkg.crossplane.io/function-auto-ready --timeout=5m
   kubectl apply -f platform/crossplane/xrd.yaml
   kubectl apply -f platform/crossplane/compositions/
   ```

4. Create a **separate** repository `YASHMAHAKAL/preview-gitops` with a `main` branch and a `previews/.gitkeep` file. Clone it into a dedicated local checkout, then run `kubectl apply -f platform/argocd/preview-appset.yaml`. The evaluator must have push access to this repository; Argo CD needs read access and repository credentials if it is private. Keep PR source branches out of Argo CD.

5. Set `repository` and `trustedAuthors` in a private copy of [config.example.json](platform/evaluator/config.example.json). The source repository must contain `preview.request.json`, the Incident Tracker code at `app/incident-tracker`, and [preview-image.yaml](.github/workflows/preview-image.yaml). GHCR images must be readable by kind, for example by making the package public. Run the Go watcher and status API:

   ```sh
   cd platform/evaluator
   GITHUB_TOKEN=<read-only-token> go run ./cmd/watcher -config /path/to/config.json -gitops /path/to/preview-gitops
   go run ./cmd/status-api -gitops /path/to/preview-gitops -preview-port 8088
   ```

   The watcher needs GitHub metadata/actions read access. The GitOps checkout uses your local Git credential helper for its push. Use separate credentials with narrow scopes where possible.

6. In an existing Backstage backend, register [the catalog entity](platform/backstage/catalog/incident-tracker.yaml) and [request template](platform/backstage/templates/request-preview/template.yaml). Install `@backstage/plugin-mcp-actions-backend`, expose catalog and scaffolder actions, and add [the preview status plugin](platform/backstage/plugin-preview-backend/src/index.ts). Set `preview.statusApiBaseUrl: http://127.0.0.1:8090` in Backstage config. See [Backstage integration](docs/backstage.md).

## Demonstration checks

- Request `app-only` in Backstage UI or via its `scaffolder.execute-template` MCP action. The Go decision should be `namespace`, and the status action should eventually return `http://incident-tracker-pr-<n>.localhost:8088` after `/healthz` succeeds.
- Request `incident-policy`; the fixed CRD file should lead to `vcluster`. Check the CRD exists **inside** the virtual cluster and is absent from the host cluster.
- Edit a disallowed cluster manifest such as a `Node`; the evaluator should report `unsupported-cluster-resource` and write no XR.
- Close or merge the PR. Check the GitOps XR directory, Argo CD Application, XR, host namespace, Helm Release, ingress, and app resources disappear. Do not treat the evaluator's `cleaning` decision alone as proof of resource deletion.

The preview hostname uses `.localhost` and host port 8088. The kind config maps host port 8088 to the ingress controller's NodePort 30080. Port 80 is occupied by another local service. If the host port changes, pass the same value to `status-api -preview-port`.
