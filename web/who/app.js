import {state, applyModel} from './state.js';
import {segments, shuffled, tabParam} from './dom.js';
import {loadTagRelations} from './storage.js';
import {familyEntries} from './families.js';
import {initChrome, renderNav, setChrome, finishRender, renderUserChrome, renderSuperEditBanner, syncSuperEditCheckboxes, renderSpoofBanner, renderPrivacyMenuAlert} from './chrome.js';
import {initSearch} from './search.js';
import {maybeShowInstallPrompt} from './install.js';
import {renderPeople} from './pages/people.js';
import {renderListPage} from './pages/list.js';
import {renderGreenvelopePage} from './pages/invites.js';
import {renderPersonDetail} from './pages/person.js';
import {renderFamilyDetail} from './pages/family.js';
import {renderClassroomsPage, renderGradeDetail, renderClassroomDetail} from './pages/classrooms.js';
import {renderStaffPage} from './pages/staff.js';
import {renderPrivacyPage} from './pages/privacy.js';
import {renderMapPage} from './pages/map.js';

const sectionTitles = {
  people: 'People',
  classrooms: 'Gradebands',
  'my-family': 'My Family',
  staff: 'Staff',
  map: 'Map',
  'email-list': 'Everyone',
  greenvelope: 'Invites',
  'my-privacy': 'My Privacy',
};

function render() {
  renderNav();
  setChrome(sectionTitles[segments()[0]] || 'Helios Who?', null);
  const seg = segments();
  if (seg[0] === 'people' && seg[1]) {
    renderPersonDetail(seg[1]);
  } else if (seg[0] === 'families' && seg[1]) {
    renderFamilyDetail(seg[1]);
  } else if (seg[0] === 'people') {
    const tagParam = new URLSearchParams(location.search).get('tag');
    if (tagParam) {
      state.filterTags = new Set([tagParam]);
      state.filterTagRelations = loadTagRelations(tagParam);
      state.tagListView = 'faces';
      renderListPage();
    } else {
      state.tab = tabParam('everyone');
      renderPeople();
    }
  } else if (seg[0] === 'classrooms' && seg[1]) {
    state.rosterTab = tabParam('students');
    renderClassroomDetail(seg[1]);
  } else if (seg[0] === 'grades' && seg[1]) {
    state.rosterTab = tabParam('students');
    renderGradeDetail(seg[1]);
  } else if (seg[0] === 'classrooms') {
    state.classTab = tabParam('by-classroom');
    renderClassroomsPage();
  } else if (seg[0] === 'staff') {
    state.q = '';
    renderStaffPage();
  } else if (seg[0] === 'email-list') {
    state.q = '';
    state.filterTags = new Set();
    state.filterTagRelations = new Set();
    state.tagListView = 'emails';
    renderListPage();
  } else if (seg[0] === 'greenvelope') {
    state.q = '';
    state.filterTags = new Set();
    state.filterTagRelations = new Set();
    renderGreenvelopePage();
  } else if (seg[0] === 'map') {
    state.q = '';
    renderMapPage();
  } else if (seg[0] === 'my-privacy') {
    renderPrivacyPage();
  }
  finishRender();
}

export async function load() {
  const res = await fetch('/api/directory/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  applyModel(await res.json());
  state.everyoneOrder = shuffled(state.model.people);
  state.familyOrder = shuffled(familyEntries());
  renderUserChrome();
  renderSuperEditBanner();
  syncSuperEditCheckboxes();
  renderSpoofBanner();
  renderPrivacyMenuAlert();
  render();
  maybeShowInstallPrompt();
}

initChrome();
initSearch();
load();
