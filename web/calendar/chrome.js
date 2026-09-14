import {state, me, today, bands, tagGroups, defaultTags, savedView, searchResults, eventPath, eventTint, timeLine, weekdayShort, parseDate, selectedClassrooms, toggleClassroom, setClassrooms, classroomNames, myClassrooms, tagNames, selectedTags, toggleTag, setTags, resetFilters, filtersAreDefault, colorOf, hiddenMatches, hiddenClassroomMatches} from './state.js';
import {el, svg, link, button, toast} from './dom.js';
import {dayColumn} from './day.js';
import {renderAvatars, renderAlerts, renderProfileLink, onSlash, initAppSwitch, initUserMenu} from '/toolbar.js';

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
  const saved = savedView() && filtersAreDefault() ? ' · your saved view' : '';
  return `${roomWords} · ${tagWords}${saved}`;
}

// Whether the calendar page's filters are unfolded: folded on every visit,
// open only while someone has opened them.
let filtersUnfolded = false;

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
  const open = !opts.collapsible || filtersUnfolded;
  if (opts.collapsible) {
    const head = el('div', 'filters-head');
    const fold = el('button', 'filters-fold');
    fold.type = 'button';
    fold.setAttribute('aria-expanded', String(open));
    fold.append(el('span', 'filters-title', 'Filters'), el('span', 'filters-summary', filterSummary()));
    fold.addEventListener('click', () => {
      filtersUnfolded = !open;
      fillFilters(wrap, opts);
    });
    head.append(fold);
    const chevron = el('button', 'filters-chevron');
    chevron.type = 'button';
    chevron.setAttribute('aria-label', open ? 'Fold the filters' : 'Unfold the filters');
    chevron.append(svg('chevron'));
    chevron.addEventListener('click', () => {
      filtersUnfolded = !open;
      fillFilters(wrap, opts);
    });
    head.append(chevron);
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
  // pick keeps the tags in the sheet's order, and the default set is the
  // default rather than a list of it.
  const pick = list => {
    const ordered = tagNames().filter(t => list.includes(t));
    setTags(ordered.join() === defaultTags().join() ? null : ordered);
  };
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
  wrap.append(rows);
  // Under the rows, the three sweeps: Select all turns every classroom and
  // category on, Clear all turns every category off (the classrooms stay,
  // since none at all shows nothing), and Reset filters returns to the
  // defaults - each only while it would change something.
  const foot = el('div', 'filters-foot');
  const everything = selectedClassrooms().length === classroomNames().length && selectedTags().length === tagNames().length;
  if (!everything) {
    foot.append(button('Select all', null, 'button button-secondary button-small', () => {
      setClassrooms(classroomNames());
      setTags(tagNames());
      refresh();
    }));
  }
  if (selectedTags().length) {
    foot.append(button('Clear all', null, 'button button-secondary button-small', () => {
      pick([]);
      refresh();
    }));
  }
  if (!filtersAreDefault()) {
    foot.append(button('Reset filters', null, 'button button-secondary button-small', () => {
      resetFilters();
      refresh();
    }));
    // Save keeps the choice as this person's own default, on every device
    // and for Heliosian's Upcoming Events.
    foot.append(button('Save as my default', 'check', 'button button-small', async () => {
      const res = await fetch('/api/calendar/settings', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({classrooms: selectedClassrooms(), tags: selectedTags()})});
      if (!res.ok) {
        toast(await res.text());
        return;
      }
      resetFilters();
      toast('Saved. The calendar opens to this view for you now - and the events on the Heliosian home page follow it too.', 6000);
      await reloadModel();
    }));
  } else if (savedView()) {
    foot.append(button('Forget my default', null, 'button button-secondary button-small', async () => {
      const res = await fetch('/api/calendar/settings', {method: 'DELETE'});
      if (!res.ok) {
        toast(await res.text());
        return;
      }
      toast('Back to the calendar\u2019s own defaults, here and on the Heliosian home page.', 5000);
      await reloadModel();
    }));
  }
  if (foot.childElementCount) {
    wrap.append(foot);
  }
}

async function reloadModel() {
  const {load} = await import('./app.js');
  await load();
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
  renderProfileLink(user.email);
  for (const line of document.querySelectorAll('.user-menu-email')) {
    line.textContent = user.email;
  }
  // Admin Tools is in the account menu, for the calendar admins alone.
  for (const row of document.querySelectorAll('.user-menu-admin')) {
    row.hidden = !user.isAdmin;
  }
}

// The top bar's search box, as in the other apps. Typing opens a list of
// the events the words find, whatever page is open - each with its date,
// walked with the arrow keys, Enter opening the one chosen - and on the
// calendar page the words also light up what they find in every panel.
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
  closeResults();
  searchInput().placeholder = defaultPlaceholder;
}

export function resetSearch() {
  search('');
}

function search(value) {
  sync(value);
  showResults(value.trim());
  if (onSearch) {
    onSearch(value.trim());
  }
}

// The list under the box: up to a dozen of the events the words find, in
// date order, with how many more there are; one the filters keep off the
// page says so. Arrow keys move the choice, Enter opens it, Escape closes
// the list, and a click on a row opens that one.
const resultLimit = 12;
let resultRows = [];
let activeRow = -1;

function resultsNode() {
  return document.querySelector('#search-results');
}

function closeResults() {
  const node = resultsNode();
  node.hidden = true;
  node.replaceChildren();
  resultRows = [];
  activeRow = -1;
}

function showResults(query) {
  const node = resultsNode();
  const found = query ? searchResults(query) : [];
  if (!found.length) {
    closeResults();
    if (query) {
      node.append(el('div', 'search-empty', 'Nothing matches.'));
      node.hidden = false;
    }
    return;
  }
  node.replaceChildren();
  resultRows = [];
  activeRow = -1;
  for (const {event, hidden} of found.slice(0, resultLimit)) {
    const row = link(eventPath(event), 'search-row' + (hidden ? ' is-hidden-by-filters' : ''));
    row.style.setProperty('--c', eventTint(event));
    const when = el('span', 'search-row-date');
    const day = event.start.slice(0, 10);
    // The year shows only when it is not this one, so last year's rows
    // are not mistaken for this year's.
    const dayWords = parseDate(day).toLocaleDateString('en-US', day.slice(0, 4) === today().slice(0, 4) ? {month: 'short', day: 'numeric'} : {month: 'short', day: 'numeric', year: 'numeric'});
    when.append(el('span', 'search-row-dow', weekdayShort(day)), el('span', 'search-row-day', dayWords));
    const body = el('span', 'search-row-body');
    body.append(el('span', 'search-row-title', event.title));
    const line = [timeLine(event)];
    if (event.location) {
      line.push(event.location);
    }
    if (hidden) {
      line.push('off under the filters');
    }
    body.append(el('span', 'search-row-line', line.join(' · ')));
    row.append(el('span', 'search-row-bar'), when, body);
    // The box keeps focus through a click on a row, so the list is still
    // there for the click to land on.
    row.addEventListener('mousedown', e => e.preventDefault());
    row.addEventListener('click', closeResults);
    row.addEventListener('mouseenter', () => setActive(resultRows.indexOf(row)));
    resultRows.push(row);
    node.append(row);
  }
  if (found.length > resultLimit) {
    node.append(el('div', 'search-more', `${found.length - resultLimit} more - keep typing to narrow it down`));
  }
  node.hidden = false;
}

function setActive(index) {
  activeRow = index;
  resultRows.forEach((row, i) => row.classList.toggle('is-active', i === index));
  if (index >= 0) {
    resultRows[index].scrollIntoView({block: 'nearest'});
  }
}

function onSearchKey(e) {
  const node = resultsNode();
  if (node.hidden) {
    return;
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault();
    if (!resultRows.length) {
      return;
    }
    const step = e.key === 'ArrowDown' ? 1 : -1;
    setActive((activeRow + step + resultRows.length) % resultRows.length);
  } else if (e.key === 'Enter') {
    if (activeRow >= 0) {
      e.preventDefault();
      resultRows[activeRow].click();
    }
  } else if (e.key === 'Escape') {
    e.preventDefault();
    closeResults();
    searchInput().blur();
  }
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
  searchInput().addEventListener('keydown', onSearchKey);
  // Back in the box with words still there, the list comes back; leaving
  // it, the list goes but the words and their highlights stay.
  searchInput().addEventListener('focus', () => showResults(typed().trim()));
  searchInput().addEventListener('blur', closeResults);
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
  initUserMenu();
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
