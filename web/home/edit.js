import {state, linkCategories, tagLabelsOf} from './state.js';
import {categoryIcons, iconOf, categoryMark} from './dom.js';
import {el, svg, toast} from '/elements.js';
import {load} from '/router.js';
import {imageTools} from '/images.js';
import {createPersonPicker} from '/picker.js';
import {listed} from '/directory.js';
import {appOrigin} from '/appswitch.js';
import {api} from '/api.js';
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
  let opened = null;
  let counts = [];
  let previewTimer = null;
  mount.replaceChildren();
  const head = el('span', '', withHead ? 'Visibility ' : '');
  head.append(el('small', 'audience-note', everyoneNote));
  const list = el('div', 'rules');
  const adders = el('div', 'rule-adders');
  const preview = el('div', 'audience-preview');
  mount.append(head, list, adders, preview);
  const countChip = rule => {
    const i = draft.indexOf(rule);
    const chip = el('span', 'rule-count');
    const n = counts[i];
    chip.hidden = n === undefined;
    chip.textContent = n === undefined ? '' : rule.kind === 'exclude' ? `${n} excluded` : `${n} match`;
    return chip;
  };
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
        for (const row of list.children) {
          if (row.refreshCount) {
            row.refreshCount();
          }
        }
        const names = answer.names.join(', ');
        preview.textContent = answer.count ? `Picks out ${answer.count} ${answer.count === 1 ? 'person' : 'people'}: ${names}${answer.count > answer.names.length ? '…' : ''}` : 'Picks out nobody yet.';
      } catch (err) {
        preview.textContent = err.message;
      }
    }, 300);
  };
  const render = () => {
    list.replaceChildren();
    for (const rule of draft) {
      list.append(rules.ruleRow(rule, askPreview, () => {
        draft.splice(draft.indexOf(rule), 1);
        if (opened === rule) {
          opened = null;
        }
        render();
        askPreview();
      }, opened === rule, countChip));
    }
    opened = null;
    adders.replaceChildren();
    for (const [kind, words] of [['include', 'Add include rule'], ['exclude', 'Add exclude rule']]) {
      const b = el('button', 'button button-secondary button-small', '');
      b.type = 'button';
      b.append(svg('plus'), el('span', '', words));
      b.addEventListener('click', () => {
        opened = rules.newRule(kind);
        draft.push(opened);
        render();
      });
      adders.append(b);
    }
  };
  render();
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
let editingWidget = '';

export function openWidgetAudience(key, title) {
  editingWidget = key;
  const rules = ((state.model.widgets || {})[key] || {}).rules || [];
  document.querySelector('#widget-modal-title').textContent = `Who sees ${title}`;
  widgetAudience = audienceCard(document.querySelector('#widget-audience'), rules, 'No rules means everyone.', 'widget:' + key, false);
  setStatus('#widget-status', '');
  widgetModal.hidden = false;
}

async function saveWidgetAudience(e) {
  e.preventDefault();
  setStatus('#widget-status', 'Saving…');
  try {
    await api('POST', '/api/apps/widgets/audience', {widget: editingWidget, rules: widgetAudience.rules});
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

export function openLinkEditor(link, category) {
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

export function openCategoryEditor(category) {
  editingCategory = category;
  document.querySelector('#category-modal-title').textContent = category ? 'Edit Category' : 'Add Category';
  document.querySelector('#category-title').value = category ? category.title : '';
  const events = Boolean(category && category.style === 'events');
  document.querySelector('#category-style').value = category && !events ? category.style : 'tiles';
  document.querySelector('#category-style-field').hidden = events;
  document.querySelector('#category-events-note').hidden = !events;
  syncAppsNote();
  document.querySelector('#category-delete').hidden = !category || events;
  document.querySelector('#category-max').value = category && category.max ? String(category.max) : '';
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

export async function moveLink(id, by) {
  try {
    await api('POST', '/api/apps/link/move', {id, by});
    await load();
  } catch (err) {
    toast(err.message);
  }
}

export async function moveApp(key, by) {
  const keys = (state.model.apps || []).map(a => a.key);
  const i = keys.indexOf(key);
  const j = i + by;
  if (i < 0 || j < 0 || j >= keys.length) {
    return;
  }
  keys.splice(j, 0, ...keys.splice(i, 1));
  try {
    await api('POST', '/api/admin/visibility/order', {apps: keys});
    await load();
  } catch (err) {
    toast(err.message);
  }
}

export async function moveWidget(key, by) {
  const keys = [...(state.model.widgetOrder || [])];
  const i = keys.indexOf(key);
  const j = i + by;
  if (i < 0 || j < 0 || j >= keys.length) {
    return;
  }
  keys.splice(j, 0, ...keys.splice(i, 1));
  try {
    await api('POST', '/api/apps/widgets/order', {widgets: keys});
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

export async function openAppEditor(app) {
  editingApp = app;
  const v = app.visibility || {visibility: 'list', emails: [], rules: []};
  appMode = v.visibility;
  appEmails = [...(v.emails || [])];
  document.querySelector('#app-modal-title').textContent = 'Edit ' + app.name;
  document.querySelector('#app-modal-mark').src = `/brand/apps/${app.key}.png` + (app.mark ? `?v=${app.mark}` : '');
  document.querySelector('#app-modal-host').textContent = appOrigin(app.key).replace(/^https?:\/\//, '');
  document.querySelector('#app-name').value = v.name || app.name;
  document.querySelector('#app-tagline').value = v.tagline || app.tagline;
  setStatus('#app-status', '');
  showTab('app', 'details');
  appModal.hidden = false;
  appAudience = audienceCard(document.querySelector('#app-audience'), v.rules || [], 'The people these rules pick out. No rules and nobody named means nobody.', 'app:' + app.key, false);
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
  const byEmail = new Map(everyone.map(p => [p.email, p.fullName]));
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

async function saveApp(e) {
  e.preventDefault();
  setStatus('#app-status', 'Saving…');
  try {
    await api('POST', '/api/admin/visibility', {
      app: editingApp.key,
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
    const {at: from, go: move} = dragging;
    dragging = null;
    let to = zone(e) === 'before' ? at : at + 1;
    if (to > from) {
      to -= 1;
    }
    if (to !== from) {
      move(to - from);
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

function categoryItems(category) {
  const group = el('div', 'item-children');
  const apps = category.style === 'apps';
  const items = apps ? state.model.apps || [] : category.links;
  items.forEach((item, at) => {
    const moves = {at, group: category.id};
    if (apps) {
      const img = el('img');
      img.src = `/brand/apps/${item.key}.png` + (item.mark ? `?v=${item.mark}` : '');
      img.alt = '';
      moves.go = by => moveApp(item.key, by);
      const v = item.visibility || {visibility: 'list', rules: [], emails: []};
      let who = 'Shown to everyone';
      if (v.visibility === 'list') {
        who = (v.rules || []).length || (v.emails || []).length ? shownTo(v.rules, v.emails || []) : 'Shown to nobody';
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
    moves.go = by => moveLink(item.id, by);
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

function widgetSection() {
  const section = el('div', 'item-section');
  const heading = el('div', 'category-row item-heading');
  const body = el('div', 'category-row-body');
  body.append(el('div', 'category-row-title', 'Widgets'));
  heading.append(body);
  const group = el('div', 'item-children');
  const rows = widgetRows();
  rows.forEach((w, at) => {
    const moves = {at, group: 'widgets', go: by => moveWidget(w.key, by)};
    group.append(itemRow(w.mark, w.name, w.meta, moves, () => openWidgetAudience(w.key, w.name), null));
  });
  section.append(heading, group);
  collapsible(section, heading, 'widgets', 'Widgets');
  return section;
}

function renderItemList() {
  const list = document.querySelector('#item-list');
  list.replaceChildren(widgetSection());
  const categories = linkCategories();
  if (!categories.length) {
    list.append(el('div', 'category-empty', 'No categories yet.'));
    return;
  }
  categories.forEach((category, at) => {
    const section = el('div', 'item-section');
    const heading = categoryRow(category, at);
    section.append(heading, categoryItems(category));
    collapsible(section, heading, category.id, category.title);
    list.append(section);
  });
}

async function removeLink(link) {
  if (!confirm(`Delete “${link.title}”?`)) {
    return;
  }
  setStatus('#items-status', 'Deleting…');
  try {
    await api('DELETE', '/api/apps/link', {id: link.id});
    setStatus('#items-status', '');
    await load();
  } catch (err) {
    setStatus('#items-status', err.message, true);
  }
}

export function openEditPanel() {
  setStatus('#items-status', '');
  renderItemList();
  itemsModal.hidden = false;
}

async function moveCategory(id, by) {
  const hidden = state.model.categories.filter(c => c.style === 'events').map(c => c.id);
  const ids = linkCategories().map(c => c.id);
  const at = ids.indexOf(id);
  const to = at + by;
  if (at < 0 || to < 0 || to >= ids.length) {
    return;
  }
  ids.splice(to, 0, ...ids.splice(at, 1));
  ids.push(...hidden);
  setStatus('#items-status', 'Saving…');
  try {
    await api('POST', '/api/apps/categories/order', {ids});
    setStatus('#items-status', '');
    await load();
  } catch (err) {
    setStatus('#items-status', err.message, true);
  }
}

async function removeCategory(category) {
  if (!confirm(`Delete the category \u201C${category.title}\u201D?`)) {
    return;
  }
  setStatus('#items-status', 'Deleting\u2026');
  try {
    await api('DELETE', '/api/apps/category', {id: category.id});
    setStatus('#items-status', '');
    await load();
  } catch (err) {
    setStatus('#items-status', err.message, true);
  }
}

function categoryRow(category, at) {
  const row = el('div', 'category-row');
  const mark = el('div', 'category-row-image');
  mark.append(categoryMark(iconOf(category)));
  row.append(mark);
  const body = el('div', 'category-row-body');
  body.append(el('div', 'category-row-title', category.title));
  const limit = category.max ? ` \u00b7 shows ${category.max}` : '';
  if (category.style === 'events') {
    const n = (state.model.upcoming || []).length;
    body.append(el('div', 'category-row-meta', `Upcoming events from Helios When \u00b7 ${n} ahead${limit}`));
  } else if (category.style === 'apps') {
    const n = (state.model.apps || []).length;
    body.append(el('div', 'category-row-meta', `The community apps \u00b7 ${n} you see${limit} \u00b7 ${shownTo(category.rules)}`));
  } else {
    const style = category.style === 'cards' ? 'Feature cards' : 'Compact tiles';
    body.append(el('div', 'category-row-meta', `${style} \u00b7 ${category.links.length} link${category.links.length === 1 ? '' : 's'}${limit} \u00b7 ${shownTo(category.rules)}`));
  }
  row.append(body);

  const actions = el('div', 'category-row-actions');
  const edit = el('button', 'row-button');
  edit.type = 'button';
  edit.setAttribute('aria-label', `Edit ${category.title}`);
  edit.append(svg('edit'));
  edit.addEventListener('click', () => openCategoryEditor(category));
  const remove = el('button', 'row-button is-danger');
  remove.type = 'button';
  remove.setAttribute('aria-label', `Delete ${category.title}`);
  remove.textContent = '\u00d7';
  remove.disabled = category.style === 'events';
  remove.title = category.style === 'events' ? 'The events section can be renamed or moved, not deleted' : '';
  remove.addEventListener('click', () => removeCategory(category));
  actions.append(edit, remove);
  row.append(actions);
  sortable(row, 'categories', at, by => moveCategory(category.id, by));
  return row;
}

async function saveLink(e) {
  e.preventDefault();
  setStatus('#link-status', 'Saving…');
  try {
    await api('POST', '/api/apps/link', {
      id: editingLink ? editingLink.id : '',
      title: document.querySelector('#link-title').value,
      description: document.querySelector('#link-description').value,
      url: document.querySelector('#link-url').value,
      image: linkImage.value(),
      category: document.querySelector('#link-category').value,
      visible: document.querySelector('#link-visible').checked,
      rules: linkAudience.rules,
    });
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
    await api('DELETE', '/api/apps/link', {id: editingLink.id});
    closeModals();
    await load();
  } catch (err) {
    setStatus('#link-status', err.message, true);
  }
}

async function saveCategory(e) {
  e.preventDefault();
  setStatus('#category-status', 'Saving…');
  try {
    await api('POST', '/api/apps/category', {
      id: editingCategory ? editingCategory.id : '',
      title: document.querySelector('#category-title').value,
      style: editingCategory && editingCategory.style === 'events' ? 'events' : document.querySelector('#category-style').value,
      emoji: document.querySelector('#category-emoji').value.trim(),
      max: document.querySelector('#category-max').value.trim(),
      rules: categoryAudience.rules,
    });
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
    await api('DELETE', '/api/apps/category', {id: editingCategory.id});
    closeModals();
    await load();
  } catch (err) {
    setStatus('#category-status', err.message, true);
  }
}

function syncAppsNote() {
  const style = document.querySelector('#category-style');
  document.querySelector('#category-apps-note').hidden = style.closest('.field').hidden || style.value !== 'apps';
}

export function initEditing() {
  document.querySelector('#category-style').addEventListener('change', syncAppsNote);
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
