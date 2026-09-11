import {applyModel, activity} from './state.js';
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
      return myPage();
    case 'calendar':
      return calendarPage();
    case 'admin':
      return adminPage();
    case 'approvals':
      return approvalsPage();
    case 'activities': {
      // Everything under an activity is an activity with an id, so one shape of
      // URL reaches a headline event and a single shift alike.
      const act = activity(parts[1]);
      return act ? activityPage(act) : notFound('That activity');
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
