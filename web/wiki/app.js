import {loadModel, pageAt, pagePath} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {startApp, load, notFound} from '/router.js';
import {initModal} from '/modal.js';
import {listPage, viewPage, editPage} from './pages.js';

const routes = {
  '': () => listPage(),
  new: () => editPage(''),
  p: parts => {
    const p = pageAt(parts[1] || '');
    if (!p || (parts[2] && parts[2] !== 'edit')) {
      return notFound('That page');
    }
    const here = pagePath(p) + (parts[2] ? '/edit' : '');
    if (location.pathname !== here) {
      history.replaceState(history.state, '', here + location.search + location.hash);
    }
    return parts[2] ? editPage(p.id) : viewPage(p.id);
  },
};

initChrome();
initModal(load);
startApp({
  model: loadModel,
  routes,
  missing: 'is not in the wiki.',
  prepare: clearSearch,
  show: showPage,
});
