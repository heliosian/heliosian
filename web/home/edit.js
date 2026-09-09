import {state, categoryTitles} from './state.js';
import {el} from './dom.js';
import {load} from './app.js';

const linkModal = document.querySelector('#link-modal');
const linkForm = document.querySelector('#link-form');
const categoryModal = document.querySelector('#category-modal');
const categoryForm = document.querySelector('#category-form');

let editingLink = null;
let editingCategory = null;
let pendingLinkImage = '';
let pendingCategoryImage = '';

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
  const remove = document.querySelector(`#${prefix}-image-remove`);
  preview.hidden = !url;
  placeholder.hidden = Boolean(url);
  remove.hidden = !url;
  if (url) {
    preview.src = url;
  }
}

function wireImagePicker(prefix, onChange) {
  const file = document.querySelector(`#${prefix}-image-file`);
  file.addEventListener('change', async () => {
    if (!file.files.length) {
      return;
    }
    setStatus(`#${prefix}-status`, 'Uploading image…');
    try {
      const name = await uploadImage(file.files[0]);
      onChange(name);
      showImage(prefix, '/' + name);
      setStatus(`#${prefix}-status`, '');
    } catch (err) {
      setStatus(`#${prefix}-status`, err.message, true);
    }
    file.value = '';
  });
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
  for (const title of categoryTitles()) {
    const option = el('option', '', title);
    option.value = title;
    option.selected = title === selected;
    select.append(option);
  }
}

export function openLinkEditor(link) {
  editingLink = link;
  pendingLinkImage = link ? link.image || '' : '';
  document.querySelector('#link-modal-title').textContent = link ? 'Edit Link' : 'Add Link';
  document.querySelector('#link-title').value = link ? link.title : '';
  document.querySelector('#link-description').value = link ? link.description || '' : '';
  document.querySelector('#link-url').value = link ? link.url : '';
  document.querySelector('#link-visible').checked = link ? link.visible : true;
  document.querySelector('#link-delete').hidden = !link;
  fillCategories(link ? link.category : categoryTitles()[0]);
  showImage('link', link && link.imageUrl ? link.imageUrl : '');
  setStatus('#link-status', '');
  linkModal.hidden = false;
  document.querySelector('#link-title').focus();
}

export function openCategoryEditor(category) {
  editingCategory = category;
  pendingCategoryImage = category ? category.image || '' : '';
  document.querySelector('#category-modal-title').textContent = category ? 'Edit Category' : 'Add Category';
  document.querySelector('#category-title').value = category ? category.title : '';
  document.querySelector('#category-delete').hidden = !category;
  showImage('category', category && category.imageUrl ? category.imageUrl : '');
  setStatus('#category-status', '');
  categoryModal.hidden = false;
  document.querySelector('#category-title').focus();
}

function closeModals() {
  linkModal.hidden = true;
  categoryModal.hidden = true;
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
      image: pendingCategoryImage,
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
  document.querySelector('#add-link').addEventListener('click', () => openLinkEditor(null));
  document.querySelector('#add-category').addEventListener('click', () => openCategoryEditor(null));
  linkForm.addEventListener('submit', saveLink);
  categoryForm.addEventListener('submit', saveCategory);
  document.querySelector('#link-delete').addEventListener('click', deleteLink);
  document.querySelector('#category-delete').addEventListener('click', deleteCategory);
  wireImagePicker('link', name => {
    pendingLinkImage = name;
  });
  wireImagePicker('category', name => {
    pendingCategoryImage = name;
  });
  for (const button of document.querySelectorAll('[data-close]')) {
    button.addEventListener('click', closeModals);
  }
  for (const overlay of [linkModal, categoryModal]) {
    overlay.addEventListener('click', e => {
      if (e.target === overlay) {
        closeModals();
      }
    });
  }
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeModals();
    }
  });
}
