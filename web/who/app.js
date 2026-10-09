import {state, model, loadModel, tagKey, listKey, tags, lists} from './state.js';
import {segments, shuffled} from './dom.js';
import {tabParam} from '/tabs.js';
import {startApp, notFound} from '/router.js';
import {loadTagRelations} from './storage.js';
import {familyEntries} from './families.js';
import {initChrome, preparePage, showPage, renderUserChrome} from './chrome.js';
import {initSearch} from './search.js';
import {maybeShowInstallPrompt} from './install.js';
import {peoplePage} from './pages/people.js';
import {listPage} from './pages/list.js';
import {invitesPage} from './pages/invites.js';
import {personPage} from './pages/person.js';
import {familyPage, myFamilyPage} from './pages/family.js';
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
  if (parts[1]) {
    return personPage(parts[1]);
  }
  const params = new URLSearchParams(location.search);
  const key = params.get('tag') ? tagKey(params.get('tag')) : params.get('list') ? listKey(params.get('list')) : '';
  if (key) {
    if (!tags[key] && !lists[key]) {
      return notFound('That tag');
    }
    state.filterTags = new Set([key]);
    state.filterTagRelations = loadTagRelations(key);
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
  'my-family': () => myFamilyPage(),
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

async function load() {
  await loadModel();
  state.everyoneOrder = shuffled(model.people);
  state.familyOrder = shuffled(familyEntries());
  renderUserChrome();
}

initChrome();
initSearch();
startApp({
  model: load,
  routes,
  redirect: () => model.moved,
  missing: 'is not in the directory.',
  prepare: () => preparePage(sectionTitles[segments()[0]] || 'Helios Who?'),
  show: showPage,
}).then(maybeShowInstallPrompt);
