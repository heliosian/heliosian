import {byId, bySlug, photosOf, thumbOf, photoUrl, emailOf, isStudent, isStaff, familyOf, kidsOf, gradeOf, gradeName, classroomName, crewName, staffColor} from './state.js';
import {withFrom, hue} from './dom.js';
import {el} from '/elements.js';
import {tagControl} from './tags.js';

export function personByKey(key) {
  return key ? bySlug[key.toLowerCase()] || byId[key] : undefined;
}

export function personPath(p) {
  return '/people/' + encodeURIComponent(p.slug || p.id);
}

export function personLink(p) {
  return withFrom(personPath(p));
}

export function guestCard(g) {
  const card = el('a', 'person-card');
  card.href = personLink(g);
  const wrap = el('div', 'photo-wrap photo-wrap-peek');
  wrap.append(photoOrInitials('', g.name_show, 'person-photo'));
  card.append(wrap);
  card.append(el('div', 'role-label role-label-guest', 'Guest'));
  card.append(el('div', 'person-name', g.name_show));
  return card;
}

export function familyPhoto(family) {
  return family ? photosOf(family.id)[0] : undefined;
}

export function personPhoto(p) {
  return photosOf(p.id)[0] || familyPhoto(familyOf(p));
}

export function personPhotoUrl(p) {
  return thumbOf(personPhoto(p));
}

export function personImageUrl(p) {
  return photoUrl(photosOf(p.id)[0]);
}

export function photoOrInitials(url, name, className) {
  if (url) {
    const img = el('img', className);
    img.src = url;
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

export function gradeColor(p) {
  const g = gradeOf(p);
  return g ? g.color || '' : '';
}

function ringColorFor(p) {
  if (isStudent(p)) {
    return gradeColor(p) || staffColor() || null;
  }
  if (isStaff(p)) {
    return staffColor() || null;
  }
  const family = familyOf(p);
  const kids = family ? kidsOf(family) : [];
  if (kids.length) {
    const pick = kids[hue(emailOf(p) || p.id) % kids.length];
    if (gradeColor(pick)) {
      return gradeColor(pick);
    }
  }
  return staffColor() || null;
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
  if (p.source === 'guest') {
    return 'Guest';
  }
  if (isStudent(p)) {
    return 'Student';
  }
  if (isStaff(p)) {
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
  return [gradeName(p), classroomName(p), crewName(p)].filter(Boolean).join(' ▶ ');
}

function personContext(p) {
  if (isStudent(p)) {
    return [classroomName(p), crewName(p)].filter(Boolean).join(' ▶ ');
  }
  if (isStaff(p) && p.job_title) {
    return p.job_title;
  }
  const family = familyOf(p);
  if (family) {
    return kidsOf(family).map(k => k.name_show).join(', ');
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
  if (!isStudent(p) || !gradeName(p)) {
    return null;
  }
  const badge = el('div', 'grade-badge', gradeName(p));
  const color = gradeColor(p);
  if (color) {
    badge.style.background = `color-mix(in srgb, ${color} 65%, black)`;
  }
  return badge;
}

export function personCard(p) {
  if (p.source === 'guest') {
    return guestCard(p);
  }
  const card = el('a', 'person-card');
  card.href = personLink(p);
  card.append(photoWithTag(applyRingColor(photoOrInitials(personPhotoUrl(p), p.name_show, 'person-photo'), p), p.id, gradeBadge(p)));
  card.append(el('div', 'role-label role-label-' + baseRole(p).toLowerCase(), baseRole(p)));
  card.append(el('div', 'person-name', p.name_show));
  const context = personContext(p);
  if (context) {
    card.append(el('div', 'person-sub', context));
  }
  return card;
}

export function sortPeople(list) {
  const copy = [...list];
  copy.sort((a, b) => (a.name_sort || '').localeCompare(b.name_sort || '') || a.name_show.localeCompare(b.name_show));
  return copy;
}
