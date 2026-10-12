import { useEffect, useState } from 'react';
import {
  Box,
  Button,
  CircularProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@material-ui/core';
import RefreshIcon from '@material-ui/icons/Refresh';
import { discoveryApiRef, fetchApiRef, useApi } from '@backstage/frontend-plugin-api';

type GuestResource = {
  kind: string;
  name: string;
  namespace?: string;
  state: string;
  fields: { name: string; value: string }[];
};

type GuestInventory = {
  name: string;
  headSHA: string;
  resources: GuestResource[];
};

export function GuestResourcesPanel({ prNumber, headSHA }: { prNumber: number; headSHA: string }) {
  const discovery = useApi(discoveryApiRef);
  const fetchApi = useApi(fetchApiRef);
  const [inventory, setInventory] = useState<GuestInventory>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let active = true;
    async function refresh() {
      if (!active) return;
      setLoading(true);
      try {
        const base = await discovery.getBaseUrl('proxy');
        const response = await fetchApi.fetch(
          `${base}/preview-status/api/previews/incident-tracker-pr-${prNumber}/guest-resources`,
          { cache: 'no-store' },
        );
        if (!response.ok) throw new Error(`Virtual cluster inventory returned HTTP ${response.status}`);
        const result = await response.json() as GuestInventory;
        if (result.headSHA !== headSHA) throw new Error('Virtual cluster inventory is for an older PR commit');
        if (!active) return;
        setInventory(result);
        setError('');
      } catch (cause) {
        if (active) {
          setInventory(undefined);
          setError(cause instanceof Error ? cause.message : 'Could not load virtual cluster resources');
        }
      } finally {
        if (active) setLoading(false);
      }
    }
    refresh();
    const timer = window.setInterval(refresh, 20_000);
    return () => { active = false; window.clearInterval(timer); };
  }, [prNumber, headSHA, discovery, fetchApi, refreshKey]);

  return <Paper id="virtual-resources" style={{ padding: 24, borderRadius: 16, marginTop: 20 }}>
    <Box display="flex" justifyContent="space-between" alignItems="center" flexWrap="wrap" style={{ gap: 12 }}>
      <Box>
        <Typography variant="h6" style={{ fontWeight: 700 }}>Virtual cluster resources</Typography>
        <Typography color="textSecondary" variant="body2" style={{ marginTop: 4 }}>
          Live, read-only details from the Kubernetes API inside this PR&apos;s vCluster.
        </Typography>
      </Box>
      <Button variant="outlined" startIcon={<RefreshIcon />} onClick={() => setRefreshKey(key => key + 1)} disabled={loading}>
        Refresh
      </Button>
    </Box>
    {loading && <Box display="flex" alignItems="center" style={{ gap: 8, marginTop: 20 }}>
      <CircularProgress size={18} /><Typography variant="body2">Reading virtual cluster…</Typography>
    </Box>}
    {error && <Typography role="alert" color="error" style={{ marginTop: 16 }}>{error}</Typography>}
    {inventory && inventory.resources.length === 0 &&
      <Typography color="textSecondary" style={{ marginTop: 16 }}>No matching resources were found for this PR commit.</Typography>}
    {inventory && inventory.resources.length > 0 && <Box style={{ overflowX: 'auto', marginTop: 18 }}>
      <Table size="small" aria-label="Virtual cluster resources">
        <TableHead><TableRow>
          <TableCell>Kind</TableCell><TableCell>Name</TableCell><TableCell>Namespace</TableCell><TableCell>Status</TableCell><TableCell>Definition</TableCell>
        </TableRow></TableHead>
        <TableBody>{inventory.resources.map(resource => <TableRow key={`${resource.kind}/${resource.namespace ?? ''}/${resource.name}`}>
          <TableCell>{resource.kind}</TableCell>
          <TableCell>{resource.name}</TableCell>
          <TableCell>{resource.namespace || 'Cluster-wide'}</TableCell>
          <TableCell>{resource.state}</TableCell>
          <TableCell>{resource.fields.map(field => <Typography key={field.name} variant="body2" style={{ overflowWrap: 'anywhere' }}>
            <strong>{field.name}:</strong> {field.value}
          </Typography>)}</TableCell>
        </TableRow>)}</TableBody>
      </Table>
    </Box>}
  </Paper>;
}
