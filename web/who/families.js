import {state, byEmail, familiesByEmail} from './state.js';
import {withFrom, firstName} from './dom.js';

export function familyLink(key) {
  return withFrom('/families/' + encodeURIComponent(key));
}

export function familiesOf(p) {
  return (p && familiesByEmail[p.email]) || [];
}

export function familyOf(p) {
  return familiesOf(p)[0];
}

export function myFamilyKey() {
  const family = familyOf(byEmail[document.body.dataset.userEmail]);
  return (family && family.key) || '';
}

// Only families with at least one kid on record - a staff member with no kids
// (a Family record with no kidEmails, or no Family record at all) isn't a
// family in the school-community sense the Families tab is showing, so those
// don't get an entry here at all.
export function familyEntries() {
  return Object.values(state.model.families)
    .filter(f => (f.kidEmails || []).length)
    .map(f => {
      const members = [...(f.kidEmails || []), ...(f.adultEmails || [])];
      const kidGrades = [...new Set((f.kidEmails || []).map(e => byEmail[e]?.grade).filter(Boolean))];
      return {
        key: f.key,
        name: (f.name || '').replace(/ Family$/, ''),
        grades: kidGrades,
        members: members.map(e => byEmail[e] ? firstName(byEmail[e].fullName) : '').filter(Boolean),
        photoUrl: f.photoUrl,
        href: familyLink(f.key),
      };
    });
}

export function familySearchText(family) {
  const members = [...(family.kidEmails || []), ...(family.adultEmails || [])]
    .map(e => byEmail[e]).filter(Boolean).map(p => p.fullName);
  return `${family.name || ''} ${members.join(' ')}`.toLowerCase();
}
