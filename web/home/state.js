import {superEditOn} from '/superedit.js';

export const state = {model: null};

export function applyModel(model) {
  state.model = model;
}

export function isSystemAdmin() {
  return Boolean(state.model && state.model.user.isAdmin);
}

// The switch outlives losing admin, a spoofed standard user or another person
// on the same device, so it counts only while the model says admin.
export function isAdmin() {
  return isSystemAdmin() && superEditOn();
}

export function tagLabelsOf(rule) {
  const named = state.model.tagLabels || {};
  return rule.tagLabels || (rule.tags || []).map(t => named[t] || t);
}

export function categoryTitles() {
  return state.model.categories.map(c => c.title);
}

export function linkCategoryTitles() {
  return state.model.categories.filter(c => c.style !== 'events').map(c => c.title);
}
