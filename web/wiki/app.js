import {loadModel} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {startApp, load, notFound} from '/router.js';
import {initModal} from '/modal.js';
import {listPage, viewPage, editPage} from './pages.js';

const routes = {
  '': () => listPage(),
  new: () => editPage(''),
  p: parts => {
    if (parts[2] === 'edit') {
      return editPage(parts[1] || '');
    }
    return parts[2] ? notFound('That page') : viewPage(parts[1] || '');
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
