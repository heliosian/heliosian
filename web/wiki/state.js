import {api} from '/api.js';
import {me as whoAmI} from '/data.js';

export const state = {pages: [], sides: new Map(), moved: new Map(), user: null, viewer: '', admin: false};

export function sidesOf(p) {
  return state.sides.get(p.id) || [];
}

const bodies = new Map();

function query(tree) {
  return api('QUERY', '/api/q', tree);
}

const viewerIs = column => ({'=': [{path: column}, {path: '@viewer'}]});

export async function loadModel() {
  const [viewer, pages, person, photo, admin, cards, redirects] = await Promise.all([
    whoAmI(),
    query({from: 'DOCUMENT', where: [{'=': [{path: 'kind'}, 'wiki']}], order: [{path: 'name', dir: 'asc'}]}),
    query({from: 'PERSON', where: [viewerIs('id')]}),
    query({from: 'PHOTO', where: [viewerIs('person'), {path: 'ready'}], order: [{path: 'order', dir: 'asc'}], limit: 1}),
    query({from: 'PERSON', where: [viewerIs('id'), {admin_of: ['wiki']}]}),
    query({from: 'DOCUMENT', where: [{'=': [{path: 'relation'}, 'side']}], order: [{path: 'order', dir: 'asc'}]}),
    query({from: 'REDIRECT', where: [{'=': [{path: 'app'}, 'wiki']}]}),
  ]);
  state.moved = new Map(redirects.result.map(id => {
    const row = redirects.resources.REDIRECT[id];
    return [row.old, row.new];
  }));
  state.sides = new Map();
  for (const id of cards.result) {
    const row = cards.resources.DOCUMENT[id];
    const list = state.sides.get(row.parent) || [];
    list.push({id, name: row.name, content: row.content || ''});
    state.sides.set(row.parent, list);
  }
  state.admin = admin.result.length > 0;
  state.pages = pages.result.map(id => {
    const row = pages.resources.DOCUMENT[id];
    return {id, name: row.name, content: row.content, parent: row.parent || '', order: row.order || '', author: row.author || '', slug: row.slug || '', hidden: row.hidden === 'Yes'};
  });
  state.viewer = person.result[0] || '';
  const self = state.viewer ? person.resources.PERSON[state.viewer] : null;
  const name = self ? self.name_show : viewer.email;
  state.user = {
    email: viewer.email,
    name,
    initial: name[0].toUpperCase(),
    photoUrl: photo.result.length ? `/api/blob/${photo.result[0]}/thumbnail` : '',
  };
}

export function me() {
  return state.user;
}

export function page(id) {
  return state.pages.find(p => p.id === id) || null;
}

export function pageAt(key) {
  const found = page(key) || state.pages.find(p => p.slug === key);
  if (found) {
    return found;
  }
  const moved = state.moved.get(`/p/${key}`);
  return moved ? page(moved.slice('/p/'.length)) : null;
}

export function pagePath(p) {
  return `/p/${p.slug || p.id}`;
}

export function editPath(p) {
  return `${pagePath(p)}/edit`;
}

function bySiblingOrder(a, b) {
  if (a.order !== b.order) {
    if (!a.order || !b.order) {
      return a.order ? -1 : 1;
    }
    return a.order < b.order ? -1 : 1;
  }
  return a.name.localeCompare(b.name);
}

export function mine(p) {
  return Boolean(state.viewer) && p.author === state.viewer;
}

export function listed(p) {
  return !p.hidden || mine(p) || state.admin;
}

export function childrenOf(id) {
  return state.pages.filter(p => p.parent === id && listed(p)).sort(bySiblingOrder);
}

export function hasChildren(p) {
  return state.pages.some(c => c.parent === p.id);
}

export function trail(p) {
  const out = [];
  for (let at = page(p.parent); at; at = page(at.parent)) {
    out.unshift(at);
  }
  return out;
}

export function under(p, ancestor) {
  return trail(p).some(a => a.id === ancestor.id);
}

export function setHidden(id, hidden) {
  return api('POST', '/api/q', {batch: [{set: id, cells: {hidden}}]});
}

export function setOrder(id, key) {
  return api('POST', '/api/q', {batch: [{set: id, cells: {order: key}}]});
}

export async function body(p) {
  if (!p.content) {
    return '';
  }
  if (!bodies.has(p.content)) {
    const res = await fetch(`/api/blob/${p.content}/blob`);
    if (!res.ok) {
      throw new Error(`Couldn’t read the page (${res.status}).`);
    }
    bodies.set(p.content, await res.text());
  }
  return bodies.get(p.content);
}

export const picturePath = '/api/wiki/picture/';

export function splitHeader(md) {
  const front = md.match(/^---\n([\s\S]*?)\n---\n/);
  if (!front) {
    return {header: '', text: md};
  }
  const line = front[1].split('\n').find(l => l.startsWith('header_image:'));
  const value = line ? line.slice('header_image:'.length).trim() : '';
  return {header: value.startsWith(picturePath) ? value : '', text: md.slice(front[0].length)};
}

const backgrounds = ['backpacking', 'building', 'camping', 'library', 'math', 'music', 'nature', 'science', 'writing'];

export function defaultHeader(p) {
  const top = trail(p)[0] || p;
  let sum = 0;
  for (const c of top.id) {
    sum = (sum * 31 + c.charCodeAt(0)) >>> 0;
  }
  return `/backgrounds/${backgrounds[sum % backgrounds.length]}.jpg`;
}

export async function headerFor(p) {
  const chain = [p, ...trail(p).reverse()];
  const texts = await Promise.all(chain.map(body));
  for (const md of texts) {
    const {header} = splitHeader(md);
    if (header) {
      return header;
    }
  }
  return defaultHeader(p);
}

export function firstSentence(md) {
  let fenced = false;
  for (const line of splitHeader(md).text.split('\n')) {
    if (line.startsWith('```')) {
      fenced = !fenced;
      continue;
    }
    if (fenced || /^\s*#{1,6}\s/.test(line)) {
      continue;
    }
    const text = line
      .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
      .replace(/^\s*(#{1,6}\s|>\s?|[-*]\s+|\d+[.)]\s+)/, '')
      .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/\*\*|\*|`/g, '')
      .trim();
    if (!text) {
      continue;
    }
    const end = text.search(/[.!?](\s|$)/);
    return end < 0 ? text : text.slice(0, end + 1);
  }
  return '';
}

export function joinHeader(header, text) {
  if (!header) {
    return text;
  }
  return `---\nheader_image: ${header}\n---\n${text}`;
}

export async function setHeader(id, header) {
  await loadModel();
  const p = page(id);
  const {text} = splitHeader(await body(p));
  return save(p.id, p.parent, p.slug, p.name, joinHeader(header, text));
}

export function save(document, parent, slug, name, text, sides) {
  return api('POST', '/api/do/wiki', {document, parent, slug, name, body: text, sides});
}

export function remove(p) {
  return api('POST', '/api/q', {batch: [...sidesOf(p).map(card => ({delete: card.id})), {delete: p.id}]});
}
