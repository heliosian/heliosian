import {api} from '/api.js';

let byType = {};
let byId = {};
let serverTime = '';

function load(reply) {
  byType = {};
  byId = {};
  for (const [type, resources] of Object.entries(reply.included)) {
    byType[type] = resources;
    Object.assign(byId, resources);
  }
  serverTime = reply.now;
  return reply.data;
}

export async function query(path) {
  return load(await api('GET', path));
}

export async function batch(paths) {
  const body = {};
  for (const [name, path] of Object.entries(paths)) {
    body[name] = {path};
  }
  return load(await api('POST', '/api/query', body));
}

export function get(id) {
  return byId[id];
}

export function all(type) {
  return Object.values(byType[type] || {});
}

export function follow(resource, relation) {
  const target = resource[relation];
  if (Array.isArray(target)) {
    return target.map(get);
  }
  return target ? get(target) : null;
}

export function now() {
  return serverTime;
}

export function act(type, id, action, body) {
  return api('POST', `/api/${type}/${encodeURIComponent(id)}/${action}`, body);
}

export function create(type, body) {
  return api('POST', `/api/${type}`, body);
}

export function remove(type, id) {
  return api('DELETE', `/api/${type}/${encodeURIComponent(id)}`);
}

export function me() {
  return api('GET', '/api/me');
}
