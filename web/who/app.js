import {state, model, listAt, loadModel, groupKeyOf, openGroup, loadMembers, kindAt} from './state.js';
import {segments, shuffled} from './dom.js';
import {tabParam} from '/tabs.js';
import {el} from '/elements.js';
import {startApp, notFound, render} from '/router.js';
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
  people: 'Directory',
  p: 'Directory',
  g: 'Directory',
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

function people() {
  state.tab = tabParam('everyone');
  return peoplePage();
}

const lookedUp = new Set();

function group(parts) {
  const kind = kindAt(parts);
  if (kind === 'family') {
    return familyPage(parts[1]);
  }
  if (kind === 'classroom') {
    state.rosterTab = tabParam('students');
    return classroomPage(parts[1]);
  }
  if (kind === 'grade') {
    state.rosterTab = tabParam('students');
    return gradePage(parts[1]);
  }
  const key = parts[1] ? groupKeyOf(parts[1]) : '';
  if (!key && parts[1] && !lookedUp.has(parts[1])) {
    openGroup(parts[1]).then(() => {
      lookedUp.add(parts[1]);
      render();
    });
    return el('div', 'content container');
  }
  if (!key) {
    return notFound('That group');
  }
  state.filterTags = new Set([key]);
  state.filterTagRelations = loadTagRelations(key);
  state.tagListView = 'faces';
  if (listAt(key) && listAt(key).people === null) {
    loadMembers([key]).then(render);
    return el('div', 'content container');
  }
  return listPage();
}

const routes = {
  admin: () => adminPage(),
  people,
  p: parts => parts[1] ? personPage(parts[1]) : notFound('That person'),
  g: group,
  'my-family': () => myFamilyPage(),
  classrooms: () => {
    state.classTab = tabParam('by-classroom');
    return classroomsPage();
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
  redirect: () => location.pathname + location.search === model.movedFrom ? model.moved : '',
  missing: 'is not in the directory.',
  prepare: entered => preparePage(sectionTitles[segments()[0]] || 'Helios Who?', entered),
  show: showPage,
}).then(maybeShowInstallPrompt);
