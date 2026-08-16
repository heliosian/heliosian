const state = {model: null, tab: 'everyone', classTab: 'by-classroom', rosterTab: 'students', q: ''};
let byEmail = {};

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
  heart: '<svg viewBox="0 0 24 24"><path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z"/></svg>',
  mail: '<svg viewBox="0 0 24 24"><rect width="20" height="16" x="2" y="4" rx="2"/><path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7"/></svg>',
  copy: '<svg viewBox="0 0 24 24"><rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>',
  message: '<svg viewBox="0 0 24 24"><path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/></svg>',
  phone: '<svg viewBox="0 0 24 24"><path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72 12.84 12.84 0 0 0 .7 2.81 2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45 12.84 12.84 0 0 0 2.81.7A2 2 0 0 1 22 16.92z"/></svg>',
};

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

function personLink(p) {
  return '/people/' + encodeURIComponent(p.email);
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  const seg = segments()[0] === 'families' ? 'people' : segments()[0];
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
  return [p.grade, p.classroom, p.section].filter(Boolean).join(' ▶ ');
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

function personCard(p) {
  const card = el('a', 'person-card');
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
    return `${p.fullName} ${family ? family.name : ''}`.toLowerCase().includes(q);
  });
  for (const p of matches) {
    grid.append(personCard(p));
  }
  return matches.length;
}

function renderStudents(grid) {
  grid.className = 'student-grid';
  const matches = state.model.people.filter(p => p.isStudent && p.fullName.toLowerCase().includes(state.q));
  for (const p of matches) {
    const card = el('a', 'student-card');
    card.href = personLink(p);
    if (p.photoUrl) {
      const img = el('img', 'student-photo');
      img.src = thumbUrl(p.photoUrl);
      img.loading = 'lazy';
      img.alt = '';
      card.append(img);
    }
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
      href: '/families/' + encodeURIComponent(f.key),
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
      });
    }
  }
  entries.sort((a, b) => a.name.localeCompare(b.name));
  return entries;
}

function renderFamilies(grid) {
  grid.className = 'family-grid';
  const matches = familyEntries().filter(f =>
    `${f.name} ${f.members.join(' ')}`.toLowerCase().includes(state.q));
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
    p.isStaff && `${p.fullName} ${p.jobTitle || ''}`.toLowerCase().includes(state.q));
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
  const main = document.querySelector('#main');
  main.replaceChildren();

  const tabs = el('div', 'tabs');
  const tabsRow = el('div', 'container tabs-row');
  for (const tab of peopleTabs) {
    const node = el('div', 'tab' + (tab.key === state.tab ? ' active' : ''));
    node.append(svg(tab.key === 'staff' ? 'staff-tab' : tab.key), el('span', '', tab.label));
    node.addEventListener('click', () => {
      state.tab = tab.key;
      state.q = '';
      history.replaceState(null, '', '/people?tab=' + tab.key);
      renderPeople();
    });
    tabsRow.append(node);
  }
  tabs.append(tabsRow);
  main.append(tabs);

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
  const filter = el('button', 'filter-button');
  filter.append(svg('filter'), el('span', '', 'Filter'), svg('chevron'));
  controls.append(search, filter);
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

function referrerCrumbs() {
  if (!document.referrer) {
    return null;
  }
  const ref = new URL(document.referrer);
  if (ref.origin !== location.origin) {
    return null;
  }
  const seg = ref.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  if (seg[0] === 'grades' && seg[1]) {
    const grade = state.model.grades.find(g => slugify(g.name) === seg[1]);
    if (grade) {
      return [['Classrooms', '/classrooms'], [grade.name, ref.pathname + ref.search]];
    }
  }
  if (seg[0] === 'classrooms' && seg[1]) {
    const classroom = state.model.classrooms.find(c => slugify(c.name) === seg[1]);
    if (classroom) {
      return [['Classrooms', '/classrooms'], [classroom.name, ref.pathname + ref.search]];
    }
  }
  if (seg[0] === 'classrooms') {
    return [['Classrooms', '/classrooms']];
  }
  if (seg[0] === 'staff') {
    return [['Staff', '/staff']];
  }
  if (seg[0] === 'profile') {
    return [['Profile', '/profile']];
  }
  return null;
}

function breadcrumbs(parts) {
  const top = el('div', 'detail-top container');
  const crumbs = el('div', 'crumbs');
  const back = el('a', 'crumb-back');
  const parent = [...parts].reverse().find(([, href]) => href);
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
  const heart = el('button', 'heart-button');
  heart.append(svg('heart'));
  top.append(heart);
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
  see.href = '/families/' + encodeURIComponent(family.key);
  see.append(el('span', '', `See ${family.name}`));
  const chev = el('div', 'member-chevron');
  chev.append(svg('chevron-right'));
  see.append(chev);
  right.append(see);
  grid.append(right);
  band.append(grid);
  return band;
}

function renderPersonDetail(email) {
  const main = document.querySelector('#main');
  main.replaceChildren();
  const p = byEmail[email];
  if (!p) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
  }
  const origin = referrerCrumbs() || [['People', '/people']];
  main.append(breadcrumbs([...origin, [p.fullName, null]]));

  const content = el('div', 'container detail-content');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  if (p.photoUrl) {
    const img = el('img', 'detail-photo');
    img.src = p.photoUrl;
    img.alt = '';
    left.append(img);
  }
  grid.append(left);

  const right = el('div');
  right.append(el('div', 'role-label', roleLabel(p)));
  right.append(el('h1', 'detail-name', p.fullName));
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
  if (p.phone) {
    right.append(contactRow(el('div', 'contact-value', p.phone), [
      copyButton(p.phone),
      iconButton('message', 'Text', 'sms:' + p.phone),
      iconButton('phone', 'Call', 'tel:' + p.phone),
    ]));
  }
  right.append(contactRow(el('div', 'contact-value', p.email), [
    copyButton(p.email),
    iconButton('mail', 'Email', 'mailto:' + p.email),
  ]));
  const family = state.model.families[p.familyKey];
  if (family && family.address) {
    const block = el('div');
    block.append(el('div', 'field-label', 'Address'));
    block.append(el('div', 'contact-value', family.address));
    right.append(contactRow(block, [
      copyButton(family.address),
      iconButton('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
    ]));
  }
  if (p.pronunciationUrl) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    const audio = el('audio', 'pronounce-player');
    audio.controls = true;
    audio.preload = 'metadata';
    audio.src = p.pronunciationUrl;
    right.append(audio);
  }
  grid.append(right);
  content.append(grid);

  if (p.facts) {
    content.append(el('h2', 'about-header', 'About Me'));
    content.append(el('div', 'about-text', p.facts));
  }
  main.append(content);

  if (family) {
    main.append(familyBand(p, family));
  }
}

function renderFamilyDetail(key) {
  const main = document.querySelector('#main');
  main.replaceChildren();
  const family = state.model.families[key];
  if (!family) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
  }
  const shortName = (family.name || '').replace(/ Family$/, '');
  let crumbs = [['People', '/people'], [shortName, null], ['Family', null]];
  if (document.referrer && new URL(document.referrer).origin === location.origin) {
    const ref = new URL(document.referrer);
    const rseg = ref.pathname.split('/').filter(Boolean).map(decodeURIComponent);
    if (rseg[0] === 'people' && rseg[1] && byEmail[rseg[1]]) {
      crumbs = [['People', '/people'], [byEmail[rseg[1]].fullName, ref.pathname], ['Family', null]];
    } else if (rseg[0] === 'profile') {
      crumbs = [['Profile', '/profile'], ['Family', null]];
    }
  }
  main.append(breadcrumbs(crumbs));

  const content = el('div', 'container detail-content');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  if (family.photoUrl) {
    const link = el('a');
    link.href = family.photoUrl;
    link.target = '_blank';
    const img = el('img', 'detail-photo');
    img.src = family.photoUrl;
    img.alt = '';
    link.append(img);
    left.append(link);
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
  if (family.pronunciationUrl) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    const audio = el('audio', 'pronounce-player');
    audio.controls = true;
    audio.preload = 'metadata';
    audio.src = family.pronunciationUrl;
    right.append(audio);
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
      list.append(listRow(c.imageUrl, `${students} students`, c.name, '', '/classrooms/' + slugify(c.name)));
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
      list.append(listRow(gradeImage(name), `${students} students`, name, '', '/grades/' + slugify(name)));
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
  const main = document.querySelector('#main');
  main.replaceChildren();

  const tabs = el('div', 'tabs');
  const tabsRow = el('div', 'container tabs-row');
  for (const tab of classroomsTabs) {
    const node = el('div', 'tab' + (tab.key === state.classTab ? ' active' : ''));
    node.append(el('span', '', tab.label));
    node.addEventListener('click', () => {
      state.classTab = tab.key;
      state.q = '';
      history.replaceState(null, '', '/classrooms?tab=' + tab.key);
      renderClassroomsPage();
    });
    tabsRow.append(node);
  }
  tabs.append(tabsRow);
  main.append(tabs);

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
  const main = document.querySelector('#main');
  main.replaceChildren();

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
  const filter = el('button', 'filter-button');
  filter.append(svg('filter'), el('span', '', 'Filter'), svg('chevron'));
  controls.append(search, filter);
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
  for (const section of state.model.sections) {
    if (!classroomNames.includes(section.classroom)) {
      continue;
    }
    for (const name of section.teachers || []) {
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
  const main = document.querySelector('#main');
  main.replaceChildren();
  main.append(breadcrumbs([['Classrooms', '/classrooms'], [title, null]]));

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

  const tabs = el('div', 'tabs roster-tabs');
  const tabsRow = el('div', 'tabs-row');
  for (const tab of memberTabs) {
    const node = el('div', 'tab' + (tab.key === state.rosterTab ? ' active' : ''));
    node.append(svg(tab.icon), el('span', '', tab.label), el('span', 'tab-count', String(tab.count)));
    node.addEventListener('click', () => {
      state.rosterTab = tab.key;
      history.replaceState(null, '', location.pathname + '?tab=' + tab.key);
      renderRoster(title, image, groups, backLabel);
    });
    tabsRow.append(node);
  }
  tabs.append(tabsRow);
  content.append(tabs);

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
    document.querySelector('#main').replaceChildren(el('div', 'empty', 'Not found.'));
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
    document.querySelector('#main').replaceChildren(el('div', 'empty', 'Not found.'));
    return;
  }
  const students = studentsOf(p => p.classroom === classroom.name);
  const sections = [...new Set(students.map(s => s.section).filter(Boolean))].sort();
  const groups = sections.length
    ? sections.map(name => ({header: name, students: students.filter(s => s.section === name)}))
    : [{header: classroom.name, students}];
  renderRoster(classroom.name, classroom.imageUrl, groups);
}

function renderProfile() {
  const main = document.querySelector('#main');
  main.replaceChildren();
  const me = byEmail[document.body.dataset.userEmail];
  if (!me) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
  }
  const content = el('div', 'container content');
  const family = state.model.families[me.familyKey];
  if (family) {
    content.append(el('h1', 'profile-heading', 'Family Photo'));
    content.append(listRow(thumbUrl(family.photoUrl), '', 'Family Photo', family.photoCaption || '', '/families/' + encodeURIComponent(me.familyKey)));
    const kids = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
    if (kids.length) {
      content.append(el('h1', 'profile-heading', 'Students'));
      for (const k of kids) {
        content.append(listRow(thumbUrl(k.photoUrl), (k.pronouns || '').toUpperCase(), firstName(k.fullName), '', personLink(k)));
      }
    }
  }
  main.append(content);
}

function tabParam(fallback) {
  return new URLSearchParams(location.search).get('tab') || fallback;
}

function render() {
  renderNav();
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
  } else if (seg[0] === 'profile') {
    renderProfile();
  }
}

const userMenu = document.querySelector('#user-menu');
document.querySelector('#user').addEventListener('click', e => {
  e.stopPropagation();
  userMenu.hidden = !userMenu.hidden;
});
document.addEventListener('click', () => {
  userMenu.hidden = true;
});
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    userMenu.hidden = true;
  }
});

async function load() {
  const res = await fetch('/api/directory/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  state.model = await res.json();
  byEmail = {};
  for (const p of state.model.people) {
    byEmail[p.email] = p;
  }
  render();
}

load();
