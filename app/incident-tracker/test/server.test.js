import test from 'node:test';
import assert from 'node:assert/strict';
import { Readable } from 'node:stream';
import { createIncidentServer } from '../src/server.js';

function request(server, method, path, body) {
  return new Promise((resolve, reject) => {
    const req = Readable.from(body === undefined ? [] : [JSON.stringify(body)]);
    req.method = method;
    req.url = path;
    const res = {
      status: null,
      writeHead(status) { this.status = status; },
      end(payload) {
        try { resolve({ status: this.status, body: JSON.parse(payload) }); }
        catch (error) { reject(error); }
      },
    };
    server.emit('request', req, res);
  });
}

test('creates, updates, and annotates an incident for the preview', async () => {
  let sequence = 0;
  const server = createIncidentServer({ id: () => `11111111-1111-1111-1111-${String(++sequence).padStart(12, '0')}`, previewMode: 'vcluster' });
  const initial = await request(server, 'GET', '/api/incidents');
  assert.equal(initial.body.incidents.length, 4);
  const created = await request(server, 'POST', '/api/incidents', {
    title: 'Slow API', description: 'p95 is high', service: 'api-gateway', severity: 'high', owner: 'Sam',
  });
  assert.equal(created.status, 201);
  assert.equal(created.body.incident.status, 'open');
  assert.equal(created.body.incident.key, 'INC-1043');
  assert.equal(created.body.incident.service, 'api-gateway');
  const note = await request(server, 'POST', `/api/incidents/${created.body.incident.id}/notes`, { text: 'Investigating upstream latency.' });
  assert.equal(note.status, 201);
  assert.equal(note.body.incident.activity[0].type, 'note');
  const updated = await request(server, 'PATCH', `/api/incidents/${created.body.incident.id}`, { status: 'resolved' });
  assert.equal(updated.status, 200);
  const listed = await request(server, 'GET', '/api/incidents');
  assert.equal(listed.body.incidents[0].status, 'resolved');
  assert.equal(listed.body.incidents[0].activity.length, 3);
  assert.deepEqual((await request(server, 'GET', '/healthz')).body, { status: 'ok' });
  assert.deepEqual((await request(server, 'GET', '/api/context')).body, { mode: 'vcluster', label: 'vCluster preview' });
});

test('rejects invalid inputs', async () => {
  const server = createIncidentServer({ seed: false });
  assert.equal((await request(server, 'POST', '/api/incidents', { title: '' })).status, 400);
  assert.equal((await request(server, 'POST', '/api/incidents', {
    title: 'Bad service', description: 'Test', service: 'unknown', severity: 'high', owner: 'Sam',
  })).status, 400);
  assert.equal((await request(server, 'PATCH', '/api/incidents/11111111-1111-1111-1111-111111111111', { status: 'gone' })).status, 404);
});
