import {loadModel, group, fetchGroup} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {startApp, load, loadRow} from '/router.js';
import {initModal} from '/modal.js';
import {groupsPage} from './pages/groups.js';
import {groupPage, newGroupModal} from './pages/group.js';

const routes = {
  '': () => groupsPage(),
  new: () => {
    setTimeout(newGroupModal);
    return groupsPage();
  },
  g: parts => {
    const key = parts[1] || '';
    return loadRow(key, {
      find: group,
      load: fetchGroup,
      show: groupPage,
      missing: key || 'That group',
    });
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
