export const state = {model: null, superAdmin: readSuperAdmin()};

function readSuperAdmin() {
  try {
    return localStorage.getItem('heliosian.superAdmin') === '1';
  } catch {
    return false;
  }
}

export function setSuperAdmin(on) {
  state.superAdmin = on;
  try {
    localStorage.setItem('heliosian.superAdmin', on ? '1' : '0');
  } catch {
    return;
  }
}

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

export function categoryTitles() {
  return state.model.categories.map(c => c.title);
}

export function linkCategoryTitles() {
  return state.model.categories.filter(c => c.style !== 'events').map(c => c.title);
}
