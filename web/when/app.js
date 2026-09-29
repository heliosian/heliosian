import {loadModel, event, eventPath, fetchEvent, today, parseDate, state, eventDates, allCalendars, setActiveFeed, setClassrooms, setTags, feedClassrooms, feedTags} from './state.js';
import {el} from '/elements.js';
import {initChrome, clearSearch} from './chrome.js';
import {showPage} from '/shell.js';
import {startApp, render, load, notFound} from '/router.js';
import {initModal} from '/modal.js';
import {homePage} from './pages/home.js';
import {eventPage} from './pages/event.js';
import {adminPage} from './pages/admin.js';
import {minePage} from './pages/mine.js';

const fetched = new Set();

function eventRoute(parts) {
  const id = parts.slice(1).join('/');
  const e = event(id);
  if (!e) {
    if (!fetched.has(id)) {
      fetched.add(id);
      fetchEvent(id).then(found => {
        if (found) {
          render();
        }
      });
      return el('div', 'list-page', 'Looking…');
    }
    return notFound('That event');
  }
  if (e.address && id !== e.address) {
    history.replaceState(null, '', eventPath(e) + location.search);
  }
  state.day = eventDates(e)[0];
  return eventPage(e);
}

const routes = {
  '': () => homePage(today()),
  c: parts => {
    const f = allCalendars().find(x => x.token === parts[1]);
    if (!f) {
      return notFound('That calendar');
    }
    if (state.appliedCalendar !== location.pathname) {
      state.appliedCalendar = location.pathname;
      setActiveFeed(f.token);
      setClassrooms(feedClassrooms(f));
      setTags(feedTags(f));
    }
    return homePage(today());
  },
  day: parts => parseDate(parts[1]) ? homePage(parts[1]) : notFound('That day'),
  e: eventRoute,
  events: eventRoute,
  mine: parts => minePage(parts[1] || ''),
  admin: () => adminPage(),
};

initChrome();
initModal(load);
startApp({
  model: loadModel,
  routes,
  missing: 'is not on the calendar.',
  prepare: clearSearch,
  show: showPage,
});
