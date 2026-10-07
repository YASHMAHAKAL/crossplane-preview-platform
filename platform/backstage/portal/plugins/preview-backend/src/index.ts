import { createBackendPlugin, coreServices } from '@backstage/backend-plugin-api';
import { actionsRegistryServiceRef } from '@backstage/backend-plugin-api/alpha';

/** Read-only status action exposed through Backstage's MCP Actions Backend. */
export const previewPlugin = createBackendPlugin({
  pluginId: 'preview',
  register(env) {
    env.registerInit({
      deps: {
        actionsRegistry: actionsRegistryServiceRef,
        auth: coreServices.auth,
        config: coreServices.rootConfig,
        discovery: coreServices.discovery,
      },
      async init({ actionsRegistry, auth, config, discovery }) {
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
            if (response.status === 404) {
              return { output: {
                name: `incident-tracker-pr-${input.prNumber}`,
                phase: 'pending-evaluation',
                reasonCodes: ['status-not-yet-recorded'],
                evidence: ['The watcher has not recorded this PR yet; retry shortly.'],
                headSHA: '',
                prNumber: input.prNumber,
                updatedAt: new Date().toISOString(),
              } };
            }
            if (!response.ok) throw new Error(`Preview status request failed: HTTP ${response.status}`);
            return { output: await response.json() };
          },
        });
        actionsRegistry.register({
          name: 'get-request-task',
          title: 'Get preview request PR',
          description: 'Resolve a completed preview Scaffolder task ID to its GitHub PR URL and number. Poll this after scaffolder.execute-template, then use preview.get-status with the PR number.',
          attributes: { readOnly: true, idempotent: true, destructive: false },
          schema: {
            input: z => z.object({ taskId: z.string().uuid() }),
            output: z => z.object({
              taskId: z.string(), status: z.string(),
              prNumber: z.number().int().positive().optional(),
              prUrl: z.string().url().optional(),
            }),
          },
          action: async ({ input, credentials }) => {
            const baseUrl = await discovery.getBaseUrl('scaffolder');
            const { token } = await auth.getPluginRequestToken({
              onBehalfOf: credentials,
              targetPluginId: 'scaffolder',
            });
            const taskPath = `${baseUrl}/v2/tasks/${encodeURIComponent(input.taskId)}`;
            const headers = { Authorization: `Bearer ${token}` };
            const taskResponse = await fetch(taskPath, { headers });
            if (!taskResponse.ok) throw new Error(`Scaffolder task request failed: HTTP ${taskResponse.status}`);
            const task = await taskResponse.json() as { status: string };
            const output = { taskId: input.taskId, status: task.status, prNumber: undefined as number | undefined, prUrl: undefined as string | undefined };
            if (task.status !== 'completed') return { output };

            const eventsResponse = await fetch(`${taskPath}/events`, { headers });
            if (!eventsResponse.ok) throw new Error(`Scaffolder task events request failed: HTTP ${eventsResponse.status}`);
            const events = await eventsResponse.json() as Array<{
              type?: string;
              body?: { output?: { links?: Array<{ title?: string; url?: string }> } };
            }>;
            const completion = events.find(event => event.type === 'completion');
            const prLink = completion?.body?.output?.links?.find(link => link.title === 'Preview request PR');
            const match = prLink?.url?.match(/^https:\/\/github\.com\/YASHMAHAKAL\/crossplane-preview-platform\/pull\/([1-9][0-9]*)$/);
            if (match) {
              output.prNumber = Number(match[1]);
              output.prUrl = prLink?.url;
            }
            return { output };
          },
        });
      },
    });
  },
});

export default previewPlugin;
