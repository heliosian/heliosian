// The day column: a small month to pick a day from, and the chosen day as a
// card - its name, its date, the day plan with its hours, and its events
// down a timeline. On a wide window it lives in the rail (chrome.js
// draws it there for every page); on a phone the calendar page draws the
// card at the top of the page.
import {state, eventsOn, today, addDays, parseDate, formatDate, monthOf, shiftMonth, monthLabel, weekStart, shortDayLabel, weekdayLong, specials, isSchoolDay, dayTypeClass, eventTint, timeLine, eventPath} from './state.js';
import {el, link, svg, button} from './dom.js';
import {planCards, emptyNote} from './events.js';

let lastDate = '';

// miniMonth is the rail's month: a row of weekday letters and the days,
// the chosen day filled, today in amber, the days of the months either side
// faded, and under a day a dot in the color of what is on it - its first
// event's, or its day type's. Its arrows page
// it alone; opening a day brings it back to that day's month.
function miniMonth(date) {
  const wrap = el('div', 'mini-month');
  const paint = () => {
    wrap.replaceChildren();
    const month = state.railMonth;
    const head = el('div', 'mini-head');
    const arrows = el('span', 'mini-arrows');
    const back = button('', 'back', 'mini-arrow', () => {
      state.railMonth = shiftMonth(month, -1);
      paint();
    });
    back.setAttribute('aria-label', 'Previous month');
    const fwd = button('', 'chevron', 'mini-arrow', () => {
      state.railMonth = shiftMonth(month, 1);
      paint();
    });
    fwd.setAttribute('aria-label', 'Next month');
    arrows.append(back, fwd);
    head.append(el('span', 'mini-label', monthLabel(month)), arrows);
    wrap.append(head);
    const grid = el('div', 'mini-grid');
    for (const letter of ['S', 'M', 'T', 'W', 'T', 'F', 'S']) {
      grid.append(el('span', 'mini-dow', letter));
    }
    const first = month + '-01';
    const last = formatDate(new Date(parseDate(first).getFullYear(), parseDate(first).getMonth() + 1, 0));
    let d = weekStart(first);
    while (d <= last || parseDate(d).getDay() !== 0) {
      const cell = link('/day/' + d, 'mini-day' + (monthOf(d) === month ? '' : ' is-outside') + (d === date ? ' is-on' : '') + (d === today() ? ' is-today' : '') + (isSchoolDay(d) ? '' : ' is-off'));
      cell.append(el('span', 'mini-num', String(parseDate(d).getDate())));
      const events = eventsOn(d);
      const groups = specials(d);
      if (events.length) {
        const dot = el('span', 'mini-dot');
        dot.style.background = eventTint(events[0]);
        cell.append(dot);
      } else if (groups.length) {
        cell.append(el('span', 'mini-dot ' + dayTypeClass(groups[0].name)));
      }
      grid.append(cell);
      d = addDays(d, 1);
    }
    wrap.append(grid);
  };
  paint();
  return wrap;
}

// timelineRow is one event down the day's timeline: a dot in its color on
// the line, then its hours, its title and its place.
function timelineRow(e, date) {
  const row = link(eventPath(e), 'timeline-row');
  const dot = el('span', 'timeline-dot');
  dot.style.background = eventTint(e);
  const body = el('span', 'timeline-body');
  body.append(el('span', 'timeline-time', timeLine(e, date)), el('span', 'timeline-title', e.title));
  if (e.location) {
    body.append(el('span', 'timeline-place', e.location));
  }
  row.append(dot, body);
  return row;
}

function timeline(date) {
  const events = eventsOn(date);
  if (!events.length) {
    return emptyNote(state.query ? 'Nothing matches on this day.' : 'Nothing on the calendar for these classrooms and tags.');
  }
  const list = el('div', 'timeline');
  for (const e of events) {
    list.append(timelineRow(e, date));
  }
  return list;
}

// dayColumn is the small month and the day card. paint redraws both when
// the search words change: the plan stays only for a day type the words
// name, and the month's dots follow the matches.
export function dayColumn(date) {
  if (date !== lastDate || !state.railMonth) {
    state.railMonth = monthOf(date);
    lastDate = date;
  }
  const col = el('section', 'home-day');
  const month = el('div');
  const card = el('div', 'day-card');
  const head = el('div', 'day-card-head');
  head.append(el('h1', 'day-card-title', date === today() ? 'Today' : weekdayLong(date)));
  head.append(el('p', 'day-card-date', shortDayLabel(date)));
  const plan = el('div', 'day-card-plan');
  const events = el('div');
  card.append(head, plan, events);
  col.append(month, card);
  const paint = () => {
    month.replaceChildren(miniMonth(date));
    plan.replaceChildren();
    if (!state.query) {
      plan.append(planCards(date));
    } else if (specials(date).length) {
      plan.append(planCards(date, specials(date)));
    }
    events.replaceChildren(timeline(date));
  };
  paint();
  return {node: col, paint};
}
