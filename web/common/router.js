import {el} from '/elements.js';
import {setTitle} from '/shell.js';
import {closeLayers} from '/modal.js';

let app = null;

export function startApp(config) {
  app = config;
  document.addEventListener('click', e => {
    const a = e.target.closest('a[data-link]');
    if (!a || e.defaultPrevented || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) {
      return;
    }
    e.preventDefault();
    navigate(a.getAttribute('href'));
  });
  window.addEventListener('popstate', () => {
    closeLayers();
    draw(true);
    main().scrollTo(0, (history.state && history.state.scroll) || 0);
  });
  return load();
}

export async function load() {
  await app.model();
  render();
}

function main() {
  return document.querySelector('#main');
}

function here() {
  return location.pathname + location.search;
}

export function trail() {
  return (history.state && history.state.trail) || [];
}

function trailTo(path) {
  const at = trail().indexOf(path);
  if (at >= 0) {
    return trail().slice(at + 1);
  }
  return [here(), ...trail()].slice(0, 3);
}

export function navigate(path) {
  closeLayers();
  history.replaceState({...history.state, scroll: main().scrollTop}, '');
  history.pushState({trail: trailTo(path)}, '', path);
  draw(true);
  main().scrollTo(0, 0);
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
  draw(false);
}

function draw(entered) {
  app.prepare(entered);
  const node = route();
  document.body.classList.toggle('is-admin', location.pathname === '/admin');
  app.show(node);
}
