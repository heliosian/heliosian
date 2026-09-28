import {initAppSwitch, currentApp} from '/appswitch.js';
import {initUserMenu, renderAvatars, renderProfileLink} from '/usermenu.js';
import {initAlerts, renderAlerts} from '/alerts.js';
import {initSpoof} from '/spoof.js';
import {noteError} from '/feedback.js';
import {api} from '/api.js';
import {el} from '/elements.js';
import {me} from '/data.js';

let app = null;
let allowances = [];
let onSearch = null;
let carriedQuery = '';

const icon = d => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="${d}"/></svg>`;
const menuIcon = icon('M4 7h16M4 12h16M4 17h16');
const closeIcon = icon('M6 6l12 12M18 6L6 18');

const searchMarkup = `<div class="topbar-search" id="topbar-search">
<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
<input type="search" id="search-input" autocomplete="off" spellcheck="false" aria-label="Search">
<kbd class="topbar-key">/</kbd>
</div>`;

const accountMarkup = `<div class="topbar-user">
<span class="topbar-alert-wrap"><a class="topbar-alert topbar-alert-count stale-alert" hidden><span class="stale-count"></span></a></span>
<span class="topbar-alert-wrap"><a class="topbar-alert privacy-alert" hidden><svg viewBox="0 0 24 24"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/><line x1="12" x2="12" y1="9" y2="13"/><line x1="12" x2="12.01" y1="17" y2="17"/></svg></a></span>
<button type="button" class="user" id="user" aria-label="Account"><span class="user-avatar"></span></button>
<div class="user-menu" id="user-menu" hidden>
<div class="user-menu-email"></div>
<a class="user-menu-profile">View Profile</a>
<a href="/admin" class="user-menu-admin" data-link hidden>Admin Tools</a>
<form method="post" action="/auth/logout"><button>Sign Out</button></form>
</div>
</div>
<div class="app-switch">
<a class="app-switch-button" aria-label="Heliosian" aria-haspopup="menu"><img src="/brand/heliosian-tile.png" alt=""><svg viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></svg></a>
<div class="app-switch-menu" hidden></div>
</div>`;

function buildTopbar() {
  const bar = document.querySelector('#topbar');
  const menu = app.menuButton === false ? '' : `<button type="button" class="topbar-menu" id="menu-button" aria-label="Menu">${menuIcon}</button>`;
  bar.innerHTML = menu + (app.search ? searchMarkup : '<div class="topbar-spacer"></div>') + accountMarkup;
  if (app.search) {
    searchInput().placeholder = app.search.placeholder;
    if (app.search.results) {
      const results = el('div', 'search-results');
      results.id = 'search-results';
      results.hidden = true;
      bar.querySelector('#topbar-search').append(results);
    }
  }
  const admin = bar.querySelector('.user-menu-admin');
  for (const row of app.menuRows || []) {
    admin.before(row);
  }
}

export function isEditableTarget(target) {
  return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT' || target.isContentEditable;
}

export function onSlash(open) {
  document.addEventListener('keydown', e => {
    if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
      e.preventDefault();
      open();
    }
  });
}

export function searchInput() {
  return document.querySelector('#search-input');
}

export function appSymbol() {
  const mark = el('span', 'app-symbol');
  mark.setAttribute('aria-hidden', 'true');
  return mark;
}

export function setSearch(placeholder, handler) {
  onSearch = handler;
  const query = carriedQuery;
  carriedQuery = '';
  const input = searchInput();
  input.value = query;
  input.placeholder = placeholder || app.search.placeholder;
  if (query) {
    handler(query.trim().toLowerCase());
  }
}

export function clearSearch() {
  onSearch = null;
  const input = searchInput();
  input.value = '';
  input.placeholder = app.search.placeholder;
}

function search(value) {
  if (onSearch) {
    onSearch(value.trim().toLowerCase());
    return;
  }
  if (!value.trim() || !app.search.carry) {
    return;
  }
  carriedQuery = value;
  app.search.carry();
}

export function setTitle(title) {
  document.querySelector('#mobile-title').textContent = title;
  document.title = title === app.name ? title : `${title} · ${app.name}`;
}

export function syncViewportHeight() {
  const standalone = window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
  const height = standalone ? screen.height : window.innerHeight;
  document.documentElement.style.setProperty('--vh100', height + 'px');
}

export function renderAccount() {
  const user = app.me();
  renderAvatars({photoUrl: user.photoUrl && user.photoUrl + '?thumb=1', initial: user.initial});
  if (app.alerts) {
    renderAlerts(app.alerts());
  }
  renderProfileLink(user.email);
  for (const line of document.querySelectorAll('.user-menu-email')) {
    line.textContent = user.email;
  }
  renderAdmin();
}

function adminHere() {
  return allowances.includes(currentApp() + '.admins');
}

function renderAdmin() {
  for (const row of document.querySelectorAll('.user-menu-admin')) {
    row.hidden = !adminHere();
  }
}

function renderDrawer() {
  const drawer = document.querySelector('#drawer');
  drawer.replaceChildren();
  const head = el('div', 'drawer-head');
  const mark = el('img');
  mark.src = '/brand/logo-mark.png';
  mark.alt = '';
  const close = el('button', 'icon-button');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  close.innerHTML = closeIcon;
  close.addEventListener('click', closeDrawer);
  head.append(mark, el('span', '', app.name), close);
  const nav = el('nav', 'app-nav drawer-nav');
  app.fillNav(nav);
  const user = el('div', 'drawer-user');
  user.append(el('div', 'name', app.me().name), el('div', 'email', app.me().email));
  if (adminHere()) {
    const admin = el('a', 'drawer-admin', 'Admin Tools');
    admin.href = '/admin';
    admin.setAttribute('data-link', '');
    user.append(admin);
  }
  const form = el('form');
  form.method = 'post';
  form.action = '/auth/logout';
  form.append(el('button', 'button button-secondary button-small', 'Sign Out'));
  user.append(form);
  drawer.append(head, nav, user);
}

export function openDrawer() {
  renderDrawer();
  document.querySelector('#drawer-overlay').hidden = false;
}

export function closeDrawer() {
  document.querySelector('#drawer-overlay').hidden = true;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.user-menu')) {
    menu.hidden = true;
  }
  if (app.closeMenus) {
    app.closeMenus();
  }
}

export function renderChrome() {
  syncViewportHeight();
  renderAccount();
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  app.fillNav(nav);
  const bar = document.querySelector('#tabbar');
  bar.replaceChildren();
  app.fillTabbar(bar);
  if (app.afterRender) {
    app.afterRender();
  }
}

export function showPage(node) {
  const page = document.querySelector('#page');
  page.className = '';
  page.replaceChildren(node);
  renderChrome();
}

const heldSafeArea = {};

// iOS reports a zero safe area once an in-app browser closes over a home-screen app,
// so keep the largest inset seen in each orientation.
function holdSafeArea(probe) {
  const style = getComputedStyle(probe);
  const orientation = window.innerWidth > window.innerHeight ? 'landscape' : 'portrait';
  const held = heldSafeArea[orientation] || {top: 0, bottom: 0};
  held.top = Math.max(held.top, parseFloat(style.paddingTop));
  held.bottom = Math.max(held.bottom, parseFloat(style.paddingBottom));
  heldSafeArea[orientation] = held;
  document.documentElement.style.setProperty('--safe-top', held.top + 'px');
  document.documentElement.style.setProperty('--safe-bottom', held.bottom + 'px');
}

export function initTopbar(config) {
  app = config;
  const probe = el('div', 'safe-area-probe');
  document.body.append(probe);
  holdSafeArea(probe);
  window.addEventListener('resize', () => holdSafeArea(probe));
  buildTopbar();
  initAppSwitch();
  if (app.search && !app.search.own) {
    searchInput().addEventListener('input', () => search(searchInput().value));
    onSlash(() => searchInput().focus());
  }
  initAlerts();
  initUserMenu();
  initSpoof();
  me().then(m => {
    allowances = m.allowances;
    renderAdmin();
  }).catch(err => noteError('/api/me: ' + err.message));
  if (!app.alerts) {
    api('GET', '/api/apps/alerts').catch(err => {
      noteError('/api/apps/alerts: ' + err.message);
      return {broken: true};
    }).then(renderAlerts);
  }
  document.addEventListener('click', e => {
    if (app.keepOpen && e.target.closest(app.keepOpen)) {
      return;
    }
    closeMenus();
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeMenus();
    }
  });
}

export function initShell(config) {
  initTopbar(config);
  document.querySelector('#menu-button').addEventListener('click', openDrawer);
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
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeDrawer();
    }
  });
  window.addEventListener('resize', syncViewportHeight);
  window.addEventListener('orientationchange', syncViewportHeight);
}
