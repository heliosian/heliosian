import {state, applyModel, isAdmin} from './state.js';
import {renderCategories, renderNav} from './cards.js';
import {initEditing} from './edit.js';
import {githubBadge} from '/github-badge.js';

function renderChrome() {
  const user = state.model.user;
  // The same hero photo the directory leads with (their own, else their
  // family's); the initial only stands in when there is no photo at all.
  const avatar = document.querySelector('.user-avatar');
  if (user.photoUrl) {
    const img = document.createElement('img');
    img.src = user.photoUrl + '?thumb=1';
    img.alt = '';
    avatar.replaceChildren(img);
  } else {
    avatar.textContent = user.initial;
  }
  document.querySelector('.user-menu-email').textContent = user.email;
  document.querySelector('.user-menu-admin').hidden = !user.isAdmin;
  document.querySelector('.page-actions').hidden = !isAdmin();
}

export async function load() {
  const res = await fetch('/api/apps/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  applyModel(await res.json());
  renderChrome();
  renderNav();
  renderCategories(document.querySelector('#search').value);
}

function initSearch() {
  const search = document.querySelector('#search');
  search.addEventListener('input', () => renderCategories(search.value));
  search.addEventListener('keydown', e => {
    if (e.key === 'Escape' && search.value) {
      // Swallow the key so the modal/menu handlers do not also fire on what
      // the user meant as "clear the box".
      e.stopPropagation();
      search.value = '';
      renderCategories('');
    }
  });
}

function initChrome() {
  document.querySelector('#site-footer').append(githubBadge());
  const menu = document.querySelector('#user-menu');
  document.querySelector('#user').addEventListener('click', e => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
  });
  document.addEventListener('click', () => {
    menu.hidden = true;
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      menu.hidden = true;
    }
  });
}

initChrome();
initSearch();
initEditing();
load();
