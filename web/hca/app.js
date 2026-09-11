import {applyModel, resolvePath, activityPath, isFamily} from './state.js';
import {el} from './dom.js';
import {initChrome, renderChrome, setTitle, clearSearch} from './chrome.js';
import {initModal} from './edit.js';
import {signUpPage} from './pages/signup.js';
import {myPage} from './pages/my.js';
import {calendarPage} from './pages/calendar.js';
import {activityPage} from './pages/detail.js';
import {adminPage} from './pages/admin.js';
import {approvalsPage} from './pages/approvals.js';

export async function load() {
  const res = await fetch('/api/events/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  applyModel(await res.json());
  renderChrome();
  render();
}

export function navigate(path) {
  history.pushState(null, '', path);
  render();
  document.querySelector('#main').scrollTo(0, 0);
}

function notFound(what) {
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Not here'), el('p', 'row-text', `${what} is not in the portal, or is not something you can see.`));
  setTitle('Not here');
  return page;
}

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  if (!parts.length) {
    return signUpPage(null);
  }
  switch (parts[0]) {
    case 'years':
      return signUpPage(parts[1] || null);
    case 'my':
      // /my/{email} is a household member's sign-ups; anyone else's is not a
      // page.
      if (parts[1] && !isFamily(parts[1])) {
        return notFound('That person');
      }
      return myPage(parts[1] || null);
    case 'calendar':
      return calendarPage();
    case 'admin':
      return adminPage();
    case 'approvals':
      return approvalsPage();
    case 'activities':
    case 'v': {
      // /activities/{id}/... and /v/{pretty}/... both reach one page per node,
      // through the Redirects tab when an address has since changed; the bar
      // is corrected so the address people copy next is the live one.
      const act = resolvePath(location.pathname);
      if (act && activityPath(act) !== location.pathname) {
        history.replaceState(null, '', activityPath(act));
      }
      return act ? activityPage(act) : notFound(parts[0] === 'v' ? 'That address' : 'That activity');
    }
  }
  return notFound('That page');
}

export function render() {
  const page = document.querySelector('#page');
  page.className = '';
  clearSearch();
  page.replaceChildren(route());
  // After the page, so the rail's active item and its per-category counts
  // reflect where we just landed and what that page filtered to.
  renderChrome();
}

document.addEventListener('click', e => {
  const a = e.target.closest('a[data-link]');
  if (!a || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) {
    return;
  }
  e.preventDefault();
  navigate(a.getAttribute('href'));
});

window.addEventListener('popstate', render);
document.addEventListener('hca:refresh', render);

initChrome();
initModal();
load();
