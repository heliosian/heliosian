import {applyModel, event, today, parseDate} from './state.js';
import {el} from './dom.js';
import {initChrome, renderChrome, setTitle, clearSearch} from './chrome.js';
import {homePage} from './pages/home.js';
import {eventPage} from './pages/event.js';
import {feedsPage} from './pages/feeds.js';

export async function load() {
  const res = await fetch('/api/calendar/model');
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
  page.append(el('h1', '', 'Not here'), el('p', 'row-text', `${what} is not on the calendar.`));
  setTitle('Not here');
  return page;
}

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  if (!parts.length) {
    return homePage(today());
  }
  switch (parts[0]) {
    case 'day':
      return parseDate(parts[1]) ? homePage(parts[1]) : notFound('That day');
    case 'events': {
      const e = event(parts.slice(1).join('/'));
      return e ? eventPage(e) : notFound('That event');
    }
    case 'feeds':
      return feedsPage();
  }
  return notFound('That page');
}

export function render() {
  const page = document.querySelector('#page');
  page.className = '';
  clearSearch();
  page.replaceChildren(route());
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
document.addEventListener('calendar:refresh', render);

initChrome();
load();
