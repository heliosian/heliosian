import {state, me, today, bands, tagGroups, defaultTags, savedView, searchResults, eventPath, eventTint, timeLine, weekdayShort, parseDate, selectedClassrooms, toggleClassroom, setClassrooms, classroomNames, myClassrooms, tagNames, selectedTags, toggleTag, setTags, resetFilters, filtersAreDefault, colorOf, hiddenMatches, hiddenClassroomMatches, feedClassrooms, feedTags, showsFeed, setActiveFeed, activeFeed, allCalendars, defaultFeed} from './state.js';
import {el, svg, link, button, toast, feedMark, popup, emojiPicker} from './dom.js';
import {dayColumn} from './day.js';
import {renderAvatars, renderAlerts, renderProfileLink, onSlash, initAppSwitch, initUserMenu} from '/toolbar.js';

const primary = [
  {href: '/', icon: 'today', label: 'Calendar'},
  {href: '/feeds', icon: 'feed', label: 'Feeds'},
];

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return path === '/' || path.startsWith('/c/') || path.startsWith('/day/') || path.startsWith('/events/');
  }
  return path === href || path.startsWith(href + '/');
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(svg(item.icon), el('span', '', item.label));
  return a;
}

// editFeedPopup is the small popup for a saved calendar's name and emoji
// - from the pencil beside it in the rail, and from the mark over the
// calendar - saved with its filters as they stand.
export function editFeedPopup(f) {
  const form = el('form');
  const nameField = el('label', 'field');
  nameField.append(el('span', '', 'Name'));
  const name = el('input');
  name.type = 'text';
  name.maxLength = 80;
  name.value = f.name;
  nameField.append(name);
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
  // Delete, at the row's far end, takes the calendar and its feed away -
  // not My Heliosian's, which everyone keeps.
  const remove = f.locked ? el('span', 'modal-delete modal-locked', '\ud83d\udd12 Everyone keeps this one') : button('Delete calendar', 'trash', 'link-button danger modal-delete', async () => {
    if (!confirm(`Delete ${f.name}? A calendar app subscribed to its feed stops updating.`)) {
      return;
    }
    const res = await fetch('/api/calendar/feeds', {method: 'DELETE', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({token: f.token})});
    if (!res.ok) {
      toast(await res.text());
      return;
    }
    shut();
    toast(`${f.name} deleted`);
    const {load} = await import('./app.js');
    await load();
  });
  actions.append(submit, button('Cancel', '', 'button button-secondary', shut), status, remove);
  // Make default moves it to the head of the rail: the calendar the page
  // opens to, and Heliosian reads.
  if (defaultFeed().token !== f.token) {
    const first = button('Make default', 'star', 'button button-secondary modal-default', async () => {
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
    const body = {token: f.token, name: name.value.trim(), emoji: input.value.trim(), classrooms: f.classrooms, tags: f.tags};
    const res = await fetch('/api/calendar/feeds', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
    submit.disabled = false;
    if (!res.ok) {
      status.textContent = await res.text();
      status.classList.add('error');
      return;
    }
    shut();
    const {load} = await import('./app.js');
    await load();
  });
  name.focus();
  name.select();
}

// makeDefaultFeed makes one calendar - a saved one, or My Heliosian - the
// viewer's default, and reloads.
export async function makeDefaultFeed(f) {
  const res = await fetch('/api/calendar/default', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({token: f.token})});
  if (!res.ok) {
    toast(await res.text());
    return;
  }
  toast(`${f.name} is your default calendar now.`, 4000);
  const {load} = await import('./app.js');
  await load();
}

// orderFeeds puts the viewer's saved calendars in the order the tokens
// give and reloads; the first is their default calendar.
async function orderFeeds(tokens) {
  const res = await fetch('/api/calendar/feeds/order', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({tokens})});
  if (!res.ok) {
    toast(await res.text());
    return false;
  }
  const {load} = await import('./app.js');
  await load();
  return true;
}

// calendarMenu is a dropdown of the viewer's calendars - My Heliosian and
// the saved ones, the default starred - each opening the calendar as it
// sees it; the headline drops it, so a phone without the rail can switch.
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
      star.append(svg('star'));
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
      setActiveFeed(f.token);
      setClassrooms(feedClassrooms(f));
      setTags(feedTags(f));
      history.pushState(null, '', '/c/' + f.token);
      document.dispatchEvent(new CustomEvent('calendar:navigate'));
      refresh();
    });
    menu.append(item);
  }
  return menu;
}

// Whether the rail's saved calendars show their edit and remove buttons:
// the pencil on Calendar toggles it, and it lasts until toggled back.
let editingNav = false;

// fillNav lists the pages, and under Calendar the viewer's own saved
// calendars by name: each opens the calendar filtered to what it carries,
// and is lit while the filters are its own. The pencil on Calendar turns
// on a pencil and a cross on each of them - the pencil opens its form on
// the Feeds page, the cross removes it.
function fillNav(nav) {
  for (const item of primary) {
    const top = navLink(item);
    nav.append(top);
    if (item.href !== '/') {
      continue;
    }
    const feeds = state.model.feeds || [];
    if (feeds.length) {
      const pencil = el('button', 'nav-edit' + (editingNav ? ' is-on' : ''));
      pencil.type = 'button';
      pencil.title = editingNav ? 'Done editing' : 'Edit your saved calendars';
      pencil.setAttribute('aria-label', pencil.title);
      pencil.append(svg(editingNav ? 'check' : 'pencil'));
      pencil.addEventListener('click', e => {
        e.preventDefault();
        e.stopPropagation();
        editingNav = !editingNav;
        renderNav();
      });
      top.append(pencil);
    }
    let dragging = null;
    if (editingNav && !nav.dataset.dropReady) {
      // The gaps between rows and the nav's own ground take a drop too, so
      // letting go there is not a cancelled drag.
      nav.dataset.dropReady = '1';
      nav.addEventListener('dragover', e => {
        if (nav.querySelector('.is-dragging')) {
          e.preventDefault();
        }
      });
      nav.addEventListener('drop', e => e.preventDefault());
    }
    const chosen = defaultFeed();
    const working = activeFeed();
    const all = allCalendars();
    for (const f of all) {
      // My Heliosian drags and renames like the rest; only its filters are
      // locked and it cannot be removed.
      const row = el('div', 'nav-sub-row' + (editingNav ? ' is-draggable' : '') + (f.locked ? ' is-locked' : ''));
      row.dataset.token = f.token;
      if (editingNav) {
        // Rows drag into a new order; the drop saves it, the first being
        // the default calendar.
        row.draggable = true;
        row.addEventListener('dragstart', e => {
          dragging = row;
          row.classList.add('is-dragging');
          e.dataTransfer.effectAllowed = 'move';
          e.dataTransfer.setData('text/plain', f.token);
        });
        // The rows shuffle as the pointer passes over them; the drop is
        // allowed everywhere so the browser fires dragend either way, and
        // dragend saves whatever order the rows are in then.
        row.addEventListener('dragover', e => {
          if (!dragging) {
            return;
          }
          e.preventDefault();
          e.dataTransfer.dropEffect = 'move';
          if (dragging === row) {
            return;
          }
          const box = row.getBoundingClientRect();
          const after = e.clientY > box.top + box.height / 2;
          row.parentNode.insertBefore(dragging, after ? row.nextSibling : row);
        });
        row.addEventListener('drop', e => e.preventDefault());
        row.addEventListener('dragend', async () => {
          row.classList.remove('is-dragging');
          dragging = null;
          const order = [...nav.querySelectorAll('.nav-sub-row')].map(r => r.dataset.token);
          if (order.join() !== all.map(x => x.token).join()) {
            await orderFeeds(order);
          }
        });
      }
      // Lit while the filters are its own; softer while the viewer is
      // working from it with the filters moved.
      const a = el('a', 'nav-sub' + (active('/') && showsFeed(f) ? ' is-active' : active('/') && working && working.token === f.token ? ' is-working' : ''));
      a.href = '/c/' + f.token;
      a.append(feedMark(f), el('span', '', f.name));
      a.title = f.locked ? 'The calendar\u2019s own view, for everyone' : 'Show the calendar as ' + f.name + ' sees it';
      // The default calendar wears a star at its end, My Heliosian a
      // lock - always, so its standing shows wherever it sits; while the
      // rail is being edited the lock is the tool where its cross would be.
      const marks = el('span', 'nav-sub-marks');
      if (f.token === chosen.token) {
        const star = el('span', 'nav-sub-star');
        star.title = 'Your default calendar';
        star.append(svg('star'));
        marks.append(star);
      }
      if (f.locked && !editingNav) {
        const lock = el('span', 'nav-sub-locked');
        lock.title = 'Everyone keeps this one; its filters are the calendar\u2019s own';
        lock.append(svg('lock'));
        marks.append(lock);
      }
      if (marks.childElementCount) {
        a.append(marks);
      }
      a.addEventListener('click', e => {
        e.preventDefault();
        setActiveFeed(f.token);
        setClassrooms(feedClassrooms(f));
        setTags(feedTags(f));
        history.pushState(null, '', '/c/' + f.token);
        document.dispatchEvent(new CustomEvent('calendar:navigate'));
        refresh();
      });
      if (editingNav) {
        const grip = el('span', 'nav-sub-grip');
        grip.title = 'Drag to reorder - the first is your default calendar';
        grip.append(svg('grip'));
        row.append(grip);
      }
      row.append(a);
      if (editingNav) {
        const edit = el('button', 'nav-sub-tool');
        edit.type = 'button';
        edit.title = 'Edit ' + f.name;
        edit.setAttribute('aria-label', edit.title);
        edit.append(svg('pencil'));
        edit.addEventListener('click', () => editFeedPopup(f));
        row.append(edit);
        if (f.locked) {
          // A lock where the cross would be: it cannot be removed.
          const lock = el('span', 'nav-sub-tool nav-sub-lock');
          lock.title = 'Everyone keeps this one; its filters are the calendar\u2019s own';
          lock.append(svg('lock'));
          row.append(lock);
          nav.append(row);
          continue;
        }
        const remove = el('button', 'nav-sub-tool nav-sub-remove');
        remove.type = 'button';
        remove.title = 'Remove ' + f.name;
        remove.setAttribute('aria-label', remove.title);
        remove.append(svg('close'));
        remove.addEventListener('click', async () => {
          if (!confirm(`Remove ${f.name}? A calendar app subscribed to its feed stops updating.`)) {
            return;
          }
          const res = await fetch('/api/calendar/feeds', {method: 'DELETE', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({token: f.token})});
          if (!res.ok) {
            toast(await res.text());
            return;
          }
          toast(`${f.name} removed`);
          const {load} = await import('./app.js');
          await load();
        });
        row.append(remove);
      }
      nav.append(row);
    }
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

// filterSummary is the one line the folded filters show: the classrooms
// in view by name - "All classrooms" when every one is - then each
// category line the same way, "All School day" for a whole line, its
// chosen names otherwise, a line with none chosen left out. The line
// ellipsizes where the window is narrow.
function filterSummary() {
  const rooms = selectedClassrooms();
  const parts = [rooms.length === classroomNames().length ? 'All classrooms' : rooms.length ? classroomNames().filter(c => rooms.includes(c)).join(', ') : 'No classrooms'];
  const tags = selectedTags();
  for (const group of tagGroups()) {
    const chosen = group.tags.filter(t => tags.includes(t.name));
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
  // Reset filters goes back to the saved calendar being worked from, and
  // shows only while the filters have moved off it; with none, back to
  // the calendar's own defaults.
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
    // A view saved earlier, before My Heliosian was locked, still stands
    // in for the calendar's defaults until let go.
    foot.append(button('Forget my saved view', null, 'button button-secondary button-small', async () => {
      const res = await fetch('/api/calendar/settings', {method: 'DELETE'});
      if (!res.ok) {
        toast(await res.text());
        return;
      }
      toast('My Heliosian is back to the calendar\u2019s own defaults, here and on the Heliosian home page.', 5000);
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
  // Filters unfolds the page's own card and scrolls to it.
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
