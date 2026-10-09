'use strict';

const state = {
  view: 'folders',        // folders | trash
  data: null,             // latest scan report (partial while a scan runs)
  scan: { running: false, done: 0, total: 0, error: '' },
  trash: null,            // { items, retention_days }
  filter: 'all',          // all | orphan | linked
  search: '',
  sort: { key: 'status', dir: 1 },
  selected: new Set(),    // folder names (orphans only)
  busy: false,
};

const $ = (id) => document.getElementById(id);

// ---------- helpers ----------

function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function formatBytes(n) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
  return (i === 0 ? n : n.toFixed(n < 10 ? 1 : 0)) + ' ' + units[i];
}

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'short' });
const ZERO_TIME = '0001-01-01T00:00:00Z';

// count(3, 'folder') === '3 folders'
function count(n, noun) {
  return n + ' ' + noun + (n === 1 ? '' : 's');
}

function formatDate(iso) {
  return iso && iso !== ZERO_TIME ? dateFormat.format(new Date(iso)) : '—';
}

async function api(url, options) {
  const res = await fetch(url, options);
  if (res.status === 401) {
    location.replace('/login.html');
    throw new Error('Session expired');
  }
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.status + ' ' + res.statusText);
  return body;
}

function postJSON(url, body) {
  return api(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

let toastTimer;
function toast(message, kind = 'success') {
  const el = $('toast');
  el.textContent = message;
  el.className = 'toast ' + kind;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, kind === 'error' ? 8000 : 4000);
}

// Runs a batch API call and reports per-item results.
// label(name) maps a result name (folder name or trash id) to a display name.
async function runBatch(url, body, doneMsg, label = (n) => n) {
  state.busy = true;
  try {
    const { results } = await postJSON(url, body);
    const failed = results.filter((r) => r.error);
    const ok = results.length - failed.length;
    if (failed.length) {
      toast(ok + ' ' + doneMsg + ', ' + failed.length + ' failed:\n'
        + failed.map((r) => '• ' + label(r.name) + ': ' + r.error).join('\n'), 'error');
    } else {
      toast(count(ok, 'folder') + ' ' + doneMsg + '.');
    }
    return results.filter((r) => !r.error).map((r) => r.name);
  } catch (e) {
    toast('Error: ' + e.message, 'error');
    return [];
  } finally {
    state.busy = false;
  }
}

// ---------- data ----------

function visibleFolders() {
  const q = state.search.trim().toLowerCase();
  const folders = state.data.folders.filter((f) => {
    if (state.filter === 'orphan' && !f.orphan) return false;
    if (state.filter === 'linked' && f.orphan) return false;
    if (q && !f.name.toLowerCase().includes(q)
        && !(f.containers || []).some((c) => c.toLowerCase().includes(q))) return false;
    return true;
  });

  const { key, dir } = state.sort;
  const byName = (a, b) => a.name.localeCompare(b.name);
  const comparators = {
    name: byName,
    size: (a, b) => a.size - b.size || byName(a, b),
    status: (a, b) => (b.orphan - a.orphan) || byName(a, b),
  };
  return folders.sort((a, b) => dir * comparators[key](a, b));
}

// The server scans in the background; we poll its status while it runs.
const POLL_MS = 700;
let pollTimer;

async function startScan() {
  try {
    applyStatus(await postJSON('/api/scan', {}));
  } catch (e) {
    toast('Error: ' + e.message, 'error');
  }
  poll();
}

async function poll() {
  clearTimeout(pollTimer);
  let st;
  try {
    st = await api('/api/scan');
  } catch (e) {
    st = { running: false, report: state.data, error: e.message };
  }
  const finished = state.scan.running && !st.running;
  applyStatus(st);
  if (st.running) pollTimer = setTimeout(poll, POLL_MS);
  else if (finished) loadTrash();
}

function applyStatus(st) {
  state.scan = { running: st.running, done: st.done || 0, total: st.total || 0, error: st.error || '' };
  state.data = st.report;
  if (state.data) {
    // Drop selections that no longer exist or are no longer orphans.
    const orphans = new Set(state.data.folders.filter((f) => f.orphan).map((f) => f.name));
    state.selected = new Set([...state.selected].filter((n) => orphans.has(n)));
  }
  renderScanStatus();
  if (state.view === 'folders') render();
}

function renderScanStatus() {
  const { running, done, total, error } = state.scan;
  $('scanBtn').disabled = running;
  $('scanBtn').textContent = running ? 'Scanning… ' + (total ? done + '/' + total : '') : '⟳ Scan';
  $('scanProgress').hidden = !running;
  $('scanProgressBar').style.width = (total ? (100 * done) / total : 0) + '%';

  const info = $('scanInfo');
  info.classList.toggle('error', !!error);
  if (error) info.textContent = 'Last scan failed: ' + error;
  else if (running) info.textContent = 'Computing sizes… ' + done + '/' + total;
  else if (state.data) info.textContent = 'Scanned ' + formatDate(state.data.scanned_at);
  else info.textContent = '';
}

async function loadTrash() {
  try {
    state.trash = await api('/api/trash');
  } catch (e) {
    state.trash = null;
    if (state.view === 'trash') {
      $('content').innerHTML = '<div class="error-box">Error: ' + esc(e.message) + '</div>';
    }
    return;
  }
  const n = state.trash.items.length;
  $('trashCount').textContent = n;
  $('trashCount').hidden = n === 0;
  if (state.view === 'trash') renderTrash();
}

async function trashFolders(names) {
  const done = await runBatch('/api/delete', { names }, 'moved to trash');
  done.forEach((n) => state.selected.delete(n));
  poll(); // the server drops trashed folders from its report, no rescan needed
  loadTrash();
}

function trashLabel(id) {
  const item = state.trash && state.trash.items.find((i) => i.id === id);
  return item ? item.name : id;
}

async function restoreTrash(ids) {
  await runBatch('/api/trash/restore', { ids }, 'restored', trashLabel);
  poll(); // the server rescans to get the usage and size of restored folders
  loadTrash();
}

async function purgeTrash(ids) {
  await runBatch('/api/trash/purge', { ids }, 'permanently deleted', trashLabel);
  loadTrash();
}

// ---------- views ----------

function showView(view) {
  state.view = view;
  const folders = view === 'folders';
  $('trashBar').hidden = folders;
  $('statsBar').hidden = $('controlsBar').hidden = !folders || !state.data;
  $('bulkBar').hidden = !folders || state.selected.size === 0;
  if (folders) {
    render();
  } else {
    $('content').innerHTML = '<div class="loading"><div class="spinner"></div>Loading…</div>';
    loadTrash();
  }
}

// ---------- folders view ----------

function render() {
  const d = state.data;
  if (!d) {
    $('statsBar').hidden = $('controlsBar').hidden = $('bulkBar').hidden = true;
    $('content').innerHTML = state.scan.error && !state.scan.running
      ? '<div class="error-box">Error: ' + esc(state.scan.error) + '</div>'
      : '<div class="loading"><div class="spinner"></div>Scanning…</div>';
    return;
  }

  $('pathBadge').textContent = d.root;
  $('statsBar').hidden = false;
  $('controlsBar').hidden = false;
  const approx = state.scan.running ? '≈ ' : '';
  $('statTotal').textContent = d.folders.length;
  $('statTotalSize').textContent = approx + formatBytes(d.total_size) + ' total';
  $('statUsed').textContent = d.used_count;
  $('statOrphan').textContent = d.orphan_count;
  $('statOrphanSize').textContent = approx + formatBytes(d.orphan_size);

  renderTable();
  renderBulkBar();
}

function sortHeader(key, label) {
  const arrow = state.sort.key === key ? ' <span class="arrow">' + (state.sort.dir > 0 ? '▲' : '▼') + '</span>' : '';
  return '<th data-sort="' + key + '">' + label + arrow + '</th>';
}

function renderTable() {
  const folders = visibleFolders();
  if (folders.length === 0) {
    $('content').innerHTML = '<div class="empty-state">No folders match these filters.</div>';
    return;
  }

  const rows = folders.map((f) => {
    const name = esc(f.name);
    const select = f.orphan
      ? '<input type="checkbox" data-select="' + name + '"' + (state.selected.has(f.name) ? ' checked' : '') + '>'
      : '';
    const status = f.orphan
      ? '<span class="status-badge orphan">● Orphan</span>'
      : '<span class="status-badge linked">● In use</span>';
    const containers = f.orphan
      ? '<span class="muted">—</span>'
      : f.containers.map((c) => '<span class="container-tag">' + esc(c) + '</span>').join('');
    const action = f.orphan
      ? '<button class="btn btn-danger btn-sm" data-delete="' + name + '">Trash</button>'
      : '';

    return '<tr class="' + (f.orphan ? 'orphan' : '') + '">'
      + '<td class="select-col">' + select + '</td>'
      + '<td><span class="folder-name">' + name + '</span></td>'
      + '<td>' + sizeCell(f) + '</td>'
      + '<td>' + status + '</td>'
      + '<td>' + containers + '</td>'
      + '<td>' + action + '</td>'
      + '</tr>';
  });

  $('content').innerHTML = '<div class="table-wrap"><table><thead><tr>'
    + '<th class="select-col"><input type="checkbox" id="selectAll" title="Select the orphans shown"></th>'
    + sortHeader('name', 'Folder') + sortHeader('size', 'Size') + sortHeader('status', 'Status')
    + '<th>Containers</th><th></th>'
    + '</tr></thead><tbody>' + rows.join('') + '</tbody></table></div>';

  syncSelectAll(folders);
}

function sizeCell(f) {
  if (!f.size_pending) return '<span class="folder-size">' + formatBytes(f.size) + '</span>';
  const text = f.size ? formatBytes(f.size) : '…';
  return '<span class="folder-size pending" title="Computing…">' + text + '</span>';
}

function syncSelectAll(folders = visibleFolders()) {
  const cb = $('selectAll');
  if (!cb) return;
  const orphans = folders.filter((f) => f.orphan);
  const count = orphans.filter((f) => state.selected.has(f.name)).length;
  cb.disabled = orphans.length === 0;
  cb.checked = orphans.length > 0 && count === orphans.length;
  cb.indeterminate = count > 0 && count < orphans.length;
}

function renderBulkBar() {
  const n = state.selected.size;
  $('bulkBar').hidden = n === 0 || state.view !== 'folders';
  $('bulkCount').textContent = n + ' selected';
}

function retentionText() {
  const days = state.trash ? state.trash.retention_days : 0;
  return days > 0
    ? 'Permanently deleted after ' + count(days, 'day') + '.'
    : 'Kept until the trash is emptied.';
}

function confirmTrash(names) {
  if (names.length === 0) return;
  const size = state.data.folders
    .filter((f) => names.includes(f.name))
    .reduce((sum, f) => sum + f.size, 0);
  const list = names.map((n) => '<code>' + esc(n) + '</code>').join(', ');
  const what = names.length === 1 ? list : '<strong>' + names.length + ' folders</strong>';
  openModal({
    title: 'Move to trash',
    html: '<p>Move ' + what + ' (' + formatBytes(size) + ') to the trash?</p>'
      + (names.length > 1 ? '<p>' + list + '</p>' : '')
      + '<p>You can restore them from the trash. ' + retentionText() + '</p>',
    confirmLabel: 'Move to trash',
    onConfirm: () => trashFolders(names),
  });
}

// ---------- trash view ----------

function renderTrash() {
  const { items } = state.trash;
  const total = items.reduce((sum, i) => sum + i.size, 0);
  $('trashInfo').textContent = items.length
    ? count(items.length, 'folder') + ', ' + formatBytes(total) + '. ' + retentionText()
    : retentionText();
  $('emptyTrashBtn').disabled = items.length === 0;

  if (items.length === 0) {
    $('content').innerHTML = '<div class="empty-state">The trash is empty.</div>';
    return;
  }

  const rows = items.map((i) => {
    const id = esc(i.id);
    return '<tr>'
      + '<td><span class="folder-name">' + esc(i.name) + '</span></td>'
      + '<td><span class="folder-size">' + formatDate(i.deleted_at) + '</span></td>'
      + '<td><span class="folder-size">' + formatBytes(i.size) + '</span></td>'
      + '<td><div class="row-actions">'
      + '<button class="btn btn-ghost btn-sm" data-restore="' + id + '">Restore</button>'
      + '<button class="btn btn-danger btn-sm" data-purge="' + id + '">Delete</button>'
      + '</div></td>'
      + '</tr>';
  });

  $('content').innerHTML = '<div class="table-wrap"><table><thead><tr>'
    + '<th>Folder</th><th>Deleted</th><th>Size</th><th></th>'
    + '</tr></thead><tbody>' + rows.join('') + '</tbody></table></div>';
}

function confirmPurge(ids) {
  if (ids.length === 0) return;
  const items = state.trash.items.filter((i) => ids.includes(i.id));
  const size = items.reduce((sum, i) => sum + i.size, 0);
  const what = items.length === 1
    ? '<code>' + esc(items[0].name) + '</code>'
    : '<strong>' + items.length + ' folders</strong>';
  openModal({
    title: 'Delete permanently',
    html: '<p>Permanently delete ' + what + ' (' + formatBytes(size) + ')?</p>'
      + '<p>This cannot be undone.</p>',
    confirmLabel: 'Delete permanently',
    onConfirm: () => purgeTrash(ids),
  });
}

// ---------- modal ----------

let pendingConfirm = null;

function openModal({ title, html, confirmLabel, onConfirm }) {
  $('modalTitle').textContent = title;
  $('modalBody').innerHTML = html;
  $('modalConfirm').textContent = confirmLabel;
  pendingConfirm = onConfirm;
  $('modalOverlay').hidden = false;
  $('modalCancel').focus();
}

function hideModal() {
  $('modalOverlay').hidden = true;
  pendingConfirm = null;
}

// ---------- events ----------

$('scanBtn').addEventListener('click', () => {
  if (state.view !== 'folders') showView('folders');
  startScan();
});
$('trashBtn').addEventListener('click', () => showView('trash'));
$('backBtn').addEventListener('click', () => showView('folders'));
$('emptyTrashBtn').addEventListener('click', () => confirmPurge(state.trash.items.map((i) => i.id)));

$('logoutBtn').addEventListener('click', async () => {
  await fetch('/api/logout', { method: 'POST' }).catch(() => {});
  location.replace('/login.html');
});

$('controlsBar').addEventListener('click', (e) => {
  const btn = e.target.closest('[data-filter]');
  if (!btn) return;
  state.filter = btn.dataset.filter;
  document.querySelectorAll('.filter-btn').forEach((b) => b.classList.toggle('active', b === btn));
  renderTable();
});

$('searchInput').addEventListener('input', (e) => {
  state.search = e.target.value;
  renderTable();
});

$('content').addEventListener('click', (e) => {
  const th = e.target.closest('th[data-sort]');
  if (th) {
    const key = th.dataset.sort;
    state.sort = state.sort.key === key
      ? { key, dir: -state.sort.dir }
      : { key, dir: key === 'size' ? -1 : 1 };
    renderTable();
    return;
  }
  const btn = e.target.closest('[data-delete], [data-restore], [data-purge]');
  if (!btn || state.busy) return;
  if (btn.dataset.delete !== undefined) confirmTrash([btn.dataset.delete]);
  else if (btn.dataset.restore !== undefined) restoreTrash([btn.dataset.restore]);
  else confirmPurge([btn.dataset.purge]);
});

$('content').addEventListener('change', (e) => {
  const cb = e.target;
  if (cb.id === 'selectAll') {
    visibleFolders()
      .filter((f) => f.orphan)
      .forEach((f) => (cb.checked ? state.selected.add(f.name) : state.selected.delete(f.name)));
    renderTable();
  } else if (cb.dataset.select !== undefined) {
    if (cb.checked) state.selected.add(cb.dataset.select);
    else state.selected.delete(cb.dataset.select);
    syncSelectAll();
  }
  renderBulkBar();
});

$('bulkDeleteBtn').addEventListener('click', () => confirmTrash([...state.selected].sort()));
$('clearSelectionBtn').addEventListener('click', () => {
  state.selected.clear();
  renderTable();
  renderBulkBar();
});

$('modalCancel').addEventListener('click', hideModal);
$('modalConfirm').addEventListener('click', () => {
  const action = pendingConfirm;
  hideModal();
  if (action) action();
});
$('modalOverlay').addEventListener('click', (e) => { if (e.target === e.currentTarget) hideModal(); });
document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && !$('modalOverlay').hidden) hideModal(); });

api('/api/session')
  .then((session) => { $('logoutBtn').hidden = !session.auth; })
  .catch(() => {});
render();
// The server scans at startup; only start one if it has nothing yet.
api('/api/scan')
  .then((st) => (st.report || st.running || st.error ? poll() : startScan()))
  .catch((e) => applyStatus({ running: false, report: null, error: e.message }));
loadTrash();
