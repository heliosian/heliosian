import {state, peopleOf} from './state.js';
import {segments} from './dom.js';
import {el} from '/elements.js';
import {chipToggle, filterControl} from '/rules.js';
import {familyOf, familiesOf} from './families.js';
import {members, tagFacetOptions} from './tags.js';

function personFacets(p, field) {
  if (p.isStudent) {
    return p[field] ? [p[field]] : [];
  }
  if (p.isParent) {
    const family = familyOf(p);
    return (family ? peopleOf(family.kids) : []).map(k => k[field]).filter(Boolean);
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
    state.filterTags.size || state.filterNew);
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
  const hasKids = tagged.some(p => p.isStudent);
  const hasParents = tagged.some(p => p.isParent);
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
  for (const family of familiesOf(p)) {
    if (state.filterTagRelations.has('Parents') && p.isParent &&
      family.kids.some(id => tagged.includes(id))) {
      return true;
    }
    if (state.filterTagRelations.has('Children') && p.isStudent &&
      family.adults.some(id => tagged.includes(id))) {
      return true;
    }
    if (state.filterTagRelations.has('Siblings') && p.isStudent &&
      family.kids.some(id => id !== p.id && tagged.includes(id))) {
      return true;
    }
  }
  return false;
}

export function matchesFilters(p) {
  const gradeOK = !state.filterGrades.size || personFacets(p, 'grade').some(g => state.filterGrades.has(g));
  const classOK = !state.filterClassrooms.size || personFacets(p, 'classroom').some(c => state.filterClassrooms.has(c));
  const roleOK = (!state.filterRoles.size ||
    (state.filterRoles.has('Student') && p.isStudent) ||
    (state.filterRoles.has('Parent') && p.isParent) ||
    (state.filterRoles.has('Staff') && p.isStaff)) &&
    (!roleChipsVisible() ||
    (p.isStudent && !state.filterRoleExcluded.has('Student')) ||
    (p.isParent && !state.filterRoleExcluded.has('Parent')) ||
    (p.isStaff && !state.filterRoleExcluded.has('Staff')));
  const cityOK = !state.filterCities.size || state.filterCities.has(cityOf(p));
  const pronounsOK = !state.filterPronouns.size || (p.pronouns && state.filterPronouns.has(p.pronouns.toLowerCase()));
  const newOK = !state.filterNew || p.isNew;
  const tagOK = !state.filterTags.size || [...state.filterTags].some(k => members(k).includes(p.id)) || tagRelatedMatch(p);
  return gradeOK && classOK && roleOK && cityOK && pronounsOK && newOK && tagOK;
}

export function familyMatchesFilters(key) {
  const family = state.model.families[key];
  if (!family) {
    return !anyFiltersActive();
  }
  const members = peopleOf([...family.kids, ...family.adults]);
  return !anyFiltersActive() || members.some(matchesFilters);
}

export function gradeOptions() {
  const present = new Set(state.model.people.filter(p => p.isStudent).map(p => p.grade).filter(Boolean));
  return state.model.grades.map(g => g.name).filter(n => present.has(n));
}

function cityOptions() {
  return [...new Set(state.model.people.map(cityOf).filter(Boolean))].sort();
}

function pronounOptions() {
  return [...new Set(state.model.people.map(p => p.pronouns).filter(Boolean).map(p => p.toLowerCase()))].sort();
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
    sections.push({label: 'Classroom', values: state.model.classrooms.map(c => c.name), chosen: state.filterClassrooms});
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
  const toggles = [];
  if (options.newToHelios !== false) {
    toggles.push({label: 'New to Helios', on: state.filterNew, onChange: on => {
      state.filterNew = on;
      rerender();
    }});
  }
  return filterControl(sections, rerender, toggles);
}
