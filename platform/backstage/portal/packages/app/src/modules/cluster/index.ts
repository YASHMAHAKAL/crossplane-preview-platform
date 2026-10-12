import { createFrontendModule } from '@backstage/frontend-plugin-api';
import { convertLegacyEntityContentExtension } from '@backstage/plugin-catalog-react/alpha';
import { EntityKubernetesClusterContent } from '@backstage/plugin-kubernetes-cluster';

const clusterContent = convertLegacyEntityContentExtension(
  EntityKubernetesClusterContent,
  {
    name: 'cluster',
    path: '/cluster',
    title: 'Cluster',
    filter: entity => entity.kind.toLowerCase() === 'resource' &&
      entity.spec?.type === 'kubernetes-cluster',
  },
);

export const clusterModule = createFrontendModule({
  pluginId: 'catalog',
  extensions: [clusterContent],
});
