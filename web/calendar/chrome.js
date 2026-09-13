import {state, me, bands, selectedClassrooms, toggleClassroom, setClassrooms, classroomNames, myClassrooms, tagNames, selectedTags, toggleTag, setTags, resetFilters, filtersAreDefault} from './state.js';
import {el, svg, link, button} from './dom.js';
import {renderAvatars, renderAlerts, onSlash, initAppSwitch} from '/toolbar.js';

const primary = [
  {href: '/', icon: 'today', label: 'Today'},
  {href: '/upcoming', icon: 'upcoming', label: 'Upcoming'},
  {href: '/month', icon: 'calendar', label: 'Calendar'},
  {href: '/feeds', icon: 'feed', label: 'Feeds'},
];

function active(href) {
  const path = location.pathname;
  switch (href) {
    case '/':
      return path === '/' || path.startsWith('/day/');
    case '/month':
      return path.startsWith('/month') || path.startsWith('/week') || path === '/list';
  }
  return path === href || path.startsWith(href + '/');
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(svg(item.icon), el('span', '', item.label));
  return a;
}

function fillNav(nav) {
  for (const item of primary) {
    nav.append(navLink(item));
  }
}

function refresh() {
  document.dispatchEvent(new CustomEvent('calendar:refresh'));
}

function chip(label, on, onClick, className) {
  const b = el('button', 'filter-chip' + (on ? ' is-on' : '') + (className ? ' ' + className : ''), label);
  b.type = 'button';
  b.addEventListener('click', onClick);
  return b;
}

// fillFilters draws the two filter groups: the classrooms, grouped by band
// with Mine and All shortcuts, and the tags with All and None. Every change
// is remembered and repaints the page.
export function fillFilters(wrap) {
  wrap.replaceChildren();
  const rooms = el('div', 'filter-group');
  const roomsHead = el('div', 'filter-head');
  roomsHead.append(el('span', 'filter-title', 'Classrooms'));
  const roomActions = el('span', 'filter-actions');
  if (myClassrooms().length) {
    roomActions.append(chip('Mine', selectedClassrooms().join() === myClassrooms().join(), () => {
      setClassrooms(null);
      refresh();
    }, 'filter-chip-small'));
  }
  roomActions.append(chip('All', selectedClassrooms().length === classroomNames().length, () => {
    setClassrooms(classroomNames());
    refresh();
  }, 'filter-chip-small'));
  roomsHead.append(roomActions);
  rooms.append(roomsHead);
  const selected = selectedClassrooms();
  for (const band of bands()) {
    const row = el('div', 'filter-band');
    if (band.classrooms.length > 1) {
      row.append(el('span', 'filter-band-name', band.name));
    }
    const chips = el('div', 'filter-chips');
    for (const c of band.classrooms) {
      chips.append(chip(c.name, selected.includes(c.name), () => {
        toggleClassroom(c.name);
        refresh();
      }));
    }
    row.append(chips);
    rooms.append(row);
  }
  wrap.append(rooms);

  const tags = el('div', 'filter-group');
  const tagsHead = el('div', 'filter-head');
  tagsHead.append(el('span', 'filter-title', 'Show'));
  const tagActions = el('span', 'filter-actions');
  tagActions.append(chip('All', selectedTags().length === tagNames().length, () => {
    setTags(null);
    refresh();
  }, 'filter-chip-small'));
  tagActions.append(chip('None', selectedTags().length === 0, () => {
    setTags([]);
    refresh();
  }, 'filter-chip-small'));
  tagsHead.append(tagActions);
  tags.append(tagsHead);
  const chips = el('div', 'filter-chips');
  const on = selectedTags();
  for (const t of state.model.tags) {
    const c = chip(t.name, on.includes(t.name), () => {
      toggleTag(t.name);
      refresh();
    });
    c.title = t.description;
    chips.append(c);
  }
  tags.append(chips);
  wrap.append(tags);
  if (!filtersAreDefault()) {
    wrap.append(button('Reset filters', null, 'link-button filter-reset', () => {
      resetFilters();
      refresh();
    }));
  }
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  fillNav(nav);
  fillFilters(document.querySelector('#filters'));
}

function renderTabbar() {
  const bar = document.querySelector('#tabbar');
  bar.replaceChildren();
  for (const item of primary) {
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
  head.append(icon, el('span', '', 'Helios Calendar'), close);
  drawer.append(head);
  const nav = el('nav', 'app-nav drawer-nav');
  fillNav(nav);
  drawer.append(nav);
  const filters = el('div', 'rail-filters drawer-filters');
  fillFilters(filters);
  drawer.append(filters);
  const user = el('div', 'drawer-user');
  user.append(el('div', 'name', me().name), el('div', 'email', me().email));
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

function closeMenus() {
  for (const menu of document.querySelectorAll('.user-menu')) {
    menu.hidden = true;
  }
}

function renderUser() {
  const user = me();
  renderAvatars({photoUrl: user.photoUrl && user.photoUrl + '?thumb=1', initial: user.initial});
  renderAlerts(state.model.alerts || {});
  for (const line of document.querySelectorAll('.user-menu-email')) {
    line.textContent = user.email;
  }
}

// One search box, in the top bar. A page with a list binds it; any other
// page jumps to the full list with the words carried along.
let onSearch = null;
let carriedQuery = '';

const defaultPlaceholder = 'Search events…';

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
    handler(query.trim());
  }
}

export function clearSearch() {
  onSearch = null;
  state.query = '';
  const input = searchInput();
  input.value = '';
  input.placeholder = defaultPlaceholder;
}

function search(value) {
  if (onSearch) {
    onSearch(value.trim());
    return;
  }
  if (!value.trim()) {
    return;
  }
  carriedQuery = value;
  history.pushState(null, '', '/list');
  refresh();
}

export function setTitle(title) {
  document.querySelector('#mobile-title').textContent = title;
  document.title = `${title} · Helios Calendar`;
}

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
