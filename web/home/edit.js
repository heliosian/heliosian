import {state, categoryTitles, linkCategoryTitles, tagLabelsOf} from './state.js';
import {el, svg, categoryIcons, iconOf, toast} from './dom.js';
import {load} from './app.js';
import {openCropTool} from '/crop.js';
import {createPersonPicker} from '/picker.js';
import {appOrigin} from '/toolbar.js';
import {rulesEditor} from '/rules.js';
import {tabStrip} from '/tabs.js';

const linkModal = document.querySelector('#link-modal');
const linkForm = document.querySelector('#link-form');
const categoryModal = document.querySelector('#category-modal');
const appModal = document.querySelector('#app-modal');
const appForm = document.querySelector('#app-form');
const categoryForm = document.querySelector('#category-form');
const categoriesModal = document.querySelector('#categories-modal');
const imageSearchModal = document.querySelector('#image-search-modal');

let editingLink = null;
let editingCategory = null;
let pendingLinkImage = '';

function setStatus(selector, message, error) {
  const status = document.querySelector(selector);
  status.textContent = message;
  status.classList.toggle('error', Boolean(error));
}

async function uploadImage(file) {
  const body = new FormData();
  body.append('image', file);
  const res = await fetch('/api/apps/image', {method: 'POST', body});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return (await res.json()).name;
}

function showImage(prefix, url) {
  const preview = document.querySelector(`#${prefix}-image-preview`);
  const placeholder = document.querySelector(`#${prefix}-image-placeholder`);
  preview.hidden = !url;
  placeholder.hidden = Boolean(url);
  for (const id of ['remove', 'crop']) {
    document.querySelector(`#${prefix}-image-${id}`).hidden = !url;
  }
  if (url) {
    preview.src = url;
  }
}

function wireImagePicker(prefix, onChange) {
  const zone = document.querySelector(`#${prefix}-image-drop`);
  const file = document.querySelector(`#${prefix}-image-file`);
  const preview = document.querySelector(`#${prefix}-image-preview`);
  const upload = async picked => {
    if (!picked) {
      return;
    }
    setStatus(`#${prefix}-status`, 'Uploading image…');
    try {
      const name = await uploadImage(picked);
      onChange(name);
      showImage(prefix, '/' + name);
      setStatus(`#${prefix}-status`, '');
    } catch (err) {
      setStatus(`#${prefix}-status`, err.message, true);
    }
    file.value = '';
  };
  file.addEventListener('change', () => upload(file.files[0]));
  zone.addEventListener('click', e => {
    if (!e.target.closest('label, button')) {
      file.click();
    }
  });
  zone.addEventListener('dragover', e => {
    e.preventDefault();
    zone.classList.add('is-dragover');
  });
  zone.addEventListener('dragleave', () => zone.classList.remove('is-dragover'));
  zone.addEventListener('drop', e => {
    e.preventDefault();
    zone.classList.remove('is-dragover');
    upload(e.dataTransfer.files[0]);
  });
  document.querySelector(`#${prefix}-image-crop`).addEventListener('click', () => openCropTool(preview.src, false, async blob => {
    await upload(new File([blob], 'crop.jpg', {type: 'image/jpeg'}));
    return true;
  }));
  document.querySelector(`#${prefix}-image-remove`).addEventListener('click', () => {
    onChange('');
    showImage(prefix, '');
  });
}

async function send(method, url, body) {
  const res = await fetch(url, {method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  if (!res.ok) {
    throw new Error(await res.text());
  }
}

function fillCategories(selected) {
  const select = document.querySelector('#link-category');
  select.replaceChildren();
  for (const title of linkCategoryTitles()) {
    const option = el('option', '', title);
    option.value = title;
    option.selected = title === selected;
    select.append(option);
  }
}

const rules = rulesEditor({
  el, svg,
  options: () => state.model.options || {classrooms: [], grades: [], tags: [], lists: [], roles: ['Student', 'Parent', 'Staff'], relations: ['Parents', 'Children', 'Siblings']},
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
        const res = await fetch('/api/apps/audience/preview', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({thing, rules: said})});
        if (!res.ok) {
          throw new Error(await res.text());
        }
        const answer = await res.json();
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

function showTab(prefix, key) {
  const strip = tabStrip([{key: 'details', label: 'Details'}, {key: 'visibility', label: 'Visibility'}], key, 2, k => showTab(prefix, k));
  document.querySelector(`#${prefix}-tabs`).replaceChildren(strip);
  document.querySelector(`#${prefix}-tab-details`).hidden = key !== 'details';
  document.querySelector(`#${prefix}-tab-visibility`).hidden = key !== 'visibility';
}

export function openLinkEditor(link, category) {
  editingLink = link;
  pendingLinkImage = link ? link.image || '' : '';
  document.querySelector('#link-modal-title').textContent = link ? 'Edit Link' : 'Add Link';
  document.querySelector('#link-title').value = link ? link.title : '';
  document.querySelector('#link-description').value = link ? link.description || '' : '';
  document.querySelector('#link-url').value = link ? link.url : '';
  document.querySelector('#link-visible').checked = link ? link.visible : true;
  linkAudience = audienceCard(document.querySelector('#link-audience'), link ? link.rules : [], 'No rules means everyone.', 'link:' + (link ? link.title : ''));
  document.querySelector('#link-delete').hidden = !link;
  document.querySelector('#link-image-find').hidden = !imageSearchOn();
  fillCategories(link ? link.category : category || linkCategoryTitles()[0]);
  showImage('link', link && link.imageUrl ? link.imageUrl : '');
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
  categoryAudience = audienceCard(document.querySelector('#category-audience'), category ? category.rules : [], 'No rules means everyone; the whole section, links and all.', 'category:' + (category ? category.title : ''));
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
    button.append(svg(name), el('span', '', words));
    button.addEventListener('click', () => setEmoji('icon:' + name));
    grid.append(button);
  }
  document.querySelector('#category-emoji-clear').addEventListener('click', () => setEmoji(''));
}


function closeModals() {
  linkModal.hidden = true;
  categoryModal.hidden = true;
  appModal.hidden = true;
}

export async function moveLink(title, by) {
  try {
    await send('POST', '/api/apps/link/move', {title, by});
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
  [keys[i], keys[j]] = [keys[j], keys[i]];
  try {
    await send('POST', '/api/admin/visibility/order', {apps: keys});
    await load();
  } catch (err) {
    toast(err.message);
  }
}

let editingApp = null;
let appMode = 'everyone';
let appEmails = [];
let adminState = null;
let appPicker = null;

async function loadAdminState() {
  if (!adminState) {
    const res = await fetch('/api/admin/state');
    if (!res.ok) {
      throw new Error(await res.text());
    }
    adminState = await res.json();
  }
  return adminState;
}

export async function openAppEditor(app) {
  editingApp = app;
  const v = app.visibility || {visibility: 'list', emails: [], rules: []};
  appMode = v.visibility;
  appEmails = [...(v.emails || [])];
  document.querySelector('#app-modal-title').textContent = 'Edit ' + app.name;
  document.querySelector('#app-modal-mark').src = `/brand/apps/${app.key}.png` + (app.mark ? `?v=${app.mark}` : '');
  document.querySelector('#app-modal-host').textContent = appOrigin(app.host || app.key).replace(/^https?:\/\//, '');
  document.querySelector('#app-name').value = v.name || app.name;
  document.querySelector('#app-tagline').value = v.tagline || app.tagline;
  setStatus('#app-status', '');
  showTab('app', 'details');
  appModal.hidden = false;
  appAudience = audienceCard(document.querySelector('#app-audience'), v.rules || [], 'The people these rules pick out. No rules and nobody named means nobody.', 'app:' + app.key, false);
  let people = [];
  try {
    const admin = await loadAdminState();
    people = admin.people || [];
  } catch (err) {
    setStatus('#app-status', err.message, true);
  }
  if (!appPicker) {
    appPicker = createPersonPicker(document.querySelector('#app-people-picker'));
  }
  appPicker.setPeople(people);
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
  const byEmail = new Map((adminState ? adminState.people : []).map(p => [p.email, p.name]));
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
  const email = (appPicker.value || appPicker.text).toLowerCase();
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
    await send('POST', '/api/admin/visibility', {
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

export function refreshCategoryManager() {
  if (!categoriesModal.hidden) {
    renderCategoryList();
  }
}

async function moveCategory(title, by) {
  const titles = categoryTitles();
  const at = titles.indexOf(title);
  const to = at + by;
  if (at < 0 || to < 0 || to >= titles.length) {
    return;
  }
  titles.splice(to, 0, ...titles.splice(at, 1));
  setStatus('#categories-status', 'Saving…');
  try {
    await send('POST', '/api/apps/categories/order', {titles});
    setStatus('#categories-status', '');
    await load();
  } catch (err) {
    setStatus('#categories-status', err.message, true);
  }
}

async function removeCategory(category) {
  if (!confirm(`Delete the category \u201C${category.title}\u201D?`)) {
    return;
  }
  setStatus('#categories-status', 'Deleting\u2026');
  try {
    await send('DELETE', '/api/apps/category', {title: category.title});
    setStatus('#categories-status', '');
    await load();
  } catch (err) {
    setStatus('#categories-status', err.message, true);
  }
}

function categoryRow(category, at, total) {
  const row = el('div', 'category-row');
  const mark = el('div', 'category-row-image');
  mark.append(svg(iconOf(category)));
  row.append(mark);
  const body = el('div', 'category-row-body');
  body.append(el('div', 'category-row-title', category.title));
  const limit = category.max ? ` \u00b7 shows ${category.max}` : '';
  if (category.style === 'events') {
    const n = (state.model.upcoming || []).length;
    body.append(el('div', 'category-row-meta', `Upcoming events from Helios When \u00b7 ${n} ahead${limit}`));
  } else if (category.style === 'apps') {
    const n = (state.model.apps || []).length;
    body.append(el('div', 'category-row-meta', `The community apps \u00b7 ${n} you see${limit}`));
  } else {
    const style = category.style === 'cards' ? 'Feature cards' : 'Compact tiles';
    body.append(el('div', 'category-row-meta', `${style} \u00b7 ${category.links.length} link${category.links.length === 1 ? '' : 's'}${limit}`));
  }
  row.append(body);

  const actions = el('div', 'category-row-actions');
  const up = el('button', 'row-button');
  up.type = 'button';
  up.setAttribute('aria-label', `Move ${category.title} up`);
  up.textContent = '\u2191';
  up.disabled = at === 0;
  up.addEventListener('click', () => moveCategory(category.title, -1));
  const down = el('button', 'row-button');
  down.type = 'button';
  down.setAttribute('aria-label', `Move ${category.title} down`);
  down.textContent = '\u2193';
  down.disabled = at === total - 1;
  down.addEventListener('click', () => moveCategory(category.title, 1));
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
  actions.append(up, down, edit, remove);
  row.append(actions);
  return row;
}

function renderCategoryList() {
  const list = document.querySelector('#category-list');
  const categories = state.model.categories;
  list.replaceChildren();
  if (!categories.length) {
    list.append(el('div', 'category-empty', 'No categories yet.'));
    return;
  }
  categories.forEach((category, at) => list.append(categoryRow(category, at, categories.length)));
}

export function openCategoryManager() {
  setStatus('#categories-status', '');
  renderCategoryList();
  categoriesModal.hidden = false;
}

async function saveLink(e) {
  e.preventDefault();
  setStatus('#link-status', 'Saving…');
  try {
    await send('POST', '/api/apps/link', {
      original: editingLink ? editingLink.title : '',
      title: document.querySelector('#link-title').value,
      description: document.querySelector('#link-description').value,
      url: document.querySelector('#link-url').value,
      image: pendingLinkImage,
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
    await send('DELETE', '/api/apps/link', {title: editingLink.title});
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
    await send('POST', '/api/apps/category', {
      original: editingCategory ? editingCategory.title : '',
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
    await send('DELETE', '/api/apps/category', {title: editingCategory.title});
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
  document.querySelector('#add-category').addEventListener('click', () => openCategoryEditor(null));
  document.querySelector('#edit-categories').addEventListener('click', openCategoryManager);
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
  wireImagePicker('link', name => {
    pendingLinkImage = name;
  });
  document.querySelector('#link-image-find').addEventListener('click', e => {
    e.stopPropagation();
    openImageSearch(document.querySelector('#link-title').value.trim(), name => {
      pendingLinkImage = name;
      showImage('link', '/' + name);
    });
  });
  wireEmojiPicker();
  wireImageSearch();
  for (const button of document.querySelectorAll('[data-close]')) {
    button.addEventListener('click', () => {
      button.closest('.modal-overlay').hidden = true;
    });
  }
  for (const overlay of [linkModal, categoryModal, appModal, categoriesModal, imageSearchModal]) {
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
    if (!linkModal.hidden || !categoryModal.hidden) {
      closeModals();
      return;
    }
    categoriesModal.hidden = true;
  });
}

let onImagePicked = null;
let imageSearchBusy = false;

function imageSearchOn() {
  return Boolean(state.model && state.model.imageSearch);
}

function openImageSearch(initial, onPicked) {
  onImagePicked = onPicked;
  const input = document.querySelector('#image-search-input');
  input.value = initial || '';
  document.querySelector('#image-search-grid').replaceChildren();
  document.querySelector('#image-search-status').textContent = '';
  imageSearchModal.hidden = false;
  input.focus();
  if (input.value) {
    runImageSearch();
  }
}

async function runImageSearch() {
  const q = document.querySelector('#image-search-input').value.trim();
  const status = document.querySelector('#image-search-status');
  const grid = document.querySelector('#image-search-grid');
  if (!q || imageSearchBusy) {
    return;
  }
  imageSearchBusy = true;
  status.textContent = 'Searching…';
  grid.replaceChildren();
  try {
    const res = await fetch(`/api/apps/images/search?q=${encodeURIComponent(q)}`);
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const hits = await res.json();
    status.textContent = hits.length ? '' : 'Nothing found.';
    for (const hit of hits) {
      const tile = el('button', 'image-search-hit');
      tile.type = 'button';
      const img = el('img');
      img.src = hit.thumb;
      img.alt = hit.title;
      img.loading = 'lazy';
      img.addEventListener('load', () => img.classList.add('is-loaded'));
      tile.append(img);
      tile.title = `${hit.title} - ${hit.width}×${hit.height}`;
      tile.addEventListener('click', () => importImage(hit, tile));
      grid.append(tile);
    }
  } catch (err) {
    status.textContent = err.message;
  }
  imageSearchBusy = false;
}

async function importImage(hit, tile) {
  const status = document.querySelector('#image-search-status');
  if (imageSearchBusy) {
    return;
  }
  imageSearchBusy = true;
  status.textContent = 'Importing…';
  tile.classList.add('is-picked');
  try {
    const res = await fetch('/api/apps/images/import', {
      method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: hit.id}),
    });
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const {name} = await res.json();
    imageSearchModal.hidden = true;
    onImagePicked(name);
  } catch (err) {
    status.textContent = err.message;
    tile.classList.remove('is-picked');
  }
  imageSearchBusy = false;
}

function wireImageSearch() {
  document.querySelector('#image-search-go').addEventListener('click', runImageSearch);
  document.querySelector('#image-search-input').addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      runImageSearch();
    }
  });
}
