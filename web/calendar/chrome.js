import {state, me, bands, selectedClassrooms, toggleClassroom, setClassrooms, classroomNames, myClassrooms, tagNames, selectedTags, toggleTag, setTags, resetFilters, filtersAreDefault, colorOf, hiddenMatches, hiddenClassroomMatches} from './state.js';
import {el, svg, link, button} from './dom.js';
import {renderAvatars, renderAlerts, onSlash, initAppSwitch} from '/toolbar.js';

const primary = [
  {href: '/', icon: 'today', label: 'Calendar'},
  {href: '/feeds', icon: 'feed', label: 'Feeds'},
];

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return path === '/' || path.startsWith('/day/') || path.startsWith('/events/');
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

// A filter change repaints the page and carries the search words across,
// so turning on a lit tag shows the matches it was hiding.
function refresh() {
  carriedQuery = typed();
  document.dispatchEvent(new CustomEvent('calendar:refresh'));
}

function chip(label, on, onClick, color) {
  const b = el('button', 'filter-chip' + (on ? ' is-on' : ''), label);
  b.type = 'button';
  if (color) {
    b.style.setProperty('--room', color);
    b.classList.add('has-color');
  }
  b.addEventListener('click', onClick);
  return b;
}

// action is one of the words at a filter group's right edge: the same shape
// in every group, All first.
function action(label, on, onClick) {
  const b = el('button', 'filter-action' + (on ? ' is-on' : ''), label);
  b.type = 'button';
  b.addEventListener('click', onClick);
  return b;
}

function groupHead(title, actions) {
  const head = el('div', 'filter-head');
  head.append(el('span', 'filter-title', title));
  const wrap = el('span', 'filter-actions');
  for (const a of actions) {
    wrap.append(a);
  }
  head.append(wrap);
  return head;
}

// fillFilters draws the two filter groups: the classrooms, one line per band
// in their colors, and the tags in alphabetical order. Every change is
// remembered and repaints.
export function fillFilters(wrap) {
  wrap.replaceChildren();
  const rooms = el('div', 'filter-group');
  const roomActions = [action('All', selectedClassrooms().length === classroomNames().length, () => {
    setClassrooms(classroomNames());
    refresh();
  })];
  if (myClassrooms().length) {
    roomActions.push(action('Mine', selectedClassrooms().join() === myClassrooms().join(), () => {
      setClassrooms(null);
      refresh();
    }));
  }
  rooms.append(groupHead('Classrooms', roomActions));
  const selected = selectedClassrooms();
  const roomRows = el('div', 'filter-rows');
  for (const band of bands()) {
    const roomChips = el('div', 'filter-chips');
    roomRows.append(roomChips);
    for (const c of band.classrooms) {
      const on = selected.includes(c.name);
      const room = chip(c.name, on, () => {
        toggleClassroom(c.name);
        refresh();
      }, colorOf(c.name));
      const hidden = on ? 0 : hiddenClassroomMatches(c.name);
      if (hidden) {
        room.classList.add('has-hidden');
        room.title = `${hidden} match${hidden === 1 ? '' : 'es'} for ${c.name}, which is off`;
      }
      roomChips.append(room);
    }
  }
  rooms.append(roomRows);
  wrap.append(rooms);

  const tags = el('div', 'filter-group');
  tags.append(groupHead('Show', [
    action('All', selectedTags().length === tagNames().length, () => {
      setTags(null);
      refresh();
    }),
    action('None', selectedTags().length === 0, () => {
      setTags([]);
      refresh();
    }),
  ]));
  const chips = el('div', 'filter-chips');
  const on = selectedTags();
  const sorted = [...state.model.tags].sort((a, b) => a.name.localeCompare(b.name));
  for (const t of sorted) {
    const c = chip(t.name, on.includes(t.name), () => {
      toggleTag(t.name);
      refresh();
    });
    c.title = t.description;
    // A tag that is off but hides things the search words find lights up,
    // with how many.
    const hidden = on.includes(t.name) ? 0 : hiddenMatches(t.name);
    if (hidden) {
      c.classList.add('has-hidden');
      c.title = `${hidden} match${hidden === 1 ? '' : 'es'} under ${t.name}, which is off`;
    }
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

export function renderFilters() {
  fillFilters(document.querySelector('#filters'));
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  fillNav(nav);
  renderFilters();
}

function renderTabbar() {
  const bar = document.querySelector('#tabbar');
  bar.replaceChildren();
  for (const item of primary) {
    bar.append(navLink(item));
  }
  const filters = el('a', '');
  filters.href = '#';
  filters.append(svg('tag'), el('span', '', 'Filters'));
  filters.addEventListener('click', e => {
    e.preventDefault();
    openDrawer();
  });
  bar.append(filters);
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

// Two search boxes with one value: the rail's on a wide window, the top
// bar's on a phone. The calendar page binds them to its three panels; any
// other page jumps home with the words carried along.
let onSearch = null;
let carriedQuery = '';

const defaultPlaceholder = 'Search the year…';

function searchInputs() {
  return [document.querySelector('#search-input'), document.querySelector('#rail-search-input')];
}

function typed() {
  return searchInputs()[0].value;
}

// sync writes the words into both boxes and marks the page as searching.
function sync(value) {
  for (const input of searchInputs()) {
    if (input.value !== value) {
      input.value = value;
    }
  }
  document.body.classList.toggle('is-searching', Boolean(value.trim()));
}

export function setSearch(placeholder, handler) {
  onSearch = handler;
  const query = carriedQuery;
  carriedQuery = '';
  sync(query);
  for (const input of searchInputs()) {
    input.placeholder = placeholder || defaultPlaceholder;
  }
  if (query) {
    handler(query.trim());
  }
}

export function clearSearch() {
  onSearch = null;
  state.query = '';
  sync('');
  for (const input of searchInputs()) {
    input.placeholder = defaultPlaceholder;
  }
}

export function resetSearch() {
  search('');
}

function search(value) {
  sync(value);
  if (onSearch) {
    onSearch(value.trim());
    return;
  }
  if (!value.trim()) {
    return;
  }
  carriedQuery = value;
  history.pushState(null, '', '/');
  refresh();
}

function focusSearch() {
  const visible = searchInputs().find(input => input.offsetParent !== null);
  if (visible) {
    visible.focus();
  }
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
  for (const input of searchInputs()) {
    input.addEventListener('input', () => search(input.value));
  }
  onSlash(focusSearch);
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
