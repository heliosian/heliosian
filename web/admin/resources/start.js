import {el} from '/elements.js';
import {api} from '/api.js';
import {chrome, query, labelOf} from '/chrome.js';
import {all, byName, link, cell, grid} from '/grid.js';
import {policyList, clauseQuery, queryHref, clauseHref} from '/policies.js';

chrome('resources');

const limit = 1000;
const sheets = ['datapeople', 'datagroups', 'datadocuments', 'datamail', 'dataconfig', '*', 'generated'];
const sheetNames = {'*': 'every sheet'};

const rail = document.getElementById('rail');
const view = document.getElementById('view');
const heading = document.getElementById('heading');
const filter = document.getElementById('filter');
const summary = document.getElementById('summary');

const referrers = new Map();
for (const t of all.filter(t => t.name !== 'CHANGES')) {
  for (const c of t.columns) {
    if (c.kind !== 'ref' && c.kind !== 'refs') {
      continue;
    }
    const target = c.relation || '*';
    if (!referrers.has(target)) {
      referrers.set(target, []);
    }
    referrers.get(target).push({table: t, column: c.name});
  }
}

function scan(table, where) {
  const out = {from: table.name, limit};
  if (where) {
    out.where = where;
  }
  const include = table.columns.filter(c => c.kind === 'ref' && c.relation).map(c => c.name);
  if (include.length) {
    out.include = include;
  }
  return query(out);
}

const changed = () => filter.oninput?.();

let grids = [];

function refresh() {
  const text = filter.value.trim().toLowerCase();
  return grids.reduce((n, g) => n + g.filterBy(text), 0);
}

for (const [id, expand] of [['open', true], ['close', false]]) {
  document.getElementById(id).addEventListener('click', () => {
    for (const g of grids) {
      g.setAll(expand);
    }
    changed();
  });
}

function showTreeButtons() {
  const tree = grids.some(g => g.tree);
  document.getElementById('open').hidden = !tree;
  document.getElementById('close').hidden = !tree;
}

function drawRail(current) {
  const out = [];
  for (const sheet of sheets) {
    const list = all.filter(t => t.sheet === sheet).sort((a, b) => a.name.localeCompare(b.name));
    if (!list.length) {
      continue;
    }
    const section = el('div', 'sheet');
    section.dataset.sheet = sheet;
    section.append(el('div', 'sheet-name', sheetNames[sheet] ?? sheet.replace(/^data/, '')));
    for (const t of list) {
      section.append(link(`#${t.name}`, t.name === current ? 'here' : '', t.name));
    }
    out.push(section);
  }
  rail.replaceChildren(...out);
}

async function listView(table) {
  heading.textContent = table.name;
  const answer = await scan(table);
  const g = grid(table, answer, answer.result, changed);
  grids = [g];
  showTreeButtons();
  view.replaceChildren(g.wrap);
  const update = () => {
    const shown = refresh();
    summary.textContent = `${shown} of ${g.count}${g.count === limit ? ` (first ${limit})` : ''} rows shown`;
  };
  filter.oninput = update;
  update();
}

function mark(holds) {
  return el('span', holds ? 'mark held' : 'mark', holds ? '✓' : '✗');
}

function verdictItem(clauses, v) {
  const c = clauses[v.clause];
  const li = el('li', v.holds ? 'held' : '');
  const text = el('div', 'verdict-text');
  text.append(link(clauseHref(v.clause), 'verdict-comment', c.comment || '(no comment)'), el('code', '', c.condition));
  li.append(mark(v.holds), text, link(queryHref(clauseQuery(c)), 'run', 'run ↗'));
  return li;
}

async function accessCard(table, id) {
  const [clauses, ex] = await Promise.all([policyList(), api('GET', `/api/explain/${id}`)]);
  const box = el('details', 'card access');
  box.dataset.sheet = table.sheet;
  const held = ex.clauses.filter(v => v.holds).length;
  box.append(el('summary', '', `who may see this · ${ex.readable ? 'readable' : 'not readable'} as the viewer, ${held} of ${ex.clauses.length} row clauses hold`));
  const rows = el('ul', 'verdicts');
  for (const v of ex.clauses) {
    rows.append(verdictItem(clauses, v));
  }
  box.append(rows);
  const columns = el('table', 'column-access');
  for (const c of ex.columns) {
    const tr = el('tr', c.readable ? 'held' : '');
    const why = el('td');
    if (c.private) {
      why.textContent = 'private: the import alone';
    } else if (!c.clauses.length) {
      why.textContent = 'no column clause names it';
    } else {
      for (const v of c.clauses) {
        const a = link(clauseHref(v.clause), v.holds ? 'held' : '', clauses[v.clause].comment || clauses[v.clause].condition);
        a.title = clauses[v.clause].condition;
        why.append(a, ' ');
      }
    }
    tr.append(el('td', 'column-name', c.column), el('td', '', ''), why);
    tr.children[1].append(c.private ? el('span', 'mark', '·') : mark(c.readable));
    columns.append(tr);
  }
  box.append(el('h3', '', 'columns'), columns);
  return box;
}

function withValues(changes) {
  const columns = [];
  for (const c of changes.columns) {
    columns.push(c);
    if (c.name === 'previous') {
      columns.push({name: 'value', kind: 'text', relation: '', schema: {description: 'what the column held after the change: the next change\'s previous, or the row as it is now'}});
    }
  }
  return {...changes, columns};
}

function fillValues(answer, current) {
  const after = {...current};
  for (const id of answer.result) {
    const change = answer.resources.CHANGES[id];
    if (!change.column) {
      continue;
    }
    change.value = change.action === 'delete' ? '' : (after[change.column] ?? '');
    after[change.column] = change.previous ?? '';
  }
}

async function historyCard(id, current) {
  const changes = withValues(byName.get('CHANGES'));
  const box = el('details', 'card history');
  box.dataset.sheet = changes.sheet;
  try {
    const answer = await query({from: 'CHANGES', where: [{'=': [{path: 'row'}, id]}], order: [{path: 'at', dir: 'desc'}]});
    box.append(el('summary', '', answer.result.length ? `history · ${answer.result.length} changes, the last ${answer.resources.CHANGES[answer.result[0]].at}` : 'history · no changes the viewer can see'));
    if (answer.result.length) {
      fillValues(answer, current);
      const g = grid(changes, answer, answer.result, changed, ['column', 'previous', 'value']);
      grids.push(g);
      box.append(g.wrap);
    }
  } catch (err) {
    box.append(el('summary', '', 'history'), el('p', 'error', err.message));
  }
  return box;
}

async function detailView(table, id) {
  heading.replaceChildren(link(`#${table.name}`, '', table.name), ` / ${id}`);
  const answer = await scan(table, [{'=': [{path: 'id'}, id]}]);
  const row = answer.resources[table.name]?.[id];
  if (!row) {
    const parts = [el('p', 'error', `no ${table.name} ${id} that the viewer can see`)];
    try {
      parts.push(await accessCard(table, id));
    } catch {
      parts.push(el('p', 'note', 'nor one you could see as yourself, so there is nothing to explain'));
    }
    view.replaceChildren(...parts);
    return;
  }
  const card = el('section', 'card');
  card.dataset.sheet = table.sheet;
  card.append(el('h2', '', labelOf(row)));
  const fields = el('dl', 'fields');
  for (const c of table.columns) {
    if (!row[c.name]) {
      continue;
    }
    const dt = el('dt', '', c.name);
    dt.title = c.kind;
    const dd = el('dd');
    dd.append(...cell(answer, c, row[c.name], row.id, true).childNodes);
    fields.append(dt, dd);
  }
  card.append(fields);
  const sections = [card];
  if (table.sheet !== 'generated') {
    sections.push(await accessCard(table, id), await historyCard(id, row));
  }
  const found = [];
  const refs = [...(referrers.get(table.name) ?? []), ...(referrers.get('*') ?? [])];
  const answers = await Promise.all(refs.map(async ({table: from, column}) => {
    try {
      return {from, column, answer: await scan(from, [{'=': [{path: column}, id]}])};
    } catch (err) {
      return {from, column, err};
    }
  }));
  for (const {from, column, answer, err} of answers) {
    if (err) {
      found.push(el('p', 'error', `${from.name}.${column}: ${err.message}`));
      continue;
    }
    if (!answer.result.length) {
      continue;
    }
    const g = grid(from, answer, answer.result, changed);
    grids.push(g);
    const section = el('section', 'referrer');
    section.dataset.sheet = from.sheet;
    section.append(el('h3', '', `${from.name}.${column} (${answer.result.length})`), g.wrap);
    found.push(section);
  }
  sections.push(...found);
  showTreeButtons();
  view.replaceChildren(...sections);
  filter.oninput = refresh;
  summary.textContent = `${found.length} tables point here`;
}

async function route() {
  const [name, id] = decodeURIComponent(location.hash.slice(1)).split('/');
  const table = byName.get(name) ?? byName.get('PERSON');
  drawRail(table.name);
  filter.value = '';
  summary.textContent = '';
  grids = [];
  showTreeButtons();
  view.replaceChildren('Loading…');
  try {
    if (id) {
      await detailView(table, id);
    } else {
      await listView(table);
    }
  } catch (err) {
    view.replaceChildren(el('p', 'error', err.message));
  }
}

addEventListener('hashchange', route);
route();
