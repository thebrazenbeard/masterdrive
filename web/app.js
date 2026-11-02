import {safeName, appendPath, parentPath, folderCrumbs, hostsUrl, sharesUrl, entriesUrl, downloadUrl, searchUrl, parseSearchPage, readableSize, readableDate} from './paths.mjs';

const el = id => document.getElementById(id);
const ui = Object.fromEntries(['host-list','share-list','host-count','share-count','mesh-summary','location-title','location-subtitle','breadcrumbs','file-list','filter','refresh','item-count','folder-search-form','folder-search','folder-search-submit','folder-search-clear','search-feedback'].map(id => [id, el(id)]));
const newSearchState = () => ({active: false, term: '', entries: [], loading: false, loadingMore: false, nextOffset: null, scanned: 0, truncated: false, complete: false, error: ''});
const state = {hosts: [], shares: [], entries: [], host: null, share: null, path: '', loading: false, loadingMore: false, nextOffset: null, error: '', query: '', search: newSearchState()};
let generation = 0;
let sessionToken = null;
let searchAbort = null;
const collator = new Intl.Collator(undefined, {numeric:true, sensitivity:'base'});

function node(tag, className, text) {
  const item = document.createElement(tag);
  if (className) item.className = className;
  if (text !== undefined) item.textContent = String(text);
  return item;
}

function apiError(response, payload) {
  const code = payload?.error?.code || payload?.code || payload?.error_code;
  if (response.status === 401 || code === 'session_invalid' || code === 'session_required') return 'Access denied. Open the session link from your MasterDrive controller.';
  if (response.status === 403 && code === 'peer_token_missing') return 'This device needs a peer credential configured on the controller before its files can be accessed.';
  if (response.status === 403) return 'This device denied access. Check its source credential and share permissions.';
  if (response.status === 404) return 'This drive or folder is no longer available.';
  if (response.status === 502 || response.status === 503 || response.status === 504) return 'The selected device is unavailable. Check its connection to the mesh.';
  return `The mesh request failed (HTTP ${response.status}).`;
}

async function json(url, options = {}) {
  const response = await fetch(url, {credentials:'same-origin', cache:'no-store', ...options});
  if (!response.ok) {
    let payload;
    try { payload = await response.json(); } catch { /* A proxy or a device may return non-JSON errors. */ }
    throw new Error(apiError(response, payload));
  }
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

function resetSearch() {
  if (searchAbort) searchAbort.abort();
  searchAbort = null;
  state.search = newSearchState();
  ui['folder-search'].value = '';
}

function renderSearchFeedback() {
  const enabled = !!state.host && !!state.share && !state.loading;
  ui['folder-search'].disabled = !enabled;
  ui['folder-search-submit'].disabled = !enabled || state.search.loading || state.search.loadingMore;
  ui['folder-search-clear'].disabled = !state.search.active || state.loading;
  const s = state.search;
  let message = '';
  if (s.active) {
    if (s.loading) message = `Searching filenames in this folder for “${s.term}”…`;
    else if (s.error) message = `Search incomplete: ${s.error}`;
    else if (s.nextOffset !== null) message = `${s.entries.length} matching names found in ${s.scanned} checked entries. More folder entries may match; choose Load more matches.`;
    else if (s.truncated || !s.complete) message = `${s.entries.length} names found. Results are incomplete; this device may have more names than can be checked.`;
    else message = `${s.entries.length} matching names · ${s.scanned} entries checked · Immediate folder only.`;
  }
  ui['search-feedback'].hidden = !message;
  ui['search-feedback'].textContent = message;
}

function appendFileRow(list, entry, filePath) {
  const folder = isFolder(entry);
  const downloadable = entry?.type === 'file';
  const row = node('div', 'file-row');
  const title = node('div', 'file-title');
  const icon = node('span', `file-icon ${folder ? 'folder' : 'file'}`, folder ? '▤' : '▧');
  icon.setAttribute('aria-hidden', 'true');
  const link = node(folder ? 'button' : downloadable ? 'a' : 'span', 'file-link', entry.name);
  if (folder) { link.type = 'button'; link.addEventListener('click', () => openPath(filePath)); }
  else if (downloadable) { link.href = downloadUrl(state.host.id, state.share.id, filePath); link.target = '_blank'; link.rel = 'noopener'; link.title = `Download ${entry.name}`; }
  title.append(icon, link);
  row.append(title, node('span', 'file-meta', folder ? 'Folder' : downloadable ? readableSize(entry.size) : String(entry.type || 'Other')), node('span', 'file-meta', readableDate(entry.modified_at)));
  const action = node('span', 'file-action', folder ? '›' : downloadable ? '↓' : '—');
  action.setAttribute('aria-hidden', 'true');
  row.append(action);
  list.append(row);
}

function renderHosts() {
  const list = ui['host-list'];
  list.replaceChildren();
  ui['host-count'].textContent = String(state.hosts.length);
  const online = state.hosts.filter(host => String(host.status).toLowerCase() === 'online').length;
  ui['mesh-summary'].textContent = `${online} online · ${state.hosts.length} devices`;
  if (!state.hosts.length) {
    list.append(node('p', 'sidebar-placeholder', 'No registered devices yet'));
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

function renderSearchResults() {
  const list = ui['file-list'];
  const s = state.search;
  list.setAttribute('aria-busy', String(s.loading || s.loadingMore));
  if (s.loading) { empty('Searching this folder…', 'Checking filenames only; no subfolders or file contents are scanned.', '⌕'); ui['item-count'].textContent = 'Searching…'; return; }
  if (s.error && !s.entries.length) { empty('Search could not finish', s.error, '!'); ui['item-count'].textContent = 'Search incomplete'; return; }
  const filtered = s.entries.filter(entry => safeName(entry?.name) && String(entry.name).toLocaleLowerCase().includes(state.query.toLocaleLowerCase()));
  filtered.sort((a,b) => Number(isFolder(b)) - Number(isFolder(a)) || collator.compare(a.name, b.name));
  if (!filtered.length && s.nextOffset === null) {
    empty('No matching names found', s.truncated || !s.complete ? 'Only a bounded portion of this folder was checked. Results may be incomplete.' : 'This checks filenames in the current folder only.', '⌕');
    ui['item-count'].textContent = `${s.scanned} entries checked${s.complete ? '' : ' · incomplete'}`;
    return;
  }
  list.replaceChildren();
  for (const entry of filtered) appendFileRow(list, entry, appendPath(state.path, entry.name));
  if (s.nextOffset !== null) {
    const row = node('div', 'file-row');
    const button = node('button', 'secondary-button', s.loadingMore ? 'Loading more matches…' : 'Load more matches');
    button.type = 'button';
    button.disabled = s.loadingMore;
    button.style.gridColumn = '1 / -1';
    button.addEventListener('click', loadMoreMatches);
    row.append(button);
    list.append(row);
  }
  ui['item-count'].textContent = `${filtered.length} visible matches · ${s.scanned} checked${s.complete ? '' : ' · incomplete'}`;
}

function renderFiles() {
  const list = ui['file-list'];
  list.setAttribute('aria-busy', String(state.loading));
  renderHeader();
  renderSearchFeedback();
  if (state.loading) { empty('Connecting to storage…', 'Reading this folder from the selected device.', '◌'); ui['item-count'].textContent = 'Loading…'; return; }
  if (state.error) { empty('Could not load this location', state.error, '!'); ui['item-count'].textContent = 'Request failed'; return; }
  if (!state.share) { empty(state.host ? 'Choose a shared drive' : 'Every drive. One destination.', state.host ? 'Select a drive in the sidebar.' : 'Choose a device from the left to see its shared storage.'); ui['item-count'].textContent = 'No folder selected'; return; }
  if (state.search.active) { renderSearchResults(); return; }
  const filtered = state.entries.filter(entry => String(entry.name || '').toLocaleLowerCase().includes(state.query.toLocaleLowerCase()));
  filtered.sort((a,b) => Number(isFolder(b)) - Number(isFolder(a)) || collator.compare(a.name, b.name));
  if (!filtered.length && state.nextOffset === null) { empty(state.query ? 'No matching files' : 'This folder is empty', state.query ? 'Try a different search in this folder.' : 'There is nothing here yet.', state.query ? '⌕' : '∅'); ui['item-count'].textContent = `${state.entries.length} items`; return; }
  list.replaceChildren();
  for (const entry of filtered) {
    if (!safeName(entry?.name)) continue;
    const path = appendPath(state.path, entry.name);
    appendFileRow(list, entry, path);
  }
  if (state.nextOffset !== null) {
    const row = node('div', 'file-row');
    const button = node('button', 'secondary-button', state.loadingMore ? 'Loading more…' : 'Load more files');
    button.type = 'button';
    button.disabled = state.loadingMore;
    button.style.gridColumn = '1 / -1';
    button.addEventListener('click', loadMore);
    row.append(button); list.append(row);
  }
  ui['item-count'].textContent = `${filtered.length} matching · ${state.entries.length} loaded${state.nextOffset !== null ? ' · more available' : ''}`;
}

async function loadHosts() {
  const sequence = ++generation;
  const previous = {hostId: state.host?.id, shareId: state.share?.id, path: state.path};
  state.error = '';
  state.loading = true;
  renderFiles();
  try {
    const payload = await json(hostsUrl());
    if (sequence !== generation) return;
    state.hosts = Array.isArray(payload.hosts) ? payload.hosts.filter(h => h && typeof h.id === 'string') : [];
    state.host = state.hosts.find(h => h.id === previous.hostId) || null;
    renderHosts();
    if (state.host) await chooseHost(state.host, previous.shareId, previous.path);
    else if (state.hosts.length) await chooseHost(state.hosts[0]);
    else { state.share = null; state.shares = []; state.entries = []; state.loading = false; renderShares(); renderFiles(); }
  } catch (error) {
    if (sequence !== generation) return;
    state.loading = false; state.error = error.message || 'Network unavailable';
    renderHosts(); renderFiles();
  }
}

async function chooseHost(host, retainedShareId = null, retainedPath = '') {
  const sequence = ++generation;
  const previouslySelected = retainedShareId;
  resetSearch();
  state.host = host; state.share = null; state.path = ''; state.entries = []; state.shares = [];
  state.nextOffset = null; state.loadingMore = false;
  state.loading = true; state.error = '';
  renderHosts(); renderShares(); renderFiles();
  try {
    const payload = await json(sharesUrl(host.id));
    if (sequence !== generation) return;
    state.shares = Array.isArray(payload.shares) ? payload.shares.filter(s => s && typeof s.id === 'string') : [];
    state.loading = false;
    renderShares(); renderFiles();
    const share = state.shares.find(s => s.id === previouslySelected);
    if (share) await chooseShare(share, retainedPath);
  } catch (error) {
    if (sequence !== generation) return;
    state.loading = false; state.error = error.message || 'Host unavailable'; renderShares(); renderFiles();
  }
}

function chooseShare(share, path = '') {
  resetSearch();
  state.share = share; state.path = ''; state.entries = []; state.query = ''; ui.filter.value = '';
  state.nextOffset = null; state.loadingMore = false;
  renderShares(); return openPath(path);
}

async function openPath(path) {
  if (!state.host || !state.share) return;
  const sequence = ++generation;
  resetSearch();
  state.path = path; state.entries = []; state.nextOffset = null; state.loadingMore = false; state.loading = true; state.error = '';
  renderFiles();
  try {
    const payload = await json(entriesUrl(state.host.id, state.share.id, path, 0, 100));
    if (sequence !== generation) return;
    state.entries = Array.isArray(payload.entries) ? payload.entries : [];
    state.nextOffset = Number.isSafeInteger(payload.next_offset) && payload.next_offset > 0 ? payload.next_offset : null;
    state.loading = false; renderFiles();
  } catch (error) {
    if (sequence !== generation) return;
    state.loading = false; state.error = error.message || 'Folder unavailable'; renderFiles();
  }
}

function searchProblem(code) {
  const messages = {
    unknown_host: 'This device is no longer registered. Refresh the device list.',
    peer_token_missing: 'This device needs a peer credential configured on the controller. Restart the controller with its token.',
    peer_unavailable: 'The selected device could not be reached. Check its private connection.',
    peer_error: 'The device rejected the search. Check that its agent supports this search endpoint and that you have access.',
    timeout: 'The search deadline was reached. Try again while the device is available.',
    invalid_peer_response: 'The device returned invalid search data, which was rejected.',
    unexpected_encoding: 'The device returned an unsupported representation.'
  };
  return messages[code] || 'The selected device could not complete the search.';
}

async function runSearch(term) {
  if (!state.host || !state.share || state.loading) return;
  // A failed folder page must not hide the result or error of an independent
  // search request on the same selected share.
  state.error = '';
  // Starting a search invalidates any in-flight folder pagination. Its stale
  // completion intentionally cannot modify state, including loadingMore.
  state.loadingMore = false;
  const byteCount = new TextEncoder().encode(term).length;
  if (byteCount < 2 || byteCount > 128) {
    ++generation;
    if (searchAbort) searchAbort.abort();
    state.search = {...newSearchState(), active: true, term, error: 'Enter a filename query between 2 and 128 UTF-8 bytes.'};
    renderFiles();
    ui['folder-search'].focus();
    return;
  }
  if (searchAbort) searchAbort.abort();
  const controller = new AbortController();
  searchAbort = controller;
  const sequence = ++generation;
  const hostID = state.host.id, shareID = state.share.id, folderPath = state.path;
  state.query = '';
  ui.filter.value = '';
  state.search = {...newSearchState(), active: true, term, loading: true};
  renderFiles();
  try {
    const payload = await json(searchUrl(hostID, shareID, folderPath, term), {signal:controller.signal});
    if (sequence !== generation) return;
    const page = parseSearchPage(payload, hostID, shareID, folderPath, term);
    state.search = {...state.search, entries:page.entries, scanned:page.scanned, nextOffset:page.nextOffset, complete:page.complete, truncated:page.truncated, error:page.error ? searchProblem(page.error) : ''};
  } catch (error) {
    if (sequence !== generation) return;
    state.search.error = error.message || 'Search unavailable. Try again.';
  } finally {
    if (sequence === generation) { state.search.loading = false; searchAbort = null; renderFiles(); }
  }
}

async function loadMoreMatches() {
  const s = state.search;
  if (!s.active || s.loading || s.loadingMore || s.nextOffset === null || !state.host || !state.share) return;
  const sequence = generation, offset = s.nextOffset;
  const hostID = state.host.id, shareID = state.share.id, folderPath = state.path, term = s.term;
  const controller = new AbortController();
  searchAbort = controller;
  s.loadingMore = true;
  renderFiles();
  try {
    const payload = await json(searchUrl(hostID, shareID, folderPath, term, offset), {signal:controller.signal});
    if (sequence !== generation) return;
    const page = parseSearchPage(payload, hostID, shareID, folderPath, term, offset);
    if (page.error) throw new Error(searchProblem(page.error));
    s.entries.push(...page.entries);
    s.scanned += page.scanned;
    s.nextOffset = page.nextOffset;
    s.truncated = page.truncated;
    s.complete = page.complete;
  } catch (error) {
    if (sequence !== generation) return;
    s.error = error.message || 'Could not load more matches. Earlier results are still shown.';
    s.nextOffset = null;
    s.complete = false;
  } finally {
    if (sequence === generation) { s.loadingMore = false; searchAbort = null; renderFiles(); }
  }
}

async function loadMore() {
  if (!state.host || !state.share || state.loading || state.loadingMore || state.nextOffset === null) return;
  const sequence = generation;
  const offset = state.nextOffset;
  state.loadingMore = true;
  renderFiles();
  try {
    const payload = await json(entriesUrl(state.host.id, state.share.id, state.path, offset, 100));
    if (sequence !== generation) return;
    state.entries.push(...(Array.isArray(payload.entries) ? payload.entries : []));
    state.nextOffset = Number.isSafeInteger(payload.next_offset) && payload.next_offset > offset ? payload.next_offset : null;
  } catch (error) {
    if (sequence !== generation) return;
    state.error = error.message || 'Could not load more files';
  } finally {
    if (sequence === generation) { state.loadingMore = false; renderFiles(); }
  }
}

ui.filter.addEventListener('input', event => { state.query = event.target.value; renderFiles(); });
ui['folder-search-form'].addEventListener('submit', event => {
  event.preventDefault();
  runSearch(ui['folder-search'].value.trim());
});
ui['folder-search-clear'].addEventListener('click', () => {
  if (state.loading) return;
  ++generation;
  resetSearch();
  renderFiles();
  ui['folder-search'].focus();
});
ui.refresh.addEventListener('click', async () => {
  const prior = state.search.active && state.search.term ? {term:state.search.term, hostID:state.host?.id, shareID:state.share?.id, path:state.path} : null;
  if (sessionToken) {
    try { await bootstrapSession(); } catch (error) { state.error = error.message; renderFiles(); return; }
  }
  await loadHosts();
  if (prior && !state.error && state.host?.id === prior.hostID && state.share?.id === prior.shareID && state.path === prior.path) {
    ui['folder-search'].value = prior.term;
    await runSearch(prior.term);
  }
});

try { await bootstrapSession(); await loadHosts(); }
catch (error) { state.loading = false; state.error = error.message || 'Unable to connect'; renderFiles(); }
