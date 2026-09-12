import {state, me, isAdmin, pendingParties, hostedParties, parties, household, familyMember, myPath} from './state.js';
import {el, svg, link, button} from './dom.js';
import {renderAvatars, renderAlerts, onSlash, initAppSwitch} from '/toolbar.js';
import {openParty} from './edit.js';

// The rail and the drawer show these; the phone's tab bar drops the admin one.
// /admin is deliberately absent - Admin Tools is reached from the account menu.
const primary = [
  {href: '/', icon: 'party', label: 'Parties'},
  {href: '/my', icon: 'home', label: "My Family's Parties"},
  {href: '/hosting', icon: 'star', label: 'Hosting', count: () => hostedParties().length},
  {href: '/approvals', icon: 'hourglass', label: 'Approval Needed', admin: true, count: () => pendingParties().length},
];

function navItems() {
  return primary.filter(item => !item.admin || isAdmin());
}

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return path === '/' || path.startsWith('/parties/') || path.startsWith('/celebrations/');
  }
  if (href === '/approvals') {
    return path === '/hosting' && state.tab === 'approvals';
  }
  return path === href || path.startsWith(href + '/');
}

// partiesPath is the parties page for the chosen celebration: the root for
// the current one, /celebrations/... for any other.
export function partiesPath() {
  const code = state.celebration;
  return code === state.model.current ? '/' : `/celebrations/${encodeURIComponent(code)}`;
}

function navLink(item) {
  let href = item.href === '/' ? partiesPath() : item.href;
  // Approval Needed is the Hosting page's admin tab.
  if (item.href === '/approvals') {
    href = '/hosting';
  }
  const a = link(href, active(item.href) ? 'is-active' : '');
  a.addEventListener('click', () => {
    if (item.href === '/approvals') {
      state.tab = 'approvals';
    } else if (item.href === '/hosting') {
      state.tab = 'mine';
    }
  });
  a.append(svg(item.icon), el('span', '', item.label));
  const n = item.count ? item.count() : 0;
  if (n) {
    a.append(el('span', item.admin ? 'nav-count is-alert' : 'nav-count', String(n)));
  }
  return a;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.user-menu')) {
    menu.hidden = true;
  }
}

// Hosting a party is the one thing anyone can start from anywhere, so it sits
// under the nav rather than on the parties page.
function hostButton() {
  return button('Host a Party', 'plus', 'button nav-action', () => openParty(null));
}

// The three tabs of the parties list sit under Parties in the rail, each
// with how many parties it holds for the chosen celebration - the same
// numbers the tab bar shows, from the same filter (pages/parties.js).
const listTabs = [
  {key: 'available', label: 'Available'},
  {key: 'waitlist', label: 'Waitlist'},
  {key: 'all', label: 'All Parties'},
];

export function inTab(p, tab) {
  switch (tab) {
    case 'available':
      return p.availability === 'available' && p.status === 'Open';
    case 'waitlist':
      return p.availability === 'waitlist' && p.status === 'Open';
  }
  return true;
}

function tabLinks() {
  const wrap = el('div', 'nav-sub');
  const onList = location.pathname === '/' || location.pathname.startsWith('/celebrations/');
  for (const t of listTabs) {
    const n = parties().filter(p => inTab(p, t.key)).length;
    const on = onList && state.tab === t.key;
    const item = el('button', 'nav-sub-item' + (on ? ' is-on' : ''));
    item.type = 'button';
    item.append(el('span', 'nav-sub-name', t.label), el('span', 'nav-sub-count', String(n)));
    item.addEventListener('click', async () => {
      state.tab = t.key;
      const {navigate} = await import('./app.js');
      navigate(partiesPath());
    });
    wrap.append(item);
  }
  return wrap;
}

// familyLinks sit under My Family's Parties: the viewer, then their partner
// and children, each with how many parties they hold a ticket to or wait
// for. Nobody with no household gets a list of one.
function familyLinks() {
  const people = household();
  if (people.length < 2) {
    return null;
  }
  const wrap = el('div', 'nav-sub');
  const current = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const shown = current[0] === 'my' && current[1] ? familyMember(current[1]) : null;
  people.forEach((person, i) => {
    const n = state.model.parties.filter(p => [...p.attendees, ...p.waitlisted].some(a => a.email === person.email)).length;
    const on = current[0] === 'my' && (i === 0 ? !current[1] : shown && shown.email === person.email);
    const a = link(i === 0 ? '/my' : myPath(person), 'nav-sub-item nav-family-item' + (on ? ' is-on' : ''));
    a.append(el('span', 'nav-sub-name', i === 0 ? 'Me' : person.name), el('span', 'nav-sub-count', String(n)));
    wrap.append(a);
  });
  return wrap;
}

function fillNav(nav) {
  for (const item of navItems()) {
    nav.append(navLink(item));
    if (item.href === '/') {
      nav.append(tabLinks());
    }
    if (item.href === '/my') {
      const people = familyLinks();
      if (people) {
        nav.append(people);
      }
    }
  }
  nav.append(hostButton());
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  fillNav(nav);
}

function renderTabbar() {
  const bar = document.querySelector('#tabbar');
  bar.replaceChildren();
  for (const item of primary.filter(i => !i.admin)) {
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
  head.append(icon, el('span', '', 'Helios Celebrate'), close);
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
  renderAvatars({photoUrl: user.photoUrl && user.photoUrl + '?thumb=1', initial: user.initial});
  renderAlerts(state.model.alerts || {});
  for (const line of document.querySelectorAll('.user-menu-email')) {
    line.textContent = user.email;
  }
  for (const admin of document.querySelectorAll('.user-menu-admin')) {
    admin.hidden = !isAdmin();
  }
}

// One search box, in the top bar, and each page says what it filters. app.js
// clears the binding on every route change. A page that filters nothing (a
// party's page) keeps the box, with the parties list as its subject: typing
// there jumps to that list with the words carried along.
let onSearch = null;
let carriedQuery = '';

const defaultPlaceholder = 'Search parties…';

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
  history.pushState(null, '', partiesPath());
  document.dispatchEvent(new CustomEvent('celebrate:refresh'));
}

export function setTitle(title) {
  document.querySelector('#mobile-title').textContent = title;
  document.title = `${title} · Helios Celebrate`;
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
  const panel = document.querySelector('#user-menu');
  document.querySelector('#user').addEventListener('click', e => {
    e.stopPropagation();
    const opening = panel.hidden;
    closeMenus();
    panel.hidden = !opening;
  });
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
