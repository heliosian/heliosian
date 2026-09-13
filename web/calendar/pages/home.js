import {state, eventsOn, today, addDays, parseDate, formatDate, longDayLabel, monthLabel, monthOf, shiftMonth, weekStart, weekdayShort, specials, isSchoolDay, dayTypeClass, eventColors, selectedClassrooms, classroomNames} from '../state.js';
import {el, link, svg, button} from '../dom.js';
import {setTitle, setSearch, renderFilters} from '../chrome.js';
import {eventRow, dayHeading, planCards, emptyNote} from '../events.js';

let lastDate = '';

// weekStrip is the week the chosen day sits in, Sunday to Saturday, each day
// a button carrying its events as dots, with arrows to the weeks either side
// and a way back to today that keeps its place whether or not it shows.
function weekStrip(date) {
  const strip = el('div', 'week-strip');
  const start = weekStart(date);
  const prev = link('/day/' + addDays(date, -7), 'icon-button strip-arrow');
  prev.setAttribute('aria-label', 'Previous week');
  prev.append(svg('back'));
  strip.append(prev);
  const days = el('div', 'strip-days');
  for (let i = 0; i < 7; i++) {
    const d = addDays(start, i);
    const cell = link('/day/' + d, 'strip-day' + (d === date ? ' is-on' : '') + (d === today() ? ' is-today' : '') + (isSchoolDay(d) ? '' : ' is-off'));
    cell.append(el('span', 'strip-dow', weekdayShort(d)), el('span', 'strip-num', String(parseDate(d).getDate())));
    const dots = el('span', 'strip-dots');
    const seen = new Set();
    for (const e of eventsOn(d)) {
      const colors = eventColors(e);
      for (const entry of colors.length ? colors : [{color: ''}]) {
        if (!seen.has(entry.color) && seen.size < 4) {
          seen.add(entry.color);
          const dot = el('span', 'strip-dot' + (entry.color ? '' : ' is-plain'));
          if (entry.color) {
            dot.style.background = entry.color;
          }
          dots.append(dot);
        }
      }
    }
    cell.append(dots);
    const groups = specials(d);
    if (groups.length) {
      cell.append(el('span', 'strip-mark ' + dayTypeClass(groups[0].name)));
    }
    days.append(cell);
  }
  strip.append(days);
  const next = link('/day/' + addDays(date, 7), 'icon-button strip-arrow');
  next.setAttribute('aria-label', 'Next week');
  next.append(svg('chevron'));
  strip.append(next);
  strip.append(link('/', 'button button-secondary button-small strip-today' + (date === today() ? ' is-hidden' : ''), 'Today'));
  return strip;
}

function roomsLine() {
  const rooms = selectedClassrooms();
  return rooms.length === classroomNames().length ? 'Every classroom' : rooms.join(', ');
}

function eventList(date) {
  const events = eventsOn(date);
  if (!events.length) {
    return emptyNote(state.query ? 'Nothing matches on this day.' : 'Nothing on the calendar for these classrooms and tags.');
  }
  const list = el('div', 'event-list');
  for (const e of events) {
    list.append(eventRow(e));
  }
  return list;
}

// dayColumn is the chosen day: the strip, the heading, the plan, the events.
// paint redraws the strip and the events when the search words change.
function dayColumn(date) {
  const col = el('section', 'home-day');
  const strip = el('div');
  const head = el('div', 'home-day-head');
  head.append(el('h1', 'page-title', date === today() ? 'Today' : longDayLabel(date)));
  head.append(el('p', 'page-intro', (date === today() ? longDayLabel(date) + ' · ' : '') + roomsLine()));
  const events = el('div');
  col.append(strip, head, planCards(date), el('h2', 'section-title', 'Events'), events);
  const paint = () => {
    strip.replaceChildren(weekStrip(date));
    events.replaceChildren(eventList(date));
  };
  paint();
  return {node: col, paint};
}

// upcomingPanel scrolls on its own beside the day: every day ahead that has
// something on it, an event or a day that is not regular, through the end of
// the year; with words in the search box it is the whole year's matches.
function upcomingPanel(date) {
  const panel = el('aside', 'home-upcoming');
  const head = el('div', 'upcoming-head');
  const body = el('div', 'upcoming-body');
  const paint = () => {
    head.textContent = state.query ? 'Matching events' : 'Upcoming';
    body.replaceChildren();
    const from = state.query ? today() : (date > today() ? date : today());
    let any = false;
    const until = addDays(from, 400);
    for (let d = from; d < until; d = addDays(d, 1)) {
      const events = eventsOn(d);
      if (!events.length && (state.query || !specials(d).length)) {
        continue;
      }
      any = true;
      const group = el('div', 'day-group');
      group.append(dayHeading(d));
      if (events.length) {
        const list = el('div', 'event-list');
        for (const e of events) {
          list.append(eventRow(e));
        }
        group.append(list);
      }
      body.append(group);
    }
    if (!any) {
      body.append(emptyNote(state.query ? 'Nothing matches.' : 'Nothing ahead for these classrooms and tags.'));
    }
  };
  paint();
  panel.append(head, body);
  return {node: panel, paint};
}

function dayCell(date, month) {
  const cell = link('/day/' + date, 'month-cell' + (monthOf(date) === month ? '' : ' is-outside') + (date === today() ? ' is-today' : '') + (isSchoolDay(date) ? '' : ' is-off'));
  cell.append(el('span', 'month-cell-num', String(parseDate(date).getDate())));
  const groups = specials(date);
  const all = selectedClassrooms();
  for (const g of groups) {
    const mark = el('div', 'month-mark day-type-words ' + dayTypeClass(g.name), g.classrooms.length === all.length ? g.name : `${g.name} (${g.classrooms.length})`);
    mark.title = `${g.name}: ${g.classrooms.join(', ')}`;
    cell.append(mark);
  }
  const events = eventsOn(date);
  const room = Math.max(0, 3 - groups.length);
  for (const e of events.slice(0, room)) {
    const pip = el('div', 'month-pip' + (e.allDay ? ' is-all-day' : ''), e.title);
    pip.title = e.title;
    cell.append(pip);
  }
  if (events.length > room) {
    cell.append(el('div', 'month-more', `+${events.length - room} more`));
  }
  return cell;
}

// monthGrid sits under the day and the upcoming panel, paged in place with
// the arrows at a fixed spot ahead of the month's name; a day cell opens
// that day above.
function monthGrid() {
  const wrap = el('section', 'home-month');
  const paint = () => {
    wrap.replaceChildren();
    const month = state.month;
    const pager = el('div', 'pager');
    const back = button('', 'back', 'icon-button strip-arrow', () => {
      state.month = shiftMonth(month, -1);
      paint();
    });
    back.setAttribute('aria-label', 'Previous month');
    const fwd = button('', 'chevron', 'icon-button strip-arrow', () => {
      state.month = shiftMonth(month, 1);
      paint();
    });
    fwd.setAttribute('aria-label', 'Next month');
    const current = button('This month', null, 'button button-secondary button-small' + (month === monthOf(today()) ? ' is-hidden' : ''), () => {
      state.month = monthOf(today());
      paint();
    });
    pager.append(back, fwd, el('h2', 'pager-label', monthLabel(month)), current);
    wrap.append(pager);
    const grid = el('div', 'month-grid');
    for (const name of ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']) {
      grid.append(el('div', 'month-dow', name));
    }
    const first = month + '-01';
    const last = formatDate(new Date(parseDate(first).getFullYear(), parseDate(first).getMonth() + 1, 0));
    let d = weekStart(first);
    while (d <= last || parseDate(d).getDay() !== 0) {
      grid.append(dayCell(d, month));
      d = addDays(d, 1);
    }
    wrap.append(grid);
  };
  paint();
  return {node: wrap, paint};
}

export function homePage(date) {
  setTitle(date === today() ? 'Today' : longDayLabel(date));
  if (date !== lastDate || !state.month) {
    state.month = monthOf(date);
    lastDate = date;
  }
  const page = el('div', 'home');
  const day = dayColumn(date);
  const upcoming = upcomingPanel(date);
  const month = monthGrid();
  const top = el('div', 'home-top');
  top.append(day.node, upcoming.node);
  page.append(top, month.node);
  // The search words filter all three at once, and light up the tags in the
  // rail that hide more matches.
  setSearch('Search the year…', q => {
    state.query = q;
    day.paint();
    upcoming.paint();
    month.paint();
    renderFilters();
  });
  return page;
}
