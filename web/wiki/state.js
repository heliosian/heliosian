import {api} from '/api.js';
import {me as whoAmI} from '/data.js';

export const state = {pages: [], sides: new Map(), user: null, viewer: '', admin: false, imageSearch: false};

export function sidesOf(p) {
  return state.sides.get(p.id) || [];
}

const bodies = new Map();

function query(tree) {
  return api('QUERY', '/api/q', tree);
}

const viewerIs = column => ({'=': [{path: column}, {path: '@viewer'}]});

export async function loadModel() {
  const [viewer, pages, person, photo, admin, images, cards] = await Promise.all([
    whoAmI(),
    query({from: 'DOCUMENT', where: [{'=': [{path: 'kind'}, 'wiki']}], order: [{path: 'name', dir: 'asc'}]}),
    query({from: 'PERSON', where: [viewerIs('id')]}),
    query({from: 'PHOTO', where: [viewerIs('person'), {path: 'ready'}], order: [{path: 'order', dir: 'asc'}], limit: 1}),
    query({from: 'PERSON', where: [viewerIs('id'), {admin_of: ['wiki']}]}),
    api('GET', '/api/wiki/images'),
    query({from: 'DOCUMENT', where: [{'=': [{path: 'relation'}, 'side']}], order: [{path: 'order', dir: 'asc'}]}),
  ]);
  state.imageSearch = images.search;
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
    return {id, name: row.name, content: row.content, parent: row.parent || '', order: row.order || '', author: row.author || '', hidden: row.hidden === 'Yes', slug: row.slug || '', image: row.image || ''};
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

export function pagePath(p) {
  return p.slug ? `/${p.slug}` : `/p/${p.id}`;
}

export function editPath(p) {
  return `/p/${p.id}/edit`;
}

export function pageAt(slug) {
  return state.pages.find(p => p.slug === slug) || null;
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

export function imageOf(p) {
  for (let at = p; at; at = page(at.parent)) {
    if (at.image) {
      return `/api/blob/${at.id}/image`;
    }
  }
  return '';
}

export function setImage(id, image) {
  return api('POST', '/api/q', {batch: [{set: id, cells: {image}}]});
}

export function setHidden(id, hidden) {
  return api('POST', '/api/q', {batch: [{set: id, cells: {hidden}}]});
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

export function save(document, parent, slug, name, text, sides) {
  return api('POST', '/api/do/wiki', {document, parent, slug, name, body: text, sides});
}

export function remove(p) {
  return api('POST', '/api/q', {batch: [...sidesOf(p).map(card => ({delete: card.id})), {delete: p.id}]});
}
