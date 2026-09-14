import {state} from './state.js';
import {el, svg} from './dom.js';
import {whenOrigin, rsvpButtons} from './cards.js';

// The rail's calendar, from Helios When: a small month, paged on its own,
// with a dot under each day in the colour of what is on it and today ringed
// in amber, and under it the day picked - today until one is - as a card:
// the date, what kind of day it is for this person's classrooms when it is
// not simply regular, and every event on it, each opening its page on When
// with its Yes and No under it.
// The month is the viewer's as When first shows it (their classrooms, the
// default categories); today is the school's, reckoned by the server, so
// the ring does not drift with the browser's clock.

let month = null;
let selected = '';

// A YYYY-MM-DD as a local date, without the time zone shifting it.
function parseDate(date) {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(y, m - 1, d);
}

const pad = n => String(n).padStart(2, '0');

function dateOf(d) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

const dayTypeClass = {'No School': 'dt-no-school', 'Early Dismissal': 'dt-early', 'No Aftercare': 'dt-no-aftercare'};

// tint is where an event comes from, as When colours it: a party pink, an
// HCA event purple, the school's own blue.
function tint(event) {
  return event.linkApp === 'celebrate' ? 'is-celebrate' : event.linkApp === 'team' ? 'is-team' : 'is-school';
}

function lastDay(event) {
  return (event.endAt || event.startAt).slice(0, 10);
}

function eventsOn(date) {
  return (month.events || []).filter(e => e.start <= date && lastDay(e) >= date);
}

// hours are what a row says beside the title: the hours the event's when
// line carries, else how far a whole-day event runs - "All day", or
// "Through Fri" while it has days to go.
function hours(event, date) {
  const [, time] = event.when.split(' · ');
  if (time) {
    return time;
  }
  const last = lastDay(event);
  if (last > date) {
    return 'Through ' + parseDate(last).toLocaleDateString('en-US', {weekday: 'short'});
  }
  return 'All day';
}

function monthLabel(ym) {
  return parseDate(ym + '-01').toLocaleDateString('en-US', {month: 'long', year: 'numeric'});
}

function shiftMonth(ym, by) {
  const d = parseDate(ym + '-01');
  d.setMonth(d.getMonth() + by);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}`;
}

// Paging asks the server for the month, read for the viewer the way the
// first was; the day picked becomes today when it is in the month, else
// the first.
async function page(by) {
  const ym = shiftMonth(month.month, by);
  try {
    const res = await fetch('/api/apps/calendar?month=' + ym);
    if (!res.ok) {
      return;
    }
    month = await res.json();
  } catch {
    return;
  }
  selected = month.today.startsWith(month.month) ? month.today : month.month + '-01';
  renderMonth();
}

function grid() {
  const wrap = el('div', 'mini');
  const head = el('div', 'mini-head');
  const back = el('button', 'mini-page');
  back.type = 'button';
  back.setAttribute('aria-label', 'Previous month');
  back.append(svg('chevron'));
  back.addEventListener('click', () => page(-1));
  const next = el('button', 'mini-page');
  next.type = 'button';
  next.setAttribute('aria-label', 'Next month');
  next.append(svg('chevron'));
  next.addEventListener('click', () => page(1));
  head.append(back, el('span', 'mini-title', monthLabel(month.month)), next);
  wrap.append(head);
  const days = el('div', 'mini-grid');
  for (const w of ['S', 'M', 'T', 'W', 'T', 'F', 'S']) {
    days.append(el('span', 'mini-weekday', w));
  }
  const first = parseDate(month.month + '-01');
  for (let i = 0; i < first.getDay(); i++) {
    days.append(el('span', 'mini-blank'));
  }
  const last = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate();
  for (let n = 1; n <= last; n++) {
    const date = `${month.month}-${pad(n)}`;
    const day = month.days[date];
    const cell = el('button', 'mini-day');
    cell.type = 'button';
    if (date === month.today) {
      cell.classList.add('is-today');
    }
    if (date === selected) {
      cell.classList.add('is-selected');
    }
    if (!day) {
      cell.classList.add('is-off');
    } else if (day.kinds.length) {
      cell.classList.add(dayTypeClass[day.kinds[0].name] || 'dt-other');
    }
    cell.append(el('span', 'mini-number', String(n)));
    const dots = el('span', 'mini-dots');
    const seen = new Set();
    for (const e of eventsOn(date)) {
      const t = tint(e);
      if (!seen.has(t)) {
        seen.add(t);
        dots.append(el('span', 'mini-dot ' + t));
      }
    }
    cell.append(dots);
    cell.addEventListener('click', () => {
      selected = date;
      renderMonth();
    });
    days.append(cell);
  }
  wrap.append(days);
  return wrap;
}

function dayCard() {
  const card = el('div', 'rail-day');
  const date = parseDate(selected);
  const head = el('a', 'rail-day-head');
  head.href = whenOrigin('calendar') + '/day/' + selected;
  head.append(el('span', 'rail-day-title', selected === month.today ? 'Today' : date.toLocaleDateString('en-US', {weekday: 'long'})));
  head.append(el('span', 'rail-day-date', date.toLocaleDateString('en-US', {month: 'long', day: 'numeric'})));
  card.append(head);
  const day = month.days[selected];
  if (day && day.kinds.length) {
    const kinds = el('div', 'rail-day-kinds');
    for (const kind of day.kinds) {
      kinds.append(el('span', 'rail-day-kind ' + (dayTypeClass[kind.name] || 'dt-other'), kind.words));
    }
    card.append(kinds);
  }
  const events = eventsOn(selected);
  const list = el('div', 'rail-day-events');
  if (!events.length) {
    list.append(el('div', 'rail-day-empty', day ? 'Nothing on the calendar.' : 'No school.'));
  }
  // Each event is its row, opening its page on When, with its answer's
  // buttons under it - the same Yes and No the Upcoming cards carry.
  for (const event of events) {
    const item = el('div', 'rail-item');
    const row = el('a', 'rail-event ' + tint(event));
    row.href = whenOrigin('calendar') + event.path;
    row.append(el('span', 'rail-event-dot'));
    const body = el('span', 'rail-event-body');
    body.append(el('span', 'rail-event-title', event.title));
    body.append(el('span', 'rail-event-hours', hours(event, selected)));
    row.append(body);
    const rsvp = el('div', 'rail-rsvp');
    rsvp.append(rsvpButtons(event));
    item.append(row, rsvp);
    list.append(item);
  }
  card.append(list);
  return card;
}

export function renderMonth() {
  const root = document.querySelector('#rail-calendar');
  root.replaceChildren();
  if (!month) {
    month = state.model.calendar;
    if (!month || !month.month) {
      return;
    }
    selected = month.today;
  }
  root.append(grid(), dayCard());
}
