import {state, me, isAdmin, isSystemAdmin, setSuperEdit, managed, groupPath} from './state.js';
import {el, svg, link} from './dom.js';
import {renderAvatars, renderAlerts, renderProfileLink, onSlash, initAppSwitch, initUserMenu, initSpoof, renderSuperToggle} from '/toolbar.js';

const appName = 'Helios Loop';

const items = [
  {href: '/', icon: 'app', label: 'My Groups'},
  {href: '/new', icon: 'plus', label: 'New Group'},
];

// The group whose page is open, if any.
function openGroup() {
  const path = decodeURIComponent(location.pathname);
  return state.model ? state.model.groups.find(g => path === decodeURIComponent(groupPath(g))) : null;
}

// My Groups is lit on the front page and on a group's page, unless the
// group is one the viewer has archived: then Archived is lit instead.
function active(href) {
  const path = location.pathname;
  if (href === '/') {
    const g = openGroup();
    return path === '/' || (path.startsWith('/groups/') && !(g && g.archived));
  }
  return path === href;
}

// appSymbol is the app's own mark, worn by the rail's first item (toolbar.css).
function appSymbol() {
  const mark = el('span', 'app-symbol');
  mark.setAttribute('aria-hidden', 'true');
  return mark;
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(item.icon === 'app' ? appSymbol() : svg(item.icon), el('span', '', item.label));
  return a;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.user-menu, .facet-panel')) {
    menu.hidden = true;
  }
  for (const open of document.querySelectorAll('.facet-button.open')) {
    open.classList.remove('open');
  }
}

// The rail lists the pages, and New Group is its action - the green
// button every rail has - rather than a row. Under My Groups sit the
// person's groups themselves, each opening its page, the one open lit,
// with the ones they have archived under their own heading below.
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
    // Archived comes folded, and opens on a click - remembered per browser -
    // or while one of its groups is the page open.
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
        // Rebuild whichever nav this is, the rail's or the drawer's.
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
  try {
    return localStorage.getItem(archivedKey) === '1';
  } catch {
    return false;
  }
}

function setArchivedOpen(open) {
  try {
    localStorage.setItem(archivedKey, open ? '1' : '0');
  } catch {
    // A browser with storage off simply forgets between pages.
  }
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  fillNav(nav);
}

function renderTabbar() {
  const bar = document.querySelector('#tabbar');
  bar.replaceChildren();
  for (const item of items) {
    bar.append(navLink(item));
  }
}

function renderDrawer() {
  const drawer = document.querySelector('#drawer');
  drawer.replaceChildren();
  const head = el('div', 'drawer-head');
  const icon = el('img');
  icon.src = '/brand/logo-mark.png';
  icon.alt = '';
  const close = el('button', 'icon-button');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  close.append(svg('close'));
  close.addEventListener('click', closeDrawer);
  head.append(icon, el('span', '', appName), close);
  drawer.append(head);
  const nav = el('nav', 'app-nav drawer-nav');
  fillNav(nav);
  drawer.append(nav);
  const user = el('div', 'drawer-user');
  user.append(el('div', 'name', me().name), el('div', 'email', me().email));
  if (isSystemAdmin()) {
    user.append(link('/admin', 'drawer-admin', 'Admin Tools'));
  }
  const form = el('form');
  form.method = 'post';
  form.action = '/auth/logout';
  form.append(el('button', 'button button-secondary button-small', 'Sign Out'));
  user.append(form);
  drawer.append(user);
}

export function openDrawer() {
  renderDrawer();
  document.querySelector('#drawer-overlay').hidden = false;
}

export function closeDrawer() {
  document.querySelector('#drawer-overlay').hidden = true;
}

function renderUser() {
  const user = me();
  renderAvatars({photoUrl: user.photoUrl, initial: user.initial});
  renderAlerts(state.model.alerts || {});
  renderProfileLink(user.email);
  for (const line of document.querySelectorAll('.user-menu-email')) {
    line.textContent = user.email;
  }
  // Admin Tools goes with being on the admin list; the pencil puts the
  // hat on or takes it off, and the page repaints as the other kind of user.
  for (const row of document.querySelectorAll('.user-menu-admin')) {
    row.hidden = !isSystemAdmin();
  }
  renderSuperToggle({show: isSystemAdmin(), on: state.superEdit, onToggle: async on => {
    setSuperEdit(on);
    const {render} = await import('./app.js');
    render();
  }});
}

// One search box, in the top bar; the front page filters its groups by
// it, and every other page leaves it be.
let onSearch = null;

function searchInput() {
  return document.querySelector('#search-input');
}

export function setSearch(handler) {
  onSearch = handler;
  const input = searchInput();
  if (input.value.trim()) {
    handler(input.value.trim().toLowerCase());
  }
}

export function clearSearch() {
  onSearch = null;
  searchInput().value = '';
}

export function setTitle(title) {
  document.querySelector('#mobile-title').textContent = title;
  document.title = title === appName ? title : `${title} · ${appName}`;
}

// syncViewportHeight is the fix Helios Who? carries for the phone shell: in
// standalone mode 100dvh can settle short after an in-page route change, so
// --vh100 stands in for it.
function syncViewportHeight() {
  const standalone = window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
  const height = standalone ? screen.height : window.innerHeight;
  document.documentElement.style.setProperty('--vh100', height + 'px');
}

export function renderChrome() {
  syncViewportHeight();
  renderUser();
  renderNav();
  renderTabbar();
}

export function initChrome() {
  initAppSwitch();
  document.querySelector('#menu-button').append(svg('menu'));
  document.querySelector('#menu-button').addEventListener('click', openDrawer);
  searchInput().addEventListener('input', () => {
    if (onSearch) {
      onSearch(searchInput().value.trim().toLowerCase());
    }
  });
  onSlash(() => searchInput().focus());
  document.querySelector('#drawer-overlay').addEventListener('click', e => {
    if (e.target === e.currentTarget) {
      closeDrawer();
    }
  });
  document.querySelector('#drawer').addEventListener('click', e => {
    if (e.target.closest('a')) {
      closeDrawer();
    }
  });
  initUserMenu();
  initSpoof();
  window.addEventListener('resize', syncViewportHeight);
  window.addEventListener('orientationchange', syncViewportHeight);
  document.addEventListener('click', e => {
    if (!e.target.closest('.facet-wrap')) {
      closeMenus();
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeMenus();
      closeDrawer();
    }
  });
}
