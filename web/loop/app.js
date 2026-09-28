import {applyModel, group} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {api} from '/api.js';
import {startApp, notFound} from '/router.js';
import {groupsPage} from './pages/groups.js';
import {groupPage, newGroupModal} from './pages/group.js';
import {adminPage} from './pages/admin.js';

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
  admin: () => adminPage(),
};

initChrome();
startApp({
  model: async () => applyModel(await api('GET', '/api/loop/model')),
  routes,
  missing: 'is not in the app.',
  prepare: clearSearch,
  show: showPage,
});
