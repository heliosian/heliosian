import {loadNavOpen} from './storage.js';

export const state = {model: null, tab: 'everyone', classTab: 'by-classroom', rosterTab: 'students', rosterSectionExcluded: new Set(), q: '', filterGrades: new Set(), filterClassrooms: new Set(), filterRoles: new Set(), filterRoleExcluded: new Set(), filterCities: new Set(), filterPronouns: new Set(), filterTags: new Set(), filterTagRelations: new Set(), filterNew: false, staffDeptExcluded: new Set(), tagListView: 'faces', navOpen: loadNavOpen(), gvGreeting: 'Family of the kids', gvSiblings: true, gvKidEmail: false, gvInviteBy: 'group', gvSystem: ''};

export let byEmail = {};
export let tags = {};

// familiesByEmail inverts the model's family member lists once per load: every
// family a person belongs to, in sorted key order - one for a parent (an adult
// belongs to at most one household), one or more for a kid in two households.
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

// The platform config, fetched from /api/config beside the model: the privacy links
// (admin-editable, since both URLs belong to other systems this app doesn't control),
// the staleness thresholds, and the colors keyed by grade and classroom name.
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
  byEmail = {};
  for (const p of model.people) {
    byEmail[p.email] = p;
  }
  indexFamilies();
}
