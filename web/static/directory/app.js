const state = {model: null, tab: 'everyone', classTab: 'by-classroom', rosterTab: 'students', q: '', filterGrades: new Set(), filterClassrooms: new Set(), filterRoles: new Set(), filterCities: new Set(), filterPronouns: new Set(), filterTags: new Set(), filterNew: false};
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
  students: '<svg viewBox="0 0 24 24"><circle cx="8.5" cy="5.5" r="2"/><path d="M8.5 7.5v5M8.5 12.5l-2.5 5M8.5 12.5l2.5 5M5 9.5l3.5 1 3.5-1"/><circle cx="16.5" cy="7" r="1.7"/><path d="M16.5 8.7v4.3M16.5 13l-2 4M16.5 13l2 4M13.8 10.5l2.7.8 2.7-.8"/></svg>',
  families: '<svg viewBox="0 0 24 24"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
  'staff-tab': '<svg viewBox="0 0 24 24"><line x1="10" x2="14" y1="2" y2="2"/><line x1="12" x2="15" y1="14" y2="11"/><circle cx="12" cy="14" r="8"/></svg>',
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
};

function isMobile() {
  return matchMedia('(max-width: 900px)').matches;
}

const navSections = [
  {path: 'people', label: 'People'},
  {path: 'classrooms', label: 'Classrooms'},
  {path: 'my-family', label: 'My Family'},
  {path: 'staff', label: 'Staff'},
  {path: 'map', label: 'Map'},
  {path: 'email-list', label: 'Email List'},
];

const peopleTabs = [
  {key: 'everyone', label: 'Everyone'},
  {key: 'students', label: 'Students'},
  {key: 'families', label: 'Families'},
  {key: 'staff', label: 'Staff'},
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

function personLink(p) {
  return withFrom('/people/' + encodeURIComponent(p.email));
}

function familyLink(key) {
  return withFrom('/families/' + encodeURIComponent(key));
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

const optInForm = 'https://docs.google.com/forms/d/e/1FAIpQLSehrwYXWLJ6LK5_0f5ccdIA1gF0q7jAeDMxV5FWb_Myr4uRog/viewform';

function optInBanner() {
  const banner = el('div', 'optin');
  const inner = el('div', 'optin-inner container');
  inner.append(el('h2', '', 'Help! Opt-In Required'));
  inner.append(el('p', '', 'You have not yet opted into the Helios Community Apps and will lose access on Sept 1. Please opt-in. Thank you!'));
  const action = el('a', 'optin-button', 'Opt-In');
  action.href = optInForm;
  action.target = '_blank';
  action.rel = 'noopener';
  inner.append(action);
  banner.append(inner);
  return banner;
}

const staleYears = {photo: 0.75, facts: 0.6, familyPhoto: 1.5};

function agedPast(present, updated, years) {
  if (!present) {
    return false;
  }
  const when = Date.parse(updated);
  return Number.isNaN(when) || Date.now() - when > years * 365.25 * 24 * 60 * 60 * 1000;
}

function staleItems() {
  const me = byEmail[document.body.dataset.userEmail];
  if (!me) {
    return [];
  }
  const family = state.model.families[me.familyKey];
  const kids = ((family && family.kidEmails) || []).map(e => byEmail[e]).filter(Boolean);
  const items = [];
  for (const p of [me, ...kids.filter(k => k.email !== me.email)].filter(p => p.isStudent)) {
    const whose = p.email === me.email ? 'your' : `${p.fullName}'s`;
    const href = personLink(p) + '&edit=1';
    if (agedPast(p.photoUrl, p.photoUpdated, staleYears.photo)) {
      items.push({text: `Update ${whose} photo for new year`, href});
    }
    if (agedPast(p.facts, p.factsUpdated, staleYears.facts)) {
      items.push({text: `Update ${whose} facts for new year`, href});
    }
  }
  if (family && agedPast(family.photoUrl, family.photoUpdated, staleYears.familyPhoto)) {
    items.push({text: 'Update your family photo for new year', href: familyLink(family.key)});
  }
  return items;
}

function staleBanner(items) {
  const banner = el('div', 'stale');
  const inner = el('div', 'stale-inner container');
  for (const item of items) {
    const row = el('a', 'stale-row');
    row.href = item.href;
    row.append(el('div', 'stale-mark'));
    row.append(el('div', 'stale-text', item.text));
    const chev = el('div', 'member-chevron');
    chev.append(svg('chevron-right'));
    row.append(chev);
    inner.append(row);
  }
  const action = el('a', 'stale-button', 'Update Family Info');
  action.href = '/my-family';
  inner.append(action);
  banner.append(inner);
  return banner;
}

function resetMain(...children) {
  const main = document.querySelector('#main');
  main.replaceChildren();
  const me = byEmail[document.body.dataset.userEmail];
  if (me && me.optStatus === 'default') {
    main.append(optInBanner());
  }
  const stale = staleItems();
  if (stale.length) {
    main.append(staleBanner(stale));
  }
  main.append(...children);
  return main;
}

function renderNav() {
  const seg = activeSection();
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  for (const item of navSections) {
    if (item.divider) {
      nav.append(el('div', 'nav-divider'));
      continue;
    }
    const a = el('a');
    a.href = '/' + item.path;
    if (item.path === seg) {
      a.className = 'active';
    }
    a.append(svg(item.path), el('span', '', item.label));
    nav.append(a);
  }
  const tabs = document.querySelector('#mobile-tabs');
  tabs.replaceChildren();
  for (const item of navSections) {
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
  div.style.background = `hsl(${hue(name)} 45% 55%)`;
  return div;
}

function roleLabel(p) {
  let role = 'Parent';
  if (p.isStudent) {
    role = 'Student';
  } else if (p.isStaff) {
    role = 'Staff';
  }
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
  card.append(photoOrInitials(p.photoUrl, p.fullName, 'person-photo'));
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
  const matches = state.model.people.filter(p => {
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

function familyEntries() {
  const entries = Object.values(state.model.families).map(f => {
    const members = [...(f.kidEmails || []), ...(f.adultEmails || [])];
    const kidGrades = [...new Set((f.kidEmails || []).map(e => byEmail[e]?.grade).filter(Boolean))];
    return {
      key: f.key,
      name: (f.name || '').replace(/ Family$/, ''),
      label: kidGrades.length ? kidGrades.join(', ') : 'Staff',
      members: members.map(e => byEmail[e] ? firstName(byEmail[e].fullName) : '').filter(Boolean),
      photoUrl: f.photoUrl,
      href: familyLink(f.key),
    };
  });
  for (const p of state.model.people) {
    if (p.isStaff && !p.isParent && !p.isStudent && !state.model.families[p.familyKey]) {
      entries.push({
        name: p.fullName.trim().split(/\s+/).slice(-1)[0],
        label: 'Staff',
        members: [firstName(p.fullName)],
        photoUrl: p.photoUrl,
        href: personLink(p),
        email: p.email,
      });
    }
  }
  entries.sort((a, b) => a.name.localeCompare(b.name));
  return entries;
}

function renderFamilies(grid) {
  grid.className = 'family-grid';
  const matches = familyEntries().filter(f =>
    `${f.name} ${f.members.join(' ')}`.toLowerCase().includes(state.q) &&
    (f.key ? familyMatchesFilters(f.key) : matchesFilters(byEmail[f.email])));
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
      card.append(photoOrInitials(p.photoUrl, p.fullName, 'person-photo'));
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

  const items = peopleTabs.map(t => ({...t, icon: t.key === 'staff' ? 'staff-tab' : t.key}));
  main.append(tabStrip(items, state.tab, 2, key => {
    state.tab = key;
    state.q = '';
    history.replaceState(null, '', tabHref(key));
    renderPeople();
  }));

  const content = el('div', 'content container');
  const header = el('div', 'content-header');
  header.append(el('h1', '', peopleTabs.find(t => t.key === state.tab).label));
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
  controls.append(search, filterControl(renderGrid));
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
    state.filterCities.size || state.filterPronouns.size || state.filterTags.size || state.filterNew);
}

function matchesFilters(p) {
  const gradeOK = !state.filterGrades.size || personFacets(p, 'grade').some(g => state.filterGrades.has(g));
  const classOK = !state.filterClassrooms.size || personFacets(p, 'classroom').some(c => state.filterClassrooms.has(c));
  const roleOK = !state.filterRoles.size ||
    (state.filterRoles.has('Student') && p.isStudent) ||
    (state.filterRoles.has('Parent') && p.isParent) ||
    (state.filterRoles.has('Staff') && p.isStaff);
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

function filterControl(rerender) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button');
  button.append(svg('filter'), el('span', '', 'Filter'), svg('chevron'));
  const panel = el('div', 'filter-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
  });

  const sections = [
    {label: 'Role', values: ['Student', 'Parent', 'Staff'], set: state.filterRoles},
    {label: 'Class', values: state.model.classrooms.map(c => c.name), set: state.filterClassrooms},
    {label: 'Grade', values: gradeOptions(), set: state.filterGrades},
    {label: 'City', values: cityOptions(), set: state.filterCities},
    {label: 'Pronouns', values: pronounOptions(), set: state.filterPronouns},
    {label: 'Tags', values: tagNames(), set: state.filterTags},
  ];
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

  const footer = el('div', 'filter-footer');
  const clear = el('button', 'filter-clear', 'Clear all');
  clear.addEventListener('click', () => {
    state.filterGrades.clear();
    state.filterClassrooms.clear();
    state.filterRoles.clear();
    state.filterCities.clear();
    state.filterPronouns.clear();
    state.filterTags.clear();
    state.filterNew = false;
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
    top.append(tagControl(tagEmail, 'tag-wrap', 'tag-button', () => {}));
  }
  return top;
}

function iconButton(name, label, action) {
  const node = el(typeof action === 'string' ? 'a' : 'button', 'icon-button');
  if (typeof action === 'string') {
    node.href = action;
    node.target = '_blank';
  } else if (action) {
    node.addEventListener('click', action);
  }
  node.append(svg(name), el('span', '', label));
  return node;
}

function copyButton(text) {
  return iconButton('copy', 'Copy', () => navigator.clipboard.writeText(text));
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

const photoSourceLabels = {upload: 'Uploaded photo', veracross: 'School photo'};

// The switcher only changes what this viewer is looking at. Making a choice stick for
// everyone else is a separate, deliberate action, and only the person themselves can
// take it.
function photoSources(p, img, editable, status) {
  const row = el('div', 'photo-sources');
  const primary = (p.photos.find((s) => s.url === p.photoUrl) || p.photos[0]).source;
  let showing = primary;
  const buttons = new Map();
  const use = el('button', 'photo-source-use');
  use.textContent = 'Show this one to everyone';
  const refresh = () => {
    for (const [source, btn] of buttons) {
      btn.classList.toggle('selected', source === showing);
    }
    use.hidden = !editable || showing === primary;
  };
  for (const photo of p.photos) {
    const btn = el('button', 'photo-source');
    btn.textContent = photoSourceLabels[photo.source] || photo.source;
    btn.addEventListener('click', () => {
      showing = photo.source;
      img.src = photo.url;
      refresh();
    });
    buttons.set(photo.source, btn);
    row.append(btn);
  }
  use.addEventListener('click', () => submitField(p.email, 'primary-photo', showing, status));
  row.append(use);
  refresh();
  return row;
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

function memberRow(p, label, sub) {
  const row = el('a', 'member-row');
  row.href = personLink(p);
  if (p.photoUrl) {
    const img = el('img', 'member-thumb');
    img.src = thumbUrl(p.photoUrl);
    img.loading = 'lazy';
    img.alt = '';
    row.append(img);
  }
  const info = el('div', 'member-info');
  if (label) {
    info.append(el('div', 'member-label', label));
  }
  info.append(el('div', 'member-name', p.fullName));
  if (sub) {
    info.append(el('div', 'member-email', sub));
  }
  row.append(info);
  const chev = el('div', 'member-chevron');
  chev.append(svg('chevron-right'));
  row.append(chev);
  return row;
}

function familyBand(p, family) {
  const band = el('div', 'band');
  const grid = el('div', 'band-grid container');
  const left = el('div', 'band-left');
  left.append(el('h2', '', `${p.fullName}'s Family`));
  if (family.photoUrl && !p.isStudent) {
    const img = el('img', 'band-photo');
    img.src = thumbUrl(family.photoUrl);
    img.alt = '';
    left.append(img);
  }
  if (family.photoCaption) {
    left.append(el('div', 'band-caption', family.photoCaption));
  }
  grid.append(left);

  const right = el('div', 'band-right');
  const kidsList = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
  if (kidsList.length && !(p.isStudent && kidsList.length === 1 && kidsList[0].email === p.email)) {
    right.append(el('div', 'member-header', 'Kids'));
    for (const kid of kidsList) {
      if (p.isStudent && kid.email === p.email) {
        continue;
      }
      const label = kid.pronouns ? `${gradeChain(kid)} (${kid.pronouns})` : gradeChain(kid);
      right.append(memberRow(kid, label.toUpperCase(), kid.email));
    }
  }
  const adults = (family.adultEmails || []).map(e => byEmail[e]).filter(Boolean).filter(a => a.email !== p.email);
  if (adults.length) {
    right.append(el('div', 'member-header', 'Adults'));
    for (const adult of adults) {
      const label = [adult.phone, adult.pronouns ? `(${adult.pronouns.toUpperCase()})` : ''].filter(Boolean).join(' ');
      right.append(memberRow(adult, label, adult.email));
    }
  }
  const see = el('a', 'see-family');
  see.href = familyLink(family.key);
  see.append(el('span', '', `See ${family.name}`));
  const chev = el('div', 'member-chevron');
  chev.append(svg('chevron-right'));
  see.append(chev);
  right.append(see);
  grid.append(right);
  band.append(grid);
  return band;
}

let personEdit = null;

function renderPersonDetail(email) {
  const main = resetMain();
  const p = byEmail[email];
  if (!p) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
  }
  const params = new URLSearchParams(location.search);
  if (params.get('edit') === '1') {
    params.delete('edit');
    const query = params.toString();
    history.replaceState(null, '', location.pathname + (query ? '?' + query : ''));
    personEdit = email;
  }
  const origin = fromCrumbs() || [['People', '/people']];
  main.append(breadcrumbs([...origin, [p.fullName, null]], p.email));

  const content = el('div', 'container detail-content');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  const editable = canEditPerson(p.email);
  const editing = editable && personEdit === p.email;
  if (p.photoUrl || editing) {
    const wrap = el('div', 'photo-wrap');
    let img = null;
    if (p.photoUrl) {
      img = el('img', 'detail-photo');
      img.src = p.photoUrl;
      img.alt = '';
      wrap.append(img);
    } else {
      wrap.append(el('div', 'detail-photo detail-photo-empty'));
    }
    left.append(wrap);
    const status = el('div', 'media-status');
    if (editing) {
      wrap.append(uploadIcon('camera', 'Upload photo', 'image/*', 'person', p.email, 'photo', status));
    }
    // Anyone may look through the photos a person has; only they choose which one
    // the rest of the directory sees.
    if (img && (p.photos || []).length > 1) {
      left.append(photoSources(p, img, editable, status));
    }
    if (editing || status.textContent) {
      left.append(status);
    }
  }
  grid.append(left);

  const right = el('div');
  const topRow = el('div', 'detail-top');
  topRow.append(el('div', 'role-label', roleLabel(p)));
  if (editable) {
    const toggle = el('button', 'media-button edit-toggle', editing ? 'Done' : 'Edit info');
    toggle.addEventListener('click', () => {
      personEdit = editing ? null : p.email;
      renderPersonDetail(email);
    });
    topRow.append(toggle);
  }
  right.append(topRow);
  const nameHeader = el('h1', 'detail-name');
  nameHeader.append(el('span', '', p.fullName));
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
      copyButton(p.phone),
      iconButton('message', 'Text', 'sms:' + p.phone),
      iconButton('phone', 'Call', 'tel:' + p.phone),
    ] : [];
    const phoneValue = el('div', 'contact-value editable-value');
    phoneValue.append(el('span', '', p.phone || 'No phone number'));
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
  right.append(contactRow(el('div', 'contact-value', p.email), [
    copyButton(p.email),
    iconButton('mail', 'Email', 'mailto:' + p.email),
  ]));
  const family = state.model.families[p.familyKey];
  const addressEditable = editing && family && p.email === document.body.dataset.userEmail &&
    (family.adultEmails || []).includes(p.email);
  if (family && (family.address || addressEditable)) {
    const block = el('div');
    block.append(el('div', 'field-label', 'Address'));
    const addressValue = el('div', 'contact-value editable-value');
    addressValue.append(el('span', '', family.address || 'No address'));
    block.append(addressValue);
    const actions = family.address ? [
      copyButton(family.address),
      iconButton('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
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
  if (p.pronunciationUrl || editing) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    if (p.pronunciationUrl) {
      const audio = el('audio', 'pronounce-player');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = p.pronunciationUrl;
      right.append(audio);
    }
    if (editing) {
      right.append(pronounceEditor('person', p.email));
    }
  }
  grid.append(right);
  content.append(grid);

  if (p.facts || editing) {
    const header = el('h2', 'about-header', 'About Me');
    content.append(header);
    const text = el('div', 'about-text', p.facts || '');
    const status = el('div', 'media-status about-status');
    if (editing) {
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
    }
    content.append(text, status);
  }

  if (editing) {
    const self = p.email === document.body.dataset.userEmail;
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
    content.append(header, button, status);
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
  const shortName = (family.name || '').replace(/ Family$/, '');
  let crumbs = [['People', '/people'], [shortName, null], ['Family', null]];
  const from = fromURL();
  if (key === myFamilyKey()) {
    crumbs = [['My Family', '/my-family'], ['Family', null]];
  } else if (from) {
    const rseg = from.pathname.split('/').filter(Boolean).map(decodeURIComponent);
    const back = from.pathname + from.search;
    if (rseg[0] === 'people' && rseg[1] && byEmail[rseg[1]]) {
      const person = byEmail[rseg[1]];
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

  const content = el('div', 'container detail-content');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  const editable = key === myFamilyKey();
  if (family.photoUrl || editable) {
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
      wrap.append(el('div', 'detail-photo detail-photo-empty'));
    }
    left.append(wrap);
    if (editable) {
      const status = el('div', 'media-status');
      wrap.append(uploadIcon('camera', 'Upload family photo', 'image/*', 'family', key, 'photo', status));
      left.append(status);
    }
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
    right.append(contactRow(el('div', 'contact-value', family.address), [
      copyButton(family.address),
      iconButton('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
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
  content.append(grid);
  main.append(content);

  const band = el('div', 'band');
  const inner = el('div', 'container');
  inner.append(el('h2', 'band-title', 'Family Members'));
  const cols = el('div', 'band-grid');
  const adultsCol = el('div');
  if (adults.length) {
    adultsCol.append(el('div', 'member-header', 'Adults'));
    for (const a of adults) {
      adultsCol.append(memberRow(a, a.pronouns ? a.pronouns.toUpperCase() : ''));
    }
  }
  const kidsCol = el('div');
  if (kids.length) {
    kidsCol.append(el('div', 'member-header', 'Kids'));
    for (const k of kids) {
      kidsCol.append(memberRow(k, k.pronouns ? k.pronouns.toUpperCase() : '', k.grade));
    }
  }
  cols.append(adultsCol, kidsCol);
  inner.append(cols);
  band.append(inner);
  main.append(band);
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
    info.append(el('div', 'list-sub', sub));
  }
  row.append(info);
  const chev = el('div', 'list-chevron');
  chev.append(svg('chevron-right'));
  row.append(chev);
  return row;
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
    list.append(el('h2', 'group-header', group.label));
    for (const c of rows) {
      const students = studentsOf(p => p.classroom === c.name).length;
      list.append(listRow(c.imageUrl, `${students} students`, c.name, '', withFrom('/classrooms/' + slugify(c.name))));
      count++;
    }
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
    list.append(el('h2', 'group-header', group.label));
    for (const name of rows) {
      const students = studentsOf(p => p.grade === name).length;
      list.append(listRow(gradeImage(name), `${students} students`, name, '', withFrom('/grades/' + slugify(name))));
      count++;
    }
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
    list.append(el('h2', 'group-header', group.label));
    for (const p of parents) {
      list.append(listRow(thumbUrl(p.photoUrl), '', p.fullName, kidsSummary(p), personLink(p)));
      count++;
    }
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

  main.append(tabStrip(classroomsTabs, state.classTab, 1, key => {
    state.classTab = key;
    state.q = '';
    history.replaceState(null, '', tabHref(key));
    renderClassroomsPage();
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

  const content = el('div', 'content container');
  const header = el('div', 'content-header');
  header.append(el('h1', '', 'Staff'));
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

function renderRoster(title, image, groups, backLabel) {
  const main = resetMain();
  const from = fromURL();
  const back = from && from.pathname === '/classrooms' ? from.pathname + from.search : '/classrooms';
  main.append(breadcrumbs([['Classrooms', back], [title, null]]));

  const content = el('div', 'container detail-content');
  const head = el('div', 'class-head');
  if (image) {
    const img = el('img', 'class-tile');
    img.src = image;
    img.alt = '';
    head.append(img);
  }
  head.append(el('h1', 'class-title', title));
  content.append(head);

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
  strip.querySelector('.tabs-row').classList.remove('container');
  content.append(strip);

  const list = el('div');
  if (state.rosterTab === 'students') {
    const heading = el('h2', 'roster-heading', `${allStudents.length} Students`);
    list.append(heading);
    for (const group of groups) {
      if (group.header) {
        list.append(el('h2', 'group-header', group.header));
      }
      for (const s of group.students) {
        const row = listRow(thumbUrl(s.photoUrl), otherFamilyMembers(s).toUpperCase(), s.fullName, s.facts || '', personLink(s));
        list.append(row);
      }
    }
  } else if (state.rosterTab === 'staff') {
    for (const email of teachers) {
      const person = byEmail[email.toLowerCase()];
      if (person) {
        list.append(listRow(thumbUrl(person.photoUrl), (person.jobTitle || '').toUpperCase(), person.fullName, person.facts || '', personLink(person)));
      } else {
        list.append(el('div', 'list-row plain', email));
      }
    }
  } else {
    for (const p of parents) {
      list.append(listRow(thumbUrl(p.photoUrl), '', p.fullName, kidsSummary(p), personLink(p)));
    }
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
    ? crews.map(name => ({header: name, students: students.filter(s => s.crew === name)}))
    : [{header: classroom.name, students}];
  renderRoster(classroom.name, classroom.imageUrl, groups);
}

const emailTabs = [
  {key: 'parents', label: 'Parents'},
  {key: 'students', label: 'Students'},
  {key: 'both', label: 'Students & Parents'},
  {key: 'tagged', label: 'My Tags'},
];

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

function emailEntries(tab) {
  if (tab === 'tagged') {
    return emailEntries('both').filter(r => isTagged(r.p.email));
  }
  const students = state.model.people
    .filter(p => p.isStudent)
    .sort((a, b) => a.fullName.localeCompare(b.fullName));
  const rows = [];
  const seen = new Set();
  const add = (p, role, grade, classroom) => {
    if (!seen.has(p.email)) {
      seen.add(p.email);
      rows.push({p, role, grade, classroom});
    }
  };
  for (const s of students) {
    if (tab !== 'parents') {
      add(s, 'Student', s.grade || '', s.classroom || '');
    }
    if (tab !== 'students') {
      const family = state.model.families[s.familyKey];
      for (const email of (family && family.adultEmails) || []) {
        const parent = byEmail[email];
        if (parent) {
          add(parent, 'Parent', kidsField(parent, 'grade'), kidsField(parent, 'classroom'));
        }
      }
    }
  }
  return rows;
}

function renderEmailListPage() {
  const main = resetMain();

  main.append(el('div', 'container email-hint', 'Use the filters to select for specific grades or classrooms.'));

  const items = emailTabs.map(t => (t.key === 'tagged' ? {...t, icon: 'tag'} : t));
  main.append(tabStrip(items, state.emailTab, 2, key => {
    state.emailTab = key;
    state.q = '';
    history.replaceState(null, '', tabHref(key));
    renderEmailListPage();
  }));

  const content = el('div', 'content container');
  const header = el('div', 'content-header');
  header.append(el('h1', '', emailTabs.find(t => t.key === state.emailTab).label));
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
  controls.append(search, filterControl(renderTable), download);
  header.append(controls);
  content.append(header);

  const holder = el('div', 'email-holder');
  content.append(holder);
  main.append(content);

  function renderTable() {
    holder.replaceChildren();
    const rows = emailEntries(state.emailTab)
      .filter(r => (r.p.fullName.toLowerCase().includes(state.q) || r.p.email.toLowerCase().includes(state.q)) && matchesFilters(r.p));
    const csv = [emailColumns.map(c => c.label).join(',')]
      .concat(rows.map(r => emailColumns.map(c => csvField(c.get(r))).join(',')))
      .join('\n');
    download.href = 'data:text/csv;charset=utf-8,' + encodeURIComponent(csv);
    download.download = state.emailTab + '.csv';
    if (!rows.length) {
      holder.append(el('div', 'empty', state.emailTab === 'tagged' ? 'No tagged people yet.' : 'No matches.'));
      return;
    }
    const table = el('table', 'email-table');
    const thead = el('thead');
    const headRow = el('tr');
    headRow.append(el('th'));
    for (const c of emailColumns) {
      const th = el('th', '', c.label);
      th.append(copyGlyph(rows.map(c.get).filter(Boolean).join('\n')));
      headRow.append(th);
    }
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
        if (state.emailTab === 'tagged' || state.filterTags.size) {
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
  action.href = withFrom('/people/' + encodeURIComponent(document.body.dataset.userEmail) + '?edit=1');
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
  if (email === meEmail) {
    return true;
  }
  const me = byEmail[meEmail];
  const family = me && state.model.families[me.familyKey];
  return Boolean(family && (family.kidEmails || []).includes(email));
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
    state.emailTab = tabParam('parents');
    state.q = '';
    renderEmailListPage();
  } else if (seg[0] === 'map') {
    state.q = '';
    renderMapPage();
  }
}

const userMenu = document.querySelector('#user-menu');
document.querySelector('#user').addEventListener('click', e => {
  e.stopPropagation();
  userMenu.hidden = !userMenu.hidden;
});
document.querySelector('#user .user-avatar').addEventListener('click', e => {
  e.stopPropagation();
  location.href = withFrom('/people/' + encodeURIComponent(document.body.dataset.userEmail));
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
function closeFilterPanels() {
  for (const panel of document.querySelectorAll('.filter-panel')) {
    panel.hidden = true;
    panel.parentElement.querySelector('.filter-button').classList.remove('open');
  }
}

document.addEventListener('click', e => {
  userMenu.hidden = true;
  drawerUserMenu.hidden = true;
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
    closeFilterPanels();
    for (const menu of document.querySelectorAll('.more-menu, .card-menu')) {
      menu.hidden = true;
    }
  }
});

async function load() {
  const res = await fetch('/api/directory/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  state.model = await res.json();
  tags = state.model.tags || {};
  byEmail = {};
  for (const p of state.model.people) {
    byEmail[p.email] = p;
  }
  render();
}

load();
