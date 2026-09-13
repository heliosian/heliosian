import {state, me, today, bands, tagGroups, selectedClassrooms, toggleClassroom, setClassrooms, classroomNames, myClassrooms, tagNames, selectedTags, toggleTag, setTags, resetFilters, filtersAreDefault, colorOf, hiddenMatches, hiddenClassroomMatches} from './state.js';
import {el, svg, link, button} from './dom.js';
import {dayColumn} from './day.js';
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

// action is one of the pills after a row's label - All, Mine, None - the
// same shape in every row, All first.
function action(label, on, onClick) {
  const b = el('button', 'filter-action' + (on ? ' is-on' : ''), label);
  b.type = 'button';
  b.addEventListener('click', onClick);
  return b;
}

// filterRow is one row of the filters: its label, its pills, then its chips.
function filterRow(label, actions, chips) {
  const row = el('div', 'filter-row');
  row.append(el('span', 'filter-label', label));
  for (const a of actions) {
    row.append(a);
  }
  const wrap = el('span', 'filter-chips');
  for (const c of chips) {
    wrap.append(c);
  }
  row.append(wrap);
  return row;
}

function classroomChips() {
  const selected = selectedClassrooms();
  const chips = [];
  for (const band of bands()) {
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
      chips.push(room);
    }
  }
  return chips;
}

function tagChips(tags) {
  const on = selectedTags();
  const sorted = [...tags].sort((a, b) => a.name.localeCompare(b.name));
  return sorted.map(t => {
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
    return c;
  });
}

// filterSummary is the one line the folded filters show: which classrooms
// and how many categories.
function filterSummary() {
  const rooms = selectedClassrooms();
  const roomWords = rooms.length === classroomNames().length ? 'All classrooms' : rooms.length ? rooms.join(', ') : 'No classrooms';
  const tags = selectedTags();
  const tagWords = tags.length === tagNames().length ? 'all categories' : tags.length ? `${tags.length} of ${tagNames().length} categories` : 'no categories';
  return `${roomWords} · ${tagWords}`;
}

// Whether the calendar page's filters are unfolded - folded until someone
// opens them, and remembered per browser from then on.
function filtersOpen() {
  try {
    return localStorage.getItem('calendar.filtersOpen') === 'yes';
  } catch (err) {
    return false;
  }
}

function setFiltersOpen(open) {
  try {
    localStorage.setItem('calendar.filtersOpen', open ? 'yes' : 'no');
  } catch (err) {
    // A browser that refuses storage just folds them again next time.
  }
}

// fillFilters draws the rows: the classrooms, band by band in their colors,
// then the categories, one row per group the sheet files them under (the
// ungrouped last, as plain Categories), each in alphabetical order and
// headed by its label and its pills - All and None for that row alone -
// with the way to reset everything at the end. Every change is remembered
// and repaints. With opts.collapsible the rows sit under a head that folds
// them away to one line, as on the calendar page; the drawer shows them
// plain.
export function fillFilters(wrap, opts = {}) {
  wrap.replaceChildren();
  const open = !opts.collapsible || filtersOpen();
  if (opts.collapsible) {
    const head = el('button', 'filters-head');
    head.type = 'button';
    head.setAttribute('aria-expanded', String(open));
    head.append(el('span', 'filters-title', 'Filters'), el('span', 'filters-summary', filterSummary()), svg('chevron'));
    head.addEventListener('click', () => {
      setFiltersOpen(!open);
      fillFilters(wrap, opts);
    });
    wrap.append(head);
    wrap.classList.toggle('is-open', open);
  }
  if (!open) {
    return;
  }
  const rows = el('div', 'filter-rows');
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
  rows.append(filterRow('Classrooms', roomActions, classroomChips()));
  // pick keeps the tags in the sheet's order, and every tag on is the
  // default rather than a list of them all.
  const pick = list => setTags(list.length === tagNames().length ? null : tagNames().filter(t => list.includes(t)));
  let last = null;
  for (const group of tagGroups()) {
    const names = group.tags.map(t => t.name);
    const on = selectedTags();
    const onHere = names.filter(n => on.includes(n));
    last = filterRow(group.name || 'Categories', [
      action('All', onHere.length === names.length, () => {
        pick([...on, ...names]);
        refresh();
      }),
      action('None', onHere.length === 0, () => {
        pick(on.filter(n => !names.includes(n)));
        refresh();
      }),
    ], tagChips(group.tags));
    rows.append(last);
  }
  if (!filtersAreDefault()) {
    last.append(button('Reset filters', null, 'link-button filter-reset', () => {
      resetFilters();
      refresh();
    }));
  }
  wrap.append(rows);
}

// The rail's day column, under the nav on a wide window: the day last opened
// on the calendar page, today until one is. Drawn again after every page and
// whenever the search words change.
export function renderRailDay() {
  const day = dayColumn(state.day || today());
  document.querySelector('#rail-day').replaceChildren(day.node);
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  fillNav(nav);
  renderRailDay();
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
  // Admin Tools is in the account menu, for the calendar admins alone.
  for (const row of document.querySelectorAll('.user-menu-admin')) {
    row.hidden = !user.isAdmin;
  }
}

// The top bar's search box, as in the other apps. The calendar page binds it
// to its three panels; any other page jumps home with the words carried
// along.
let onSearch = null;
let carriedQuery = '';

const defaultPlaceholder = 'Search the year…';

function searchInput() {
  return document.querySelector('#search-input');
}

function typed() {
  return searchInput().value;
}

// sync writes the words into the box and marks the page as searching.
function sync(value) {
  const input = searchInput();
  if (input.value !== value) {
    input.value = value;
  }
  document.body.classList.toggle('is-searching', Boolean(value.trim()));
}

export function setSearch(placeholder, handler) {
  onSearch = handler;
  const query = carriedQuery;
  carriedQuery = '';
  sync(query);
  searchInput().placeholder = placeholder || defaultPlaceholder;
  if (query) {
    handler(query.trim());
  }
}

export function clearSearch() {
  onSearch = null;
  state.query = '';
  sync('');
  searchInput().placeholder = defaultPlaceholder;
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
  searchInput().focus();
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
  searchInput().addEventListener('input', () => search(typed()));
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
