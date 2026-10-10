import {loadModel, group} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {startApp, load, notFound} from '/router.js';
import {initModal} from '/modal.js';
import {groupsPage} from './pages/groups.js';
import {groupPage, newGroupModal} from './pages/group.js';
import {appOrigin} from '/appswitch.js';

const routes = {
  '': () => groupsPage(),
  new: () => {
    setTimeout(newGroupModal);
    return groupsPage();
  },
  groups: parts => {
    const g = group(parts[1] || '');
    return g ? groupPage(g) : notFound(parts[1] || 'That email list');
  },
  admin: () => {
    location.replace(appOrigin('who') + '/groups/loop-admins');
    return groupsPage();
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
