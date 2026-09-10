import {state, applyModel, isAdmin} from './state.js';
import {renderCategories} from './cards.js';
import {initEditing} from './edit.js';
import {githubBadge} from '/github-badge.js';

function renderChrome() {
  const user = state.model.user;
  document.querySelector('.user-avatar').textContent = user.initial;
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
  renderCategories();
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
initEditing();
load();
