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

## Verification boundary

The portal dependencies installed and its TypeScript check passed. The backend started with the `preview`, `catalog`, `scaffolder`, and `mcp-actions` plugins. The request template appeared in the catalog. MCP `initialize` returned the configured server description and instructions; `tools/list` exposed `catalog.get-catalog-entity`, `catalog.query-catalog-entities`, `scaffolder.execute-template`, and `preview.get-status`. The live Scaffolder action schema confirmed that `publish:github:pull-request` accepts the template's `update: true` input.

The Incident Tracker component initially failed catalog validation because its link was relative; its link is now an absolute portal URL and needs a fresh runtime check after publication. The `platform-team` Group location also needs a fresh runtime check after publication. The status action has been listed but not invoked through MCP. The frontend UI, a Codex client connection, and a Backstage-created PR have not been exercised yet. These are the next phase's checks.

The generated dependency graph selected a published `@yarnpkg/core@4.9.2` release whose metadata referred to an unavailable publisher-local patch file. The portal pins `@yarnpkg/core` to 4.9.1, which resolved and installed successfully with the generated Yarn 4.13.0 release.

References: [Backstage MCP Actions Backend](https://backstage.io/docs/ai/mcp-actions/), [well-known actions](https://backstage.io/docs/ai/well-known-actions/), [Scaffolder templates](https://backstage.io/docs/features/software-templates/writing-templates/), and [Actions Registry](https://backstage.io/docs/backend-system/core-services/actions-registry/).
