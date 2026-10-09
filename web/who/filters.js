import {state, model, peopleOf, isStudent, isParent, isStaff, familyOf, familiesOf, kidsOf, adultsOf, membersOf, gradeName, classroomName} from './state.js';
import {segments} from './dom.js';
import {el} from '/elements.js';
import {chipToggle, filterControl} from '/rules.js';
import {members, tagFacetOptions} from './tags.js';

const facets = {grade: gradeName, classroom: classroomName};

function personFacets(p, field) {
  const of = facets[field];
  if (isStudent(p)) {
    return of(p) ? [of(p)] : [];
  }
  if (isParent(p)) {
    const family = familyOf(p);
    return (family ? kidsOf(family) : []).map(of).filter(Boolean);
  }
  return [];
}

function cityOf(p) {
  const family = familyOf(p);
  if (!family || !family.address) {
    return '';
  }
  const parts = family.address.split(',').map(s => s.trim());
  return parts.length >= 2 ? parts[parts.length - 2] : parts[0];
}

export function anyFiltersActive() {
  return Boolean(state.filterGrades.size || state.filterClassrooms.size || state.filterRoles.size ||
    state.filterRoleExcluded.size || state.filterCities.size || state.filterPronouns.size ||
    state.filterTags.size);
}

function roleChipsVisible() {
  const seg = segments();
  if (seg[0] === 'email-list' || seg[0] === 'greenvelope') {
    return true;
  }
  if (seg[0] === 'people') {
    return state.filterTags.size > 0 || state.tab === 'everyone';
  }
  return false;
}

export function tagRelationOptionsFor(tag) {
  const tagged = peopleOf(members(tag));
  const hasKids = tagged.some(isStudent);
  const hasParents = tagged.some(isParent);
  const options = [];
  if (hasKids) {
    options.push('Parents');
  }
  if (hasParents) {
    options.push('Children');
  }
  if (hasKids) {
    options.push('Siblings');
  }
  for (const chosen of [...state.filterTagRelations]) {
    if (!options.includes(chosen)) {
      state.filterTagRelations.delete(chosen);
    }
  }
  return options;
}

function tagRelatedMatch(p) {
  if (state.filterTags.size !== 1 || !state.filterTagRelations.size) {
    return false;
  }
  const [tag] = state.filterTags;
  const tagged = members(tag);
  const has = list => list.some(x => tagged.includes(x.id));
  for (const family of familiesOf(p)) {
    if (state.filterTagRelations.has('Parents') && isParent(p) && has(kidsOf(family))) {
      return true;
    }
    if (state.filterTagRelations.has('Children') && isStudent(p) && has(adultsOf(family))) {
      return true;
    }
    if (state.filterTagRelations.has('Siblings') && isStudent(p) && has(kidsOf(family).filter(k => k.id !== p.id))) {
      return true;
    }
  }
  return false;
}

export function matchesFilters(p) {
  const gradeOK = !state.filterGrades.size || personFacets(p, 'grade').some(g => state.filterGrades.has(g));
  const classOK = !state.filterClassrooms.size || personFacets(p, 'classroom').some(c => state.filterClassrooms.has(c));
  const roleOK = (!state.filterRoles.size ||
    (state.filterRoles.has('Student') && isStudent(p)) ||
    (state.filterRoles.has('Parent') && isParent(p)) ||
    (state.filterRoles.has('Staff') && isStaff(p))) &&
    (!roleChipsVisible() ||
    (isStudent(p) && !state.filterRoleExcluded.has('Student')) ||
    (isParent(p) && !state.filterRoleExcluded.has('Parent')) ||
    (isStaff(p) && !state.filterRoleExcluded.has('Staff')));
  const cityOK = !state.filterCities.size || state.filterCities.has(cityOf(p));
  const pronounsOK = !state.filterPronouns.size || (p.pronouns && state.filterPronouns.has(p.pronouns.toLowerCase()));
  const tagOK = !state.filterTags.size || [...state.filterTags].some(k => members(k).includes(p.id)) || tagRelatedMatch(p);
  return gradeOK && classOK && roleOK && cityOK && pronounsOK && tagOK;
}

export function familyMatchesFilters(family) {
  return !anyFiltersActive() || membersOf(family).some(matchesFilters);
}

export function gradeOptions() {
  const present = new Set(model.people.filter(isStudent).map(gradeName).filter(Boolean));
  return model.grades.map(g => g.name).filter(n => present.has(n));
}

export function classroomOptions() {
  return model.classrooms.map(c => c.name);
}

function cityOptions() {
  return [...new Set(model.people.map(cityOf).filter(Boolean))].sort();
}

function pronounOptions() {
  return [...new Set(model.people.map(p => p.pronouns).filter(Boolean).map(p => p.toLowerCase()))].sort();
}

const roleChipFacets = [
  ['Student', 'Students'],
  ['Parent', 'Parents'],
  ['Staff', 'Staff'],
];

export function roleChips(rerender) {
  const bar = el('div', 'chip-row');
  for (const [key, label] of roleChipFacets) {
    bar.append(chipToggle(label, !state.filterRoleExcluded.has(key), on => {
      if (on) {
        state.filterRoleExcluded.delete(key);
      } else {
        state.filterRoleExcluded.add(key);
      }
      rerender();
    }, key.toLowerCase()));
  }
  return bar;
}

export function directoryFilter(rerender, options = {}) {
  const sections = [];
  if (options.role !== false) {
    sections.push({label: 'Role', values: ['Student', 'Parent', 'Staff'], chosen: state.filterRoles});
  }
  if (options.classroom !== false) {
    sections.push({label: 'Classroom', values: classroomOptions(), chosen: state.filterClassrooms});
  }
  if (options.grade !== false) {
    sections.push({label: 'Grade', values: gradeOptions(), chosen: state.filterGrades});
  }
  if (options.city !== false) {
    sections.push({label: 'City', values: cityOptions(), chosen: state.filterCities});
  }
  if (options.pronouns !== false) {
    sections.push({label: 'Pronouns', values: pronounOptions(), chosen: state.filterPronouns});
  }
  if (options.tags !== false && tagFacetOptions().length) {
    sections.push({label: 'Tags', values: tagFacetOptions(), chosen: state.filterTags});
  }
  return filterControl(sections, rerender, []);
}
