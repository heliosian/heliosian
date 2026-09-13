import {state, eventsOn, today, addDays, parseDate, formatDate, monthLabel, dayLabel, specials, isSchoolDay, dayTypeClass, visibleEvents, matches, eventDates, selectedClassrooms} from '../state.js';
import {el, link, svg, segmented} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {eventRow, dayHeading, planChips, emptyNote} from '../events.js';

const views = [
  {key: 'month', label: 'Month'},
  {key: 'week', label: 'Week'},
  {key: 'list', label: 'List'},
];

function go(path) {
  history.pushState(null, '', path);
  document.dispatchEvent(new CustomEvent('calendar:refresh'));
}

function monthOf(date) {
  return date.slice(0, 7);
}

function shiftMonth(month, n) {
  const d = parseDate(month + '-01');
  d.setMonth(d.getMonth() + n);
  return formatDate(d).slice(0, 7);
}

// weekStart is the Sunday on or before a date, the way a US wall calendar
// starts its rows.
function weekStart(date) {
  const d = parseDate(date);
  return addDays(date, -d.getDay());
}

function viewBar(view, month, week) {
  const bar = el('div', 'view-bar');
  bar.append(segmented(views, view, key => {
    switch (key) {
      case 'month':
        return go('/month/' + (week ? monthOf(week) : month));
      case 'week':
        return go('/week/' + (week || (month === monthOf(today()) ? weekStart(today()) : weekStart(month + '-01'))));
    }
    return go('/list');
  }));
  return bar;
}

function pager(label, prev, next, current) {
  const wrap = el('div', 'pager');
  const back = link(prev, 'icon-button');
  back.setAttribute('aria-label', 'Previous');
  back.append(svg('back'));
  const fwd = link(next, 'icon-button');
  fwd.setAttribute('aria-label', 'Next');
  fwd.append(svg('chevron'));
  wrap.append(back, el('h2', 'pager-label', label), fwd);
  if (current) {
    wrap.append(link(current, 'button button-secondary button-small', 'Today'));
  }
  return wrap;
}

// dayCell is one square of the month grid: the number, the plan when the day
// is not regular for the selected classrooms, and the first few events.
function dayCell(date, inMonth) {
  const cell = link('/day/' + date, 'month-cell' + (inMonth ? '' : ' is-outside') + (date === today() ? ' is-today' : '') + (isSchoolDay(date) ? '' : ' is-off'));
  const head = el('div', 'month-cell-head');
  head.append(el('span', 'month-cell-num', String(parseDate(date).getDate())));
  cell.append(head);
  const groups = specials(date);
  const all = selectedClassrooms();
  for (const g of groups) {
    const mark = el('div', 'month-mark ' + dayTypeClass(g.name), g.classrooms.length === all.length ? g.name : `${g.name} (${g.classrooms.length})`);
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

function monthView(month) {
  const wrap = el('div');
  wrap.append(pager(monthLabel(month), '/month/' + shiftMonth(month, -1), '/month/' + shiftMonth(month, 1), month === monthOf(today()) ? null : '/month/' + monthOf(today())));
  const grid = el('div', 'month-grid');
  for (const name of ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']) {
    grid.append(el('div', 'month-dow', name));
  }
  const first = month + '-01';
  const last = formatDate(new Date(parseDate(first).getFullYear(), parseDate(first).getMonth() + 1, 0));
  let date = weekStart(first);
  while (date <= last || parseDate(date).getDay() !== 0) {
    grid.append(dayCell(date, monthOf(date) === month));
    date = addDays(date, 1);
  }
  wrap.append(grid);
  return wrap;
}

function weekView(start) {
  const wrap = el('div');
  const end = addDays(start, 6);
  const label = `${dayLabel(start)} – ${dayLabel(end)}`;
  wrap.append(pager(label, '/week/' + addDays(start, -7), '/week/' + addDays(start, 7), start === weekStart(today()) ? null : '/week/' + weekStart(today())));
  const columns = el('div', 'week-grid');
  for (let date = start; date <= end; date = addDays(date, 1)) {
    const col = el('div', 'week-col' + (date === today() ? ' is-today' : '') + (isSchoolDay(date) ? '' : ' is-off'));
    const head = link('/day/' + date, 'week-head');
    head.append(el('span', 'week-dow', parseDate(date).toLocaleDateString('en-US', {weekday: 'short'})), el('span', 'week-num', String(parseDate(date).getDate())));
    col.append(head);
    if (isSchoolDay(date)) {
      col.append(planChips(date));
    }
    const events = eventsOn(date);
    for (const e of events) {
      const item = link('/events/' + e.id.split('/').map(encodeURIComponent).join('/'), 'week-event' + (e.allDay ? ' is-all-day' : ''));
      if (!e.allDay) {
        item.append(el('span', 'week-event-time', e.start.slice(11)));
      }
      item.append(el('span', 'week-event-title', e.title));
      col.append(item);
    }
    columns.append(col);
  }
  wrap.append(columns);
  return wrap;
}

// listView is every visible event, grouped by month then day, and what the
// search box searches: the whole year at once.
function listView() {
  const wrap = el('div');
  const paint = () => {
    wrap.replaceChildren();
    const seen = new Set();
    const dates = [];
    for (const e of visibleEvents()) {
      if (!matches(e, state.query)) {
        continue;
      }
      const date = eventDates(e)[0];
      if (!seen.has(date)) {
        seen.add(date);
        dates.push(date);
      }
    }
    dates.sort();
    if (!dates.length) {
      wrap.append(emptyNote(state.query ? 'Nothing matches.' : 'Nothing on the calendar for these classrooms and tags.'));
      return;
    }
    let month = '';
    let monthBox = null;
    for (const date of dates) {
      if (monthOf(date) !== month) {
        month = monthOf(date);
        wrap.append(el('h2', 'section-title list-month', monthLabel(month)));
        monthBox = el('div');
        wrap.append(monthBox);
      }
      const day = el('div', 'day-group');
      day.append(dayHeading(date));
      const list = el('div', 'event-list');
      for (const e of eventsOn(date)) {
        if (eventDates(e)[0] === date) {
          list.append(eventRow(e));
        }
      }
      day.append(list);
      monthBox.append(day);
    }
  };
  paint();
  setSearch('Search the whole year…', q => {
    state.query = q;
    paint();
  });
  return wrap;
}

export function calendarPage(view, arg) {
  setTitle('Calendar');
  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Calendar'));
  head.append(main);
  let month = null;
  let week = null;
  if (view === 'month') {
    month = /^\d{4}-\d{2}$/.test(arg || '') ? arg : monthOf(today());
  }
  if (view === 'week') {
    week = parseDate(arg) ? weekStart(arg) : weekStart(today());
  }
  head.append(viewBar(view, month, week));
  page.append(head);
  switch (view) {
    case 'month':
      page.append(monthView(month));
      break;
    case 'week':
      page.append(weekView(week));
      break;
    default:
      page.append(listView());
  }
  return page;
}
