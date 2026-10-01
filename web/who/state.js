import {batch, me as whoAmI} from '/data.js';
import {loadNavOpen} from './storage.js';

export const state = {model: null, everyoneOrder: [], familyOrder: [], tab: 'everyone', classTab: 'by-classroom', rosterTab: 'students', rosterSectionExcluded: new Set(), q: '', filterGrades: new Set(), filterClassrooms: new Set(), filterRoles: new Set(), filterRoleExcluded: new Set(), filterCities: new Set(), filterPronouns: new Set(), filterTags: new Set(), filterTagRelations: new Set(), filterNew: false, staffDeptExcluded: new Set(), tagListView: 'faces', navOpen: loadNavOpen(), gvGreeting: '', gvSiblings: true, gvKidEmail: false, gvInviteBy: 'group', gvSystem: ''};

export let byId = {};
export let tags = {};
export let shared = {};
export let lists = {};

export function tagKey(id) {
  return 'tag:' + id;
}

export let familiesById = {};

function indexFamilies() {
  familiesById = {};
  for (const key of Object.keys(state.model.families).sort()) {
    const f = state.model.families[key];
    for (const id of [...f.adults, ...f.kids]) {
      (familiesById[id] = familiesById[id] || []).push(f);
    }
  }
}

export let privacyLinks = null;
export let staleYears = null;
export let colors = null;

export function applyConfig(settings) {
  privacyLinks = settings.privacyLinks;
  staleYears = settings.staleYears;
  colors = {staff: settings.staffColor, grades: settings.gradeColors, classrooms: settings.classroomColors};
}

export function peopleOf(ids) {
  return ids.map(id => byId[id]).filter(Boolean);
}

export function viewer() {
  return byId[state.model.user.id];
}

export function allowed(allowance) {
  return state.model.allowances.includes(allowance);
}

export async function loadModel() {
  const [read, who] = await Promise.all([batch({
    settings: '/api/who-settings?include=viewer',
    people: '/api/people',
    families: '/api/families?include=adults,kids',
    classrooms: '/api/classrooms',
    grades: '/api/grades?include=room-parents',
    crews: '/api/crews?include=classroom,teachers',
    departments: '/api/departments',
    tags: '/api/tags?include=people,managers,owner',
    magicTags: '/api/magic-tags?include=people',
  }), whoAmI()]);
  const all = name => read.result[name].map(read.get);
  const settings = read.get(read.result.settings[0]);
  applyConfig(settings);
  const person = read.follow(settings, 'viewer');
  const name = person ? person.fullName : who.email;
  const families = {};
  for (const f of all('families')) {
    families[f.id] = f;
  }
  state.model = {
    user: {id: person ? person.id : '', email: who.email, name, initial: name[0].toUpperCase(), slug: person ? person.slug : who.email.split('@')[0]},
    allowances: who.allowances,
    mapsKey: settings.mapsKey,
    people: all('people'),
    families,
    classrooms: all('classrooms'),
    grades: all('grades'),
    crews: all('crews'),
    departments: all('departments').map(d => d.name),
  };
  tags = {};
  shared = {};
  for (const t of all('tags')) {
    (t.me.mine ? tags : shared)[tagKey(t.id)] = t;
  }
  lists = {};
  for (const l of all('magicTags')) {
    lists[l.key] = l;
  }
  byId = {};
  for (const p of state.model.people) {
    byId[p.id] = p;
  }
  indexFamilies();
}
