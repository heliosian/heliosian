import {api} from '/api.js';

function result(reply) {
  const byType = reply.resources;
  const byId = {};
  for (const resources of Object.values(byType)) {
    Object.assign(byId, resources);
  }
  const get = id => byId[id];
  return {
    result: reply.result,
    now: reply.now,
    get,
    all: type => Object.values(byType[type] || {}),
    follow(resource, relation) {
      const target = resource[relation];
      if (Array.isArray(target)) {
        return target.map(get);
      }
      return target ? get(target) : null;
    },
  };
}

export async function query(path) {
  return result(await api('GET', path));
}

export async function batch(paths) {
  const body = {};
  for (const [name, path] of Object.entries(paths)) {
    body[name] = {path};
  }
  return result(await api('POST', '/api/query', body));
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

export function actAll(writes) {
  return api('POST', '/api/act', writes);
}

export function me() {
  return api('GET', '/api/me');
}
