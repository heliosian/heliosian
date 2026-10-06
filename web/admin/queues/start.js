import {el} from '/elements.js';
import {api} from '/api.js';
import {chrome} from '/chrome.js';

chrome('queues');

const body = document.querySelector('#queues tbody');
const summary = document.getElementById('summary');
const reload = document.getElementById('reload');

function row(q) {
  const tr = el('tr', q.waiting > 0 ? 'busy' : 'idle');
  const name = el('td', 'name');
  if (q.query) {
    const a = el('a', '', q.name);
    a.href = '/query?q=' + encodeURIComponent(q.query);
    name.append(a);
  } else {
    name.textContent = q.name;
  }
  const done = el('td', 'num', q.total ? `${q.done} / ${q.total}` : '');
  const bar = el('td', 'bar');
  if (q.total) {
    const fill = el('span', 'fill');
    fill.style.width = `${(100 * q.done / q.total).toFixed(1)}%`;
    bar.append(el('span', 'track'));
    bar.firstChild.append(fill);
  }
  tr.append(name, el('td', 'num', String(q.waiting)), done, bar, el('td', 'about', q.about));
  return tr;
}

async function show() {
  reload.disabled = true;
  try {
    const report = await api('GET', '/api/queues');
    body.replaceChildren(...report.queues.map(row));
    const refreshed = report.lastRefresh && !report.lastRefresh.startsWith('0001') ? new Date(report.lastRefresh).toLocaleString() : 'not since start';
    summary.textContent = `as of ${new Date().toLocaleTimeString()} · last refresh ${refreshed}`;
  } catch (err) {
    summary.replaceChildren(el('span', 'error', err.message));
  } finally {
    reload.disabled = false;
  }
}

reload.addEventListener('click', show);
show();
