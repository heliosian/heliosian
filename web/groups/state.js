export const state = {model: null, adminTab: ''};

const byName = new Map();

export function applyModel(model) {
  state.model = model;
  byName.clear();
  for (const g of model.groups) {
    byName.set(g.name, g);
  }
}

export function me() {
  return state.model.user;
}

export function isAdmin() {
  return state.model.user.isAdmin;
}

export function group(name) {
  return byName.get(name) || null;
}

export function groupPath(g) {
  return `/groups/${encodeURIComponent(g.name)}`;
}

export function options() {
  return state.model.options;
}

export function matches(g, query) {
  if (!query) {
    return true;
  }
  return `${g.title} ${g.address} ${g.description || ''}`.toLowerCase().includes(query);
}
