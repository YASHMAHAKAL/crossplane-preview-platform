import { createBackendPlugin, coreServices } from '@backstage/backend-plugin-api';
import { actionsRegistryServiceRef } from '@backstage/backend-plugin-api/alpha';

/** Read-only status action exposed through Backstage's MCP Actions Backend. */
export const previewPlugin = createBackendPlugin({
  pluginId: 'preview',
  register(env) {
    env.registerInit({
      deps: { actionsRegistry: actionsRegistryServiceRef, config: coreServices.rootConfig },
      async init({ actionsRegistry, config }) {
        const statusApi = config.getString('preview.statusApiBaseUrl').replace(/\/$/, '');
        actionsRegistry.register({
          name: 'get-status',
          title: 'Get PR preview status',
          description: 'Read the evaluator decision, evidence, phase, and live URL for an Incident Tracker PR preview.',
          attributes: { readOnly: true, idempotent: true, destructive: false },
          schema: {
            input: z => z.object({ prNumber: z.number().int().positive() }),
            output: z => z.object({
              name: z.string(), phase: z.string(), mode: z.string().optional(),
              reasonCodes: z.array(z.string()), evidence: z.array(z.string()),
              headSHA: z.string(), prNumber: z.number(), url: z.string().optional(),
              updatedAt: z.string(), expiresAt: z.string().optional(),
            }),
          },
          action: async ({ input }) => {
            const response = await fetch(`${statusApi}/api/previews/incident-tracker-pr-${input.prNumber}`);
            if (!response.ok) throw new Error(`Preview status request failed: HTTP ${response.status}`);
            return { output: await response.json() };
          },
        });
      },
    });
  },
});

export default previewPlugin;
