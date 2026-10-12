import React, { useCallback, useEffect, useState } from 'react';
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

type Container = {
  name?: string;
  image?: string;
  ports?: { containerPort?: number }[];
  resources?: { requests?: Record<string, string>; limits?: Record<string, string> };
};

type Resource = {
  metadata?: { name?: string; namespace?: string; creationTimestamp?: string; labels?: Record<string, string> };
  spec?: {
    replicas?: number;
    type?: string;
    clusterIP?: string;
    nodeName?: string;
    serviceAccountName?: string;
    containers?: Container[];
    template?: { spec?: { containers?: Container[] } };
    strategy?: { type?: string };
    selector?: Record<string, string> | { matchLabels?: Record<string, string> };
    ports?: { name?: string; port?: number; targetPort?: number | string; protocol?: string }[];
    ingressClassName?: string;
    rules?: { host?: string; http?: { paths?: { path?: string; backend?: { service?: { name?: string; port?: { number?: number } } } }[] } }[];
    hard?: Record<string, string>;
    decision?: { mode?: string; reasonCodes?: string[] };
    serviceRef?: string;
    pr?: { number?: number; headSHA?: string };
    image?: { digest?: string };
    request?: { size?: string; ttlMinutes?: number };
    deployment?: { replicas?: number };
    forProvider?: { namespace?: string; chart?: { name?: string; repository?: string; version?: string } };
  };
  status?: {
    phase?: string;
    readyReplicas?: number;
    availableReplicas?: number;
    conditions?: { type?: string; status?: string }[];
    used?: Record<string, string>;
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

type DefinitionField = { name: string; value: string };

function definitionFor(key: KindKey, item: Resource): DefinitionField[] {
  const fields: DefinitionField[] = [];
  const add = (name: string, value?: string | number) => {
    if (value !== undefined && value !== '') fields.push({ name, value: String(value) });
  };
  const map = (value?: Record<string, string>) =>
    Object.entries(value ?? {}).map(([name, entry]) => `${name}=${entry}`).join(', ');
  const containers = (items?: Container[]) => {
    for (const container of items ?? []) {
      add(`Container ${container.name ?? 'app'}`, container.image);
      add('Container ports', container.ports?.map(port => port.containerPort).filter(Boolean).join(', '));
      add('Requests', map(container.resources?.requests));
      add('Limits', map(container.resources?.limits));
    }
  };
  if (key === 'namespaces') {
    add('Phase', item.status?.phase);
    add('Preview service', item.metadata?.labels?.['preview.platform.example.org/service']);
    add('PR number', item.metadata?.labels?.['preview.platform.example.org/pr']);
  } else if (key === 'pods') {
    add('Node', item.spec?.nodeName);
    add('Service account', item.spec?.serviceAccountName);
    containers(item.spec?.containers);
  } else if (key === 'deployments' || key === 'statefulsets') {
    add('Desired replicas', item.spec?.replicas);
    add('Ready replicas', item.status?.readyReplicas ?? 0);
    add('Update strategy', item.spec?.strategy?.type);
    const selector = item.spec?.selector?.matchLabels;
    if (selector && typeof selector === 'object') add('Selector', map(selector));
    containers(item.spec?.template?.spec?.containers);
  } else if (key === 'services') {
    add('Type', item.spec?.type);
    add('Cluster IP', item.spec?.clusterIP);
    add('Selector', map(item.spec?.selector as Record<string, string> | undefined));
    add('Ports', item.spec?.ports?.map(port => `${port.name ?? 'port'}: ${port.port} → ${port.targetPort ?? port.port}/${port.protocol ?? 'TCP'}`).join(', '));
  } else if (key === 'ingresses') {
    add('Ingress class', item.spec?.ingressClassName);
    for (const rule of item.spec?.rules ?? []) {
      add('Host', rule.host);
      add('Paths', rule.http?.paths?.map(path => `${path.path ?? '/'} → ${path.backend?.service?.name ?? 'service'}:${path.backend?.service?.port?.number ?? 80}`).join(', '));
    }
  } else if (key === 'resourcequotas') {
    add('Limits', map(item.spec?.hard));
    add('Used', map(item.status?.used));
  } else if (key === 'previewenvironments') {
    add('Service', item.spec?.serviceRef);
    add('PR number', item.spec?.pr?.number);
    add('Source commit', item.spec?.pr?.headSHA);
    add('Mode', item.spec?.decision?.mode);
    add('Reason', item.spec?.decision?.reasonCodes?.join(', '));
    add('Image digest', item.spec?.image?.digest);
    add('Size', item.spec?.request?.size);
    add('TTL minutes', item.spec?.request?.ttlMinutes);
    add('Replicas', item.spec?.deployment?.replicas);
  } else if (key === 'releases') {
    add('Namespace', item.spec?.forProvider?.namespace);
    add('Chart', item.spec?.forProvider?.chart?.name);
    add('Chart version', item.spec?.forProvider?.chart?.version);
    add('Repository', item.spec?.forProvider?.chart?.repository);
  }
  return fields;
}

export function ClusterResourcesPage() {
  const { entity } = useEntity();
  const clusterName = entity.metadata.name;
  const kubernetesApi = useApi(kubernetesApiRef);
  const [inventory, setInventory] = useState<Inventory>({});
  const [errors, setErrors] = useState<InventoryErrors>({});
  const [selected, setSelected] = useState<KindKey>('pods');
  const [expanded, setExpanded] = useState('');
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
            <TableCell>Name</TableCell><TableCell>Namespace</TableCell><TableCell>Status</TableCell><TableCell>Details</TableCell><TableCell>Definition</TableCell>
          </TableRow></TableHead>
          <TableBody>{filtered.map(item => {
            const key = `${selected}/${item.metadata?.namespace ?? ''}/${item.metadata?.name ?? ''}`;
            const open = expanded === key;
            return <React.Fragment key={key}>
              <TableRow>
                <TableCell>{item.metadata?.name ?? '—'}</TableCell>
                <TableCell>{item.metadata?.namespace ?? '—'}</TableCell>
                <TableCell>{statusFor(selected, item)}</TableCell>
                <TableCell>{detailFor(selected, item)}</TableCell>
                <TableCell><Button size="small" onClick={() => setExpanded(open ? '' : key)} aria-expanded={open}>
                  {open ? 'Hide details' : 'Inspect'}
                </Button></TableCell>
              </TableRow>
              {open && <TableRow><TableCell colSpan={5}>
                <Box component="dl" style={{ display: 'grid', gridTemplateColumns: 'minmax(120px, 180px) minmax(0, 1fr)', gap: '8px 20px', padding: '12px 8px' }}>
                  {definitionFor(selected, item).map(field => <React.Fragment key={field.name}>
                    <Typography component="dt" color="textSecondary" variant="body2">{field.name}</Typography>
                    <Typography component="dd" variant="body2" style={{ margin: 0, overflowWrap: 'anywhere' }}>{field.value}</Typography>
                  </React.Fragment>)}
                </Box>
              </TableCell></TableRow>}
            </React.Fragment>;
          })}</TableBody>
        </Table>}
        {updatedAt && <Typography variant="caption" color="textSecondary" style={{ display: 'block', marginTop: 16 }}>
          Updated {updatedAt.toLocaleTimeString()} · refreshes every 30 seconds
        </Typography>}
      </Paper>
    </Box>
  );
}
