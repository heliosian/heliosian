import {el} from '/elements.js';
import {signedIn} from '/api.js';
import {chrome} from '/chrome.js';

chrome('search');

const form = document.getElementById('ask');
const words = document.getElementById('words');
const summary = document.getElementById('summary');
const button = document.getElementById('run');
const lists = {words: document.getElementById('by-words'), meaning: document.getElementById('by-meaning')};
const tables = ['GROUP', 'PERSON', 'DOCUMENT'];
const sheetOf = {GROUP: 'datagroups', PERSON: 'datapeople', DOCUMENT: 'datadocuments'};

function show(kind, results) {
  lists[kind].replaceChildren(...tables.flatMap(table => results[table][kind].map(h => {
    const item = el('li', '');
    item.dataset.sheet = sheetOf[table];
    const name = el('a', 'name', h.name || h.id);
    name.href = `/resources#${table}/${h.id}`;
    item.append(name, el('span', 'kind', table.toLowerCase()), el('div', 'about', h.summary));
    return item;
  })));
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
  }
  summary.textContent = '';
  const started = performance.now();
  const res = await signedIn(await fetch('/api/do/search', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({words: text})}));
  if (!res.ok) {
    summary.textContent = `${res.status}`;
    lists.words.replaceChildren(el('li', 'error', await res.text()));
    return;
  }
  const results = await res.json();
  show('words', results);
  show('meaning', results);
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
