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
    let p = pageAt(parts.slice(1));
    const edit = !p && parts.at(-1) === 'edit';
    if (edit) {
      p = pageAt(parts.slice(1, -1));
    }
    if (!p) {
      return notFound('That page');
    }
    const here = pagePath(p) + (edit ? '/edit' : '');
    if (location.pathname !== here) {
      history.replaceState(history.state, '', here + location.search + location.hash);
    }
    return edit ? editPage(p.id) : viewPage(p.id);
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
