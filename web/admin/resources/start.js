import {el} from '/elements.js';
import {api} from '/api.js';
import {chrome, query, labelOf} from '/chrome.js';
import {all, byName, link, cell, grid, valueTitle} from '/grid.js';
import {policyList, clauseQuery, queryHref, clauseHref, actorOf, actorLabel, definitions, highlight} from '/policies.js';

chrome('resources');

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
  const out = {from: table.name};
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
  heading.title = table.description;
  const answer = await scan(table);
  const g = grid(table, answer, answer.result, changed);
  grids = [g];
  showTreeButtons();
  view.replaceChildren(el('p', 'note table-about', table.description), g.wrap);
  const update = () => {
    const shown = refresh();
    summary.textContent = `${shown} of ${g.count} rows shown`;
  };
  filter.oninput = update;
  update();
}

function mark(holds) {
  return el('span', holds ? 'mark held' : 'mark', holds ? '✓' : '✗');
}

function restMark(v) {
  const m = mark(v.rest);
  if (!v.actor) {
    m.classList.add('moot');
    m.title = v.rest ? 'the row meets this, but the viewer isn’t this actor' : 'the row doesn’t meet this, and the viewer isn’t this actor';
  }
  return m;
}

function verdictItem(clauses, defined, v) {
  const c = clauses[v.clause];
  const li = el('li', v.holds ? 'held' : '');
  const text = el('div', 'verdict-text');
  const form = el('pre', 'verdict-form');
  form.append(highlight(c.rest, defined));
  text.append(link(clauseHref(v.clause), 'verdict-comment', c.comment || '(no comment)'), form);
  li.append(restMark(v), text, link(queryHref(clauseQuery(c)), 'run', 'run ↗'));
  return li;
}

function partHead(name, note) {
  const out = el('div', 'part-head');
  out.append(el('h3', '', name), el('span', 'note', note));
  return out;
}

function grant(clauses, defined, v) {
  const c = clauses[v.clause];
  const out = el('span', v.holds ? 'grant held' : 'grant');
  out.title = c.condition;
  if (actorOf(c) !== 'nobody') {
    out.append(mark(v.actor));
  }
  out.append(actorLabel(actorOf(c), defined), restMark(v), link(clauseHref(v.clause), '', c.comment || c.rest));
  return out;
}

async function accessCard(table, id) {
  const [clauses, ex] = await Promise.all([policyList(), api('GET', `/api/explain/${id}`)]);
  const defined = definitions(clauses);
  const box = el('details', 'card access');
  box.dataset.sheet = table.sheet;
  const held = ex.clauses.filter(v => v.holds).length;
  box.append(el('summary', '', `who may see this · ${ex.readable ? 'readable' : 'not readable'} as the viewer, ${held} of ${ex.clauses.length} row clauses hold`));
  const rows = el('div', 'access-part');
  rows.append(partHead('row', 'any one clause whose actor and criteria both hold lets the viewer see the row'));
  const actors = new Map();
  for (const v of ex.clauses) {
    const key = actorOf(clauses[v.clause]);
    if (!actors.has(key)) {
      actors.set(key, []);
    }
    actors.get(key).push(v);
  }
  for (const [key, verdicts] of actors) {
    const group = el('div', 'actor-group');
    const heading = el('div', 'actor');
    if (key !== 'nobody') {
      heading.append(mark(verdicts[0].actor));
    }
    heading.append(actorLabel(key, defined));
    const list = el('ul', 'verdicts');
    for (const v of verdicts) {
      list.append(verdictItem(clauses, defined, v));
    }
    group.append(heading, list);
    rows.append(group);
  }
  const columns = el('table', 'column-access');
  for (const c of ex.columns) {
    const tr = el('tr', c.readable ? 'held' : '');
    const why = el('td');
    if (c.private) {
      why.textContent = 'private: the import alone';
    } else if (!c.clauses.length) {
      why.textContent = 'no column clause names it';
    } else {
      const grants = el('div', 'grants');
      grants.append(...c.clauses.map(v => grant(clauses, defined, v)));
      why.append(grants);
    }
    const state = el('td');
    state.append(c.private ? el('span', 'mark', '·') : mark(c.readable));
    tr.append(el('td', 'column-name', c.column), state, why);
    columns.append(tr);
  }
  const fields = el('div', 'access-part');
  fields.append(partHead('columns', 'which cells the viewer reads once they see the row'), columns);
  box.append(rows, fields);
  return box;
}

function changedColumn(change) {
  const c = byName.get(change.table)?.columns.find(c => c.name === change.column);
  if (!c || c.kind === 'blob') {
    return {name: change.column, kind: 'text', relation: '', schema: {}};
  }
  return c;
}

function withValues(changes) {
  const columns = [];
  for (const c of changes.columns) {
    if (c.name === 'previous') {
      columns.push({...c, label: 'before', cellOf: changedColumn});
      columns.push({name: 'after', kind: 'text', relation: '', schema: {description: 'what the column held after the change: the next change\'s previous, or the row as it is now'}, cellOf: changedColumn});
      continue;
    }
    columns.push(c);
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
    change.after = change.action === 'delete' ? '' : (after[change.column] ?? '');
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
      const g = grid(changes, answer, answer.result, changed, ['column', 'previous', 'after']);
      grids.push(g);
      box.append(g.wrap);
    }
  } catch (err) {
    box.append(el('summary', '', 'history'), el('p', 'error', err.message));
  }
  return box;
}

function editable(c) {
  return c.kind !== 'id' && c.kind !== 'blob' && !c.schema['x-generated'] && !c.schema['x-private'];
}

function button(text, className = '') {
  const b = el('button', className, text);
  b.type = 'button';
  return b;
}

function input(c, value) {
  const options = c.kind === 'bool' ? ['', 'Yes', 'No'] : c.kind === 'enum' ? c.schema.enum : null;
  if (options) {
    const select = el('select');
    for (const v of options.includes(value) ? options : [...options, value]) {
      const option = el('option', '', v || '(blank)');
      option.value = v;
      option.title = valueTitle(c, v);
      select.append(option);
    }
    select.value = value;
    return select;
  }
  if (c.kind === 'text') {
    const area = el('textarea');
    area.value = value;
    area.rows = Math.max(1, value.split('\n').length);
    return area;
  }
  const box = el('input');
  box.type = 'text';
  box.value = value;
  box.placeholder = c.schema.examples?.[0] ?? '';
  return box;
}

function editor(table, row) {
  const form = el('form', 'editor');
  const list = el('dl', 'fields');
  const inputs = new Map();
  for (const c of table.columns.filter(editable)) {
    const value = row[c.name] ?? '';
    const field = input(c, value);
    const dt = el('dt', '', c.name);
    dt.title = [c.kind, c.relation && `→ ${c.relation}`, c.schema.description].filter(Boolean).join(' · ');
    const dd = el('dd');
    dd.append(field);
    list.append(dt, dd);
    inputs.set(c.name, {field, value});
  }
  const error = el('p', 'error');
  error.hidden = true;
  const save = el('button', 'primary', 'save');
  save.type = 'submit';
  const cancel = button('cancel');
  const actions = el('div', 'actions');
  actions.append(save, cancel);
  form.append(list, error, actions);
  form.addEventListener('submit', async e => {
    e.preventDefault();
    const cells = {};
    for (const [name, {field, value}] of inputs) {
      if (field.value !== value) {
        cells[name] = field.value;
      }
    }
    if (!Object.keys(cells).length) {
      cancel.click();
      return;
    }
    save.disabled = true;
    try {
      await api('POST', '/api/q', {batch: [{set: row.id, cells}]});
      await route();
    } catch (err) {
      error.textContent = err.message;
      error.hidden = false;
      save.disabled = false;
    }
  });
  return {form, cancel};
}

function rowActions(table, row, fields) {
  const actions = el('div', 'actions');
  const error = el('p', 'error');
  error.hidden = true;
  const edit = button('edit');
  edit.addEventListener('click', () => {
    const {form, cancel} = editor(table, row);
    cancel.addEventListener('click', () => {
      form.replaceWith(fields);
      actions.hidden = false;
    });
    fields.replaceWith(form);
    actions.hidden = true;
    error.hidden = true;
    form.querySelector('select, textarea, input')?.focus();
  });
  const remove = button('delete', 'danger');
  remove.addEventListener('click', async () => {
    if (!confirm(`Delete ${table.name} ${row.id}, ${labelOf(row)}?`)) {
      return;
    }
    remove.disabled = true;
    try {
      await api('POST', '/api/q', {batch: [{delete: row.id}]});
      location.hash = `#${table.name}`;
    } catch (err) {
      error.textContent = err.message;
      error.hidden = false;
      remove.disabled = false;
    }
  });
  actions.append(edit, remove);
  return {actions, error};
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
  const top = el('div', 'card-top');
  top.append(el('h2', '', labelOf(row)));
  card.append(top);
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
  if (table.sheet !== 'generated' && !table.appendOnly) {
    const {actions, error} = rowActions(table, row, fields);
    top.append(actions);
    card.append(error);
  }
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
