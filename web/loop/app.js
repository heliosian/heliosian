import {loadModel, group, fetchGroup} from './state.js';
import {initChrome} from './chrome.js';
import {el} from '/elements.js';
import {showPage, clearSearch} from '/shell.js';
import {startApp, load, render, notFound} from '/router.js';
import {initModal} from '/modal.js';
import {groupsPage} from './pages/groups.js';
import {groupPage, newGroupModal} from './pages/group.js';

const missing = new Set();

const routes = {
  '': () => groupsPage(),
  new: () => {
    setTimeout(newGroupModal);
    return groupsPage();
  },
  groups: parts => {
    const key = parts[1] || '';
    const g = group(key);
    if (g) {
      return groupPage(g);
    }
    if (!key || missing.has(key)) {
      return notFound(key || 'That group');
    }
    fetchGroup(key).then(found => {
      if (!found) {
        missing.add(key);
      }
      render();
    });
    return el('div', 'list-page');
  },
};

initChrome();
initModal(load);
startApp({
  model: loadModel,
  routes,
  missing: 'is not in the app.',
  prepare: clearSearch,
  show: showPage,
});
