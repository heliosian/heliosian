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

function sparkline(samples, pick, format, label, from, to) {
  const w = 280, h = 44;
  samples = samples.filter(s => new Date(s.time) >= from);
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
  const times = samples.map(s => new Date(s.time).getTime());
  const top = Math.max(...values) || 1;
  const x = i => (times[i] - from) / (to - from) * w;
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
    const at = from.getTime() + (e.clientX - rect.left) / rect.width * (to - from);
    let i = 0;
    for (let j = 1; j < times.length; j++) {
      if (Math.abs(times[j] - at) < Math.abs(times[i] - at)) {
        i = j;
      }
    }
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
  const to = new Date(d.at);
  const from = new Date(to - 3600 * 1000);
  body('runtime').replaceChildren(
    sparkline(s, v => v.heapMiB, v => `${count(v)} MiB`, 'heap · last hour', from, to),
    sparkline(s, v => v.sysMiB, v => `${count(v)} MiB`, 'sys', from, to),
    sparkline(s, v => v.cpu, v => `${v.toFixed(2)} cores`, 'cpu', from, to),
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
  for (const b of d.ops.buckets) {
    const tr = el('tr');
    const r = b.reading;
    if (r.error) {
      tr.append(el('td', '', b.name), el('td', 'bad', r.error));
      tr.lastChild.colSpan = 3;
      buckets.append(tr);
      continue;
    }
    const folders = Object.values(r.value);
    const measured = !r.fetched.startsWith('0001');
    tr.append(
      el('td', '', b.name),
      el('td', 'num', measured ? count(folders.reduce((n, f) => n + f.objects, 0)) : ''),
      el('td', 'num', measured ? bytes(folders.reduce((n, f) => n + f.bytes, 0)) : ''),
      el('td', 'num', measured ? ago(r.fetched) : 'measuring…'),
    );
    buckets.append(tr);
  }
  body('data').replaceChildren(table, buckets);
}

function pending(reading) {
  if (reading.error) {
    return el('div', 'bad', reading.error);
  }
  if (reading.fetched.startsWith('0001')) {
    return el('div', 'quiet', 'reading…');
  }
  return null;
}

function link(href, text, className) {
  const a = el('a', className ?? '', text);
  a.href = href;
  a.target = '_blank';
  a.rel = 'noopener';
  return a;
}

function minutes(from, to) {
  const s = Math.max(0, Math.round((new Date(to) - new Date(from)) / 1000));
  return s < 90 ? `${s}s` : `${Math.round(s / 60)}m`;
}

const buildWords = {
  QUEUED: ['queued', 'warn'],
  PENDING: ['queued', 'warn'],
  WORKING: ['building', 'warn'],
  FAILURE: ['build failed', 'bad'],
  INTERNAL_ERROR: ['build errored', 'bad'],
  TIMEOUT: ['build timed out', 'bad'],
  CANCELLED: ['cancelled', 'quiet'],
  EXPIRED: ['expired', 'quiet'],
};

function stage(commit, build, servingBuild, serving, older) {
  if (servingBuild && commit.sha === servingBuild.sha) {
    return [`serving · ${serving.revision}`, 'good'];
  }
  if (!build) {
    return ['no build', 'quiet'];
  }
  if (build.status === 'SUCCESS') {
    return older ? ['deployed earlier', 'quiet'] : [`built in ${minutes(build.started, build.finished)}, deploying`, 'warn'];
  }
  const [word, tone] = buildWords[build.status] ?? [build.status.toLowerCase(), 'quiet'];
  if (build.status === 'WORKING') {
    return [`${word} ${minutes(build.started, new Date())}`, tone];
  }
  return [word, tone];
}

function showDeploy(d) {
  const {commits, builds, serving} = d.ops;
  const waiting = pending(commits) ?? pending(builds) ?? pending(serving);
  if (waiting) {
    body('deploy').replaceChildren(waiting);
    return;
  }
  const table = el('table', 'rows deploys');
  const servingBuild = builds.value.find(b => b.digest && b.digest === serving.value.digest);
  let older = false;
  for (const c of commits.value) {
    const build = builds.value.find(b => b.sha === c.sha);
    const [word, tone] = stage(c, build, servingBuild, serving.value, older);
    if (servingBuild && c.sha === servingBuild.sha) {
      older = true;
    }
    const status = build?.logUrl ? link(build.logUrl, word) : el('span', '', word);
    const tr = el('tr', tone);
    const sha = el('td', 'sha');
    sha.append(link(c.url, c.sha.slice(0, 7)));
    tr.append(sha, el('td', 'what', c.message), el('td', 'where', `${c.author} · ${ago(c.time)}`), el('td', 'stage'));
    tr.lastChild.append(status);
    table.append(tr);
  }
  body('deploy').replaceChildren(table);
}

function monthStart(iso) {
  const now = new Date(iso);
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1));
}

function todayNoon(iso) {
  const now = new Date(iso);
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate(), 12));
}

function dollars(n) {
  return n.toLocaleString('en-US', {style: 'currency', currency: 'USD'});
}

function showClaude(d) {
  const spend = d.ops.spend;
  const waiting = pending(spend);
  if (waiting) {
    body('claude').replaceChildren(waiting);
    return;
  }
  const days = spend.value.days.map(day => ({time: day.date + 'T12:00:00Z', total:Object.values(day.byModel).reduce((n, v) => n + v, 0), byModel: day.byModel}));
  const month = {};
  for (const day of days) {
    for (const [model, v] of Object.entries(day.byModel)) {
      month[model] = (month[model] ?? 0) + v;
    }
  }
  const today = days.length ? days[days.length - 1].total : 0;
  const stats = el('div', 'stats');
  stats.append(stat('today (UTC)', dollars(today)), stat('this month', dollars(Object.values(month).reduce((n, v) => n + v, 0))));
  const table = el('table', 'rows');
  for (const [model, v] of Object.entries(month).sort((a, b) => b[1] - a[1])) {
    const tr = el('tr');
    tr.append(el('td', '', model), el('td', 'num', dollars(v)));
    table.append(tr);
  }
  body('claude').replaceChildren(
    stats,
    sparkline(days, v => v.total, dollars, 'per day this month', monthStart(d.at), todayNoon(d.at)),
    table,
    el('div', 'quiet', `read ${ago(spend.fetched)}`),
  );
}

function showIssues(d) {
  const issues = d.ops.issues;
  const waiting = pending(issues);
  if (waiting) {
    body('issues').replaceChildren(waiting);
    return;
  }
  const list = el('ol', 'issues');
  for (const i of issues.value.items) {
    const li = el('li');
    const title = link(i.url, i.title, 'what');
    title.title = i.title;
    li.append(el('span', 'when', `#${i.number}`), title, el('span', 'labels', i.labels.join(' ')), el('span', 'where', ago(i.created)));
    list.append(li);
  }
  body('issues').replaceChildren(stat('open', count(issues.value.open)), list);
}

function show(d) {
  showDeploy(d);
  showClaude(d);
  showIssues(d);
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
