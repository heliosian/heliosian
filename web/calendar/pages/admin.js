import {state, me, tagGroups, bands, classroomNames, myClassrooms, event, eventDates, eventPath, addDays, parseDate, dayLabel} from '../state.js';
import {el, link, svg, button, toast} from '../dom.js';
import {setTitle} from '../chrome.js';
import {eventForm} from '../eventform.js';
import {openSheet, imageControl} from '../images.js';

async function refreshModel() {
  const {load} = await import('../app.js');
  await load();
}

// The categories tool works on its own copy of the sheet's tags, grouped as
// the filters show them: every change - a description, a group's name, a
// category moved between or within groups, a group moved, a category or
// group added, a default switched - is on the page until Save writes them
// all at once. A group with nothing in it lives only on the page: the sheet
// knows a group by the tags that name it.
function working() {
  return tagGroups().map(g => ({name: g.name, open: true, tags: g.tags.filter(t => !t.builtIn).map(t => ({name: t.name, description: t.description, on: t.default, image: t.image || '', imageUrl: t.imageUrl || ''}))})).filter(g => g.tags.length);
}

function move(list, from, to) {
  if (to < 0 || to >= list.length || from === to) {
    return;
  }
  const [item] = list.splice(from, 1);
  list.splice(to, 0, item);
}

function arrow(icon, label, disabled, onClick) {
  const b = button('', icon, 'icon-button admin-arrow', onClick);
  b.setAttribute('aria-label', label);
  b.disabled = disabled;
  return b;
}

// The value the group picker's last option carries, standing for a group
// named on the spot.
const newGroupChoice = '+new';

function categoriesTool() {
  const wrap = el('div', 'card admin-tool');
  const groups = working();
  const board = el('div', 'admin-groups');
  const status = el('span', 'save-status');
  let dragging = null;

  const groupNames = () => groups.map(g => g.name).filter(Boolean);

  // placeGroup adds a group to the board: a named one ahead of the unnamed
  // group when there is one, the unnamed one last.
  const placeGroup = name => {
    const group = {name, open: true, tags: []};
    const loose = groups.findIndex(g => !g.name);
    groups.splice(name && loose >= 0 ? loose : groups.length, 0, group);
    return group;
  };

  const head = el('div', 'admin-tool-head');
  head.append(el('h2', '', 'Categories'));
  head.append(button('Add category group', 'plus', 'button', () => {
    const name = (prompt('Name for the new group') || '').trim();
    if (!name) {
      return;
    }
    if (groups.some(g => g.name === name)) {
      toast(`There is already a group called ${name}`);
      return;
    }
    placeGroup(name);
    paint();
  }));
  wrap.append(head);
  wrap.append(el('p', 'hint', 'The categories events are filed under, in the groups and the order the filters show them. A group with no name is the plain Categories line at the end. Rename a group here to rename it for every category in it; a category itself keeps its name, since every event carries it. A category that is off by default is one people see only when they switch it on, and Reset filters leaves it off. A category\u2019s picture is what an event under it wears across the top of its page when it has none of its own; the first of an event\u2019s categories with a picture wins.'));

  // groupPicker moves a category to another group, the unnamed one, or a
  // new one named on the spot.
  const groupPicker = (g, t) => {
    const select = el('select', 'admin-select');
    for (const name of [...groupNames(), '']) {
      const option = el('option', '', name || 'No group');
      option.value = name;
      option.selected = name === g.name;
      select.append(option);
    }
    const fresh = el('option', '', 'New group…');
    fresh.value = newGroupChoice;
    select.append(fresh);
    select.addEventListener('change', () => {
      let target = select.value;
      if (target === newGroupChoice) {
        target = (prompt('Name for the new group') || '').trim();
        if (!target) {
          paint();
          return;
        }
      }
      g.tags.splice(g.tags.indexOf(t), 1);
      const dest = groups.find(x => x.name === target) || placeGroup(target);
      dest.tags.push(t);
      dest.open = true;
      paint();
    });
    return select;
  };

  // Dragging: a row picked up by its handle drops before or after another
  // row, or onto a group's head to join that group at the end.
  const dropOn = (dest, index) => {
    if (!dragging) {
      return;
    }
    const from = dragging.group;
    const at = from.tags.indexOf(dragging.tag);
    from.tags.splice(at, 1);
    if (from === dest && at < index) {
      index -= 1;
    }
    dest.tags.splice(Math.min(index, dest.tags.length), 0, dragging.tag);
    dest.open = true;
    dragging = null;
    paint();
  };

  const clearOver = () => {
    for (const over of board.querySelectorAll('.is-over, .is-over-below')) {
      over.classList.remove('is-over', 'is-over-below');
    }
  };

  const groupHead = (g, gi) => {
    const head = el('div', 'admin-group-head');
    head.append(svg('tag'));
    const name = el('input', 'admin-group-name');
    name.type = 'text';
    name.maxLength = 40;
    name.placeholder = 'No group';
    name.value = g.name;
    name.readOnly = true;
    name.addEventListener('change', () => {
      g.name = name.value.trim();
      paint();
    });
    name.addEventListener('blur', () => {
      name.readOnly = true;
    });
    name.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        name.blur();
      }
    });
    head.append(name);
    head.append(el('span', 'admin-count', `${g.tags.length} categor${g.tags.length === 1 ? 'y' : 'ies'}`));
    head.append(button('Rename group', 'pencil', 'button button-secondary button-small admin-rename', () => {
      name.readOnly = false;
      name.focus();
      name.select();
    }));
    head.append(arrow('back', 'Move group up', gi === 0, () => {
      move(groups, gi, gi - 1);
      paint();
    }));
    head.append(arrow('chevron', 'Move group down', gi === groups.length - 1, () => {
      move(groups, gi, gi + 1);
      paint();
    }));
    const fold = button('', 'chevron', 'icon-button admin-fold', () => {
      g.open = !g.open;
      paint();
    });
    fold.setAttribute('aria-label', g.open ? 'Fold' : 'Unfold');
    head.append(fold);
    head.addEventListener('dragover', e => {
      if (dragging) {
        e.preventDefault();
        head.classList.add('is-over');
      }
    });
    head.addEventListener('dragleave', () => head.classList.remove('is-over'));
    head.addEventListener('drop', e => {
      e.preventDefault();
      dropOn(g, g.tags.length);
    });
    return head;
  };

  const tagRow = (g, t, ti) => {
    const row = el('div', 'admin-row');
    const handle = el('span', 'admin-handle');
    handle.append(svg('menu'));
    handle.title = 'Drag to move';
    handle.draggable = true;
    handle.addEventListener('dragstart', e => {
      dragging = {group: g, tag: t};
      row.classList.add('is-dragging');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', t.name);
    });
    handle.addEventListener('dragend', () => {
      dragging = null;
      row.classList.remove('is-dragging');
      clearOver();
    });
    row.addEventListener('dragover', e => {
      if (dragging && dragging.tag !== t) {
        e.preventDefault();
        const below = e.clientY > row.getBoundingClientRect().top + row.offsetHeight / 2;
        row.classList.toggle('is-over-below', below);
        row.classList.toggle('is-over', !below);
      }
    });
    row.addEventListener('dragleave', () => row.classList.remove('is-over', 'is-over-below'));
    row.addEventListener('drop', e => {
      e.preventDefault();
      const below = row.classList.contains('is-over-below');
      dropOn(g, ti + (below ? 1 : 0));
    });
    row.append(handle, imageControl(t, paint), el('span', 'admin-row-name', t.name));
    const description = el('input', 'admin-row-description');
    description.type = 'text';
    description.value = t.description;
    description.title = 'Click to edit the description';
    description.addEventListener('change', () => {
      t.description = description.value.trim();
    });
    row.append(description);
    const on = el('button', 'admin-default' + (t.on ? ' is-on' : ''), t.on ? 'On by default' : 'Off by default');
    on.type = 'button';
    on.title = 'Whether people see this category before choosing';
    on.addEventListener('click', () => {
      t.on = !t.on;
      paint();
    });
    row.append(on, groupPicker(g, t));
    return row;
  };

  const paint = () => {
    board.replaceChildren();
    groups.forEach((g, gi) => {
      const card = el('div', 'admin-group' + (g.open ? ' is-open' : ''));
      card.append(groupHead(g, gi));
      if (g.open) {
        const list = el('div', 'admin-list');
        if (!g.tags.length) {
          list.append(el('div', 'admin-empty', 'Nothing here yet - drag a category in, or add one below. An empty group is not kept.'));
        }
        g.tags.forEach((t, ti) => list.append(tagRow(g, t, ti)));
        card.append(list);
      }
      board.append(card);
    });
  };
  paint();
  wrap.append(board);

  // The built-in tags, for the record: they sit on their line and cannot
  // be moved, so they are named rather than listed for editing.
  const builtIn = state.model.tags.filter(t => t.builtIn);
  if (builtIn.length) {
    wrap.append(el('p', 'admin-note', `${builtIn.map(t => t.name).join(', ')} are built in and always sit under ${builtIn[0].group}.`));
  }

  // Adding a category: its name, its description for the classifier, and
  // the group it starts in. It joins the list here; Save writes it.
  const add = el('form', 'feed-form admin-add');
  add.append(el('h3', 'section-title', 'Add a category'));
  const nameField = el('label', 'field');
  nameField.append(el('span', '', 'Name'));
  const newName = el('input');
  newName.type = 'text';
  newName.maxLength = 40;
  newName.required = true;
  nameField.append(newName);
  const descField = el('label', 'field');
  descField.append(el('span', '', 'Description'));
  const newDesc = el('input');
  newDesc.type = 'text';
  newDesc.required = true;
  descField.append(newDesc, el('small', '', 'What the category means - the definition the classifier files events by.'));
  const groupField = el('label', 'field');
  groupField.append(el('span', '', 'Group'));
  const newGroup = el('input');
  newGroup.type = 'text';
  newGroup.maxLength = 40;
  newGroup.placeholder = 'No group';
  newGroup.setAttribute('list', 'tag-groups');
  const known = el('datalist');
  known.id = 'tag-groups';
  groupField.append(newGroup, known);
  add.append(nameField, descField, groupField);
  const addActions = el('div', 'modal-actions');
  const addButton = el('button', 'button button-secondary');
  addButton.type = 'submit';
  addButton.append(svg('plus'), el('span', '', 'Add to the list'));
  addActions.append(addButton);
  add.append(addActions);
  add.addEventListener('focusin', () => {
    known.replaceChildren();
    for (const name of groupNames()) {
      const option = el('option');
      option.value = name;
      known.append(option);
    }
  });
  add.addEventListener('submit', e => {
    e.preventDefault();
    const name = newName.value.trim();
    if (groups.some(g => g.tags.some(t => t.name === name)) || state.model.tags.some(t => t.name === name)) {
      toast(`There is already a category called ${name}`);
      return;
    }
    const target = newGroup.value.trim();
    const dest = groups.find(g => g.name === target) || placeGroup(target);
    dest.tags.push({name, description: newDesc.value.trim(), on: true, image: '', imageUrl: ''});
    dest.open = true;
    add.reset();
    paint();
    toast(`${name} added - Save categories to keep it`);
  });
  wrap.append(add);

  const actions = el('div', 'modal-actions admin-save');
  const save = button('Save categories', 'check', 'button', async () => {
    const tags = [];
    for (const g of groups) {
      for (const t of g.tags) {
        tags.push({name: t.name, description: t.description, group: g.name, default: t.on, image: t.image});
      }
    }
    save.disabled = true;
    status.classList.remove('error');
    status.textContent = 'Saving…';
    const res = await fetch('/api/calendar/tags', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({tags})});
    save.disabled = false;
    if (!res.ok) {
      status.textContent = await res.text();
      status.classList.add('error');
      return;
    }
    status.textContent = '';
    toast('Categories saved');
    await refreshModel();
  });
  actions.append(save, status);
  wrap.append(actions);
  return wrap;
}

// addEventTool is the form that puts an event in the Events tab: what it
// is, when, where, who it is for and what kind of thing it is, the day type
// it imposes, its search words - and, to repeat it, how many weeks apart
// and how many more times. Opened from an event's page to clone it, the
// form starts filled from that event with its dates four weeks on.
// openAddEvent is the add-event form in a sheet over the page, filled from
// an event to clone when there is one; done, the list is drawn again.
function openAddEvent(from, shift, repaint) {
  let shut = null;
  const form = eventForm({from, shift, onDone: async () => {
    await refreshModel();
    shut();
    repaint();
  }});
  shut = openSheet(from ? 'Add a copy of ' + from.title : 'Add an event', form);
}

// whenPicker is a date and a time for one end of an event, blank time for
// all day, reading back as the sheet writes a moment.
function whenPicker(value) {
  const wrap = el('span', 'admin-when-pick');
  const date = el('input');
  date.type = 'date';
  date.value = (value || '').slice(0, 10);
  const time = el('input');
  time.type = 'time';
  time.value = value && value.length > 10 ? value.slice(11, 16) : '';
  wrap.append(date, time);
  return {node: wrap, inputs: [date, time], value: () => (date.value ? date.value + (time.value ? ' ' + time.value : '') : '')};
}

// eventsTool is the Events tab: the hand-added events as a list, each with
// its start and end as pickers that save on change, and Add Event, which
// opens the form. Opened to clone an event, the form is already up.
function eventsTool() {
  const card = el('div', 'card');
  const head = el('div', 'admin-tool-head');
  head.append(el('h2', '', 'Events'));
  const list = el('div', 'admin-events');
  // The filters over the list: words in the title, a category, a
  // classroom, a span of dates, and whether past events show.
  const filters = el('div', 'admin-filters');
  const search = el('input', 'admin-filter-search');
  search.type = 'search';
  search.placeholder = 'Search titles…';
  const category = el('select', 'admin-select');
  const classroom = el('select', 'admin-select');
  const fromDate = el('input');
  fromDate.type = 'date';
  const toDate = el('input');
  toDate.type = 'date';
  const past = el('label', 'admin-filter-past');
  const pastBox = el('input');
  pastBox.type = 'checkbox';
  past.append(pastBox, el('span', '', 'Show past'));
  const count = el('span', 'admin-filter-count');
  filters.append(search, category, classroom, el('span', 'admin-event-label', 'From'), fromDate, el('span', 'admin-event-label', 'To'), toDate, past, count);
  const fillSelects = () => {
    const mine = state.model.events.filter(e => e.source === 'sheet');
    const cats = [...new Set(mine.flatMap(e => e.tags.filter(t => !classroomNames().includes(t))))].sort();
    const chosenCat = category.value;
    category.replaceChildren();
    for (const name of ['', ...cats]) {
      const option = el('option', '', name || 'Any category');
      option.value = name;
      category.append(option);
    }
    category.value = cats.includes(chosenCat) ? chosenCat : '';
    const chosenRoom = classroom.value;
    classroom.replaceChildren();
    for (const name of ['', ...classroomNames()]) {
      const option = el('option', '', name || 'Any classroom');
      option.value = name;
      classroom.append(option);
    }
    classroom.value = classroomNames().includes(chosenRoom) ? chosenRoom : '';
  };
  const admits = e => {
    const words = search.value.trim().toLowerCase();
    if (words && !e.title.toLowerCase().includes(words)) {
      return false;
    }
    if (category.value && !e.tags.includes(category.value)) {
      return false;
    }
    if (classroom.value && !e.classrooms.includes(classroom.value)) {
      return false;
    }
    if (fromDate.value && e.end.slice(0, 10) < fromDate.value) {
      return false;
    }
    if (toDate.value && e.start.slice(0, 10) > toDate.value) {
      return false;
    }
    if (!pastBox.checked && !fromDate.value && e.end.slice(0, 10) < state.model.today) {
      return false;
    }
    return true;
  };
  const decide = async (e, path, done) => {
    const res = await fetch('/api/calendar/events/' + path, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: e.id})});
    if (!res.ok) {
      toast(await res.text());
      return;
    }
    toast(done);
    await refreshModel();
    paint();
  };
  const paint = () => {
    list.replaceChildren();
    fillSelects();
    // The events people shared wait at the top, each with Approve and
    // Decline, until an admin decides.
    const waiting = state.model.events.filter(e => e.pending || e.declined).sort((a, b) => (a.declined ? 1 : 0) - (b.declined ? 1 : 0) || a.start.localeCompare(b.start));
    if (waiting.length) {
      const pending = waiting.filter(e => e.pending).length;
      list.append(el('div', 'admin-pending-head', pending ? `Waiting for approval \u00b7 ${pending}` : 'Declined'));
      for (const e of waiting) {
        const row = el('div', 'admin-row admin-event' + (e.declined ? ' is-declined' : ' is-pending'));
        const words = el('div', 'admin-row-words');
        const title = link(eventPath(e), 'admin-row-name');
        title.textContent = e.title;
        const who = (state.model.names || {})[e.addedBy] || e.addedBy;
        words.append(title, el('div', 'admin-row-note', [dayLabel(e.start.slice(0, 10)) + (e.allDay ? '' : ' ' + e.start.slice(11)), e.location, 'shared by ' + who].filter(Boolean).join(' \u00b7 ')));
        const actions = el('div', 'admin-pending-actions');
        actions.append(button('Approve', 'check', 'button button-small', () => decide(e, 'approve', 'On the calendar')));
        if (e.declined) {
          actions.append(el('span', 'admin-declined-note', 'Declined'));
        } else {
          actions.append(button('Decline', 'close', 'button button-secondary button-small', () => {
            if (confirm(`Decline ${e.title}? It comes off the calendar; you can approve it later.`)) {
              decide(e, 'decline', 'Declined');
            }
          }));
        }
        row.append(words, actions);
        list.append(row);
      }
    }
    const all = state.model.events.filter(e => e.source === 'sheet' && !e.pending && !e.declined).sort((a, b) => a.start.localeCompare(b.start));
    if (!all.length) {
      list.append(el('div', 'admin-empty', waiting.length ? 'Nothing on the calendar by hand yet.' : 'No events added by hand yet - everything on the calendar came from the school or the other apps.'));
      count.textContent = '';
      return;
    }
    const mine = all.filter(admits);
    count.textContent = mine.length === all.length ? `${all.length} events` : `${mine.length} of ${all.length}`;
    if (!mine.length) {
      list.append(el('div', 'admin-empty', 'Nothing matches these filters.'));
      return;
    }
    const today = state.model.today;
    for (const e of mine) {
      const row = el('div', 'admin-row admin-event' + (e.end.slice(0, 10) < today ? ' is-past' : ''));
      const words = el('div', 'admin-row-words');
      const title = link(eventPath(e), 'admin-row-name');
      title.textContent = e.title;
      words.append(title, el('div', 'admin-row-note', [e.location, e.tags.filter(t => !classroomNames().includes(t)).join(', ')].filter(Boolean).join(' · ')));
      const start = whenPicker(e.start);
      const end = whenPicker(e.end);
      const save = button('Save', 'check', 'button button-small admin-event-save', async () => {
        const res = await fetch('/api/calendar/events/when', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: e.id, start: start.value(), end: end.value()})});
        if (!res.ok) {
          toast(await res.text());
          return;
        }
        toast('Moved');
        await refreshModel();
        paint();
      });
      save.hidden = true;
      const changed = () => {
        save.hidden = start.value() === e.start && end.value() === e.end;
      };
      for (const input of [...start.inputs, ...end.inputs]) {
        input.addEventListener('input', changed);
      }
      const when = el('div', 'admin-event-when');
      when.append(el('span', 'admin-event-label', 'Starts'), start.node, el('span', 'admin-event-label', 'Ends'), end.node, save);
      const clone = button('', 'copyplus', 'icon-button admin-event-clone', () => openAddEvent(e, 4, paint));
      clone.title = 'Add a copy 4 weeks on';
      row.append(words, when, clone);
      list.append(row);
    }
  };
  head.append(button('Add Event', 'plus', 'button', () => openAddEvent(null, 4, paint)));
  card.append(head, el('p', 'hint', 'The events added by hand, in the Events tab. Change when one is right here; the school\u2019s own events are corrected in the sheet\u2019s Overrides tab.'), filters, list);
  for (const input of [search, category, classroom, fromDate, toDate, pastBox]) {
    input.addEventListener('input', paint);
    input.addEventListener('change', paint);
  }
  paint();
  const params = new URLSearchParams(location.search);
  if (params.get('clone') && event(params.get('clone'))) {
    setTimeout(() => openAddEvent(event(params.get('clone')), Number(params.get('weeks') || 4), paint), 0);
  }
  return card;
}

// The tools, grouped as the rail lists them.
const sections = [
  {title: 'Calendar', tabs: [
    {key: 'events', label: 'Events', card: eventsTool},
    {key: 'categories', label: 'Categories', card: categoriesTool},
  ]},
];

// adminPage is Admin Tools as the other apps have it: its own window over
// the shell - a teal header with the tile, Admin, the signed-in address and
// a close button - a rail of tabs, and a card per tool.
export function adminPage() {
  setTitle('Admin Tools');
  const page = el('div', 'admin admin-strip');
  const header = el('header');
  const brand = el('a', 'brand-link');
  brand.href = '/';
  brand.setAttribute('data-link', '');
  const mark = el('img', 'admin-tile');
  mark.src = '/brand/icon-192.png';
  mark.alt = 'Helios When';
  brand.append(mark, el('span', '', 'Admin'));
  const right = el('span', 'right');
  right.append(el('span', 'email', me().email));
  const close = el('a', 'admin-close');
  close.href = '/';
  close.setAttribute('data-link', '');
  close.setAttribute('aria-label', 'Close admin tools');
  close.append(svg('close'));
  right.append(close);
  header.append(brand, right);

  const layout = el('div', 'layout');
  const rail = el('nav', 'sidebar');
  const container = el('div', 'container');
  const panels = {};
  const tabList = [];
  const show = key => {
    for (const tab of tabList) {
      tab.classList.toggle('active', tab.dataset.panel === key);
    }
    for (const [k, panel] of Object.entries(panels)) {
      panel.hidden = k !== key;
    }
    state.adminTab = key;
  };
  for (const section of sections) {
    const group = el('div', 'sidebar-section');
    group.append(el('div', 'sidebar-section-title', section.title));
    for (const item of section.tabs) {
      // Appearance - what colours the app - is the platform's super admins'
      // alone; a regular admin's page is built without it.
      if (item.superOnly && !me().isSuperAdmin) {
        continue;
      }
      const tab = el('div', 'tab', item.label);
      tab.dataset.panel = item.key;
      tab.addEventListener('click', () => show(item.key));
      tabList.push(tab);
      group.append(tab);
      const panel = el('div', 'panel');
      panel.append(item.card());
      panels[item.key] = panel;
      container.append(panel);
    }
    rail.append(group);
  }
  const wanted = new URLSearchParams(location.search).get('tab') || (new URLSearchParams(location.search).get('clone') ? 'events' : state.adminTab);
  show(wanted && panels[wanted] ? wanted : sections[0].tabs[0].key);
  layout.append(rail, container);
  page.append(header, layout);
  return page;
}
