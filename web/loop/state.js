import {superEditOn} from '/superedit.js';

export const state = {model: null, allGroups: []};

const byName = new Map();

export function applyModel(model) {
  state.model = model;
  state.allGroups = model.groups;
  applySuperEdit();
}

export function applySuperEdit() {
  state.model.groups = isAdmin() || !isSystemAdmin() ? state.allGroups : state.allGroups.filter(g => g.open || g.mine);
  byName.clear();
  for (const g of state.model.groups) {
    byName.set(g.name, g);
  }
}

export function me() {
  return state.model.user;
}

export function isSystemAdmin() {
  return Boolean(state.model && state.model.user.isAdmin);
}

export function isAdmin() {
  return isSystemAdmin() && superEditOn();
}

export function managed(g) {
  return g.mine || isAdmin();
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
