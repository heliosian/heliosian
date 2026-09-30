import {state, feed, readMonth, shiftMonth} from './state.js';
import {el, svg} from '/elements.js';
import {rsvpButtons, calendarMark, calendarMenu, dropdown} from './cards.js';
import {dayTypeClass} from '/daytype.js';
import {appOrigin} from '/appswitch.js';
import {parseWhen, timeRange} from '/datecard.js';

let month = null;
let selected = '';
let loaded = null;

function parseDate(date) {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(y, m - 1, d);
}

const pad = n => String(n).padStart(2, '0');

function tint(event) {
  return event.linkApp === 'celebrate' ? 'is-celebrate' : event.linkApp === 'team' ? 'is-team' : 'is-school';
}

function lastDay(event) {
  return event.endAt.slice(0, 10);
}

function eventsOn(date) {
  return month.events.filter(e => e.dates.includes(date));
}

function hours(event, date) {
  const last = lastDay(event);
  if (!event.allDay && event.start === last) {
    const from = parseWhen(event.startAt).date;
    const to = parseWhen(event.endAt).date;
    return to > from ? timeRange(from, to) : from.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
  }
  if (last > date) {
    return 'Through ' + parseDate(last).toLocaleDateString('en-US', {weekday: 'short'});
  }
  return 'All day';
}

function monthLabel(ym) {
  return parseDate(ym + '-01').toLocaleDateString('en-US', {month: 'long', year: 'numeric'});
}

function days() {
  return feed(month.calendar).days || {};
}

async function fetchMonth(ym, calendar) {
  try {
    month = await readMonth(ym, calendar);
  } catch {
    return;
  }
  selected = state.model.today.startsWith(month.month) ? state.model.today : month.month + '-01';
  renderMonth();
}

function page(by) {
  return fetchMonth(shiftMonth(month.month, by), month.calendar);
}

function picker() {
  const list = state.model.calendars;
  const current = feed(month.calendar);
  const chosen = list[0];
  const wrap = el('div', 'mini-calendar');
  const toggle = el('button', 'mini-calendar-toggle');
  toggle.type = 'button';
  toggle.title = current.locked ? 'The calendar’s own view, for everyone' : 'The saved calendar this month is read under';
  toggle.append(calendarMark(current), el('span', 'mini-calendar-name', current.name), svg('chevron-right'));
  const menu = calendarMenu(list, current, chosen, c => fetchMonth(month.month, c.id));
  dropdown(toggle, menu);
  wrap.append(toggle, menu);
  return wrap;
}

function grid() {
  const wrap = el('div', 'mini');
  const head = el('div', 'mini-head');
  const back = el('button', 'mini-page');
  back.type = 'button';
  back.setAttribute('aria-label', 'Previous month');
  back.append(svg('chevron-right'));
  back.addEventListener('click', () => page(-1));
  const next = el('button', 'mini-page');
  next.type = 'button';
  next.setAttribute('aria-label', 'Next month');
  next.append(svg('chevron-right'));
  next.addEventListener('click', () => page(1));
  head.append(back, el('span', 'mini-title', monthLabel(month.month)), next);
  wrap.append(picker(), head);
  const cells = el('div', 'mini-grid');
  for (const w of ['S', 'M', 'T', 'W', 'T', 'F', 'S']) {
    cells.append(el('span', 'mini-weekday', w));
  }
  const first = parseDate(month.month + '-01');
  for (let i = 0; i < first.getDay(); i++) {
    cells.append(el('span', 'mini-blank'));
  }
  const last = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate();
  const kinds = days();
  for (let n = 1; n <= last; n++) {
    const date = `${month.month}-${pad(n)}`;
    const day = kinds[date];
    const cell = el('button', 'mini-day');
    cell.type = 'button';
    if (date === state.model.today) {
      cell.classList.add('is-today');
    }
    if (date === selected) {
      cell.classList.add('is-selected');
    }
    if (!day) {
      cell.classList.add('is-off');
    } else if (day.length) {
      cell.classList.add(dayTypeClass(day[0].name));
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
    cells.append(cell);
  }
  wrap.append(cells);
  return wrap;
}

function dayCard() {
  const card = el('div', 'rail-day');
  const date = parseDate(selected);
  const head = el('a', 'rail-day-head');
  head.href = appOrigin('when') + '/day/' + selected;
  head.append(el('span', 'rail-day-title', selected === state.model.today ? 'Today' : date.toLocaleDateString('en-US', {weekday: 'long'})));
  head.append(el('span', 'rail-day-date', date.toLocaleDateString('en-US', {month: 'long', day: 'numeric'})));
  card.append(head);
  const day = days()[selected];
  if (day && day.length) {
    const kinds = el('div', 'rail-day-kinds');
    for (const kind of day) {
      kinds.append(el('span', 'rail-day-kind ' + dayTypeClass(kind.name), kind.words));
    }
    card.append(kinds);
  }
  const events = eventsOn(selected);
  const list = el('div', 'rail-day-events');
  if (!events.length) {
    list.append(el('div', 'rail-day-empty', day ? 'Nothing on the calendar.' : 'No school.'));
  }
  for (const event of events) {
    const item = el('div', 'rail-item');
    const row = el('a', 'rail-event ' + tint(event));
    row.href = appOrigin('when') + event.path;
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
  if (loaded !== state.model.month) {
    loaded = state.model.month;
    month = loaded;
    selected = state.model.today;
  }
  root.append(grid(), dayCard());
}
