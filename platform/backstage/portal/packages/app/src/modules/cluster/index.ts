import { createFrontendModule } from '@backstage/frontend-plugin-api';
import { EntityContentBlueprint, convertLegacyEntityContentExtension } from '@backstage/plugin-catalog-react/alpha';
import { EntityKubernetesClusterContent } from '@backstage/plugin-kubernetes-cluster';
import { createElement } from 'react';
import { ClusterResourcesPage } from './ClusterResourcesPage';

const isClusterEntity = (entity: { kind: string; spec?: { type?: string } }) =>
  entity.kind.toLowerCase() === 'resource' && entity.spec?.type === 'kubernetes-cluster';

const clusterContent = convertLegacyEntityContentExtension(
  EntityKubernetesClusterContent,
  {
    name: 'cluster',
    path: '/cluster',
    title: 'Cluster',
    filter: isClusterEntity,
  },
);

const resourcesContent = EntityContentBlueprint.make({
  name: 'resources',
  params: {
    path: '/resources',
    title: 'Resources',
    filter: isClusterEntity,
    loader: async () => createElement(ClusterResourcesPage),
  },
});

export const clusterModule = createFrontendModule({
  pluginId: 'catalog',
  extensions: [clusterContent, resourcesContent],
});
