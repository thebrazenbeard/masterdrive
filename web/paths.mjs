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
export function entriesUrl(host, share, path = '') { return `${folder(host, share)}/entries?${new URLSearchParams({path})}`; }
export function downloadUrl(host, share, path) { return `${folder(host, share)}/download?${new URLSearchParams({path})}`; }

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
