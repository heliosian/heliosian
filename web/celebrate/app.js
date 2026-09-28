import {applyModel, celebrationByCode, familyMember, resolvePath, partyPath} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {api} from '/api.js';
import {initModal} from '/modal.js';
import {startApp, load, notFound} from '/router.js';
import {partiesPage} from './pages/parties.js';
import {partyPage} from './pages/party.js';
import {myPage} from './pages/my.js';
import {hostingPage} from './pages/hosting.js';
import {adminPage} from './pages/admin.js';

function party(parts) {
  const p = resolvePath(location.pathname);
  if (p && partyPath(p) !== location.pathname) {
    history.replaceState(null, '', partyPath(p));
  }
  return p ? partyPage(p) : notFound(parts[0] === 'p' ? 'That address' : 'That party');
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
  model: async () => applyModel(await api('GET', '/api/celebrate/model')),
  routes,
  missing: 'is not on the site, or is not something you can see.',
  prepare: clearSearch,
  show: showPage,
});
