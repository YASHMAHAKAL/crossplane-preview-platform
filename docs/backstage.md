# Backstage UI and Codex discovery/status surface

**Current workflow (Phase 1):** A developer edits Incident Tracker app files and opens a normal GitHub PR. The Go watcher starts the preview from the diff and current-head CI artifact. The Incident Tracker catalog entry links to **PR Previews**, which accepts a PR number and shows the decision and live URL. Codex MCP currently exposes catalog discovery and `preview.get-status`. The request template described in the dated verification history below has been removed from active catalog configuration; those PRs remain historical evidence. Backstage source-change templates and cluster visibility are later phases.

The local [portal](../platform/backstage/portal) was generated with `@backstage/create-app@0.9.2`. Its backend loads the Incident Tracker catalog entity, MCP Actions Backend, and the [read-only preview status action](../platform/backstage/portal/plugins/preview-backend/src/index.ts). The catalog entity is read from this repository's public `main` branch, so publish edits before checking it in the portal.

The retired request template remains in the repository as a historical fixture. It previously wrote `preview.request.json` and the fixed IncidentPolicy CRD. The current evaluator ignores the request file and obtains size and TTL from trusted local config.

## Run locally

For a repeatable setup, use the [local setup runbook](local-setup.md) and `deploy/local/start-portal.sh`; it prepares the committed Yarn dependencies and TypeScript build, reads the existing `gh` credential, creates or reuses a private MCP token, and starts the portal in the foreground. `deploy/local/verify-platform.sh --portal` then checks the authenticated catalog entries as well as the control plane.

For a manual start, use Node 22 or 24, Yarn 4.13.0 from the generated portal, and a GitHub token that can create pull requests in the source repository. Keep tokens outside Git. In one terminal, start the trusted status API:

```sh
cd platform/evaluator
go run ./cmd/status-api -gitops /path/to/preview-gitops -kube-context kind-preview-platform -preview-port 8088
```

In another terminal, set `GITHUB_TOKEN` and a random `MCP_TOKEN`, then start the portal:

```sh
cd platform/backstage/portal
node .yarn/releases/yarn-4.13.0.cjs install
node .yarn/releases/yarn-4.13.0.cjs tsc
node .yarn/releases/yarn-4.13.0.cjs start
```

The intended UI is `http://localhost:3000`, and the backend is bound to `127.0.0.1:7007`. The Incident Tracker catalog entry links to **PR Previews**. The portal uses a guest sign-in for this local demonstration. Its `MCP_TOKEN` is configured as a static external access token with access restricted to the MCP, catalog, Scaffolder, and preview plugins. The sidebar's **PR Previews** page accepts a PR number or a PR URL. It polls the same Go status API used by the MCP status action every ten seconds and shows phase, mode, reason, evidence, source commit, expiry, live URL, and cleanup result. When ready, the displayed URL is a clickable link as well as the **Open live preview** button. The backend proxies only authenticated GET requests to the local status API at `127.0.0.1:8090`; start that API with access to the `kind-preview-platform` Kubernetes context before opening the page. It requires a current-head Ready XR and healthy route before reporting `ready`.

The MCP endpoint is `http://127.0.0.1:7007/api/mcp-actions/v1`. This repository's [project Codex config](../.codex/config.toml) points to it and reads the bearer token from `PREVIEW_MCP_TOKEN`. Set that environment variable to the **same value** as Backstage's `MCP_TOKEN` before launching a new Codex session from this repository; `codex mcp list` should show `preview_backstage` enabled. The token value is never stored in Git. The configuration prompts for write actions in interactive sessions. This local verification used the signed-in account selected by `CODEX_HOME=/home/yash/.codex-account3`; the other Codex accounts were not changed.

Codex discovers the service with `catalog.query-catalog-entities` and `catalog.get-catalog-entity`, then calls `preview.get-status` with a source PR number. Its current MCP allowlist exposes only these read actions. The developer may use Codex to edit source files and open a GitHub PR using normal workspace and GitHub permissions; the watcher still decides the preview from that PR.

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
