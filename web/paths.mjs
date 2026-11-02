export function safeName(value) {
  return typeof value === 'string' && value !== '' && value !== '.' && value !== '..' && !/[\\/\0]/.test(value);
}

export function appendPath(path, name) {
  if (!safeName(name)) throw new TypeError('Invalid file or folder name');
  return path ? `${path}/${name}` : name;
}

export function parentPath(path) {
  const index = path.lastIndexOf('/');
  return index < 0 ? '' : path.slice(0, index);
}

export function folderCrumbs(path) {
  if (!path) return [];
  return path.split('/').filter(Boolean).map((name, i, parts) => ({name, path: parts.slice(0, i + 1).join('/')}));
}

const segment = value => encodeURIComponent(String(value));
const folder = (host, share) => `/api/v1/hosts/${segment(host)}/shares/${segment(share)}`;

export function hostsUrl() { return '/api/v1/hosts'; }
export function sharesUrl(host) { return `/api/v1/hosts/${segment(host)}/shares`; }
export function entriesUrl(host, share, path = '', offset = 0, limit = 100) { return `${folder(host, share)}/entries?${new URLSearchParams({path, offset:String(offset), limit:String(limit)})}`; }
export function downloadUrl(host, share, path) { return `${folder(host, share)}/download?${new URLSearchParams({path})}`; }

// A browser search is deliberately scoped to exactly one selected host/share;
// this helper never silently fans out across a device or the entire mesh.
export function searchUrl(host, share, path, query, offset = 0, limit = 25) {
  return `/api/v1/search?${new URLSearchParams({q:query, target:`${host}:${share}`, path, offset:String(offset), limit:String(limit)})}`;
}

// Peer output is untrusted even after controller validation. Confine each
// returned name to the selected immediate folder before rendering links.
export function parseSearchPage(payload, host, share, folderPath, query, offset = 0) {
  const source = payload?.sources?.length === 1 ? payload.sources[0] : null;
  if (payload?.scope !== 'immediate_children' || payload?.query !== query || payload?.path !== folderPath ||
      !source || source.host_id !== host || source.share_id !== share || !Array.isArray(source.entries) ||
      !Number.isSafeInteger(source.scanned) || source.scanned < 0 || source.scanned > 256 ||
      typeof payload.complete !== 'boolean' || typeof source.truncated !== 'boolean') {
    throw new Error('Search returned inconsistent folder results. Refresh and try again.');
  }
  if (source.entries.length > 50 || source.entries.length > source.scanned || source.entries.some(entry =>
      !safeName(entry?.name) || entry.path !== appendPath(folderPath, entry.name))) {
    throw new Error('Search returned names outside this folder. Results were rejected.');
  }
  const next = source.next_offset;
  if (next !== null && (!Number.isSafeInteger(next) || source.scanned === 0 || next <= offset || next !== offset + source.scanned || next > 10000)) {
    throw new Error('Search returned an invalid continuation. Results were rejected.');
  }
  if (source.error && (typeof source.error !== 'string' || source.entries.length || next !== null)) {
    throw new Error('Search returned inconsistent source status.');
  }
  if (payload.complete && (source.error || source.truncated || next !== null)) {
    throw new Error('Search incorrectly marked incomplete results as complete.');
  }
  return {entries:source.entries, scanned:source.scanned, nextOffset:next, truncated:source.truncated, complete:payload.complete, error:source.error || ''};
}

export function readableSize(value) {
  if (!Number.isFinite(value) || value < 0) return '—';
  if (value < 1024) return `${value} B`;
  const units = ['KB','MB','GB','TB','PB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)) - 1, units.length - 1);
  return `${(value / (1024 ** (index + 1))).toFixed(value < 1024 ** (index + 2) * 10 ? 1 : 0)} ${units[index]}`;
}

export function readableDate(value) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? '—' : new Intl.DateTimeFormat(undefined, {month:'short', day:'numeric', year:'numeric'}).format(date);
}
