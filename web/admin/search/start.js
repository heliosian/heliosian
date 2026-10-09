import {el} from '/elements.js';
import {signedIn} from '/api.js';
import {chrome} from '/chrome.js';

chrome('search');

const form = document.getElementById('ask');
const words = document.getElementById('words');
const summary = document.getElementById('summary');
const button = document.getElementById('run');
const list = document.getElementById('results');
const tables = ['GROUP', 'PERSON', 'DOCUMENT'];
const sheetOf = {GROUP: 'datagroups', PERSON: 'datapeople', DOCUMENT: 'datadocuments'};

function link(text, id) {
  const a = el('a', '', text);
  a.href = `/resources#DOCUMENT/${id}`;
  return a;
}

function ref(r) {
  const line = el('div', 'ref', '');
  const name = link(r.name || r.terminal, r.terminal);
  name.className = 'name';
  line.append(name, link(r.source || 'itself', r.document), link('extract', r.extract), el('span', 'score', r.score), el('div', 'about', r.summary));
  return line;
}

function show(results) {
  list.replaceChildren(...tables.flatMap(table => results[table].map(r => {
    const item = el('li', '');
    item.dataset.sheet = sheetOf[table];
    if (r.refs) {
      item.append(el('span', 'kind', table.toLowerCase()), ...r.refs.map(ref));
      return item;
    }
    const name = el('a', 'name', r.name || r.id);
    name.href = `/resources#${table}/${r.id}`;
    item.append(name, el('span', 'kind', table.toLowerCase()), el('span', 'score', r.score), el('div', 'about', r.summary));
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
  list.replaceChildren();
  summary.textContent = '';
  const started = performance.now();
  const res = await signedIn(await fetch('/api/do/search', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({words: text})}));
  if (!res.ok) {
    summary.textContent = `${res.status}`;
    list.replaceChildren(el('li', 'error', await res.text()));
    return;
  }
  show(await res.json());
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
    list.replaceChildren(el('li', 'error', err.message));
  }).finally(() => {
    button.ariaBusy = 'false';
  });
});

const initial = new URL(location.href).searchParams.get('q');
if (initial) {
  words.value = initial;
  form.requestSubmit();
}
