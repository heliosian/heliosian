import {loadModel, event, fetchEvent, today, parseDate, state, eventDates, allCalendars, setActiveFeed, setClassrooms, setTags, feedClassrooms, feedTags} from './state.js';
import {initChrome, clearSearch} from './chrome.js';
import {showPage} from '/shell.js';
import {startApp, load, loadRow, notFound} from '/router.js';
import {initModal} from '/modal.js';
import {homePage} from './pages/home.js';
import {eventPage} from './pages/event.js';
import {adminPage} from './pages/admin.js';
import {minePage} from './pages/mine.js';

function eventRoute(parts) {
  const id = parts.slice(1).join('/');
  return loadRow(id, {
    find: event,
    load: fetchEvent,
    settle: e => {
      if (e.path !== location.pathname) {
        history.replaceState(null, '', e.path + location.search);
      }
    },
    show: e => {
      state.day = eventDates(e)[0];
      return eventPage(e);
    },
    missing: 'That event',
  });
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
  g: eventRoute,
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
