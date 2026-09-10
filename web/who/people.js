import {state, byEmail, colors} from './state.js';
import {el, withFrom, thumbUrl, hue, lastName} from './dom.js';
import {familyOf} from './families.js';
import {tagControl} from './tags.js';

export function personSlug(email) {
  return (email || '').split('@')[0];
}

export function personByKey(key) {
  if (!key) {
    return undefined;
  }
  if (byEmail[key]) {
    return byEmail[key];
  }
  const lower = key.toLowerCase();
  return Object.values(byEmail).find(p => personSlug(p.email).toLowerCase() === lower);
}

export function personLink(p) {
  return withFrom('/people/' + encodeURIComponent(personSlug(p.email)));
}

// A person's own photo if they have one, else their family's - the same fallback
// the profile page hero uses, so a directory card never shows initials when a
// family photo is already available to show instead.
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
  // A lighter tint of the same hue, used for the hover ring so it always relates to
  // this specific avatar's color instead of one fixed ring color for everyone.
  div.style.setProperty('--avatar-hue', h);
  return div;
}

// Which admin-configured color a person's hover ring should use: a student gets
// their own grade's color, a parent gets one of their kids' grade colors (picked
// stably via a hash of their own email so it doesn't change from render to render),
// and staff - or anyone with no grade-band to inherit from, like a parent with no
// kids on record - falls back to the one global staff color.
function ringColorFor(p) {
  if (p.isStudent) {
    return colors.grades[p.grade] || colors.staff || null;
  }
  if (p.isStaff) {
    return colors.staff || null;
  }
  const family = familyOf(p);
  const kids = ((family && family.kidEmails) || []).map(e => byEmail[e]).filter(Boolean);
  if (kids.length) {
    const pick = kids[hue(p.email) % kids.length];
    if (colors.grades[pick.grade]) {
      return colors.grades[pick.grade];
    }
  }
  return colors.staff || null;
}

// Sets the hover-ring color (see ringColorFor) as an inline CSS variable so
// a.person-card:hover's outline can pick it up without per-page-type CSS.
export function applyRingColor(photoEl, p) {
  const color = ringColorFor(p);
  if (color) {
    photoEl.style.setProperty('--ring-color', color);
    // Initials fallbacks (a DIV; a real photo is an IMG) show a lightened tint of
    // their gradeband color instead of the generic per-name hue in photoOrInitials,
    // so a roster card's color always means "this grade" rather than "this name
    // happened to hash here." Lightened (not the full-strength color) so it reads
    // as a bit lighter than the peek-card behind it (.photo-wrap-peek, also full
    // strength via --ring-color) instead of vanishing into an identical color.
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

// roleWithPronouns is roleLabel's sentence-case counterpart, for spots that
// don't already uppercase via CSS (the profile page's own header) or don't
// want to (a family member's row, where "Parent" reads as plain text next to
// the name rather than a shouty label).
export function roleWithPronouns(p) {
  const role = baseRole(p);
  return p.pronouns ? `${role} (${formatPronouns(p.pronouns)})` : role;
}

// "She/Her" -> "she / her" - forced lowercase regardless of how the source
// data is cased (older records predate saving pronouns lowercase), spaced out
// around the slash for readability in running text.
export function formatPronouns(pronouns) {
  return pronouns.toLowerCase().split('/').join(' / ');
}

export function gradeChain(p) {
  return [p.grade, p.classroom, p.crew].filter(Boolean).join(' ▶ ');
}

function personContext(p) {
  if (p.isStudent) {
    // Grade shows as its own badge on the photo (see gradeBadge/personCard)
    // instead of leading this line, so it doesn't also get repeated here.
    return [p.classroom, p.crew].filter(Boolean).join(' ▶ ');
  }
  if (p.isStaff && p.jobTitle) {
    return p.jobTitle;
  }
  const family = familyOf(p);
  if (family) {
    return (family.kidEmails || []).map(e => byEmail[e]?.fullName).filter(Boolean).join(', ');
  }
  return '';
}

export function cardMore(email) {
  return tagControl(email, 'card-more-wrap', 'card-more', () => {});
}

// Pins the personal-tag button to the photo's own corner rather than the
// surrounding card - the card is often much wider than the photo once it's
// centered in a flexible grid column, which left the button floating in
// blank space instead of sitting on the photo. topLeft, when given, is
// another such corner overlay (see gradeBadge) pinned to the opposite side.
export function photoWithTag(photoEl, email, topLeft) {
  // photo-wrap-peek (as opposed to the plain .photo-wrap the profile/family
  // pages build directly, with no gradeband context) opts into the offset
  // peek-card behind the photo - see .photo-wrap-peek::before.
  const wrap = el('div', 'photo-wrap photo-wrap-peek');
  // Carries the gradeband color set by applyRingColor (see ringColorFor) from
  // the photo itself onto the wrap, since the offset peek-card behind the photo
  // needs it but a child's custom property isn't visible to its parent's own CSS.
  const ringColor = photoEl.style.getPropertyValue('--ring-color');
  if (ringColor) {
    wrap.style.setProperty('--peek-color', ringColor);
  }
  wrap.append(photoEl);
  if (topLeft) {
    wrap.append(topLeft);
  }
  wrap.append(cardMore(email));
  return wrap;
}

// A student's grade as a small badge pinned to the photo's top-left corner
// (see photoWithTag) instead of leading the card's subtitle line (see
// personContext) - keeps the grade visible at a glance without it pushing
// classroom/crew out into a longer line of text.
function gradeBadge(p) {
  if (!p.isStudent || !p.grade) {
    return null;
  }
  const badge = el('div', 'grade-badge', p.grade);
  const color = colors.grades[p.grade];
  if (color) {
    // Darkened rather than the raw admin color: this sits on top of an
    // arbitrary photo, so it needs to stay legible with white text even for
    // a light source color (yellow, say) that wouldn't have enough contrast
    // used directly as a background.
    badge.style.background = `color-mix(in srgb, ${color} 65%, black)`;
  }
  return badge;
}

export function personCard(p) {
  const card = el('a', 'person-card');
  card.href = personLink(p);
  card.append(photoWithTag(applyRingColor(photoOrInitials(personPhotoUrl(p), p.fullName, 'person-photo'), p), p.email, gradeBadge(p)));
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
