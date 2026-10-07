# Backstage UI and Codex request surface

The local [portal](../platform/backstage/portal) was generated with `@backstage/create-app@0.9.2`. Its backend loads the Incident Tracker catalog entity, request template, GitHub Scaffolder module, MCP Actions Backend, and the [read-only preview status action](../platform/backstage/portal/plugins/preview-backend/src/index.ts). The catalog and template are read from this repository's public `main` branch, so publish edits before checking them in the portal.

The template accepts `requestId`, the source `repoUrl`, `size` (`small` or `medium`), `ttlMinutes` (5–240), and `capability` (`app-only` or `incident-policy`). It writes `preview.request.json` and, for the second capability, the fixed IncidentPolicy CRD. The Go evaluator validates the CRD digest and decides the preview mode. Reuse a request ID to update size or lifetime on its existing branch. The lifetime stays anchored to the first approval; reducing the TTL moves expiry earlier without resetting that clock. Use a new ID if the required capability changes; the template does not remove an earlier CRD file from an existing branch.

## Run locally

Use Node 22, Yarn 4.13.0 from the generated portal, and a GitHub token that can create pull requests in the source repository. Keep tokens outside Git. In one terminal, start the trusted status API:

```sh
cd platform/evaluator
go run ./cmd/status-api -gitops /path/to/preview-gitops -preview-port 8088
```

In another terminal, set `GITHUB_TOKEN` and a random `MCP_TOKEN`, then start the portal:

```sh
cd platform/backstage/portal
node .yarn/releases/yarn-4.13.0.cjs install
node .yarn/releases/yarn-4.13.0.cjs tsc
node .yarn/releases/yarn-4.13.0.cjs start
```

The intended UI is `http://localhost:3000`, and the backend is bound to `127.0.0.1:7007`. The Incident Tracker catalog entry links to the request template under Create. The portal uses a guest sign-in for this local demonstration. Its `MCP_TOKEN` is configured as a static external access token with access restricted to the MCP, catalog, Scaffolder, and preview plugins.

The MCP endpoint is `http://127.0.0.1:7007/api/mcp-actions/v1`. This repository's [project Codex config](../.codex/config.toml) points to it and reads the bearer token from `PREVIEW_MCP_TOKEN`. Set that environment variable to the **same value** as Backstage's `MCP_TOKEN` before launching a new Codex session from this repository; `codex mcp list` should show `preview_backstage` enabled. The token value is never stored in Git. The configuration prompts for write actions in interactive sessions. This local verification used the signed-in account selected by `CODEX_HOME=/home/yash/.codex-account3`; the other Codex accounts were not changed.

Codex discovers the service with `catalog.query-catalog-entities` and `catalog.get-catalog-entity`, inspects the Template inputs, and calls `scaffolder.execute-template`. That action returns a task ID. Codex polls `preview.get-request-task` until the task completes and returns the PR URL and number, then calls `preview.get-status` with that number. The UI and Codex use the same Scaffolder template and Go evaluator; neither writes an XR directly.

## Verified locally on 2026-10-07

The portal dependencies installed and its TypeScript check passed. The frontend and backend started together. The catalog loaded the Incident Tracker Component, `platform-team` Group, and request Template from the published repository. MCP `initialize` and `tools/list` exposed catalog discovery, `scaffolder.execute-template`, and `preview.get-status`. Calls to `catalog.get-catalog-entity` returned the service link and the template's five accepted inputs.

A guest user submitted the template through the actual Backstage browser UI, creating [PR #3](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/3). A separate MCP `scaffolder.execute-template` call created [PR #4](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/4). Both PRs contained only `preview.request.json`; source CI passed. The Go watcher approved both for `namespace` with `namespaced-app-change`, published the trusted GitOps records, and Argo CD and Crossplane brought up both Pulseboard instances. `/healthz` returned HTTP 200 at each local URL. `preview.get-status` returned `ready`, the decision, and the matching URL for each PR. Both PRs were closed; the watcher removed their GitOps directories, and Argo CD, Crossplane, namespaces, and ingress routes were verified gone. See [live PR evidence](live-pr-verification.md).

Those PR #3 and #4 MCP calls used a local HTTP client. A separate real Codex CLI run completed the direct client check: codex3 discovered the service and template through Backstage MCP, submitted an `app-only` request, and created [PR #5](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/5). Its source CI passed; the watcher chose `namespace`; Argo CD and Crossplane became healthy; `/healthz` returned HTTP 200. Codex queried `preview.get-status` and reported the `ready` phase, decision, reason, and local URL. After closing the PR, Codex reported `cleaning` with no URL, and the GitOps directory, Argo Application, XR, namespace, and route were verified gone. The new `preview.get-request-task` action was also exercised by codex3: it resolved the task ID to PR #5, which Codex then used for a status call without a supplied PR number. See [live PR evidence](live-pr-verification.md).

The non-interactive Codex CLI initially denied the write action because its approval policy was `never`; a one-run `--approve-for-me` invocation with `approval_policy="on-request"` allowed the already-authorized test request. The committed project config keeps its `writes` approval setting. The status API reports `cleaning` after close but does not itself confirm completed cluster deletion.

The update path was then verified with [PR #6](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/6). Codex reused `requestId: update-phase-20261007` to change `small`/120 minutes to `medium`/90 minutes, and Backstage updated the same PR. The watcher recorded `waiting-for-ci` for the new SHA and withdrew the old XR until current-head CI passed. Argo pruned the old resources; the route briefly returned 404. The new XR used the new head, digest, size, and expiry; Crossplane served the same PR URL again with medium resource requests. Replaying the same approved head produced no extra GitOps commit or preview. Codex resolved the second task ID to PR #6 and reported its new `ready` status; close cleanup was verified. See [live PR evidence](live-pr-verification.md). A pre-watcher status query that previously returned HTTP 500 now returns `pending-evaluation` with a retry explanation.

A Backstage-requested vCluster, a live rejection, and a merged PR remain unverified.

The generated dependency graph selected a published `@yarnpkg/core@4.9.2` release whose metadata referred to an unavailable publisher-local patch file. The portal pins `@yarnpkg/core` to 4.9.1, which resolved and installed successfully with the generated Yarn 4.13.0 release.

References: [Backstage MCP Actions Backend](https://backstage.io/docs/ai/mcp-actions/), [well-known actions](https://backstage.io/docs/ai/well-known-actions/), [Scaffolder templates](https://backstage.io/docs/features/software-templates/writing-templates/), [Actions Registry](https://backstage.io/docs/backend-system/core-services/actions-registry/), [Backstage Auth Service](https://backstage.io/docs/backend-system/core-services/auth/), and [OpenAI Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).
