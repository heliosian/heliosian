import {eventsOn, today, addDays, longDayLabel, nextSpecials, selectedClassrooms, classroomNames} from '../state.js';
import {el, link, svg, button} from '../dom.js';
import {setTitle} from '../chrome.js';
import {eventRow, planCards, specialRow, emptyNote} from '../events.js';

// dayNav steps a day at a time, with a way back to today.
function dayNav(date) {
  const nav = el('div', 'day-nav');
  const prev = link('/day/' + addDays(date, -1), 'icon-button day-nav-arrow');
  prev.setAttribute('aria-label', 'Previous day');
  prev.append(svg('back'));
  const next = link('/day/' + addDays(date, 1), 'icon-button day-nav-arrow');
  next.setAttribute('aria-label', 'Next day');
  next.append(svg('chevron'));
  nav.append(prev, next);
  if (date !== today()) {
    nav.append(link('/', 'button button-secondary button-small', 'Today'));
  }
  return nav;
}

function roomsLine() {
  const rooms = selectedClassrooms();
  if (rooms.length === classroomNames().length) {
    return 'Every classroom';
  }
  return rooms.join(', ');
}

export function dayPage(date) {
  const isToday = date === today();
  setTitle(isToday ? 'Today' : longDayLabel(date));
  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', isToday ? 'Today' : longDayLabel(date)));
  main.append(el('p', 'page-intro', (isToday ? longDayLabel(date) + ' · ' : '') + roomsLine()));
  head.append(main, dayNav(date));
  page.append(head);

  page.append(el('h2', 'section-title', 'The day'));
  page.append(planCards(date));

  page.append(el('h2', 'section-title', 'Events'));
  const events = eventsOn(date);
  if (!events.length) {
    page.append(emptyNote('Nothing on the calendar for these classrooms and tags.'));
  } else {
    const list = el('div', 'event-list');
    for (const e of events) {
      list.append(eventRow(e));
    }
    page.append(list);
  }

  const ahead = nextSpecials(date, 6);
  if (ahead.length) {
    page.append(el('h2', 'section-title', 'Coming up'));
    const list = el('div', 'special-list');
    for (const item of ahead) {
      list.append(specialRow(item));
    }
    page.append(list);
    page.append(button('See what is upcoming', 'chevron', 'link-button', () => {
      history.pushState(null, '', '/upcoming');
      document.dispatchEvent(new CustomEvent('calendar:refresh'));
    }));
  }
  return page;
}
