import {state, applyModel, applyConfig} from './state.js';
import {segments, shuffled} from './dom.js';
import {tabParam} from '/tabs.js';
import {api} from '/api.js';
import {startApp, notFound} from '/router.js';
import {loadTagRelations} from './storage.js';
import {familyEntries} from './families.js';
import {initChrome, preparePage, showPage, renderUserChrome, renderSuperEditBanner} from './chrome.js';
import {initSearch} from './search.js';
import {maybeShowInstallPrompt} from './install.js';
import {peoplePage} from './pages/people.js';
import {listPage} from './pages/list.js';
import {invitesPage} from './pages/invites.js';
import {personPage} from './pages/person.js';
import {guestPage} from './pages/guest.js';
import {familyPage} from './pages/family.js';
import {classroomsPage, gradePage, classroomPage} from './pages/classrooms.js';
import {staffPage} from './pages/staff.js';
import {privacyPage} from './pages/privacy.js';
import {mapPage} from './pages/map.js';
import {adminPage} from './pages/admin.js';

const sectionTitles = {
  people: 'People',
  classrooms: 'Gradebands',
  'my-family': 'My Family',
  staff: 'Staff',
  map: 'Map',
  'email-list': 'Everyone',
  greenvelope: 'Invites',
  'my-privacy': 'My Privacy',
  admin: 'Admin Tools',
};

function resetTagFilter() {
  state.q = '';
  state.filterTags = new Set();
  state.filterTagRelations = new Set();
}

function people(parts) {
  if (parts[1] && parts[1].startsWith('guest:')) {
    return guestPage(parts[1].slice('guest:'.length));
  }
  if (parts[1]) {
    return personPage(parts[1]);
  }
  const params = new URLSearchParams(location.search);
  const tagParam = params.get('tag') || params.get('list') || (params.get('shared') ? 'shared:' + params.get('shared') : '');
  if (tagParam) {
    state.filterTags = new Set([tagParam]);
    state.filterTagRelations = loadTagRelations(tagParam);
    state.tagListView = 'faces';
    return listPage();
  }
  state.tab = tabParam('everyone');
  return peoplePage();
}

const routes = {
  admin: () => adminPage(),
  people,
  families: parts => parts[1] ? familyPage(parts[1]) : notFound('That family'),
  classrooms: parts => {
    if (parts[1]) {
      state.rosterTab = tabParam('students');
      return classroomPage(parts[1]);
    }
    state.classTab = tabParam('by-classroom');
    return classroomsPage();
  },
  grades: parts => {
    state.rosterTab = tabParam('students');
    return parts[1] ? gradePage(parts[1]) : notFound('That grade');
  },
  staff: () => {
    state.q = '';
    return staffPage();
  },
  'email-list': () => {
    resetTagFilter();
    state.tagListView = 'emails';
    return listPage();
  },
  greenvelope: () => {
    resetTagFilter();
    return invitesPage();
  },
  map: () => {
    state.q = '';
    return mapPage();
  },
  'my-privacy': () => privacyPage(),
};

async function model() {
  const [directory, config] = await Promise.all([api('GET', '/api/directory/model'), api('GET', '/api/config')]);
  applyConfig(config);
  applyModel(directory);
  state.everyoneOrder = shuffled(state.model.people);
  state.familyOrder = shuffled(familyEntries());
  renderUserChrome();
  renderSuperEditBanner();
}

initChrome();
initSearch();
startApp({
  model,
  routes,
  missing: 'is not in the directory.',
  prepare: () => preparePage(sectionTitles[segments()[0]] || 'Helios Who?'),
  show: showPage,
}).then(maybeShowInstallPrompt);
