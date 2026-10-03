import {el} from '/elements.js';
import {chrome} from '/chrome.js';
import {policyList, clauseQuery, queryHref} from '/policies.js';

chrome('policies');

const list = document.getElementById('list');
const filter = document.getElementById('filter');
const summary = document.getElementById('summary');

function link(href, className, text) {
  const a = el('a', className, text);
  a.href = href;
  return a;
}

function target(c) {
  if (c.kind === 'define') {
    return [c.name, ...(c.params ?? [])].join(' ');
  }
  if (c.kind === 'read columns') {
    return `${c.table} (${c.columns.join(' ')})`;
  }
  return c.column ? `${c.table}.${c.column}` : c.table;
}

function card(c, index) {
  const out = el('article', 'clause');
  out.id = `clause-${index}`;
  out.dataset.kind = c.kind;
  out.dataset.text = [c.kind, target(c), c.comment, c.form].join(' ').toLowerCase();
  const head = el('div', 'clause-head');
  head.append(el('span', 'kind', c.kind), el('span', 'target', target(c)));
  const q = clauseQuery(c);
  if (q) {
    head.append(link(queryHref(q), 'run', 'run ↗'));
  }
  out.append(head);
  if (c.comment) {
    out.append(el('p', 'comment', c.comment));
  }
  out.append(el('pre', 'form', c.form));
  return out;
}

function apply() {
  const text = filter.value.trim().toLowerCase();
  let shown = 0;
  let total = 0;
  for (const section of list.querySelectorAll('section')) {
    let any = false;
    for (const c of section.querySelectorAll('.clause')) {
      total++;
      c.hidden = Boolean(text) && !c.dataset.text.includes(text);
      any ||= !c.hidden;
      shown += c.hidden ? 0 : 1;
    }
    section.hidden = !any;
  }
  summary.textContent = text ? `${shown} of ${total} clauses` : `${total} clauses`;
}

try {
  const clauses = await policyList();
  const sections = new Map();
  clauses.forEach((c, i) => {
    if (!sections.has(c.section)) {
      sections.set(c.section, []);
    }
    sections.get(c.section).push(card(c, i));
  });
  const out = [];
  for (const [name, cards] of sections) {
    const section = el('section');
    section.append(el('h2', '', name || 'untitled'), ...cards);
    out.push(section);
  }
  list.replaceChildren(...out);
  filter.addEventListener('input', apply);
  apply();
  if (location.hash) {
    const at = document.getElementById(location.hash.slice(1));
    at?.classList.add('here');
    at?.scrollIntoView({block: 'center'});
  }
} catch (err) {
  list.replaceChildren(el('p', 'error', err.message));
}
