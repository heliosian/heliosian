import {state, me, settingsId, today, bands, tagGroups, defaultTags, savedView, searchResults, eventPath, eventTint, timeLine, weekdayShort, parseDate, selectedClassrooms, toggleClassroom, setClassrooms, classroomNames, myClassrooms, tagIds, selectedTags, toggleTag, setTags, resetFilters, filtersAreDefault, colorOf, hiddenMatches, hiddenClassroomMatches, feedClassrooms, feedTags, showsFeed, setActiveFeed, activeFeed, allCalendars, defaultFeed, myEvents} from './state.js';
import {feedMark, emojiPicker} from './dom.js';
import {el, svg, link, button, toast, longToast} from '/elements.js';
import {popup} from '/modal.js';
import {field, text} from '/form.js';
import {dayColumn} from './day.js';
import {initShell, appSymbol, onSlash} from '/shell.js';
import {load, render, setPath} from '/router.js';
import {act, remove as removeResource} from '/data.js';

const primary = [
  {href: '/', icon: 'app', label: 'Calendar'},
  {href: '/mine', icon: 'calcheck', label: 'My Events'},
];

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return path === '/' || path.startsWith('/c/') || path.startsWith('/day/') || path.startsWith('/e/') || path.startsWith('/events/');
  }
  return path === href || path.startsWith(href + '/');
}

function mineRows(nav, item) {
  const mine = myEvents();
  const sections = [
    ['/mine/rsvp', 'calendar', 'RSVP', mine.waiting, true],
    ['/mine/attending', 'check', 'Attending', mine.going, false],
    ['/mine/hosting', 'star', 'Hosting', mine.hosted, false],
    ['/mine/pending', 'clock', 'Pending Approval', mine.pending, false],
  ].filter(([, , , list]) => list.length);
  if (!sections.length) {
    return;
  }
  nav.append(link(item.href, location.pathname === item.href ? 'is-active' : ''));
  nav.lastChild.append(svg(item.icon), el('span', '', item.label));
  for (const [href, icon, words, list, owed] of sections) {
    const a = link(href, 'nav-sub nav-mine nav-mine-' + icon + (location.pathname === href ? ' is-active' : ''));
    a.append(svg(icon), el('span', 'nav-mine-words', words));
    if (list.length) {
      const badge = el('span', 'nav-badge' + (owed ? '' : ' is-quiet'), String(list.length));
      badge.title = owed ? `${list.length} ${list.length === 1 ? 'invitation waits' : 'invitations wait'} for your reply` : `${list.length} coming up`;
      a.append(badge);
    }
    nav.append(a);
  }
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(item.icon === 'app' ? appSymbol() : svg(item.icon), el('span', '', item.label));
  return a;
}

export function editFeedPopup(f) {
  const form = el('form');
  const name = text(f.name, {maxLength: 80});
  const nameField = field('Name', name);
  const emojiField = el('div', 'field');
  emojiField.append(el('span', '', 'Emoji'));
  const {node, input} = emojiPicker(f.emoji || '');
  emojiField.append(node, el('small', '', 'Shown before the name in the rail and over the calendar; none means the calendar icon.'));
  form.append(nameField, emojiField);
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('check'), el('span', '', 'Save'));
  const {shut} = popup('Edit calendar', form);
  const remove = f.locked ? el('span', 'modal-delete modal-locked', '\ud83d\udd12 Everyone keeps this one') : button('Delete calendar', 'trash', 'link-button danger modal-delete', async () => {
    if (!confirm(`Delete ${f.name}? A calendar app subscribed to its feed stops updating.`)) {
      return;
    }
    try {
      await removeResource('calendar-feeds', f.id);
    } catch (err) {
      toast(err.message);
      return;
    }
    shut();
    toast(`${f.name} deleted`);
    await load();
  });
  actions.append(submit, button('Cancel', '', 'button button-secondary', shut), status, remove);
  if (defaultFeed().token !== f.token) {
    const first = button('Make default', 'pin', 'button button-secondary modal-default', async () => {
      shut();
      await makeDefaultFeed(f);
    });
    actions.insertBefore(first, status);
  } else {
    actions.insertBefore(el('span', 'modal-default-note', '\u2605 Your default calendar'), status);
  }
  form.append(actions);
  form.addEventListener('submit', async e => {
    e.preventDefault();
    if (!name.value.trim()) {
      status.textContent = 'Give it a name.';
      status.classList.add('error');
      return;
    }
    submit.disabled = true;
    status.classList.remove('error');
    status.textContent = 'Saving\u2026';
    const body = {name: name.value.trim(), emoji: input.value.trim(), classrooms: f.classrooms, tags: f.tags};
    try {
      await act('calendar-feeds', f.id, 'edit', body);
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      return;
    } finally {
      submit.disabled = false;
    }
    shut();
    await load();
  });
  name.focus();
  name.select();
}

export async function makeDefaultFeed(f) {
  try {
    await act('when-settings', settingsId(), 'default', {token: f.token});
  } catch (err) {
    toast(err.message);
    return;
  }
  longToast(`${f.name} is your default calendar now.`);
  await load();
}

async function orderFeeds(tokens) {
  try {
    await act('when-settings', settingsId(), 'order-feeds', {tokens});
  } catch (err) {
    toast(err.message);
    return false;
  }
  await load();
  return true;
}

export function calendarMenu(onPick) {
  const menu = el('div', 'calendar-menu');
  const chosen = defaultFeed();
  const working = activeFeed();
  for (const f of allCalendars()) {
    const item = el('button', 'calendar-menu-item' + (working && working.token === f.token ? ' is-on' : ''));
    item.type = 'button';
    item.append(feedMark(f), el('span', 'calendar-menu-name', f.name));
    const tail = el('span', 'calendar-menu-tail');
    if (f.token === chosen.token) {
      const star = el('span', 'nav-sub-star');
      star.append(svg('pin'));
      tail.append(star);
    }
    if (f.locked) {
      const lock = el('span', 'calendar-menu-lock');
      lock.append(svg('lock'));
      tail.append(lock);
    }
    item.append(tail);
    item.addEventListener('click', () => {
      onPick();
      pickFeed(f);
    });
    menu.append(item);
  }
  return menu;
}

let editingNav = false;

const lockedWords = 'You can\u2019t modify this calendar, but you can create a new one based off of it.';

function fillNav(nav) {
  for (const item of primary) {
    if (item.href === '/mine') {
      mineRows(nav, item);
      continue;
    }
    const top = navLink(item);
    nav.append(top);
    if (item.href === '/') {
      calendarRows(nav, top);
    }
  }
}

function calendarRows(nav, top) {
  if ((state.model.feeds || []).length) {
    top.append(editPencil());
  }
  if (editingNav) {
    acceptDrops(nav);
  }
  const rail = {nav, all: allCalendars(), chosen: defaultFeed(), working: activeFeed(), dragging: null};
  for (const f of rail.all) {
    nav.append(feedRow(f, rail));
  }
}

function editPencil() {
  const pencil = button('', editingNav ? 'check' : 'edit', 'nav-edit' + (editingNav ? ' is-on' : ''), () => {
    editingNav = !editingNav;
    renderNav();
  });
  pencil.title = editingNav ? 'Done editing' : 'Edit your saved calendars';
  pencil.setAttribute('aria-label', pencil.title);
  return pencil;
}

function acceptDrops(nav) {
  if (nav.dataset.dropReady) {
    return;
  }
  nav.dataset.dropReady = '1';
  nav.addEventListener('dragover', e => {
    if (nav.querySelector('.is-dragging')) {
      e.preventDefault();
    }
  });
  nav.addEventListener('drop', e => e.preventDefault());
}

function feedRow(f, rail) {
  const row = el('div', 'nav-sub-row' + (editingNav ? ' is-draggable' : '') + (f.locked ? ' is-locked' : ''));
  row.dataset.token = f.token;
  if (!editingNav) {
    row.append(feedLink(f, rail));
    return row;
  }
  draggableRow(row, f, rail);
  const grip = el('span', 'nav-sub-grip');
  grip.title = 'Drag to reorder - the first is your default calendar';
  grip.append(svg('grip'));
  row.append(grip, feedLink(f, rail), feedTool('edit', 'nav-sub-tool', 'Edit ' + f.name, () => editFeedPopup(f)));
  if (f.locked) {
    const lock = el('span', 'nav-sub-tool nav-sub-lock');
    lock.title = lockedWords;
    lock.append(svg('lock'));
    row.append(lock);
    return row;
  }
  row.append(feedTool('close', 'nav-sub-tool nav-sub-remove', 'Remove ' + f.name, () => removeFeed(f)));
  return row;
}

function draggableRow(row, f, rail) {
  row.draggable = true;
  row.addEventListener('dragstart', e => {
    rail.dragging = row;
    row.classList.add('is-dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', f.token);
  });
  row.addEventListener('dragover', e => {
    if (!rail.dragging) {
      return;
    }
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (rail.dragging === row) {
      return;
    }
    const box = row.getBoundingClientRect();
    const after = e.clientY > box.top + box.height / 2;
    row.parentNode.insertBefore(rail.dragging, after ? row.nextSibling : row);
  });
  row.addEventListener('drop', e => e.preventDefault());
  row.addEventListener('dragend', async () => {
    row.classList.remove('is-dragging');
    rail.dragging = null;
    const order = [...rail.nav.querySelectorAll('.nav-sub-row')].map(r => r.dataset.token);
    if (order.join() !== rail.all.map(x => x.token).join()) {
      await orderFeeds(order);
    }
  });
}

function feedLink(f, rail) {
  const a = el('a', 'nav-sub' + (active('/') && showsFeed(f) ? ' is-active' : active('/') && rail.working && rail.working.token === f.token ? ' is-working' : ''));
  a.href = '/c/' + f.token;
  a.append(feedMark(f), el('span', '', f.name));
  a.title = f.locked ? 'The calendar\u2019s own view, for everyone' : 'Show the calendar as ' + f.name + ' sees it';
  const marks = el('span', 'nav-sub-marks');
  if (f.token === rail.chosen.token) {
    const star = el('span', 'nav-sub-star');
    star.title = 'Your default calendar';
    star.append(svg('pin'));
    marks.append(star);
  }
  if (f.locked && !editingNav) {
    const lock = el('span', 'nav-sub-locked');
    lock.title = lockedWords;
    lock.append(svg('lock'));
    marks.append(lock);
  }
  if (marks.childElementCount) {
    a.append(marks);
  }
  a.addEventListener('click', e => {
    e.preventDefault();
    pickFeed(f);
  });
  return a;
}

function feedTool(icon, className, title, onClick) {
  const tool = el('button', className);
  tool.type = 'button';
  tool.title = title;
  tool.setAttribute('aria-label', title);
  tool.append(svg(icon));
  tool.addEventListener('click', onClick);
  return tool;
}

async function removeFeed(f) {
  if (!confirm(`Remove ${f.name}? A calendar app subscribed to its feed stops updating.`)) {
    return;
  }
  try {
    await removeResource('calendar-feeds', f.id);
  } catch (err) {
    toast(err.message);
    return;
  }
  toast(`${f.name} removed`);
  await load();
}

function refresh() {
  carriedQuery = typed();
  render();
}

function pickFeed(f) {
  setActiveFeed(f.token);
  setClassrooms(feedClassrooms(f));
  setTags(feedTags(f));
  state.appliedCalendar = '';
  carriedQuery = typed();
  setPath('/c/' + f.token);
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

function action(label, on, onClick) {
  const b = el('button', 'filter-action' + (on ? ' is-on' : ''), label);
  b.type = 'button';
  b.addEventListener('click', onClick);
  return b;
}

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
    const c = chip(t.name, on.includes(t.id), () => {
      toggleTag(t.id);
      refresh();
    });
    c.title = t.description;
    const hidden = on.includes(t.id) ? 0 : hiddenMatches(t.id);
    if (hidden) {
      c.classList.add('has-hidden');
      c.title = `${hidden} match${hidden === 1 ? '' : 'es'} under ${t.name}, which is off`;
    }
    return c;
  });
}

function filterSummary() {
  const rooms = selectedClassrooms();
  const parts = [rooms.length === classroomNames().length ? 'All classrooms' : rooms.length ? classroomNames().filter(c => rooms.includes(c)).join(', ') : 'No classrooms'];
  const tags = selectedTags();
  for (const group of tagGroups()) {
    const chosen = group.tags.filter(t => tags.includes(t.id));
    if (!chosen.length) {
      continue;
    }
    const label = group.name || 'categories';
    parts.push(chosen.length === group.tags.length ? `All ${label}` : chosen.map(t => t.name).join(', '));
  }
  if (parts.length === 1) {
    parts.push('No categories');
  }
  const saved = savedView() && filtersAreDefault() ? ' · your saved view' : '';
  return parts.join(' · ') + saved;
}

let filtersUnfolded = false;

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
    chevron.append(svg('chevron-right'));
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
  const pick = list => {
    const ordered = tagIds().filter(t => list.includes(t));
    setTags(ordered.join() === defaultTags().join() ? null : ordered);
  };
  let last = null;
  for (const group of tagGroups()) {
    const ids = group.tags.map(t => t.id);
    const on = selectedTags();
    const onHere = ids.filter(id => on.includes(id));
    last = filterRow(group.name || 'Categories', [
      action('All', onHere.length === ids.length, () => {
        pick([...on, ...ids]);
        refresh();
      }),
      action('None', onHere.length === 0, () => {
        pick(on.filter(id => !ids.includes(id)));
        refresh();
      }),
    ], tagChips(group.tags));
    rows.append(last);
  }
  wrap.append(rows);
  const foot = el('div', 'filters-foot');
  const everything = selectedClassrooms().length === classroomNames().length && selectedTags().length === tagIds().length;
  if (!everything) {
    foot.append(button('Select all', null, 'button button-secondary button-small', () => {
      setClassrooms(classroomNames());
      setTags(tagIds());
      refresh();
    }));
  }
  if (selectedTags().length) {
    foot.append(button('Clear all', null, 'button button-secondary button-small', () => {
      pick([]);
      refresh();
    }));
  }
  const working = activeFeed();
  if (working ? !showsFeed(working) : !filtersAreDefault()) {
    foot.append(button('Reset filters', null, 'button button-secondary button-small', () => {
      if (working) {
        setClassrooms(feedClassrooms(working));
        setTags(feedTags(working));
      } else {
        resetFilters();
      }
      refresh();
    }));
  } else if (working && working.locked && savedView()) {
    foot.append(button('Forget my saved view', null, 'button button-secondary button-small', async () => {
      try {
        await act('when-settings', settingsId(), 'forget-view');
      } catch (err) {
        toast(err.message);
        return;
      }
      longToast('My Heliosian is back to the calendar\u2019s own defaults, here and on the Heliosian home page.');
      await load();
    }));
  }
  if (foot.childElementCount) {
    wrap.append(foot);
  }
}

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

function fillTabbar(bar) {
  for (const item of primary) {
    bar.append(navLink(item));
  }
  const filters = el('a', '');
  filters.href = '#';
  filters.append(svg('tag'), el('span', '', 'Filters'));
  filters.addEventListener('click', e => {
    e.preventDefault();
    filtersUnfolded = true;
    refresh();
    requestAnimationFrame(() => document.querySelector('.home-filters')?.scrollIntoView({block: 'start', behavior: 'smooth'}));
  });
  bar.append(filters);
}

let onSearch = null;
let carriedQuery = '';

const defaultPlaceholder = 'Search the year…';

function searchInput() {
  return document.querySelector('#search-input');
}

function typed() {
  return searchInput().value;
}

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

export function initChrome() {
  initShell({
    name: 'Helios When',
    me,
    fillNav,
    fillTabbar,
    search: {placeholder: defaultPlaceholder, results: true, own: true},
    afterRender: renderRailDay,
  });
  searchInput().addEventListener('input', () => search(typed()));
  searchInput().addEventListener('keydown', onSearchKey);
  searchInput().addEventListener('focus', () => showResults(typed().trim()));
  searchInput().addEventListener('blur', closeResults);
  onSlash(focusSearch);
}
