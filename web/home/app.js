import {state, loadModel, isAdmin} from './state.js';
import {el} from '/elements.js';
import {renderCategories, renderNav} from './cards.js';
import {renderMonth} from './month.js';
import {renderWidgets} from './widgets.js';
import {initEditing, refreshPanels, openEditPanel} from './edit.js';
import {initTopbar, renderAccount, searchInput, onSlash} from '/shell.js';
import {startApp} from '/router.js';
import {adminPage} from './adminpage.js';

const editPage = el('button', 'user-menu-super', 'Edit Page');
editPage.type = 'button';
editPage.hidden = true;
editPage.addEventListener('click', openEditPanel);

function renderChrome() {
  renderAccount();
  editPage.hidden = !isAdmin();
}

const main = document.querySelector('#main');
const view = [...main.children];

function paintHome() {
  renderNav();
  renderCategories(searchInput().value);
  refreshPanels();
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
    search: {placeholder: 'Search apps, links, or events…', own: true},
    menuRows: [editPage],
  });
  initDrawer();
}

initChrome();
initSearch();
initEditing();
startApp({
  model: loadModel,
  routes: {
    '': homePage,
    admin: () => adminPage(),
  },
  missing: 'is not on Heliosian.',
  prepare: renderChrome,
  show: node => main.replaceChildren(node),
});
