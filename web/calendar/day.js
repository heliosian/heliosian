// The day column: a small month to pick a day from, and the chosen day as a
// card - its name, its date, the day plan with its hours, and its events
// down a timeline. On a wide window it lives in the rail (chrome.js
// draws it there for every page); on a phone the calendar page draws the
// card at the top of the page.
import {state, eventsOn, today, addDays, parseDate, formatDate, monthOf, shiftMonth, monthLabel, weekStart, weekdayLong, specials, isSchoolDay, dayTypeClass, eventTint, timeLine, eventPath, isMatch, isGray} from './state.js';
import {el, link, svg, button, popup} from './dom.js';
import {eventForm} from './eventform.js';
import {planCards} from './events.js';

// The rail's paging, kept across renders: the month its small month is
// open to, and the day that set it.
const railPaging = {month: '', date: ''};

// miniMonth is a small month: a row of weekday letters and the days, the
// chosen day filled, today in amber, the days of the months either side
// faded, and under a day a dot in the color of what is on it - its first
// event's, or its day type's. Its arrows page it alone, through paging
// (the month it is open to), which the caller keeps as long as it likes.
function miniMonth(date, paging) {
  const wrap = el('div', 'mini-month');
  const paint = () => {
    wrap.replaceChildren();
    const month = paging.month;
    const head = el('div', 'mini-head');
    const arrows = el('span', 'mini-arrows');
    const back = button('', 'back', 'mini-arrow', () => {
      paging.month = shiftMonth(month, -1);
      paint();
    });
    back.setAttribute('aria-label', 'Previous month');
    const fwd = button('', 'chevron', 'mini-arrow', () => {
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
  const row = link(eventPath(e), 'timeline-row' + (isMatch(e) ? ' is-match' : '') + (isGray(e) ? ' is-hidden' : '') + (e.pending ? ' is-pending' : '') + (e.declined ? ' is-declined' : '') + (e.inviteOnly ? ' is-invite' : ''));
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

// dayNote is the card's word when a day has no events: a soft box with a
// ticked calendar.
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

// dayColumn is the small month and the day card. paint redraws both when
// the search words change, so the matching events light up. Without a
// paging of its own it is the rail's, whose month is kept across renders
// until a different day is opened.
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
  // Under the day: anyone can share an event with the community - it
  // waits for an admin's approval before the calendar carries it.
  // Added, the event's own page opens - where its link, and its guest
  // list, are.
  const share = button('Share Event', 'plus', 'button share-event', () => {
    let shut = null;
    const form = eventForm({onDone: async ids => {
      shut();
      const {load, navigate} = await import('./app.js');
      await load();
      if (ids && ids.length) {
        navigate('/e/' + ids[0]);
      }
    }});
    shut = popup('Share an event', form, {wide: true}).shut;
  });
  share.title = 'Add an event for the community; an admin approves it onto the calendar';
  col.append(month, card, share);
  const paint = () => {
    month.replaceChildren(miniMonth(date, paging));
    plan.replaceChildren(planCards(date));
    events.replaceChildren(timeline(date));
  };
  paint();
  return {node: col, paint};
}
