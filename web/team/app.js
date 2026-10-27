import {state, loadModel, resolvePath, resolveAlias, isFamily, allows} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {initModal} from '/modal.js';
import {startApp, load, loadRow, notFound} from '/router.js';
import {signUpPage} from './pages/signup.js';
import {myPage} from './pages/my.js';
import {calendarPage} from './pages/calendar.js';
import {activityPage} from './pages/detail.js';
import {adminPage} from './pages/admin.js';
import {approvalsPage} from './pages/approvals.js';

function activity() {
  return loadRow(location.pathname, {
    find: resolvePath,
    load: resolveAlias,
    settle: found => {
      if (found.path !== location.pathname) {
        history.replaceState(null, '', found.path);
      }
    },
    show: activityPage,
    missing: 'That activity',
  });
}

const routes = {
  '': () => signUpPage(null),
  years: parts => signUpPage(parts[1] || null),
  my: parts => parts[1] && !isFamily(parts[1]) ? notFound('That person') : myPage(parts[1] || null),
  calendar: () => calendarPage(),
  admin: () => adminPage(),
  approvals: () => allows('team.curate') ? approvalsPage() : notFound('That page'),
  g: activity,
};

initChrome();
initModal(load);
startApp({
  model: loadModel,
  routes,
  missing: 'is not in the portal, or is not something you can see.',
  prepare: () => {
    state.category = '';
    clearSearch();
  },
  show: showPage,
});
