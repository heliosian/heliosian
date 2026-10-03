const skipped = new Set(['id', 'kind', 'title', 'parent']);

async function query(src) {
  const response = await fetch('/api/q', {method: 'QUERY', headers: {'Content-Type': 'text/plain'}, body: src});
  if (!response.ok) {
    throw new Error(`${src}: ${response.status} ${await response.text()}`);
  }
  return response.json();
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

function compareKeys(a, b) {
  if (a === b) {
    return 0;
  }
  if (!a) {
    return 1;
  }
  if (!b) {
    return -1;
  }
  return a < b ? -1 : 1;
}

function byOrder(a, b) {
  return compareKeys(a.order, b.order) || compareKeys(a.start, b.start) || (a.title ?? '').localeCompare(b.title ?? '');
}

const tree = document.getElementById('tree');
const filter = document.getElementById('filter');
const summary = document.getElementById('summary');

let groups;
let children;
let counts;
try {
  const [groupAnswer, memberAnswer] = await Promise.all([query('(from GROUP)'), query('(from MEMBER)')]);
  groups = groupAnswer.resources.GROUP ?? {};
  children = new Map();
  for (const g of Object.values(groups)) {
    const parent = g.parent && groups[g.parent] ? g.parent : '';
    if (!children.has(parent)) {
      children.set(parent, []);
    }
    children.get(parent).push(g);
  }
  for (const list of children.values()) {
    list.sort(byOrder);
  }
  counts = new Map();
  for (const m of Object.values(memberAnswer.resources.MEMBER ?? {})) {
    if (!counts.has(m.group)) {
      counts.set(m.group, {member: 0, manager: 0, waitlist: 0});
    }
    counts.get(m.group)[m.role] += 1;
  }
} catch (err) {
  tree.replaceChildren(el('p', 'error', err.message));
  throw err;
}

function label(g) {
  const out = el('summary');
  out.append(el('span', 'title', g.title || g.slug || '(untitled)'));
  out.append(el('span', 'tag kind', g.kind));
  if (g.slug && g.title) {
    out.append(el('span', 'tag', g.slug));
  }
  if (g.listed !== 'Yes' && g.kind !== 'family') {
    out.append(el('span', 'tag off', 'unlisted'));
  }
  if (g.mail === 'Yes') {
    out.append(el('span', 'tag', 'mail'));
  }
  if (g.status && g.status !== 'open') {
    out.append(el('span', 'tag off', g.status));
  }
  if (g.visibility && g.visibility !== 'everyone') {
    out.append(el('span', 'tag off', `visible: ${g.visibility}`));
  }
  const meta = [];
  if (g.start) {
    meta.push(g.end && g.end !== g.start ? `${g.start} – ${g.end}` : g.start);
  }
  const c = counts.get(g.id);
  if (c) {
    const parts = [];
    for (const role of ['member', 'manager', 'waitlist']) {
      if (c[role]) {
        parts.push(`${c[role]} ${role}${c[role] === 1 ? '' : 's'}`);
      }
    }
    meta.push(parts.join(', '));
  }
  const kids = children.get(g.id)?.length ?? 0;
  if (kids) {
    meta.push(`${kids} under it`);
  }
  if (meta.length) {
    out.append(el('span', 'meta', meta.join(' · ')));
  }
  out.append(el('span', 'id', g.id));
  return out;
}

function fields(g) {
  const out = el('dl', 'fields');
  for (const [column, value] of Object.entries(g).sort(([a], [b]) => a.localeCompare(b))) {
    if (skipped.has(column) || value === '' || value === undefined) {
      continue;
    }
    out.append(el('dt', '', column), el('dd', '', value));
  }
  if (g.parent && !groups[g.parent]) {
    out.append(el('dt', '', 'parent'), el('dd', '', `${g.parent} (not visible to you)`));
  }
  return out;
}

function render(text) {
  const matches = g => !text || [g.title, g.slug, g.id].some(v => (v ?? '').toLowerCase().includes(text));
  let shown = 0;
  const node = g => {
    const below = (children.get(g.id) ?? []).map(node).filter(Boolean);
    const hit = matches(g);
    if (text && !hit && below.length === 0) {
      return null;
    }
    shown++;
    const out = el('details');
    if (text && hit) {
      out.classList.add('hit');
    }
    if (text && below.length) {
      out.open = true;
    }
    out.append(label(g), fields(g), ...below);
    return out;
  };
  const byKind = new Map();
  for (const g of children.get('') ?? []) {
    const rendered = node(g);
    if (!rendered) {
      continue;
    }
    if (!byKind.has(g.kind)) {
      byKind.set(g.kind, []);
    }
    byKind.get(g.kind).push(rendered);
  }
  const sections = [];
  for (const kind of [...byKind.keys()].sort()) {
    const section = el('details', 'section');
    section.open = Boolean(text);
    section.append(el('summary', '', `${kind} (${byKind.get(kind).length} at the top)`), ...byKind.get(kind));
    sections.push(section);
  }
  tree.replaceChildren(...sections);
  const total = Object.keys(groups).length;
  summary.textContent = text ? `${shown} of ${total} groups shown` : `${total} groups you can see`;
}

let timer;
filter.addEventListener('input', () => {
  clearTimeout(timer);
  timer = setTimeout(() => render(filter.value.trim().toLowerCase()), 200);
});

function setAll(open) {
  for (const d of tree.querySelectorAll('details')) {
    d.open = open;
  }
}

document.getElementById('open').addEventListener('click', () => setAll(true));
document.getElementById('close').addEventListener('click', () => setAll(false));

render('');
