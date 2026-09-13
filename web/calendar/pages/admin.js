import {state, tagGroups} from '../state.js';
import {el, svg, button, toast} from '../dom.js';
import {setTitle} from '../chrome.js';

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
  return tagGroups().map(g => ({name: g.name, open: true, tags: g.tags.filter(t => !t.builtIn).map(t => ({name: t.name, description: t.description, on: t.default}))})).filter(g => g.tags.length);
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
  const wrap = el('section', 'admin-tool');
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
  head.append(el('h2', 'admin-tool-title', 'Categories'));
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
  wrap.append(el('p', 'page-intro', 'The categories events are filed under, in the groups and the order the filters show them. A group with no name is the plain Categories line at the end. Rename a group here to rename it for every category in it; a category itself keeps its name, since every event carries it. A category that is off by default is one people see only when they switch it on, and Reset filters leaves it off.'));

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
    row.append(handle, el('span', 'admin-row-name', t.name));
    const description = el('input', 'admin-row-description');
    description.type = 'text';
    description.value = t.description;
    description.title = 'Click to edit the description';
    description.addEventListener('change', () => {
      t.description = description.value.trim();
    });
    row.append(description);
    const on = el('label', 'admin-default');
    const box = el('input');
    box.type = 'checkbox';
    box.checked = t.on;
    box.addEventListener('change', () => {
      t.on = box.checked;
    });
    on.append(box, el('span', '', 'On by default'));
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
    dest.tags.push({name, description: newDesc.value.trim(), on: true});
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
        tags.push({name: t.name, description: t.description, group: g.name, default: t.on});
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

export function adminPage() {
  setTitle('Admin Tools');
  const page = el('div');
  const head = el('div', 'page-head admin-head');
  const mark = el('span', 'admin-mark');
  mark.append(svg('calendar'));
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Admin Tools'));
  main.append(el('p', 'page-intro', 'What the calendar admins can change from here. Everything else is edited in the sheet.'));
  head.append(mark, main);
  page.append(head, categoriesTool());
  return page;
}
