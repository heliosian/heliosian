import {state, applyModel, group} from './state.js';
import {el} from './dom.js';
import {initChrome, renderChrome, setTitle, clearSearch} from './chrome.js';
import {groupsPage} from './pages/groups.js';
import {groupPage, newGroupPage} from './pages/group.js';
import {adminPage} from './pages/admin.js';

export async function load() {
  const res = await fetch('/api/groups/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  applyModel(await res.json());
  render();
}

export function navigate(path) {
  history.pushState(null, '', path);
  render();
  document.querySelector('#main').scrollTo(0, 0);
}

function notFound(what) {
  const page = el('div', 'list-page');
  page.append(el('h1', 'page-title', 'Not here'), el('p', 'row-text', `${what} is not in the app.`));
  setTitle('Not here');
  return page;
}

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  if (!parts.length) {
    return groupsPage();
  }
  switch (parts[0]) {
    case 'new':
      return newGroupPage();
    case 'groups': {
      const g = group(parts[1] || '');
      return g ? groupPage(g) : notFound(parts[1] || 'That group');
    }
    case 'admin':
      return adminPage();
  }
  return notFound('That page');
}

export function render() {
  const page = document.querySelector('#page');
  page.className = '';
  clearSearch();
  page.replaceChildren(route());
  document.body.classList.toggle('is-admin', location.pathname === '/admin');
  renderChrome();
}

document.addEventListener('click', e => {
  const a = e.target.closest('a[data-link]');
  if (!a || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) {
    return;
  }
  e.preventDefault();
  navigate(a.getAttribute('href'));
});

window.addEventListener('popstate', render);
document.addEventListener('groups:refresh', render);

initChrome();
load();
