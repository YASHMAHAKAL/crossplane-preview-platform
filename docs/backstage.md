# Backstage UI and Codex discovery/status surface

**Current workflow:** A developer edits supported source files and opens a normal GitHub PR, or uses a Backstage source-change template to create that PR. The Go watcher starts the preview from the diff and current-head CI artifact. The Incident Tracker catalog entry links to **PR Previews**, which accepts a PR number and shows the decision and live URL. Its **Kubernetes** tab reads the matching resources from the local kind cluster. Codex MCP exposes catalog discovery, `preview.get-status`, and Kubernetes resource discovery. The request template described in the dated verification history below has been removed from active catalog configuration; those PRs remain historical evidence.

The local [portal](../platform/backstage/portal) was generated with `@backstage/create-app@0.9.2`. Its backend loads the Incident Tracker catalog entity, MCP Actions Backend, and the [read-only preview status action](../platform/backstage/portal/plugins/preview-backend/src/index.ts). The catalog entity is read from this repository's public `main` branch, so publish edits before checking it in the portal.

The retired request template remains in the repository as a historical fixture. It previously wrote `preview.request.json` and the fixed IncidentPolicy CRD. The current evaluator ignores the request file and obtains size and TTL from trusted local config.

## Create a source PR from Backstage

Start the portal, open `http://localhost:3000/create`, and select one of these registered templates:

| Template | Source file changed | Expected evaluator decision |
| --- | --- | --- |
| **Edit Incident Tracker appearance** | `app/incident-tracker/public/appearance.json` | Namespace; the welcome text and palette change in the app |
| **Tune Incident Tracker deployment** | `deploy/incident-tracker/preview.json` | Namespace; bounded replicas and resource requests/limits change |
| **Add critical IncidentPolicy severity** | `deploy/cluster/incident-policy.json` | vCluster; the virtual API gains the `critical` severity value |

Enter a unique change ID and the template's supported inputs, then submit. The output links to the ordinary GitHub PR and to **PR Previews** for that PR. Reusing a change ID updates its template branch and PR. The evaluator reads the resulting GitHub diff and determines isolation; none of the templates has an isolation selector or writes an XR. A template may create a PR with no diff if the requested values already match `main`; change at least one value. After the PR closes or merges, the existing watcher cleans up its preview.

The three templates target this repository's `main` branch and require Backstage's configured GitHub token to have permission to create branches and PRs. The catalog reads template definitions from public `main`, so push template changes before checking them in the portal. Codex can discover their schemas through Backstage MCP and invoke the same Scaffolder action, or edit source directly and open a PR.

All three templates were exercised as [live source PRs #25–#27](backstage-source-templates-verification.md). Their exact file diffs selected namespace, namespace, and vCluster respectively; each served a healthy preview and reached verified cleanup after closure.

## Inspect the local cluster

The catalog has a **Local kind preview cluster** Resource owned by `platform-team`. The Incident Tracker Component depends on it and has a **Kubernetes** tab. Its selector matches `preview.platform.example.org/service=incident-tracker` across the configured `kind-preview-platform` cluster. Open that tab, then select the cluster card to see matching preview XRs, Deployments, Services, and Ingresses. The Kubernetes backend also returns matching quotas and provider-helm Releases. The PR number is in each preview resource's labels or name. The **PR Previews** page links to the tab when its selected preview is ready. The current app Pod template lacks the preview service label, so the cluster card shows zero matching Pods even while the Deployment is healthy. A vCluster's virtual-only CRDs stay inside its own API and are not shown as host resources.

`deploy/local/bootstrap.sh` applies [the dedicated reader RBAC](../platform/backstage/kubernetes/reader.yaml). It grants cluster-wide **get/list/watch** only for the displayed resource types; it cannot read Secrets or create/update/delete workloads. Cluster-wide reads are needed because preview namespaces are created dynamically. This local guest-auth portal shares that read access among its users, so it is not a multi-tenant authorization design.

`deploy/local/start-portal.sh` requests an eight-hour Kubernetes service account token at startup, reads the kind API URL and CA from the `kind-preview-platform` kubeconfig context, and supplies them to Backstage through environment variables. The token is never committed or written to a file. Restart the portal to renew it after eight hours. TLS verification remains enabled. If the reader is missing on an existing cluster, apply `platform/backstage/kubernetes/reader.yaml` once with the kind context or rerun bootstrap.

Codex can call Backstage MCP's `kubernetes.get-kubernetes-clusters` and `kubernetes.get-kubernetes-resources-for-entity` after discovering the Incident Tracker Component. These actions use the same Backstage Kubernetes backend and its read-only cluster credential. They complement `preview.get-status`: use status for the evaluator's decision and ready URL, and Kubernetes resources for observed workloads.

The browser tab, backend API, MCP resource action, and close cleanup were [verified with live PR #28](backstage-cluster-verification.md).

## Run locally

For a repeatable setup, use the [local setup runbook](local-setup.md) and `deploy/local/start-portal.sh`; it prepares the committed Yarn dependencies and TypeScript build, reads the existing `gh` credential, creates or reuses a private MCP token, and starts the portal in the foreground. `deploy/local/verify-platform.sh --portal` then checks the authenticated catalog entries as well as the control plane.

For a manual start, use Node 22 or 24 and a GitHub token that can create pull requests in the source repository. Keep tokens outside Git. In one terminal, start the trusted status API:

```sh
cd platform/evaluator
go run ./cmd/status-api -gitops /path/to/preview-gitops -kube-context kind-preview-platform -preview-port 8088
```

In another terminal, run the portal launcher from the repository root. It supplies the Kubernetes connection and temporary token as well as the Backstage credentials:

```sh
./deploy/local/start-portal.sh
```

Open the UI at `http://localhost:3000` so guest sign-in uses the configured browser origin. Opening it at `127.0.0.1:3000` does not use the matching origin.

The intended UI is `http://localhost:3000`, and the backend is bound to `127.0.0.1:7007`. The Incident Tracker catalog entry links to **PR Previews** and its Kubernetes tab. The portal uses a guest sign-in for this local demonstration. Its `MCP_TOKEN` is configured as a static external access token with access restricted to the MCP, catalog, Scaffolder, preview, and Kubernetes plugins. The sidebar's **PR Previews** page accepts a PR number or a PR URL. It polls the same Go status API used by the MCP status action every ten seconds and shows phase, mode, reason, evidence, source commit, expiry, live URL, and cleanup result. When ready, the displayed URL is a clickable link as well as the **Open live preview** button. The backend proxies only authenticated GET requests to the local status API at `127.0.0.1:8090`; start that API with access to the `kind-preview-platform` Kubernetes context before opening the page. It requires a current-head Ready XR and healthy route before reporting `ready`.

The MCP endpoint is `http://127.0.0.1:7007/api/mcp-actions/v1`. This repository's [project Codex config](../.codex/config.toml) points to it and reads the bearer token from `PREVIEW_MCP_TOKEN`. Set that environment variable to the **same value** as Backstage's `MCP_TOKEN` before launching a new Codex session from this repository; `codex mcp list` should show `preview_backstage` enabled. The token value is never stored in Git. The configuration prompts for write actions in interactive sessions. This local verification used the signed-in account selected by `CODEX_HOME=/home/yash/.codex-account3`; the other Codex accounts were not changed.

Codex discovers the service and template schemas with `catalog.query-catalog-entities` and `catalog.get-catalog-entity`. It can execute a source-change template through `scaffolder.execute-template`, resolve the completed task to a PR with `preview.get-request-task`, and call `preview.get-status` with that PR number. The developer may also use Codex to edit source files and open a GitHub PR using normal workspace and GitHub permissions; the watcher still decides the preview from that PR.

## Verified locally on 2026-10-07

The portal dependencies installed and its TypeScript check passed. The frontend and backend started together. The catalog loaded the Incident Tracker Component, `platform-team` Group, and request Template from the published repository. MCP `initialize` and `tools/list` exposed catalog discovery, `scaffolder.execute-template`, and `preview.get-status`. Calls to `catalog.get-catalog-entity` returned the service link and the template's five accepted inputs.

A guest user submitted the template through the actual Backstage browser UI, creating [PR #3](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/3). A separate MCP `scaffolder.execute-template` call created [PR #4](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/4). Both PRs contained only `preview.request.json`; source CI passed. The Go watcher approved both for `namespace` with `namespaced-app-change`, published the trusted GitOps records, and Argo CD and Crossplane brought up both Pulseboard instances. `/healthz` returned HTTP 200 at each local URL. `preview.get-status` returned `ready`, the decision, and the matching URL for each PR. Both PRs were closed; the watcher removed their GitOps directories, and Argo CD, Crossplane, namespaces, and ingress routes were verified gone. See [live PR evidence](live-pr-verification.md).

Those PR #3 and #4 MCP calls used a local HTTP client. A separate real Codex CLI run completed the direct client check: codex3 discovered the service and template through Backstage MCP, submitted an `app-only` request, and created [PR #5](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/5). Its source CI passed; the watcher chose `namespace`; Argo CD and Crossplane became healthy; `/healthz` returned HTTP 200. Codex queried `preview.get-status` and reported the `ready` phase, decision, reason, and local URL. After closing the PR, Codex reported `cleaning` with no URL, and the GitOps directory, Argo Application, XR, namespace, and route were verified gone. The new `preview.get-request-task` action was also exercised by codex3: it resolved the task ID to PR #5, which Codex then used for a status call without a supplied PR number. See [live PR evidence](live-pr-verification.md).

The non-interactive Codex CLI initially denied the write action because its approval policy was `never`; a one-run `--approve-for-me` invocation with `approval_policy="on-request"` allowed the already-authorized test request. The committed project config keeps its `writes` approval setting. The status API reports `cleaning` during cleanup and `deleted` after the watcher verifies the cluster resources and route are gone; a ten-minute timeout yields `cleanup-failed` with evidence.

The update path was then verified with [PR #6](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/6). Codex reused `requestId: update-phase-20261007` to change `small`/120 minutes to `medium`/90 minutes, and Backstage updated the same PR. The watcher recorded `waiting-for-ci` for the new SHA and withdrew the old XR until current-head CI passed. Argo pruned the old resources; the route briefly returned 404. The new XR used the new head, digest, size, and expiry; Crossplane served the same PR URL again with medium resource requests. Replaying the same approved head produced no extra GitOps commit or preview. Codex resolved the second task ID to PR #6 and reported its new `ready` status; close cleanup was verified. See [live PR evidence](live-pr-verification.md). A pre-watcher status query that previously returned HTTP 500 now returns `pending-evaluation` with a retry explanation.

A direct [unsupported-resource PR #7](live-pr-verification.md) verified the live rejection path. After current-head CI passed, the Go watcher returned `unsupported-cluster-resource`; Backstage and codex3 read the same reason and evidence through `preview.get-status`, with no URL or preview resources. The Backstage template offers supported inputs, so this invalid manifest was submitted directly as a test PR.

The browser UI's `incident-policy` choice then created [PR #8](live-pr-verification.md). Its Scaffolder task completed, CI passed, the Go watcher selected `vcluster`, and Argo/Crossplane brought up the fixed CRD and Incident Tracker inside the virtual API. Backstage status returned a ready local URL; the app served requests and accepted an incident create/update. Closing the PR removed the preview resources and route. That original PR used the manual installer grant. A later [synthetic vCluster run](live-pr-verification.md#automatic-vcluster-installer-grant) verified the new automatic grant and cleanup. A separate README-only [PR #9](live-pr-verification.md) verified that merging an approved namespace preview also removes its resources and URL.

The new status page and automatic grant were tested together with [PR #10](live-pr-verification.md#backstage-status-page-and-automatic-vcluster-grant). A guest user selected `incident-policy` in the browser form; the completed task linked to `/previews?prUrl=.../pull/10`. The page displayed the live `ready` vCluster result and its CRD evidence. The trusted GitOps directory supplied the installer Role and RoleBinding automatically. After the PR closed, the page's underlying status API reported `deleted`, `pr-closed`, and `cleanup-verified` with no URL. The virtual CRD and deployment were checked through the virtual API, and the host API had no IncidentPolicy CRD.

The generated dependency graph selected a published `@yarnpkg/core@4.9.2` release whose metadata referred to an unavailable publisher-local patch file. The portal pins `@yarnpkg/core` to 4.9.1, which resolved and installed successfully with the generated Yarn 4.13.0 release.

References: [Backstage MCP Actions Backend](https://backstage.io/docs/ai/mcp-actions/), [well-known actions](https://backstage.io/docs/ai/well-known-actions/), [Scaffolder templates](https://backstage.io/docs/features/software-templates/writing-templates/), [Actions Registry](https://backstage.io/docs/backend-system/core-services/actions-registry/), [Backstage Auth Service](https://backstage.io/docs/backend-system/core-services/auth/), and [OpenAI Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).
