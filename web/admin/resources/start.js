import {el} from '/elements.js';
import {chrome, query, labelOf} from '/chrome.js';
import {all, byName, link, cell, grid} from '/grid.js';

chrome('resources');

const limit = 1000;
const sheets = ['datapeople', 'datagroups', 'datadocuments', 'datamail', 'dataconfig', 'generated'];

const rail = document.getElementById('rail');
const view = document.getElementById('view');
const heading = document.getElementById('heading');
const filter = document.getElementById('filter');
const summary = document.getElementById('summary');

const referrers = new Map();
for (const t of all) {
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
    section.append(el('div', 'sheet-name', sheet.replace(/^data/, '')));
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

async function detailView(table, id) {
  heading.replaceChildren(link(`#${table.name}`, '', table.name), ` / ${id}`);
  const answer = await scan(table, [{'=': [{path: 'id'}, id]}]);
  const row = answer.resources[table.name]?.[id];
  if (!row) {
    view.replaceChildren(el('p', 'error', `no ${table.name} ${id} that you can see`));
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
  const found = [];
  grids = [];
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
