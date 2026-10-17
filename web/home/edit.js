import {state, linkCategories, holdsApps, inGrid, widgetGrid, widgetKey, categoryOf, fraction} from './state.js';
import {categoryIcons, categoryMark, categoryMarkOf} from './dom.js';
import {el, svg, toast} from '/elements.js';
import {load} from '/router.js';
import {imageTools} from '/images.js';
import {createPersonPicker} from '/picker.js';
import {listed} from '/directory.js';
import {appOrigin} from '/appswitch.js';
import {api} from '/api.js';
import {rulesCard, ruleOf, ruleSaysSomething, nameRulesWith} from '/listrules.js';
import {tabStrip} from '/tabs.js';
import {movedKey} from '/order.js';
import {widgetRows} from './widgets.js';
import {shownTo} from './cards.js';
import * as writes from './writes.js';

const linkModal = document.querySelector('#link-modal');
const linkForm = document.querySelector('#link-form');
const categoryModal = document.querySelector('#category-modal');
const appModal = document.querySelector('#app-modal');
const widgetModal = document.querySelector('#widget-modal');
const appForm = document.querySelector('#app-form');
const categoryForm = document.querySelector('#category-form');
const {imagePicker} = imageTools('/api/home', {state});

let editingLink = null;
let editingCategory = null;
let linkImage = null;
let categoryImage = null;

nameRulesWith({
  group: id => state.model.audiences[id] || null,
  person: id => (state.model.people[id] || {}).name_show || 'someone',
});

function setStatus(selector, message, error) {
  const status = document.querySelector(selector);
  status.textContent = message;
  status.classList.toggle('error', Boolean(error));
}

function fillCategories(selected) {
  const select = document.querySelector('#link-category');
  select.replaceChildren();
  for (const category of linkCategories().filter(c => !holdsApps(c))) {
    const option = el('option', '', category.title);
    option.value = category.id;
    option.selected = category.id === selected;
    select.append(option);
  }
}

function audienceCard(mount, thing, everyoneNote, people = () => []) {
  const draft = (thing ? thing.rules || [] : []).map(ruleOf);
  const counts = new Map();
  let previewTimer = null;
  mount.replaceChildren();
  const note = el('small', 'audience-note', everyoneNote);
  const preview = el('div', 'audience-preview');
  const askPreview = () => {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(async () => {
      const said = draft.filter(ruleSaysSomething);
      const named = people();
      if (!said.length && !named.length) {
        counts.clear();
        preview.textContent = '';
        card.render();
        return;
      }
      try {
        const answer = await api('POST', '/api/do/draft-members', {
          rules: said.map(r => ({exclude: r.exclude, target: r.target, replace_with: r.replace_with, within: r.within, person: r.person, search: r.search})),
          members: named.map(person => ({person, member: 'yes'})),
        });
        counts.clear();
        said.forEach((r, i) => counts.set(r, answer.ruleCounts[i]));
        card.render();
        const names = answer.members.map(m => m.name).filter(Boolean);
        const n = answer.members.length;
        preview.textContent = n ? `Picks out ${n} ${n === 1 ? 'person' : 'people'}: ${names.slice(0, 12).join(', ')}${n > 12 ? '…' : ''}` : 'Picks out nobody yet.';
      } catch (err) {
        preview.textContent = err.message;
      }
    }, 300);
  };
  const card = rulesCard({
    list: () => draft,
    onAdd: rule => draft.push(rule),
    onChange: askPreview,
    onRemove: rule => {
      draft.splice(draft.indexOf(rule), 1);
      card.render();
      askPreview();
    },
    count: rule => counts.get(rule),
    empty: 'No rules yet.',
    hint: 'Each rule takes in a group\'s members, or their parents, children or households, and can keep to one role. A Leave out rule keeps whoever it picks off.',
  });
  mount.append(note, card.card, preview);
  askPreview();
  return {
    get rules() {
      return draft.filter(ruleSaysSomething);
    },
    refresh: askPreview,
  };
}

let linkAudience = null;
let categoryAudience = null;
let appAudience = null;
let widgetAudience = null;
let editingWidget = null;

function openWidgetAudience(widget, title) {
  editingWidget = {widget, title};
  document.querySelector('#widget-modal-title').textContent = title;
  document.querySelector('#widget-sidebar').checked = widget.sidebar;
  widgetAudience = audienceCard(document.querySelector('#widget-audience'), widget, 'No rules means everyone.');
  setStatus('#widget-status', '');
  widgetModal.hidden = false;
}

async function saved(selector, work) {
  setStatus(selector, 'Saving…');
  try {
    await work();
    closeModals();
    await load();
  } catch (err) {
    setStatus(selector, err.message, true);
  }
}

function saveWidgetAudience(e) {
  e.preventDefault();
  const {widget, title} = editingWidget;
  saved('#widget-status', () => writes.saveWidget(widget, title, {rules: widgetAudience.rules, sidebar: document.querySelector('#widget-sidebar').checked}));
}

function showTab(prefix, key) {
  const strip = tabStrip([{key: 'details', label: 'Details'}, {key: 'visibility', label: 'Visibility'}], key, 2, k => showTab(prefix, k));
  document.querySelector(`#${prefix}-tabs`).replaceChildren(strip);
  document.querySelector(`#${prefix}-tab-details`).hidden = key !== 'details';
  document.querySelector(`#${prefix}-tab-visibility`).hidden = key !== 'visibility';
}

let linkImageChanged = false;

function openLinkEditor(link, category) {
  editingLink = link;
  linkImageChanged = false;
  linkImage = imagePicker('', link && link.imageUrl ? link.imageUrl : '', {
    dropzone: true,
    hint: 'Optional - without one the category’s emoji shows.',
    query: () => document.querySelector('#link-title').value.trim(),
    onChange: () => {
      linkImageChanged = true;
    },
  });
  document.querySelector('#link-image').replaceChildren(linkImage.wrap);
  document.querySelector('#link-modal-title').textContent = link ? 'Edit Link' : 'Add Link';
  document.querySelector('#link-title').value = link ? link.title : '';
  document.querySelector('#link-description').value = link ? link.description || '' : '';
  document.querySelector('#link-url').value = link ? link.url : '';
  document.querySelector('#link-visible').checked = link ? link.visible : true;
  linkAudience = audienceCard(document.querySelector('#link-audience'), link, 'No rules means everyone.');
  document.querySelector('#link-delete').hidden = !link;
  fillCategories(link ? link.category : category);
  setStatus('#link-status', '');
  showTab('link', 'details');
  linkModal.hidden = false;
  document.querySelector('#link-title').focus();
}

function openCategoryEditor(category) {
  editingCategory = category;
  document.querySelector('#category-modal-title').textContent = category ? 'Edit Category' : 'Add Category';
  document.querySelector('#category-title').value = category ? category.title : '';
  document.querySelector('#category-style').value = category && inGrid(category) ? 'grid' : 'list';
  document.querySelector('#category-apps-note').hidden = !category || !holdsApps(category);
  document.querySelector('#category-delete').hidden = !category || holdsApps(category);
  document.querySelector('#category-descriptions').checked = category ? category.descriptions : true;
  const widget = category && widgetOf(category);
  document.querySelector('#category-sidebar').checked = Boolean(widget && widget.sidebar);
  categoryAudience = audienceCard(document.querySelector('#category-audience'), widget, 'No rules means everyone; the whole section, links and all.');
  const emoji = category ? category.emoji || '' : '';
  categoryImage = imagePicker(emoji.startsWith('image:') ? emoji.slice('image:'.length) : '', category && category.imageUrl ? category.imageUrl : '', {
    plain: true,
    label: 'Your own icon',
    hint: 'A square picture with a transparent background looks best.',
    onChange: name => {
      showCustom(name ? '/' + name : '');
      setEmoji(name ? 'image:' + name : '');
    },
  });
  const upload = document.querySelector('#category-image');
  upload.replaceChildren(categoryImage.wrap);
  upload.hidden = true;
  showCustom(category && category.imageUrl ? category.imageUrl : '');
  setEmoji(emoji);
  setStatus('#category-status', '');
  showTab('category', 'details');
  categoryModal.hidden = false;
  document.querySelector('#category-title').focus();
}

function setEmoji(value) {
  const input = document.querySelector('#category-emoji');
  input.value = value;
  document.querySelector('#category-emoji-clear').hidden = !value;
  for (const button of document.querySelectorAll('#category-emoji-grid button')) {
    button.classList.toggle('is-picked', button === customTile ? value.startsWith('image:') : button.dataset.icon === value);
  }
}

let customTile = null;

function showCustom(url) {
  const words = url ? 'Custom' : 'Upload';
  let mark = svg('image');
  if (url) {
    mark = el('img', 'icon-image');
    mark.src = url;
    mark.alt = '';
  }
  customTile.replaceChildren(mark, el('span', '', words));
  customTile.title = words;
  customTile.setAttribute('aria-label', words);
}

function wireEmojiPicker() {
  const grid = document.querySelector('#category-emoji-grid');
  const upload = document.querySelector('#category-image');
  for (const [name, words] of categoryIcons) {
    const button = el('button', 'icon-choice');
    button.type = 'button';
    button.dataset.icon = 'icon:' + name;
    button.title = words;
    button.setAttribute('aria-label', words);
    button.append(categoryMark(name), el('span', '', words));
    button.addEventListener('click', () => {
      upload.hidden = true;
      setEmoji('icon:' + name);
    });
    grid.append(button);
  }
  customTile = el('button', 'icon-choice icon-choice-custom');
  customTile.type = 'button';
  customTile.addEventListener('click', () => {
    if (!categoryImage.value() && !(editingCategory && editingCategory.photo)) {
      categoryImage.wrap.querySelector('input[type=file]').click();
      return;
    }
    upload.hidden = false;
    setEmoji('image:' + categoryImage.value());
  });
  grid.prepend(customTile);
  document.querySelector('#category-emoji-clear').addEventListener('click', () => {
    upload.hidden = true;
    setEmoji('');
  });
}

function closeModals() {
  linkModal.hidden = true;
  categoryModal.hidden = true;
  appModal.hidden = true;
  widgetModal.hidden = true;
}

async function move(list, from, to) {
  try {
    await writes.move(list, from, to);
    await load();
  } catch (err) {
    toast(err.message);
  }
}

let editingApp = null;
let appEveryone = true;
let appPeople = [];
let appPicker = null;

function openAppEditor(app) {
  editingApp = app;
  appEveryone = app.visibleTo === state.model.everyone;
  appPeople = (state.model.audienceMembers[app.visibleTo] || []).map(m => m.person);
  document.querySelector('#app-modal-title').textContent = 'Edit ' + app.name;
  document.querySelector('#app-modal-mark').src = `/brand/apps/${app.key}.png`;
  document.querySelector('#app-modal-host').textContent = appOrigin(app.key).replace(/^https?:\/\//, '');
  document.querySelector('#app-name').value = app.name;
  document.querySelector('#app-tagline').value = app.tagline;
  setStatus('#app-status', '');
  showTab('app', 'details');
  appModal.hidden = false;
  appAudience = audienceCard(document.querySelector('#app-audience'), appEveryone ? null : app, 'The people these rules pick out, and anyone named below.', () => appPeople);
  if (!appPicker) {
    appPicker = createPersonPicker(document.querySelector('#app-people-picker'), {people: listed, onPick: (_, person) => addAppPerson(person)});
  }
  appPicker.reset();
  const modes = document.querySelector('#app-mode');
  modes.replaceChildren();
  for (const [value, words] of [[true, 'Everyone'], [false, 'Only some people']]) {
    const chip = el('button', 'audience-chip' + (appEveryone === value ? ' is-on' : ''), words);
    chip.type = 'button';
    chip.addEventListener('click', () => {
      appEveryone = value;
      for (const c of modes.children) {
        c.classList.toggle('is-on', c === chip);
      }
      document.querySelector('#app-list').hidden = appEveryone;
    });
    modes.append(chip);
  }
  document.querySelector('#app-list').hidden = appEveryone;
  renderAppPeople();
}

async function renderAppPeople() {
  const list = document.querySelector('#app-people');
  const everyone = await listed();
  const byId = new Map(everyone.map(p => [p.id, p.name_show]));
  list.replaceChildren();
  for (const person of appPeople) {
    const row = el('div', 'app-person');
    row.append(el('span', 'app-person-name', byId.get(person) || (state.model.people[person] || {}).name_show || person));
    const remove = el('button', 'link-button', 'Remove');
    remove.type = 'button';
    remove.addEventListener('click', () => {
      appPeople = appPeople.filter(p => p !== person);
      renderAppPeople();
      appAudience.refresh();
    });
    row.append(remove);
    list.append(row);
  }
}

function addAppPerson(person) {
  if (!person || appPeople.includes(person.id)) {
    return;
  }
  appPeople.push(person.id);
  appPicker.reset();
  renderAppPeople();
  appAudience.refresh();
}

function saveApp(e) {
  e.preventDefault();
  saved('#app-status', () => writes.saveApp(editingApp, {
    name: document.querySelector('#app-name').value,
    tagline: document.querySelector('#app-tagline').value,
    everyone: appEveryone,
    rules: appAudience.rules,
    people: appPeople,
  }));
}

const itemsModal = document.querySelector('#items-modal');

export function refreshPanels() {
  if (!itemsModal.hidden) {
    renderItemList();
    renderLayout();
  }
}

function rowButton(label, content, onClick) {
  const b = el('button', 'row-button');
  b.type = 'button';
  b.setAttribute('aria-label', label);
  b.title = label;
  b.append(content);
  b.addEventListener('click', onClick);
  return b;
}

let dragging = null;

function sortable(row, group, at, go) {
  row.draggable = true;
  row.classList.add('is-draggable');
  const grip = el('span', 'row-grip');
  grip.append(svg('grip'));
  row.prepend(grip);
  row.addEventListener('dragstart', e => {
    e.stopPropagation();
    dragging = {group, at, go};
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', group);
    row.classList.add('is-dragging');
  });
  row.addEventListener('dragend', () => {
    dragging = null;
    row.classList.remove('is-dragging');
  });
  const accepts = () => dragging && dragging.group === group && dragging.at !== at;
  const zone = e => {
    const box = row.getBoundingClientRect();
    return e.clientY - box.top < box.height / 2 ? 'before' : 'after';
  };
  const clear = () => row.classList.remove('is-drop-before', 'is-drop-after');
  row.addEventListener('dragover', e => {
    if (!accepts()) {
      return;
    }
    e.preventDefault();
    clear();
    row.classList.add('is-drop-' + zone(e));
  });
  row.addEventListener('dragleave', clear);
  row.addEventListener('drop', e => {
    clear();
    if (!accepts()) {
      return;
    }
    e.preventDefault();
    const {at: from, go} = dragging;
    dragging = null;
    let to = zone(e) === 'before' ? at : at + 1;
    if (to > from) {
      to -= 1;
    }
    if (to !== from) {
      go(from, to);
    }
  });
}

function itemRow(mark, title, meta, moves, edit, remove) {
  const row = el('div', 'category-row');
  const image = el('div', 'category-row-image');
  image.append(mark);
  const body = el('div', 'category-row-body');
  body.append(el('div', 'category-row-title', title));
  if (meta) {
    body.append(el('div', 'category-row-meta', meta));
  }
  const actions = el('div', 'category-row-actions');
  actions.append(rowButton(`Edit ${title}`, svg('edit'), edit));
  if (remove) {
    const x = rowButton(`Delete ${title}`, '×', remove);
    x.classList.add('is-danger');
    actions.append(x);
  }
  row.append(image, body, actions);
  sortable(row, moves.group, moves.at, moves.go);
  return row;
}

function categoryChildren(category) {
  const group = el('div', 'item-children');
  const apps = holdsApps(category);
  const items = apps ? state.model.apps : category.links;
  items.forEach((item, at) => {
    const moves = {at, group: category.id, go: (from, to) => move(items, from, to)};
    if (apps) {
      const img = el('img');
      img.src = `/brand/apps/${item.key}.png`;
      img.alt = '';
      group.append(itemRow(img, item.name, shownTo(item), moves, () => openAppEditor(item), null));
      return;
    }
    let mark = categoryMarkOf(category);
    if (item.imageUrl) {
      mark = el('img');
      mark.src = item.imageUrl;
      mark.alt = '';
    }
    const meta = [shownTo(item), item.url.replace(/^https?:\/\//, '')].filter(Boolean).join(' · ');
    group.append(itemRow(mark, item.title, meta, moves, () => openLinkEditor(item), () => removeLink(item)));
  });
  if (!items.length) {
    group.append(el('div', 'category-empty', apps ? 'No apps to show.' : 'No links yet.'));
  }
  if (!apps) {
    const add = el('button', 'button button-secondary button-small item-add');
    add.type = 'button';
    add.append(svg('plus'), el('span', '', 'Add Link'));
    add.addEventListener('click', () => openLinkEditor(null, category.id));
    group.append(add);
  }
  return group;
}

const expanded = new Set();

function collapsible(section, heading, id, title) {
  const toggle = rowButton(`Show or hide what is in ${title}`, svg('chevron-right'), () => {
    if (expanded.has(id)) {
      expanded.delete(id);
    } else {
      expanded.add(id);
    }
    paint();
  });
  toggle.classList.add('item-toggle');
  const paint = () => {
    section.classList.toggle('is-collapsed', !expanded.has(id));
    toggle.setAttribute('aria-expanded', String(expanded.has(id)));
  };
  paint();
  heading.insertBefore(toggle, heading.querySelector('.category-row-image') || heading.querySelector('.category-row-body'));
}

function renderItemList() {
  const list = document.querySelector('#item-list');
  list.replaceChildren();
  const go = (from, to) => move(state.model.widgets, from, to);
  widgetRows().forEach((w, at) => {
    const moves = {at, group: 'page', go};
    const section = el('div', 'item-section');
    const category = categoryOf(w.widget.key);
    if (category) {
      const heading = categoryRow(category, moves);
      section.append(heading, categoryChildren(category));
      collapsible(section, heading, category.id, category.title);
    } else {
      const row = itemRow(w.mark, w.name, w.meta, moves, () => openWidgetAudience(w.widget, w.name), null);
      row.classList.add('item-heading');
      row.insertBefore(el('span', 'item-toggle-space'), row.querySelector('.category-row-image'));
      section.append(row);
    }
    list.append(section);
  });
}

function shapeOf(layout) {
  const shape = el('span', 'layout-shape');
  for (const share of layout.split(' ')) {
    const part = el('span', 'layout-part');
    part.style.flexGrow = String(fraction(share));
    shape.append(part);
  }
  return shape;
}

async function layoutSave(work) {
  const list = document.querySelector('#layout-list');
  list.inert = true;
  setStatus('#items-status', 'Saving…');
  try {
    await work();
    setStatus('#items-status', '');
    await load();
  } catch (err) {
    setStatus('#items-status', err.message, true);
  } finally {
    list.inert = false;
  }
}

function saveLayout(rows) {
  return layoutSave(() => writes.saveLayout(rows));
}

function placeWidget(position, key) {
  const widgets = state.model.widgets;
  const from = widgets.findIndex(w => w.key === key);
  const last = widgets.length - 1;
  const to = Math.min(position, last);
  if (from === to) {
    return;
  }
  return layoutSave(() => {
    const moved = movedKey(widgets.map(w => w.order), from, to);
    const batch = [{set: widgets[from].id, cells: {order: moved}}];
    if (position <= last) {
      const there = widgets[position];
      const after = widgets.filter((_, i) => i !== from);
      after.splice(to, 0, {...widgets[from], order: moved});
      batch.push({set: there.id, cells: {order: movedKey(after.map(w => w.order), after.indexOf(there), from)}});
    }
    return writes.write(batch);
  });
}

function slotPicker(position, key, names) {
  const widgets = state.model.widgets;
  const select = el('select', 'layout-slot' + (key ? '' : ' is-empty'));
  const widget = widgets.find(w => w.key === key);
  const limited = Boolean(widget && widget.visibleTo !== state.model.everyone);
  select.setAttribute('aria-label', `Widget in place ${position + 1}` + (limited ? `, ${shownTo(widget)}` : ''));
  if (!key) {
    const none = el('option', '', 'Empty');
    none.value = '';
    none.selected = true;
    select.append(none);
  }
  if (position <= widgets.length) {
    for (const w of widgets) {
      const option = el('option', '', names.get(w.key));
      option.value = w.key;
      option.selected = w.key === key;
      select.append(option);
    }
  }
  select.disabled = position > widgets.length;
  select.addEventListener('change', () => placeWidget(position, select.value));
  const place = el('span', 'layout-place' + (limited ? ' is-limited' : ''));
  const chevron = svg('chevron-right');
  chevron.classList.add('layout-chevron');
  place.append(select, chevron);
  if (limited) {
    select.title = shownTo(widget);
    const eye = svg('eye');
    eye.classList.add('layout-eye');
    place.append(eye);
  }
  return place;
}

function renderLayout() {
  const list = document.querySelector('#layout-list');
  list.replaceChildren();
  const rows = widgetGrid();
  const names = new Map(widgetRows().map(w => [w.widget.key, w.name]));
  const set = state.model.layout.length;
  let position = 0;
  rows.forEach((row, at) => {
    const line = el('div', 'layout-row');
    line.append(el('span', 'layout-row-number', String(at + 1)));
    const preview = el('div', 'layout-preview');
    const empty = row.slots.every(slot => !slot.key);
    for (const slot of row.slots) {
      const box = slotPicker(position++, slot.key, names);
      box.style.flexGrow = String(fraction(slot.share));
      preview.append(box);
    }
    const choices = el('div', 'layout-choices');
    for (const layout of state.model.layouts) {
      const choice = el('button', 'layout-choice' + (layout === row.layout ? ' is-on' : ''));
      choice.type = 'button';
      choice.title = layout;
      choice.setAttribute('aria-label', `Row ${at + 1}: ${layout}`);
      choice.setAttribute('aria-pressed', String(layout === row.layout));
      choice.append(shapeOf(layout));
      choice.addEventListener('click', () => {
        if (layout === row.layout && row.set) {
          return;
        }
        const next = rows.slice(0, Math.max(at + 1, set)).map(r => r.layout);
        next[at] = layout;
        saveLayout(next);
      });
      choices.append(choice);
    }
    line.append(preview, choices);
    if (empty && row.set) {
      const x = rowButton(`Remove row ${at + 1}`, '×', () => saveLayout(state.model.layout.filter((_, i) => i !== at)));
      x.classList.add('is-danger');
      line.append(x);
    }
    list.append(line);
  });
}

function showPageTab(key) {
  const strip = tabStrip([{key: 'widgets', label: 'Widgets'}, {key: 'layout', label: 'Layout'}], key, 2, showPageTab);
  document.querySelector('#page-tabs').replaceChildren(strip);
  document.querySelector('#page-tab-widgets').hidden = key !== 'widgets';
  document.querySelector('#page-tab-layout').hidden = key !== 'layout';
  document.querySelector('#add-section').hidden = key !== 'widgets';
}

async function removeLink(link) {
  if (!confirm(`Delete “${link.title}”?`)) {
    return;
  }
  setStatus('#items-status', 'Deleting…');
  try {
    await writes.deleteLink(link);
    setStatus('#items-status', '');
    await load();
  } catch (err) {
    setStatus('#items-status', err.message, true);
  }
}

export function openEditPanel() {
  setStatus('#items-status', '');
  renderItemList();
  renderLayout();
  showPageTab('widgets');
  itemsModal.hidden = false;
}

async function removeCategory(category) {
  if (category.links.length) {
    toast('Move or delete its links first.');
    return;
  }
  if (!confirm(`Delete the category “${category.title}”?`)) {
    return;
  }
  setStatus('#items-status', 'Deleting…');
  try {
    await writes.deleteCategory(category);
    setStatus('#items-status', '');
    await load();
  } catch (err) {
    setStatus('#items-status', err.message, true);
  }
}

function categoryRow(category, moves) {
  const row = el('div', 'category-row');
  const mark = el('div', 'category-row-image');
  mark.append(categoryMarkOf(category));
  row.append(mark);
  const body = el('div', 'category-row-body');
  body.append(el('div', 'category-row-title', category.title));
  const style = inGrid(category) ? 'Icon grid' : 'Compact list';
  if (holdsApps(category)) {
    const n = state.model.apps.length;
    body.append(el('div', 'category-row-meta', `The community apps · ${style} · ${n} you see · ${shownTo(category)}`));
  } else {
    body.append(el('div', 'category-row-meta', `${style} · ${category.links.length} link${category.links.length === 1 ? '' : 's'} · ${shownTo(category)}`));
  }
  row.append(body);
  const actions = el('div', 'category-row-actions');
  actions.append(rowButton(`Edit ${category.title}`, svg('edit'), () => openCategoryEditor(category)));
  if (!holdsApps(category)) {
    const remove = rowButton(`Delete ${category.title}`, '×', () => removeCategory(category));
    remove.classList.add('is-danger');
    actions.append(remove);
  }
  row.append(actions);
  sortable(row, moves.group, moves.at, moves.go);
  return row;
}

function saveLink(e) {
  e.preventDefault();
  saved('#link-status', () => writes.saveLink(editingLink, {
    title: document.querySelector('#link-title').value,
    description: document.querySelector('#link-description').value,
    url: document.querySelector('#link-url').value,
    image: linkImageChanged ? linkImage.value() : undefined,
    category: document.querySelector('#link-category').value,
    visible: document.querySelector('#link-visible').checked,
    rules: linkAudience.rules,
  }));
}

async function deleteLink() {
  if (!confirm(`Delete “${editingLink.title}”?`)) {
    return;
  }
  setStatus('#link-status', 'Deleting…');
  try {
    await writes.deleteLink(editingLink);
    closeModals();
    await load();
  } catch (err) {
    setStatus('#link-status', err.message, true);
  }
}

function saveCategory(e) {
  e.preventDefault();
  const category = editingCategory;
  saved('#category-status', () => writes.saveCategory(category, {
    title: document.querySelector('#category-title').value,
    style: styleOf(category, document.querySelector('#category-style').value === 'grid'),
    emoji: document.querySelector('#category-emoji').value.trim(),
    descriptions: document.querySelector('#category-descriptions').checked,
    sidebar: document.querySelector('#category-sidebar').checked,
    rules: categoryAudience.rules,
  }));
}

async function deleteCategory() {
  if (editingCategory.links.length) {
    setStatus('#category-status', 'Move or delete its links first.', true);
    return;
  }
  if (!confirm(`Delete the category “${editingCategory.title}”?`)) {
    return;
  }
  setStatus('#category-status', 'Deleting…');
  try {
    await writes.deleteCategory(editingCategory);
    closeModals();
    await load();
  } catch (err) {
    setStatus('#category-status', err.message, true);
  }
}

function widgetOf(category) {
  return state.model.widgets.find(w => w.key === widgetKey(category));
}

function styleOf(category, grid) {
  if (category && holdsApps(category)) {
    return grid ? 'apps-grid' : 'apps';
  }
  return grid ? 'cards' : 'tiles';
}

export function initEditing() {
  document.querySelector('#add-section').addEventListener('click', () => openCategoryEditor(null));
  linkForm.addEventListener('submit', saveLink);
  categoryForm.addEventListener('submit', saveCategory);
  appForm.addEventListener('submit', saveApp);
  document.querySelector('#app-people-button').hidden = true;
  document.querySelector('#link-delete').addEventListener('click', deleteLink);
  document.querySelector('#category-delete').addEventListener('click', deleteCategory);
  wireEmojiPicker();
  for (const button of document.querySelectorAll('[data-close]')) {
    button.addEventListener('click', () => {
      button.closest('.modal-overlay').hidden = true;
    });
  }
  document.querySelector('#widget-form').addEventListener('submit', saveWidgetAudience);
  for (const overlay of [linkModal, categoryModal, appModal, widgetModal, itemsModal]) {
    overlay.addEventListener('click', e => {
      if (e.target === overlay) {
        overlay.hidden = true;
      }
    });
  }
  document.addEventListener('keydown', e => {
    if (e.key !== 'Escape') {
      return;
    }
    if (!linkModal.hidden || !categoryModal.hidden || !appModal.hidden || !widgetModal.hidden) {
      closeModals();
      return;
    }
    itemsModal.hidden = true;
  });
}
