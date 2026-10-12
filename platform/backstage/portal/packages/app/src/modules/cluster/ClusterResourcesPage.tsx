import { useCallback, useEffect, useState } from 'react';
import {
  Box,
  Button,
  CircularProgress,
  MenuItem,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@material-ui/core';
import RefreshIcon from '@material-ui/icons/Refresh';
import { useApi } from '@backstage/frontend-plugin-api';
import { useEntity } from '@backstage/plugin-catalog-react';
import { kubernetesApiRef } from '@backstage/plugin-kubernetes-react';

type Resource = {
  metadata?: { name?: string; namespace?: string; creationTimestamp?: string };
  spec?: {
    replicas?: number;
    type?: string;
    clusterIP?: string;
    nodeName?: string;
    rules?: { host?: string }[];
    hard?: Record<string, string>;
    decision?: { mode?: string };
  };
  status?: {
    phase?: string;
    readyReplicas?: number;
    availableReplicas?: number;
    conditions?: { type?: string; status?: string }[];
  };
};

type ResourceList = {
  items?: Resource[];
  metadata?: { continue?: string };
};

const kinds = [
  { key: 'namespaces', label: 'Namespaces', path: '/api/v1/namespaces' },
  { key: 'pods', label: 'Pods', path: '/api/v1/pods' },
  { key: 'deployments', label: 'Deployments', path: '/apis/apps/v1/deployments' },
  { key: 'statefulsets', label: 'StatefulSets', path: '/apis/apps/v1/statefulsets' },
  { key: 'services', label: 'Services', path: '/api/v1/services' },
  { key: 'ingresses', label: 'Ingresses', path: '/apis/networking.k8s.io/v1/ingresses' },
  { key: 'resourcequotas', label: 'Quotas', path: '/api/v1/resourcequotas' },
  { key: 'previewenvironments', label: 'Preview XRs', path: '/apis/preview.platform.example.org/v1alpha1/previewenvironments' },
  { key: 'releases', label: 'Helm Releases', path: '/apis/helm.crossplane.io/v1beta1/releases' },
] as const;

type KindKey = (typeof kinds)[number]['key'];
type Inventory = Partial<Record<KindKey, Resource[]>>;
type InventoryErrors = Partial<Record<KindKey, string>>;

function statusFor(key: KindKey, item: Resource): string {
  if (key === 'pods' || key === 'namespaces') return item.status?.phase ?? 'Unknown';
  if (key === 'deployments' || key === 'statefulsets') {
    return `${item.status?.readyReplicas ?? 0}/${item.spec?.replicas ?? 1} ready`;
  }
  if (key === 'services') return item.spec?.type ?? 'Unknown';
  if (key === 'ingresses') return item.spec?.rules?.some(rule => rule.host) ? 'Configured' : 'No host';
  const ready = item.status?.conditions?.find(condition => condition.type === 'Ready');
  return ready ? (ready.status === 'True' ? 'Ready' : 'Not ready') : '—';
}

function detailFor(key: KindKey, item: Resource): string {
  if (key === 'pods') return item.spec?.nodeName ?? 'No node assigned';
  if (key === 'services') return item.spec?.clusterIP ?? '—';
  if (key === 'ingresses') return item.spec?.rules?.map(rule => rule.host).filter(Boolean).join(', ') || '—';
  if (key === 'resourcequotas') return item.spec?.hard?.pods ? `${item.spec.hard.pods} pod limit` : '—';
  if (key === 'previewenvironments') return item.spec?.decision?.mode ?? '—';
  return item.metadata?.creationTimestamp ? new Date(item.metadata.creationTimestamp).toLocaleString() : '—';
}

export function ClusterResourcesPage() {
  const { entity } = useEntity();
  const clusterName = entity.metadata.name;
  const kubernetesApi = useApi(kubernetesApiRef);
  const [inventory, setInventory] = useState<Inventory>({});
  const [errors, setErrors] = useState<InventoryErrors>({});
  const [selected, setSelected] = useState<KindKey>('pods');
  const [namespace, setNamespace] = useState('');
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [updatedAt, setUpdatedAt] = useState<Date>();
  const [refreshNumber, setRefreshNumber] = useState(0);

  const readList = useCallback(async (path: string): Promise<Resource[]> => {
    const items: Resource[] = [];
    let continuation = '';
    // Bound the view to 2,500 objects per kind so a large cluster cannot
    // stall the local portal. The UI reports when that limit is reached.
    for (let page = 0; page < 10; page += 1) {
      const query = new URLSearchParams({ limit: '250' });
      if (continuation) query.set('continue', continuation);
      const response = await kubernetesApi.proxy({ clusterName, path: `${path}?${query}` });
      if (!response.ok) throw new Error(`Kubernetes API returned HTTP ${response.status}`);
      const result = await response.json() as ResourceList;
      if (!Array.isArray(result.items)) throw new Error('Kubernetes API returned an invalid list');
      items.push(...result.items);
      continuation = result.metadata?.continue ?? '';
      if (!continuation) return items;
    }
    throw new Error('This kind has more than 2,500 objects and exceeds the local inventory limit');
  }, [clusterName, kubernetesApi]);

  useEffect(() => {
    let active = true;
    setLoading(true);
    Promise.all(kinds.map(async kind => {
      try {
        return { key: kind.key, items: await readList(kind.path) };
      } catch (error) {
        return { key: kind.key, error: error instanceof Error ? error.message : String(error) };
      }
    })).then(results => {
      if (!active) return;
      const nextInventory: Inventory = {};
      const nextErrors: InventoryErrors = {};
      for (const result of results) {
        if ('items' in result) nextInventory[result.key] = result.items;
        if ('error' in result) nextErrors[result.key] = result.error;
      }
      setInventory(nextInventory);
      setErrors(nextErrors);
      setUpdatedAt(new Date());
      setLoading(false);
    });
    return () => { active = false; };
  }, [readList, refreshNumber]);

  useEffect(() => {
    const timer = window.setInterval(() => setRefreshNumber(value => value + 1), 30_000);
    return () => window.clearInterval(timer);
  }, []);

  const namespaces = (inventory.namespaces ?? [])
    .map(item => item.metadata?.name ?? '')
    .filter(Boolean)
    .sort();
  const selectedKind = kinds.find(kind => kind.key === selected)!;
  const filtered = (inventory[selected] ?? []).filter(item => {
    const name = item.metadata?.name ?? '';
    const itemNamespace = selected === 'namespaces' ? name : item.metadata?.namespace ?? '';
    return (!namespace || itemNamespace === namespace) &&
      (!search || `${itemNamespace}/${name}`.toLowerCase().includes(search.toLowerCase().trim()));
  });

  return (
    <Box paddingTop={2} paddingBottom={4}>
      <Paper style={{ padding: 24, marginBottom: 20, borderRadius: 12 }}>
        <Box display="flex" justifyContent="space-between" alignItems="center" flexWrap="wrap" style={{ gap: 12 }}>
          <Box>
            <Typography variant="h5">Cluster resources</Typography>
            <Typography variant="body2" color="textSecondary">
              Read-only inventory from {clusterName}. Secrets are not included.
            </Typography>
          </Box>
          <Button variant="outlined" startIcon={<RefreshIcon />} onClick={() => setRefreshNumber(value => value + 1)}>
            Refresh
          </Button>
        </Box>
        <Box display="flex" flexWrap="wrap" marginTop={3} style={{ gap: 8 }}>
          {kinds.map(kind => (
            <Button key={kind.key} size="small" variant={selected === kind.key ? 'contained' : 'outlined'}
              color={selected === kind.key ? 'primary' : 'default'} onClick={() => setSelected(kind.key)}>
              {kind.label} ({inventory[kind.key]?.length ?? 0})
            </Button>
          ))}
        </Box>
      </Paper>

      <Paper style={{ padding: 24, borderRadius: 12 }}>
        <Box display="flex" justifyContent="space-between" alignItems="center" flexWrap="wrap" style={{ gap: 12, marginBottom: 16 }}>
          <Typography variant="h6">{selectedKind.label}</Typography>
          <Box display="flex" flexWrap="wrap" style={{ gap: 12 }}>
            <TextField id="cluster-resource-namespace" select size="small" variant="outlined" label="Namespace" value={namespace}
              onChange={event => setNamespace(event.target.value)} style={{ minWidth: 180 }}>
              <MenuItem value="">All namespaces</MenuItem>
              {namespaces.map(name => <MenuItem key={name} value={name}>{name}</MenuItem>)}
            </TextField>
            <TextField id="cluster-resource-search" size="small" variant="outlined" label="Search name" value={search}
              onChange={event => setSearch(event.target.value)} />
          </Box>
        </Box>
        {loading && <Box display="flex" alignItems="center" style={{ gap: 10, padding: 12 }}>
          <CircularProgress size={18} /><Typography variant="body2">Refreshing cluster resources…</Typography>
        </Box>}
        {errors[selected] && <Typography color="error" role="alert">{selectedKind.label}: {errors[selected]}</Typography>}
        {!errors[selected] && !loading && filtered.length === 0 &&
          <Typography color="textSecondary">No {selectedKind.label.toLowerCase()} match this filter.</Typography>}
        {filtered.length > 0 && <Table size="small" aria-label={`${selectedKind.label} inventory`}>
          <TableHead><TableRow>
            <TableCell>Name</TableCell><TableCell>Namespace</TableCell><TableCell>Status</TableCell><TableCell>Details</TableCell>
          </TableRow></TableHead>
          <TableBody>{filtered.map(item => (
            <TableRow key={`${item.metadata?.namespace ?? ''}/${item.metadata?.name ?? ''}`}>
              <TableCell>{item.metadata?.name ?? '—'}</TableCell>
              <TableCell>{item.metadata?.namespace ?? '—'}</TableCell>
              <TableCell>{statusFor(selected, item)}</TableCell>
              <TableCell>{detailFor(selected, item)}</TableCell>
            </TableRow>
          ))}</TableBody>
        </Table>}
        {updatedAt && <Typography variant="caption" color="textSecondary" style={{ display: 'block', marginTop: 16 }}>
          Updated {updatedAt.toLocaleTimeString()} · refreshes every 30 seconds
        </Typography>}
      </Paper>
    </Box>
  );
}
