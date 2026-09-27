import {el} from '/toolbar.js';
import {setTitle} from '/shell.js';
import {closeLayers} from '/modal.js';

let app = null;

export function startApp(config) {
  app = config;
  document.addEventListener('click', e => {
    const a = e.target.closest('a[data-link]');
    if (!a || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) {
      return;
    }
    e.preventDefault();
    navigate(a.getAttribute('href'));
  });
  window.addEventListener('popstate', () => {
    closeLayers();
    render();
  });
  return load();
}

export async function load() {
  await app.model();
  render();
}

export function navigate(path) {
  closeLayers();
  history.pushState(null, '', path);
  render();
  document.querySelector('#main').scrollTo(0, 0);
}

export function setPath(path) {
  history.pushState(null, '', path);
  render();
}

export function notFound(what) {
  const target = app.redirect ? app.redirect(location.pathname) : '';
  if (target) {
    location.replace(target + (target.includes('?') ? '' : location.search));
    return el('div', 'list-page');
  }
  const page = el('div', 'list-page');
  page.append(el('h1', 'page-title', 'Not here'), el('p', 'row-text', `${what} ${app.missing}`));
  setTitle('Not here');
  return page;
}

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const handler = app.routes[parts[0] || ''];
  return handler ? handler(parts) : notFound('That page');
}

export function render() {
  app.prepare();
  const node = route();
  document.body.classList.toggle('is-admin', location.pathname === '/admin');
  app.show(node);
}
