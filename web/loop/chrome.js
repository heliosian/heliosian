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

function active(href) {
  return location.pathname === href;
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
    if (g.run) {
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
    section(nav, 'loop.runningOpen', 'star', 'Running', groups.filter(g => g.run && g.member), true);
    section(nav, 'loop.joinedOpen', 'check', 'Joined', groups.filter(g => !g.run && g.member), true);
    section(nav, 'loop.managingOpen', 'edit', 'Managing', groups.filter(g => g.run && !g.member), false);
    section(nav, 'loop.otherOpen', 'mail', 'Other Email Lists', groups.filter(g => !g.run && !g.member), false);
  }
}

function section(nav, key, icon, title, groups, openFirst) {
  if (!groups.length) {
    return;
  }
  const g = openGroup();
  const here = Boolean(g && groups.includes(g));
  const open = here || sectionOpen(key, openFirst);
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

function sectionOpen(key, openFirst) {
  const kept = localStorage.getItem(key);
  return kept === null ? openFirst : kept === '1';
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
