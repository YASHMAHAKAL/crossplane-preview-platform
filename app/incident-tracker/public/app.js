const state = { incidents: [], status: 'all', severity: 'all', query: '', selectedId: null };
const serviceNames = { 'api-gateway': 'API Gateway', payments: 'Payments', identity: 'Identity', 'incident-tracker': 'Incident Tracker' };
const statuses = ['open', 'investigating', 'resolved'];
const $ = id => document.getElementById(id);
const createDialog = $('create-dialog');
const detailDialog = $('detail-dialog');
let toastTimer;

function el(tag, className, value) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (value !== undefined) node.textContent = value;
  return node;
}
function icon(name) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('class', 'icon');
  svg.setAttribute('aria-hidden', 'true');
  const use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
  use.setAttribute('href', `#i-${name}`);
  svg.append(use);
  return svg;
}
function label(value) { return value[0].toUpperCase() + value.slice(1); }
function relative(iso) {
  const minutes = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 60000));
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}
function toast(message, error = false) {
  const target = $('toast');
  target.textContent = message;
  target.classList.toggle('error', error);
  target.classList.add('show');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => target.classList.remove('show'), 4000);
}
async function api(path, options = {}) {
  const response = await fetch(path, { ...options, headers: { 'content-type': 'application/json', ...options.headers } });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || `Request failed (${response.status})`);
  return result;
}
function filteredIncidents() {
  return state.incidents.filter(item => {
    if (state.status !== 'all' && item.status !== state.status) return false;
    if (state.severity !== 'all' && item.severity !== state.severity) return false;
    const haystack = `${item.key} ${item.title} ${item.description} ${serviceNames[item.service]} ${item.owner}`.toLowerCase();
    return haystack.includes(state.query);
  });
}
function renderMetrics() {
  const active = state.incidents.filter(item => item.status !== 'resolved');
  $('metric-active').textContent = active.length;
  $('metric-critical').textContent = active.filter(item => item.severity === 'critical').length;
  $('metric-investigating').textContent = active.filter(item => item.status === 'investigating').length;
  $('metric-resolved').textContent = state.incidents.filter(item => item.status === 'resolved').length;
  $('nav-active-count').textContent = active.length;
  $('queue-count').textContent = state.incidents.length;
  for (const status of ['all', ...statuses]) $('tab-' + status).textContent = status === 'all' ? state.incidents.length : state.incidents.filter(item => item.status === status).length;
}
function renderIncidents() {
  const list = $('incident-list');
  list.replaceChildren();
  const incidents = filteredIncidents().sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
  if (!incidents.length) {
    const empty = el('div', 'empty-state');
    empty.append(el('strong', '', 'Nothing matches these filters'), el('span', '', 'Try another status, severity, or search term.'));
    list.append(empty);
    return;
  }
  for (const incident of incidents) {
    const row = el('button', 'incident-row');
    row.type = 'button';
    row.setAttribute('aria-label', `View ${incident.key}: ${incident.title}`);
    row.addEventListener('click', () => openDetail(incident.id));
    const marker = el('span', `severity-marker ${incident.severity}`);
    const main = el('span', 'incident-main');
    const titleLine = el('span', 'incident-title-line');
    titleLine.append(el('span', 'incident-title', incident.title), el('span', 'incident-key', incident.key));
    const meta = el('span', 'incident-meta');
    meta.append(el('span', 'meta-service', serviceNames[incident.service] || incident.service), el('span', 'meta-separator'), el('span', '', incident.owner));
    main.append(titleLine, el('span', 'incident-description', incident.description), meta);
    const right = el('span', 'incident-right');
    right.append(el('span', `status-badge ${incident.status}`, label(incident.status)), el('span', 'row-time', relative(incident.updatedAt)));
    row.append(marker, main, right);
    list.append(row);
  }
}
function renderServices() {
  const container = $('service-list');
  container.replaceChildren();
  for (const [key, name] of Object.entries(serviceNames)) {
    const active = state.incidents.filter(item => item.service === key && item.status !== 'resolved');
    const health = active.some(item => item.severity === 'critical') ? 'Disrupted' : active.length ? 'Degraded' : 'Operational';
    const row = el('div', 'service-row');
    row.append(el('span', 'service-icon', name.slice(0, 2).toUpperCase()), el('span', 'service-name', name), el('span', `service-state ${health.toLowerCase()}`, health));
    container.append(row);
  }
}
function renderActivity() {
  const container = $('activity-list');
  container.replaceChildren();
  const events = state.incidents.flatMap(item => (item.activity || []).map(event => ({ ...event, incident: item })));
  events.sort((a, b) => new Date(b.at) - new Date(a.at));
  for (const event of events.slice(0, 4)) {
    const row = el('div', 'activity-item');
    const body = el('div');
    body.append(el('strong', '', `${event.incident.key} · ${event.text}`), el('p', '', `${event.actor} · ${relative(event.at)}`));
    row.append(el('span', 'activity-dot'), body);
    container.append(row);
  }
  if (!events.length) container.append(el('p', 'rail-note', 'No activity yet.'));
}
function render() { renderMetrics(); renderIncidents(); renderServices(); renderActivity(); }
function field(labelText, value) {
  const wrapper = el('div', 'detail-field');
  wrapper.append(el('small', '', labelText), el('strong', '', value));
  return wrapper;
}
function openDetail(id) {
  state.selectedId = id;
  renderDetail();
  if (!detailDialog.open) detailDialog.showModal();
}
function renderDetail() {
  const incident = state.incidents.find(item => item.id === state.selectedId);
  if (!incident) { detailDialog.close(); return; }
  const root = $('detail-content');
  root.replaceChildren();
  const heading = el('div', 'detail-heading');
  const headText = el('div');
  headText.append(el('p', 'eyebrow subdued', incident.key), el('h2', '', incident.title));
  const close = el('button', 'icon-button');
  close.type = 'button'; close.setAttribute('aria-label', 'Close incident details'); close.append(icon('close'));
  close.addEventListener('click', () => detailDialog.close());
  heading.append(headText, close);
  const body = el('div', 'detail-body');
  const tags = el('div', 'detail-tags');
  tags.append(el('span', `severity-badge ${incident.severity}`, `${label(incident.severity)} severity`), el('span', `status-badge ${incident.status}`, label(incident.status)));
  const grid = el('div', 'detail-grid');
  grid.append(field('Affected service', serviceNames[incident.service] || incident.service), field('Owner', incident.owner), field('Created', new Date(incident.createdAt).toLocaleString()), field('Last updated', new Date(incident.updatedAt).toLocaleString()));
  const statusRow = el('div', 'detail-status');
  const statusLabel = el('label', '', 'Incident status');
  const statusSelect = el('select');
  statusSelect.setAttribute('aria-label', 'Incident status');
  for (const status of statuses) { const option = el('option', '', label(status)); option.value = status; statusSelect.append(option); }
  statusSelect.value = incident.status;
  statusSelect.addEventListener('change', async () => {
    try { await api(`/api/incidents/${incident.id}`, { method: 'PATCH', body: JSON.stringify({ status: statusSelect.value }) }); await load(); toast('Incident status updated.'); }
    catch (error) { statusSelect.value = incident.status; toast(error.message, true); }
  });
  statusRow.append(statusLabel, statusSelect);
  const timeline = el('div', 'timeline');
  for (const event of incident.activity || []) {
    const item = el('div', 'timeline-item');
    item.append(el('strong', '', event.text), ...(event.type === 'note' ? [el('p', '', `Note by ${event.actor}`)] : []), el('small', '', `${event.actor} · ${new Date(event.at).toLocaleString()}`));
    timeline.append(item);
  }
  const noteForm = el('form', 'note-form');
  const note = el('textarea'); note.name = 'text'; note.maxLength = 500; note.required = true; note.placeholder = 'Share an update with the team…'; note.setAttribute('aria-label', 'Incident note');
  const submit = el('button', 'button button-primary', 'Add update'); submit.type = 'submit';
  noteForm.append(note, submit);
  noteForm.addEventListener('submit', async event => {
    event.preventDefault();
    try { await api(`/api/incidents/${incident.id}/notes`, { method: 'POST', body: JSON.stringify({ text: note.value }) }); await load(); toast('Update added.'); }
    catch (error) { toast(error.message, true); }
  });
  body.append(tags, el('p', 'detail-description', incident.description), grid, statusRow, el('h3', 'detail-section-title', 'Activity timeline'), timeline, el('h3', 'detail-section-title', 'Add an update'), noteForm);
  root.append(heading, body);
}
async function load() {
  const result = await api('/api/incidents');
  state.incidents = result.incidents;
  render();
  if (detailDialog.open) renderDetail();
}

document.querySelectorAll('[data-open-create]').forEach(button => button.addEventListener('click', () => createDialog.showModal()));
document.querySelectorAll('[data-close-create]').forEach(button => button.addEventListener('click', () => createDialog.close()));
$('create-form').addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget;
  try {
    await api('/api/incidents', { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(form))) });
    form.reset(); createDialog.close(); await load(); toast('Incident created.');
  } catch (error) { toast(error.message, true); }
});
$('search').addEventListener('input', event => { state.query = event.target.value.trim().toLowerCase(); renderIncidents(); });
$('severity-filter').addEventListener('change', event => { state.severity = event.target.value; renderIncidents(); });
document.querySelectorAll('[data-status]').forEach(button => button.addEventListener('click', () => {
  state.status = button.dataset.status;
  document.querySelectorAll('[data-status]').forEach(tab => { tab.classList.toggle('active', tab === button); tab.setAttribute('aria-selected', String(tab === button)); });
  renderIncidents();
}));
$('today-label').textContent = `OPERATIONS / ${new Date().toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' }).toUpperCase()}`;
load().catch(error => toast(error.message, true));
api('/api/context').then(context => { $('preview-context').textContent = context.label; }).catch(() => {});
