import { createServer } from 'node:http';
import { readFileSync, mkdirSync, writeFileSync, renameSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomUUID } from 'node:crypto';

const here = dirname(fileURLToPath(import.meta.url));
const assets = new Map([
  ['/', ['text/html; charset=utf-8', readFileSync(join(here, '../public/index.html'))]],
  ['/app.js', ['text/javascript; charset=utf-8', readFileSync(join(here, '../public/app.js'))]],
  ['/styles.css', ['text/css; charset=utf-8', readFileSync(join(here, '../public/styles.css'))]],
]);
const statuses = new Set(['open', 'investigating', 'resolved']);
const severities = new Set(['critical', 'high', 'medium', 'low']);
const services = new Set(['api-gateway', 'payments', 'identity', 'incident-tracker']);

function json(res, status, value) {
  res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' });
  res.end(JSON.stringify(value));
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let body = '';
    req.on('data', chunk => {
      body += chunk;
      if (body.length > 65536) {
        reject(new Error('request body exceeds 64 KiB'));
        req.destroy();
      }
    });
    req.on('end', () => {
      try { resolve(JSON.parse(body)); }
      catch { reject(new Error('body must be valid JSON')); }
    });
    req.on('error', reject);
  });
}

function seedIncidents(now) {
  const base = now().getTime();
  const entries = [
    ['INC-1042', 'Elevated checkout failures', 'Payments', 'payments', 'critical', 'investigating', 'Maya Chen', 25,
      'Customers are seeing intermittent payment failures during checkout. The on-call team is tracing a recent provider timeout.'],
    ['INC-1041', 'API response times above SLO', 'API Gateway', 'api-gateway', 'high', 'open', 'Noah Williams', 110,
      'Requests in the EU region have crossed the p95 latency objective. Initial checks suggest an upstream dependency.'],
    ['INC-1040', 'Intermittent sign-in errors', 'Identity', 'identity', 'medium', 'investigating', 'Ava Patel', 260,
      'A subset of users receive an unexpected error after completing multi-factor authentication.'],
    ['INC-1039', 'Incident feed was delayed', 'Incident Tracker', 'incident-tracker', 'low', 'resolved', 'Leo Martin', 1680,
      'The incident feed lagged after a queue restart. Processing caught up and the feed is current.'],
  ];
  return entries.map(([key, title, , service, severity, status, owner, minutes, description], index) => {
    const createdAt = new Date(base - minutes * 60_000).toISOString();
    const updatedAt = new Date(base - Math.max(minutes - 12, 0) * 60_000).toISOString();
    return {
      id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, '0')}`,
      key, title, description, service, severity, status, owner, createdAt, updatedAt,
      activity: [
        { id: `seed-${index}-1`, type: 'created', actor: owner, text: 'Incident created', at: createdAt },
        ...(status === 'open' ? [] : [{ id: `seed-${index}-2`, type: 'status', actor: owner,
          text: `Status changed to ${status}`, at: updatedAt }]),
      ],
    };
  });
}

export function createIncidentServer({ dataFile = null, now = () => new Date(), id = randomUUID, seed = true, previewMode = 'local' } = {}) {
  let incidents;
  if (dataFile) {
    try {
      const saved = JSON.parse(readFileSync(dataFile, 'utf8'));
      if (!Array.isArray(saved)) throw new Error('incident data must be an array');
      incidents = saved;
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
    }
  }
  incidents ??= seed ? seedIncidents(now) : [];

  function save() {
    if (!dataFile) return;
    mkdirSync(dirname(dataFile), { recursive: true });
    const temporary = `${dataFile}.${process.pid}.tmp`;
    writeFileSync(temporary, JSON.stringify(incidents, null, 2) + '\n', { mode: 0o600 });
    renameSync(temporary, dataFile);
  }

  return createServer(async (req, res) => {
    const path = new URL(req.url, 'http://localhost').pathname;
    if (req.method === 'GET' && path === '/healthz') return json(res, 200, { status: 'ok' });
    if (req.method === 'GET' && path === '/api/context') {
      const labels = { namespace: 'Namespace preview', vcluster: 'vCluster preview', local: 'Local workspace' };
      return json(res, 200, { mode: previewMode, label: labels[previewMode] || labels.local });
    }
    if (req.method === 'GET' && path === '/api/incidents') return json(res, 200, { incidents });
    if (req.method === 'GET' && assets.has(path)) {
      const [type, body] = assets.get(path);
      res.writeHead(200, { 'content-type': type });
      return res.end(body);
    }
    if (req.method === 'POST' && path === '/api/incidents') {
      try {
        const input = await readBody(req);
        if (typeof input.title !== 'string' || !input.title.trim() || input.title.length > 120) {
          return json(res, 400, { error: 'Title must contain 1–120 characters.' });
        }
        if (typeof input.description !== 'string' || !input.description.trim() || input.description.length > 2000) {
          return json(res, 400, { error: 'Description must contain 1–2000 characters.' });
        }
        if (!services.has(input.service)) return json(res, 400, { error: 'Choose a valid affected service.' });
        if (!severities.has(input.severity)) return json(res, 400, { error: 'Choose a valid severity.' });
        if (typeof input.owner !== 'string' || !input.owner.trim() || input.owner.length > 60) {
          return json(res, 400, { error: 'Owner must contain 1–60 characters.' });
        }
        const timestamp = now().toISOString();
        const sequence = Math.max(1042, ...incidents.map(item => Number(item.key?.slice(4)) || 0)) + 1;
        const incident = {
          id: id(), key: `INC-${sequence}`, title: input.title.trim(), description: input.description.trim(),
          service: input.service, severity: input.severity, owner: input.owner.trim(), status: 'open',
          createdAt: timestamp, updatedAt: timestamp,
          activity: [{ id: id(), type: 'created', actor: input.owner.trim(), text: 'Incident created', at: timestamp }],
        };
        incidents = [incident, ...incidents];
        save();
        return json(res, 201, { incident });
      } catch (error) { return json(res, 400, { error: error.message }); }
    }
    const match = path.match(/^\/api\/incidents\/([0-9a-f-]+)(?:\/(notes))?$/);
    if (match && (req.method === 'PATCH' || req.method === 'POST')) {
      const incident = incidents.find(item => item.id === match[1]);
      if (!incident) return json(res, 404, { error: 'Incident not found.' });
      try {
        const input = await readBody(req);
        if (req.method === 'PATCH' && !match[2]) {
          if (!statuses.has(input.status)) return json(res, 400, { error: 'Invalid incident status.' });
          if (incident.status !== input.status) {
            incident.status = input.status;
            incident.updatedAt = now().toISOString();
            incident.activity.unshift({ id: id(), type: 'status', actor: 'Operator',
              text: `Status changed to ${input.status}`, at: incident.updatedAt });
            save();
          }
          return json(res, 200, { incident });
        }
        if (req.method === 'POST' && match[2] === 'notes') {
          if (typeof input.text !== 'string' || !input.text.trim() || input.text.length > 500) {
            return json(res, 400, { error: 'Note must contain 1–500 characters.' });
          }
          incident.updatedAt = now().toISOString();
          incident.activity.unshift({ id: id(), type: 'note', actor: 'Operator', text: input.text.trim(), at: incident.updatedAt });
          save();
          return json(res, 201, { incident });
        }
      } catch (error) { return json(res, 400, { error: error.message }); }
    }
    json(res, 404, { error: 'Route not found.' });
  });
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  const server = createIncidentServer({ dataFile: process.env.DATA_FILE || null, previewMode: process.env.PREVIEW_MODE || 'local' });
  const port = Number(process.env.PORT || 8080);
  server.listen(port, '0.0.0.0', () => process.stdout.write(`Incident Tracker listening on ${port}\n`));
}
