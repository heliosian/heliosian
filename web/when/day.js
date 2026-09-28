import {eventsOn, today, addDays, parseDate, formatDate, monthOf, shiftMonth, monthLabel, weekStart, weekdayLong, specials, isSchoolDay, eventTint, timeLine, eventPath, isMatch, isGray} from './state.js';
import {dayTypeClass} from '/daytype.js';
import {el, link, svg, button} from '/elements.js';
import {popup} from '/modal.js';
import {load, navigate} from '/router.js';
import {eventForm} from './eventform.js';
import {planFolds} from './events.js';
import {attachRowPeek} from './peek.js';

const railPaging = {month: '', date: ''};

function miniMonth(date, paging) {
  const wrap = el('div', 'mini-month');
  const paint = () => {
    wrap.replaceChildren();
    const month = paging.month;
    const head = el('div', 'mini-head');
    const arrows = el('span', 'mini-arrows');
    const back = button('', 'chevron-left', 'mini-arrow', () => {
      paging.month = shiftMonth(month, -1);
      paint();
    });
    back.setAttribute('aria-label', 'Previous month');
    const fwd = button('', 'chevron-right', 'mini-arrow', () => {
      paging.month = shiftMonth(month, 1);
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
        dot.style.setProperty('--mini-dot', eventTint(events[0]));
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

function timelineRow(e, date) {
  const row = link(eventPath(e), 'timeline-row' + (isMatch(e) ? ' is-match' : '') + (isGray(e) ? ' is-hidden' : '') + (e.pending ? ' is-pending' : '') + (e.declined ? ' is-declined' : '') + (e.sharing !== 'Public' ? ' is-invite' : ''));
  const dot = el('span', 'timeline-dot');
  dot.style.background = eventTint(e);
  const body = el('span', 'timeline-body');
  body.append(el('span', 'timeline-time', timeLine(e, date)), el('span', 'timeline-title', e.title));
  if (e.location) {
    body.append(el('span', 'timeline-place', e.location));
  }
  row.append(dot, body);
  attachRowPeek(row, date, e);
  return row;
}

function dayNote(words) {
  const note = el('div', 'day-note');
  note.append(svg('calcheck'), el('span', '', words));
  return note;
}

function timeline(date) {
  const events = eventsOn(date);
  if (!events.length) {
    return dayNote('Nothing on the calendar for these classrooms and tags.');
  }
  const list = el('div', 'timeline');
  for (const e of events) {
    list.append(timelineRow(e, date));
  }
  return list;
}

export function openAddEvent() {
  let shut = null;
  const form = eventForm({onDone: async ids => {
    shut();
    await load();
    if (ids && ids.length) {
      navigate('/e/' + ids[0]);
    }
  }});
  shut = popup('Add an event', form, {wide: true}).shut;
}

export function dayColumn(date, paging) {
  if (!paging) {
    if (railPaging.date !== date || !railPaging.month) {
      railPaging.month = monthOf(date);
      railPaging.date = date;
    }
    paging = railPaging;
  }
  const col = el('section', 'home-day');
  const month = el('div');
  const card = el('div', 'day-card');
  const art = el('img', 'day-card-art');
  art.src = '/brand/schedule-box.png';
  art.alt = '';
  card.append(art);
  const head = el('div', 'day-card-head');
  head.append(el('h1', 'day-card-title', date === today() ? 'Today' : weekdayLong(date)));
  head.append(el('p', 'day-card-date', parseDate(date).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'})));
  const plan = el('div', 'day-card-plan');
  const events = el('div');
  card.append(head, plan, events);
  col.append(month, card);
  const paint = () => {
    month.replaceChildren(miniMonth(date, paging));
    plan.replaceChildren(planFolds(date));
    events.replaceChildren(timeline(date));
  };
  paint();
  return {node: col, paint};
}
