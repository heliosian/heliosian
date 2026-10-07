import {el} from '/elements.js';
import {api} from '/api.js';
import {chrome} from '/chrome.js';

chrome('queues');

const body = document.querySelector('#queues tbody');
const summary = document.getElementById('summary');
const reload = document.getElementById('reload');

function stamp(d) {
  const two = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())} ${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`;
}

function row(q, part) {
  const tr = el('tr', [part ? 'part' : 'queue', q.pending > 0 ? 'busy' : 'idle'].join(' '));
  const name = el('td', 'name');
  if (q.query) {
    const a = el('a', '', q.name);
    a.href = '/query?q=' + encodeURIComponent(q.query);
    name.append(a);
  } else {
    name.textContent = q.name;
  }
  tr.append(
    name,
    el('td', 'num pending', String(q.pending)),
    el('td', 'num', q.total ? String(q.done) : ''),
    el('td', 'num', q.total ? String(q.total) : ''),
    el('td', 'about', q.about ?? ''),
  );
  return tr;
}

async function show() {
  if (reload.ariaBusy === 'true') {
    return;
  }
  reload.ariaBusy = 'true';
  try {
    const report = await api('GET', '/api/queues');
    body.replaceChildren(...report.queues.flatMap(q => [row(q, false), ...(q.parts ?? []).map(p => row(p, true))]));
    const refreshed = report.lastRefresh && !report.lastRefresh.startsWith('0001') ? stamp(new Date(report.lastRefresh)) : 'none since this server started';
    summary.textContent = `as of ${stamp(new Date())} · last refresh ${refreshed}`;
  } catch (err) {
    summary.replaceChildren(el('span', 'error', err.message));
  } finally {
    reload.ariaBusy = 'false';
  }
}

reload.addEventListener('click', show);
show();
