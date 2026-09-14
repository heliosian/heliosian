import {state, applyModel, staff, charity, isUnassigned, isSystemAdmin} from './state.js';
import {el} from './dom.js';
import {initChrome, renderChrome, setTitle, clearSearch} from './chrome.js';
import {initModal, offerTeam} from './edit.js';
import {jobsPage} from './pages/jobs.js';
import {processPage} from './pages/process.js';
import {calendarPage} from './pages/calendar.js';
import {staffPage} from './pages/staff.js';
import {charitiesPage, charityPage} from './pages/charities.js';
import {newslettersPage, newsletterPage} from './pages/newsletters.js';
import {skippedPage} from './pages/skipped.js';
import {unassignedPage} from './pages/unassigned.js';
import {adminPage} from './pages/admin.js';

let offered = false;

export async function load() {
  const res = await fetch('/api/birthday/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  applyModel(await res.json());
  render();
  // Once the first page is up, the newcomer's question.
  if (!offered && location.pathname !== '/admin') {
    offered = true;
    offerTeam();
  }
}

export function navigate(path) {
  history.pushState(null, '', path);
  render();
  document.querySelector('#main').scrollTo(0, 0);
}

function notFound(what) {
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Not here'), el('p', 'row-text', `${what} is not in the app.`));
  setTitle('Not here');
  return page;
}

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  // The front page is the unassigned birthdays while there are any, and
  // My Jobs once everyone has someone.
  if (!parts.length) {
    return state.model.staff.some(isUnassigned) ? unassignedPage() : jobsPage();
  }
  switch (parts[0]) {
    case 'jobs':
      return jobsPage();
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
      return parts[1] && state.model.newsletterDates.includes(parts[1]) ? newsletterPage(parts[1]) : parts[1] ? notFound(parts[1]) : newslettersPage();
    case 'skipped':
      return isSystemAdmin() ? skippedPage() : notFound('That page');
    case 'unassigned':
      return unassignedPage();
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
  const page = document.querySelector('#page');
  page.className = '';
  clearSearch();
  page.replaceChildren(route());
  // Admin Tools is its own window: the shell's rail, toolbar and tab bar
  // step aside for the admin chrome (see pages/admin.js).
  document.body.classList.toggle('is-admin', location.pathname === '/admin');
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
document.addEventListener('birthday:refresh', render);

initChrome();
initModal();
load();
