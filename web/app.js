import {safeName, appendPath, parentPath, folderCrumbs, hostsUrl, sharesUrl, entriesUrl, downloadUrl, readableSize, readableDate} from './paths.mjs';

const el = id => document.getElementById(id);
const ui = Object.fromEntries(['host-list','share-list','host-count','share-count','mesh-summary','location-title','location-subtitle','breadcrumbs','file-list','filter','refresh','item-count'].map(id => [id, el(id)]));
const state = {hosts: [], shares: [], entries: [], host: null, share: null, path: '', loading: false, error: '', query: ''};
let generation = 0;
let sessionToken = null;
const collator = new Intl.Collator(undefined, {numeric:true, sensitivity:'base'});

function node(tag, className, text) {
  const item = document.createElement(tag);
  if (className) item.className = className;
  if (text !== undefined) item.textContent = String(text);
  return item;
}

function apiError(response) {
  if (response.status === 401 || response.status === 403) return 'Access denied. Open the session link from your MasterDrive controller.';
  if (response.status === 404) return 'This drive or folder is no longer available.';
  return `The mesh request failed (HTTP ${response.status}).`;
}

async function json(url, options = {}) {
  const response = await fetch(url, {credentials:'same-origin', cache:'no-store', ...options});
  if (!response.ok) throw new Error(apiError(response));
  return response.json();
}

async function bootstrapSession() {
  const fragment = new URLSearchParams(location.hash.replace(/^#/, ''));
  sessionToken = fragment.get('session');
  if (!sessionToken) return;
  history.replaceState(null, '', location.pathname + location.search);
  const response = await fetch('/api/v1/session', {method:'POST', headers:{'X-MasterDrive-Session':sessionToken}, credentials:'same-origin', cache:'no-store'});
  if (!response.ok) throw new Error('Session pairing failed. Restart MasterDrive and open the new session link.');
  sessionToken = null;
}

function empty(message, detail, icon = '⌁') {
  const container = node('div', 'empty-state');
  const symbol = node('div', 'empty-symbol', icon);
  symbol.setAttribute('aria-hidden', 'true');
  container.append(symbol, node('h3', '', message), node('p', '', detail));
  ui['file-list'].replaceChildren(container);
}

function isFolder(entry) { return entry?.type === 'directory' || entry?.type === 'dir' || entry?.type === 'folder'; }
function labelForHost(host) { return String(host?.name || host?.id || 'Unnamed device'); }

function renderHosts() {
  const list = ui['host-list'];
  list.replaceChildren();
  ui['host-count'].textContent = String(state.hosts.length);
  const online = state.hosts.filter(host => String(host.status).toLowerCase() === 'online').length;
  ui['mesh-summary'].textContent = `${online} online · ${state.hosts.length} devices`;
  if (!state.hosts.length) {
    list.append(node('p', 'sidebar-placeholder', 'No paired devices yet'));
    return;
  }
  for (const host of state.hosts) {
    const button = node('button', 'nav-item');
    button.type = 'button';
    button.setAttribute('aria-current', state.host?.id === host.id ? 'page' : 'false');
    if (state.host?.id === host.id) button.classList.add('active');
    const icon = node('span', 'device-icon', '▣');
    icon.setAttribute('aria-hidden', 'true');
    const names = node('span', 'nav-label');
    names.append(node('strong', '', labelForHost(host)), node('small', '', String(host.status || 'Unknown').toLowerCase()));
    const dot = node('span', `status-dot ${String(host.status).toLowerCase() === 'online' ? 'online' : ''}`);
    dot.setAttribute('aria-hidden', 'true');
    button.append(icon, names, dot);
    button.addEventListener('click', () => chooseHost(host));
    list.append(button);
  }
}

function renderShares() {
  const list = ui['share-list'];
  list.replaceChildren();
  ui['share-count'].textContent = String(state.shares.length);
  if (!state.host) {
    list.append(node('p', 'sidebar-placeholder', 'Choose a device'));
    return;
  }
  if (!state.shares.length) {
    list.append(node('p', 'sidebar-placeholder', 'No shared drives on this device'));
    return;
  }
  for (const share of state.shares) {
    const button = node('button', 'nav-item share-item');
    button.type = 'button';
    button.setAttribute('aria-current', state.share?.id === share.id ? 'page' : 'false');
    if (state.share?.id === share.id) button.classList.add('active');
    const icon = node('span', 'drive-icon', '▰');
    icon.setAttribute('aria-hidden', 'true');
    const names = node('span', 'nav-label');
    names.append(node('strong', '', share.name || share.id), node('small', '', share.read_only === false ? 'Shared drive' : 'Read only'));
    button.append(icon, names);
    button.addEventListener('click', () => chooseShare(share));
    list.append(button);
  }
}

function renderCrumbs() {
  const nav = ui.breadcrumbs;
  nav.replaceChildren();
  if (!state.share) { nav.append(node('span', '', 'All devices')); return; }
  const crumbs = [{name: state.share.name || state.share.id, path:''}, ...folderCrumbs(state.path)];
  crumbs.forEach(({name,path}, i) => {
    if (i) { const slash = node('span', 'crumb-separator', '/'); slash.setAttribute('aria-hidden', 'true'); nav.append(slash); }
    if (i === crumbs.length - 1) {
      const current = node('span', 'crumb-current', name);
      current.setAttribute('aria-current', 'location');
      nav.append(current);
    } else {
      const button = node('button', 'crumb-button', name);
      button.type = 'button';
      button.addEventListener('click', () => openPath(path));
      nav.append(button);
    }
  });
}

function renderHeader() {
  ui['location-title'].textContent = state.share ? (state.path ? state.path.split('/').at(-1) : state.share.name || state.share.id) : (state.host ? labelForHost(state.host) : 'Your storage universe');
  ui['location-subtitle'].textContent = state.share ? `Exploring ${state.share.name || state.share.id} on ${labelForHost(state.host)} · Mesh storage` : (state.host ? 'Choose one of its shared drives to explore.' : 'Select a device and a shared drive to explore files across your mesh.');
  renderCrumbs();
}

function renderFiles() {
  const list = ui['file-list'];
  list.setAttribute('aria-busy', String(state.loading));
  renderHeader();
  if (state.loading) { empty('Connecting to storage…', 'Reading this folder from the selected device.', '◌'); ui['item-count'].textContent = 'Loading…'; return; }
  if (state.error) { empty('Could not load this location', state.error, '!'); ui['item-count'].textContent = 'Request failed'; return; }
  if (!state.share) { empty(state.host ? 'Choose a shared drive' : 'Every drive. One destination.', state.host ? 'Select a drive in the sidebar.' : 'Choose a device from the left to see its shared storage.'); ui['item-count'].textContent = 'No folder selected'; return; }
  const filtered = state.entries.filter(entry => String(entry.name || '').toLocaleLowerCase().includes(state.query.toLocaleLowerCase()));
  filtered.sort((a,b) => Number(isFolder(b)) - Number(isFolder(a)) || collator.compare(a.name, b.name));
  if (!filtered.length) { empty(state.query ? 'No matching files' : 'This folder is empty', state.query ? 'Try a different search in this folder.' : 'There is nothing here yet.', state.query ? '⌕' : '∅'); ui['item-count'].textContent = `${state.entries.length} items`; return; }
  list.replaceChildren();
  for (const entry of filtered) {
    if (!safeName(entry?.name)) continue;
    const path = appendPath(state.path, entry.name);
    const row = node('div', 'file-row');
    const title = node('div', 'file-title');
    const icon = node('span', `file-icon ${isFolder(entry) ? 'folder' : 'file'}`, isFolder(entry) ? '▤' : '▧');
    icon.setAttribute('aria-hidden', 'true');
    const link = isFolder(entry) ? node('button', 'file-link', entry.name) : node('a', 'file-link', entry.name);
    if (isFolder(entry)) { link.type = 'button'; link.addEventListener('click', () => openPath(path)); }
    else { link.href = downloadUrl(state.host.id, state.share.id, path); link.setAttribute('download', entry.name); link.title = `Download ${entry.name}`; }
    title.append(icon, link);
    row.append(title, node('span', 'file-meta', isFolder(entry) ? 'Folder' : readableSize(entry.size)), node('span', 'file-meta', readableDate(entry.modified_at)));
    const action = node('span', 'file-action', isFolder(entry) ? '›' : '↓');
    action.setAttribute('aria-hidden', 'true');
    row.append(action);
    list.append(row);
  }
  ui['item-count'].textContent = `${filtered.length} of ${state.entries.length} items`;
}

async function loadHosts() {
  const sequence = ++generation;
  state.error = '';
  state.loading = true;
  renderFiles();
  try {
    const payload = await json(hostsUrl());
    if (sequence !== generation) return;
    state.hosts = Array.isArray(payload.hosts) ? payload.hosts.filter(h => h && typeof h.id === 'string') : [];
    const oldHost = state.host?.id;
    state.host = state.hosts.find(h => h.id === oldHost) || null;
    renderHosts();
    if (state.host) await chooseHost(state.host);
    else if (state.hosts.length) await chooseHost(state.hosts[0]);
    else { state.share = null; state.shares = []; state.entries = []; state.loading = false; renderShares(); renderFiles(); }
  } catch (error) {
    if (sequence !== generation) return;
    state.loading = false; state.error = error.message || 'Network unavailable';
    renderHosts(); renderFiles();
  }
}

async function chooseHost(host) {
  const sequence = ++generation;
  const previouslySelected = state.host?.id === host.id ? state.share?.id : null;
  state.host = host; state.share = null; state.path = ''; state.entries = []; state.shares = [];
  state.loading = true; state.error = '';
  renderHosts(); renderShares(); renderFiles();
  try {
    const payload = await json(sharesUrl(host.id));
    if (sequence !== generation) return;
    state.shares = Array.isArray(payload.shares) ? payload.shares.filter(s => s && typeof s.id === 'string') : [];
    state.loading = false;
    renderShares(); renderFiles();
    const share = state.shares.find(s => s.id === previouslySelected);
    if (share) await chooseShare(share);
  } catch (error) {
    if (sequence !== generation) return;
    state.loading = false; state.error = error.message || 'Host unavailable'; renderShares(); renderFiles();
  }
}

function chooseShare(share) {
  state.share = share; state.path = ''; state.entries = []; state.query = ''; ui.filter.value = '';
  renderShares(); return openPath('');
}

async function openPath(path) {
  if (!state.host || !state.share) return;
  const sequence = ++generation;
  state.path = path; state.loading = true; state.error = '';
  renderFiles();
  try {
    const payload = await json(entriesUrl(state.host.id, state.share.id, path));
    if (sequence !== generation) return;
    state.entries = Array.isArray(payload.entries) ? payload.entries : [];
    state.loading = false; renderFiles();
  } catch (error) {
    if (sequence !== generation) return;
    state.loading = false; state.error = error.message || 'Folder unavailable'; renderFiles();
  }
}

ui.filter.addEventListener('input', event => { state.query = event.target.value; renderFiles(); });
ui.refresh.addEventListener('click', async () => {
  if (sessionToken) {
    try { await bootstrapSession(); } catch (error) { state.error = error.message; renderFiles(); return; }
  }
  if (state.share) await openPath(state.path);
  else await loadHosts();
});

try { await bootstrapSession(); await loadHosts(); }
catch (error) { state.loading = false; state.error = error.message || 'Unable to connect'; renderFiles(); }
