function loadNavOpen() {
  try {
    const raw = localStorage.getItem('navOpen');
    if (raw) {
      return JSON.parse(raw);
    }
  } catch (e) {
    // ignore, fall through to defaults
  }
  return {directory: true, family: false, tools: true};
}

function saveNavOpen(navOpen) {
  try {
    localStorage.setItem('navOpen', JSON.stringify(navOpen));
  } catch (e) {
    // ignore
  }
}

const state = {model: null, tab: 'everyone', classTab: 'by-classroom', rosterTab: 'students', rosterSectionExcluded: new Set(), q: '', filterGrades: new Set(), filterClassrooms: new Set(), filterRoles: new Set(), filterRoleExcluded: new Set(), filterCities: new Set(), filterPronouns: new Set(), filterTags: new Set(), filterNew: false, navOpen: loadNavOpen()};
let byEmail = {};
let tags = {};

function tagNames() {
  return Object.keys(tags).sort((a, b) => a.localeCompare(b));
}

function tagsOf(email) {
  return tagNames().filter(name => tags[name].includes(email));
}

function isTagged(email) {
  return tagNames().some(name => tags[name].includes(email));
}

async function setTag(email, tag, on) {
  const people = tags[tag] || [];
  if (on) {
    tags[tag] = people.includes(email) ? people : [...people, email];
  } else {
    tags[tag] = people.filter(e => e !== email);
    if (!tags[tag].length) {
      delete tags[tag];
    }
  }
  const form = new FormData();
  form.append('person', email);
  form.append('tag', tag);
  form.append('on', on ? '1' : '0');
  const res = await fetch('/api/directory/tag', {method: 'POST', body: form});
  if (!res.ok) {
    alert(await res.text());
  }
}

const icons = {
  people: '<svg viewBox="0 0 24 24"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
  classrooms: '<svg viewBox="0 0 24 24"><path d="M4 10a4 4 0 0 1 4-4h8a4 4 0 0 1 4 4v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2Z"/><path d="M9 6V4a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2"/><path d="M8 21v-5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v5"/><path d="M8 10h8"/></svg>',
  'my-family': '<svg viewBox="0 0 24 24"><path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z"/></svg>',
  staff: '<svg viewBox="0 0 24 24"><path d="M12 20.94c1.5 0 2.75 1.06 4 1.06 3 0 6-8 6-12.22A4.91 4.91 0 0 0 17 5c-2.22 0-4 1.44-5 2-1-.56-2.78-2-5-2a4.9 4.9 0 0 0-5 4.78C2 14 5 22 8 22c1.25 0 2.5-1.06 4-1.06Z"/><path d="M10 2c1 .5 2 2 2 5"/></svg>',
  map: '<svg viewBox="0 0 24 24"><path d="M20 10c0 4.993-5.539 10.193-7.399 11.799a1 1 0 0 1-1.202 0C9.539 20.193 4 14.993 4 10a8 8 0 0 1 16 0"/><circle cx="12" cy="10" r="3"/></svg>',
  'email-list': '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8"/></svg>',
  everyone: '<svg viewBox="0 0 24 24"><rect width="7" height="7" x="3" y="3" rx="1"/><rect width="7" height="7" x="14" y="3" rx="1"/><rect width="7" height="7" x="14" y="14" rx="1"/><rect width="7" height="7" x="3" y="14" rx="1"/></svg>',
  students: '<svg viewBox="0 0 24 24"><path d="M4 10a4 4 0 0 1 4-4h8a4 4 0 0 1 4 4v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2Z"/><path d="M9 6V4a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2"/><path d="M8 21v-5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v5"/><path d="M8 10h8"/></svg>',
  families: '<svg viewBox="0 0 24 24"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
  'staff-tab': '<svg viewBox="0 0 24 24"><path d="M12 20.94c1.5 0 2.75 1.06 4 1.06 3 0 6-8 6-12.22A4.91 4.91 0 0 0 17 5c-2.22 0-4 1.44-5 2-1-.56-2.78-2-5-2a4.9 4.9 0 0 0-5 4.78C2 14 5 22 8 22c1.25 0 2.5-1.06 4-1.06Z"/><path d="M10 2c1 .5 2 2 2 5"/></svg>',
  search: '<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>',
  filter: '<svg viewBox="0 0 24 24"><path d="M5 7h14M8 12h8M10.5 17h3"/></svg>',
  chevron: '<svg viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></svg>',
  'chevron-left': '<svg viewBox="0 0 24 24"><path d="m15 18-6-6 6-6"/></svg>',
  'chevron-right': '<svg viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></svg>',
  tag: '<svg viewBox="0 0 24 24"><path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z"/><circle cx="7.5" cy="7.5" r=".5"/></svg>',
  mail: '<svg viewBox="0 0 24 24"><rect width="20" height="16" x="2" y="4" rx="2"/><path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7"/></svg>',
  copy: '<svg viewBox="0 0 24 24"><rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>',
  check: '<svg viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5"/></svg>',
  download: '<svg viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/></svg>',
  message: '<svg viewBox="0 0 24 24"><path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/></svg>',
  phone: '<svg viewBox="0 0 24 24"><path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72 12.84 12.84 0 0 0 .7 2.81 2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45 12.84 12.84 0 0 0 2.81.7A2 2 0 0 1 22 16.92z"/></svg>',
  zap: '<svg viewBox="0 0 24 24"><path d="M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z"/></svg>',
  more: '<svg viewBox="0 0 24 24"><path d="M4 7h16M4 12h16M4 17h10"/></svg>',
  camera: '<svg viewBox="0 0 24 24"><path d="M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z"/><circle cx="12" cy="13" r="3"/></svg>',
  mic: '<svg viewBox="0 0 24 24"><path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" x2="12" y1="19" y2="22"/></svg>',
  upload: '<svg viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" x2="12" y1="3" y2="15"/></svg>',
  pencil: '<svg viewBox="0 0 24 24"><path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z"/></svg>',
  alert: '<svg viewBox="0 0 24 24"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/><line x1="12" x2="12" y1="9" y2="13"/><line x1="12" x2="12.01" y1="17" y2="17"/></svg>',
  sync: '<svg viewBox="0 0 24 24"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/></svg>',
  lock: '<svg viewBox="0 0 24 24"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>',
  gear: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>',
  volume: '<svg viewBox="0 0 24 24"><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/><path d="M15.54 8.46a5 5 0 0 1 0 7.07"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/></svg>',
};

function isMobile() {
  return matchMedia('(max-width: 900px)').matches;
}

const primaryNavItems = [
  {path: 'people', label: 'Directory'},
  {path: 'classrooms', label: 'Classrooms'},
  {path: 'staff', label: 'Staff'},
];

const toolsNavItems = [
  {path: 'map', label: 'Map'},
  {path: 'email-list', label: 'Email List'},
];

const mobileNavSections = [
  {path: 'people', label: 'Directory'},
  {path: 'classrooms', label: 'Classrooms'},
  {path: 'staff', label: 'Staff'},
  {path: 'my-family', label: 'My Family'},
  {path: 'email-list', label: 'Email List'},
];

const peopleTabs = [
  {key: 'everyone', label: 'Everyone'},
  {key: 'families', label: 'Families'},
];

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text) {
    node.textContent = text;
  }
  return node;
}

function svg(name) {
  const holder = document.createElement('template');
  holder.innerHTML = icons[name];
  return holder.content.firstChild;
}

function segments() {
  return location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
}

function withFrom(href) {
  const from = encodeURIComponent(location.pathname + location.search);
  return href + (href.includes('?') ? '&' : '?') + 'from=' + from;
}

function personSlug(email) {
  return (email || '').split('@')[0];
}

function personByKey(key) {
  if (!key) {
    return undefined;
  }
  if (byEmail[key]) {
    return byEmail[key];
  }
  const lower = key.toLowerCase();
  return Object.values(byEmail).find(p => personSlug(p.email).toLowerCase() === lower);
}

function personLink(p) {
  return withFrom('/people/' + encodeURIComponent(personSlug(p.email)));
}

function familyLink(key) {
  return withFrom('/families/' + encodeURIComponent(key));
}

// The search dropdown's second line: a student's grade chain as before, a staff
// member's job title, and a parent's "Parent to Leo (Grade 1)" instead of the
// generic role label, since who someone is a parent OF is more useful here than the
// fact that they're a parent.
function personSearchSubtitle(p) {
  if (p.isStudent) {
    return gradeChain(p);
  }
  if (p.isStaff) {
    return p.jobTitle || roleLabel(p);
  }
  const family = state.model.families[p.familyKey];
  const kids = ((family && family.kidEmails) || []).map(e => byEmail[e]).filter(Boolean);
  if (kids.length) {
    const names = kids.map(k => firstName(k.fullName)).join(', ');
    const grades = [...new Set(kids.map(k => k.grade).filter(Boolean))].join(', ');
    return `Parent to ${names}${grades ? ` (${grades})` : ''}`;
  }
  return roleLabel(p);
}

// Shared by the desktop topbar search and the mobile search overlay - both just
// point a different results container at this. People/grades/classrooms are all
// loaded client-side already (state.model), so this is a plain client-side filter
// rather than a server round trip.
function renderGlobalSearchResults(resultsEl, query) {
  const q = query.trim().toLowerCase();
  resultsEl.replaceChildren();
  if (!q || !state.model) {
    resultsEl.hidden = true;
    return;
  }
  const people = state.model.people
    .filter(p => p.fullName.toLowerCase().includes(q) || p.email.toLowerCase().includes(q))
    .slice(0, 8);
  const grades = state.model.grades
    .filter(g => g.name.toLowerCase().includes(q))
    .slice(0, 5);
  const classrooms = state.model.classrooms
    .filter(c => c.name.toLowerCase().includes(q))
    .slice(0, 5);

  if (!people.length && !grades.length && !classrooms.length) {
    resultsEl.append(el('div', 'gsearch-empty', 'No matches.'));
    resultsEl.hidden = false;
    return;
  }

  function group(label, items, buildRow) {
    if (!items.length) {
      return;
    }
    resultsEl.append(el('div', 'gsearch-label', label));
    for (const item of items) {
      resultsEl.append(buildRow(item));
    }
  }

  group('People', people, p => {
    const row = el('a', 'gsearch-result');
    row.href = personLink(p);
    row.append(photoOrInitials(p.photoUrl, p.fullName, 'gsearch-avatar'));
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', p.fullName));
    const sub = personSearchSubtitle(p);
    if (sub) {
      info.append(el('div', 'gsearch-sub', sub));
    }
    row.append(info);
    return row;
  });

  group('Grades', grades, g => {
    const row = el('a', 'gsearch-result');
    row.href = withFrom('/grades/' + slugify(g.name));
    const avatar = el('div', 'gsearch-avatar gsearch-avatar-icon');
    avatar.append(svg('students'));
    row.append(avatar);
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', g.name));
    row.append(info);
    return row;
  });

  group('Classrooms', classrooms, c => {
    const row = el('a', 'gsearch-result');
    row.href = withFrom('/classrooms/' + slugify(c.name));
    const avatar = el('div', 'gsearch-avatar gsearch-avatar-icon');
    avatar.append(svg('classrooms'));
    row.append(avatar);
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', c.name));
    row.append(info);
    return row;
  });

  resultsEl.hidden = false;
}

function myFamilyKey() {
  const me = byEmail[document.body.dataset.userEmail];
  return (me && me.familyKey) || '';
}

function activeSection() {
  const seg = segments();
  if (seg[0] === 'families') {
    return seg[1] === myFamilyKey() ? 'my-family' : 'people';
  }
  if (seg[0] === 'grades') {
    return 'classrooms';
  }
  return seg[0];
}

function setChrome(title, backHref) {
  document.querySelector('#mobile-title').textContent = title;
  const back = document.querySelector('#mobile-back');
  const menuBtn = document.querySelector('#mobile-menu-btn');
  back.hidden = !backHref;
  menuBtn.hidden = Boolean(backHref);
  if (backHref) {
    back.href = backHref;
  }
}

// Defaults for sample mode / a stale cached page; the model's own privacyLinks
// (admin-editable, since both URLs belong to other systems this app doesn't control)
// overwrite these once it loads - see `load()`.
let privacyLinks = {
  veracrossPreferences: 'https://portals.veracross.com/heliosschool/parent/directory-preferences',
  heliosWhoOptIn: 'https://hca.run/optin',
};

// onDismiss, if given, adds a small close button that removes the banner and fires the
// callback - the caller decides what "dismissed" means (e.g. persisting it), not this.
function infoBanner(kind, iconName, title, desc, buttonLabel, buttonHref, external, onDismiss) {
  const wrap = el('div', 'container infobanner-wrap');
  const card = el('div', `infobanner infobanner-${kind}`);

  const main = el('div', 'infobanner-main');
  const iconBadge = el('div', 'infobanner-icon');
  iconBadge.append(svg(iconName));
  main.append(iconBadge);
  const body = el('div', 'infobanner-body');
  body.append(el('div', 'infobanner-title', title));
  body.append(el('div', 'infobanner-desc', desc));
  main.append(body);
  card.append(main);

  const action = el('a', 'infobanner-button');
  action.href = buttonHref;
  if (external) {
    action.target = '_blank';
    action.rel = 'noopener';
  }
  action.append(el('span', '', buttonLabel), svg('chevron-right'));
  card.append(action);

  if (onDismiss) {
    const close = el('button', 'infobanner-close', '×');
    close.type = 'button';
    close.setAttribute('aria-label', 'Dismiss');
    close.addEventListener('click', () => {
      wrap.remove();
      onDismiss();
    });
    card.append(close);
  }

  wrap.append(card);
  return wrap;
}

let staleYears = {photo: 0.75, facts: 0.6, familyPhoto: 1.5};

function agedPast(present, updated, years) {
  if (!present) {
    return false;
  }
  const when = Date.parse(updated);
  return Number.isNaN(when) || Date.now() - when > years * 365.25 * 24 * 60 * 60 * 1000;
}

function monthYear(dateStr) {
  const when = Date.parse(dateStr);
  if (Number.isNaN(when)) {
    return '';
  }
  return new Date(when).toLocaleDateString('en-US', {month: 'long', year: 'numeric'});
}

function photoNeedsUpdate(p) {
  return p.isStudent && (!p.photoUrl || agedPast(p.photoUrl, p.photoUpdated, staleYears.photo));
}

function factsNeedUpdate(p) {
  return p.isStudent && (!p.facts || agedPast(p.facts, p.factsUpdated, staleYears.facts));
}

// Unlike a person's own photo, which falls back to the family photo when missing, the
// family photo has no further fallback: a missing one needs updating just as much as a
// stale one.
function familyPhotoNeedsUpdate(family) {
  return !family.photoUrl || agedPast(family.photoUrl, family.photoUpdated, staleYears.familyPhoto);
}

function staleItems() {
  const me = byEmail[document.body.dataset.userEmail];
  if (!me) {
    return [];
  }
  const family = state.model.families[me.familyKey];
  const items = [];
  for (const p of familyNavPeople()) {
    const whose = p.email === me.email ? 'your' : `${p.fullName}'s`;
    const shortWhose = p.email === me.email ? 'your' : `${firstName(p.fullName)}'s`;
    if (photoNeedsUpdate(p)) {
      items.push({type: 'photo', target: 'person', key: p.email, text: `Update ${whose} photo for new year`, label: `${shortWhose} photo`, person: p});
    }
    if (factsNeedUpdate(p)) {
      items.push({type: 'facts', target: 'person', key: p.email, text: `Update ${whose} facts for new year`, label: `${shortWhose} facts`, person: p});
    }
  }
  if (family && familyPhotoNeedsUpdate(family)) {
    items.push({type: 'photo', target: 'family', key: family.key, text: 'Update your family photo for new year', label: 'your family photo'});
  }
  return items;
}

// The banner's description names the actual items ("Sam's photo, Ella's photo…")
// rather than just a count, so it's useful at a glance without opening My Family.
function familyInfoBanner(items) {
  const count = items.length;
  const title = `${count} Update${count === 1 ? '' : 's'} Needed`;
  const desc = items.map(i => i.label).join(', ');
  return infoBanner('alert', 'alert', title, desc, 'Update Family Info', '/my-family', false);
}

function todoPhotoRow(item) {
  const row = el('label', 'todo-row');
  row.append(el('div', 'todo-mark'));
  row.append(el('div', 'todo-text', item.text));
  const status = el('div', 'todo-status');
  const input = el('input');
  input.type = 'file';
  input.accept = 'image/*';
  input.hidden = true;
  input.addEventListener('change', () => {
    if (input.files.length) {
      row.classList.add('todo-row-busy');
      submitMedia(item.target, item.key, 'photo', input.files[0], input.files[0].name, status);
    }
  });
  row.append(input, status);
  const action = el('div', 'todo-chevron');
  action.append(svg('camera'));
  row.append(action);
  return row;
}

function todoFactsRow(item) {
  const row = el('a', 'todo-row');
  row.href = withFrom(`/people/${encodeURIComponent(personSlug(item.key))}?edit=1&focus=facts`);
  row.append(el('div', 'todo-mark'));
  row.append(el('div', 'todo-text', item.text));
  const chev = el('div', 'todo-chevron');
  chev.append(svg('chevron-right'));
  row.append(chev);
  return row;
}

function todoChecklist(items) {
  const card = el('div', 'todo-card');
  card.append(el('div', 'todo-card-title', `${items.length} thing${items.length === 1 ? '' : 's'} to update for the new year`));
  for (const item of items) {
    card.append(item.type === 'photo' ? todoPhotoRow(item) : todoFactsRow(item));
  }
  return card;
}

function resetMain(...children) {
  const main = document.querySelector('#main');
  main.replaceChildren();
  const seg = segments();
  // My Privacy already shows its own, more detailed version of this per field, so the
  // summary card here would just repeat what's right below it on that page.
  if (seg[0] !== 'my-privacy' && !privacyMismatchCardDismissed()) {
    const warnings = myPrivacyWarnings();
    if (warnings.length) {
      main.append(privacyMismatchCard(warnings));
    }
  }
  const onOwnFamilyPage = seg[0] === 'families' && seg[1] === myFamilyKey();
  const familyEmails = new Set(familyNavPeople().map(fp => fp.email));
  const segPerson = seg[0] === 'people' && seg[1] ? personByKey(seg[1]) : undefined;
  const onOwnFamilyMemberPage = !!segPerson && familyEmails.has(segPerson.email);
  if (!onOwnFamilyPage && !onOwnFamilyMemberPage && seg[0] !== 'my-privacy') {
    const stale = staleItems();
    if (stale.length) {
      main.append(familyInfoBanner(stale));
    }
  }
  main.append(...children);
  return main;
}

function familyNavPeople() {
  const me = byEmail[document.body.dataset.userEmail];
  if (!me || (me.isStaff && !me.isParent)) {
    return [];
  }
  const family = state.model.families[me.familyKey];
  const emails = [me.email, ...((family && family.adultEmails) || []), ...((family && family.kidEmails) || [])];
  const seen = new Set();
  const people = [];
  for (const email of emails) {
    if (seen.has(email)) {
      continue;
    }
    seen.add(email);
    const p = byEmail[email];
    if (p) {
      people.push(p);
    }
  }
  return people;
}

function personTodoCount(p) {
  return (photoNeedsUpdate(p) ? 1 : 0) + (factsNeedUpdate(p) ? 1 : 0);
}

function navBadge(count) {
  return el('span', 'nav-badge', String(count));
}

function familyMemberRow(p, meEmail, activeEmail) {
  const a = el('a', 'nav-family-link');
  a.href = personLink(p);
  if (p.email === activeEmail) {
    a.className = 'nav-family-link active';
  }
  a.append(photoOrInitials(p.photoUrl, p.fullName, 'nav-family-avatar'));
  a.append(el('span', 'nav-family-name', p.email === meEmail ? 'Me' : firstName(p.fullName)));
  const count = personTodoCount(p);
  if (count) {
    const badge = navBadge(count);
    badge.title = `${p.email === meEmail ? 'You have' : `${firstName(p.fullName)} has`} ${count} thing${count === 1 ? '' : 's'} to update`;
    a.append(badge);
  }
  return a;
}

function renderNav() {
  const seg = activeSection();
  const rawSeg = segments();
  const me = byEmail[document.body.dataset.userEmail];
  const familyPeople = familyNavPeople();
  const familyEmails = new Set(familyPeople.map(p => p.email));
  const rawSegPerson = rawSeg[0] === 'people' && rawSeg[1] ? personByKey(rawSeg[1]) : undefined;
  const onFamilyMember = !!rawSegPerson && familyEmails.has(rawSegPerson.email);

  function renderItem(container, item, indicator) {
    const a = el('a');
    a.href = '/' + item.path;
    if (item.path === seg && !(item.path === 'people' && onFamilyMember)) {
      a.className = 'active';
    }
    const icon = svg(item.path);
    icon.classList.add('nav-icon-' + item.path);
    a.append(icon, el('span', '', item.label));
    if (indicator === 'alert') {
      const alert = el('span', 'nav-item-alert');
      alert.title = 'Some family info is missing or out of date';
      alert.append(svg('alert'));
      a.append(alert);
    } else if (indicator) {
      a.append(navBadge(indicator));
    }
    container.append(a);
  }

  function buildNavInto(nav) {
    nav.replaceChildren();

    function sectionHeading(key, title, icon, indicator, forceOpen) {
      const open = state.navOpen[key] || forceOpen;
      const heading = el('div', 'nav-heading nav-heading-toggle' + (open ? ' open' : ''));
      const chevron = el('span', 'nav-chevron');
      chevron.append(svg('chevron'));
      const headingIcon = svg(icon);
      headingIcon.classList.add('nav-heading-icon-' + icon);
      heading.append(chevron, headingIcon, el('span', 'nav-heading-title', title));
      if (indicator === 'alert') {
        const alert = el('span', 'nav-heading-alert');
        alert.title = 'Some family info is missing or out of date';
        alert.append(svg('alert'));
        heading.append(alert);
      } else if (indicator) {
        heading.append(navBadge(indicator));
      }
      heading.addEventListener('click', () => {
        state.navOpen[key] = !state.navOpen[key];
        saveNavOpen(state.navOpen);
        renderNav();
      });
      nav.append(heading);
      if (!open) {
        return null;
      }
      const body = el('div', 'nav-section-body');
      nav.append(body);
      return body;
    }

    const directoryBody = sectionHeading('directory', 'Directory', 'people', 0, false);
    if (directoryBody) {
      for (const item of primaryNavItems) {
        renderItem(directoryBody, item);
      }
    }

    if (familyPeople.length) {
      const todos = staleItems();
      const familyTodos = todos.filter(i => i.target === 'family').length;
      const familyBody = sectionHeading('family', 'My Family', 'families', todos.length, onFamilyMember);
      if (familyBody) {
        renderItem(familyBody, {path: 'my-family', label: 'My Family'}, familyTodos || (todos.length > familyTodos && 'alert'));
        for (const p of familyPeople) {
          familyBody.append(familyMemberRow(p, me.email, onFamilyMember ? rawSegPerson.email : null));
        }
      }
    }

    const toolsBody = sectionHeading('tools', 'Tools', 'gear', 0, false);
    if (toolsBody) {
      for (const item of toolsNavItems) {
        renderItem(toolsBody, item);
      }
    }
  }

  buildNavInto(document.querySelector('#nav'));
  const drawerNav = document.querySelector('#drawer-nav');
  if (drawerNav) {
    buildNavInto(drawerNav);
  }

  const tabs = document.querySelector('#mobile-tabs');
  tabs.replaceChildren();
  for (const item of mobileNavSections) {
    if (item.path === 'my-family' && !familyPeople.length) {
      continue;
    }
    const a = el('a', item.path === seg ? 'active' : '');
    a.href = '/' + item.path;
    a.append(svg(item.path), el('span', '', item.label));
    tabs.append(a);
  }
}

function hue(text) {
  let h = 0;
  for (const c of text) {
    h = (h * 31 + c.codePointAt(0)) % 360;
  }
  return h;
}

function firstName(fullName) {
  return fullName.trim().split(/\s+/)[0];
}

// Fisher-Yates, returning a new array so callers can shuffle once at load and
// keep that order stable across re-renders (typing in search shouldn't also
// reshuffle everything still on screen) - a fresh page load reshuffles again.
function shuffled(items) {
  const copy = [...items];
  for (let i = copy.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [copy[i], copy[j]] = [copy[j], copy[i]];
  }
  return copy;
}

function thumbUrl(url) {
  return url ? url + '?thumb=1' : url;
}

function photoOrInitials(url, name, className) {
  if (url) {
    const img = el('img', className);
    img.src = thumbUrl(url);
    img.loading = 'lazy';
    img.alt = '';
    return img;
  }
  const div = el('div', className, name.trim().split(/\s+/).map(w => w[0]).slice(0, 2).join(''));
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
    const grade = state.model.grades.find(g => g.name === p.grade);
    return (grade && grade.color) || state.model.staffColor || null;
  }
  if (p.isStaff) {
    return state.model.staffColor || null;
  }
  const family = state.model.families[p.familyKey];
  const kids = ((family && family.kidEmails) || []).map(e => byEmail[e]).filter(Boolean);
  if (kids.length) {
    const pick = kids[hue(p.email) % kids.length];
    const grade = state.model.grades.find(g => g.name === pick.grade);
    if (grade && grade.color) {
      return grade.color;
    }
  }
  return state.model.staffColor || null;
}

// Sets the hover-ring color (see ringColorFor) as an inline CSS variable so
// a.person-card:hover's outline can pick it up without per-page-type CSS.
function applyRingColor(photoEl, p) {
  const color = ringColorFor(p);
  if (color) {
    photoEl.style.setProperty('--ring-color', color);
  }
  return photoEl;
}

function baseRole(p) {
  if (p.isStudent) {
    return 'Student';
  }
  if (p.isStaff) {
    return 'Staff';
  }
  return 'Parent';
}

function roleLabel(p) {
  const role = baseRole(p);
  return (p.pronouns ? `${role} (${p.pronouns})` : role).toUpperCase();
}

function gradeChain(p) {
  return [p.grade, p.classroom, p.crew].filter(Boolean).join(' ▶ ');
}

function personContext(p) {
  if (p.isStudent) {
    return gradeChain(p);
  }
  if (p.isStaff && p.jobTitle) {
    return p.jobTitle;
  }
  const family = state.model.families[p.familyKey];
  if (family) {
    return (family.kidEmails || []).map(e => byEmail[e]?.fullName).filter(Boolean).join(', ');
  }
  return '';
}

function tagMenu(email, onChange) {
  const menu = el('div', 'card-menu tag-menu');
  menu.hidden = true;
  const render = () => {
    menu.replaceChildren();
    for (const name of tagNames()) {
      const row = el('label', 'tag-option');
      const box = el('input');
      box.type = 'checkbox';
      box.checked = tags[name].includes(email);
      box.addEventListener('change', async () => {
        await setTag(email, name, box.checked);
        render();
        onChange();
      });
      row.append(el('span', '', name), box);
      menu.append(row);
    }
    const form = el('form', 'tag-new');
    const input = el('input');
    input.placeholder = 'New tag';
    input.maxLength = 40;
    form.append(input);
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const name = input.value.trim();
      if (!name) {
        return;
      }
      input.value = '';
      await setTag(email, name, true);
      render();
      onChange();
    });
    menu.append(form);
  };
  render();
  menu.addEventListener('click', e => e.stopPropagation());
  return menu;
}

function tagControl(email, wrapClass, buttonClass, onChange) {
  const wrap = el('div', wrapClass);
  const button = el('button', buttonClass + (isTagged(email) ? ' active' : ''));
  button.title = 'Tags';
  button.append(svg('tag'));
  const menu = tagMenu(email, () => {
    button.classList.toggle('active', isTagged(email));
    onChange();
  });
  button.addEventListener('click', e => {
    e.preventDefault();
    e.stopPropagation();
    menu.hidden = !menu.hidden;
  });
  wrap.append(button, menu);
  return wrap;
}

function cardMore(email) {
  return tagControl(email, 'card-more-wrap', 'card-more', () => {});
}

function personCard(p) {
  const card = el('a', 'person-card');
  card.append(cardMore(p.email));
  card.href = personLink(p);
  card.append(applyRingColor(photoOrInitials(p.photoUrl, p.fullName, 'person-photo'), p));
  card.append(el('div', 'role-label', roleLabel(p)));
  card.append(el('div', 'person-name', p.fullName));
  const context = personContext(p);
  if (context) {
    card.append(el('div', 'person-sub', context));
  }
  return card;
}

function renderEveryone(grid) {
  grid.className = 'people-grid';
  const q = state.q;
  const matches = state.everyoneOrder.filter(p => {
    const family = state.model.families[p.familyKey];
    return `${p.fullName} ${family ? family.name : ''}`.toLowerCase().includes(q) && matchesFilters(p);
  });
  for (const p of matches) {
    grid.append(personCard(p));
  }
  return matches.length;
}

function renderStudents(grid) {
  grid.className = 'student-grid';
  const matches = state.model.people.filter(p => p.isStudent && p.fullName.toLowerCase().includes(state.q) && matchesFilters(p));
  for (const p of matches) {
    const card = el('a', 'student-card');
    card.href = personLink(p);
    card.append(cardMore(p.email));
    const head = el('div', 'student-head');
    const family = state.model.families[p.familyKey];
    if (family && family.photoUrl) {
      const bg = el('img', 'student-family-photo');
      bg.src = thumbUrl(family.photoUrl);
      bg.loading = 'lazy';
      bg.alt = '';
      head.append(bg);
    }
    head.append(photoOrInitials(p.photoUrl, p.fullName, 'student-photo'));
    card.append(head);
    card.append(el('div', 'student-first', firstName(p.fullName)));
    card.append(el('div', 'student-last', p.fullName.replace(firstName(p.fullName), '').trim()));
    card.append(el('div', 'student-line', gradeChain(p)));
    if (p.pronouns) {
      card.append(el('div', 'student-pronouns', p.pronouns));
    }
    grid.append(card);
  }
  return matches.length;
}

// Only families with at least one kid on record - a staff member with no kids
// (a Family record with no kidEmails, or no Family record at all) isn't a
// family in the school-community sense the Families tab is showing, so those
// don't get an entry here at all.
function familyEntries() {
  return Object.values(state.model.families)
    .filter(f => (f.kidEmails || []).length)
    .map(f => {
      const members = [...(f.kidEmails || []), ...(f.adultEmails || [])];
      const kidGrades = [...new Set((f.kidEmails || []).map(e => byEmail[e]?.grade).filter(Boolean))];
      return {
        key: f.key,
        name: (f.name || '').replace(/ Family$/, ''),
        label: kidGrades.join(', '),
        members: members.map(e => byEmail[e] ? firstName(byEmail[e].fullName) : '').filter(Boolean),
        photoUrl: f.photoUrl,
        href: familyLink(f.key),
      };
    });
}

function renderFamilies(grid) {
  grid.className = 'family-grid';
  const matches = state.familyOrder.filter(f =>
    `${f.name} ${f.members.join(' ')}`.toLowerCase().includes(state.q) && familyMatchesFilters(f.key));
  for (const f of matches) {
    const card = el('a', 'family-card');
    card.href = f.href;
    card.append(photoOrInitials(f.photoUrl, f.name, 'family-photo'));
    card.append(el('div', 'family-label', f.label));
    card.append(el('div', 'family-name', f.name));
    card.append(el('div', 'family-kids', f.members.join(', ')));
    grid.append(card);
  }
  return matches.length;
}

function renderStaff(grid, autoFit) {
  grid.className = '';
  const staff = state.model.people.filter(p =>
    p.isStaff && `${p.fullName} ${p.jobTitle || ''}`.toLowerCase().includes(state.q) && matchesFilters(p));
  const departments = state.model.departments || [];
  const groups = new Map();
  for (const p of staff) {
    const dept = p.department || 'Staff';
    if (!groups.has(dept)) {
      groups.set(dept, []);
    }
    groups.get(dept).push(p);
  }
  const ordered = [...groups.keys()].sort((a, b) => {
    const ia = departments.indexOf(a);
    const ib = departments.indexOf(b);
    return (ia < 0 ? departments.length : ia) - (ib < 0 ? departments.length : ib);
  });
  let count = 0;
  for (const dept of ordered) {
    grid.append(el('h2', 'staff-section', dept));
    const deptGrid = el('div', 'people-grid' + (autoFit ? ' autofit' : ''));
    for (const p of groups.get(dept)) {
      const card = el('a', 'person-card');
      card.href = personLink(p);
      card.append(cardMore(p.email));
      card.append(applyRingColor(photoOrInitials(p.photoUrl, p.fullName, 'person-photo'), p));
      card.append(el('div', 'role-label', p.jobTitle || 'Staff'));
      card.append(el('div', 'person-name', p.fullName));
      deptGrid.append(card);
      count++;
    }
    grid.append(deptGrid);
  }
  return count;
}

const tabRenderers = {
  everyone: renderEveryone,
  students: renderStudents,
  families: renderFamilies,
  staff: renderStaff,
};

function renderPeople() {
  const main = resetMain();

  const pageHeader = el('div', 'page-header container');
  pageHeader.append(el('h1', 'page-title', 'Directory'));
  pageHeader.append(el('div', 'page-subtitle', 'Find and connect with the Helios community.'));
  main.append(pageHeader);

  const items = peopleTabs.map(t => ({...t, icon: t.key === 'staff' ? 'staff-tab' : t.key}));
  main.append(tabStrip(items, state.tab, 2, key => {
    state.tab = key;
    state.q = '';
    // The Tags dropdown only exists on the Everyone tab - clear it on every
    // switch so a filter set there can't silently keep narrowing results on a
    // tab with no control showing it's active. Role chips get the same
    // treatment without losing the selection: matchesFilters only applies
    // filterRoleExcluded while state.tab is 'everyone' (see below), so
    // switching to Families and back restores whatever was toggled off.
    state.filterTags.clear();
    history.replaceState(null, '', tabHref(key));
    renderPeople();
    finishRender();
  }));

  const content = el('div', 'content container');
  const isEveryone = state.tab === 'everyone';
  const header = el('div', 'content-header' + (isEveryone ? '' : ' content-header-solo'));
  if (isEveryone) {
    header.append(roleChips(() => renderGrid()));
  }
  const controls = el('div', 'controls');
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    renderGrid();
  });
  search.append(input);
  controls.append(
    facetDropdown('Grade', gradeOptions(), state.filterGrades, () => renderGrid()),
    facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, () => renderGrid()),
  );
  if (isEveryone) {
    controls.append(facetDropdown('Tags', tagNames(), state.filterTags, () => renderGrid()));
  }
  controls.append(search);
  header.append(controls);
  content.append(header);

  const grid = el('div');
  content.append(grid);
  main.append(content);

  function renderGrid() {
    grid.replaceChildren();
    if (tabRenderers[state.tab](grid) === 0) {
      grid.append(el('div', 'empty', 'No matches.'));
    }
  }
  renderGrid();
  input.focus();
}

function personFacets(p, field) {
  if (p.isStudent) {
    return p[field] ? [p[field]] : [];
  }
  if (p.isParent) {
    const family = state.model.families[p.familyKey];
    return ((family && family.kidEmails) || []).map(e => byEmail[e]).filter(Boolean).map(k => k[field]).filter(Boolean);
  }
  return [];
}

function cityOf(p) {
  const family = state.model.families[p.familyKey];
  if (!family || !family.address) {
    return '';
  }
  const parts = family.address.split(',').map(s => s.trim());
  return parts.length >= 2 ? parts[parts.length - 2] : parts[0];
}

function anyFiltersActive() {
  return Boolean(state.filterGrades.size || state.filterClassrooms.size || state.filterRoles.size ||
    state.filterRoleExcluded.size || state.filterCities.size || state.filterPronouns.size ||
    state.filterTags.size || state.filterNew);
}

// Role chips currently show on the Directory page's Everyone tab and on the
// Email List page - not on Directory's Families tab, which has no chips of
// its own to reveal that anything is filtered. Keyed off the route rather
// than state.tab so a stale tab value left over from a different page can't
// make filterRoleExcluded apply (or not) on the wrong page.
function roleChipsVisible() {
  const seg = segments();
  if (seg[0] === 'people') {
    return state.tab === 'everyone';
  }
  return seg[0] === 'email-list';
}

function matchesFilters(p) {
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
  const pronounsOK = !state.filterPronouns.size || state.filterPronouns.has(p.pronouns);
  const newOK = !state.filterNew || p.isNew;
  const tagOK = !state.filterTags.size || tagsOf(p.email).some(t => state.filterTags.has(t));
  return gradeOK && classOK && roleOK && cityOK && pronounsOK && newOK && tagOK;
}

function familyMatchesFilters(key) {
  const family = state.model.families[key];
  if (!family) {
    return !anyFiltersActive();
  }
  const members = [...(family.kidEmails || []), ...(family.adultEmails || [])].map(e => byEmail[e]).filter(Boolean);
  return !anyFiltersActive() || members.some(matchesFilters);
}

function gradeOptions() {
  const present = new Set(state.model.people.filter(p => p.isStudent).map(p => p.grade).filter(Boolean));
  return state.model.grades.map(g => g.name).filter(n => present.has(n));
}

function cityOptions() {
  return [...new Set(state.model.people.map(cityOf).filter(Boolean))].sort();
}

function pronounOptions() {
  return [...new Set(state.model.people.map(p => p.pronouns).filter(Boolean))].sort();
}

const roleChipFacets = [
  ['Student', 'Students'],
  ['Parent', 'Parents'],
  ['Staff', 'Staff'],
];

// Role as always-visible toggle chips, all on by default. state.filterRoleExcluded
// tracks only the deselected roles (mirroring rosterSectionExcluded's exclusion-set
// approach) - kept separate from state.filterRoles, the inclusion set the Role
// checkboxes elsewhere use, so the two can't fight over what an empty set means.
function roleChips(rerender) {
  const bar = el('div', 'chip-row');
  for (const [key, label] of roleChipFacets) {
    const btn = el('button', 'chip-toggle' + (!state.filterRoleExcluded.has(key) ? ' active' : ''));
    btn.type = 'button';
    btn.append(el('span', '', label));
    btn.addEventListener('click', () => {
      if (state.filterRoleExcluded.has(key)) {
        state.filterRoleExcluded.delete(key);
      } else {
        state.filterRoleExcluded.add(key);
      }
      btn.classList.toggle('active');
      rerender();
    });
    bar.append(btn);
  }
  return bar;
}

// A standalone single-facet dropdown (Grade, Classroom) - the same checkbox
// list a filterControl section would show, but its own button so it doesn't
// need the drill-into-a-section step.
function facetDropdown(label, values, set, rerender) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button');
  const labelSpan = el('span', '', label);
  button.append(labelSpan, svg('chevron'));
  const panel = el('div', 'filter-panel facet-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
  });

  const updateLabel = () => {
    labelSpan.textContent = set.size ? `${label} (${set.size})` : label;
  };

  const body = el('div', 'filter-options');
  for (const v of values) {
    const row = el('label', 'filter-option');
    const box = el('input');
    box.type = 'checkbox';
    box.checked = set.has(v);
    box.addEventListener('change', () => {
      if (box.checked) {
        set.add(v);
      } else {
        set.delete(v);
      }
      updateLabel();
      rerender();
    });
    row.append(el('span', '', v), box);
    body.append(row);
  }
  panel.append(body);

  const footer = el('div', 'filter-footer');
  const clear = el('button', 'filter-clear', 'Clear');
  clear.addEventListener('click', () => {
    set.clear();
    for (const box of body.querySelectorAll('input')) {
      box.checked = false;
    }
    updateLabel();
    rerender();
  });
  const done = el('button', 'filter-done', 'Done');
  done.addEventListener('click', () => {
    panel.hidden = true;
    button.classList.remove('open');
  });
  footer.append(clear, done);
  panel.append(footer);

  updateLabel();
  wrap.append(button, panel);
  return wrap;
}

// options lets a caller opt out of a section (or the "New to Helios" toggle)
// that it surfaces some other way - the Directory page pulls Role out into
// chips and Grade/Classroom into their own dropdowns (see roleChips and
// facetDropdown below), while Staff, Email List, and Map keep the full panel.
function filterControl(rerender, options = {}) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button');
  button.append(svg('filter'), el('span', '', 'Filter'), svg('chevron'));
  const panel = el('div', 'filter-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
  });

  const sections = [];
  if (options.role !== false) {
    sections.push({label: 'Role', values: ['Student', 'Parent', 'Staff'], set: state.filterRoles});
  }
  if (options.classroom !== false) {
    sections.push({label: 'Class', values: state.model.classrooms.map(c => c.name), set: state.filterClassrooms});
  }
  if (options.grade !== false) {
    sections.push({label: 'Grade', values: gradeOptions(), set: state.filterGrades});
  }
  if (options.city !== false) {
    sections.push({label: 'City', values: cityOptions(), set: state.filterCities});
  }
  sections.push({label: 'Pronouns', values: pronounOptions(), set: state.filterPronouns});
  sections.push({label: 'Tags', values: tagNames(), set: state.filterTags});
  for (const s of sections) {
    const head = el('div', 'filter-section');
    head.append(el('span', '', s.label), svg('chevron'));
    const body = el('div', 'filter-options');
    body.hidden = true;
    head.addEventListener('click', () => {
      body.hidden = !body.hidden;
      head.classList.toggle('open', !body.hidden);
    });
    for (const v of s.values) {
      const row = el('label', 'filter-option');
      const box = el('input');
      box.type = 'checkbox';
      box.checked = s.set.has(v);
      box.addEventListener('change', () => {
        if (box.checked) {
          s.set.add(v);
        } else {
          s.set.delete(v);
        }
        rerender();
      });
      row.append(el('span', '', v), box);
      body.append(row);
    }
    panel.append(head, body);
  }

  if (options.newToHelios !== false) {
    const toggleRow = el('label', 'filter-toggle-row');
    toggleRow.append(el('span', '', 'New to Helios'));
    const toggle = el('input', 'filter-switch');
    toggle.type = 'checkbox';
    toggle.checked = state.filterNew;
    toggle.addEventListener('change', () => {
      state.filterNew = toggle.checked;
      rerender();
    });
    toggleRow.append(toggle);
    panel.append(toggleRow);
  }

  const footer = el('div', 'filter-footer');
  const clear = el('button', 'filter-clear', 'Clear all');
  clear.addEventListener('click', () => {
    if (options.grade !== false) {
      state.filterGrades.clear();
    }
    if (options.classroom !== false) {
      state.filterClassrooms.clear();
    }
    if (options.role !== false) {
      state.filterRoles.clear();
    }
    if (options.city !== false) {
      state.filterCities.clear();
    }
    state.filterPronouns.clear();
    state.filterTags.clear();
    if (options.newToHelios !== false) {
      state.filterNew = false;
    }
    for (const box of panel.querySelectorAll('input')) {
      box.checked = false;
    }
    rerender();
  });
  const done = el('button', 'filter-done', 'Done');
  done.addEventListener('click', () => {
    panel.hidden = true;
    button.classList.remove('open');
  });
  footer.append(clear, done);
  panel.append(footer);

  wrap.append(button, panel);
  return wrap;
}

function fromURL() {
  const raw = new URLSearchParams(location.search).get('from');
  if (!raw) {
    return null;
  }
  return new URL(raw, location.origin);
}

function classroomsBackOf(from) {
  const parent = new URLSearchParams(from.search).get('from');
  return parent && parent.startsWith('/classrooms') ? parent : '/classrooms';
}

function fromCrumbs() {
  const from = fromURL();
  if (!from) {
    return null;
  }
  const seg = from.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const back = from.pathname + from.search;
  if (seg[0] === 'grades' && seg[1]) {
    const grade = state.model.grades.find(g => slugify(g.name) === seg[1]);
    if (grade) {
      return [['Classrooms', classroomsBackOf(from)], [grade.name, back]];
    }
  }
  if (seg[0] === 'classrooms' && seg[1]) {
    const classroom = state.model.classrooms.find(c => slugify(c.name) === seg[1]);
    if (classroom) {
      return [['Classrooms', classroomsBackOf(from)], [classroom.name, back]];
    }
  }
  if (seg[0] === 'people' && !seg[1]) {
    return [['People', back]];
  }
  if (seg[0] === 'classrooms') {
    return [['Classrooms', back]];
  }
  if (seg[0] === 'staff') {
    return [['Staff', back]];
  }
  if (seg[0] === 'email-list') {
    return [['Email List', back]];
  }
  return null;
}

function breadcrumbs(parts, tagEmail) {
  const parent = [...parts].reverse().find(([, href]) => href);
  setChrome(parts[parts.length - 1][0], parent ? parent[1] : '/people');
  const top = el('div', 'detail-top container');
  const crumbs = el('div', 'crumbs');
  const back = el('a', 'crumb-back');
  back.href = parent ? parent[1] : '/people';
  back.append(svg('chevron-left'));
  crumbs.append(back);
  parts.forEach(([label, href], i) => {
    if (i > 0) {
      crumbs.append(el('span', 'crumb-sep', '/'));
    }
    if (href) {
      const a = el('a', 'crumb', label);
      a.href = href;
      crumbs.append(a);
    } else {
      crumbs.append(el('span', 'crumb current', label));
    }
  });
  top.append(crumbs);
  if (tagEmail) {
    const tagArea = el('div', 'tag-area');
    const tagList = el('div', 'tag-list');
    const renderTagList = () => {
      tagList.replaceChildren();
      for (const name of tagsOf(tagEmail)) {
        const chip = el('a', 'tag-chip', name);
        chip.href = '/people?tag=' + encodeURIComponent(name);
        chip.title = `See everyone tagged "${name}"`;
        tagList.append(chip);
      }
    };
    renderTagList();
    tagArea.append(tagList, tagControl(tagEmail, 'tag-wrap', 'tag-button', renderTagList));
    top.append(tagArea);
  }
  return top;
}

function iconButton(name, label, action) {
  const node = el(typeof action === 'string' ? 'a' : 'button', 'icon-button');
  node.title = label;
  if (typeof action === 'string') {
    node.href = action;
    node.target = '_blank';
  } else if (action) {
    node.addEventListener('click', action);
  }
  node.append(svg(name), el('span', '', label));
  return node;
}

// "she/her" -> "She / Her", for the pronouns line under a name on the profile page.
function formatPronouns(pronouns) {
  return pronouns.split('/').map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' / ');
}

function pronouncePill(url, name) {
  const btn = el('button', 'pronounce-pill');
  btn.type = 'button';
  btn.title = 'Hear how to pronounce ' + name;
  btn.append(svg('volume'), el('span', '', name));
  btn.addEventListener('click', () => new Audio(url).play());
  return btn;
}

function copyButton(text) {
  return iconButton('copy', 'Copy', () => navigator.clipboard.writeText(text));
}

function personSummaryText(p, family) {
  const lines = [p.fullName];
  lines.push(p.pronouns ? `${baseRole(p)} · ${formatPronouns(p.pronouns)}` : baseRole(p));
  lines.push('');
  if (p.phone) {
    lines.push('Phone: ' + p.phone);
  }
  lines.push('Email: ' + p.email);
  if (family && family.address) {
    lines.push('Address: ' + family.address);
  }
  if (p.facts) {
    lines.push('', 'About: ' + p.facts);
  }
  return lines.join('\n');
}

function copyGlyph(text) {
  const btn = el('button', 'copy-glyph');
  btn.title = 'Copy';
  btn.append(svg('copy'));
  btn.addEventListener('click', () => {
    navigator.clipboard.writeText(text);
    btn.classList.add('copied');
    btn.replaceChildren(svg('check'));
    setTimeout(() => {
      btn.classList.remove('copied');
      btn.replaceChildren(svg('copy'));
    }, 1200);
  });
  return btn;
}

async function submitField(key, field, value, status) {
  status.classList.remove('error');
  status.textContent = 'Saving…';
  const form = new FormData();
  form.append('key', key);
  form.append('field', field);
  form.append('value', value);
  const res = await fetch('/api/directory/edit', {method: 'POST', body: form});
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return false;
  }
  await load();
  return true;
}

function editPencil(title) {
  const pencil = el('button', 'edit-icon inline');
  pencil.title = title;
  pencil.append(svg('pencil'));
  return pencil;
}

function fieldEditor(anchor, pencil, opts) {
  const box = el('div', 'field-editor');
  const input = el('input');
  input.type = 'text';
  input.value = opts.current || '';
  const note = el('div', 'field-note', "This doesn't affect the values shown in Veracross.");
  const buttons = el('div', 'about-buttons');
  const status = el('div', 'media-status about-status');
  const save = el('button', 'media-button primary', 'Save');
  const cancel = el('button', 'media-button', 'Cancel');
  buttons.append(save, cancel);
  if (opts.allowHide && opts.current) {
    const hide = el('button', 'media-button', 'Hide');
    buttons.append(hide);
    hide.addEventListener('click', () => opts.submit('', status));
  }
  box.append(input, note, buttons, status);
  cancel.addEventListener('click', () => {
    box.remove();
    anchor.hidden = false;
    pencil.hidden = false;
  });
  save.addEventListener('click', () => opts.submit(input.value.trim(), status));
  anchor.hidden = true;
  pencil.hidden = true;
  anchor.after(box);
  input.focus();
}

function contactRow(value, buttons) {
  const row = el('div', 'contact-row');
  row.append(value);
  const actions = el('div', 'contact-actions');
  for (const b of buttons) {
    actions.append(b);
  }
  row.append(actions);
  return row;
}

function displayNameLine(p) {
  if (!p.legalName || p.legalName === p.fullName) {
    return null;
  }
  return p.legalName;
}

function familyCardRow(p, subtitle) {
  const row = el('a', 'fcard-row');
  row.href = personLink(p);
  row.append(photoOrInitials(p.photoUrl, p.fullName, 'fcard-avatar'));
  const info = el('div', 'fcard-info');
  info.append(el('div', 'fcard-name', p.fullName));
  if (subtitle) {
    info.append(el('div', 'fcard-sub', subtitle));
  }
  row.append(info);
  const chev = el('div', 'fcard-chevron');
  chev.append(svg('chevron-right'));
  row.append(chev);
  return row;
}

function familyBand(p, family) {
  const band = el('div', 'container fcard-wrap');
  const card = el('div', 'detail-card fcard');
  const head = el('div', 'fcard-head');
  head.append(el('h2', 'fcard-title', family.name));
  const seeLink = el('a', 'fcard-see-link');
  seeLink.href = familyLink(family.key);
  seeLink.append(el('span', '', 'View full family profile'), svg('chevron-right'));
  head.append(seeLink);
  card.append(head);

  const grid = el('div', 'fcard-grid');
  const left = el('div');
  const familyEditable = family.key === myFamilyKey() || state.model.superEdit;
  const showFamilyPhotoEdit = familyEditable && familyPhotoNeedsUpdate(family);
  // The "update for the new year" nagging (dashed outline + reminder text) is meant
  // for students, same as personal photos/facts - an adult visiting their own page
  // still gets the camera icon to make uploading easy, just without the nag.
  const nagFamilyPhoto = showFamilyPhotoEdit && p.isStudent;
  const photoWrap = el('div', 'photo-wrap' + (nagFamilyPhoto ? ' needs-update' : ''));
  if (family.photoUrl) {
    const img = el('img', 'fcard-photo');
    img.src = thumbUrl(family.photoUrl);
    img.alt = '';
    photoWrap.append(img);
  } else {
    photoWrap.append(photoOrInitials(null, family.name, 'fcard-photo fcard-photo-empty'));
  }
  left.append(photoWrap);
  if (showFamilyPhotoEdit) {
    const status = el('div', 'media-status');
    if (nagFamilyPhoto) {
      status.textContent = 'Add your family photo for the new year';
    }
    photoWrap.append(uploadIcon('camera', 'Upload family photo', 'image/*', 'family', family.key, 'photo', status));
    left.append(status);
  }
  if (family.photoCaption) {
    left.append(el('div', 'fcard-caption', family.photoCaption));
  }
  grid.append(left);

  const right = el('div');
  const kidsList = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
  if (kidsList.length && !(p.isStudent && kidsList.length === 1 && kidsList[0].email === p.email)) {
    right.append(el('div', 'fcard-section-header', 'Children'));
    for (const kid of kidsList) {
      if (p.isStudent && kid.email === p.email) {
        continue;
      }
      right.append(familyCardRow(kid, gradeChain(kid)));
    }
  }
  const adults = (family.adultEmails || []).map(e => byEmail[e]).filter(Boolean).filter(a => a.email !== p.email);
  if (adults.length) {
    right.append(el('div', 'fcard-section-header', 'Other Family Members'));
    for (const adult of adults) {
      right.append(familyCardRow(adult, baseRole(adult)));
    }
  }
  grid.append(right);
  card.append(grid);
  band.append(card);
  return band;
}

let personEdit = null;

function renderPersonDetail(email) {
  const main = resetMain();
  const p = personByKey(email);
  if (!p) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
  }
  const params = new URLSearchParams(location.search);
  const focusFacts = params.get('focus') === 'facts';
  if (params.get('edit') === '1') {
    params.delete('edit');
    params.delete('focus');
    const query = params.toString();
    history.replaceState(null, '', location.pathname + (query ? '?' + query : ''));
    personEdit = email;
  }
  const origin = fromCrumbs() || [['People', '/people']];
  main.append(breadcrumbs([...origin, [p.fullName, null]], p.email));

  const editable = canEditPerson(p.email);
  const editing = editable && personEdit === p.email;
  // The "update for the new year" nag (dashed outline + reminder text) is student-only,
  // same as elsewhere - but anyone editable with no photo yet still gets a camera icon
  // to make uploading easy, without implying it's overdue.
  const nagPhoto = editable && photoNeedsUpdate(p);
  const showPhotoEdit = editing || nagPhoto || (editable && !p.photoUrl);
  const showFactsEdit = editing || (editable && factsNeedUpdate(p));
  const self = p.email === document.body.dataset.userEmail;

  if (editable) {
    const personTasks = staleItems().filter(i => i.person && i.person.email === p.email);
    if (personTasks.length) {
      const wrap = el('div', 'container');
      wrap.append(todoChecklist(personTasks));
      main.append(wrap);
    }
  }

  const content = el('div', 'container detail-content');
  const headerCard = el('div', 'detail-card');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  const wrap = el('div', 'photo-wrap' + (nagPhoto ? ' needs-update' : ''));
  if (p.photoUrl) {
    const img = el('img', 'detail-photo');
    img.src = p.photoUrl;
    img.alt = '';
    wrap.append(img);
  } else {
    // No uploaded photo: fall back to the same colored-initials shape the directory
    // grid uses instead of an empty gray box, so a profile never looks broken.
    wrap.append(photoOrInitials(null, p.fullName, 'detail-photo detail-photo-empty'));
  }
  left.append(wrap);
  if (showPhotoEdit) {
    const status = el('div', 'media-status');
    if (nagPhoto) {
      status.textContent = `Add ${self ? 'your' : `${firstName(p.fullName)}'s`} photo for the new year`;
    }
    wrap.append(uploadIcon('camera', 'Upload photo', 'image/*', 'person', p.email, 'photo', status));
    left.append(status);
  }
  if ((p.photos || []).length > 1) {
    left.append(photoSwitcher(p, wrap.querySelector('.detail-photo'), editable));
  }
  grid.append(left);

  const right = el('div');
  const family = state.model.families[p.familyKey];
  const topRow = el('div', 'detail-top');
  const roleText = p.pronouns ? `${baseRole(p)} (${formatPronouns(p.pronouns)})` : baseRole(p);
  topRow.append(el('div', 'role-label', roleText));
  if (editable) {
    const topActions = el('div', 'detail-top-actions');
    const toggle = editing
      ? el('button', 'media-button edit-toggle', 'Done')
      : iconButton('pencil', 'Edit info', () => {
        personEdit = p.email;
        renderPersonDetail(email);
        finishRender();
      });
    if (editing) {
      toggle.addEventListener('click', () => {
        personEdit = null;
        renderPersonDetail(email);
        finishRender();
      });
    }
    topActions.append(toggle);
    topActions.append(iconButton('copy', 'Copy all info', () => navigator.clipboard.writeText(personSummaryText(p, family))));
    topRow.append(topActions);
  }
  right.append(topRow);
  const nameHeader = el('h1', 'detail-name');
  nameHeader.append(el('span', '', p.fullName));
  if (p.pronunciationUrl && !editing) {
    nameHeader.append(pronouncePill(p.pronunciationUrl, firstName(p.fullName)));
  }
  right.append(nameHeader);
  if (editing) {
    const pencil = editPencil('Edit preferred name');
    nameHeader.append(pencil);
    pencil.addEventListener('click', () => fieldEditor(nameHeader, pencil, {
      current: p.preferredName || '',
      submit: (value, status) => submitField(p.email, 'preferred-name', value, status),
    }));
  }
  const nickname = displayNameLine(p);
  if (nickname) {
    right.append(el('div', 'detail-sub', nickname));
  }
  if (p.isStudent) {
    const chain = gradeChain(p);
    if (chain) {
      right.append(el('div', 'detail-sub', chain));
    }
  }
  if (p.phone || editing) {
    const actions = p.phone ? [
      iconButton('message', 'Text', 'sms:' + p.phone),
      iconButton('phone', 'Call', 'tel:' + p.phone),
      copyButton(p.phone),
    ] : [];
    const phoneValue = el('div', 'contact-value editable-value');
    phoneValue.append(svg('phone'), el('span', '', p.phone || 'No phone number'));
    const phoneRow = contactRow(phoneValue, actions);
    right.append(phoneRow);
    if (editing) {
      const pencil = editPencil('Edit phone number');
      phoneValue.append(pencil);
      pencil.addEventListener('click', () => fieldEditor(phoneRow, pencil, {
        current: p.phone || '',
        allowHide: true,
        submit: (value, status) => submitField(p.email, 'phone', value, status),
      }));
    }
  }
  const emailValue = el('div', 'contact-value');
  emailValue.append(svg('mail'), el('span', '', p.email));
  right.append(contactRow(emailValue, [
    iconButton('mail', 'Email', 'mailto:' + p.email),
    copyButton(p.email),
  ]));
  const addressEditable = editing && family && p.email === document.body.dataset.userEmail &&
    (family.adultEmails || []).includes(p.email);
  if (family && (family.address || addressEditable)) {
    const block = el('div');
    const addressValue = el('div', 'contact-value editable-value');
    addressValue.append(svg('map'), el('span', '', family.address || 'No address'));
    block.append(addressValue);
    const actions = family.address ? [
      iconButton('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
      copyButton(family.address),
    ] : [];
    const addressRow = contactRow(block, actions);
    right.append(addressRow);
    if (addressEditable) {
      const pencil = editPencil('Edit address');
      addressValue.append(pencil);
      pencil.addEventListener('click', () => fieldEditor(addressRow, pencil, {
        current: family.address || '',
        allowHide: true,
        submit: (value, status) => submitField(p.email, 'address', value, status),
      }));
    }
  }
  if (editing) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    if (p.pronunciationUrl) {
      const audio = el('audio', 'pronounce-player');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = p.pronunciationUrl;
      right.append(audio);
    }
    right.append(pronounceEditor('person', p.email));
  }
  grid.append(right);
  headerCard.append(grid);
  content.append(headerCard);

  if (p.facts || showFactsEdit) {
    const aboutCard = el('div', 'detail-card');
    const header = el('h2', 'about-header', 'About Me');
    aboutCard.append(header);
    const needsFacts = factsNeedUpdate(p);
    const placeholder = needsFacts ? `Add ${self ? 'your' : `${firstName(p.fullName)}'s`} facts for the new year — click the pencil to get started.` : '';
    const textClass = 'about-text' + (editable && needsFacts ? ' needs-update' : '') + (!p.facts && placeholder ? ' placeholder-text' : '');
    const text = el('div', textClass, p.facts || placeholder);
    const status = el('div', 'media-status about-status');
    aboutCard.append(text);
    if (p.facts) {
      const when = monthYear(p.factsUpdated);
      if (editable && needsFacts) {
        aboutCard.append(el('div', 'about-note about-note-stale',
          when ? `Posted ${when} — please refresh this for the new year.` : 'Please refresh this for the new year.'));
      } else if (!editable && when) {
        aboutCard.append(el('div', 'about-note', `Posted ${when}`));
      }
    }
    aboutCard.append(status);
    if (showFactsEdit) {
      const pencil = el('button', 'edit-icon inline');
      pencil.title = 'Edit';
      pencil.append(svg('pencil'));
      header.append(pencil);
      pencil.addEventListener('click', () => {
        const editor = el('textarea', 'about-editor');
        editor.value = p.facts || '';
        const buttons = el('div', 'about-buttons');
        const save = el('button', 'media-button primary', 'Save');
        const cancel = el('button', 'media-button', 'Cancel');
        buttons.append(save, cancel);
        text.replaceWith(editor);
        editor.after(buttons);
        pencil.hidden = true;
        editor.focus();
        cancel.addEventListener('click', () => {
          buttons.remove();
          editor.replaceWith(text);
          pencil.hidden = false;
        });
        save.addEventListener('click', async () => {
          status.classList.remove('error');
          status.textContent = 'Saving…';
          const form = new FormData();
          form.append('key', p.email);
          form.append('facts', editor.value);
          const res = await fetch('/api/directory/facts', {method: 'POST', body: form});
          if (!res.ok) {
            status.classList.add('error');
            status.textContent = await res.text();
            return;
          }
          await load();
        });
      });
      if (focusFacts) {
        pencil.click();
      }
    }
    content.append(aboutCard);
  }

  if (editing) {
    const privacyCard = el('div', 'detail-card');
    const header = el('h2', 'about-header', 'Privacy');
    const button = el('button', 'media-button',
      'Remove ' + (self ? 'me' : firstName(p.fullName)) + ' from this directory');
    const status = el('div', 'media-status');
    button.addEventListener('click', async () => {
      const message = 'This removes all data about ' + (self ? 'you' : firstName(p.fullName)) +
        ' from this directory. ' +
        'For security, users not in the directory cannot access it. ' +
        "This doesn't affect the values shown in Veracross. Continue?";
      if (!confirm(message)) {
        return;
      }
      status.classList.remove('error');
      status.textContent = 'Removing…';
      const form = new FormData();
      form.append('key', p.email);
      const res = await fetch('/api/directory/optout', {method: 'POST', body: form});
      if (!res.ok) {
        status.classList.add('error');
        status.textContent = await res.text();
        return;
      }
      location.reload();
    });
    privacyCard.append(header, button, status);
    content.append(privacyCard);
  }
  main.append(content);

  if (family) {
    main.append(familyBand(p, family));
  }
}

function renderFamilyDetail(key) {
  const main = resetMain();
  const family = state.model.families[key];
  if (!family) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
  }
  const editable = key === myFamilyKey() || state.model.superEdit;
  const shortName = (family.name || '').replace(/ Family$/, '');
  let crumbs = [['People', '/people'], [shortName, null], ['Family', null]];
  const from = fromURL();
  if (key === myFamilyKey()) {
    crumbs = [['My Family', '/my-family'], ['Family', null]];
  } else if (from) {
    const rseg = from.pathname.split('/').filter(Boolean).map(decodeURIComponent);
    const back = from.pathname + from.search;
    const rsegPerson = rseg[0] === 'people' && rseg[1] ? personByKey(rseg[1]) : undefined;
    if (rsegPerson) {
      const person = rsegPerson;
      const peopleBack = new URLSearchParams(from.search).get('from');
      const peopleHref = peopleBack && peopleBack.startsWith('/people') && !peopleBack.startsWith('/people/') ? peopleBack : '/people';
      crumbs = [['People', peopleHref], [person.fullName, back], ['Family', null]];
    } else if (rseg[0] === 'people' && !rseg[1]) {
      crumbs = [['People', back], [shortName, null], ['Family', null]];
    } else if (rseg[0] === 'map') {
      crumbs = [['Map', back], [shortName, null], ['Family', null]];
    }
  }
  main.append(breadcrumbs(crumbs));

  if (editable) {
    const items = staleItems();
    if (items.length) {
      const wrap = el('div', 'container');
      wrap.append(todoChecklist(items));
      main.append(wrap);
    }
  }

  const content = el('div', 'container detail-content');
  const headerCard = el('div', 'detail-card');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  left.id = 'family-photo';
  const wrap = el('div', 'photo-wrap');
  if (family.photoUrl) {
    const link = el('a');
    link.href = family.photoUrl;
    link.target = '_blank';
    const img = el('img', 'detail-photo');
    img.src = family.photoUrl;
    img.alt = '';
    link.append(img);
    wrap.append(link);
  } else {
    wrap.append(photoOrInitials(null, family.name, 'detail-photo detail-photo-empty'));
  }
  left.append(wrap);
  if (editable) {
    const status = el('div', 'media-status');
    wrap.append(uploadIcon('camera', 'Upload family photo', 'image/*', 'family', key, 'photo', status));
    left.append(status);
  }
  if (family.photoCaption) {
    left.append(el('div', 'family-caption', family.photoCaption));
  }
  if (family.photoUrl) {
    left.append(el('div', 'photo-hint', 'click photo to open full size'));
  }
  grid.append(left);

  const right = el('div');
  const kids = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
  const adults = (family.adultEmails || []).map(e => byEmail[e]).filter(Boolean);
  const grades = [...new Set(kids.map(k => k.grade).filter(Boolean))];
  right.append(el('div', 'role-label', grades.length ? grades.join(', ') : 'Staff'));
  right.append(el('h1', 'detail-name', family.name));
  const firsts = [...kids, ...adults].map(m => firstName(m.fullName));
  if (firsts.length) {
    right.append(el('div', 'detail-sub', firsts.join(', ')));
  }
  if (family.address) {
    const addressValue = el('div', 'contact-value');
    addressValue.append(svg('map'), el('span', '', family.address));
    right.append(contactRow(addressValue, [
      iconButton('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
      copyButton(family.address),
    ]));
  }
  if (family.pronunciationUrl || editable) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    if (family.pronunciationUrl) {
      const audio = el('audio', 'pronounce-player');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = family.pronunciationUrl;
      right.append(audio);
    }
    if (editable) {
      right.append(pronounceEditor('family', key));
    }
  }
  grid.append(right);
  headerCard.append(grid);
  content.append(headerCard);
  main.append(content);

  const band = el('div', 'container fcard-wrap');
  const membersCard = el('div', 'detail-card fcard');
  membersCard.append(el('h2', 'fcard-title', 'Family Members'));
  const cols = el('div', 'members-grid');
  const adultsCol = el('div');
  if (adults.length) {
    adultsCol.append(el('div', 'fcard-section-header', 'Adults'));
    for (const a of adults) {
      adultsCol.append(familyCardRow(a, baseRole(a)));
    }
  }
  const kidsCol = el('div');
  if (kids.length) {
    kidsCol.append(el('div', 'fcard-section-header', 'Children'));
    for (const k of kids) {
      kidsCol.append(familyCardRow(k, gradeChain(k)));
    }
  }
  cols.append(adultsCol, kidsCol);
  membersCard.append(cols);
  band.append(membersCard);
  main.append(band);

  if (location.hash) {
    const target = document.querySelector(location.hash);
    if (target) {
      target.scrollIntoView({block: 'center'});
    }
  }
}

function slugify(name) {
  return name.toLowerCase().replaceAll(' ', '-');
}

function ordinal(gradeName) {
  const n = Number(gradeName.split(' ')[1]);
  return n + ({1: 'st', 2: 'nd', 3: 'rd'}[n] || 'th');
}

function bandGroups() {
  const groups = [];
  for (const g of state.model.grades) {
    if (!g.band || g.band === 'Eggs' || g.band === 'Alum' || g.name === 'PreK' || g.name === 'Grade 9') {
      continue;
    }
    let group = groups.find(x => x.band === g.band);
    if (!group) {
      group = {band: g.band, grades: []};
      groups.push(group);
    }
    group.grades.push(g.name);
  }
  for (const group of groups) {
    group.label = group.band === 'Hummingbirds' ? 'K' : group.grades.map(ordinal).join(' / ');
  }
  return groups;
}

function gradeImage(gradeName) {
  const grade = state.model.grades.find(g => g.name === gradeName);
  if (grade && grade.imageUrl) {
    return grade.imageUrl;
  }
  const suffix = gradeName === 'Kindergarten' ? 'k' : gradeName.split(' ')[1];
  return '/static/brand/classrooms/grade-' + suffix + '.jpg';
}

function studentsOf(filter) {
  return state.model.people.filter(p => p.isStudent && filter(p));
}

function classroomBand(name) {
  const student = state.model.people.find(p => p.isStudent && p.classroom === name);
  if (!student) {
    return '';
  }
  const grade = state.model.grades.find(g => g.name === student.grade);
  return grade ? grade.band : '';
}

function listRow(image, label, title, sub, href) {
  const row = el('a', 'list-row');
  row.href = href;
  if (image) {
    const img = el('img', 'list-tile');
    img.src = image;
    img.loading = 'lazy';
    img.alt = '';
    row.append(img);
  } else {
    row.append(el('div', 'list-tile'));
  }
  const info = el('div', 'list-info');
  if (label) {
    info.append(el('div', 'role-label', label));
  }
  info.append(el('div', 'list-title', title));
  if (sub) {
    info.append(listSub(sub));
  }
  row.append(info);
  return row;
}

const LIST_SUB_TRUNCATE_LENGTH = 70;
const LIST_SUB_LINE_PREVIEW = 4;
const BULLET_LINE = /^\s*(?:[*]|-{1,2})\s+(.+)$/;

// Recognizes "About Me" text that's really a bullet list - every non-blank line
// starts with *, -, or -- - and returns the items with their markers stripped.
// A single stray non-bulleted line (a mixed intro-plus-bullets bio) falls back
// to plain multi-line rendering rather than a half-bulleted list.
function parseBullets(lines) {
  const items = [];
  for (const line of lines) {
    const m = line.match(BULLET_LINE);
    if (!m) {
      return null;
    }
    items.push(m[1].trim());
  }
  return items;
}

// A row's "About Me" (or similar) text. Multi-line text (bulleted or not) is
// capped at LIST_SUB_LINE_PREVIEW lines with a More/Less toggle; a single long
// line is instead cut down to roughly one line by character count. Either way,
// stopPropagation on the toggle keeps that click from also triggering the
// surrounding card's own navigation link.
function listSub(text) {
  const wrap = el('div', 'list-sub');
  const lines = text.split(/\r?\n/).map(l => l.trim()).filter(Boolean);
  if (lines.length > 1) {
    const bullets = parseBullets(lines);
    wrap.append(bullets
      ? collapsibleLines(bullets, 'ul', 'list-sub-bullets', 'li')
      : collapsibleLines(lines, 'div', 'list-sub-lines', 'div'));
    return wrap;
  }
  if (text.length <= LIST_SUB_TRUNCATE_LENGTH) {
    wrap.textContent = text;
    return wrap;
  }
  let short = text.slice(0, LIST_SUB_TRUNCATE_LENGTH);
  short = short.slice(0, short.lastIndexOf(' ')) || short;
  const textSpan = el('span', '', short + '… ');
  const toggle = el('button', 'list-sub-more', 'More »');
  toggle.type = 'button';
  let expanded = false;
  toggle.addEventListener('click', e => {
    e.preventDefault();
    e.stopPropagation();
    expanded = !expanded;
    textSpan.textContent = expanded ? text + ' ' : short + '… ';
    toggle.textContent = expanded ? 'Less' : 'More »';
  });
  wrap.append(textSpan, toggle);
  return wrap;
}

// Renders items as wrapTag > itemTag*, capped at LIST_SUB_LINE_PREVIEW with a
// More(+N)/Less toggle when there are more than that many.
function collapsibleLines(items, wrapTag, wrapClass, itemTag) {
  const frag = document.createDocumentFragment();
  const list = el(wrapTag, wrapClass);
  const renderItems = shown => {
    list.replaceChildren();
    for (const item of shown) {
      list.append(el(itemTag, '', item));
    }
  };
  if (items.length <= LIST_SUB_LINE_PREVIEW) {
    renderItems(items);
    frag.append(list);
    return frag;
  }
  const collapsed = items.slice(0, LIST_SUB_LINE_PREVIEW);
  const hiddenCount = items.length - LIST_SUB_LINE_PREVIEW;
  renderItems(collapsed);
  const toggle = el('button', 'list-sub-more', `More (+${hiddenCount}) »`);
  toggle.type = 'button';
  let expanded = false;
  toggle.addEventListener('click', e => {
    e.preventDefault();
    e.stopPropagation();
    expanded = !expanded;
    renderItems(expanded ? items : collapsed);
    toggle.textContent = expanded ? 'Less' : `More (+${hiddenCount}) »`;
  });
  frag.append(list, toggle);
  return frag;
}

function badgeCard(imageUrl, label, name, href, color) {
  const card = el('a', 'classroom-card');
  card.href = href;
  const photo = imageUrl ? el('img', 'classroom-photo') : el('div', 'classroom-photo');
  if (imageUrl) {
    photo.src = imageUrl;
    photo.loading = 'lazy';
    photo.alt = '';
  }
  if (color) {
    photo.style.setProperty('--ring-color', color);
  }
  card.append(photo);
  card.append(el('div', 'role-label', label));
  card.append(el('div', 'person-name', name));
  return card;
}

const classroomsTabs = [
  {key: 'by-classroom', label: 'Explore by Classroom', heading: 'Explore by Classroom'},
  {key: 'by-grade', label: 'Explore by Grade', heading: 'Explore By Grade'},
  {key: 'room-parents', label: 'Room Parents', heading: ''},
];

function renderClassroomsList(list) {
  const q = state.q;
  let count = 0;
  for (const group of bandGroups()) {
    const rows = state.model.classrooms
      .filter(c => classroomBand(c.name) === group.band)
      .filter(c => c.name.toLowerCase().includes(q));
    if (!rows.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit');
    for (const c of rows) {
      const students = studentsOf(p => p.classroom === c.name).length;
      grid.append(badgeCard(c.imageUrl, `${students} student${students === 1 ? '' : 's'}`, c.name,
        withFrom('/classrooms/' + slugify(c.name)), c.color));
      count++;
    }
    list.append(grid);
  }
  return count;
}

function renderGradesList(list) {
  const q = state.q;
  let count = 0;
  for (const group of bandGroups()) {
    const rows = group.grades.filter(name => name.toLowerCase().includes(q));
    if (!rows.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit');
    for (const name of rows) {
      const students = studentsOf(p => p.grade === name).length;
      const grade = state.model.grades.find(g => g.name === name);
      grid.append(badgeCard(gradeImage(name), `${students} student${students === 1 ? '' : 's'}`, name,
        withFrom('/grades/' + slugify(name)), grade && grade.color));
      count++;
    }
    list.append(grid);
  }
  return count;
}

function kidsSummary(parent) {
  const family = state.model.families[parent.familyKey];
  if (!family) {
    return '';
  }
  return (family.kidEmails || [])
    .map(e => byEmail[e])
    .filter(Boolean)
    .map(k => `${firstName(k.fullName)} (${[k.classroom, k.grade].filter(Boolean).join(' - ')})`)
    .join(' • ');
}

function renderRoomParents(list) {
  const q = state.q;
  let count = 0;
  for (const group of bandGroups()) {
    const parents = (state.model.roomParents[group.label] || [])
      .map(e => byEmail[e])
      .filter(Boolean)
      .filter(p => p.fullName.toLowerCase().includes(q));
    if (!parents.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit');
    for (const p of parents) {
      const card = el('a', 'person-card');
      card.href = personLink(p);
      card.append(cardMore(p.email));
      card.append(applyRingColor(photoOrInitials(p.photoUrl, p.fullName, 'person-photo'), p));
      card.append(el('div', 'role-label', kidsSummary(p)));
      card.append(el('div', 'person-name', p.fullName));
      grid.append(card);
      count++;
    }
    list.append(grid);
  }
  return count;
}

const classroomsTabRenderers = {
  'by-classroom': renderClassroomsList,
  'by-grade': renderGradesList,
  'room-parents': renderRoomParents,
};

function renderClassroomsPage() {
  const main = resetMain();

  const pageHeader = el('div', 'page-header container');
  pageHeader.append(el('h1', 'page-title', 'Classrooms'));
  main.append(pageHeader);

  main.append(tabStrip(classroomsTabs, state.classTab, 1, key => {
    state.classTab = key;
    state.q = '';
    history.replaceState(null, '', tabHref(key));
    renderClassroomsPage();
    finishRender();
  }));

  const content = el('div', 'content container');
  const header = el('div', 'content-header');
  const heading = classroomsTabs.find(t => t.key === state.classTab).heading;
  header.append(el('h1', '', heading));
  const controls = el('div', 'controls');
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    renderList();
  });
  search.append(input);
  controls.append(search);
  header.append(controls);
  content.append(header);

  const list = el('div');
  content.append(list);
  main.append(content);

  function renderList() {
    list.replaceChildren();
    if (classroomsTabRenderers[state.classTab](list) === 0) {
      list.append(el('div', 'empty', 'No matches.'));
    }
  }
  renderList();
}

function renderStaffPage() {
  const main = resetMain();

  const pageHeader = el('div', 'page-header-plain container');
  pageHeader.append(el('h1', 'page-title', 'Staff'));
  main.append(pageHeader);

  const content = el('div', 'content container');
  const header = el('div', 'content-header content-header-solo');
  const controls = el('div', 'controls');
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    renderList();
  });
  search.append(input);
  controls.append(search, filterControl(renderList));
  header.append(controls);
  content.append(header);

  const list = el('div');
  content.append(list);
  main.append(content);

  function renderList() {
    list.replaceChildren();
    if (renderStaff(list, true) === 0) {
      list.append(el('div', 'empty', 'No matches.'));
    }
  }
  renderList();
}

function parentsOf(students) {
  const seen = new Set();
  const parents = [];
  for (const s of students) {
    const family = state.model.families[s.familyKey];
    for (const email of (family && family.adultEmails) || []) {
      if (!seen.has(email) && byEmail[email]) {
        seen.add(email);
        parents.push(byEmail[email]);
      }
    }
  }
  return parents;
}

function teachersOf(classroomNames) {
  const seen = new Set();
  const teachers = [];
  for (const crew of state.model.crews) {
    if (!classroomNames.includes(crew.classroom)) {
      continue;
    }
    for (const name of crew.teachers || []) {
      if (!seen.has(name)) {
        seen.add(name);
        teachers.push(name);
      }
    }
  }
  return teachers;
}

function otherFamilyMembers(student) {
  const family = state.model.families[student.familyKey];
  if (!family) {
    return '';
  }
  return [...(family.kidEmails || []), ...(family.adultEmails || [])]
    .filter(e => e !== student.email)
    .map(e => byEmail[e])
    .filter(Boolean)
    .map(m => m.fullName)
    .join(', ');
}

function lastName(fullName) {
  const parts = (fullName || '').trim().split(/\s+/);
  return parts[parts.length - 1] || '';
}

function sortPeople(list) {
  const copy = [...list];
  copy.sort((a, b) => lastName(a.fullName).localeCompare(lastName(b.fullName)) || a.fullName.localeCompare(b.fullName));
  return copy;
}

// Section chips let an admin narrow a multi-crew classroom (or a multi-classroom
// grade) down to just some groups, toggled on/off independently rather than
// picking one at a time - every section is on by default. Each chip is labeled
// with the group's full header (e.g. "Great Egrets"), while chipLabel - the
// shorter crew name alone (e.g. "Great") - is the filter key, matching what
// visibleGroups matches against elsewhere in renderRoster. state.rosterSectionExcluded
// tracks only the deselected keys, and stale entries left over from a different
// classroom/grade are pruned whenever the available keys change.
function sectionFilterBar(groups, rerender) {
  const sections = groups.filter(g => g.header);
  if (sections.length < 2) {
    return null;
  }
  const keys = sections.map(g => g.chipLabel || g.header);
  for (const excluded of [...state.rosterSectionExcluded]) {
    if (!keys.includes(excluded)) {
      state.rosterSectionExcluded.delete(excluded);
    }
  }
  const bar = el('div', 'chip-row');
  for (const g of sections) {
    const key = g.chipLabel || g.header;
    const active = !state.rosterSectionExcluded.has(key);
    const btn = el('button', 'chip-toggle' + (active ? ' active' : ''));
    btn.type = 'button';
    btn.append(el('span', '', g.header));
    btn.addEventListener('click', () => {
      if (active) {
        state.rosterSectionExcluded.add(key);
      } else {
        state.rosterSectionExcluded.delete(key);
      }
      rerender();
    });
    bar.append(btn);
  }
  return bar;
}

function renderRoster(title, image, groups, backLabel) {
  const main = resetMain();
  const from = fromURL();
  const back = from && from.pathname === '/classrooms' ? from.pathname + from.search : '/classrooms';

  const header = el('div', 'roster-header');
  header.append(breadcrumbs([['Classrooms', back], [title, null]]));

  const headWrap = el('div', 'container');
  const head = el('div', 'class-head');
  if (image) {
    const img = el('img', 'class-tile');
    img.src = image;
    img.alt = '';
    head.append(img);
  }
  head.append(el('h1', 'class-title', title));
  headWrap.append(head);
  header.append(headWrap);

  const allStudents = groups.flatMap(g => g.students);
  const teachers = teachersOf([...new Set(allStudents.map(s => s.classroom).filter(Boolean))]);
  const parents = parentsOf(allStudents);
  const memberTabs = [
    {key: 'students', label: 'Students', icon: 'students', count: allStudents.length},
    {key: 'staff', label: 'Staff', icon: 'staff-tab', count: teachers.length},
    {key: 'parents', label: 'Parents', icon: 'families', count: parents.length},
  ];

  const strip = tabStrip(memberTabs, state.rosterTab, 2, key => {
    state.rosterTab = key;
    history.replaceState(null, '', tabHref(key));
    renderRoster(title, image, groups, backLabel);
  });
  strip.classList.add('roster-tabs');
  header.append(strip);
  main.append(header);

  const content = el('div', 'container detail-content');
  const list = el('div');
  const rerender = () => renderRoster(title, image, groups, backLabel);
  if (state.rosterTab === 'students') {
    const headingRow = el('div', 'roster-heading-row');
    headingRow.append(el('h2', 'roster-heading', `${allStudents.length} Students`));
    const filterBar = sectionFilterBar(groups, rerender);
    if (filterBar) {
      headingRow.append(filterBar);
    }
    list.append(headingRow);
    const visibleGroups = groups.filter(g => !g.header || !state.rosterSectionExcluded.has(g.chipLabel || g.header));
    const listBody = el('div', 'roster-list grid');
    for (const group of visibleGroups) {
      if (group.header) {
        listBody.append(el('h2', 'group-header', group.header));
      }
      for (const s of sortPeople(group.students)) {
        listBody.append(listRow(thumbUrl(s.photoUrl), otherFamilyMembers(s).toUpperCase(), s.fullName, s.facts || '', personLink(s)));
      }
    }
    list.append(listBody);
  } else if (state.rosterTab === 'staff') {
    list.append(el('h2', 'roster-heading', `${teachers.length} Staff`));
    const listBody = el('div', 'roster-list grid');
    const staffItems = teachers.map(email => {
      const person = byEmail[email.toLowerCase()];
      return person ? {fullName: person.fullName, person} : {fullName: email, email};
    });
    for (const item of sortPeople(staffItems)) {
      if (item.person) {
        const person = item.person;
        listBody.append(listRow(thumbUrl(person.photoUrl), (person.jobTitle || '').toUpperCase(), person.fullName, person.facts || '', personLink(person)));
      } else {
        listBody.append(el('div', 'list-row plain', item.email));
      }
    }
    list.append(listBody);
  } else {
    list.append(el('h2', 'roster-heading', `${parents.length} Parents`));
    const listBody = el('div', 'roster-list grid');
    for (const p of sortPeople(parents)) {
      listBody.append(listRow(thumbUrl(p.photoUrl), '', p.fullName, kidsSummary(p), personLink(p)));
    }
    list.append(listBody);
  }
  content.append(list);
  main.append(content);
}

function renderGradeDetail(slug) {
  const grade = state.model.grades.find(g => slugify(g.name) === slug);
  if (!grade) {
    resetMain(el('div', 'empty', 'Not found.'));
    return;
  }
  const students = studentsOf(p => p.grade === grade.name);
  const classrooms = [...new Set(students.map(s => s.classroom).filter(Boolean))].sort();
  const groups = classrooms.length
    ? classrooms.map(name => ({header: name, students: students.filter(s => s.classroom === name)}))
    : [{header: '', students}];
  renderRoster(grade.name, gradeImage(grade.name), groups);
}

function renderClassroomDetail(slug) {
  const classroom = state.model.classrooms.find(c => slugify(c.name) === slug);
  if (!classroom) {
    resetMain(el('div', 'empty', 'Not found.'));
    return;
  }
  const students = studentsOf(p => p.classroom === classroom.name);
  const crews = [...new Set(students.map(s => s.crew).filter(Boolean))].sort();
  const groups = crews.length
    ? crews.map(name => ({header: `${name} ${classroom.name}`, chipLabel: name, students: students.filter(s => s.crew === name)}))
    : [{header: '', students}];
  renderRoster(classroom.name, classroom.imageUrl, groups);
}

const emailColumns = [
  {label: 'Full Name', get: r => r.p.fullName},
  {label: 'Email', get: r => r.p.email},
  {label: 'Role', get: r => r.role},
  {label: 'Grade', get: r => r.grade},
  {label: 'Classroom', get: r => r.classroom},
];

function csvField(value) {
  return /[",\n]/.test(value) ? '"' + value.replaceAll('"', '""') + '"' : value;
}

function kidsField(parent, field) {
  const family = state.model.families[parent.familyKey];
  const values = ((family && family.kidEmails) || [])
    .map(e => byEmail[e])
    .filter(Boolean)
    .map(k => k[field])
    .filter(Boolean);
  return [...new Set(values)].join(', ');
}

// Every student, their parents, and staff, deduped by email (a parent who's
// also staff keeps whichever role they were added under first). Role/grade/
// classroom filtering down from this full set happens the same way the
// Directory page's Everyone tab does it - via matchesFilters, driven by the
// role chips and the Grade/Classroom/Tags dropdowns.
function emailEntries() {
  const rows = [];
  const seen = new Set();
  const add = (p, role, grade, classroom) => {
    if (!seen.has(p.email)) {
      seen.add(p.email);
      rows.push({p, role, grade, classroom});
    }
  };
  const students = state.model.people
    .filter(p => p.isStudent)
    .sort((a, b) => a.fullName.localeCompare(b.fullName));
  for (const s of students) {
    add(s, 'Student', s.grade || '', s.classroom || '');
    const family = state.model.families[s.familyKey];
    for (const email of (family && family.adultEmails) || []) {
      const parent = byEmail[email];
      if (parent) {
        add(parent, 'Parent', kidsField(parent, 'grade'), kidsField(parent, 'classroom'));
      }
    }
  }
  const staff = state.model.people
    .filter(p => p.isStaff)
    .sort((a, b) => a.fullName.localeCompare(b.fullName));
  for (const s of staff) {
    add(s, 'Staff', '', '');
  }
  return rows;
}

function renderEmailListPage() {
  const main = resetMain();

  const pageHeader = el('div', 'page-header-plain container');
  pageHeader.append(el('h1', 'page-title', 'Email List'));
  pageHeader.append(el('div', 'page-subtitle', 'Use the filters to select for specific grades or classrooms.'));
  main.append(pageHeader);

  const content = el('div', 'content container');
  const header = el('div', 'content-header');
  header.append(roleChips(() => renderTable()));
  const controls = el('div', 'controls');
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    renderTable();
  });
  search.append(input);
  const download = el('a', 'filter-button email-download');
  download.title = 'Download what the table currently shows';
  download.append(svg('download'), el('span', '', 'CSV'));
  controls.append(
    facetDropdown('Grade', gradeOptions(), state.filterGrades, () => renderTable()),
    facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, () => renderTable()),
    facetDropdown('Tags', tagNames(), state.filterTags, () => renderTable()),
    search,
    download,
  );
  header.append(controls);
  content.append(header);

  const holder = el('div', 'email-holder');
  content.append(holder);
  main.append(content);

  // Which columns the corner copy button copies. Persists across search/filter
  // re-renders for this page visit.
  const selectedColumns = new Set(emailColumns.map((c, i) => i));
  let currentRows = [];

  const copyColumns = el('button', 'email-copy-columns');
  copyColumns.title = 'Copy the checked columns to the clipboard';
  copyColumns.append(svg('copy'));
  copyColumns.addEventListener('click', () => {
    const cols = emailColumns.filter((c, i) => selectedColumns.has(i));
    if (!cols.length || !currentRows.length) {
      return;
    }
    const text = [cols.map(c => c.label).join('\t')]
      .concat(currentRows.map(r => cols.map(c => c.get(r)).join('\t')))
      .join('\n');
    navigator.clipboard.writeText(text);
    copyColumns.classList.add('copied');
    copyColumns.replaceChildren(svg('check'));
    setTimeout(() => {
      copyColumns.classList.remove('copied');
      copyColumns.replaceChildren(svg('copy'));
    }, 1200);
  });

  function renderTable() {
    holder.replaceChildren();
    const rows = emailEntries()
      .filter(r => (r.p.fullName.toLowerCase().includes(state.q) || r.p.email.toLowerCase().includes(state.q)) && matchesFilters(r.p));
    currentRows = rows;
    const csv = [emailColumns.map(c => c.label).join(',')]
      .concat(rows.map(r => emailColumns.map(c => csvField(c.get(r))).join(',')))
      .join('\n');
    download.href = 'data:text/csv;charset=utf-8,' + encodeURIComponent(csv);
    download.download = 'email-list.csv';
    if (!rows.length) {
      holder.append(el('div', 'empty', 'No matches.'));
      return;
    }
    const table = el('table', 'email-table');
    const thead = el('thead');
    const headRow = el('tr');
    const leadTh = el('th', 'email-copy-cell');
    leadTh.append(copyColumns);
    headRow.append(leadTh);
    emailColumns.forEach((c, i) => {
      const th = el('th');
      const pick = el('label', 'column-pick');
      const checkbox = el('input');
      checkbox.type = 'checkbox';
      checkbox.checked = selectedColumns.has(i);
      checkbox.addEventListener('change', () => {
        if (checkbox.checked) {
          selectedColumns.add(i);
        } else {
          selectedColumns.delete(i);
        }
      });
      pick.append(el('span', '', c.label), checkbox);
      th.append(pick, copyGlyph(rows.map(c.get).filter(Boolean).join('\n')));
      headRow.append(th);
    });
    headRow.append(el('th'));
    thead.append(headRow);
    table.append(thead);
    const tbody = el('tbody');
    rows.forEach((r, i) => {
      const tr = el('tr');
      const num = el('td', 'email-num');
      num.append(el('span', '', String(i + 1)), copyGlyph(emailColumns.map(c => c.get(r)).join('\t')));
      tr.append(num);
      const nameCell = el('td', 'email-name');
      const nameLink = el('a', '', r.p.fullName);
      nameLink.href = personLink(r.p);
      nameCell.append(nameLink, copyGlyph(r.p.fullName));
      tr.append(nameCell);
      for (const c of emailColumns.slice(1)) {
        const td = el('td', '', c.get(r));
        if (c.get(r)) {
          td.append(copyGlyph(c.get(r)));
        }
        tr.append(td);
      }
      const tagCell = el('td', 'email-tag');
      tagCell.append(tagControl(r.p.email, 'tag-wrap', 'row-tag', () => {
        if (state.filterTags.size) {
          renderTable();
        }
      }));
      tr.append(tagCell);
      tbody.append(tr);
    });
    table.append(tbody);
    holder.append(table);
  }
  renderTable();
  input.focus();
}

const veracrossAddressLabels = {full: 'Full Address', partial: 'Partial (City Only)', hidden: 'Hidden'};
const veracrossPhoneLabels = {visible: 'Visible', mixed: 'Mixed', hidden: 'Hidden'};

// full/visible are Veracross's most-open state (green), hidden is fully closed (red),
// and everything in between - partial or mixed - is the amber middle ground.
function privacyDotColor(state) {
  if (state === 'hidden') {
    return 'red';
  }
  return state === 'full' || state === 'visible' ? 'green' : 'yellow';
}

function privacyVeracrossCell(state, labels) {
  const cell = el('td', 'privacy-cell');
  const inner = el('span', 'privacy-cell-inner');
  inner.append(el('span', `privacy-dot privacy-dot-${privacyDotColor(state)}`), el('span', '', labels[state] || state));
  cell.append(inner);
  return cell;
}

function privacyHeliosCell(masked) {
  const cell = el('td', 'privacy-cell');
  const badge = el('span', `privacy-icon ${masked ? 'privacy-icon-lock' : 'privacy-icon-sync'}`);
  badge.append(svg(masked ? 'lock' : 'sync'));
  const inner = el('span', 'privacy-cell-inner');
  inner.append(badge, el('span', '', masked ? 'Hidden (Override Veracross)' : 'Matches Veracross'));
  cell.append(inner);
  return cell;
}

function privacyRow(label, veracrossState, veracrossLabels, masked, shownValue) {
  const tr = el('tr');
  tr.append(el('td', 'privacy-row-label', label));
  tr.append(privacyVeracrossCell(veracrossState, veracrossLabels));
  tr.append(privacyHeliosCell(masked));
  tr.append(el('td', 'privacy-cell privacy-shown', shownValue || '(Hidden)'));
  return tr;
}

function privacyWarningBanner(label) {
  return infoBanner(
    'alert', 'alert',
    `Your ${label} is visible on Veracross but hidden here`,
    "Hiding it in the Helios Who app does not hide it on Veracross - anyone with Veracross access can still see it there. " +
      "To match what Veracross already shows, sync your Helios Who opt-in.",
    'Sync Now', privacyLinks.heliosWhoOptIn, true);
}

function privacyActionButton(iconName, label, href) {
  const a = el('a', 'media-button primary privacy-action');
  a.href = href;
  a.target = '_blank';
  a.rel = 'noopener';
  a.append(svg(iconName), el('span', '', label));
  return a;
}

function renderPrivacyPage() {
  const main = resetMain();
  const me = byEmail[document.body.dataset.userEmail];
  const family = me && state.model.families[me.familyKey];
  if (!family) {
    main.append(el('div', 'container', 'No family record found for your account.'));
    return;
  }

  for (const label of privacyWarnings(family)) {
    main.append(privacyWarningBanner(label));
  }

  const content = el('div', 'content container privacy-page');
  content.append(el('h1', '', 'Your Privacy'));

  const intro = el('div', 'privacy-intro');
  intro.append(el('p', '',
    "Helios Who's data comes from Veracross, but you can further restrict what's shown here. This means:"));
  const list = el('ul', '');
  list.append(el('li', '', 'Hiding your phone or address here does not hide it on Veracross.'));
  list.append(el('li', '',
    "Matching your Helios Who visibility to Veracross will never show more here than Veracross already shows."));
  intro.append(list);
  const syncNote = el('p', '');
  syncNote.append(
    'To keep these in sync, update the Helios Who opt-in at ',
    (() => {
      const a = el('a', '', privacyLinks.heliosWhoOptIn);
      a.href = privacyLinks.heliosWhoOptIn;
      a.target = '_blank';
      a.rel = 'noopener';
      return a;
    })(),
    ' (check both the address and phone boxes on the second page).');
  intro.append(syncNote);
  content.append(intro);

  const holder = el('div', 'email-holder');
  const table = el('table', 'email-table privacy-table');
  const thead = el('thead');
  const headRow = el('tr');
  for (const label of ['', 'Veracross', 'Helios Who', 'Shown Here']) {
    headRow.append(el('th', '', label));
  }
  thead.append(headRow);
  const tbody = el('tbody');
  tbody.append(privacyRow('Phone', family.veracrossPhone, veracrossPhoneLabels, family.phoneMasked, family.phone));
  tbody.append(privacyRow('Address', family.veracrossAddress, veracrossAddressLabels, family.addressMasked, family.address));
  table.append(thead, tbody);
  holder.append(table);
  content.append(holder);

  const actions = el('div', 'privacy-actions');
  actions.append(
    privacyActionButton('pencil', 'Update Veracross', privacyLinks.veracrossPreferences),
    privacyActionButton('sync', 'Update Helios Who Visibility', privacyLinks.heliosWhoOptIn));
  content.append(actions);

  main.append(content);
}

let mapsPromise = null;

function loadMaps() {
  if (!mapsPromise) {
    mapsPromise = new Promise(resolve => {
      window._mapsReady = resolve;
      const script = el('script');
      script.src = 'https://maps.googleapis.com/maps/api/js?key=' +
        encodeURIComponent(document.body.dataset.mapsKey) + '&callback=_mapsReady';
      script.async = true;
      document.head.append(script);
    });
  }
  return mapsPromise;
}

const pinIcon = 'data:image/svg+xml;charset=UTF-8,' + encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="34" height="34">' +
  '<path d="M20 10c0 4.993-5.539 10.193-7.399 11.799a1 1 0 0 1-1.202 0C9.539 20.193 4 14.993 4 10a8 8 0 0 1 16 0" fill="#173c41" stroke="#fff" stroke-width="1"/>' +
  '<circle cx="12" cy="10" r="3" fill="#fff"/></svg>');

function renderMapPage() {
  const main = resetMain();

  const content = el('div', 'content container');
  const header = el('div', 'content-header');
  header.append(el('h1', '', 'Map'));
  const controls = el('div', 'controls');
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    renderPins();
  });
  search.append(input);
  controls.append(search, filterControl(() => renderPins()));
  header.append(controls);
  content.append(header);

  const canvas = el('div', 'map-canvas');
  content.append(canvas);

  const update = el('div', 'map-update');
  const action = el('a', 'map-update-link');
  action.href = withFrom('/my-privacy');
  action.append(svg('zap'), el('span', '', 'Update My Address'));
  update.append(action);
  content.append(update);
  main.append(content);

  let map = null;
  let info = null;
  let markers = [];

  function familySearchText(family) {
    const members = [...(family.kidEmails || []), ...(family.adultEmails || [])]
      .map(e => byEmail[e]).filter(Boolean).map(p => p.fullName);
    return `${family.name || ''} ${members.join(' ')}`.toLowerCase();
  }

  function popupContent(family) {
    const box = el('div', 'map-popup');
    if (family.photoUrl) {
      const img = el('img', 'map-popup-photo');
      img.src = thumbUrl(family.photoUrl);
      img.alt = '';
      box.append(img);
    }
    const body = el('div', 'map-popup-body');
    body.append(el('div', 'map-popup-name', family.name));
    if (family.address) {
      body.append(el('div', 'map-popup-sub', family.address));
    }
    const link = el('a', 'map-popup-link', 'See family');
    link.href = familyLink(family.key);
    body.append(link);
    box.append(body);
    return box;
  }

  function renderPins() {
    if (!map) {
      return;
    }
    for (const m of markers) {
      m.setMap(null);
    }
    markers = [];
    for (const family of Object.values(state.model.families)) {
      if (!family.lat && !family.lng) {
        continue;
      }
      if (!familyMatchesFilters(family.key) || !familySearchText(family).includes(state.q)) {
        continue;
      }
      const marker = new google.maps.Marker({
        map,
        position: {lat: family.lat, lng: family.lng},
        icon: {url: pinIcon, anchor: new google.maps.Point(17, 33)},
        title: family.name,
      });
      marker.addListener('click', () => {
        info.setContent(popupContent(family));
        info.open(map, marker);
      });
      markers.push(marker);
    }
  }

  loadMaps().then(() => {
    if (!canvas.isConnected) {
      return;
    }
    map = new google.maps.Map(canvas, {
      mapTypeControl: false,
      streetViewControl: false,
      fullscreenControl: false,
    });
    info = new google.maps.InfoWindow({headerDisabled: true});
    map.addListener('click', () => info.close());
    const bounds = new google.maps.LatLngBounds();
    for (const family of Object.values(state.model.families)) {
      if (family.lat || family.lng) {
        bounds.extend({lat: family.lat, lng: family.lng});
      }
    }
    map.fitBounds(bounds);
    renderPins();
  });
}

async function submitMedia(target, key, kind, file, name, status) {
  status.classList.remove('error');
  status.textContent = 'Uploading…';
  const form = new FormData();
  form.append('target', target);
  form.append('key', key);
  form.append('kind', kind);
  form.append('file', file, name);
  const res = await fetch('/api/directory/upload', {method: 'POST', body: form});
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return;
  }
  await load();
}

function canEditPerson(email) {
  const meEmail = document.body.dataset.userEmail;
  if (email === meEmail || state.model.superEdit) {
    return true;
  }
  const me = byEmail[meEmail];
  const family = me && state.model.families[me.familyKey];
  return Boolean(family && [...(family.kidEmails || []), ...(family.adultEmails || [])].includes(email));
}

// photoSwitcher shows every photo a person has and swaps the big one on click. Which
// one the directory shows everywhere else is a separate question, and only somebody who
// may edit the record gets to answer it.
function photoSwitcher(p, img, editable) {
  const box = el('div', 'photo-switcher');
  const strip = el('div', 'photo-strip');
  const status = el('div', 'media-status');
  const choose = el('button', 'media-button', 'Show this one everywhere');
  let showing = p.primaryPhoto;
  // Hold the frame at the tallest of their photos, measured from the thumbnails the
  // strip loads anyway. Without it a switch changes the frame's height and everything
  // below it jumps.
  const shapes = [];
  const holdFrame = () => {
    img.parentElement.classList.add('photo-fixed');
    img.parentElement.style.aspectRatio = String(Math.min(...shapes));
  };
  const paint = () => {
    for (const thumb of strip.children) {
      thumb.classList.toggle('current', thumb.dataset.name === showing);
    }
    choose.hidden = showing === p.primaryPhoto;
  };
  for (const photo of p.photos) {
    const thumb = el('button', 'photo-thumb');
    thumb.dataset.name = photo.name;
    thumb.title = photo.source === 'veracross' ? 'School portrait' : 'Uploaded photo';
    const face = el('img');
    face.src = thumbUrl(photo.url);
    face.alt = '';
    face.addEventListener('load', () => {
      if (face.naturalHeight) {
        shapes.push(face.naturalWidth / face.naturalHeight);
        holdFrame();
      }
    });
    thumb.append(face);
    thumb.addEventListener('click', () => {
      img.src = photo.url;
      showing = photo.name;
      paint();
    });
    strip.append(thumb);
  }
  box.append(strip);
  if (editable) {
    box.append(choose, status);
    choose.addEventListener('click', () => submitField(p.email, 'primary-photo', showing, status));
  }
  paint();
  return box;
}

function uploadIcon(iconName, title, accept, target, key, kind, status) {
  const wrap = el('label', 'edit-icon');
  wrap.title = title;
  wrap.append(svg(iconName));
  const input = el('input');
  input.type = 'file';
  input.accept = accept;
  input.hidden = true;
  input.addEventListener('change', () => {
    if (input.files.length) {
      submitMedia(target, key, kind, input.files[0], input.files[0].name, status);
    }
  });
  wrap.append(input);
  return wrap;
}

function recordIcon(target, key, status, preview) {
  const button = el('button', 'edit-icon');
  button.title = 'Record pronunciation';
  button.append(svg('mic'));
  let recorder = null;
  button.addEventListener('click', async () => {
    if (recorder) {
      recorder.stop();
      return;
    }
    let stream;
    try {
      stream = await navigator.mediaDevices.getUserMedia({audio: true});
    } catch (err) {
      status.classList.add('error');
      status.textContent = 'microphone unavailable: ' + err.message;
      return;
    }
    status.classList.remove('error');
    status.textContent = 'Recording… tap the microphone again to stop';
    const chunks = [];
    recorder = new MediaRecorder(stream);
    recorder.addEventListener('dataavailable', e => chunks.push(e.data));
    recorder.addEventListener('stop', () => {
      for (const track of stream.getTracks()) {
        track.stop();
      }
      const blob = new Blob(chunks, {type: recorder.mimeType || 'audio/webm'});
      recorder = null;
      button.classList.remove('recording');
      status.textContent = '';
      preview.replaceChildren();
      const audio = el('audio');
      audio.controls = true;
      audio.src = URL.createObjectURL(blob);
      const save = el('button', 'media-button primary', 'Save');
      save.addEventListener('click', () => submitMedia(target, key, 'pronunciation', blob, 'recording', status));
      const discard = el('button', 'media-button', 'Discard');
      discard.addEventListener('click', () => preview.replaceChildren());
      preview.append(audio, save, discard);
    });
    recorder.start();
    button.classList.add('recording');
  });
  return button;
}

function pronounceEditor(target, key) {
  const box = el('div', 'pronounce-edit');
  const actions = el('div', 'pronounce-actions');
  const status = el('div', 'media-status');
  const preview = el('div', 'record-preview');
  actions.append(recordIcon(target, key, status, preview));
  actions.append(uploadIcon('upload', 'Upload an audio file', 'audio/*', target, key, 'pronunciation', status));
  box.append(actions, status, preview);
  return box;
}

function tabParam(fallback) {
  return new URLSearchParams(location.search).get('tab') || fallback;
}

function tabNode(item, active, onSelect) {
  const node = el('div', 'tab' + (active ? ' active' : ''));
  if (item.icon) {
    node.append(svg(item.icon));
  }
  node.append(el('span', '', item.label));
  if (item.count !== undefined) {
    node.append(el('span', 'tab-count', String(item.count)));
  }
  node.addEventListener('click', () => onSelect(item.key));
  return node;
}

function tabStrip(items, activeKey, mobileVisible, onSelect) {
  const tabs = el('div', 'tabs');
  const row = el('div', 'container tabs-row');
  const visible = isMobile() ? items.slice(0, mobileVisible) : items;
  const hidden = isMobile() ? items.slice(mobileVisible) : [];
  for (const item of visible) {
    row.append(tabNode(item, item.key === activeKey, onSelect));
  }
  if (hidden.length) {
    const wrap = el('div', 'more-wrap');
    const more = el('div', 'tab' + (hidden.some(t => t.key === activeKey) ? ' active' : ''));
    more.append(svg('more'), el('span', '', 'More'), svg('chevron'));
    const menu = el('div', 'more-menu');
    menu.hidden = true;
    for (const item of hidden) {
      const entry = el('div', 'more-item' + (item.key === activeKey ? ' active' : ''));
      if (item.icon) {
        entry.append(svg(item.icon));
      }
      entry.append(el('span', '', item.label));
      if (item.count !== undefined) {
        entry.append(el('span', 'tab-count', String(item.count)));
      }
      entry.addEventListener('click', () => onSelect(item.key));
      menu.append(entry);
    }
    more.addEventListener('click', e => {
      e.stopPropagation();
      menu.hidden = !menu.hidden;
    });
    wrap.append(more, menu);
    row.append(wrap);
  }
  tabs.append(row);
  return tabs;
}

function tabHref(key) {
  const params = new URLSearchParams(location.search);
  params.set('tab', key);
  return location.pathname + '?' + params;
}

const sectionTitles = {
  people: 'People',
  classrooms: 'Classrooms',
  'my-family': 'My Family',
  staff: 'Staff',
  map: 'Map',
  'email-list': 'Email List',
  'my-privacy': 'My Privacy',
};

function render() {
  renderNav();
  setChrome(sectionTitles[segments()[0]] || 'Helios Who?', null);
  const seg = segments();
  if (seg[0] === 'people' && seg[1]) {
    renderPersonDetail(seg[1]);
  } else if (seg[0] === 'families' && seg[1]) {
    renderFamilyDetail(seg[1]);
  } else if (seg[0] === 'people') {
    state.tab = tabParam('everyone');
    const tagParam = new URLSearchParams(location.search).get('tag');
    if (tagParam) {
      state.filterTags = new Set([tagParam]);
    }
    renderPeople();
  } else if (seg[0] === 'classrooms' && seg[1]) {
    state.rosterTab = tabParam('students');
    renderClassroomDetail(seg[1]);
  } else if (seg[0] === 'grades' && seg[1]) {
    state.rosterTab = tabParam('students');
    renderGradeDetail(seg[1]);
  } else if (seg[0] === 'classrooms') {
    state.classTab = tabParam('by-classroom');
    renderClassroomsPage();
  } else if (seg[0] === 'staff') {
    state.q = '';
    renderStaffPage();
  } else if (seg[0] === 'email-list') {
    state.q = '';
    renderEmailListPage();
  } else if (seg[0] === 'map') {
    state.q = '';
    renderMapPage();
  } else if (seg[0] === 'my-privacy') {
    renderPrivacyPage();
  }
  finishRender();
}

// Wraps everything the render just built so it can act as the flexible
// sticky-footer spacer: on a short page it grows to push the art down to the true
// bottom of the viewport, and on a tall page it just yields to scrolling. Called
// both after the top-level render() dispatch and after any in-page tab switch that
// re-renders by calling its render*() function directly instead of going through
// render() - those bypass this otherwise, leaving the footer art stuck from
// whatever page loaded first (or missing it entirely).
function finishRender() {
  const main = document.querySelector('#main');
  const contentWrap = el('div', 'page-content-wrap');
  contentWrap.append(...main.childNodes);
  main.append(contentWrap, el('div', 'page-footer-art'));
}

// The topbar only has room for the avatar (no name label), so - unlike the old
// sidebar row, which let a name click open the menu and an avatar click jump
// straight to the profile - the avatar's only job now is opening the menu, whose
// first item is "View Profile".
const userMenu = document.querySelector('#user-menu');
document.querySelector('#user').addEventListener('click', e => {
  e.stopPropagation();
  userMenu.hidden = !userMenu.hidden;
});

const drawer = document.querySelector('#drawer');
const drawerOverlay = document.querySelector('#drawer-overlay');
const drawerUserMenu = document.querySelector('#drawer-user-menu');

function setDrawer(open) {
  drawer.hidden = !open;
  drawerOverlay.hidden = !open;
  if (!open) {
    drawerUserMenu.hidden = true;
  }
}

document.querySelector('#mobile-menu-btn').addEventListener('click', () => setDrawer(true));
document.querySelector('#drawer-close').addEventListener('click', () => setDrawer(false));
drawerOverlay.addEventListener('click', () => setDrawer(false));
document.querySelector('#drawer-user-more').addEventListener('click', e => {
  e.stopPropagation();
  drawerUserMenu.hidden = !drawerUserMenu.hidden;
});

const topbarSearchInput = document.querySelector('#topbar-search-input');
const topbarSearchResults = document.querySelector('#topbar-search-results');
topbarSearchInput.addEventListener('input', () => {
  renderGlobalSearchResults(topbarSearchResults, topbarSearchInput.value);
});
topbarSearchInput.addEventListener('focus', () => {
  if (topbarSearchInput.value.trim()) {
    renderGlobalSearchResults(topbarSearchResults, topbarSearchInput.value);
  }
});

const mobileSearchOverlay = document.querySelector('#mobile-search-overlay');
const mobileSearchInput = document.querySelector('#mobile-search-input');
const mobileSearchResults = document.querySelector('#mobile-search-results');
function setMobileSearch(open) {
  mobileSearchOverlay.hidden = !open;
  if (open) {
    mobileSearchInput.value = '';
    mobileSearchResults.replaceChildren();
    mobileSearchInput.focus();
  }
}
document.querySelector('#mobile-search-btn').addEventListener('click', () => setMobileSearch(true));
document.querySelector('#mobile-search-close').addEventListener('click', () => setMobileSearch(false));
mobileSearchInput.addEventListener('input', () => {
  renderGlobalSearchResults(mobileSearchResults, mobileSearchInput.value);
});
function closeFilterPanels() {
  for (const panel of document.querySelectorAll('.filter-panel')) {
    panel.hidden = true;
    panel.parentElement.querySelector('.filter-button').classList.remove('open');
  }
}

document.addEventListener('click', e => {
  userMenu.hidden = true;
  drawerUserMenu.hidden = true;
  if (!topbarSearchResults.hidden && !e.target.closest('.topbar-search')) {
    topbarSearchResults.hidden = true;
  }
  for (const menu of document.querySelectorAll('.more-menu, .card-menu')) {
    if (!menu.hidden && !menu.parentElement.contains(e.target)) {
      menu.hidden = true;
    }
  }
  for (const panel of document.querySelectorAll('.filter-panel')) {
    if (!panel.hidden && !panel.parentElement.contains(e.target)) {
      panel.hidden = true;
      panel.parentElement.querySelector('.filter-button').classList.remove('open');
    }
  }
});
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    userMenu.hidden = true;
    setDrawer(false);
    setMobileSearch(false);
    topbarSearchResults.hidden = true;
    closeFilterPanels();
    for (const menu of document.querySelectorAll('.more-menu, .card-menu')) {
      menu.hidden = true;
    }
  }
});

// Both banners live in one fixed-position stack so they pile up in normal flow
// instead of both claiming top:0 and hiding one another — which is exactly what
// happened once spoofing stopped being mutually exclusive with Super Edit Mode.
function topBanners() {
  let stack = document.querySelector('#top-banners');
  if (!stack) {
    stack = el('div', '');
    stack.id = 'top-banners';
    document.body.append(stack);
  }
  return stack;
}

function updateBannerOffset() {
  const stack = document.querySelector('#top-banners');
  document.documentElement.style.setProperty('--banner-h', stack ? stack.offsetHeight + 'px' : '0px');
}
window.addEventListener('resize', updateBannerOffset);

function renderSuperEditBanner() {
  let banner = document.querySelector('.super-edit-banner');
  if (!state.model.superEdit) {
    if (banner) {
      banner.remove();
      updateBannerOffset();
    }
    return;
  }
  if (banner) {
    return;
  }
  banner = el('div', 'super-edit-banner');
  banner.append(el('span', '', 'Super Edit Mode is on — you can edit anyone’s info.'));
  const link = el('a', '', 'Turn off');
  link.href = '#';
  link.addEventListener('click', async e => {
    e.preventDefault();
    await fetch('/api/admin/super-edit', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({enabled: false}),
    });
    await load();
  });
  banner.append(link);
  topBanners().append(banner);
  updateBannerOffset();
}

function renderSpoofBanner() {
  let banner = document.querySelector('.spoof-banner');
  if (!state.model.spoofingAs) {
    if (banner) {
      banner.remove();
      updateBannerOffset();
    }
    return;
  }
  if (banner) {
    banner.querySelector('.spoof-banner-name').textContent = state.model.spoofingAs;
    updateBannerOffset();
    return;
  }
  banner = el('div', 'spoof-banner');
  banner.append(el('span', '', 'Viewing as '));
  banner.append(el('span', 'spoof-banner-name', state.model.spoofingAs));
  const link = el('a', '', 'Stop');
  link.href = '#';
  link.addEventListener('click', async e => {
    e.preventDefault();
    await fetch('/api/admin/spoof', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({email: ''}),
    });
    // A full reload, not load(): stopping spoofing changes the effective identity the
    // server renders into the page itself (username, userEmail, the Admin Tools menu
    // link), not just the JSON model a plain re-fetch would refresh.
    location.reload();
  });
  banner.append(link);
  topBanners().append(banner);
  updateBannerOffset();
}

// The one signal this whole page exists to catch: Veracross is still showing
// something to the wider parent community that the family thinks they've hidden by
// hiding it in Helios Who. Hiding it here only ever removes it from this app - never
// from Veracross. Shared by the My Privacy page, the account-menu badge and the
// dismissible summary card so the three agree on what counts as a mismatch.
function privacyWarnings(family) {
  const warnings = [];
  if (family.addressMasked && family.veracrossAddress !== 'hidden') {
    warnings.push('address');
  }
  if (family.phoneMasked && family.veracrossPhone !== 'hidden') {
    warnings.push('phone number');
  }
  return warnings;
}

function myPrivacyWarnings() {
  const me = byEmail[document.body.dataset.userEmail];
  const family = me && state.model.families[me.familyKey];
  return family ? privacyWarnings(family) : [];
}

function renderPrivacyMenuAlert() {
  const hasMismatch = myPrivacyWarnings().length > 0;
  const staleCount = staleItems().length;
  const hasStale = staleCount > 0;

  for (const badge of document.querySelectorAll('.user-menu-alert')) {
    badge.hidden = !hasMismatch;
    if (hasMismatch && !badge.firstChild) {
      badge.append(svg('alert'));
    }
  }

  // The mobile drawer only has room for one dot next to the name, so it stays a
  // single merged signal; the desktop topbar has room for two separate, clickable
  // icons - one per condition - so each links straight to where you'd fix it.
  for (const badge of document.querySelectorAll('.user-row-alert')) {
    badge.hidden = !(hasMismatch || hasStale);
    badge.title = hasMismatch && hasStale ? 'Some family info is out of date, and your privacy settings don’t match Veracross'
      : hasMismatch ? 'Your privacy settings don’t match Veracross'
      : 'Some family info is missing or out of date';
    if ((hasMismatch || hasStale) && !badge.firstChild) {
      badge.append(svg('alert'));
    }
  }

  const staleButton = document.querySelector('#topbar-stale-alert');
  if (staleButton) {
    staleButton.hidden = !hasStale;
    staleButton.title = `${staleCount} thing${staleCount === 1 ? '' : 's'} to update for the new year`;
    document.querySelector('#topbar-stale-count').textContent = String(staleCount);
  }
  const privacyButton = document.querySelector('#topbar-privacy-alert');
  if (privacyButton) {
    privacyButton.hidden = !hasMismatch;
  }
}

function privacyMismatchCardDismissed() {
  try {
    return localStorage.getItem('privacyMismatchCardDismissed') === '1';
  } catch (e) {
    return false;
  }
}

function privacyMismatchCard(warnings) {
  const desc = `Your ${warnings.join(' and ')} ${warnings.length === 1 ? 'is' : 'are'} visible on ` +
    'Veracross but hidden in Helios Who. Hiding it here does not hide it on Veracross.';
  return infoBanner(
    'alert', 'alert', 'Privacy Settings Mismatch', desc,
    'See Details', '/my-privacy', false,
    () => {
      try {
        localStorage.setItem('privacyMismatchCardDismissed', '1');
      } catch (e) {
        // ignore - the card just won't stay dismissed across reloads
      }
    });
}

async function load() {
  const res = await fetch('/api/directory/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  state.model = await res.json();
  staleYears = state.model.staleYears || staleYears;
  privacyLinks = state.model.privacyLinks || privacyLinks;
  tags = state.model.tags || {};
  byEmail = {};
  for (const p of state.model.people) {
    byEmail[p.email] = p;
  }
  state.everyoneOrder = shuffled(state.model.people);
  state.familyOrder = shuffled(familyEntries());
  renderSuperEditBanner();
  renderSpoofBanner();
  renderPrivacyMenuAlert();
  render();
}

load();
