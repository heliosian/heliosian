import {el} from '/elements.js';
import {chrome, query, tables, labelOf} from '/chrome.js';

chrome('resources');

const limit = 1000;
const blank = '\u0000blank';
const numeric = new Set(['int', 'money', 'float']);
const byCode = new Set(['id', 'order', 'date', 'moment']);
const sheets = ['datapeople', 'datagroups', 'datadocuments', 'datamail', 'dataconfig', 'generated'];

const rail = document.getElementById('rail');
const view = document.getElementById('view');
const heading = document.getElementById('heading');
const filter = document.getElementById('filter');
const summary = document.getElementById('summary');

const all = await tables();
const byName = new Map(all.map(t => [t.name, t]));
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

function link(href, className, text) {
  const a = el('a', className, text);
  a.href = href;
  return a;
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

function rowsOf(table, answer) {
  return answer.result.map(id => answer.resources[table.name][id]);
}

function refLink(answer, target, id) {
  const tableName = target || all.find(t => t.columns[0].schema.pattern && new RegExp(t.columns[0].schema.pattern).test(id))?.name;
  const row = answer.resources[tableName]?.[id];
  const a = link(`#${tableName}/${id}`, 'ref', row ? labelOf(row) : id);
  a.title = id;
  return a;
}

function cell(answer, column, value) {
  const td = el('td');
  if (!value) {
    return td;
  }
  if (column.kind === 'ref') {
    td.append(refLink(answer, column.relation, value));
    return td;
  }
  if (column.kind === 'refs') {
    for (const id of value.split(',').map(s => s.trim()).filter(Boolean)) {
      td.append(refLink(answer, column.relation, id), ' ');
    }
    return td;
  }
  if (column.kind === 'url') {
    td.append(link(value, '', value));
    return td;
  }
  td.textContent = value;
  td.title = value;
  return td;
}

function treeOrder(table, rows) {
  const self = table.columns.find(c => c.kind === 'ref' && c.relation === table.name);
  if (!self) {
    return rows.map(row => ({row, depth: 0, parent: '', kids: 0}));
  }
  const ids = new Set(rows.map(r => r.id));
  const children = new Map();
  for (const row of rows) {
    const parent = ids.has(row[self.name]) ? row[self.name] : '';
    if (!children.has(parent)) {
      children.set(parent, []);
    }
    children.get(parent).push(row);
  }
  const out = [];
  const walk = (parent, depth) => {
    for (const row of children.get(parent) ?? []) {
      out.push({row, depth, parent, kids: children.get(row.id)?.length ?? 0});
      walk(row.id, depth + 1);
    }
  };
  walk('', 0);
  return out;
}

function grid(table, answer) {
  const rows = treeOrder(table, rowsOf(table, answer));
  const tree = rows.some(r => r.kids);
  const open = new Set();
  const entries = [];
  const byId = new Map();
  const columns = table.columns.filter(c => c.name === 'id' || rows.some(({row}) => row[c.name]));
  const wrap = el('div', 'scroll');
  const out = el('table', 'grid');
  out.dataset.sheet = table.sheet;
  const head = el('tr');
  const quick = el('tr', 'quick');
  const filters = new Map();
  let sort = null;
  const changed = () => filter.oninput?.();
  for (const c of columns) {
    const th = el('th', 'sortable', c.name);
    th.title = [c.kind, c.relation && `→ ${c.relation}`, c.schema.description, 'click to sort'].filter(Boolean).join(' · ');
    th.addEventListener('click', () => {
      if (sort?.column !== c) {
        sort = {column: c, dir: 1};
      } else if (sort.dir === 1) {
        sort.dir = -1;
      } else {
        sort = null;
      }
      for (const other of head.children) {
        other.dataset.sort = '';
      }
      th.dataset.sort = sort ? (sort.dir === 1 ? 'asc' : 'desc') : '';
      reorder();
    });
    head.append(th);
    quick.append(quickFilter(c));
  }
  out.append(el('thead'), el('tbody'));
  out.tHead.append(head, quick);
  function quickFilter(c) {
    const th = el('th');
    if (c.kind === 'enum' || c.kind === 'bool') {
      const select = el('select');
      const values = [...new Set(rows.map(({row}) => row[c.name] ?? ''))].sort();
      select.append(el('option', '', ''));
      for (const v of values) {
        const option = el('option', '', v || '(blank)');
        option.value = v || blank;
        select.append(option);
      }
      select.addEventListener('change', () => {
        if (select.value) {
          filters.set(c.name, {equals: select.value === blank ? '' : select.value});
        } else {
          filters.delete(c.name);
        }
        select.classList.toggle('set', Boolean(select.value));
        changed();
      });
      th.append(select);
      return th;
    }
    const input = el('input');
    input.type = 'search';
    input.placeholder = '…';
    input.addEventListener('input', () => {
      const v = input.value.trim().toLowerCase();
      if (v) {
        filters.set(c.name, {has: v});
      } else {
        filters.delete(c.name);
      }
      changed();
    });
    th.append(input);
    return th;
  }
  const matches = e => {
    if (text && !e.text.includes(text)) {
      return false;
    }
    for (const [name, f] of filters) {
      const v = e.values[name];
      if ('equals' in f ? (v.raw ?? '') !== f.equals : !v.show.includes(f.has)) {
        return false;
      }
    }
    return true;
  };
  const compare = (a, b) => {
    const c = sort.column;
    const x = a.values[c.name].key;
    const y = b.values[c.name].key;
    if (x === '' || y === '') {
      return (x === '') - (y === '');
    }
    let n;
    if (numeric.has(c.kind)) {
      n = Number(x) - Number(y);
    } else if (byCode.has(c.kind)) {
      n = x < y ? -1 : x > y ? 1 : 0;
    } else {
      n = x.localeCompare(y, undefined, {numeric: true, sensitivity: 'base'});
    }
    return n * sort.dir;
  };
  const reorder = () => {
    const children = new Map();
    for (const e of entries) {
      if (!children.has(e.parent)) {
        children.set(e.parent, []);
      }
      children.get(e.parent).push(e);
    }
    const body = out.tBodies[0];
    const walk = parent => {
      const list = [...(children.get(parent) ?? [])];
      if (sort) {
        list.sort(compare);
      }
      for (const e of list) {
        body.append(e.tr);
        walk(e.id);
      }
    };
    walk('');
  };
  let text = '';
  const apply = () => {
    let keep = null;
    if (text || filters.size) {
      keep = new Set();
      for (const e of entries) {
        if (matches(e)) {
          for (let x = e; x && !keep.has(x.id); x = byId.get(x.parent)) {
            keep.add(x.id);
          }
        }
      }
    }
    let shown = 0;
    for (const e of entries) {
      let visible = true;
      if (keep) {
        visible = keep.has(e.id);
      } else {
        for (let p = byId.get(e.parent); p; p = byId.get(p.parent)) {
          if (!open.has(p.id)) {
            visible = false;
            break;
          }
        }
      }
      e.tr.hidden = !visible;
      shown += visible ? 1 : 0;
      if (e.toggle) {
        const expanded = keep ? e.kidIds.some(id => keep.has(id)) : open.has(e.id);
        e.toggle.textContent = expanded ? '▾' : '▸';
      }
    }
    return shown;
  };
  for (const {row, depth, parent, kids} of rows) {
    const tr = el('tr');
    const entry = {id: row.id, parent, tr, text: Object.values(row).join(' ').toLowerCase(), kidIds: [], values: {}};
    byId.get(parent)?.kidIds.push(row.id);
    for (const c of columns) {
      if (c.name === 'id') {
        const td = el('td', 'id');
        td.style.paddingLeft = `${8 + depth * 14}px`;
        if (tree) {
          const toggle = el('button', 'toggle', kids ? '▸' : '');
          toggle.type = 'button';
          toggle.disabled = !kids;
          if (kids) {
            toggle.title = `${kids} under it`;
            toggle.addEventListener('click', () => {
              if (open.has(row.id)) {
                open.delete(row.id);
              } else {
                open.add(row.id);
              }
              apply();
            });
            entry.toggle = toggle;
          }
          td.append(toggle);
        }
        td.append(link(`#${table.name}/${row.id}`, '', row.id));
        tr.append(td);
        entry.values.id = {raw: row.id, show: row.id.toLowerCase(), key: row.id};
        continue;
      }
      const td = cell(answer, c, row[c.name]);
      const shown = td.textContent.trim();
      entry.values[c.name] = {raw: row[c.name], show: shown.toLowerCase(), key: c.kind === 'ref' || c.kind === 'refs' ? shown : (row[c.name] ?? '')};
      tr.append(td);
    }
    entries.push(entry);
    byId.set(row.id, entry);
    out.tBodies[0].append(tr);
  }
  wrap.append(out);
  const filterBy = value => {
    text = value;
    return apply();
  };
  const setAll = expand => {
    open.clear();
    if (expand) {
      for (const e of entries) {
        if (e.toggle) {
          open.add(e.id);
        }
      }
    }
    return apply();
  };
  apply();
  return {wrap, count: rows.length, tree, filterBy, setAll};
}

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
    filter.oninput?.();
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
  const g = grid(table, answer);
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
    dd.append(...cell(answer, c, row[c.name]).childNodes);
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
    const g = grid(from, answer);
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
