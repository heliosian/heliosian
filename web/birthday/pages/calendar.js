import {state, parseDate, staffPath, stageClass} from '../state.js';
import {el, link, svg, button} from '../dom.js';
import {setTitle} from '../chrome.js';

let month = null;

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
      const chip = link(item.href, 'chip ' + item.className, item.title);
      chip.title = item.title;
      cell.append(chip);
    }
    cal.append(cell);
  }
  return cal;
}

export function calendarPage() {
  setTitle('Calendar');
  if (!month) {
    const now = new Date();
    month = new Date(now.getFullYear(), now.getMonth(), 1);
  }
  const page = el('div', 'list-page');
  const head = el('div', 'calendar-head');
  const title = el('h1', '', monthFormat.format(month));
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
  const legend = el('div', 'legend');
  for (const stage of ['Wait', 'Awaiting Outreach', 'Awaiting Response', 'Awaiting Newsletter', 'Complete']) {
    legend.append(el('span', 'chip ' + stageClass(stage), stage));
  }
  legend.append(el('span', 'chip newsletter', 'Newsletter'));
  page.append(head, legend, grid());
  return page;
}
