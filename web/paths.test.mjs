import test from 'node:test';
import assert from 'node:assert/strict';
import {safeName, appendPath, parentPath, folderCrumbs, sharesUrl, entriesUrl, downloadUrl, searchUrl, parseSearchPage, readableSize, readableDate} from './paths.mjs';

test('navigation preserves nested paths and parent traversal stays within the logical share', () => {
  assert.equal(appendPath('', 'Photos'), 'Photos');
  assert.equal(appendPath('Photos', '2026'), 'Photos/2026');
  assert.equal(parentPath('Photos/2026'), 'Photos');
  assert.equal(parentPath('Photos'), '');
  assert.deepEqual(folderCrumbs('a/b'), [{name:'a', path:'a'}, {name:'b', path:'a/b'}]);
});

test('untrusted entry names cannot construct traversal paths', () => {
  for (const name of ['', '.', '..', '../private', 'a/b', 'a\\b', 'x\0y']) {
    assert.equal(safeName(name), false);
    assert.throws(() => appendPath('shared', name), TypeError);
  }
  assert.equal(safeName('report<script>.txt'), true);
});

test('each URL segment and query parameter is separately encoded', () => {
  assert.equal(sharesUrl('host/one'), '/api/v1/hosts/host%2Fone/shares');
  assert.equal(entriesUrl('a b', 'disk#1', 'docs/report & notes.txt'), '/api/v1/hosts/a%20b/shares/disk%231/entries?path=docs%2Freport+%26+notes.txt&offset=0&limit=100');
  assert.equal(entriesUrl('a', 'b', '', 100, 50), '/api/v1/hosts/a/shares/b/entries?path=&offset=100&limit=50');
  assert.equal(downloadUrl('a', 'b', 'x?y#z'), '/api/v1/hosts/a/shares/b/download?path=x%3Fy%23z');
});

test('formatting rejects missing or malformed metadata without throwing', () => {
  assert.equal(readableSize(0), '0 B');
  assert.equal(readableSize(1024), '1.0 KB');
  assert.equal(readableSize(-1), '—');
  assert.equal(readableDate('nope'), '—');
});

test('filename search is explicitly scoped to one host/share and selected immediate folder', () => {
  assert.equal(searchUrl('0123456789abcdef', 'photos', '2026/trips', 'sun & sea'),
    '/api/v1/search?q=sun+%26+sea&target=0123456789abcdef%3Aphotos&path=2026%2Ftrips&offset=0&limit=25');
  assert.equal(searchUrl('host', 'docs', '', 'report', 256, 50),
    '/api/v1/search?q=report&target=host%3Adocs&path=&offset=256&limit=50');
});

test('accepts matching immediate-folder names and bounded search continuation', () => {
  const payload = {query:'report', path:'reports', scope:'immediate_children', complete:false, sources:[{
    host_id:'host', share_id:'docs', scanned:3, next_offset:3, truncated:false,
    entries:[{name:'report.txt', path:'reports/report.txt', type:'file', size:12}]
  }]};
  assert.deepEqual(parseSearchPage(payload, 'host', 'docs', 'reports', 'report'), {
    entries:payload.sources[0].entries, scanned:3, nextOffset:3, truncated:false, complete:false, error:''
  });
});

test('rejects forged, inconsistent, or out-of-scope search results', () => {
  const good = {query:'report', path:'reports', scope:'immediate_children', complete:false, sources:[{
    host_id:'host', share_id:'docs', scanned:1, next_offset:null, truncated:false,
    entries:[{name:'report.txt', path:'reports/report.txt', type:'file'}]
  }]};
  const check = payload => parseSearchPage(payload, 'host', 'docs', 'reports', 'report');
  for (const [key, mutation] of [
    ['other host', p => { p.sources[0].host_id = 'other'; }],
    ['other folder', p => { p.sources[0].entries[0].path = 'private/report.txt'; }],
    ['traversal name', p => { p.sources[0].entries[0].name = '../secret'; }],
    ['too many matches', p => { p.sources[0].scanned = 0; }],
    ['fabricated continuation', p => { p.sources[0].next_offset = 7; }],
    ['zero-progress continuation', p => { p.sources[0].scanned = 0; p.sources[0].entries = []; p.sources[0].next_offset = 0; }],
    ['false completion', p => { p.complete = true; p.sources[0].next_offset = 1; }],
  ]) {
    const payload = structuredClone(good);
    mutation(payload);
    assert.throws(() => check(payload), Error, key);
  }
});
