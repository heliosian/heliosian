import {state, applyModel, resolvePath, redirectTarget, activityPath, isFamily, isAdmin} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {api} from '/api.js';
import {initModal} from '/modal.js';
import {startApp, load, notFound} from '/router.js';
import {signUpPage} from './pages/signup.js';
import {myPage} from './pages/my.js';
import {calendarPage} from './pages/calendar.js';
import {activityPage} from './pages/detail.js';
import {adminPage} from './pages/admin.js';
import {approvalsPage} from './pages/approvals.js';

function activity(parts) {
  const act = resolvePath(location.pathname);
  if (act && activityPath(act) !== location.pathname) {
    history.replaceState(null, '', activityPath(act));
  }
  return act ? activityPage(act) : notFound(parts[0] === 'v' ? 'That address' : 'That activity');
}

const routes = {
  '': () => signUpPage(null),
  years: parts => signUpPage(parts[1] || null),
  my: parts => parts[1] && !isFamily(parts[1]) ? notFound('That person') : myPage(parts[1] || null),
  calendar: () => calendarPage(),
  admin: () => adminPage(),
  approvals: () => isAdmin() ? approvalsPage() : notFound('That page'),
  activities: activity,
  v: activity,
};

initChrome();
initModal(load);
startApp({
  model: async () => applyModel(await api('GET', '/api/team/model')),
  routes,
  missing: 'is not in the portal, or is not something you can see.',
  redirect: redirectTarget,
  prepare: () => {
    state.category = '';
    clearSearch();
  },
  show: showPage,
});
