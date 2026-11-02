import test from 'node:test';
import assert from 'node:assert/strict';
import {safeName, appendPath, parentPath, folderCrumbs, sharesUrl, entriesUrl, downloadUrl, readableSize, readableDate} from './paths.mjs';

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
