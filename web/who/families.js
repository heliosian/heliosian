import {model, viewer, familyOf, membersOf, kidsOf, gradeName, familyShortName, thumbOf} from './state.js';
import {withFrom, firstName} from './dom.js';
import {familyPhoto} from './people.js';

export function familyPath(family) {
  return '/families/' + encodeURIComponent(family.id);
}

export function familyLink(family) {
  return withFrom(familyPath(family));
}

export function myFamily() {
  return familyOf(viewer());
}

export function familyEntries() {
  return model.families
    .filter(f => kidsOf(f).length)
    .map(f => ({
      family: f,
      name: familyShortName(f),
      grades: [...new Set(kidsOf(f).map(gradeName).filter(Boolean))],
      members: membersOf(f).map(p => firstName(p.name_show)).filter(Boolean),
      photoUrl: thumbOf(familyPhoto(f)),
      href: familyLink(f),
    }));
}

export function familySearchText(family) {
  return `${family.name || ''} ${membersOf(family).map(p => p.name_show).join(' ')}`.toLowerCase();
}
