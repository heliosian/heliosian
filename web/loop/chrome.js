import {state, me, groupPath} from './state.js';
import {el, svg, link} from '/elements.js';
import {initShell, appSymbol} from '/shell.js';

const items = [
  {href: '/', icon: 'app', label: 'My Email Lists'},
  {href: '/new', icon: 'plus', label: 'New Email List'},
];

function openGroup() {
  const path = decodeURIComponent(location.pathname);
  return state.model ? state.model.groups.find(g => path === decodeURIComponent(groupPath(g))) : null;
}

function yours(g) {
  return g.mine || g.member;
}

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    const g = openGroup();
    return path === '/' || (path.startsWith('/groups/') && !(g && (g.archived || !yours(g))));
  }
  return path === href;
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(item.icon === 'app' ? appSymbol() : svg(item.icon), el('span', '', item.label));
  return a;
}

function groupRows(groups) {
  const sub = el('div', 'nav-sub');
  for (const g of groups) {
    const row = link(groupPath(g), 'nav-sub-item' + (decodeURIComponent(location.pathname) === decodeURIComponent(groupPath(g)) ? ' is-on' : ''));
    row.append(el('span', 'nav-sub-name', g.title || g.name));
    if (g.mine) {
      const icon = svg('star');
      icon.classList.add('nav-sub-manage');
      icon.setAttribute('aria-label', 'You manage it');
      row.append(icon);
    }
    if (g.members) {
      row.append(el('span', 'nav-sub-count', String(g.members.length)));
    }
    sub.append(row);
  }
  return sub;
}

function fillNav(nav) {
  for (const item of items) {
    if (item.href === '/new') {
      const make = link('/new', 'button nav-action');
      make.append(svg('plus'), el('span', '', item.label));
      nav.append(make);
      continue;
    }
    nav.append(navLink(item));
    const groups = state.model ? state.model.groups : [];
    const current = groups.filter(g => yours(g) && !g.archived);
    if (current.length) {
      nav.append(groupRows(current));
    }
    section(nav, 'loop.otherOpen', 'mail', 'Other Email Lists', groups.filter(g => !yours(g) && !g.archived && g.visibility === 'everyone'));
    section(nav, 'loop.archivedOpen', 'archive', 'Archived', groups.filter(g => g.archived));
  }
}

function section(nav, key, icon, title, groups) {
  if (!groups.length) {
    return;
  }
  const g = openGroup();
  const here = Boolean(g && groups.includes(g));
  const open = here || sectionOpen(key);
  const heading = el('button', 'nav-heading-toggle' + (open ? ' open' : '') + (here ? ' is-active' : ''));
  heading.type = 'button';
  heading.setAttribute('aria-expanded', String(open));
  const chevron = el('span', 'nav-chevron');
  chevron.append(svg('chevron-down'));
  heading.append(svg(icon), el('span', 'nav-heading-title', title), el('span', 'nav-sub-count', String(groups.length)), chevron);
  heading.addEventListener('click', () => {
    setSectionOpen(key, !open);
    nav.replaceChildren();
    fillNav(nav);
  });
  nav.append(heading);
  if (open) {
    nav.append(groupRows(groups));
  }
}

function sectionOpen(key) {
  return localStorage.getItem(key) === '1';
}

function setSectionOpen(key, open) {
  localStorage.setItem(key, open ? '1' : '0');
}

function fillTabbar(bar) {
  for (const item of items) {
    bar.append(navLink(item));
  }
}

export function initChrome() {
  initShell({
    name: 'Helios Loop',
    me,
    fillNav,
    fillTabbar,
    search: {placeholder: 'Search email lists…'},
  });
}
