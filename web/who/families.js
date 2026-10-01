import {state, familiesById, peopleOf, viewer} from './state.js';
import {withFrom, firstName} from './dom.js';

export function familyLink(key) {
  return withFrom('/families/' + encodeURIComponent(key));
}

export function familiesOf(p) {
  return (p && familiesById[p.id]) || [];
}

export function familyOf(p) {
  return familiesOf(p)[0];
}

export function myFamilyKey() {
  const family = familyOf(viewer());
  return (family && family.id) || '';
}

export function familyEntries() {
  return Object.values(state.model.families)
    .filter(f => f.kids.length)
    .map(f => {
      const members = peopleOf([...f.kids, ...f.adults]);
      const kidGrades = [...new Set(peopleOf(f.kids).map(k => k.grade).filter(Boolean))];
      return {
        key: f.id,
        name: f.shortName || '',
        grades: kidGrades,
        members: members.map(p => firstName(p.fullName)).filter(Boolean),
        photoUrl: f.photoUrl,
        href: familyLink(f.id),
      };
    });
}

export function familySearchText(family) {
  const members = peopleOf([...family.kids, ...family.adults]).map(p => p.fullName);
  return `${family.name || ''} ${members.join(' ')}`.toLowerCase();
}
