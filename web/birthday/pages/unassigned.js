import {state, isUnassigned, parseDate, staffPath, stageClass, stageName, mediumDate} from '../state.js';
import {el, link, svg, thumb, button, pageHead} from '../dom.js';
import {setTitle} from '../chrome.js';
import {emptyPanel} from '../cards.js';
import {assignToMe} from '../edit.js';
import {monthGrid, monthNav} from './calendar.js';
import {appOrigin} from '/toolbar.js';

// The unassigned birthdays two ways at once: a list to pick from, by
// birthday, and the month they fall in. Picking one, in either, shows it
// under the month with the way to take it.
let month = null;
let picked = '';
// showBy is which day the month places each person on: the day to ask them
// by, or the birthday itself.
let showBy = 'ask';

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long', year: 'numeric'});

// A face's colour, for someone without a photo, settled by their name so it
// stays theirs from page to page.
const tints = ['orange', 'sky', 'green', 'yellow', 'red', 'teal'];

function tintOf(sv) {
  let n = 0;
  for (const ch of sv.email) {
    n = (n * 31 + ch.charCodeAt(0)) % 9973;
  }
  return 'face-' + tints[n % tints.length];
}

// dateLeaf is a day as a leaf off a tear-off calendar: the month on a red
// band, the day large, the weekday under it.
const leafMonth = new Intl.DateTimeFormat('en-US', {month: 'short'});
const leafWeekday = new Intl.DateTimeFormat('en-US', {weekday: 'short'});

function dateLeaf(date) {
  const leaf = el('div', 'date-leaf');
  const d = parseDate(date);
  leaf.append(el('div', 'date-leaf-month', d ? leafMonth.format(d) : '—'), el('div', 'date-leaf-day', d ? String(d.getDate()) : '?'), el('div', 'date-leaf-weekday', d ? leafWeekday.format(d) : ''));
  return leaf;
}

function unassigned() {
  return state.model.staff.filter(isUnassigned).sort((a, b) => (a.birthdayThisYear || '9').localeCompare(b.birthdayThisYear || '9'));
}

function pickedOne(rows) {
  return rows.find(sv => sv.email === picked) || rows[0] || null;
}

function row(sv, rerender) {
  const r = el('div', 'urow' + (sv.email === picked ? ' is-picked' : ''));
  r.append(thumb(sv, 'small ' + tintOf(sv)));
  const body = el('div', 'urow-body');
  const top = el('div', 'urow-top');
  top.append(el('span', 'label', sv.jobTitle || ''));
  // The day the month shows sits on the right with its icon; the other day
  // goes under the name.
  const byAsk = showBy === 'ask';
  body.append(top, el('div', 'urow-name', sv.name), el('div', 'urow-text', byAsk ? `Birthday ${mediumDate(sv.birthdayThisYear)}` : `Ask by ${mediumDate(sv.requestBy)}`));
  r.append(body);
  // The day the month shows, as a page off a calendar, on the right.
  // Beside the leaf, what the day is for: a letter to write, or a cake.
  const when = el('div', 'urow-when');
  const mark = svg(byAsk ? 'edit' : 'cake');
  mark.classList.add('day-mark');
  when.append(mark, dateLeaf(byAsk ? sv.requestBy : sv.birthdayThisYear));
  r.append(when);
  const open = link(staffPath(sv), 'urow-open');
  open.setAttribute('aria-label', `Open ${sv.name}`);
  open.append(svg('chevron'));
  open.addEventListener('click', e => e.stopPropagation());
  r.append(open);
  r.addEventListener('click', () => {
    picked = sv.email;
    rerender();
  });
  return r;
}

function list(rows, rerender) {
  if (!rows.length) {
    return emptyPanel('Everyone is assigned.');
  }
  const panel = el('div', 'urows');
  for (const sv of rows) {
    panel.append(row(sv, rerender));
  }
  return panel;
}

function grid(rows, rerender) {
  const items = [];
  for (const sv of rows) {
    const day = parseDate(showBy === 'ask' ? sv.requestBy : sv.birthdayThisYear);
    if (day) {
      items.push({key: day.toDateString(), title: sv.name, className: stageClass(sv.stage) + (sv.email === picked ? ' is-picked' : ''), onClick: () => {
        picked = sv.email;
        rerender();
      }});
    }
  }
  return monthGrid(month, items, 'compact picker');
}

// showToggle is the switch between the two days the month can show.
function showToggle(onChange) {
  const wrap = el('div', 'show-toggle');
  wrap.append(el('span', 'show-toggle-label', 'Show:'));
  const group = el('div', 'segmented');
  for (const [key, label] of [['ask', 'Ask by Date'], ['birthday', 'Actual Birthday']]) {
    const b = el('button', 'segment' + (showBy === key ? ' is-on' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      showBy = key;
      for (const other of group.children) {
        other.classList.toggle('is-on', other === b);
      }
      onChange();
    });
    group.append(b);
  }
  wrap.append(group);
  return wrap;
}

// pickCard is the one picked: who they are, when, and Take This One.
function pickCard(sv) {
  const card = el('div', 'pick-card');
  if (!sv) {
    return card;
  }
  const head = el('div', 'pick-head');
  head.append(thumb(sv, 'small ' + tintOf(sv)));
  const who = el('div', 'pick-who');
  const nameLine = el('div', 'pick-name');
  nameLine.append(el('span', '', sv.name), el('span', 'status-pill ' + stageClass(sv.stage), stageName(sv.stage)));
  who.append(nameLine, el('div', 'pick-title', sv.jobTitle || ''));
  head.append(who);
  const when = el('div', 'pick-when');
  when.append(el('div', 'pick-date', mediumDate(sv.birthdayThisYear)), el('div', 'pick-ask', `Ask by ${mediumDate(sv.requestBy)}`));
  head.append(when);
  card.append(head);
  const actions = el('div', 'pick-actions');
  // Their page on Helios Who, in a tab of its own.
  const whoLink = el('a', 'button button-secondary');
  whoLink.href = appOrigin('who') + '/people/' + encodeURIComponent(sv.email);
  whoLink.target = '_blank';
  whoLink.rel = 'noopener';
  whoLink.append(svg('open'), el('span', '', 'Open Helios Who'));
  actions.append(whoLink, button('Assign to Me', null, 'button', () => assignToMe(sv)));
  card.append(actions);
  return card;
}

export function unassignedPage() {
  setTitle('Unassigned');
  if (!month) {
    const now = new Date();
    month = new Date(now.getFullYear(), now.getMonth(), 1);
  }
  const page = el('div', 'list-page');
  const rows = unassigned();
  const head = pageHead('Unassigned');
  head.querySelector('.page-head-main').append(el('div', 'page-subtitle', rows.length ? `${rows.length} ${rows.length === 1 ? 'birthday has' : 'birthdays have'} nobody yet. Pick some up for yourself!` : 'Every birthday has someone.'));
  page.append(head);
  const columns = el('div', 'split');
  const left = el('div', 'split-list card-panel');
  const right = el('div', 'split-calendar');
  const calCard = el('div', 'card-panel');
  const calHead = el('div', 'split-calendar-head');
  const title = el('h2', '', monthFormat.format(month));
  const rerender = () => {
    left.replaceChildren(list(rows, rerender));
    calCard.querySelector('.calendar').replaceWith(grid(rows, rerender));
    right.querySelector('.pick-card').replaceWith(pickCard(pickedOne(rows)));
  };
  calHead.append(title, monthNav(n => {
    const now = new Date();
    month = n ? new Date(month.getFullYear(), month.getMonth() + n, 1) : new Date(now.getFullYear(), now.getMonth(), 1);
    title.textContent = monthFormat.format(month);
    calCard.querySelector('.calendar').replaceWith(grid(rows, rerender));
  }));
  picked = (pickedOne(rows) || {}).email || '';
  left.append(list(rows, rerender));
  calCard.append(calHead, showToggle(rerender), grid(rows, rerender));
  right.append(pickCard(pickedOne(rows)), calCard);
  columns.append(left, right);
  page.append(columns);
  return page;
}
