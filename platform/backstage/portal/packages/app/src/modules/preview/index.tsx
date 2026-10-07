import { createFrontendPlugin, createRouteRef, PageBlueprint } from '@backstage/frontend-plugin-api';
import DashboardIcon from '@material-ui/icons/Dashboard';
import { PreviewStatusPage } from './PreviewStatusPage';

const routeRef = createRouteRef();

const statusPage = PageBlueprint.make({
  params: {
    path: '/previews',
    routeRef,
    title: 'PR Previews',
    icon: <DashboardIcon fontSize="inherit" />,
    loader: async () => <PreviewStatusPage />,
  },
});

export const previewPlugin = createFrontendPlugin({
  pluginId: 'preview',
  routes: { root: routeRef },
  extensions: [statusPage],
});
