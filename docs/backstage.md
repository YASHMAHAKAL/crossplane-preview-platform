# Backstage UI and Codex request surface

The [Incident Tracker catalog entity](../platform/backstage/catalog/incident-tracker.yaml) is the discovery entry. The [Scaffolder template](../platform/backstage/templates/request-preview/template.yaml) publishes its accepted inputs to the Backstage UI and can be executed through the same Scaffolder backend by Codex. Both paths open or update the same kind of GitHub PR; neither can write an XR directly.

The template accepts `requestId`, the source `repoUrl`, `size` (`small` or `medium`), `ttlMinutes` (5–240), and `capability` (`app-only` or `incident-policy`). It writes `preview.request.json`, and for the second capability adds the fixed CRD contract. The Go evaluator validates the exact CRD digest before choosing vCluster.

Reuse a request ID to update size or lifetime on its existing branch. Use a new request ID if the required capability changes, and close the previous PR when its preview is no longer needed. The template does not remove an earlier CRD file from an existing branch.

In an existing Backstage app, install the Scaffolder GitHub module and `@backstage/plugin-mcp-actions-backend`, register the catalog and template locations, and add the preview plugin to `packages/backend/src/index.ts`. Use the Backstage release's own dependency versions for its packages. Example backend wiring:

```ts
backend.add(import('@backstage/plugin-scaffolder-backend-module-github'));
backend.add(import('@backstage/plugin-mcp-actions-backend'));
backend.add(import('@preview/plugin-preview-backend'));
```

Example configuration:

```yaml
backend:
  actions:
    pluginSources: [catalog, scaffolder, preview]
  auth:
    externalAccess:
      - type: static
        options:
          token: ${MCP_TOKEN}
          subject: mcp-clients
        accessRestrictions:
          - plugin: mcp-actions
          - plugin: catalog
          - plugin: scaffolder
          - plugin: preview
mcpActions:
  name: Preview Platform
  description: Discover services, inspect preview requests, open PRs, and read preview status.
  instructions: Inspect the Incident Tracker catalog and template inputs before executing a preview request.
preview:
  statusApiBaseUrl: http://127.0.0.1:8090
```

The MCP endpoint is `/api/mcp-actions/v1`. Discover `catalog` entities and the Scaffolder template, call `scaffolder.execute-template` with its typed input, then call `preview.get-status` with the PR number from the Scaffolder output. In the UI, open the same template under Create and use the PR link it returns. Authentication is required; keep `MCP_TOKEN` outside Git.

The plugin is source code awaiting integration and a live Backstage run. This workspace has no Backstage installation, so the UI/MCP parity check is still pending.

References: [Backstage MCP Actions Backend](https://backstage.io/docs/ai/mcp-actions/), [well-known actions](https://backstage.io/docs/ai/well-known-actions/), [Scaffolder templates](https://backstage.io/docs/features/software-templates/writing-templates/), and [Actions Registry](https://backstage.io/docs/backend-system/core-services/actions-registry/).
