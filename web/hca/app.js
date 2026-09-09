import {applyModel, activity, findRole} from './state.js';
import {el} from './dom.js';
import {initChrome, renderChrome, setTitle} from './chrome.js';
import {initModal} from './edit.js';
import {signUpPage} from './pages/signup.js';
import {myPage} from './pages/my.js';
import {calendarPage} from './pages/calendar.js';
import {activityPage, rolePage} from './pages/detail.js';
import {allPage} from './pages/all.js';
import {peoplePage, personPage} from './pages/people.js';
import {adminPage} from './pages/admin.js';

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
  window.scrollTo(0, 0);
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
    case 'all':
      return allPage();
    case 'people':
      return parts[1] ? personPage(parts[1]) : peoplePage();
    case 'admin':
      return adminPage();
    case 'activities': {
      const act = activity(parts[1], parts[2]);
      if (!act) {
        return notFound(parts[2] || 'That activity');
      }
      if (parts[3] === 'roles') {
        const role = findRole(act, parts[4]);
        return role ? rolePage(act, role) : notFound(parts[4] || 'That role');
      }
      return activityPage(act);
    }
  }
  return notFound('That page');
}

export function render() {
  const main = document.querySelector('#page');
  main.className = '';
  main.replaceChildren(route());
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

initChrome();
initModal();
load();
