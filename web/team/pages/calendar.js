import {state, descendants, mySignUp, rootOf, parentOf, revealed} from '../state.js';
import {parseWhen} from '/datecard.js';
import {el, link, svg, button} from '/elements.js';
import {setTitle} from '/shell.js';
import {categoryClass} from '../cards.js';

let month = null;
let filter = '';

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long', year: 'numeric'});

function shown(node) {
  return revealed(node);
}

function mine(node) {
  return Boolean(mySignUp(node));
}

function entries() {
  const out = [];
  const add = (node, isChild) => {
    const start = parseWhen(node.start);
    if (!start || !shown(node)) {
      return;
    }
    const own = mine(node);
    if (isChild && !own) {
      const parent = parentOf(node);
      if (node.whenFrom || (parent && parent.start === node.start && parent.end === node.end)) {
        return;
      }
    }
    const root = rootOf(node);
    if (filter === 'mine' ? !own : (filter && (root.category || '') !== filter)) {
      return;
    }
    const end = parseWhen(node.end);
    const last = end ? end.date : start.date;
    const day = new Date(start.date.getFullYear(), start.date.getMonth(), start.date.getDate());
    for (let i = 0; i < 31 && day <= last; i++) {
      out.push({key: day.toDateString(), title: node.title, href: node.path, isChild, mine: own, category: root.category || ''});
      day.setDate(day.getDate() + 1);
    }
  };
  for (const act of state.model.activities) {
    add(act, false);
    if (shown(act)) {
      for (const child of descendants(act)) {
        add(child, true);
      }
    }
  }
  return out;
}

function chipRow(onChange) {
  const row = el('div', 'chip-row calendar-chips');
  const paint = () => {
    row.replaceChildren();
    const add = (id, label, cls) => {
      const chip = el('button', 'chip ' + cls + (filter === id ? ' is-on' : ''));
      chip.type = 'button';
      chip.textContent = label;
      chip.addEventListener('click', () => {
        filter = filter === id ? '' : id;
        paint();
        onChange();
      });
      row.append(chip);
    };
    add('', 'All', 'chip-all');
    add('mine', '⭐ Mine', 'chip-mine');
    const present = new Set(state.model.activities.filter(a => parseWhen(a.start) && shown(a)).map(a => a.category || ''));
    for (const c of state.model.categories) {
      if (present.has(c.id)) {
        add(c.id, c.title, categoryClass(c.id));
      }
    }
  };
  paint();
  return row;
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
      const cls = item.mine ? ' mine' : ' ' + categoryClass(item.category) + (item.isChild ? ' role' : '');
      const chip = link(item.href, 'chip' + cls, (item.mine ? '⭐ ' : '') + item.title);
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
  page.append(el('h1', '', 'Calendar'));
  page.append(chipRow(() => page.querySelector('.calendar').replaceWith(grid())));
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
  prev.append(svg('chevron-left'));
  prev.addEventListener('click', () => step(-1));
  const next = el('button', 'icon-button');
  next.type = 'button';
  next.append(svg('chevron-right'));
  next.addEventListener('click', () => step(1));
  nav.append(prev, next);
  head.append(title, nav);
  page.append(head, grid());
  return page;
}
