import {state, isAdmin, descendants, parseWhen, activityPath} from '../state.js';
import {el, link, svg, toggle, button} from '../dom.js';
import {setTitle} from '../chrome.js';

let month = null;
let showUnapproved = false;

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long', year: 'numeric'});

function shown(status) {
  return status === 'Open' || status === 'Done' || (showUnapproved && status === 'Pending');
}

// entries lists every dated activity, root or child, one per day it spans.
function entries() {
  const out = [];
  const add = (node, isChild) => {
    const start = parseWhen(node.start);
    if (!start || !shown(node.status)) {
      return;
    }
    const end = parseWhen(node.end);
    const last = end ? end.date : start.date;
    const day = new Date(start.date.getFullYear(), start.date.getMonth(), start.date.getDate());
    for (let i = 0; i < 31 && day <= last; i++) {
      out.push({key: day.toDateString(), title: node.title, href: activityPath(node), isChild});
      day.setDate(day.getDate() + 1);
    }
  };
  for (const act of state.model.activities) {
    add(act, false);
    if (shown(act.status)) {
      for (const child of descendants(act)) {
        add(child, true);
      }
    }
  }
  return out;
}

function grid() {
  const items = entries();
  const cal = el('div', 'calendar');
  for (const d of ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']) {
    cal.append(el('div', 'dow', d));
  }
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  const start = new Date(first);
  start.setDate(1 - first.getDay());
  const today = new Date().toDateString();
  for (let i = 0; i < 42; i++) {
    const day = new Date(start);
    day.setDate(start.getDate() + i);
    const cell = el('div', 'day' + (day.getMonth() !== month.getMonth() ? ' other' : '') + (day.toDateString() === today ? ' today' : ''));
    cell.append(el('div', 'num', String(day.getDate())));
    for (const item of items.filter(e => e.key === day.toDateString())) {
      const chip = link(item.href, 'chip' + (item.isChild ? ' role' : ''), item.title);
      chip.title = item.title;
      cell.append(chip);
    }
    cal.append(cell);
  }
  return cal;
}

export function calendarPage() {
  setTitle('HCA Calendar');
  if (!month) {
    const now = new Date();
    month = new Date(now.getFullYear(), now.getMonth(), 1);
  }
  const page = el('div', 'list-page');
  if (isAdmin()) {
    page.append(toggle('Show Unapproved', showUnapproved, on => {
      showUnapproved = on;
      page.querySelector('.calendar').replaceWith(grid());
    }));
  }
  page.append(el('h1', '', 'Calendar'));
  const head = el('div', 'calendar-head');
  const title = el('h2', '', monthFormat.format(month));
  const nav = el('div', 'row-actions');
  const step = n => {
    month = new Date(month.getFullYear(), month.getMonth() + n, 1);
    title.textContent = monthFormat.format(month);
    page.querySelector('.calendar').replaceWith(grid());
  };
  nav.append(button('Today', null, 'button button-secondary button-small', () => {
    const now = new Date();
    month = new Date(now.getFullYear(), now.getMonth(), 1);
    step(0);
  }));
  const prev = el('button', 'icon-button');
  prev.type = 'button';
  prev.append(svg('prev'));
  prev.addEventListener('click', () => step(-1));
  const next = el('button', 'icon-button');
  next.type = 'button';
  next.append(svg('next'));
  next.addEventListener('click', () => step(1));
  nav.append(prev, next);
  head.append(title, nav);
  page.append(head, grid());
  return page;
}
