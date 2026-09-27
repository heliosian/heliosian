import {state, me, isSystemAdmin, applySuperEdit, managed, groupPath} from './state.js';
import {el, svg, link} from './dom.js';
import {initShell, appSymbol} from '/shell.js';

const items = [
  {href: '/', icon: 'app', label: 'My Groups'},
  {href: '/new', icon: 'plus', label: 'New Group'},
];

function openGroup() {
  const path = decodeURIComponent(location.pathname);
  return state.model ? state.model.groups.find(g => path === decodeURIComponent(groupPath(g))) : null;
}

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    const g = openGroup();
    return path === '/' || (path.startsWith('/groups/') && !(g && g.archived));
  }
  return path === href;
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(item.icon === 'app' ? appSymbol() : svg(item.icon), el('span', '', item.label));
  return a;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.facet-panel')) {
    menu.hidden = true;
  }
  for (const open of document.querySelectorAll('.facet-button.open')) {
    open.classList.remove('open');
  }
}

function groupRows(groups) {
  const sub = el('div', 'nav-sub');
  for (const g of groups) {
    const row = link(groupPath(g), 'nav-sub-item' + (decodeURIComponent(location.pathname) === decodeURIComponent(groupPath(g)) ? ' is-on' : ''));
    row.append(el('span', 'nav-sub-name', g.title || g.name));
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
    const mine = state.model ? state.model.groups.filter(managed) : [];
    const current = mine.filter(g => !g.archived);
    const archived = mine.filter(g => g.archived);
    if (item.href === '/' && current.length) {
      nav.append(groupRows(current));
    }
    if (item.href === '/' && archived.length) {
      const g = openGroup();
      const here = Boolean(g && g.archived);
      const open = here || archivedOpen();
      const heading = el('button', 'nav-heading-toggle' + (open ? ' open' : '') + (here ? ' is-active' : ''));
      heading.type = 'button';
      heading.setAttribute('aria-expanded', String(open));
      const chevron = el('span', 'nav-chevron');
      chevron.append(svg('chevron'));
      heading.append(svg('archive'), el('span', 'nav-heading-title', 'Archived'), el('span', 'nav-sub-count', String(archived.length)), chevron);
      heading.addEventListener('click', () => {
        setArchivedOpen(!open);
        nav.replaceChildren();
        fillNav(nav);
      });
      nav.append(heading);
      if (open) {
        nav.append(groupRows(archived));
      }
    }
  }
}

const archivedKey = 'loop.archivedOpen';

function archivedOpen() {
  return localStorage.getItem(archivedKey) === '1';
}

function setArchivedOpen(open) {
  localStorage.setItem(archivedKey, open ? '1' : '0');
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
    isSystemAdmin,
    onSuper: async () => {
      applySuperEdit();
      const {render} = await import('./app.js');
      render();
    },
    fillNav,
    fillTabbar,
    search: {placeholder: 'Search groups…'},
    keepOpen: '.facet-wrap',
    closeMenus,
  });
}
