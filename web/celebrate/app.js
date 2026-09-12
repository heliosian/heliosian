import {applyModel, celebration, familyMember, resolvePath, partyPath} from './state.js';
import {el} from './dom.js';
import {initChrome, renderChrome, setTitle, clearSearch} from './chrome.js';
import {initModal} from './edit.js';
import {partiesPage} from './pages/parties.js';
import {partyPage} from './pages/party.js';
import {myPage} from './pages/my.js';
import {hostingPage} from './pages/hosting.js';
import {adminPage} from './pages/admin.js';

export async function load() {
  const res = await fetch('/api/celebrate/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  applyModel(await res.json());
  renderChrome();
  render();
}

export function navigate(path) {
  history.pushState(null, '', path);
  render();
  document.querySelector('#main').scrollTo(0, 0);
}

function notFound(what) {
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Not here'), el('p', 'row-text', `${what} is not on the site, or is not something you can see.`));
  setTitle('Not here');
  return page;
}

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  if (!parts.length) {
    return partiesPage(null);
  }
  switch (parts[0]) {
    case 'celebrations':
      return celebration(parts[1]) ? partiesPage(parts[1]) : notFound('That celebration');
    case 'parties':
    case 'p': {
      // /parties/{id} and /p/{pretty} both reach the party, through the
      // Redirects tab when an address has since changed; the bar is
      // corrected so the address people copy next is the live one.
      const p = resolvePath(location.pathname);
      if (p && partyPath(p) !== location.pathname) {
        history.replaceState(null, '', partyPath(p));
      }
      return p ? partyPage(p) : notFound(parts[0] === 'p' ? 'That address' : 'That party');
    }
    case 'my': {
      // /my/{name} is one member of the household, by the address's local
      // part; anyone else is not a page.
      if (!parts[1]) {
        return myPage(null);
      }
      const person = familyMember(parts[1]);
      return person ? myPage(person.email) : notFound('That person');
    }
    case 'hosting':
      return hostingPage();
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
  // Admin Tools is its own window: the shell's rail, toolbar and tab bar
  // step aside for the admin chrome (see pages/admin.js).
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
document.addEventListener('celebrate:refresh', render);

initChrome();
initModal();
load();
