import {state, me, isAdmin, isSystemAdmin, setSuperEdit, isUnassigned} from './state.js';
import {el, svg, link} from './dom.js';
import {renderAvatars, renderAlerts, renderProfileLink, onSlash, initAppSwitch, initUserMenu, initSpoof, markSuper} from '/toolbar.js';

const appName = 'Helios Staff Birthdays';

// The rail and the drawer show all of these; the phone's tab bar keeps the
// first three. Unassigned leads, and only while someone is unassigned. /admin
// is deliberately absent - Admin Tools is reached from the account menu, as
// in every app.
const unassignedItem = {href: '/unassigned', icon: 'users', label: 'Unassigned'};

const primaryItems = [
  {href: '/jobs', icon: 'jobs', label: 'My Jobs'},
  {href: '/process', icon: 'process', label: 'Process'},
  {href: '/calendar', icon: 'calendar', label: 'Calendar'},
];

function primary() {
  return state.model && state.model.staff.some(isUnassigned) ? [unassignedItem, ...primaryItems] : primaryItems;
}

const moreItems = [
  {href: '/newsletters', icon: 'newsletter', label: 'Newsletters'},
];

// Charities and Skipped - the charity list, and who is missing a birthday
// or opted out - are the admins' tabs; a charity's page still opens for
// anyone from a donation.
const adminItems = [
  {href: '/charities', icon: 'gift', label: 'Charities'},
  {href: '/skipped', icon: 'skipped', label: 'Skipped'},
];

function more() {
  return isSystemAdmin() ? [...moreItems, ...adminItems] : moreItems;
}

function active(href) {
  const path = location.pathname;
  // The front page stands for whichever of the two it is showing.
  if (path === '/') {
    const anyUnassigned = state.model && state.model.staff.some(isUnassigned);
    return href === (anyUnassigned ? '/unassigned' : '/jobs');
  }
  if (href === '/process') {
    return path === '/process' || path.startsWith('/staff/');
  }
  return path === href || path.startsWith(href + '/');
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(svg(item.icon), el('span', '', item.label));
  return a;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.row-menu, .user-menu')) {
    menu.hidden = true;
  }
}

function fillNav(nav) {
  for (const item of [...primary(), ...more()]) {
    nav.append(navLink(item));
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
  for (const item of primary().slice(0, 3)) {
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
  // The switch and Admin Tools go with being on the admin list; what the
  // pages offer comes and goes with the hat.
  for (const row of document.querySelectorAll('.user-menu-super, .user-menu-admin')) {
    row.hidden = !isSystemAdmin();
  }
  for (const box of document.querySelectorAll('.super-edit-checkbox')) {
    box.checked = state.superEdit;
  }
  markSuper(isAdmin());
}

// One search box, in the top bar, and each page says what it filters. app.js
// clears the binding on every route change. A page that filters nothing (a
// staff member's page, the calendar) keeps the box, with the process list
// as its subject: typing there jumps to that list with the words carried
// along.
let onSearch = null;
let carriedQuery = '';

const defaultPlaceholder = 'Search staff…';

function searchInput() {
  return document.querySelector('#search-input');
}

export function setSearch(placeholder, handler) {
  onSearch = handler;
  const query = carriedQuery;
  carriedQuery = '';
  const input = searchInput();
  input.value = query;
  input.placeholder = placeholder || defaultPlaceholder;
  if (query) {
    handler(query.trim().toLowerCase());
  }
}

export function clearSearch() {
  onSearch = null;
  const input = searchInput();
  input.value = '';
  input.placeholder = defaultPlaceholder;
}

function search(value) {
  if (onSearch) {
    onSearch(value.trim().toLowerCase());
    return;
  }
  if (!value.trim()) {
    return;
  }
  carriedQuery = value;
  history.pushState(null, '', '/process');
  document.dispatchEvent(new CustomEvent('birthday:refresh'));
}

export function setTitle(title) {
  document.querySelector('#mobile-title').textContent = title;
  document.title = title === appName ? title : `${title} · ${appName}`;
}

// syncViewportHeight is the fix Helios Who? carries for the phone shell: in
// standalone mode 100dvh can settle short after an in-page route change, so
// --vh100 stands in for it (see team's chrome.js for the long version).
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
  searchInput().addEventListener('input', () => search(searchInput().value));
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
  // Super Admin Mode puts an admin's hat on or takes it off; the page
  // repaints as the other kind of user.
  for (const box of document.querySelectorAll('.super-edit-checkbox')) {
    box.addEventListener('change', () => {
      setSuperEdit(box.checked);
      document.dispatchEvent(new CustomEvent('birthday:refresh'));
    });
  }
  window.addEventListener('resize', syncViewportHeight);
  window.addEventListener('orientationchange', syncViewportHeight);
  document.addEventListener('click', closeMenus);
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeMenus();
      closeDrawer();
    }
  });
}
