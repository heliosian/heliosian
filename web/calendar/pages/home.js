import {state, eventsOn, today, addDays, parseDate, formatDate, longDayLabel, dayLabel, monthLabel, monthOf, shiftMonth, weekStart, specials, scheduleOn, isSchoolDay, dayTypeMatches, selectedClassrooms, eventTint, timeLine, startTime, eventPath, weekdayShort, isMatch, isHidden, isGray} from '../state.js';
import {dayTypeClass} from '/daytype.js';
import {el, link, svg, button, peopleLine, toast, copyText, feedMark} from '../dom.js';
import {popup} from '/modal.js';
import {setTitle, setSearch, fillFilters, renderRailDay, editFeedPopup, makeDefaultFeed, calendarMenu} from '../chrome.js';
import {dayColumn, openAddEvent} from '../day.js';
import {callPill, emptyNote, roomDots, planCards} from '../events.js';
import {answerOf, answer, linkURL, selectedTags, classroomNames, tagNames, defaultFeedName, showsFeed, activeFeed, setActiveFeed, defaultFeed, feedURL, webcalURL} from '../state.js';

let lastDate = '';

// upcomingDay is the band over one day's rows: the weekday and the date at
// the left, how many things at the right, and the day itself behind it.
function upcomingDay(date, count) {
  const head = link('/day/' + date, 'up-dayhead');
  const when = el('span', 'up-dayhead-date');
  when.append(el('span', 'up-dayhead-dow', weekdayShort(date)), el('span', 'up-dayhead-sep', '\u00b7'), el('span', '', parseDate(date).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})));
  head.append(when, el('span', 'up-dayhead-count', `${count} ${count === 1 ? 'event' : 'events'}`));
  return head;
}

// upcomingRow is one thing on the day: a dot on a line in its colour at the
// left, the title - starred when the viewer hosts it - with, for an event
// another app runs, the way to its tickets or sign-up or where the
// household stands under it, and when it starts at the right. A day
// that is not regular and has no event of its own is a row too, in its day
// type's colour.
function upcomingRow(date, e, group) {
  const row = link(e ? eventPath(e) : '/day/' + date, 'up-row' + (e ? (isMatch(e) ? ' is-match' : '') + (e.pending ? ' is-pending' : '') + (e.declined ? ' is-declined' : '') + (e.sharing !== 'Public' ? ' is-invite' : '') : ' ' + dayTypeClass(group.name)));
  const rail = el('span', 'up-rail');
  rail.append(el('span', 'up-dot'));
  const body = el('span', 'up-body');
  const when = el('span', 'up-time');
  if (e) {
    row.style.setProperty('--c', eventTint(e));
    const title = el('span', 'up-title', e.title);
    if (e.hosted) {
      const star = svg('star');
      star.classList.add('host-star');
      star.setAttribute('aria-label', 'You host this');
      title.prepend(star);
    }
    body.append(title);
    if (e.link && e.call) {
      body.append(callPill(e));
    } else if (e.invited && !e.hosted && !e.cancelled && !answerOf(e)) {
      // An invitation still waiting for the viewer's word carries RSVP, to
      // its page where the word is given - as My Events' RSVP counts it.
      body.append(el('span', 'event-pill', 'RSVP'));
    }
    // Just when it starts - the page has the rest.
    when.textContent = e.allDay ? 'All day' : startTime(e, date);
  } else {
    const all = selectedClassrooms();
    body.append(el('span', 'up-title', group.classrooms.length === all.length ? group.name : `${group.name} \u00b7 ${group.classrooms.join(', ')}`));
  }
  row.append(rail, body, when);
  return row;
}

// upcomingPanel scrolls on its own beside the month: every event from the
// day shown on, and - while the Schedule tag is on - every day that is not
// regular, through the end of the year, day by day under a band for each,
// the first two weeks under Upcoming and the rest under Later This Year;
// the search words light up the rows they find.
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
      const rows = shown.length ? shown.map(e => upcomingRow(d, e)) : events.length ? [] : [upcomingRow(d, null, groups[0])];
      if (!rows.length) {
        continue;
      }
      const day = el('div', 'up-day');
      day.append(upcomingDay(d, rows.length), ...rows);
      body.append(day);
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
  if (e && e.more) {
    fillMorePeek(node, date, e.more);
    return;
  }
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

// fillMorePeek is the card on a cell's "+N more": the events that did not
// fit, each its hours and title in its colour, a link to its page, under
// the day, which opens the day.
function fillMorePeek(node, date, events) {
  const head = link('/day/' + date, 'day-peek-head');
  head.append(el('span', 'day-peek-date', dayLabel(date)));
  node.append(head);
  for (const e of events) {
    const row = link(eventPath(e), 'day-peek-row day-peek-event day-peek-more');
    row.style.setProperty('--c', eventTint(e));
    const body = el('span', 'day-peek-body');
    body.append(el('span', 'day-peek-time', e.allDay ? 'All day' : timeLine(e, date)), el('span', 'day-peek-title', e.title));
    row.append(el('span', 'day-peek-bar'), body);
    row.addEventListener('click', hidePeek);
    node.append(row);
  }
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
  // A party has no yes or no: Add to my calendar with a ticket in the
  // household, Add Ticket without one.
  if (e.source === 'celebrate') {
    const held = (e.minePeople || []).some(p => p.note !== 'waitlisted');
    if (held) {
      buttons.append(button(word === 'yes' ? 'Invite sent' : 'Add to my calendar', word === 'yes' ? 'check' : 'calendar', 'button button-small' + (word === 'yes' ? ' button-secondary' : ''), () => say('yes')));
    } else {
      const add = el('a', 'button button-small' + (e.availability === 'available' ? '' : ' button-secondary'));
      add.href = linkURL(e);
      add.append(svg('ticket'), el('span', '', e.availability === 'available' ? 'Add Ticket' : e.call || 'See the party'));
      buttons.append(add);
    }
    node.append(buttons);
    return;
  }
  buttons.append(
    button('Yes', 'check', 'button button-small' + (word === 'yes' ? '' : ' button-secondary'), () => say(word === 'yes' ? '' : 'yes')),
    button('Maybe', 'clock', 'button button-small' + (word === 'maybe' ? '' : ' button-secondary'), () => say(word === 'maybe' ? '' : 'maybe')),
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
    const more = ev.target.closest('.month-more');
    const e = pip ? pip.peekEvent : more ? {more: more.peekMore} : null;
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
  // Every event is in the cell, in a list that shows what fits and, under
  // the pointer, scrolls to the rest; "+N more" under it counts what is out
  // of view, and hovered lists them in a card.
  const events = eventsOn(date);
  const room = Math.max(1, 3 - groups.length);
  const pips = el('div', 'month-pips');
  for (const e of events) {
    // An event the viewer hid, or said no to, is plain gray words, not a pill.
    const pip = link(eventPath(e), 'month-pip' + (isMatch(e) ? ' is-match' : '') + (isGray(e) ? ' is-hidden' : '') + (e.pending ? ' is-pending' : '') + (e.declined ? ' is-declined' : '') + (e.sharing !== 'Public' ? ' is-invite' : ''));
    pip.style.setProperty('--c', eventTint(e));
    pip.append(el('span', 'month-pip-title', e.title));
    if (!e.allDay) {
      pip.append(el('span', 'month-pip-time', timeLine(e, date)));
    }
    pip.addEventListener('click', hidePeek);
    pip.peekEvent = e;
    pips.append(pip);
  }
  cell.append(pips);
  if (events.length) {
    const more = link('/day/' + date, 'month-more');
    more.hidden = true;
    more.addEventListener('click', hidePeek);
    // What is out of view, counted from where the pills sit - which only
    // the drawn page knows - on every resize and scroll of the list.
    const count = () => {
      // The list stands as tall as its first few pills, as many as the
      // cell showed before it scrolled, so the month's rows keep their size.
      if (!pips.style.maxHeight && pips.children.length > room && pips.offsetHeight) {
        const top = pips.getBoundingClientRect().top;
        pips.style.maxHeight = `${pips.children[room - 1].getBoundingClientRect().bottom - top}px`;
      }
      const box = pips.getBoundingClientRect();
      const out = [...pips.children].filter(p => {
        const r = p.getBoundingClientRect();
        return r.bottom > box.bottom + 1 || r.top < box.top - 1;
      });
      more.hidden = !out.length;
      more.textContent = `+${out.length} more`;
      more.peekMore = out.map(p => p.peekEvent);
    };
    new ResizeObserver(count).observe(pips);
    pips.addEventListener('scroll', count, {passive: true});
    cell.append(more);
  }
  attachPeek(cell, date);
  return cell;
}

// saveCalendar keeps what the filters show as a saved calendar - a feed
// row, by name - listed under Calendar in the rail from then on; its
// Subscribe menu puts it into a calendar app. A small popup takes the
// name.
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

// subscribeMenu is Subscribe beside the calendar's name: the calendar as a
// feed a calendar app keeps in step with - into Google Calendar, into
// Apple Calendar, or its address to copy for any other. A saved calendar's
// address is its own token's; My Heliosian's is asked of the server, which
// makes one for its owner the first time.
async function feedToken(f) {
  if (!f.locked) {
    return f.token;
  }
  const res = await fetch('/api/calendar/feeds/my-heliosian', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{}'});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return (await res.json()).token;
}

// The companies' own marks for their calendars: Google's four-colour G and
// Apple's apple, in the ink of the text beside it.
const brandMarks = {
  google: '<svg class="subscribe-mark" viewBox="0 0 48 48" aria-hidden="true"><path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z"/><path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"/><path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"/><path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"/></svg>',
  apple: '<svg class="subscribe-mark subscribe-mark-apple" viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M12.152 6.896c-.948 0-2.415-1.078-3.96-1.04-2.04.027-3.91 1.183-4.961 3.014-2.117 3.675-.546 9.103 1.519 12.09 1.013 1.454 2.208 3.09 3.792 3.039 1.52-.065 2.09-.987 3.935-.987 1.831 0 2.35.987 3.96.948 1.637-.026 2.676-1.48 3.676-2.948 1.156-1.688 1.636-3.325 1.662-3.415-.039-.013-3.182-1.221-3.22-4.857-.026-3.04 2.48-4.494 2.597-4.559-1.429-2.09-3.623-2.324-4.39-2.376-2-.156-3.675 1.09-4.61 1.09zM15.53 3.83c.843-1.012 1.4-2.427 1.245-3.83-1.207.052-2.662.805-3.532 1.818-.78.896-1.454 2.338-1.273 3.714 1.338.104 2.715-.688 3.559-1.701"/></svg>',
};

function subscribeMenu(f) {
  const wrap = el('div', 'subscribe');
  const open = button('Subscribe', 'calendar', 'button button-secondary button-small subscribe-button', () => {
    menu.hidden = !menu.hidden;
    if (!menu.hidden) {
      document.addEventListener('click', () => {
        menu.hidden = true;
      }, {once: true});
    }
  });
  open.append(svg('down'));
  const menu = el('div', 'subscribe-menu');
  menu.hidden = true;
  menu.addEventListener('click', e => e.stopPropagation());
  menu.append(el('div', 'subscribe-title', `Subscribe to ${f.name}`), el('p', 'subscribe-lead', 'Keep this calendar in sync with your calendar app. Changes to it update there on their own.'));
  const item = (icon, words, onClick) => {
    const b = el('button', 'subscribe-item');
    b.type = 'button';
    if (brandMarks[icon]) {
      b.insertAdjacentHTML('beforeend', brandMarks[icon]);
    } else {
      b.append(svg(icon));
    }
    b.append(el('span', '', words));
    b.addEventListener('click', async () => {
      try {
        onClick(await feedToken(f));
      } catch (err) {
        toast(err.message);
      }
    });
    menu.append(b);
  };
  item('google', 'Add to Google Calendar', token => window.open('https://calendar.google.com/calendar/render?cid=' + encodeURIComponent(webcalURL(token)), '_blank', 'noopener'));
  item('apple', 'Add to Apple Calendar', token => {
    location.href = webcalURL(token);
  });
  item('link', 'Copy calendar feed URL', token => copyText(feedURL(token), 'Feed address copied'));
  const note = el('div', 'subscribe-note');
  note.append(svg('info'), el('span', '', 'Use the feed address with Outlook and other calendar apps.'));
  menu.append(note);
  wrap.append(open, menu);
  return wrap;
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
    const add = button('Add Event', 'plus', 'button button-small pager-add', openAddEvent);
    add.title = 'Add an event for the community; an admin approves a public one onto the calendar';
    pager.append(add, button('Today', null, 'button button-secondary button-small pager-today', goToday));
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
    mark.append(feedMark(shown, true));
    mark.addEventListener('click', () => editFeedPopup(shown));
    // The name drops a menu of every calendar, to switch without the rail.
    const pick = el('div', 'calendar-pick');
    const name = el('button', 'calendar-headline-name');
    name.type = 'button';
    name.title = 'Switch calendar';
    name.append(el('h1', '', shown.name), svg('down'));
    const menu = calendarMenu(() => {
      menu.hidden = true;
    });
    menu.hidden = true;
    name.addEventListener('click', e => {
      e.stopPropagation();
      menu.hidden = !menu.hidden;
      if (!menu.hidden) {
        document.addEventListener('click', () => {
          menu.hidden = true;
        }, {once: true});
      }
    });
    menu.addEventListener('click', e => e.stopPropagation());
    pick.append(name, menu);
    headline.append(mark, pick);
    if (shown.locked) {
      const lock = el('span', 'calendar-lock');
      lock.title = 'You can\u2019t modify this calendar, but you can create a new one based off of it.';
      lock.append(svg('lock'));
      headline.append(lock);
    }
    if (!showsFeed(shown)) {
      const changed = el('span', 'calendar-changed', 'Filters changed \u00b7 not saved');
      changed.title = shown.locked ? 'My Heliosian keeps its own filters; save these under a new name' : 'Save Calendar saves these filters onto ' + shown.name;
      headline.append(changed, saveButton());
    }
    // Subscribe, then the default: at the row's far end.
    headline.append(subscribeMenu(shown));
    // The default calendar says so at the row's far end; any other offers
    // to become it, faintly.
    if (shown.token === defaultFeed().token) {
      const badge = el('span', 'calendar-default');
      badge.append(svg('pushpin'), el('span', '', 'Default Calendar'));
      badge.title = 'The calendar this page opens to, and Heliosian reads';
      headline.append(badge);
    } else {
      const make = button('Make Default', 'pushpin', 'calendar-make-default', () => makeDefaultFeed(shown));
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
