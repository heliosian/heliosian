const state = {model: null, tab: 'everyone', q: ''};
let byEmail = {};

const icons = {
  people: '<svg viewBox="0 0 24 24"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
  classrooms: '<svg viewBox="0 0 24 24"><path d="M4 10a4 4 0 0 1 4-4h8a4 4 0 0 1 4 4v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2Z"/><path d="M9 6V4a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2"/><path d="M8 21v-5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v5"/><path d="M8 10h8"/></svg>',
  'my-family': '<svg viewBox="0 0 24 24"><path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z"/></svg>',
  staff: '<svg viewBox="0 0 24 24"><path d="M12 20.94c1.5 0 2.75 1.06 4 1.06 3 0 6-8 6-12.22A4.91 4.91 0 0 0 17 5c-2.22 0-4 1.44-5 2-1-.56-2.78-2-5-2a4.9 4.9 0 0 0-5 4.78C2 14 5 22 8 22c1.25 0 2.5-1.06 4-1.06Z"/><path d="M10 2c1 .5 2 2 2 5"/></svg>',
  map: '<svg viewBox="0 0 24 24"><path d="M20 10c0 4.993-5.539 10.193-7.399 11.799a1 1 0 0 1-1.202 0C9.539 20.193 4 14.993 4 10a8 8 0 0 1 16 0"/><circle cx="12" cy="10" r="3"/></svg>',
  'email-list': '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8"/></svg>',
  'data-view': '<svg viewBox="0 0 24 24"><rect width="7" height="7" x="3" y="3" rx="1"/><rect width="7" height="7" x="14" y="3" rx="1"/><rect width="7" height="7" x="14" y="14" rx="1"/><rect width="7" height="7" x="3" y="14" rx="1"/></svg>',
  'bug-report': '<svg viewBox="0 0 24 24"><circle cx="12" cy="5" r="1"/><path d="m9 20 3-6 3 6"/><path d="m6 8 6 2 6-2"/><path d="M12 10v4"/></svg>',
  about: '<svg viewBox="0 0 24 24"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" x2="15.42" y1="13.51" y2="17.49"/><line x1="15.41" x2="8.59" y1="6.51" y2="10.49"/></svg>',
  everyone: '<svg viewBox="0 0 24 24"><rect width="7" height="7" x="3" y="3" rx="1"/><rect width="7" height="7" x="14" y="3" rx="1"/><rect width="7" height="7" x="14" y="14" rx="1"/><rect width="7" height="7" x="3" y="14" rx="1"/></svg>',
  students: '<svg viewBox="0 0 24 24"><circle cx="8.5" cy="5.5" r="2"/><path d="M8.5 7.5v5M8.5 12.5l-2.5 5M8.5 12.5l2.5 5M5 9.5l3.5 1 3.5-1"/><circle cx="16.5" cy="7" r="1.7"/><path d="M16.5 8.7v4.3M16.5 13l-2 4M16.5 13l2 4M13.8 10.5l2.7.8 2.7-.8"/></svg>',
  families: '<svg viewBox="0 0 24 24"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
  'staff-tab': '<svg viewBox="0 0 24 24"><line x1="10" x2="14" y1="2" y2="2"/><line x1="12" x2="15" y1="14" y2="11"/><circle cx="12" cy="14" r="8"/></svg>',
  search: '<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>',
  filter: '<svg viewBox="0 0 24 24"><path d="M5 7h14M8 12h8M10.5 17h3"/></svg>',
  chevron: '<svg viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></svg>',
};

const navSections = [
  {path: 'people', label: 'People'},
  {path: 'classrooms', label: 'Classrooms'},
  {path: 'my-family', label: 'My Family'},
  {path: 'staff', label: 'Staff'},
  {path: 'map', label: 'Map'},
  {path: 'email-list', label: 'Email List'},
  {divider: true},
  {path: 'data-view', label: 'Data View'},
  {path: 'bug-report', label: 'Bug Report'},
  {path: 'about', label: 'Share & About'},
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

function section() {
  return location.pathname.replaceAll('/', '');
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  for (const item of navSections) {
    if (item.divider) {
      nav.append(el('div', 'nav-divider'));
      continue;
    }
    const a = el('a');
    a.href = '/' + item.path;
    if (item.path === section()) {
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

function photoOrInitials(url, name, className) {
  if (url) {
    const img = el('img', className);
    img.src = url;
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

function personContext(p) {
  if (p.isStudent) {
    return [p.grade, p.classroom, p.section].filter(Boolean).join(' ▶ ');
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
  const card = el('div', 'person-card');
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
    const card = el('div', 'student-card');
    if (p.photoUrl) {
      const img = el('img', 'student-photo');
      img.src = p.photoUrl;
      img.loading = 'lazy';
      img.alt = '';
      card.append(img);
    }
    card.append(el('div', 'student-first', firstName(p.fullName)));
    card.append(el('div', 'student-last', p.fullName.replace(firstName(p.fullName), '').trim()));
    card.append(el('div', 'student-line', [p.grade, p.classroom, p.section].filter(Boolean).join(' ▶ ')));
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
      name: (f.name || '').replace(/ Family$/, ''),
      label: kidGrades.length ? kidGrades.join(', ') : 'Staff',
      members: members.map(e => byEmail[e] ? firstName(byEmail[e].fullName) : '').filter(Boolean),
      photoUrl: f.photoUrl,
    };
  });
  for (const p of state.model.people) {
    if (p.isStaff && !p.isParent && !p.isStudent && !state.model.families[p.familyKey]) {
      entries.push({
        name: p.fullName.trim().split(/\s+/).slice(-1)[0],
        label: 'Staff',
        members: [firstName(p.fullName)],
        photoUrl: p.photoUrl,
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
    const card = el('div', 'family-card');
    card.append(photoOrInitials(f.photoUrl, f.name, 'family-photo'));
    card.append(el('div', 'family-label', f.label));
    card.append(el('div', 'family-name', f.name));
    card.append(el('div', 'family-kids', f.members.join(', ')));
    grid.append(card);
  }
  return matches.length;
}

function renderStaff(grid) {
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
    const deptGrid = el('div', 'people-grid');
    for (const p of groups.get(dept)) {
      const card = el('div', 'person-card');
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

function render() {
  renderNav();
  if (section() === 'people') {
    renderPeople();
  }
}

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
