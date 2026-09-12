import {state, categoryTitles, linkCategoryTitles} from './state.js';
import {el, svg} from './dom.js';
import {load} from './app.js';
import {openCropTool} from '/crop.js';

const linkModal = document.querySelector('#link-modal');
const linkForm = document.querySelector('#link-form');
const categoryModal = document.querySelector('#category-modal');
const categoryForm = document.querySelector('#category-form');
const categoriesModal = document.querySelector('#categories-modal');
const imageSearchModal = document.querySelector('#image-search-modal');
const emojiLibraryModal = document.querySelector('#emoji-library-modal');

let editingLink = null;
let editingCategory = null;
let pendingLinkImage = '';

// The emoji on offer for a category, in groups a school community reaches
// for; any other emoji can be pasted into the box above them.
const emojiGroups = [
  ['School', ['🏫', '📚', '🎒', '🧑‍🏫', '🍎', '🚌', '✏️', '📝', '📖', '🔬', '🧪', '🧮', '🖍️', '🎓', '🏛️', '🔔', '🗓️', '📅', '📋', '📎']],
  ['Celebrations', ['🎉', '🎊', '🎂', '🎁', '🎈', '🥳', '🎃', '🎄', '🎆', '🎇', '🪅', '🎀', '🕯️', '🍀', '🐣', '❄️', '🌟', '✨', '🏮', '🎏']],
  ['Sports & Play', ['⚽', '🏀', '🏈', '⚾', '🎾', '🏐', '🏊', '🚴', '🏃', '🤸', '🧗', '⛷️', '🏄', '🥋', '🏆', '🥇', '🎯', '🎲', '🧩', '🪁']],
  ['Arts & Music', ['🎨', '🖌️', '🎭', '🎵', '🎶', '🎤', '🎸', '🎹', '🥁', '🎻', '📷', '🎬', '📽️', '🖼️', '✂️', '🧵', '🧶', '📻', '🎧', '🩰']],
  ['Food', ['🍕', '🍔', '🌮', '🍜', '🍣', '🥗', '🍪', '🧁', '🍩', '🍦', '🍿', '☕', '🍵', '🧃', '🍫', '🍓', '🥐', '🍱', '🍲', '🥤']],
  ['Outdoors', ['🏕️', '🌞', '🌱', '🌻', '🌳', '🌲', '🌈', '🌊', '🏔️', '🐦', '🦋', '🐝', '🐢', '🌸', '🍁', '🍂', '☀️', '🌙', '🔥', '🧭']],
  ['Community', ['💬', '🤝', '❤️', '🫶', '👋', '👨‍👩‍👧‍👦', '🧑‍🤝‍🧑', '🙌', '👏', '🗳️', '📣', '📰', '💌', '🏠', '🏘️', '🚗', '🧡', '💛', '💚', '💙']],
  ['Things', ['⭐', '🔗', '📌', '📍', '🛠️', '🔧', '💡', '🔑', '💰', '🧾', '📦', '🛒', '🎟️', '🗂️', '📊', '💻', '📱', '🖨️', '⏰', '🧭']],
];

function setStatus(selector, message, error) {
  const status = document.querySelector(selector);
  status.textContent = message;
  status.classList.toggle('error', Boolean(error));
}

// Image choices are uploaded on selection, so the save that follows only
// needs to record the name the server handed back.
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

// The link's picture, the way HCA-Team's editors pick one: a zone that takes a
// drop or a click, a Choose and a Find an image button, and once there is a
// picture, Crop (the freeform tool from web/common/crop.js) and Remove.
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
  // The zone itself takes a click (anywhere but the buttons) and a drop.
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

// category is the one to start in for a new link (an add card names its own).
export function openLinkEditor(link, category) {
  editingLink = link;
  pendingLinkImage = link ? link.image || '' : '';
  document.querySelector('#link-modal-title').textContent = link ? 'Edit Link' : 'Add Link';
  document.querySelector('#link-title').value = link ? link.title : '';
  document.querySelector('#link-description').value = link ? link.description || '' : '';
  document.querySelector('#link-url').value = link ? link.url : '';
  document.querySelector('#link-visible').checked = link ? link.visible : true;
  document.querySelector('#link-delete').hidden = !link;
  fillCategories(link ? link.category : category || linkCategoryTitles()[0]);
  showImage('link', link && link.imageUrl ? link.imageUrl : '');
  setStatus('#link-status', '');
  linkModal.hidden = false;
  document.querySelector('#link-title').focus();
}

export function openCategoryEditor(category) {
  editingCategory = category;
  document.querySelector('#category-modal-title').textContent = category ? 'Edit Category' : 'Add Category';
  document.querySelector('#category-title').value = category ? category.title : '';
  // The events section keeps its style and cannot be deleted; the rest of
  // the editor - name and emoji - is its to use.
  const events = Boolean(category && category.style === 'events');
  document.querySelector('#category-style').value = category && !events ? category.style : 'tiles';
  document.querySelector('#category-style-field').hidden = events;
  document.querySelector('#category-events-note').hidden = !events;
  document.querySelector('#category-delete').hidden = !category || events;
  document.querySelector('#category-max').value = category && category.max ? String(category.max) : '';
  setEmoji(category ? category.emoji || '' : '');
  setStatus('#category-status', '');
  categoryModal.hidden = false;
  document.querySelector('#category-title').focus();
}

// The emoji field: the box holds the choice, the big swatch shows it, and the
// grid below marks it; setEmoji keeps the three agreeing.
function setEmoji(value) {
  const input = document.querySelector('#category-emoji');
  input.value = value;
  document.querySelector('#category-emoji-current').textContent = value;
  document.querySelector('#category-emoji-clear').hidden = !value;
  for (const button of document.querySelectorAll('#category-emoji-grid button')) {
    button.classList.toggle('is-picked', button.textContent === value);
  }
}

function wireEmojiPicker() {
  const grid = document.querySelector('#category-emoji-grid');
  for (const [name, choices] of emojiGroups) {
    grid.append(el('div', 'emoji-group', name));
    for (const emoji of choices) {
      const button = el('button', '', emoji);
      button.type = 'button';
      button.setAttribute('aria-label', emoji);
      button.addEventListener('click', () => setEmoji(emoji));
      grid.append(button);
    }
  }
  const input = document.querySelector('#category-emoji');
  input.addEventListener('input', () => setEmoji(input.value.trim()));
  document.querySelector('#category-emoji-clear').addEventListener('click', () => setEmoji(''));
  document.querySelector('#category-emoji-more').addEventListener('click', openEmojiLibrary);
  document.querySelector('#emoji-library-search').addEventListener('input', paintEmojiLibrary);
}

// The whole library - every single-character emoji, by group, with its
// Unicode name for the search box - loads from /emoji.json the first time it
// is opened. A pick lands in the category editor and closes the sheet.
let emojiLibrary = null;

async function openEmojiLibrary() {
  emojiLibraryModal.hidden = false;
  const search = document.querySelector('#emoji-library-search');
  search.value = '';
  search.focus();
  if (!emojiLibrary) {
    document.querySelector('#emoji-library').textContent = 'Loading…';
    const res = await fetch('/emoji.json');
    emojiLibrary = res.ok ? await res.json() : [];
  }
  paintEmojiLibrary();
}

function paintEmojiLibrary() {
  const root = document.querySelector('#emoji-library');
  const needle = document.querySelector('#emoji-library-search').value.trim().toLowerCase();
  root.replaceChildren();
  let shown = 0;
  for (const [group, entries] of emojiLibrary || []) {
    const matches = entries.filter(([, name]) => !needle || name.includes(needle));
    if (!matches.length) {
      continue;
    }
    root.append(el('div', 'emoji-group', group));
    const row = el('div', 'emoji-library-row');
    for (const [emoji, name] of matches) {
      const button = el('button', '', emoji);
      button.type = 'button';
      button.title = name;
      button.setAttribute('aria-label', name);
      button.addEventListener('click', () => {
        setEmoji(emoji);
        emojiLibraryModal.hidden = true;
      });
      row.append(button);
    }
    root.append(row);
    shown += matches.length;
  }
  if (!shown) {
    root.append(el('div', 'modal-hint', 'Nothing by that name.'));
  }
}

function closeModals() {
  linkModal.hidden = true;
  categoryModal.hidden = true;
}

// The manager hands off to the category editor and stays open behind it, so
// every save and delete re-renders this list rather than closing it.
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
  row.append(el('div', 'category-row-image' + (category.emoji ? '' : ' is-blank'), category.emoji || category.title.slice(0, 1).toUpperCase()));
  const body = el('div', 'category-row-body');
  body.append(el('div', 'category-row-title', category.title));
  const limit = category.max ? ` \u00b7 shows ${category.max}` : '';
  if (category.style === 'events') {
    const n = (state.model.upcoming || []).length;
    body.append(el('div', 'category-row-meta', `Upcoming events from HCA-Team \u00b7 ${n} ahead${limit}`));
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

export function initEditing() {
  document.querySelector('#add-category').addEventListener('click', () => openCategoryEditor(null));
  document.querySelector('#edit-categories').addEventListener('click', openCategoryManager);
  linkForm.addEventListener('submit', saveLink);
  categoryForm.addEventListener('submit', saveCategory);
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
  for (const overlay of [linkModal, categoryModal, categoriesModal, imageSearchModal, emojiLibraryModal]) {
    overlay.addEventListener('click', e => {
      if (e.target === overlay) {
        overlay.hidden = true;
      }
    });
  }
  // Escape peels one layer: the editor first when it is over the manager.
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

// The picture search, as HCA-Team's editors have it: a box, a grid of results
// from whichever library is set up (Wikimedia Commons always; Unsplash,
// Pexels, Pixabay and Google Images behind their keys), and a click on one
// imports it through the server - which fetches and stores the picture like an
// upload - and hands the stored name to onPicked. SafeSearch is on server-side.
const sourceNotes = {
  'Unsplash': 'Free to use under the Unsplash License; the photographer is credited on each tile.',
  'Pexels': 'Free to use under the Pexels License; the photographer is credited on each tile.',
  'Pixabay': 'Photos, illustrations and vectors, free to use under the Pixabay Content License.',
  'Wikimedia Commons': 'Everything here is free to use; the licence is on each tile, and CC BY ones ask to be credited.',
  'Google Images': 'Pick a picture you have the right to use - a school photo, a poster, a flag, a public-domain image.',
};

let imageSource = '';
let onImagePicked = null;
let imageSearchBusy = false;

function imageSources() {
  return (state.model && state.model.imageSources) || ['Wikimedia Commons'];
}

function paintImageSources() {
  const wrap = document.querySelector('#image-search-sources');
  wrap.replaceChildren();
  const sources = imageSources();
  if (!sources.includes(imageSource)) {
    imageSource = sources[0];
  }
  // With more than one place to look, a segmented switch picks between them.
  if (sources.length > 1) {
    for (const source of sources) {
      const button = el('button', 'segment' + (source === imageSource ? ' is-on' : ''), source);
      button.type = 'button';
      button.addEventListener('click', () => {
        imageSource = source;
        paintImageSources();
        runImageSearch();
      });
      wrap.append(button);
    }
  }
  document.querySelector('#image-search-input').placeholder = `Search ${imageSource}…`;
  document.querySelector('#image-search-note').textContent = sourceNotes[imageSource] || '';
}

function openImageSearch(initial, onPicked) {
  onImagePicked = onPicked;
  const input = document.querySelector('#image-search-input');
  input.value = initial || '';
  document.querySelector('#image-search-grid').replaceChildren();
  document.querySelector('#image-search-status').textContent = '';
  paintImageSources();
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
    const res = await fetch(`/api/apps/images/search?q=${encodeURIComponent(q)}&source=${encodeURIComponent(imageSource)}`);
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
      tile.append(img, el('span', 'image-search-source', hit.credit || (hit.license ? `${hit.license} · ${hit.source}` : hit.source)));
      tile.title = `${hit.title} - ${hit.width}×${hit.height}${hit.license ? ` - ${hit.license}` : ''}`;
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
      method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({url: hit.url, download: hit.download || ''}),
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
