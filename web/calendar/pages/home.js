import {state, eventsOn, today, addDays, parseDate, formatDate, longDayLabel, dayLabel, monthLabel, monthOf, shiftMonth, weekStart, specials, scheduleOn, isSchoolDay, dayTypeClass, dayTypeMatches, selectedClassrooms, eventTint, timeLine, eventPath, weekdayShort, call, isMatch, isHidden} from '../state.js';
import {el, link, svg, button} from '../dom.js';
import {setTitle, setSearch, fillFilters, renderRailDay} from '../chrome.js';
import {dayColumn} from '../day.js';
import {callPill, emptyNote, roomDots, planCards} from '../events.js';
import {answerOf, answer, linkURL} from '../state.js';

let lastDate = '';

// upcomingRow is one thing ahead: the date at the left, a bar and a dot in
// its color, and between them the title, the hours, the place, and for a
// linked event the way to its tickets or sign-up. A day that is not regular
// and has no event of its own is a row too, in its day type's color.
function upcomingRow(date, e, group) {
  const row = link(e ? eventPath(e) : '/day/' + date, 'up-row' + (e ? (isMatch(e) ? ' is-match' : '') : ' ' + dayTypeClass(group.name)));
  const when = el('span', 'up-date');
  when.append(el('span', 'up-dow', weekdayShort(date)), el('span', 'up-day', parseDate(date).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})));
  const bar = el('span', 'up-bar');
  const body = el('span', 'up-body');
  const dot = el('span', 'up-dot');
  if (e) {
    bar.style.background = eventTint(e);
    dot.style.background = eventTint(e);
    body.append(el('span', 'up-title', e.title), el('span', 'up-time', timeLine(e, date)));
    if (e.location) {
      body.append(el('span', 'up-place', e.location));
    }
    if (e.link && call(e)) {
      body.append(callPill(e));
    }
  } else {
    const all = selectedClassrooms();
    body.append(el('span', 'up-title', group.classrooms.length === all.length ? group.name : `${group.name} · ${group.classrooms.join(', ')}`));
  }
  row.append(when, bar, body, dot);
  return row;
}

// upcomingPanel scrolls on its own beside the month: every event from the
// day shown on, and - while the Schedule tag is on - every day that is not
// regular, through the end of the year, the first two weeks under Upcoming
// and the rest under Later This Year; the search words light up the rows
// they find.
function upcomingPanel(date) {
  const panel = el('aside', 'home-upcoming');
  const head = el('div', 'upcoming-head', 'Upcoming Events');
  const body = el('div', 'upcoming-body');
  const paint = () => {
    body.replaceChildren();
    const from = date;
    const soon = addDays(from, 14);
    const days = scheduleOn();
    let any = false;
    let later = false;
    const until = addDays(from, 400);
    for (let d = from; d < until; d = addDays(d, 1)) {
      const events = eventsOn(d);
      const groups = days ? specials(d) : [];
      if (!events.some(e => !isHidden(e)) && !groups.length) {
        continue;
      }
      if (!later && d >= soon) {
        later = true;
        body.append(el('div', 'upcoming-later', 'Later This Year'));
      }
      any = true;
      const shown = events.filter(e => !isHidden(e));
      for (const e of shown) {
        body.append(upcomingRow(d, e));
      }
      if (!shown.length && !events.length) {
        body.append(upcomingRow(d, null, groups[0]));
      }
    }
    if (!any) {
      body.append(emptyNote('Nothing ahead for these classrooms and tags.'));
    }
  };
  paint();
  panel.append(head, body);
  return {node: panel, paint};
}

// go opens a path the way a data-link does, from a click that is not on
// one.
function go(path) {
  history.pushState(null, '', path);
  document.dispatchEvent(new CustomEvent('calendar:refresh'));
}

// The card that opens over a month cell while the pointer rests on it. On
// the cell itself it is the day: its name and the day plan with its hours
// as the rail shows it. On one of the cell's pills it is that event alone
// - hours, title, place, the household's part, and Yes and No. One card for
// the page, moved and refilled; it stays while the pointer is on it, so
// its buttons and links can be clicked.
let peek = null;
let peekTimer = 0;
let peekCell = null;
let peekEvent = null;

function peekNode() {
  if (!peek) {
    peek = el('div', 'day-peek');
    peek.hidden = true;
    // Arriving on the card keeps it: no closing, and no other cell's card
    // taking its place on the way over.
    peek.addEventListener('mouseenter', () => {
      clearTimeout(peekGrace);
      clearTimeout(peekTimer);
    });
    peek.addEventListener('mouseleave', hidePeekSoon);
    document.body.append(peek);
  }
  return peek;
}

function fillPeek(date, e) {
  const node = peekNode();
  node.replaceChildren();
  if (e) {
    fillEventPeek(node, date, e);
    return;
  }
  const head = link('/day/' + date, 'day-peek-head');
  head.append(el('span', 'day-peek-date', dayLabel(date)));
  if (date === today()) {
    head.append(el('span', 'day-heading-today', 'Today'));
  }
  node.append(head, planCards(date));
}

// fillEventPeek is the card for one event: its hours, title and place as a
// link to it, who in the household is in it, and Yes and No - the one
// given filled - which answer without leaving the month.
function fillEventPeek(node, date, e) {
  const row = link(eventPath(e), 'day-peek-row day-peek-event');
  row.style.setProperty('--c', eventTint(e));
  const body = el('span', 'day-peek-body');
  body.append(el('span', 'day-peek-time', timeLine(e, date)), el('span', 'day-peek-title', e.title));
  if (e.location) {
    body.append(el('span', 'day-peek-place', e.location));
  }
  row.append(el('span', 'day-peek-bar'), body, roomDots(e));
  node.append(row);
  if (e.description) {
    node.append(el('div', 'day-peek-words', e.description.length > 160 ? e.description.slice(0, 160).replace(/\s+\S*$/, '') + '\u2026' : e.description));
  }
  if (e.minePeople && e.minePeople.length) {
    const people = el('ul', 'side-people');
    for (const p of e.minePeople) {
      const item = el('li');
      item.append(svg(e.source === 'celebrate' ? 'ticket' : 'people'), el('span', 'side-person', p.name));
      if (p.note) {
        item.append(el('span', 'side-person-note', p.note));
      }
      people.append(item);
    }
    node.append(people);
  }
  const word = answerOf(e);
  const buttons = el('div', 'day-peek-answer');
  const say = async next => {
    try {
      await answer(e, next);
      fillPeek(date, e);
    } catch (err) {
      alert(err.message);
    }
  };
  // A party has no yes or no: Send Invite with a ticket in the household,
  // Add Ticket without one.
  if (e.source === 'celebrate') {
    const held = (e.minePeople || []).some(p => p.note !== 'waitlisted');
    if (held) {
      buttons.append(button(word === 'yes' ? 'Invite sent' : 'Send Invite', word === 'yes' ? 'check' : 'calendar', 'button button-small' + (word === 'yes' ? ' button-secondary' : ''), () => say('yes')));
    } else {
      const add = el('a', 'button button-small' + (e.availability === 'available' ? '' : ' button-secondary'));
      add.href = linkURL(e);
      add.append(svg('ticket'), el('span', '', e.availability === 'available' ? 'Add Ticket' : call(e) || 'See the party'));
      buttons.append(add);
    }
    node.append(buttons);
    return;
  }
  buttons.append(
    button('Yes', 'check', 'button button-small' + (word === 'yes' ? '' : ' button-secondary'), () => say(word === 'yes' ? '' : 'yes')),
    button('No', 'close', 'button button-small' + (word === 'no' ? '' : ' button-secondary'), () => say(word === 'no' ? '' : 'no')),
  );
  node.append(buttons);
}

// placePeek sets the card under the cell, flush with its left edge and
// touching its bottom, or above it when there is no room below, and
// within the window's sides.
function placePeek(cell) {
  const node = peekNode();
  const box = cell.getBoundingClientRect();
  node.hidden = false;
  const width = node.offsetWidth;
  const height = node.offsetHeight;
  let left = Math.min(box.left, window.innerWidth - width - 12);
  left = Math.max(12, left);
  let top = box.bottom - 1;
  if (top + height > window.innerHeight - 12) {
    top = Math.max(12, box.top - height + 1);
  }
  node.style.left = left + 'px';
  node.style.top = top + 'px';
}

function showPeek(cell, date, e) {
  peekCell = cell;
  peekEvent = e || null;
  fillPeek(date, e);
  placePeek(cell);
}

function hidePeek() {
  clearTimeout(peekTimer);
  clearTimeout(peekGrace);
  peekCell = null;
  peekEvent = null;
  if (peek) {
    peek.hidden = true;
  }
}

// The card touches the cell, so the pointer crosses straight onto it and
// arriving there keeps it; off both, the card goes after a short moment,
// long enough for a corner cut across a neighbour, not long enough to
// linger.
let peekGrace = 0;

function hidePeekSoon() {
  clearTimeout(peekGrace);
  peekGrace = setTimeout(hidePeek, 250);
}

// The card waits a beat so sweeping the pointer across the grid does not
// flash it, and closes when the pointer leaves the cell for anywhere but
// the card.
function attachPeek(cell, date) {
  // Over a pill the card is that event's; off it, the day's. While another
  // cell's card is open, this one waits long enough for a pointer only
  // passing through on its way to that card.
  cell.addEventListener('mouseover', ev => {
    const pip = ev.target.closest('.month-pip');
    const e = pip ? pip.peekEvent : null;
    const open = peek && !peek.hidden;
    clearTimeout(peekGrace);
    clearTimeout(peekTimer);
    if (open && peekCell === cell) {
      if (e) {
        showPeek(cell, date, e);
      } else if (peekEvent) {
        // Off a pill onto the cell's own ground, the event's card stays
        // long enough to reach - the pointer is most likely on its way
        // there - and the day's takes over only after that.
        peekTimer = setTimeout(() => showPeek(cell, date, null), 600);
      }
      return;
    }
    peekTimer = setTimeout(() => showPeek(cell, date, e), open ? 400 : 300);
  });
  // Leaving a cell for anywhere but the card starts the card closing -
  // whichever cell's it is, since passing through this one cancelled the
  // last cell's closing.
  cell.addEventListener('mouseleave', e => {
    clearTimeout(peekTimer);
    if (peek && !peek.hidden && e.relatedTarget && peek.contains(e.relatedTarget)) {
      return;
    }
    if (peek && !peek.hidden) {
      hidePeekSoon();
    }
  });
}

// A scroll moves the cells out from under the card, so it goes at once.
document.addEventListener('scroll', () => {
  if (peek && !peek.hidden) {
    hidePeek();
  }
}, {capture: true, passive: true});

// dayCell opens its day; a pill in it opens that event instead. The cell
// is a box rather than a link so the pills can be links of their own.
function dayCell(date, month) {
  const cell = el('div', 'month-cell' + (monthOf(date) === month ? '' : ' is-outside') + (date === today() ? ' is-today' : '') + (date === state.day ? ' is-on' : '') + (isSchoolDay(date) ? '' : ' is-off'));
  cell.append(link('/day/' + date, 'month-cell-num', String(parseDate(date).getDate())));
  cell.addEventListener('click', e => {
    if (e.target.closest('a') || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) {
      return;
    }
    hidePeek();
    go('/day/' + date);
  });
  const groups = specials(date);
  const all = selectedClassrooms();
  for (const g of groups) {
    const mark = el('div', 'month-mark ' + dayTypeClass(g.name) + (state.query && dayTypeMatches(g.name, state.query) ? ' is-match' : ''), g.classrooms.length === all.length ? g.name : `${g.name} (${g.classrooms.length})`);
    cell.append(mark);
  }
  const events = eventsOn(date);
  const room = Math.max(0, 3 - groups.length);
  for (const e of events.slice(0, room)) {
    // An event the viewer hid is plain gray words, not a pill.
    const pip = link(eventPath(e), 'month-pip' + (isMatch(e) ? ' is-match' : '') + (isHidden(e) ? ' is-hidden' : ''));
    pip.style.setProperty('--c', eventTint(e));
    pip.append(el('span', 'month-pip-title', e.title));
    if (!e.allDay) {
      pip.append(el('span', 'month-pip-time', timeLine(e, date)));
    }
    pip.addEventListener('click', hidePeek);
    pip.peekEvent = e;
    cell.append(pip);
  }
  if (events.length > room) {
    cell.append(el('div', 'month-more', `+${events.length - room} more`));
  }
  attachPeek(cell, date);
  return cell;
}

// goToday opens today with the grid on its month, from wherever the page
// is paged to.
function goToday() {
  state.month = monthOf(today());
  go('/');
}

// monthGrid is paged in place with the arrows ahead of the month's name and,
// at the row's far end, the way to a feed of what the filters show and to
// today; a day cell opens that day in the rail.
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
    const feed = link('/feeds#new', 'button button-secondary button-small pager-feed');
    feed.append(svg('feed'), el('span', '', 'Get Feed'));
    feed.title = 'A feed for your own calendar app, of what the filters show';
    pager.append(back, fwd, el('h2', 'pager-label', monthLabel(month)), feed, button('Today', null, 'button button-secondary button-small pager-today', goToday));
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

// filterGroups are the classroom and tag chips above the month, repainted
// with the search words so the ones hiding matches light up.
function filterGroups() {
  const wrap = el('div', 'home-filters');
  const paint = () => fillFilters(wrap, {collapsible: true});
  paint();
  return {node: wrap, paint};
}

// The page on a wide window: the filters, then the month with upcoming
// beside it - the day itself is in the rail. On a phone, where there is no
// rail, the day heads the page and the filters are the drawer's.
export function homePage(date) {
  hidePeek();
  setTitle(date === today() ? 'Today' : longDayLabel(date));
  if (date !== lastDate || !state.month) {
    state.month = monthOf(date);
    lastDate = date;
  }
  state.day = date;
  const page = el('div', 'home');
  const day = dayColumn(date);
  const filters = filterGroups();
  const upcoming = upcomingPanel(date);
  const month = monthGrid();
  const grid = el('div', 'home-grid');
  grid.append(month.node, upcoming.node);
  page.append(day.node, filters.node, grid);
  // The search words light up what they find in every panel, and the
  // chips that hide more matches.
  setSearch('Search the year…', q => {
    state.query = q;
    day.paint();
    renderRailDay();
    filters.paint();
    upcoming.paint();
    month.paint();
  });
  return page;
}
