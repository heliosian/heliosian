import {today, dayLabel, eventTint, timeLine, eventPath, answerOf, answer, linkURL} from './state.js';
import {peopleLine} from './dom.js';
import {el, link, svg, button} from '/elements.js';
import {roomDots, planCards} from './events.js';
import {answerIcon} from './inviteparts.js';

let peek = null;
let peekTimer = 0;
let peekCell = null;
let peekEvent = null;

function peekNode() {
  if (!peek) {
    peek = el('div', 'day-peek');
    peek.hidden = true;
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
    node.append(el('div', 'day-peek-words', e.description.length > 160 ? e.description.slice(0, 160).replace(/\s+\S*$/, '') + '…' : e.description));
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
  if (word === 'yes' || word === 'maybe' || word === 'no') {
    const row = el('div', 'day-peek-said');
    const said = el('div', 'invite-said rsvp-compact-said is-' + word);
    const mark = el('span', 'invite-said-mark');
    mark.append(svg(answerIcon(word)));
    said.append(mark, el('strong', '', word === 'yes' ? 'You’re going' : word === 'maybe' ? 'Maybe' : 'Not going'));
    row.append(said, button('Change', null, 'link-button', () => row.replaceWith(buttons)));
    node.append(row);
  } else {
    node.append(buttons);
  }
  const hide = el('button', 'day-peek-hide', word === 'hidden' ? 'Show event' : 'Hide event');
  hide.type = 'button';
  hide.addEventListener('click', () => say(word === 'hidden' ? '' : 'hidden'));
  node.append(hide);
}

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

export function hidePeek() {
  clearTimeout(peekTimer);
  clearTimeout(peekGrace);
  peekCell = null;
  peekEvent = null;
  if (peek) {
    peek.hidden = true;
  }
}

let peekGrace = 0;

function hidePeekSoon() {
  clearTimeout(peekGrace);
  peekGrace = setTimeout(hidePeek, 250);
}

function leavePeek(e) {
  clearTimeout(peekTimer);
  if (peek && !peek.hidden && e.relatedTarget && peek.contains(e.relatedTarget)) {
    return;
  }
  if (peek && !peek.hidden) {
    hidePeekSoon();
  }
}

export function attachPeek(cell, date) {
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
        peekTimer = setTimeout(() => showPeek(cell, date, null), 600);
      }
      return;
    }
    peekTimer = setTimeout(() => showPeek(cell, date, e), open ? 400 : 300);
  });
  cell.addEventListener('mouseleave', leavePeek);
}

export function attachRowPeek(row, date, e) {
  row.addEventListener('mouseenter', () => {
    const open = peek && !peek.hidden;
    clearTimeout(peekGrace);
    clearTimeout(peekTimer);
    if (open && peekCell === row) {
      return;
    }
    peekTimer = setTimeout(() => showPeek(row, date, e), open ? 400 : 300);
  });
  row.addEventListener('mouseleave', leavePeek);
  row.addEventListener('click', hidePeek);
}

document.addEventListener('scroll', () => {
  if (peek && !peek.hidden) {
    hidePeek();
  }
}, {capture: true, passive: true});
