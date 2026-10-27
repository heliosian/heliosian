import {loadModel, celebrationByCode, familyMember, partyAt, fetchParty} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {initModal} from '/modal.js';
import {startApp, load, loadRow, notFound} from '/router.js';
import {partiesPage} from './pages/parties.js';
import {partyPage} from './pages/party.js';
import {myPage} from './pages/my.js';
import {hostingPage} from './pages/hosting.js';
import {adminPage} from './pages/admin.js';

function party(parts) {
  const segment = parts.length === 2 ? parts[1] : '';
  return loadRow(segment, {
    find: partyAt,
    load: fetchParty,
    settle: p => {
      if (p.path !== location.pathname) {
        history.replaceState(null, '', p.path);
      }
    },
    show: partyPage,
    missing: 'That party',
  });
}

const routes = {
  '': () => partiesPage(null),
  celebrations: parts => {
    const c = celebrationByCode(parts[1]);
    return c ? partiesPage(c.id) : notFound('That celebration');
  },
  g: party,
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
