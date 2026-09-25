import {state, applyModel, setSuperAdmin, superOn} from './state.js';
import {renderCategories, renderNav} from './cards.js';
import {renderMonth} from './month.js';
import {initEditing, refreshCategoryManager} from './edit.js';
import {renderAvatars, renderAlerts, renderProfileLink, onSlash, initAppSwitch, initUserMenu, initSpoof, renderSuperToggle} from '/toolbar.js';

function renderChrome() {
  const user = state.model.user;
  // The same hero photo the directory leads with (their own, else their
  // family's); the initial only stands in when there is no photo at all.
  renderAvatars({photoUrl: user.photoUrl && user.photoUrl + '?thumb=1', initial: user.initial});
  renderAlerts(state.model.alerts || {});
  renderProfileLink(user.email);
  document.querySelector('.user-menu-email').textContent = user.email;
  for (const item of document.querySelectorAll('.user-menu-admin')) {
    item.hidden = !user.isAdmin;
  }
  // Edit Categories is an edit, so it waits for the pencil; Admin Tools
  // above does not.
  for (const item of document.querySelectorAll('.user-menu-super')) {
    item.hidden = !superOn();
  }
  // The pencil repaints the links in or out of Super Admin Mode.
  renderSuperToggle({show: user.isAdmin, on: state.superAdmin, onToggle: on => {
    setSuperAdmin(on);
    renderChrome();
    renderNav();
    renderCategories(document.querySelector('#search').value);
  }});
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
  // The rail's month is drawn last, after everything the page is for: it is
  // the one part fed by another app's model, and drawing it first once cost
  // the whole page - a field the calendar had stopped sending left the month
  // throwing, and the links and every category never ran.
  renderMonth();
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

// On a phone the rail is a drawer behind the toolbar's hamburger: open on
// the button, closed on the backdrop, a section link, or Escape.
function setDrawer(open) {
  document.body.classList.toggle('drawer-open', open);
  document.querySelector('#drawer-overlay').hidden = !open;
}

function initDrawer() {
  document.querySelector('#menu-button').addEventListener('click', () => setDrawer(!document.body.classList.contains('drawer-open')));
  document.querySelector('#drawer-overlay').addEventListener('click', () => setDrawer(false));
  document.querySelector('#app-nav').addEventListener('click', e => {
    if (e.target.closest('a')) {
      setDrawer(false);
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      setDrawer(false);
    }
  });
}

function initChrome() {
  initAppSwitch();
  initDrawer();
  initUserMenu();
  initSpoof();
  const menu = document.querySelector('#user-menu');
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
