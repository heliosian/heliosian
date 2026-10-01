import {state, colors, peopleOf} from './state.js';
import {withFrom, thumbUrl, hue, lastName} from './dom.js';
import {el} from '/elements.js';
import {familyOf} from './families.js';
import {tagControl} from './tags.js';

export function personByKey(key) {
  if (!key) {
    return undefined;
  }
  const lower = key.toLowerCase();
  return state.model.people.find(p => p.email === lower) || state.model.people.find(p => p.slug.toLowerCase() === lower);
}

export function personLink(p) {
  if (p.guest) {
    return withFrom('/people/' + encodeURIComponent('guest:' + p.guest.id));
  }
  return withFrom(p.path);
}

export function guestPerson(g) {
  return {fullName: g.name, email: g.email || '', guest: g};
}

export function guestCard(g) {
  const card = el('a', 'person-card');
  card.href = personLink(guestPerson(g));
  const wrap = el('div', 'photo-wrap photo-wrap-peek');
  wrap.append(photoOrInitials('', g.name, 'person-photo'));
  card.append(wrap);
  card.append(el('div', 'role-label role-label-guest', 'Guest'));
  card.append(el('div', 'person-name', g.name));
  if (g.purchaserName) {
    card.append(el('div', 'person-sub', 'Guest of ' + g.purchaserName));
  }
  return card;
}

export function personPhotoUrl(p) {
  const family = familyOf(p);
  return p.photoUrl || (family && family.photoUrl) || '';
}

export function photoOrInitials(url, name, className) {
  if (url) {
    const img = el('img', className);
    img.src = thumbUrl(url);
    img.loading = 'lazy';
    img.alt = '';
    return img;
  }
  const div = el('div', className, name.trim().split(/\s+/).filter(w => /\p{L}/u.test(w[0])).map(w => w[0]).slice(0, 2).join(''));
  const h = hue(name);
  div.style.background = `hsl(${h} 45% 55%)`;
  div.style.setProperty('--avatar-hue', h);
  return div;
}

function ringColorFor(p) {
  if (p.isStudent) {
    return colors.grades[p.grade] || colors.staff || null;
  }
  if (p.isStaff) {
    return colors.staff || null;
  }
  const family = familyOf(p);
  const kids = family ? peopleOf(family.kids) : [];
  if (kids.length) {
    const pick = kids[hue(p.email || p.id) % kids.length];
    if (colors.grades[pick.grade]) {
      return colors.grades[pick.grade];
    }
  }
  return colors.staff || null;
}

export function applyRingColor(photoEl, p) {
  const color = ringColorFor(p);
  if (color) {
    photoEl.style.setProperty('--ring-color', color);
    if (photoEl.tagName === 'DIV') {
      photoEl.style.background = `color-mix(in srgb, ${color} 65%, white)`;
    }
  }
  return photoEl;
}

export function baseRole(p) {
  if (p.isStudent) {
    return 'Student';
  }
  if (p.isStaff) {
    return 'Staff';
  }
  return 'Parent';
}

export function roleLabel(p) {
  const role = baseRole(p);
  return (p.pronouns ? `${role} (${p.pronouns})` : role).toUpperCase();
}

export function roleWithPronouns(p) {
  const role = baseRole(p);
  return p.pronouns ? `${role} (${formatPronouns(p.pronouns)})` : role;
}

export function formatPronouns(pronouns) {
  return pronouns.toLowerCase().split('/').join(' / ');
}

export function gradeChain(p) {
  return [p.grade, p.classroom, p.crew].filter(Boolean).join(' ▶ ');
}

function personContext(p) {
  if (p.isStudent) {
    return [p.classroom, p.crew].filter(Boolean).join(' ▶ ');
  }
  if (p.isStaff && p.jobTitle) {
    return p.jobTitle;
  }
  const family = familyOf(p);
  if (family) {
    return peopleOf(family.kids).map(k => k.fullName).filter(Boolean).join(', ');
  }
  return '';
}

export function cardMore(id) {
  return tagControl(id, 'card-more-wrap', 'card-more', () => {});
}

export function photoWithTag(photoEl, id, topLeft) {
  const wrap = el('div', 'photo-wrap photo-wrap-peek');
  const ringColor = photoEl.style.getPropertyValue('--ring-color');
  if (ringColor) {
    wrap.style.setProperty('--peek-color', ringColor);
  }
  wrap.append(photoEl);
  if (topLeft) {
    wrap.append(topLeft);
  }
  wrap.append(cardMore(id));
  return wrap;
}

function gradeBadge(p) {
  if (!p.isStudent || !p.grade) {
    return null;
  }
  const badge = el('div', 'grade-badge', p.grade);
  const color = colors.grades[p.grade];
  if (color) {
    badge.style.background = `color-mix(in srgb, ${color} 65%, black)`;
  }
  return badge;
}

export function personCard(p) {
  const card = el('a', 'person-card');
  card.href = personLink(p);
  card.append(photoWithTag(applyRingColor(photoOrInitials(personPhotoUrl(p), p.fullName, 'person-photo'), p), p.id, gradeBadge(p)));
  card.append(el('div', 'role-label role-label-' + baseRole(p).toLowerCase(), baseRole(p)));
  card.append(el('div', 'person-name', p.fullName));
  const context = personContext(p);
  if (context) {
    card.append(el('div', 'person-sub', context));
  }
  return card;
}

export function sortPeople(list) {
  const copy = [...list];
  copy.sort((a, b) => lastName(a.fullName).localeCompare(lastName(b.fullName)) || a.fullName.localeCompare(b.fullName));
  return copy;
}
