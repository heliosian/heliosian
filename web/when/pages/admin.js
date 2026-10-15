import {state, me, classroomNames, categoryTags, tagName, event, eventPath, dayLabel} from '../state.js';
import {el, link, svg, button, toast} from '/elements.js';
import {popup} from '/modal.js';
import {saveCategories, setPicture, setStatus, moveEvent} from '../writes.js';
import {adminPage as buildAdminPage} from '/admin.js';
import {eventForm} from '../eventform.js';
import {imageControl} from '../imagecontrol.js';
import {load} from '/router.js';

function working() {
  return state.model.tags.filter(t => !t.builtIn).map(t => ({id: t.id, name: t.name, description: t.description, on: t.default, image: t.image || '', imageUrl: t.imageUrl || ''}));
}

function categoriesTool() {
  const wrap = el('div', 'card admin-tool');
  const tags = working();
  const board = el('div', 'admin-list');
  const status = el('span', 'save-status');
  let dragging = null;

  const head = el('div', 'admin-tool-head');
  head.append(el('h2', '', 'Categories'));
  wrap.append(head);
  wrap.append(el('p', 'hint', 'The categories events are filed under, in the order the filters show them; drag one to move it. A category keeps its name, since every event carries it. A category that is off by default is one people see only when they switch it on, and Reset filters leaves it off. A category\u2019s picture is what an event under it wears across the top of its page when it has none of its own; the first of an event\u2019s categories with a picture wins.'));

  const dropOn = index => {
    if (!dragging) {
      return;
    }
    const at = tags.indexOf(dragging);
    tags.splice(at, 1);
    if (at < index) {
      index -= 1;
    }
    tags.splice(Math.min(index, tags.length), 0, dragging);
    dragging = null;
    paint();
  };

  const clearOver = () => {
    for (const over of board.querySelectorAll('.is-over, .is-over-below')) {
      over.classList.remove('is-over', 'is-over-below');
    }
  };

  const tagRow = (t, ti) => {
    const row = el('div', 'admin-row');
    const handle = el('span', 'admin-handle');
    handle.append(svg('menu'));
    handle.title = 'Drag to move';
    handle.draggable = true;
    handle.addEventListener('dragstart', e => {
      dragging = t;
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
      if (dragging && dragging !== t) {
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
      dropOn(ti + (below ? 1 : 0));
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
    row.append(on);
    return row;
  };

  const paint = () => {
    board.replaceChildren(...tags.map(tagRow));
  };
  paint();
  wrap.append(board);

  const builtIn = state.model.tags.filter(t => t.builtIn);
  if (builtIn.length) {
    wrap.append(el('p', 'admin-note', `${builtIn.map(t => t.name).join(' and ')} are built in: each person's own, from their household's answers, tickets and sign-ups.`));
  }

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
  add.append(nameField, descField);
  const addActions = el('div', 'modal-actions');
  const addButton = el('button', 'button button-secondary');
  addButton.type = 'submit';
  addButton.append(svg('plus'), el('span', '', 'Add to the list'));
  addActions.append(addButton);
  add.append(addActions);
  add.addEventListener('submit', e => {
    e.preventDefault();
    const name = newName.value.trim();
    if (tags.some(t => t.name === name) || state.model.tags.some(t => t.name === name)) {
      toast(`There is already a category called ${name}`);
      return;
    }
    tags.push({id: '', name, description: newDesc.value.trim(), on: true, image: '', imageUrl: ''});
    add.reset();
    paint();
    toast(`${name} added - Save categories to keep it`);
  });
  wrap.append(add);

  const actions = el('div', 'modal-actions admin-save');
  const save = button('Save categories', 'check', 'button', async () => {
    const saving = tags.map(t => ({id: t.id, name: t.name, slug: t.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''), description: t.description, default: t.on, image: t.image}));
    save.disabled = true;
    status.classList.remove('error');
    status.textContent = 'Saving…';
    try {
      await saveCategories(saving);
      for (const t of saving) {
        const old = state.model.tags.find(x => x.id === t.id);
        if (old && t.image !== old.image) {
          await setPicture({id: t.id, photo: old.photo}, t.image);
        }
      }
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      return;
    } finally {
      save.disabled = false;
    }
    status.textContent = '';
    toast('Categories saved');
    await load();
  });
  actions.append(save, status);
  wrap.append(actions);
  return wrap;
}

function openAddEvent(from, shift, repaint) {
  let shut = null;
  const form = eventForm({from, shift, onDone: async () => {
    await load();
    shut();
    repaint();
  }});
  shut = popup(from ? 'Add a copy of ' + from.title : 'Add an event', form, {wide: true}).shut;
}

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

function eventsTool() {
  const card = el('div', 'card');
  const head = el('div', 'admin-tool-head');
  head.append(el('h2', '', 'Events'));
  const list = el('div', 'admin-events');
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
    const cats = [...new Set(mine.flatMap(categoryTags))].sort((a, b) => tagName(a).localeCompare(tagName(b)));
    const chosenCat = category.value;
    category.replaceChildren();
    for (const id of ['', ...cats]) {
      const option = el('option', '', id ? tagName(id) : 'Any category');
      option.value = id;
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
  const decide = async (e, action, done) => {
    try {
      await setStatus(e, action === 'approve' ? 'open' : 'closed');
    } catch (err) {
      toast(err.message);
      return;
    }
    toast(done);
    await load();
    paint();
  };
  const paint = () => {
    list.replaceChildren();
    fillSelects();
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
      words.append(title, el('div', 'admin-row-note', [e.location, categoryTags(e).map(tagName).join(', ')].filter(Boolean).join(' · ')));
      const start = whenPicker(e.start);
      const end = whenPicker(e.end);
      const save = button('Save', 'check', 'button button-small admin-event-save', async () => {
        try {
          await moveEvent(e, start.value(), end.value());
        } catch (err) {
          toast(err.message);
          return;
        }
        toast('Moved');
        await load();
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

const sections = [
  {title: 'Calendar', tabs: [
    {key: 'events', label: 'Events', card: eventsTool},
    {key: 'categories', label: 'Categories', card: categoriesTool},
  ]},
];

export function adminPage() {
  return buildAdminPage({appName: 'Helios When', allowed: state.model.admin, email: me().email, sections});
}
