import {el} from '/elements.js';
import {signedIn} from '/api.js';
import {chrome} from '/chrome.js';
import {byName, grid} from '/grid.js';

chrome('query');

const src = document.getElementById('src');
const out = document.getElementById('out');
const filter = document.getElementById('filter');
const summary = document.getElementById('summary');
const open = document.getElementById('open');
const close = document.getElementById('close');
const button = document.getElementById('run');
const words = document.getElementById('words');
const compose = document.getElementById('compose');
const understood = document.getElementById('understood');

let grids = [];

function refresh() {
  const text = filter.value.trim().toLowerCase();
  for (const g of grids) {
    g.filterBy(text);
  }
}

filter.addEventListener('input', refresh);
for (const [button, expand] of [[open, true], [close, false]]) {
  button.addEventListener('click', () => {
    for (const g of grids) {
      g.setAll(expand);
    }
    refresh();
  });
}

function section(title, table, answer, ids) {
  const g = grid(table, answer, ids, refresh);
  grids.push(g);
  const box = el('section', 'answer');
  box.dataset.sheet = table.sheet;
  box.append(el('h3', '', title), g.wrap);
  return box;
}

function showError(message) {
  out.replaceChildren(el('p', 'error', message));
  const at = /^at (\d+):/.exec(message);
  if (at) {
    const offset = Number(at[1]) - 1;
    src.focus();
    src.setSelectionRange(offset, offset + 1);
  }
}

async function run() {
  const text = src.value;
  if (!text.trim() || button.ariaBusy === 'true') {
    return;
  }
  button.ariaBusy = 'true';
  try {
    await runText(text);
  } finally {
    button.ariaBusy = 'false';
  }
}

async function runText(text) {
  const url = new URL(location.href);
  url.searchParams.set('q', text);
  history.replaceState(null, '', url);
  summary.textContent = '';
  const started = performance.now();
  const res = await signedIn(await fetch('/api/q', {method: 'QUERY', headers: {'Content-Type': 'text/plain'}, body: text}));
  const took = Math.round(performance.now() - started);
  if (!res.ok) {
    summary.textContent = `${res.status} in ${took} ms`;
    showError(await res.text());
    return;
  }
  const answer = await res.json();
  grids = [];
  const main = answer.query.match(/^\(from\s+([A-Z_]+)/)[1];
  const parts = [el('pre', 'canonical', answer.query)];
  parts.push(section(`${main} · ${answer.result.length} answered`, byName.get(main), answer, answer.result));
  const answered = new Set(answer.result);
  for (const [name, rows] of Object.entries(answer.resources)) {
    const ids = Object.keys(rows).filter(id => name !== main || !answered.has(id));
    if (ids.length) {
      parts.push(section(`${name} · ${ids.length} included`, byName.get(name), answer, ids));
    }
  }
  out.replaceChildren(...parts);
  open.hidden = close.hidden = !grids.some(g => g.tree);
  summary.textContent = `${answer.result.length} rows in ${took} ms · now ${answer.now}`;
  refresh();
}

async function write() {
  const text = words.value.trim();
  if (!text || compose.ariaBusy === 'true') {
    return;
  }
  compose.ariaBusy = 'true';
  understood.hidden = true;
  try {
    const res = await signedIn(await fetch('/api/do/compose', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({words: text})}));
    if (!res.ok) {
      understood.className = 'understood error';
      understood.textContent = await res.text();
      understood.hidden = false;
      return;
    }
    const composed = await res.json();
    understood.className = composed.query ? 'understood' : 'understood error';
    understood.textContent = composed.understanding;
    understood.hidden = false;
    if (!composed.query) {
      return;
    }
    src.value = composed.query;
  } finally {
    compose.ariaBusy = 'false';
  }
  await run();
}

compose.addEventListener('click', write);
words.addEventListener('keydown', e => {
  if (e.key === 'Enter') {
    e.preventDefault();
    write();
  }
});

button.addEventListener('click', run);
src.addEventListener('keydown', e => {
  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault();
    run();
  }
});

const initial = new URL(location.href).searchParams.get('q');
if (initial) {
  src.value = initial;
  run();
}
