import {el} from '/elements.js';
import {chrome} from '/chrome.js';

chrome('dashboard');

const summary = document.getElementById('summary');
const svgNS = 'http://www.w3.org/2000/svg';

function body(id) {
  return document.querySelector(`#${id} .body`);
}

function clock(iso) {
  const d = new Date(iso);
  const two = n => String(n).padStart(2, '0');
  return `${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`;
}

function ago(iso) {
  if (!iso || iso.startsWith('0001')) {
    return 'never';
  }
  const s = Math.round((Date.now() - new Date(iso)) / 1000);
  if (s < 90) {
    return `${s}s ago`;
  }
  if (s < 5400) {
    return `${Math.round(s / 60)}m ago`;
  }
  return `${Math.round(s / 3600)}h ago`;
}

function bytes(n) {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`;
}

function count(n) {
  return n.toLocaleString('en-US');
}

function stat(label, value, className) {
  const node = el('div', 'stat ' + (className ?? ''));
  node.append(el('span', 'value', value), el('span', 'label', label));
  return node;
}

function sparkline(samples, pick, format, label) {
  const w = 280, h = 44;
  const box = el('div', 'spark');
  const head = el('div', 'spark-head');
  const name = el('span', 'label', label);
  const readout = el('span', 'readout', samples.length ? format(pick(samples[samples.length - 1])) : '');
  head.append(name, readout);
  const chart = document.createElementNS(svgNS, 'svg');
  chart.setAttribute('viewBox', `0 0 ${w} ${h}`);
  chart.setAttribute('preserveAspectRatio', 'none');
  box.append(head, chart);
  if (samples.length < 2) {
    return box;
  }
  const values = samples.map(pick);
  const top = Math.max(...values) || 1;
  const x = i => i / (values.length - 1) * w;
  const y = v => h - 2 - v / top * (h - 4);
  const line = document.createElementNS(svgNS, 'polyline');
  line.setAttribute('points', values.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' '));
  const cursor = document.createElementNS(svgNS, 'line');
  cursor.setAttribute('class', 'cursor');
  cursor.setAttribute('y1', '0');
  cursor.setAttribute('y2', String(h));
  cursor.style.display = 'none';
  chart.append(line, cursor);
  chart.addEventListener('mousemove', e => {
    const rect = chart.getBoundingClientRect();
    const i = Math.round((e.clientX - rect.left) / rect.width * (values.length - 1));
    cursor.setAttribute('x1', String(x(i)));
    cursor.setAttribute('x2', String(x(i)));
    cursor.style.display = '';
    readout.textContent = `${format(values[i])} at ${clock(samples[i].time)}`;
  });
  chart.addEventListener('mouseleave', () => {
    cursor.style.display = 'none';
    readout.textContent = format(values[values.length - 1]);
  });
  return box;
}

function showErrors(d) {
  const list = el('ol', 'errors');
  for (const e of d.errors.slice(0, 12)) {
    const li = el('li');
    const where = [e.attrs.app, e.attrs.user].filter(Boolean).join(' · ');
    const detail = e.attrs.error ?? '';
    li.append(el('span', 'when', clock(e.time)), el('span', 'what', e.message), el('span', 'where', where));
    if (detail) {
      li.append(el('span', 'detail', detail));
    }
    list.append(li);
  }
  body('errors').replaceChildren(
    stat('in the last hour', count(d.errorCount), d.errorCount ? 'bad' : 'good'),
    d.errors.length ? list : el('div', 'quiet', 'no errors since this server started'),
  );
}

function showRuntime(d) {
  const s = d.samples;
  if (!s.length) {
    body('runtime').replaceChildren(el('div', 'quiet', 'no samples yet: the first comes 5s after the server starts'));
    return;
  }
  const last = s[s.length - 1];
  const stats = el('div', 'stats');
  stats.append(stat('goroutines', count(last.goroutines)), stat('gc runs', count(last.gc)));
  body('runtime').replaceChildren(
    sparkline(s, v => v.heapMiB, v => `${count(v)} MiB`, 'heap'),
    sparkline(s, v => v.sysMiB, v => `${count(v)} MiB`, 'sys'),
    sparkline(s, v => v.cpu, v => `${v.toFixed(2)} cores`, 'cpu'),
    stats,
  );
}

function showQueues(d) {
  const table = el('table', 'rows');
  for (const q of d.queues) {
    const tr = el('tr', q.pending ? 'busy' : 'idle');
    tr.append(el('td', '', q.name), el('td', 'num', count(q.pending)));
    table.append(tr);
  }
  body('queues').replaceChildren(table, el('div', 'quiet', `last sheet refresh ${ago(d.lastRefresh)}`));
}

function showLatency(d) {
  if (!d.latency.length) {
    body('latency').replaceChildren(el('div', 'quiet', 'no requests in the last hour'));
    return;
  }
  const table = el('table', 'rows');
  const head = el('tr');
  for (const h of ['app', 'requests', 'failed', 'p50 ms', 'p95 ms', 'max ms', 'over 750ms']) {
    head.append(el('th', h === 'app' ? '' : 'num', h));
  }
  table.append(head);
  for (const l of d.latency) {
    const tr = el('tr');
    tr.append(
      el('td', '', l.app),
      el('td', 'num', count(l.requests)),
      el('td', 'num' + (l.failed ? ' bad' : ''), count(l.failed)),
      el('td', 'num', l.p50.toFixed(0)),
      el('td', 'num' + (l.p95 >= 750 ? ' bad' : ''), l.p95.toFixed(0)),
      el('td', 'num' + (l.max >= 750 ? ' warn' : ''), l.max.toFixed(0)),
      el('td', 'num' + (l.slow ? ' warn' : ''), count(l.slow)),
    );
    table.append(tr);
  }
  body('latency').replaceChildren(table);
}

function showData(d) {
  const bySheet = new Map();
  for (const t of d.tables) {
    const s = bySheet.get(t.sheet) ?? {rows: 0, cells: 0};
    s.rows += t.rows;
    s.cells += t.rows * t.columns;
    bySheet.set(t.sheet, s);
  }
  const table = el('table', 'rows');
  const head = el('tr');
  head.append(el('th', '', 'sheet'), el('th', 'num', 'rows'), el('th', 'num', 'cells'));
  table.append(head);
  for (const [sheet, s] of bySheet) {
    const tr = el('tr');
    tr.dataset.sheet = sheet;
    tr.append(el('td', 'sheet-name', sheet.replace(/^data/, '')), el('td', 'num', count(s.rows)), el('td', 'num', count(s.cells)));
    table.append(tr);
  }
  const buckets = el('table', 'rows');
  const bhead = el('tr');
  bhead.append(el('th', '', 'bucket'), el('th', 'num', 'objects'), el('th', 'num', 'size'), el('th', 'num', 'measured'));
  buckets.append(bhead);
  for (const b of d.buckets) {
    const tr = el('tr');
    if (b.error) {
      tr.append(el('td', '', b.name), el('td', 'bad', b.error));
      tr.lastChild.colSpan = 3;
      buckets.append(tr);
      continue;
    }
    const folders = Object.values(b.folders ?? {});
    const measured = !b.measured.startsWith('0001');
    tr.append(
      el('td', '', b.name),
      el('td', 'num', measured ? count(folders.reduce((n, f) => n + f.objects, 0)) : ''),
      el('td', 'num', measured ? bytes(folders.reduce((n, f) => n + f.bytes, 0)) : ''),
      el('td', 'num', measured ? ago(b.measured) : 'measuring…'),
    );
    buckets.append(tr);
  }
  body('data').replaceChildren(table, buckets);
}

function show(d) {
  showErrors(d);
  showRuntime(d);
  showQueues(d);
  showLatency(d);
  showData(d);
  summary.textContent = `live · as of ${clock(d.at)}`;
  summary.classList.remove('lost');
}

const events = new EventSource('/api/dashboard');
events.addEventListener('message', e => show(JSON.parse(e.data)));
events.addEventListener('error', () => {
  summary.textContent = 'disconnected · reconnecting…';
  summary.classList.add('lost');
});
