import {applyModel, event, fetchEvent, today, parseDate, me, state, eventDates, allCalendars, setActiveFeed, setClassrooms, setTags, feedClassrooms, feedTags} from './state.js';
import {el} from './dom.js';
import {initChrome, renderChrome, setTitle, clearSearch} from './chrome.js';
import {homePage} from './pages/home.js';
import {eventPage} from './pages/event.js';
import {feedsPage} from './pages/feeds.js';
import {adminPage} from './pages/admin.js';
import {minePage} from './pages/mine.js';

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

// appliedCalendar is the /c/{token} address whose calendar the filters
// were last set from, so arriving at one sets them once and the chips can
// be changed after that without every repaint setting them back.
let appliedCalendar = '';

// fetched is every event id asked of the server once, so a link to
// nothing is not asked again and again.
const fetched = new Set();

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  if (!parts.length) {
    return homePage(today());
  }
  switch (parts[0]) {
    // /c/{token} opens the calendar as one saved calendar (or My
    // Heliosian) sees it - the address the rail's rows set.
    case 'c': {
      const f = allCalendars().find(x => x.token === parts[1]);
      if (!f) {
        return notFound('That calendar');
      }
      if (appliedCalendar !== location.pathname) {
        appliedCalendar = location.pathname;
        setActiveFeed(f.token);
        setClassrooms(feedClassrooms(f));
        setTags(feedTags(f));
      }
      return homePage(today());
    }
    case 'day':
      return parseDate(parts[1]) ? homePage(parts[1]) : notFound('That day');
    case 'e':
    case 'events': {
      const id = parts.slice(1).join('/');
      const e = event(id);
      if (!e) {
        // An invite-only event is not in the model: fetch it by its link,
        // then draw the page again with it in hand.
        if (!fetched.has(id)) {
          fetched.add(id);
          fetchEvent(id).then(found => {
            if (found) {
              render();
            }
          });
          return el('div', 'list-page', 'Looking\u2026');
        }
        return notFound('That event');
      }
      // The rail's day jumps to the event's, so its month and its plan
      // and events are the ones beside the page.
      state.day = eventDates(e)[0];
      return eventPage(e);
    }
    case 'feeds':
      return feedsPage();
    case 'mine':
      return minePage();
    case 'admin':
      return me().isAdmin ? adminPage() : notFound('That page');
  }
  return notFound('That page');
}

export function render() {
  const page = document.querySelector('#page');
  page.className = '';
  clearSearch();
  page.replaceChildren(route());
  // Admin Tools is its own window: the shell's rail, toolbar and tab bar
  // step aside for the admin chrome (pages/admin.js).
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
document.addEventListener('calendar:refresh', render);
// A rail row clicked again re-applies its calendar even at the same address.
document.addEventListener('calendar:navigate', () => {
  appliedCalendar = '';
});

initChrome();
load();
