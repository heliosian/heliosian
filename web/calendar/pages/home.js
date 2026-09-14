import {state, eventsOn, today, addDays, parseDate, formatDate, longDayLabel, dayLabel, monthLabel, monthOf, shiftMonth, weekStart, specials, scheduleOn, isSchoolDay, dayTypeClass, dayTypeMatches, selectedClassrooms, eventTint, timeLine, eventPath, weekdayShort, call, isMatch, isHidden, isGray} from '../state.js';
import {el, link, svg, button, peopleLine, toast, popup, copyText, feedMark} from '../dom.js';
import {setTitle, setSearch, fillFilters, renderRailDay, editFeedPopup, makeDefaultFeed} from '../chrome.js';
import {dayColumn} from '../day.js';
import {callPill, emptyNote, roomDots, planCards} from '../events.js';
import {answerOf, answer, linkURL, selectedTags, classroomNames, tagNames, defaultFeedName, showsFeed, activeFeed, setActiveFeed, defaultFeed, feedURL, webcalURL} from '../state.js';

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
    node.append(peopleLine(e.minePeople, e.source === 'celebrate' ? 'ticket' : 'people'));
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
  // Under them, small: Hide - gray on the month, out of the lists - or
  // Show once hidden.
  const hide = el('button', 'day-peek-hide', word === 'hidden' ? 'Show event' : 'Hide event');
  hide.type = 'button';
  hide.addEventListener('click', () => say(word === 'hidden' ? '' : 'hidden'));
  node.append(hide);
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
    // An event the viewer hid, or said no to, is plain gray words, not a pill.
    const pip = link(eventPath(e), 'month-pip' + (isMatch(e) ? ' is-match' : '') + (isGray(e) ? ' is-hidden' : ''));
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

// saveCalendar keeps what the filters show as a saved calendar - a feed
// row, by name - listed under Calendar in the rail from then on; the feed
// address for a calendar app waits on the Feeds page until wanted. A small
// popup takes the name.
function saveCalendar() {
  const form = el('form');
  const rooms = selectedClassrooms();
  const tags = selectedTags();
  const roomWords = rooms.length === classroomNames().length ? 'every classroom' : rooms.join(', ');
  const tagWords = tags.length === tagNames().length ? 'every category' : `${tags.length} of ${tagNames().length} categories`;
  form.append(el('p', 'modal-intro', `Keeps what the filters show now - ${roomWords} \u00b7 ${tagWords} - under Calendar in the rail, to come back to by name. Get Feed then gives you its address for your own calendar app.`));
  const field = el('label', 'field');
  field.append(el('span', '', 'Name'));
  const name = el('input');
  name.type = 'text';
  name.maxLength = 80;
  name.value = defaultFeedName();
  field.append(name);
  form.append(field);
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('check'), el('span', '', 'Save'));
  const {shut} = popup('Save Calendar', form);
  actions.append(submit, button('Cancel', '', 'button button-secondary', shut), status);
  form.append(actions);
  form.addEventListener('submit', async e => {
    e.preventDefault();
    if (!rooms.length || !tags.length) {
      status.textContent = 'Pick at least one classroom and one category first.';
      status.classList.add('error');
      return;
    }
    submit.disabled = true;
    status.classList.remove('error');
    status.textContent = 'Saving\u2026';
    const body = {
      name: name.value.trim() || defaultFeedName(),
      classrooms: rooms.length === classroomNames().length ? [] : classroomNames().filter(c => rooms.includes(c)),
      tags: tags.length === tagNames().length ? [] : tagNames().filter(t => tags.includes(t)),
    };
    const res = await fetch('/api/calendar/feeds', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
    submit.disabled = false;
    if (!res.ok) {
      status.textContent = await res.text();
      status.classList.add('error');
      return;
    }
    shut();
    const made = await res.json();
    setActiveFeed(made.token);
    toast(`Saved. ${body.name} is under Calendar in the rail.`, 5000);
    const {load} = await import('../app.js');
    await load();
  });
  name.focus();
  name.select();
}

// saveButton is Save Calendar beside the month while the filters are not
// a saved calendar's. Working from a saved calendar it is a split button:
// a click saves the filters onto it, and the caret drops a menu with that
// and Save New Calendar. Working from My Heliosian, whose filters are
// locked, it is Save New Calendar alone.
function saveButton() {
  const feeds = state.model.feeds || [];
  if (!feeds.length) {
    const feed = button('Save Calendar', 'save', 'button button-secondary button-small pager-feed', saveCalendar);
    feed.title = 'Keep what the filters show, by name, under Calendar in the rail';
    return feed;
  }
  // My Heliosian is locked, so working from it there is only a new one -
  // and the button says so, with no menu to drop.
  const active = activeFeed();
  const working = active && !active.locked ? active : null;
  if (!working) {
    const fresh = button('Save New Calendar', 'save', 'button button-secondary button-small pager-feed', saveCalendar);
    fresh.title = 'Keep what the filters show under a new name';
    return fresh;
  }
  const split = el('div', 'split-button pager-feed');
  const main = button('Save Calendar', 'save', 'button button-secondary button-small', () => working ? saveOnto(working) : saveCalendar());
  main.title = working ? `Save these filters onto ${working.name}` : 'Keep what the filters show, by name, under Calendar in the rail';
  const caret = button('', 'down', 'button button-secondary button-small split-caret', () => {
    menu.hidden = !menu.hidden;
    if (!menu.hidden) {
      setTimeout(() => document.addEventListener('click', () => {
        menu.hidden = true;
      }, {once: true}), 0);
    }
  });
  caret.setAttribute('aria-label', 'More ways to save');
  const menu = el('div', 'split-menu');
  menu.hidden = true;
  if (working) {
    const onto = el('button', 'split-item is-working');
    onto.type = 'button';
    onto.append(el('span', 'split-item-title', `Save onto ${working.name}`), el('span', 'split-item-note', 'Replace what it shows with these filters'));
    onto.addEventListener('click', () => saveOnto(working));
    menu.append(onto);
  }
  const fresh = el('button', 'split-item');
  fresh.type = 'button';
  fresh.append(el('span', 'split-item-title', 'Save New Calendar\u2026'), el('span', 'split-item-note', 'Keep these filters under a new name'));
  fresh.addEventListener('click', saveCalendar);
  menu.append(fresh);
  split.append(main, caret, menu);
  return split;
}

// saveOnto puts the filters as they stand onto a saved calendar, keeping
// its name, mark and address.
async function saveOnto(f) {
  const rooms = selectedClassrooms();
  const tags = selectedTags();
  if (!rooms.length || !tags.length) {
    toast('Pick at least one classroom and one category first.');
    return;
  }
  const body = {
    token: f.token, name: f.name, emoji: f.emoji || '',
    classrooms: rooms.length === classroomNames().length ? [] : classroomNames().filter(c => rooms.includes(c)),
    tags: tags.length === tagNames().length ? [] : tagNames().filter(t => tags.includes(t)),
  };
  const res = await fetch('/api/calendar/feeds', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  if (!res.ok) {
    toast(await res.text());
    return;
  }
  setActiveFeed(f.token);
  toast(`Saved onto ${f.name}.`, 4000);
  const {load} = await import('../app.js');
  await load();
}

// getFeed is the popup for a saved calendar's feed: its address to copy,
// the two ways to subscribe, and a word on each calendar app.
function getFeed(f) {
  const body = el('div');
  body.append(el('p', 'modal-intro', `${f.name} as a feed your own calendar app subscribes to. It keeps itself up to date as the school calendar changes.`));
  const url = el('div', 'feed-url');
  const input = el('input');
  input.type = 'text';
  input.readOnly = true;
  input.value = feedURL(f.token);
  input.addEventListener('focus', () => input.select());
  url.append(input, button('Copy', 'copy', 'button button-small', () => copyText(feedURL(f.token), 'Feed address copied')));
  body.append(url);
  const actions = el('div', 'feed-actions');
  const open = el('a', 'button button-secondary button-small');
  open.href = webcalURL(f.token);
  open.append(svg('calendar'), el('span', '', 'Subscribe in my calendar app'));
  const google = el('a', 'button button-secondary button-small');
  google.href = 'https://calendar.google.com/calendar/u/0/r/settings/addbyurl';
  google.target = '_blank';
  google.rel = 'noopener';
  google.append(svg('open'), el('span', '', 'Add to Google Calendar'));
  actions.append(open, google);
  body.append(actions);
  const steps = el('ul', 'help-list');
  for (const words of [
    'Apple Calendar (iPhone, iPad, Mac): Subscribe in my calendar app opens it straight away.',
    'Google Calendar: Add to Google Calendar, paste the address under From URL, and Add calendar. Google refreshes every few hours.',
    'Outlook: Add calendar \u203a Subscribe from web, and paste the address.',
    'The address is the whole secret: anyone who has it can read the feed. Remove it on the Feeds page and it stops.',
  ]) {
    steps.append(el('li', '', words));
  }
  body.append(steps);
  popup('Get Feed', body);
  input.focus();
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
    pager.append(back, fwd, el('h2', 'pager-label', monthLabel(month)));
    // Save Calendar offers while the filters are not a saved calendar's;
    // once they are one's, Get Feed offers that calendar's address.
    const shown = (state.model.feeds || []).find(showsFeed);
    if (shown) {
      const feed = button('Get Feed', 'feed', 'button button-secondary button-small pager-feed', () => getFeed(shown));
      feed.title = 'The address for your own calendar app';
      pager.append(feed);
    } else {
      pager.append(saveButton());
    }
    pager.append(button('Today', null, 'button button-secondary button-small pager-today', goToday));
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
  // The saved calendar the viewer is working from, as a headline over the
  // page - with a word on it while the filters have moved off what it
  // carries, since Save Calendar saves back onto it.
  const shown = activeFeed();
  if (shown) {
    const headline = el('div', 'calendar-headline');
    // The mark is a button that opens the name-and-emoji popup.
    const mark = el('button', 'calendar-headline-mark');
    mark.type = 'button';
    mark.title = 'Change the name or emoji';
    mark.setAttribute('aria-label', 'Change the name or emoji');
    mark.append(feedMark(shown));
    mark.addEventListener('click', () => editFeedPopup(shown));
    headline.append(mark, el('h1', 'calendar-headline-name', shown.name));
    if (shown.locked) {
      const lock = el('span', 'calendar-lock');
      lock.title = 'Its filters are the calendar\u2019s own, and it cannot be removed';
      lock.append(svg('lock'));
      headline.append(lock);
    }
    if (!showsFeed(shown)) {
      const changed = el('span', 'calendar-changed', 'Filters changed \u00b7 not saved');
      changed.title = 'Save Calendar saves these filters onto ' + shown.name;
      headline.append(changed);
    }
    // The default calendar says so at the row's far end; any other offers
    // to become it, faintly.
    if (shown.token === defaultFeed().token) {
      const badge = el('span', 'calendar-default');
      badge.append(svg('star'), el('span', '', 'Default Calendar'));
      badge.title = 'The calendar this page opens to, and Heliosian reads';
      headline.append(badge);
    } else {
      const make = button('Make Default', 'star', 'calendar-make-default', () => makeDefaultFeed(shown));
      make.title = 'Open the calendar and Heliosian to ' + shown.name + ' from now on';
      headline.append(make);
    }
    page.append(headline);
  }
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
