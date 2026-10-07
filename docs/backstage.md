# Backstage UI and Codex request surface

The local [portal](../platform/backstage/portal) was generated with `@backstage/create-app@0.9.2`. Its backend loads the Incident Tracker catalog entity, request template, GitHub Scaffolder module, MCP Actions Backend, and the [read-only preview status action](../platform/backstage/portal/plugins/preview-backend/src/index.ts). The catalog and template are read from this repository's public `main` branch, so publish edits before checking them in the portal.

The template accepts `requestId`, the source `repoUrl`, `size` (`small` or `medium`), `ttlMinutes` (5–240), and `capability` (`app-only` or `incident-policy`). It writes `preview.request.json` and, for the second capability, the fixed IncidentPolicy CRD. The Go evaluator validates the CRD digest and decides the preview mode. Reuse a request ID to update size or lifetime on its existing branch. Use a new ID if the required capability changes; the template does not remove an earlier CRD file from an existing branch.

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

The MCP endpoint is `http://127.0.0.1:7007/api/mcp-actions/v1`. Configure a Codex MCP client to send `Authorization: Bearer <MCP_TOKEN>` to that URL. Discover `catalog.query-catalog-entities` and `catalog.get-catalog-entity`, inspect the `Template` entity's accepted inputs, call `scaffolder.execute-template`, then call `preview.get-status` with the returned PR number. The UI and Codex use the same Scaffolder template and therefore reach the same Go evaluator; neither writes an XR directly.

## Verified locally on 2026-10-07

The portal dependencies installed and its TypeScript check passed. The frontend and backend started together. The catalog loaded the Incident Tracker Component, `platform-team` Group, and request Template from the published repository. MCP `initialize` and `tools/list` exposed catalog discovery, `scaffolder.execute-template`, and `preview.get-status`. Calls to `catalog.get-catalog-entity` returned the service link and the template's five accepted inputs.

A guest user submitted the template through the actual Backstage browser UI, creating [PR #3](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/3). A separate MCP `scaffolder.execute-template` call created [PR #4](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/4). Both PRs contained only `preview.request.json`; source CI passed. The Go watcher approved both for `namespace` with `namespaced-app-change`, published the trusted GitOps records, and Argo CD and Crossplane brought up both Pulseboard instances. `/healthz` returned HTTP 200 at each local URL. `preview.get-status` returned `ready`, the decision, and the matching URL for each PR. Both PRs were closed; the watcher removed their GitOps directories, and Argo CD, Crossplane, namespaces, and ingress routes were verified gone. See [live PR evidence](live-pr-verification.md).

The MCP calls used a local HTTP MCP client, not a configured Codex session. A direct Codex client connection, template PR update/replay, a Backstage requested vCluster, a live rejection, and a merged PR are still unverified. The status API reports `cleaning` after close but does not itself confirm completed cluster deletion.

The generated dependency graph selected a published `@yarnpkg/core@4.9.2` release whose metadata referred to an unavailable publisher-local patch file. The portal pins `@yarnpkg/core` to 4.9.1, which resolved and installed successfully with the generated Yarn 4.13.0 release.

References: [Backstage MCP Actions Backend](https://backstage.io/docs/ai/mcp-actions/), [well-known actions](https://backstage.io/docs/ai/well-known-actions/), [Scaffolder templates](https://backstage.io/docs/features/software-templates/writing-templates/), and [Actions Registry](https://backstage.io/docs/backend-system/core-services/actions-registry/).
