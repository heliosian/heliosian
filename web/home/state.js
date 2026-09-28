export const state = {model: null};

export function applyModel(model) {
  state.model = model;
}

export function isAdmin() {
  return Boolean(state.model && state.model.user.isAdmin);
}

export function tagLabelsOf(rule) {
  const named = state.model.tagLabels || {};
  return rule.tagLabels || (rule.tags || []).map(t => named[t] || t);
}

export function linkCategories() {
  return state.model.categories.filter(c => c.style !== 'events');
}
