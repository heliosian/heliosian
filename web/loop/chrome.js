import {state, me, isAdmin, groupPath} from './state.js';
import {el, svg, link} from './dom.js';
import {renderAvatars, renderAlerts, renderProfileLink, onSlash, initAppSwitch, initUserMenu, initSpoof, markSuper} from '/toolbar.js';

const appName = 'Helios Loop';

const items = [
  {href: '/', icon: 'groups', label: 'My Groups'},
  {href: '/new', icon: 'plus', label: 'New Group'},
];

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return path === '/' || path.startsWith('/groups/');
  }
  return path === href;
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(svg(item.icon), el('span', '', item.label));
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
// person's groups themselves, each opening its page, the one open lit.
function fillNav(nav) {
  for (const item of items) {
    if (item.href === '/new') {
      const make = link('/new', 'button nav-action');
      make.append(svg('plus'), el('span', '', item.label));
      nav.append(make);
      continue;
    }
    nav.append(navLink(item));
    if (item.href === '/' && state.model && state.model.groups.length) {
      const sub = el('div', 'nav-sub');
      for (const g of state.model.groups) {
        const row = link(groupPath(g), 'nav-sub-item' + (decodeURIComponent(location.pathname) === decodeURIComponent(groupPath(g)) ? ' is-on' : ''));
        row.append(el('span', 'nav-sub-name', g.title || g.name));
        if (g.members) {
          row.append(el('span', 'nav-sub-count', String(g.members.length)));
        }
        sub.append(row);
      }
      nav.append(sub);
    }
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
  if (isAdmin()) {
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
  for (const row of document.querySelectorAll('.user-menu-admin')) {
    row.hidden = !isAdmin();
  }
  markSuper(false);
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
