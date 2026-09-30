import {loadModel, celebrationByCode, familyMember, partyAt, fetchParty} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {el} from '/elements.js';
import {initModal} from '/modal.js';
import {startApp, load, render, notFound} from '/router.js';
import {partiesPage} from './pages/parties.js';
import {partyPage} from './pages/party.js';
import {myPage} from './pages/my.js';
import {hostingPage} from './pages/hosting.js';
import {adminPage} from './pages/admin.js';

const fetched = new Set();

function party(parts) {
  const segment = parts.length === 2 ? parts[1] : '';
  const p = segment ? partyAt(segment) : null;
  if (!p) {
    if (segment && !fetched.has(segment)) {
      fetched.add(segment);
      fetchParty(segment).then(found => {
        if (found) {
          history.replaceState(null, '', found.path);
          render();
        }
      });
      return el('div', 'list-page', 'Looking…');
    }
    return notFound(parts[0] === 'p' ? 'That address' : 'That party');
  }
  if (p.path !== location.pathname) {
    history.replaceState(null, '', p.path);
  }
  return partyPage(p);
}

const routes = {
  '': () => partiesPage(null),
  celebrations: parts => {
    const c = celebrationByCode(parts[1]);
    return c ? partiesPage(c.id) : notFound('That celebration');
  },
  parties: party,
  p: party,
  my: parts => {
    if (!parts[1]) {
      return myPage(null);
    }
    const person = familyMember(parts[1]);
    return person ? myPage(person.email) : notFound('That person');
  },
  hosting: () => hostingPage(),
  admin: () => adminPage(),
};

initChrome();
initModal(load);
startApp({
  model: loadModel,
  routes,
  missing: 'is not on the site, or is not something you can see.',
  prepare: clearSearch,
  show: showPage,
});
