import {state, applyModel, isSystemAdmin, isAdmin} from './state.js';
import {el} from '/elements.js';
import {renderCategories, renderNav} from './cards.js';
import {renderMonth} from './month.js';
import {renderWidgets} from './widgets.js';
import {initEditing, refreshCategoryManager} from './edit.js';
import {onSlash} from '/toolbar.js';
import {initTopbar, renderAccount, searchInput} from '/shell.js';
import {api} from '/api.js';
import {startApp, render} from '/router.js';
import {adminPage} from './adminpage.js';

const editCategories = el('button', 'user-menu-super', 'Edit Categories');
editCategories.type = 'button';
editCategories.id = 'edit-categories';
editCategories.hidden = true;

function renderChrome() {
  renderAccount();
  editCategories.hidden = !isAdmin();
}

const main = document.querySelector('#main');
const view = [...main.children];

function paintHome() {
  renderNav();
  renderCategories(searchInput().value);
  refreshCategoryManager();
  renderMonth();
  renderWidgets(searchInput().value);
}

function homePage() {
  // The widgets measure themselves, so paint once the router has mounted the view.
  queueMicrotask(paintHome);
  const page = document.createDocumentFragment();
  page.append(...view);
  return page;
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
    isSystemAdmin,
    onSuper: render,
    search: {placeholder: 'Search apps, links, or events…', own: true},
    menuRows: [editCategories],
  });
  initDrawer();
}

initChrome();
initSearch();
initEditing();
startApp({
  model: async () => applyModel(await api('GET', '/api/apps/model')),
  routes: {
    '': homePage,
    admin: () => adminPage(),
  },
  missing: 'is not on Heliosian.',
  prepare: renderChrome,
  show: node => main.replaceChildren(node),
});
