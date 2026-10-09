import {state, linkCategories, tagLabelsOf, holdsApps, inGrid, widgetGrid, widgetKey, categoryOf, fraction} from './state.js';
import {categoryIcons, iconOf, categoryMark} from './dom.js';
import {el, svg, toast} from '/elements.js';
import {load} from '/router.js';
import {imageTools} from '/images.js';
import {createPersonPicker} from '/picker.js';
import {listed, emailOf} from '/directory.js';
import {appOrigin} from '/appswitch.js';
import {api} from '/api.js';
import {act, create, remove} from '/data.js';
import {movedKey} from '/order.js';
import {rulesEditor} from '/rules.js';
import {tabStrip} from '/tabs.js';
import {widgetRows} from './widgets.js';
import {shownTo} from './cards.js';

const linkModal = document.querySelector('#link-modal');
const linkForm = document.querySelector('#link-form');
const categoryModal = document.querySelector('#category-modal');
const appModal = document.querySelector('#app-modal');
const widgetModal = document.querySelector('#widget-modal');
const appForm = document.querySelector('#app-form');
const categoryForm = document.querySelector('#category-form');
const {imagePicker} = imageTools('/api/apps', {state});

let editingLink = null;
let editingCategory = null;
let linkImage = null;

function setStatus(selector, message, error) {
  const status = document.querySelector(selector);
  status.textContent = message;
  status.classList.toggle('error', Boolean(error));
}

function fillCategories(selected) {
  const select = document.querySelector('#link-category');
  select.replaceChildren();
  for (const category of linkCategories()) {
    const option = el('option', '', category.title);
    option.value = category.id;
    option.selected = category.id === selected;
    select.append(option);
  }
}

const rules = rulesEditor({
  options: () => state.model.options,
  personName: () => '',
});

function audienceCard(mount, initial, everyoneNote, thing, withHead = true) {
  const draft = (initial || []).map(r => ({...r, roles: [...(r.roles || [])], classrooms: [...(r.classrooms || [])], grades: [...(r.grades || [])], tags: [...(r.tags || [])], tagLabels: tagLabelsOf(r), family: [...(r.family || [])]}));
  let counts = [];
  let previewTimer = null;
  mount.replaceChildren();
  const head = el('span', '', withHead ? 'Visibility ' : '');
  head.append(el('small', 'audience-note', everyoneNote));
  const preview = el('div', 'audience-preview');
  const askPreview = () => {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(async () => {
      const said = draft.filter(rules.ruleSaysSomething);
      if (!said.length) {
        counts = [];
        preview.textContent = '';
        return;
      }
      try {
        const answer = await api('POST', '/api/apps/audience/preview', {thing, rules: said});
        counts = [];
        said.forEach((r, i) => {
          counts[draft.indexOf(r)] = answer.ruleCounts[i];
        });
        list.refreshCounts();
        const names = answer.names.join(', ');
        preview.textContent = answer.count ? `Picks out ${answer.count} ${answer.count === 1 ? 'person' : 'people'}: ${names}${answer.count > answer.names.length ? '…' : ''}` : 'Picks out nobody yet.';
      } catch (err) {
        preview.textContent = err.message;
      }
    }, 300);
  };
  const list = rules.rulesList({
    list: () => draft,
    empty: 'No rules yet.',
    onAdd: rule => draft.push(rule),
    onChange: askPreview,
    onRemove: rule => {
      draft.splice(draft.indexOf(rule), 1);
      list.render();
      askPreview();
    },
    onDone: () => {},
    count: rule => counts[draft.indexOf(rule)],
  });
  mount.append(head, list.node, preview);
  askPreview();
  return {
    get rules() {
      return draft.filter(rules.ruleSaysSomething);
    },
  };
}

let linkAudience = null;
let categoryAudience = null;
let appAudience = null;
let widgetAudience = null;
let editingWidget = null;

function openWidgetAudience(widget, title) {
  editingWidget = widget;
  document.querySelector('#widget-modal-title').textContent = title;
  document.querySelector('#widget-sidebar').checked = widget.sidebar;
  widgetAudience = audienceCard(document.querySelector('#widget-audience'), widget.rules, 'No rules means everyone.', 'widget:' + widget.key, false);
  setStatus('#widget-status', '');
  widgetModal.hidden = false;
}

async function saveWidgetAudience(e) {
  e.preventDefault();
  setStatus('#widget-status', 'Saving…');
  try {
    await act('home-widgets', editingWidget.id, 'edit', {rules: widgetAudience.rules, sidebar: document.querySelector('#widget-sidebar').checked});
    closeModals();
    await load();
  } catch (err) {
    setStatus('#widget-status', err.message, true);
  }
}

function showTab(prefix, key) {
  const strip = tabStrip([{key: 'details', label: 'Details'}, {key: 'visibility', label: 'Visibility'}], key, 2, k => showTab(prefix, k));
  document.querySelector(`#${prefix}-tabs`).replaceChildren(strip);
  document.querySelector(`#${prefix}-tab-details`).hidden = key !== 'details';
  document.querySelector(`#${prefix}-tab-visibility`).hidden = key !== 'visibility';
}

function openLinkEditor(link, category) {
  editingLink = link;
  linkImage = imagePicker(link ? link.image || '' : '', link && link.imageUrl ? link.imageUrl : '', {
    dropzone: true,
    hint: 'Optional - without one the category’s emoji shows.',
    query: () => document.querySelector('#link-title').value.trim(),
  });
  document.querySelector('#link-image').replaceChildren(linkImage.wrap);
  document.querySelector('#link-modal-title').textContent = link ? 'Edit Link' : 'Add Link';
  document.querySelector('#link-title').value = link ? link.title : '';
  document.querySelector('#link-description').value = link ? link.description || '' : '';
  document.querySelector('#link-url').value = link ? link.url : '';
  document.querySelector('#link-visible').checked = link ? link.visible : true;
  linkAudience = audienceCard(document.querySelector('#link-audience'), link ? link.rules : [], 'No rules means everyone.', 'link:' + (link ? link.id : ''));
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
  categoryAudience = audienceCard(document.querySelector('#category-audience'), category ? category.rules : [], 'No rules means everyone; the whole section, links and all.', 'category:' + (category ? category.id : ''));
  setEmoji(category ? category.emoji || '' : '');
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
    button.classList.toggle('is-picked', button.dataset.icon === value);
  }
}

function wireEmojiPicker() {
  const grid = document.querySelector('#category-emoji-grid');
  for (const [name, words] of categoryIcons) {
    const button = el('button', 'icon-choice');
    button.type = 'button';
    button.dataset.icon = 'icon:' + name;
    button.title = words;
    button.setAttribute('aria-label', words);
    button.append(categoryMark(name), el('span', '', words));
    button.addEventListener('click', () => setEmoji('icon:' + name));
    grid.append(button);
  }
  document.querySelector('#category-emoji-clear').addEventListener('click', () => setEmoji(''));
}

function closeModals() {
  linkModal.hidden = true;
  categoryModal.hidden = true;
  appModal.hidden = true;
  widgetModal.hidden = true;
}

async function move(type, list, from, to) {
  try {
    await act(type, list[from].id, 'edit', {order: movedKey(list.map(item => item.order), from, to)});
    await load();
  } catch (err) {
    toast(err.message);
  }
}

let editingApp = null;
let appMode = 'everyone';
let appEmails = [];
let everyone = [];
let appPicker = null;

async function openAppEditor(app) {
  editingApp = app;
  appMode = app.visibility;
  appEmails = [...app.emails];
  document.querySelector('#app-modal-title').textContent = 'Edit ' + app.name;
  document.querySelector('#app-modal-mark').src = `/brand/apps/${app.key}.png` + (app.mark ? `?v=${app.mark}` : '');
  document.querySelector('#app-modal-host').textContent = appOrigin(app.key).replace(/^https?:\/\//, '');
  document.querySelector('#app-name').value = app.name;
  document.querySelector('#app-tagline').value = app.tagline;
  setStatus('#app-status', '');
  showTab('app', 'details');
  appModal.hidden = false;
  appAudience = audienceCard(document.querySelector('#app-audience'), app.rules, 'The people these rules pick out. No rules and nobody named means nobody.', 'app:' + app.key, false);
  try {
    everyone = await listed();
  } catch (err) {
    setStatus('#app-status', err.message, true);
  }
  if (!appPicker) {
    appPicker = createPersonPicker(document.querySelector('#app-people-picker'), {address: true, people: listed});
  }
  appPicker.reset();
  const modes = document.querySelector('#app-mode');
  modes.replaceChildren();
  for (const [value, words] of [['everyone', 'Everyone'], ['list', 'Only some people']]) {
    const chip = el('button', 'audience-chip' + (appMode === value ? ' is-on' : ''), words);
    chip.type = 'button';
    chip.addEventListener('click', () => {
      appMode = value;
      for (const c of modes.children) {
        c.classList.toggle('is-on', c === chip);
      }
      document.querySelector('#app-list').hidden = appMode !== 'list';
    });
    modes.append(chip);
  }
  document.querySelector('#app-list').hidden = appMode !== 'list';
  renderAppPeople();
}

function renderAppPeople() {
  const byEmail = new Map(everyone.map(p => [emailOf(p), p.name_show]));
  const list = document.querySelector('#app-people');
  list.replaceChildren();
  for (const email of appEmails) {
    const row = el('div', 'app-person');
    row.append(el('span', 'app-person-name', byEmail.get(email) || email));
    if (byEmail.has(email)) {
      row.append(el('span', 'app-person-email', email));
    }
    const remove = el('button', 'link-button', 'Remove');
    remove.type = 'button';
    remove.addEventListener('click', () => {
      appEmails = appEmails.filter(e => e !== email);
      renderAppPeople();
    });
    row.append(remove);
    list.append(row);
  }
}

function addAppPerson() {
  const email = appPicker.value;
  if (!email) {
    return;
  }
  if (!appEmails.includes(email)) {
    appEmails.push(email);
  }
  appPicker.reset();
  renderAppPeople();
}

function sameRules(a, b) {
  const plain = rules => rules.map(r => [r.kind, r.roles || [], r.search || '', r.classrooms || [], r.grades || [], r.tags || [], r.family || []]);
  return JSON.stringify(plain(a)) === JSON.stringify(plain(b));
}

function changed(before, after) {
  const out = {};
  for (const [key, value] of Object.entries(after)) {
    const same = key === 'rules' ? sameRules(before.rules, value) : JSON.stringify(before[key]) === JSON.stringify(value);
    if (!same) {
      out[key] = value;
    }
  }
  return out;
}

async function edit(type, id, before, after) {
  const body = changed(before, after);
  if (Object.keys(body).length) {
    await act(type, id, 'edit', body);
  }
}

async function saveApp(e) {
  e.preventDefault();
  setStatus('#app-status', 'Saving…');
  const app = editingApp;
  try {
    await edit('apps', app.id, {visibility: app.visibility, emails: app.emails, name: app.name, tagline: app.tagline, rules: app.rules}, {
      visibility: appMode,
      emails: appEmails,
      name: document.querySelector('#app-name').value,
      tagline: document.querySelector('#app-tagline').value,
      rules: appAudience.rules,
    });
    closeModals();
    await load();
  } catch (err) {
    setStatus('#app-status', err.message, true);
  }
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
    const moves = {at, group: category.id};
    if (apps) {
      const img = el('img');
      img.src = `/brand/apps/${item.key}.png` + (item.mark ? `?v=${item.mark}` : '');
      img.alt = '';
      moves.go = (from, to) => move('apps', items, from, to);
      let who = 'Shown to everyone';
      if (item.visibility === 'list') {
        who = item.rules.length || item.emails.length ? shownTo(item.rules, item.emails) : 'Shown to nobody';
      }
      group.append(itemRow(img, item.name, who, moves, () => openAppEditor(item), null));
      return;
    }
    let mark = categoryMark(iconOf(category));
    if (item.imageUrl) {
      mark = el('img');
      mark.src = item.imageUrl;
      mark.alt = '';
    }
    moves.go = (from, to) => move('links', items, from, to);
    const meta = [item.visible === false ? 'Hidden' : '', shownTo(item.rules), item.url.replace(/^https?:\/\//, '')].filter(Boolean).join(' · ');
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
  const go = (from, to) => move('home-widgets', state.model.widgets, from, to);
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

async function layoutSave(write) {
  const list = document.querySelector('#layout-list');
  list.inert = true;
  setStatus('#items-status', 'Saving…');
  try {
    await write();
    setStatus('#items-status', '');
    await load();
  } catch (err) {
    setStatus('#items-status', err.message, true);
  } finally {
    list.inert = false;
  }
}

function saveLayout(rows) {
  return layoutSave(() => act('home-settings', state.model.settingsId, 'layout', {rows}));
}

function placeWidget(position, key) {
  const widgets = state.model.widgets;
  const from = widgets.findIndex(w => w.key === key);
  const last = widgets.length - 1;
  const to = Math.min(position, last);
  if (from === to) {
    return;
  }
  return layoutSave(async () => {
    const moved = movedKey(widgets.map(w => w.order), from, to);
    await act('home-widgets', widgets[from].id, 'edit', {order: moved});
    if (position > last) {
      return;
    }
    const there = widgets[position];
    const after = widgets.filter((_, i) => i !== from);
    after.splice(to, 0, {...widgets[from], order: moved});
    await act('home-widgets', there.id, 'edit', {order: movedKey(after.map(w => w.order), after.indexOf(there), from)});
  });
}

function slotPicker(position, key, names) {
  const widgets = state.model.widgets;
  const select = el('select', 'layout-slot' + (key ? '' : ' is-empty'));
  const widget = widgets.find(w => w.key === key);
  const limited = Boolean(widget && widget.rules.length);
  select.setAttribute('aria-label', `Widget in place ${position + 1}` + (limited ? `, ${shownTo(widget.rules)}` : ''));
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
    select.title = shownTo(widget.rules);
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
    await remove('links', link.id);
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
  if (!confirm(`Delete the category \u201C${category.title}\u201D?`)) {
    return;
  }
  setStatus('#items-status', 'Deleting\u2026');
  try {
    await remove('link-categories', category.id);
    setStatus('#items-status', '');
    await load();
  } catch (err) {
    setStatus('#items-status', err.message, true);
  }
}

function categoryRow(category, moves) {
  const row = el('div', 'category-row');
  const mark = el('div', 'category-row-image');
  mark.append(categoryMark(iconOf(category)));
  row.append(mark);
  const body = el('div', 'category-row-body');
  body.append(el('div', 'category-row-title', category.title));
  const style = inGrid(category) ? 'Icon grid' : 'Compact list';
  if (holdsApps(category)) {
    const n = state.model.apps.length;
    body.append(el('div', 'category-row-meta', `The community apps \u00b7 ${style} \u00b7 ${n} you see \u00b7 ${shownTo(category.rules)}`));
  } else {
    body.append(el('div', 'category-row-meta', `${style} \u00b7 ${category.links.length} link${category.links.length === 1 ? '' : 's'} \u00b7 ${shownTo(category.rules)}`));
  }
  row.append(body);
  const actions = el('div', 'category-row-actions');
  actions.append(rowButton(`Edit ${category.title}`, svg('edit'), () => openCategoryEditor(category)));
  if (!holdsApps(category)) {
    const remove = rowButton(`Delete ${category.title}`, '\u00d7', () => removeCategory(category));
    remove.classList.add('is-danger');
    actions.append(remove);
  }
  row.append(actions);
  sortable(row, moves.group, moves.at, moves.go);
  return row;
}

async function saveLink(e) {
  e.preventDefault();
  setStatus('#link-status', 'Saving…');
  const link = editingLink;
  const fields = {
    title: document.querySelector('#link-title').value,
    description: document.querySelector('#link-description').value,
    url: document.querySelector('#link-url').value,
    image: linkImage.value(),
    category: document.querySelector('#link-category').value,
    visible: document.querySelector('#link-visible').checked,
    rules: linkAudience.rules,
  };
  try {
    if (link) {
      await edit('links', link.id, {title: link.title, description: link.description || '', url: link.url, image: link.image || '', category: link.category, visible: link.visible, rules: link.rules}, fields);
    } else {
      await create('links', fields);
    }
    closeModals();
    await load();
  } catch (err) {
    setStatus('#link-status', err.message, true);
  }
}

async function deleteLink() {
  if (!confirm(`Delete “${editingLink.title}”?`)) {
    return;
  }
  setStatus('#link-status', 'Deleting…');
  try {
    await remove('links', editingLink.id);
    closeModals();
    await load();
  } catch (err) {
    setStatus('#link-status', err.message, true);
  }
}

async function saveCategory(e) {
  e.preventDefault();
  setStatus('#category-status', 'Saving…');
  const category = editingCategory;
  const fields = {
    title: document.querySelector('#category-title').value,
    style: styleOf(category, document.querySelector('#category-style').value === 'grid'),
    emoji: document.querySelector('#category-emoji').value.trim(),
    descriptions: document.querySelector('#category-descriptions').checked,
    sidebar: document.querySelector('#category-sidebar').checked,
    rules: categoryAudience.rules,
  };
  try {
    if (category) {
      const widget = widgetOf(category);
      await edit('link-categories', category.id, {title: category.title, style: category.style, emoji: category.emoji || '', descriptions: category.descriptions, sidebar: Boolean(widget && widget.sidebar), rules: category.rules}, fields);
    } else {
      await create('link-categories', fields);
    }
    closeModals();
    await load();
  } catch (err) {
    setStatus('#category-status', err.message, true);
  }
}

async function deleteCategory() {
  if (!confirm(`Delete the category “${editingCategory.title}”?`)) {
    return;
  }
  setStatus('#category-status', 'Deleting…');
  try {
    await remove('link-categories', editingCategory.id);
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
  document.querySelector('#app-people-button').addEventListener('click', addAppPerson);
  document.querySelector('#app-people-picker').addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addAppPerson();
    }
  });
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
