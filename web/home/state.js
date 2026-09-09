export const state = {model: null};

export function applyModel(model) {
  state.model = model;
}

export function isAdmin() {
  return Boolean(state.model && state.model.user.isAdmin);
}

export function categoryTitles() {
  return state.model.categories.map(c => c.title);
}
