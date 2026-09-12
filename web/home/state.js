// superAdmin is the admin's Super Admin Mode switch in the account menu,
// remembered per browser: on, the page shows hidden links and the add cards;
// off, it reads as everyone else sees it.
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
    // A browser that refuses storage just forgets the switch on reload.
  }
}

export function applyModel(model) {
  state.model = model;
}

export function isAdmin() {
  return Boolean(state.model && state.model.user.isAdmin);
}

export function categoryTitles() {
  return state.model.categories.map(c => c.title);
}

// The categories a link can sit in - every one but the events section.
export function linkCategoryTitles() {
  return state.model.categories.filter(c => c.style !== 'events').map(c => c.title);
}
