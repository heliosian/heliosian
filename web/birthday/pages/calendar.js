import {state, parseDate, staffPath, stageClass, stageName, stages} from '../state.js';
import {el, link, svg, button} from '../dom.js';
import {setTitle} from '../chrome.js';

let month = null;

// The legend doubles as a filter: every kind shows until one is clicked, then
// only the clicked kinds do, and clicking the last one on shows everything again.
const kinds = [...stages.map(s => ({key: stageClass(s), label: stageName(s)})), {key: 'newsletter', label: 'Newsletter'}];
let shown = new Set();

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long', year: 'numeric'});

function entries() {
  const out = [];
  for (const sv of state.model.staff) {
    const day = parseDate(sv.birthdayThisYear);
    if (day) {
      out.push({key: day.toDateString(), title: sv.name, href: staffPath(sv), className: stageClass(sv.stage)});
    }
  }
  for (const date of state.model.newsletterDates) {
    const day = parseDate(date);
    out.push({key: day.toDateString(), title: 'Newsletter', href: '/newsletters', className: 'newsletter'});
  }
  return out;
}

// monthGrid draws one month of items - each keyed by its day's toDateString,
// with a title, a chip class, and a link or an onClick - with a class of its
// own for a compact drawing.
export function monthGrid(month, items, className) {
  const cal = el('div', 'calendar' + (className ? ' ' + className : ''));
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
      let chip;
      if (item.onClick) {
        chip = el('button', 'chip ' + item.className, item.title);
        chip.type = 'button';
        chip.addEventListener('click', item.onClick);
      } else {
        chip = link(item.href, 'chip ' + item.className, item.title);
      }
      chip.title = item.title;
      // A chip with lines to show carries them in a card that shows on hover.
      if (item.lines && item.lines.length) {
        const wrap = el('span', 'chip-wrap');
        const pop = el('span', 'chip-pop');
        for (const line of item.lines) {
          pop.append(el('span', 'chip-pop-line', line));
        }
        wrap.append(chip, pop);
        cell.append(wrap);
        continue;
      }
      cell.append(chip);
    }
    cal.append(cell);
  }
  return cal;
}

// monthNav is the Today button and the arrows, calling step with -1, 0 or 1.
export function monthNav(step) {
  const nav = el('div', 'page-actions calendar-nav');
  nav.append(button('Today', null, 'button button-secondary button-small', () => step(0)));
  const prev = el('button', 'icon-button');
  prev.type = 'button';
  prev.append(svg('prev'));
  prev.addEventListener('click', () => step(-1));
  const next = el('button', 'icon-button');
  next.type = 'button';
  next.append(svg('next'));
  next.addEventListener('click', () => step(1));
  nav.append(prev, next);
  return nav;
}

function grid() {
  return monthGrid(month, entries().filter(e => !shown.size || shown.has(e.className)));
}

export function calendarPage() {
  setTitle('Calendar');
  if (!month) {
    const now = new Date();
    month = new Date(now.getFullYear(), now.getMonth(), 1);
  }
  const page = el('div', 'list-page');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  const title = el('h1', 'page-title', monthFormat.format(month));
  main.append(title);
  const nav = monthNav(n => {
    const now = new Date();
    month = n ? new Date(month.getFullYear(), month.getMonth() + n, 1) : new Date(now.getFullYear(), now.getMonth(), 1);
    title.textContent = monthFormat.format(month);
    page.querySelector('.calendar').replaceWith(grid());
  });
  head.append(main, nav);
  const legend = el('div', 'legend');
  const paint = () => {
    for (const chip of legend.children) {
      chip.classList.toggle('is-off', shown.size > 0 && !shown.has(chip.dataset.kind));
    }
  };
  for (const kind of kinds) {
    const chip = el('button', 'chip ' + kind.key, kind.label);
    chip.type = 'button';
    chip.dataset.kind = kind.key;
    chip.title = 'Show only these';
    chip.addEventListener('click', () => {
      if (shown.has(kind.key)) {
        shown.delete(kind.key);
      } else {
        shown.add(kind.key);
      }
      if (shown.size === kinds.length) {
        shown.clear();
      }
      paint();
      page.querySelector('.calendar').replaceWith(grid());
    });
    legend.append(chip);
  }
  paint();
  page.append(head, legend, grid());
  return page;
}
