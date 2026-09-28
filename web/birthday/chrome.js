import {state, me, isAdmin, isUnassigned, commsOnly, onComms, mine, urgency} from './state.js';
import {el, svg, link} from '/elements.js';
import {initShell, appSymbol} from '/shell.js';
import {load, setPath} from '/router.js';

const unassignedItem = {href: '/unassigned', icon: 'groups', label: 'Unassigned'};
const calendarItem = {href: '/calendar', icon: 'calendar', label: 'Calendar'};
const newslettersItem = {href: '/newsletters', icon: 'newsletter', label: 'Newsletters'};

const primaryItems = [
  {href: '/jobs', icon: 'jobs', label: 'My Jobs'},
  {href: '/process', icon: 'process', label: 'Process'},
  calendarItem,
];

function primary() {
  if (state.model && commsOnly()) {
    return [newslettersItem, calendarItem];
  }
  return state.model && state.model.staff.some(isUnassigned) ? [unassignedItem, ...primaryItems] : primaryItems;
}

const moreItems = [newslettersItem, {href: '/charities', icon: 'gift', label: 'Charities'}];

const skippedItem = {href: '/skipped', icon: 'warn', label: 'Skipped'};

function more() {
  if (commsOnly()) {
    return [];
  }
  return isAdmin() ? [...moreItems, skippedItem] : moreItems;
}

function active(href) {
  const path = location.pathname;
  if (path === '/') {
    if (state.model && commsOnly()) {
      return href === '/newsletters';
    }
    const anyUnassigned = state.model && state.model.staff.some(isUnassigned);
    return href === (anyUnassigned ? '/unassigned' : '/jobs');
  }
  if (href === '/process') {
    return path === '/process' || path.startsWith('/staff/');
  }
  return path === href || path.startsWith(href + '/');
}

function flag(href) {
  const staff = state.model ? state.model.staff : [];
  const due = rows => {
    const u = rows.map(urgency);
    const late = u.filter(x => x.when === 'late').length;
    const today = u.filter(x => x.when === 'today').length;
    if (!late && !today) {
      return null;
    }
    const words = [late ? `${late} late` : '', today ? `${today} due today` : ''].filter(Boolean).join(', ');
    return circle(late + today, words);
  };
  const own = sv => urgency(sv).step !== 'newsletter';
  switch (href) {
    case '/unassigned': {
      const open = staff.filter(isUnassigned).length;
      return open ? circle(open, `${open} ${open === 1 ? 'birthday has' : 'birthdays have'} nobody yet`) : null;
    }
    case '/jobs':
      return due(staff.filter(sv => mine(sv) && own(sv)));
    case '/process':
      return isAdmin() ? due(staff) : null;
    case '/newsletters':
      return isAdmin() || onComms() ? due(staff.filter(sv => urgency(sv).step === 'newsletter')) : null;
  }
  return null;
}

function circle(n, title) {
  const c = el('span', 'nav-flag', String(n));
  c.title = title;
  c.setAttribute('aria-label', title);
  return c;
}

function navLink(item, first) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(first ? appSymbol() : svg(item.icon), el('span', '', item.label));
  const mark = flag(item.href);
  if (mark) {
    a.append(mark);
  }
  return a;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.row-menu')) {
    menu.hidden = true;
  }
}

function fillNav(nav) {
  const items = [...primary(), ...more()];
  for (const item of items) {
    nav.append(navLink(item, item === items[0]));
  }
}

function fillTabbar(bar) {
  for (const item of primary().slice(0, 3)) {
    bar.append(navLink(item));
  }
}

export function initChrome() {
  initShell({
    name: 'Helios Staff Birthdays',
    me,
    onSuper: load,
    fillNav,
    fillTabbar,
    search: {
      placeholder: 'Search staff…',
      carry: () => setPath('/process'),
    },
    closeMenus,
  });
}
