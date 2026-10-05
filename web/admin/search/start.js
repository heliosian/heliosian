import {el} from '/elements.js';
import {signedIn} from '/api.js';
import {chrome, labelOf} from '/chrome.js';

chrome('search');

const form = document.getElementById('ask');
const words = document.getElementById('words');
const summary = document.getElementById('summary');
const button = document.getElementById('run');
const lists = {words: document.getElementById('by-words'), meaning: document.getElementById('by-meaning')};
const took = {words: document.getElementById('words-took'), meaning: document.getElementById('meaning-took')};
const tableOf = {grp: 'GROUP', per: 'PERSON', doc: 'DOCUMENT'};
const sheetOf = {GROUP: 'datagroups', PERSON: 'datapeople', DOCUMENT: 'datadocuments'};

async function rowsOf(hits) {
  const byTable = {};
  for (const h of hits) {
    const table = tableOf[h.id.slice(0, 3)];
    (byTable[table] ??= []).push(h.id);
  }
  const out = {};
  for (const [table, ids] of Object.entries(byTable)) {
    const text = `(from ${table} (where (in id ${ids.map(id => JSON.stringify(id)).join(' ')})))`;
    const res = await signedIn(await fetch('/api/q', {method: 'QUERY', headers: {'Content-Type': 'text/plain'}, body: text}));
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const answer = await res.json();
    Object.assign(out, answer.resources[table] ?? {});
  }
  return out;
}

async function show(kind, hits, started) {
  took[kind].textContent = `${hits.length} in ${Math.round(performance.now() - started)} ms`;
  const rows = await rowsOf(hits);
  lists[kind].replaceChildren(...hits.map(h => {
    const table = tableOf[h.id.slice(0, 3)];
    const item = el('li', '');
    item.dataset.sheet = sheetOf[table];
    const name = el('a', 'name', rows[h.id] ? labelOf(rows[h.id]) : h.id);
    name.href = `/resources#${table}/${h.id}`;
    item.append(name, el('span', 'kind', rows[h.id]?.kind ?? table.toLowerCase()), el('div', 'about', h.summary || 'no entry yet'));
    return item;
  }));
}

async function run() {
  const text = words.value.trim();
  if (!text) {
    return;
  }
  const url = new URL(location.href);
  url.searchParams.set('q', text);
  history.replaceState(null, '', url);
  for (const kind of ['words', 'meaning']) {
    lists[kind].replaceChildren();
    took[kind].textContent = '…';
  }
  summary.textContent = '';
  const started = performance.now();
  const res = await signedIn(await fetch('/api/do/search', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({words: text})}));
  if (!res.ok) {
    summary.textContent = `${res.status}`;
    lists.words.replaceChildren(el('li', 'error', await res.text()));
    return;
  }
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = '';
  for (;;) {
    const {value, done} = await reader.read();
    if (done) {
      break;
    }
    buffer += value;
    let end;
    while ((end = buffer.indexOf('\n\n')) >= 0) {
      const block = buffer.slice(0, end);
      buffer = buffer.slice(end + 2);
      const kind = /^event: (.*)$/m.exec(block)?.[1];
      const data = JSON.parse(/^data: (.*)$/m.exec(block)?.[1] ?? 'null');
      if (kind === 'error') {
        took.meaning.textContent = data.error;
        continue;
      }
      await show(kind, data.result, started);
    }
  }
  summary.textContent = `done in ${Math.round(performance.now() - started)} ms`;
}

form.addEventListener('submit', e => {
  e.preventDefault();
  if (button.ariaBusy === 'true') {
    return;
  }
  button.ariaBusy = 'true';
  run().catch(err => {
    summary.textContent = 'failed';
    lists.words.replaceChildren(el('li', 'error', err.message));
  }).finally(() => {
    button.ariaBusy = 'false';
  });
});

const initial = new URL(location.href).searchParams.get('q');
if (initial) {
  words.value = initial;
  form.requestSubmit();
}
