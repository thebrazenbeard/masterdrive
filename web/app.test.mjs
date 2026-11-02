import test from 'node:test';
import assert from 'node:assert/strict';

// Small DOM contract fixture: exercises actual controller event wiring without
// a browser dependency or filesystem/host access.
class Element {
  constructor() {
    this.children = [];
    this.listeners = new Map();
    this.classList = {add() {}};
    this.style = {};
    this.value = '';
    this.disabled = false;
    this.hidden = false;
    this.textContent = '';
  }
  addEventListener(event, handler) { this.listeners.set(event, handler); }
  setAttribute() {}
  focus() {}
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
}

test('search invalidates in-flight folder pagination so clearing search restores Load more', async () => {
  const elements = new Map();
  globalThis.document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, new Element());
      return elements.get(id);
    },
    createElement() { return new Element(); }
  };
  globalThis.location = {hash:'', pathname:'/', search:''};
  let finishOldPage;
  const host = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
  const result = (body) => ({ok:true, json:async () => body});
  globalThis.fetch = async (url) => {
    if (url === '/api/v1/hosts') return result({hosts:[{id:host, name:'One', status:'online'}]});
    if (url.endsWith('/shares')) return result({shares:[{id:'docs', name:'Documents'}]});
    if (url.includes('/entries?')) {
      const offset = new URL(url, 'http://local.invalid').searchParams.get('offset');
      if (offset === '100') return await new Promise(resolve => { finishOldPage = resolve; });
      return result({entries:[{name:'document.txt', type:'file'}], next_offset:100});
    }
    if (url.startsWith('/api/v1/search?')) return result({
      query:'report', path:'', scope:'immediate_children', complete:true,
      sources:[{host_id:host, share_id:'docs', entries:[{name:'report.txt', path:'report.txt', type:'file', size:8}], scanned:1, next_offset:null, truncated:false}]
    });
    throw new Error(`Unexpected fetch: ${url}`);
  };
  try {
    await import('./app.js?pagination-race-test');
    const shareButton = elements.get('share-list').children[0];
    await shareButton.listeners.get('click')();

    let loadMoreButton = elements.get('file-list').children.at(-1).children[0];
    assert.equal(loadMoreButton.textContent, 'Load more files');
    const staleRequest = loadMoreButton.listeners.get('click')();
    assert.equal(typeof finishOldPage, 'function');
    loadMoreButton = elements.get('file-list').children.at(-1).children[0];
    assert.equal(loadMoreButton.disabled, true);

    elements.get('folder-search').value = 'report';
    elements.get('folder-search-form').listeners.get('submit')({preventDefault() {}});
    await new Promise(resolve => setImmediate(resolve));
    assert.match(elements.get('search-feedback').textContent, /1 matching names/);

    elements.get('folder-search-clear').listeners.get('click')();
    loadMoreButton = elements.get('file-list').children.at(-1).children[0];
    assert.equal(loadMoreButton.textContent, 'Load more files');
    assert.equal(loadMoreButton.disabled, false, 'stale pagination must not leave the button permanently disabled');

    finishOldPage(result({entries:[{name:'stale.txt', type:'file'}], next_offset:null}));
    await staleRequest;
    assert.equal(elements.get('file-list').children.length, 2, 'stale page must not be appended');
  } finally {
    delete globalThis.document;
    delete globalThis.location;
    delete globalThis.fetch;
  }
});
