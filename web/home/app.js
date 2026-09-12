import {state, applyModel, setSuperAdmin} from './state.js';
import {renderCategories, renderNav} from './cards.js';
import {initEditing, refreshCategoryManager} from './edit.js';
import {renderAvatars, renderAlerts, onSlash, initAppSwitch, markSuper} from '/toolbar.js';

function renderChrome() {
  const user = state.model.user;
  // The same hero photo the directory leads with (their own, else their
  // family's); the initial only stands in when there is no photo at all.
  renderAvatars({photoUrl: user.photoUrl && user.photoUrl + '?thumb=1', initial: user.initial});
  renderAlerts(state.model.alerts || {});
  document.querySelector('.user-menu-email').textContent = user.email;
  for (const item of document.querySelectorAll('.user-menu-admin')) {
    item.hidden = !user.isAdmin;
  }
  document.querySelector('#super-admin-mode').checked = state.superAdmin;
  markSuper(state.superAdmin);
}

export async function load() {
  const res = await fetch('/api/apps/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  applyModel(await res.json());
  renderChrome();
  renderNav();
  renderCategories(document.querySelector('#search').value);
  refreshCategoryManager();
}

function initSearch() {
  const search = document.querySelector('#search');
  search.addEventListener('input', () => renderCategories(search.value));
  search.addEventListener('keydown', e => {
    if (e.key === 'Escape' && search.value) {
      // Swallow the key so the modal/menu handlers do not also fire on what
      // the user meant as "clear the box".
      e.stopPropagation();
      search.value = '';
      renderCategories('');
    }
  });
  onSlash(() => search.focus());
}

function initChrome() {
  initAppSwitch();
  const menu = document.querySelector('#user-menu');
  document.querySelector('#user').addEventListener('click', e => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
  });
  // The switch is a row of the menu; flipping it repaints the links and
  // leaves the menu open, so the effect is visible behind it.
  const superAdmin = document.querySelector('#super-admin-mode');
  superAdmin.addEventListener('change', () => {
    setSuperAdmin(superAdmin.checked);
    markSuper(superAdmin.checked);
    renderNav();
    renderCategories(document.querySelector('#search').value);
  });
  superAdmin.closest('label').addEventListener('click', e => e.stopPropagation());
  document.addEventListener('click', () => {
    menu.hidden = true;
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      menu.hidden = true;
    }
  });
}

initChrome();
initSearch();
initEditing();
load();
