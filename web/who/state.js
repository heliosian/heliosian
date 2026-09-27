import {loadNavOpen} from './storage.js';

export const state = {model: null, everyoneOrder: [], familyOrder: [], tab: 'everyone', classTab: 'by-classroom', rosterTab: 'students', rosterSectionExcluded: new Set(), q: '', filterGrades: new Set(), filterClassrooms: new Set(), filterRoles: new Set(), filterRoleExcluded: new Set(), filterCities: new Set(), filterPronouns: new Set(), filterTags: new Set(), filterTagRelations: new Set(), filterNew: false, staffDeptExcluded: new Set(), tagListView: 'faces', navOpen: loadNavOpen(), gvGreeting: 'Family of the kids', gvSiblings: true, gvKidEmail: false, gvInviteBy: 'group', gvSystem: ''};

export let byEmail = {};
export let tags = {};
export let tagManagers = {};
export let shared = {};
export let lists = {};

export let familiesByEmail = {};

function indexFamilies() {
  familiesByEmail = {};
  for (const key of Object.keys(state.model.families || {}).sort()) {
    const f = state.model.families[key];
    for (const email of [...(f.adultEmails || []), ...(f.kidEmails || [])]) {
      (familiesByEmail[email] = familiesByEmail[email] || []).push(f);
    }
  }
}

export let privacyLinks = null;
export let staleYears = null;
export let colors = null;

export function applyConfig(config) {
  privacyLinks = config.privacyLinks;
  staleYears = config.staleYears;
  colors = {staff: config.staffColor, grades: config.gradeColors, classrooms: config.classroomColors};
}

export function applyModel(model) {
  state.model = model;
  document.body.dataset.userEmail = model.user.email;
  document.body.dataset.mapsKey = model.mapsKey;
  tags = model.tags || {};
  tagManagers = model.tagManagers || {};
  shared = {};
  for (const t of model.sharedTags || []) {
    shared[`shared:${t.owner}:${t.name}`] = t;
  }
  lists = {};
  for (const l of model.lists) {
    lists[l.key] = l;
  }
  byEmail = {};
  for (const p of model.people) {
    byEmail[p.email] = p;
  }
  indexFamilies();
}
