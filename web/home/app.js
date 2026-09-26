import {state, applyModel, setSuperAdmin, superOn} from './state.js';
import {el} from './dom.js';
import {renderCategories, renderNav} from './cards.js';
import {renderMonth} from './month.js';
import {renderWidgets} from './widgets.js';
import {initEditing, refreshCategoryManager} from './edit.js';
import {onSlash} from '/toolbar.js';
import {initTopbar, renderAccount, searchInput} from '/shell.js';

const editCategories = el('button', 'user-menu-super', 'Edit Categories');
editCategories.type = 'button';
editCategories.id = 'edit-categories';
editCategories.hidden = true;

function renderChrome() {
  renderAccount();
  editCategories.hidden = !superOn();
}

export async function load() {
  const res = await fetch('/api/apps/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  applyModel(await res.json());
  renderChrome();
  renderNav();
  renderCategories(searchInput().value);
  refreshCategoryManager();
  // The rail's month is drawn last: it is fed by another app's model, and a
  // failure there must not cost the links and categories drawn above it.
  renderMonth();
  renderWidgets(searchInput().value);
}

function initSearch() {
  const search = searchInput();
  search.addEventListener('input', () => {
    renderCategories(search.value);
    renderWidgets(search.value);
  });
  search.addEventListener('keydown', e => {
    if (e.key === 'Escape' && search.value) {
      e.stopPropagation();
      search.value = '';
      renderCategories('');
      renderWidgets('');
    }
  });
  onSlash(() => search.focus());
}

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
  initTopbar({
    name: 'Heliosian',
    me: () => state.model.user,
    alerts: () => state.model.alerts,
    isAdmin: () => state.model.user.isAdmin,
    superOn: () => state.superAdmin,
    onSuper: on => {
      setSuperAdmin(on);
      renderChrome();
      renderNav();
      renderCategories(searchInput().value);
      renderWidgets(searchInput().value);
    },
    search: {placeholder: 'Search apps, links, or events…', own: true},
    menuRows: [editCategories],
  });
  initDrawer();
}

initChrome();
initSearch();
initEditing();
load();
