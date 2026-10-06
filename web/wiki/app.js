import {state, loadModel} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {startApp, load, notFound} from '/router.js';
import {initModal} from '/modal.js';
import {listPage, viewPage, editPage} from './pages.js';

const fixed = {
  '': () => listPage(),
  new: () => editPage(''),
  p: parts => {
    if (parts[2] === 'edit') {
      return editPage(parts[1] || '');
    }
    return parts[2] ? notFound('That page') : viewPage(parts[1] || '');
  },
};

const routes = {...fixed};

async function model() {
  await loadModel();
  for (const key of Object.keys(routes)) {
    if (!(key in fixed)) {
      delete routes[key];
    }
  }
  for (const p of state.pages) {
    if (p.slug) {
      routes[p.slug] = parts => (parts[1] ? notFound('That page') : viewPage(p.id));
    }
  }
}

initChrome();
initModal(load);
startApp({
  model,
  routes,
  missing: 'is not in the wiki.',
  prepare: clearSearch,
  show: showPage,
});
