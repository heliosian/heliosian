import {el} from '/elements.js';
import {chrome} from '/chrome.js';
import {policyList, clauseQuery, queryHref} from '/policies.js';

chrome('policies');

const list = document.getElementById('list');
const filter = document.getElementById('filter');
const summary = document.getElementById('summary');

const token = /("(?:[^"\\]|\\.)*")|(@[^\s()"]+)|([()])|(-?\d+(?:\.\d+)?(?![^\s()]))|([^\s()"]+)|(\s+)/g;

function link(href, className, text) {
  const a = el('a', className, text);
  a.href = href;
  return a;
}

function highlight(text, defined) {
  const out = el('code', 'sexp');
  let head = false;
  for (const [, string, at, paren, number, name, space] of text.matchAll(token)) {
    if (space) {
      out.append(space);
      continue;
    }
    if (paren) {
      out.append(el('span', 'paren', paren));
      head = paren === '(';
      continue;
    }
    const call = head;
    head = false;
    if (string) {
      out.append(el('span', 'str', string));
    } else if (at) {
      out.append(el('span', 'at', at));
    } else if (number) {
      out.append(el('span', 'num', number));
    } else if (defined.has(name)) {
      out.append(link(`#clause-${defined.get(name)}`, 'def', name));
    } else if (call) {
      out.append(el('span', 'call', name));
    } else if (name === 'true' || name === 'false') {
      out.append(el('span', 'const', name));
    } else if (/^[A-Z][A-Z_]*$/.test(name)) {
      out.append(el('span', 'table', name));
    } else {
      out.append(el('span', 'path', name));
    }
  }
  return out;
}

function actorOf(c) {
  if (c.kind === 'define') {
    return 'definitions';
  }
  if (c.actor) {
    return c.actor;
  }
  return c.condition === 'false' ? 'nobody' : 'everyone';
}

function onColumns(c) {
  return c.kind === 'read columns' || c.kind === 'set';
}

function groupHeading(key, defined) {
  const h = el('h2');
  if (key === 'definitions') {
    h.append('definitions');
  } else if (key === 'everyone') {
    h.append('everyone', el('span', 'note', 'the row decides'));
  } else if (key === 'nobody') {
    h.append('nobody', el('span', 'note', 'the server alone'));
  } else {
    h.append(highlight(key, defined));
  }
  return h;
}

function card(c, index, defined) {
  const out = el('article', 'clause');
  out.id = `clause-${index}`;
  out.dataset.kind = c.kind;
  out.dataset.text = [actorOf(c), c.kind, c.table, c.column, ...(c.columns ?? []), c.name, c.comment, c.form].join(' ').toLowerCase();
  const head = el('div', 'clause-head');
  head.append(el('span', 'kind', c.kind === 'read columns' ? 'read' : c.kind));
  if (c.kind === 'define') {
    head.append(highlight(`(${[c.name, ...(c.params ?? [])].join(' ')})`, new Map()));
  } else if (onColumns(c)) {
    const columns = el('span', 'columns');
    for (const name of c.columns ?? [c.column]) {
      columns.append(el('span', 'column', name));
    }
    head.append(columns);
  }
  const q = clauseQuery(c);
  if (q) {
    head.append(link(queryHref(q), 'run', 'run ↗'));
  }
  out.append(head);
  if (c.comment) {
    out.append(el('p', 'comment', c.comment));
  }
  const body = el('pre', 'form');
  body.append(highlight(c.kind === 'define' ? c.condition : c.rest, defined));
  out.append(body);
  return out;
}

function part(name, cards) {
  const out = el('div', 'part');
  const list = el('div');
  list.append(...cards);
  out.append(el('div', 'part-name', name), list);
  return out;
}

function tableBlock(table, entries, defined) {
  const out = el('div', 'table-block');
  const h = el('h3');
  h.append(link(`/resources#${table}`, '', table));
  out.append(h);
  const rows = entries.filter(([c]) => !onColumns(c));
  const columns = entries.filter(([c]) => onColumns(c));
  if (rows.length) {
    out.append(part('rows', rows.map(([c, i]) => card(c, i, defined))));
  }
  if (columns.length) {
    out.append(part('columns', columns.map(([c, i]) => card(c, i, defined))));
  }
  return out;
}

function apply() {
  const text = filter.value.trim().toLowerCase();
  const clauses = list.querySelectorAll('.clause');
  let shown = 0;
  for (const c of clauses) {
    c.hidden = Boolean(text) && !c.dataset.text.includes(text);
    shown += c.hidden ? 0 : 1;
  }
  for (const box of list.querySelectorAll('.part, .table-block, section')) {
    box.hidden = !box.querySelector('.clause:not([hidden])');
  }
  summary.textContent = text ? `${shown} of ${clauses.length} clauses` : `${clauses.length} clauses`;
}

function mark() {
  for (const at of list.querySelectorAll('.here')) {
    at.classList.remove('here');
  }
  if (!location.hash) {
    return;
  }
  const at = document.getElementById(location.hash.slice(1));
  at?.classList.add('here');
  at?.scrollIntoView({block: 'center'});
}

try {
  const clauses = await policyList();
  const defined = new Map();
  clauses.forEach((c, i) => {
    if (c.kind === 'define') {
      defined.set(c.name, i);
    }
  });
  const groups = new Map();
  clauses.forEach((c, i) => {
    const key = actorOf(c);
    if (!groups.has(key)) {
      groups.set(key, []);
    }
    groups.get(key).push([c, i]);
  });
  const out = [];
  for (const [key, entries] of groups) {
    const section = el('section');
    section.append(groupHeading(key, defined));
    if (key === 'definitions') {
      section.append(...entries.map(([c, i]) => card(c, i, defined)));
      out.push(section);
      continue;
    }
    const tables = new Map();
    for (const entry of entries) {
      if (!tables.has(entry[0].table)) {
        tables.set(entry[0].table, []);
      }
      tables.get(entry[0].table).push(entry);
    }
    for (const [table, list] of tables) {
      section.append(tableBlock(table, list, defined));
    }
    out.push(section);
  }
  list.replaceChildren(...out);
  filter.addEventListener('input', apply);
  addEventListener('hashchange', mark);
  apply();
  mark();
} catch (err) {
  list.replaceChildren(el('p', 'error', err.message));
}
