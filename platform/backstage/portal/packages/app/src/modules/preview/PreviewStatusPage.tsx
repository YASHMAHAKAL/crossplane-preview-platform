import React, { useEffect, useState } from 'react';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Link,
  Paper,
  TextField,
  Typography,
} from '@material-ui/core';
import OpenInNewIcon from '@material-ui/icons/OpenInNew';
import RefreshIcon from '@material-ui/icons/Refresh';
import { discoveryApiRef, fetchApiRef, useApi } from '@backstage/frontend-plugin-api';
import { useSearchParams } from 'react-router-dom';

type PreviewStatus = {
  name: string;
  phase: string;
  mode?: string;
  reasonCodes: string[];
  evidence: string[];
  headSHA: string;
  prNumber: number;
  url?: string;
  updatedAt: string;
  expiresAt?: string;
};

const pullUrl = (number: number) =>
  `https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/${number}`;

export function parsePreviewPR(value: string): number | undefined {
  const input = value.trim();
  const match = input.match(/^[1-9][0-9]*$/) ??
    input.match(/^https:\/\/github\.com\/YASHMAHAKAL\/crossplane-preview-platform\/pull\/([1-9][0-9]*)\/?$/)?.slice(1);
  if (!match) return undefined;
  const number = Number(match[0]);
  return Number.isSafeInteger(number) ? number : undefined;
}

function labelForPhase(phase: string): string {
  return phase.split('-').map(word => word[0]?.toUpperCase() + word.slice(1)).join(' ');
}

function phaseTone(phase: string): { background: string; color: string } {
  if (phase === 'ready') return { background: '#d8f6e4', color: '#125734' };
  if (phase === 'deleted') return { background: '#e1ebff', color: '#1d4275' };
  if (phase === 'skipped') return { background: '#e9edf3', color: '#415268' };
  if (phase === 'rejected' || phase === 'cleanup-failed' || phase === 'degraded') {
    return { background: '#ffe2df', color: '#8c2922' };
  }
  return { background: '#fff0cd', color: '#704700' };
}

function readableTime(value?: string): string {
  if (!value) return '—';
  const timestamp = new Date(value);
  return Number.isNaN(timestamp.getTime()) ? value : timestamp.toLocaleString();
}

const panelStyle: React.CSSProperties = {
  padding: 24,
  borderRadius: 16,
  border: '1px solid #e1e7f0',
  boxShadow: '0 10px 30px rgba(20, 41, 72, 0.06)',
};

export function PreviewStatusPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const discovery = useApi(discoveryApiRef);
  const fetchApi = useApi(fetchApiRef);
  const requested = searchParams.get('pr') ?? searchParams.get('prUrl') ?? '';
  const prNumber = parsePreviewPR(requested);
  const [input, setInput] = useState(requested);
  const [status, setStatus] = useState<PreviewStatus | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => setInput(requested), [requested]);

  useEffect(() => {
    if (!prNumber) {
      setStatus(undefined);
      setError('');
      return undefined;
    }
    const selectedNumber = prNumber;
    let active = true;
    async function refresh() {
      if (!active) return;
      setLoading(true);
      try {
        const base = await discovery.getBaseUrl('proxy');
        const response = await fetchApi.fetch(
          `${base}/preview-status/api/previews/incident-tracker-pr-${selectedNumber}`,
          { cache: 'no-store' },
        );
        if (!active) return;
        if (response.status === 404) {
          setStatus({
            name: `incident-tracker-pr-${selectedNumber}`,
            phase: 'pending-evaluation',
            reasonCodes: ['status-not-yet-recorded'],
            evidence: ['The watcher has not recorded this PR yet. This page refreshes automatically.'],
            headSHA: '',
            prNumber: selectedNumber,
            updatedAt: new Date().toISOString(),
          });
        } else if (response.ok) {
          setStatus(await response.json() as PreviewStatus);
        } else {
          throw new Error(`Status request returned HTTP ${response.status}`);
        }
        setError('');
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : 'Could not load preview status');
      } finally {
        if (active) setLoading(false);
      }
    }
    refresh();
    const interval = window.setInterval(refresh, 10_000);
    return () => {
      active = false;
      window.clearInterval(interval);
    };
  }, [prNumber, discovery, fetchApi, refreshKey]);

  const submit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const number = parsePreviewPR(input);
    if (!number) {
      setError('Enter a PR number or a pull request URL from this project.');
      return;
    }
    setStatus(undefined);
    setError('');
    setSearchParams({ pr: String(number) });
  };

  const tone = status ? phaseTone(status.phase) : undefined;
  const isComplete = status?.phase === 'deleted';
  const isReady = status?.phase === 'ready' && Boolean(status.url);
  const isPending = status?.phase === 'pending-evaluation' || status?.phase === 'waiting-for-ci';
  let lifecycleMessage = 'The platform is evaluating this pull request.';
  if (isComplete) lifecycleMessage = 'The preview and its route have been removed.';
  else if (status?.phase === 'expired') lifecycleMessage = 'The preview deadline passed. Cleanup is pending.';
  else if (status?.phase === 'cleaning') lifecycleMessage = 'The platform is removing the preview and checking its resources.';
  else if (isReady) lifecycleMessage = 'The preview is ready to explore.';
  else if (status?.phase === 'skipped') lifecycleMessage = 'This PR has no supported preview changes.';
  else if (isPending) lifecycleMessage = 'The platform is waiting for a current decision or build.';
  let modeLabel = 'Awaiting decision';
  if (status?.mode === 'vcluster') modeLabel = 'vCluster';
  else if (status?.mode === 'namespace') modeLabel = 'Namespace';
  else if (isComplete) modeLabel = 'No active environment';
  let urlLabel = 'Available when healthy';
  if (isComplete) urlLabel = 'Removed';
  else if (status?.phase === 'expired' || status?.phase === 'cleaning') urlLabel = 'Unavailable during cleanup';

  return (
    <Box style={{ maxWidth: 1180, margin: '0 auto', padding: '24px 24px 64px' }}>
      <Paper style={{ ...panelStyle, color: '#fff', background: 'linear-gradient(125deg, #10213d, #224878 65%, #316a87)', border: 0 }}>
        <Typography variant="overline" style={{ color: '#9bd9d7', letterSpacing: 2 }}>
          PREVIEW CONTROL CENTER
        </Typography>
        <Typography variant="h4" component="h1" style={{ marginTop: 6, fontWeight: 700 }}>
          Follow a PR from change to cleanup
        </Typography>
        <Typography variant="body1" style={{ marginTop: 12, maxWidth: 650, color: '#d9e5f5' }}>
          See why the platform chose a namespace or vCluster, when the Incident Tracker is ready, and whether deletion finished.
        </Typography>
      </Paper>

      <Paper style={{ ...panelStyle, marginTop: 20 }}>
        <form onSubmit={submit} style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
          <TextField
            label="PR number or GitHub PR URL"
            value={input}
            onChange={event => setInput(event.target.value)}
            variant="outlined"
            size="small"
            style={{ flex: '1 1 320px' }}
            inputProps={{ 'aria-label': 'PR number or GitHub PR URL' }}
          />
          <Button type="submit" variant="contained" color="primary">View preview</Button>
          {prNumber && <Button onClick={() => setRefreshKey(key => key + 1)} startIcon={<RefreshIcon />} disabled={loading}>Refresh</Button>}
        </form>
        {error && <Typography role="alert" style={{ color: '#a3312b', marginTop: 12 }}>{error}</Typography>}
        {requested && !prNumber && !error && <Typography role="alert" style={{ color: '#a3312b', marginTop: 12 }}>The link does not contain a valid project PR.</Typography>}
      </Paper>

      {!prNumber && !requested && (
        <Paper style={{ ...panelStyle, marginTop: 20 }}>
          <Typography variant="h6">Start with a pull request</Typography>
          <Typography color="textSecondary" style={{ marginTop: 8 }}>Change Incident Tracker app files and open a PR in the source repository. The platform builds and evaluates the new head automatically. Enter the PR number above to follow its preview.</Typography>
        </Paper>
      )}

      {prNumber && (
        <Box style={{ marginTop: 20 }}>
          <Paper style={panelStyle}>
            <Box display="flex" justifyContent="space-between" alignItems="flex-start" flexWrap="wrap" style={{ gap: 16 }}>
              <Box>
                <Typography variant="overline" color="textSecondary">INCIDENT TRACKER · PR #{prNumber}</Typography>
                <Typography variant="h5" style={{ fontWeight: 700, marginTop: 4 }}>Preview lifecycle</Typography>
                <Typography color="textSecondary" style={{ marginTop: 6 }}>
                  {lifecycleMessage}
                </Typography>
              </Box>
              <Box display="flex" alignItems="center" style={{ gap: 10 }}>
                {loading && <CircularProgress size={20} aria-label="Refreshing status" />}
                {status && <Chip label={labelForPhase(status.phase)} style={{ fontWeight: 700, background: tone?.background, color: tone?.color }} />}
              </Box>
            </Box>
            <Box display="flex" flexWrap="wrap" style={{ gap: 12, marginTop: 24 }}>
              <Button href={pullUrl(prNumber)} target="_blank" rel="noopener noreferrer" startIcon={<OpenInNewIcon />} variant="outlined">Open GitHub PR</Button>
              {isReady && <Button href={status?.url ?? ''} target="_blank" rel="noopener noreferrer" startIcon={<OpenInNewIcon />} variant="contained" color="primary">Open live preview</Button>}
            </Box>
          </Paper>

          {status && (
            <Box style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 20, marginTop: 20 }}>
              <Paper style={panelStyle}>
                <Typography variant="h6" style={{ fontWeight: 700 }}>Decision</Typography>
                <Typography color="textSecondary" style={{ marginTop: 8 }}>Isolation mode</Typography>
                <Typography variant="h5" style={{ marginTop: 2 }}>{modeLabel}</Typography>
                <Box display="flex" flexWrap="wrap" style={{ gap: 8, marginTop: 18 }}>
                  {status.reasonCodes.map(reason => <Chip key={reason} label={reason} size="small" variant="outlined" />)}
                </Box>
                <Typography color="textSecondary" style={{ marginTop: 20 }}>Evidence</Typography>
                {status.evidence.length > 0 ? (
                  <ul style={{ paddingLeft: 20, marginBottom: 0 }}>
                    {status.evidence.map(item => <li key={item}><Typography variant="body2" style={{ overflowWrap: 'anywhere' }}>{item}</Typography></li>)}
                  </ul>
                ) : <Typography variant="body2" style={{ marginTop: 6 }}>No additional evidence recorded.</Typography>}
              </Paper>
              <Paper style={panelStyle}>
                <Typography variant="h6" style={{ fontWeight: 700 }}>Request details</Typography>
                <Box component="dl" style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '12px 20px', marginBottom: 0 }}>
                  <Typography component="dt" color="textSecondary">Source commit</Typography>
                  <Typography component="dd" style={{ margin: 0, overflowWrap: 'anywhere' }} title={status.headSHA}>{status.headSHA ? status.headSHA.slice(0, 12) : 'Pending'}</Typography>
                  <Typography component="dt" color="textSecondary">Last update</Typography>
                  <Typography component="dd" style={{ margin: 0 }}>{readableTime(status.updatedAt)}</Typography>
                  <Typography component="dt" color="textSecondary">Expiry</Typography>
                  <Typography component="dd" style={{ margin: 0 }}>{readableTime(status.expiresAt)}</Typography>
                  <Typography component="dt" color="textSecondary">Preview URL</Typography>
                  <Typography component="dd" style={{ margin: 0, overflowWrap: 'anywhere' }}>
                    {isReady && status.url ? (
                      <Link href={status.url} target="_blank" rel="noopener noreferrer" underline="always">
                        {status.url}
                      </Link>
                    ) : urlLabel}
                  </Typography>
                </Box>
              </Paper>
            </Box>
          )}
        </Box>
      )}
    </Box>
  );
}
