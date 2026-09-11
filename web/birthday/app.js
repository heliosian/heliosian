import {applyModel, staff, charity} from './state.js';
import {el} from './dom.js';
import {initChrome, renderChrome, setTitle} from './chrome.js';
import {initModal} from './edit.js';
import {jobsPage} from './pages/jobs.js';
import {processPage} from './pages/process.js';
import {calendarPage} from './pages/calendar.js';
import {staffPage} from './pages/staff.js';
import {charitiesPage, charityPage} from './pages/charities.js';
import {newslettersPage} from './pages/newsletters.js';
import {skippedPage} from './pages/skipped.js';
import {adminPage} from './pages/admin.js';

export async function load() {
  const res = await fetch('/api/birthday/model');
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
  page.append(el('h1', '', 'Not here'), el('p', 'row-text', `${what} is not in the app.`));
  setTitle('Not here');
  return page;
}

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  if (!parts.length) {
    return jobsPage();
  }
  switch (parts[0]) {
    case 'process':
      return processPage();
    case 'calendar':
      return calendarPage();
    case 'charities': {
      if (!parts[1]) {
        return charitiesPage();
      }
      const c = charity(parts[1]);
      return c ? charityPage(c) : notFound(parts[1]);
    }
    case 'newsletters':
      return newslettersPage();
    case 'skipped':
      return skippedPage();
    case 'admin':
      return adminPage();
    case 'staff': {
      const sv = staff(parts[1]);
      return sv ? staffPage(sv) : notFound(parts[1] || 'That person');
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
