// superEdit is an admin's hat, as the other apps have it: off, they see and
// can do what anyone else can (plus the groups they manage); on, every
// group is theirs to open and edit. It is remembered per browser.
export const state = {model: null, adminTab: '', superEdit: readSuperEdit()};

function readSuperEdit() {
  try {
    return localStorage.getItem('loop.superEdit') === '1';
  } catch (err) {
    return false;
  }
}

export function setSuperEdit(on) {
  state.superEdit = on;
  try {
    localStorage.setItem('loop.superEdit', on ? '1' : '0');
  } catch (err) {
    // A browser that refuses storage just forgets the choice on reload.
  }
  applyModel(state.model);
}

const byName = new Map();

// The server sends an admin every group; with the hat off the page keeps
// only those anyone else would see (open), allGroups holding the whole list
// so the hat can bring them back.
export function applyModel(model) {
  state.model = model;
  model.allGroups = model.allGroups || model.groups;
  model.groups = isAdmin() || !isSystemAdmin() ? model.allGroups : model.allGroups.filter(g => g.open || g.mine);
  byName.clear();
  for (const g of model.groups) {
    byName.set(g.name, g);
  }
}

export function me() {
  return state.model.user;
}

// isSystemAdmin is the admin list itself, whatever the hat: what the
// pencil and Admin Tools go by.
export function isSystemAdmin() {
  return Boolean(state.model && state.model.user.isAdmin);
}

// isAdmin is an admin with the hat on - what the pages' admin powers go by.
export function isAdmin() {
  return isSystemAdmin() && state.superEdit;
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
