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

// The last tag this user assigned to anyone, anywhere - remembered per
// browser so a click on the tag button can immediately reapply it instead of
// making every tagging start from an empty dropdown.
function loadLastTag() {
  try {
    return localStorage.getItem('lastTag') || '';
  } catch (e) {
    return '';
  }
}

function saveLastTag(tag) {
  try {
    localStorage.setItem('lastTag', tag);
  } catch (e) {
    // ignore
  }
}

// When each tag was last applied to someone, remembered per browser so the
// tag picker's checkbox list can surface the tags this user actually reaches
// for instead of an alphabetical list that never changes.
function loadTagUsage() {
  try {
    return JSON.parse(localStorage.getItem('tagUsage') || '{}');
  } catch (e) {
    return {};
  }
}

function recordTagUsage(tag) {
  try {
    const usage = loadTagUsage();
    usage[tag] = Date.now();
    localStorage.setItem('tagUsage', JSON.stringify(usage));
  } catch (e) {
    // ignore
  }
}

// Which relations (Parents/Children/Siblings) to pull into each tag's list,
// remembered per tag name so "Birthday" and "Carpool" can each keep their
// own Include settings across visits.
function loadTagRelations(tag) {
  try {
    const all = JSON.parse(localStorage.getItem('tagRelations') || '{}');
    return new Set(all[tag] || []);
  } catch (e) {
    return new Set();
  }
}

function saveTagRelations(tag, relations) {
  try {
    const all = JSON.parse(localStorage.getItem('tagRelations') || '{}');
    all[tag] = [...relations];
    localStorage.setItem('tagRelations', JSON.stringify(all));
  } catch (e) {
    // ignore
  }
}

const state = {model: null, tab: 'everyone', classTab: 'by-classroom', rosterTab: 'students', rosterSectionExcluded: new Set(), q: '', filterGrades: new Set(), filterClassrooms: new Set(), filterRoles: new Set(), filterRoleExcluded: new Set(), filterCities: new Set(), filterPronouns: new Set(), filterTags: new Set(), filterTagRelations: new Set(), filterNew: false, staffDeptExcluded: new Set(), tagListView: 'faces', navOpen: loadNavOpen(), gvGreeting: 'Family of the kids', gvSiblings: true, gvKidEmail: false, gvInviteBy: 'group', gvSystem: ''};

const tagListViews = [
  {key: 'faces', label: 'Profiles', icon: 'everyone'},
  {key: 'emails', label: 'Email List', icon: 'email-list'},
  {key: 'map', label: 'Map', icon: 'map'},
];

// Single-select, icon-only segmented control for how to view a list - it's
// the same filtered data underneath either way, just displayed differently,
// so it reads as a display-mode switch (like a grid/list view toggle) rather
// than a top-level tab or another filter chip.
function tagListViewSwitch(rerender) {
  const bar = el('div', 'view-switch');
  for (const v of tagListViews) {
    const btn = el('button', 'view-switch-btn' + (state.tagListView === v.key ? ' active' : ''));
    btn.type = 'button';
    btn.title = v.label;
    btn.append(svg(v.icon));
    btn.addEventListener('click', () => {
      if (state.tagListView === v.key) {
        return;
      }
      state.tagListView = v.key;
      for (const sibling of bar.querySelectorAll('.view-switch-btn')) {
        sibling.classList.remove('active');
      }
      btn.classList.add('active');
      rerender();
    });
    bar.append(btn);
  }
  return bar;
}
let byEmail = {};
let tags = {};

function tagNames() {
  return Object.keys(tags).sort((a, b) => a.localeCompare(b));
}

// Same set of tags as tagNames(), ordered most-recently-used first (falling
// back to alphabetical for tags this browser has no usage record for, e.g.
// after clearing localStorage or on another device) - this is the order the
// tag picker's checkbox list shows, so the tags someone actually uses rise to
// the top instead of sitting wherever the alphabet puts them.
function tagNamesByRecency() {
  const usage = loadTagUsage();
  return tagNames().sort((a, b) => (usage[b] || 0) - (usage[a] || 0));
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
    saveLastTag(tag);
    recordTagUsage(tag);
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
  greenvelope: '<svg viewBox="0 0 24 24"><path d="M5.8 11.3 2 22l10.7-3.79"/><path d="M4 3h.01"/><path d="M22 8h.01"/><path d="M15 2h.01"/><path d="M22 20h.01"/><path d="m22 2-2.24.75a2.9 2.9 0 0 0-1.96 3.12c.1.86-.57 1.63-1.45 1.63h-.38c-.86 0-1.6.6-1.76 1.44L14 10"/><path d="m22 13-.82-.33c-.86-.34-1.82.2-1.98 1.11c-.11.7-.72 1.22-1.43 1.22H17"/><path d="m11 2 .33.82c.34.86-.2 1.82-1.11 1.98C9.52 4.9 9 5.52 9 6.23V7"/><path d="M11 13c1.93 1.93 2.83 4.17 2 5-.83.83-3.07-.07-5-2-1.93-1.93-2.83-4.17-2-5 .83-.83 3.07.07 5 2Z"/></svg>',
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
  pencil: '<svg viewBox="0 0 24 24"><path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z"/><path d="m15 5 4 4"/></svg>',
  alert: '<svg viewBox="0 0 24 24"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/><line x1="12" x2="12" y1="9" y2="13"/><line x1="12" x2="12.01" y1="17" y2="17"/></svg>',
  sync: '<svg viewBox="0 0 24 24"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/></svg>',
  lock: '<svg viewBox="0 0 24 24"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>',
  gear: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>',
  list: '<svg viewBox="0 0 24 24"><path d="M3 12h.01"/><path d="M3 18h.01"/><path d="M3 6h.01"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M8 6h13"/></svg>',
  volume: '<svg viewBox="0 0 24 24"><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/><path d="M15.54 8.46a5 5 0 0 1 0 7.07"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/></svg>',
  eye: '<svg viewBox="0 0 24 24"><path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/></svg>',
  star: '<svg viewBox="0 0 24 24"><path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z"/></svg>',
  heart: '<svg viewBox="0 0 24 24"><path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z"/></svg>',
  crop: '<svg viewBox="0 0 24 24"><path d="M6 2v14a2 2 0 0 0 2 2h14"/><path d="M18 22V8a2 2 0 0 0-2-2H2"/></svg>',
  trash: '<svg viewBox="0 0 24 24"><path d="M10 11v6"/><path d="M14 11v6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M3 6h18"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
  'zoom-in': '<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><line x1="21" x2="16.65" y1="21" y2="16.65"/><line x1="11" x2="11" y1="8" y2="14"/><line x1="8" x2="14" y1="11" y2="11"/></svg>',
  info: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>',
  x: '<svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>',
  sparkles: '<svg viewBox="0 0 24 24"><path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .963 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.581a.5.5 0 0 1 0 .964L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.963 0z"/><path d="M19 3l.8 2.2L22 6l-2.2.8L19 9l-.8-2.2L16 6l2.2-.8z"/></svg>',
  plus: '<svg viewBox="0 0 24 24"><path d="M5 12h14"/><path d="M12 5v14"/></svg>',
  ellipsis: '<svg viewBox="0 0 24 24"><circle cx="5" cy="12" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="19" cy="12" r="2"/></svg>',
};

function isMobile() {
  return matchMedia('(max-width: 900px)').matches;
}

const primaryNavItems = [
  {path: 'people', label: 'Directory'},
  {path: 'classrooms', label: 'Gradebands'},
  {path: 'staff', label: 'Staff'},
];

const toolsNavItems = [
  {path: 'email-list', label: 'Everyone'},
  {path: 'greenvelope', label: 'Invites'},
];

const mobileNavSections = [
  {path: 'people', label: 'Directory'},
  {path: 'classrooms', label: 'Gradebands'},
  {path: 'staff', label: 'Staff'},
  {path: 'my-family', label: 'My Family'},
  // Not a real page - opens the Lists popup instead of navigating (see the
  // isListsTab handling in renderNav below). Keeps the "email-list" path so
  // it still lands on Everyone as a fallback (JS disabled, middle-click,
  // "open in new tab") and so the tab lights up whenever that page is open.
  {path: 'email-list', label: 'Lists', isListsTab: true},
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
  const family = familyOf(p);
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
    row.append(photoOrInitials(gradeImage(g.name), g.name, 'gsearch-avatar'));
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', g.name));
    row.append(info);
    return row;
  });

  group('Gradebands', classrooms, c => {
    const row = el('a', 'gsearch-result');
    row.href = withFrom('/classrooms/' + slugify(c.name));
    row.append(photoOrInitials(c.imageUrl, c.name, 'gsearch-avatar'));
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', c.name));
    row.append(info);
    return row;
  });

  // Highlight the top result so Enter in the search box goes straight to it,
  // without requiring an arrow-key press first.
  const first = resultsEl.querySelector('.gsearch-result');
  if (first) {
    first.classList.add('active');
  }

  resultsEl.hidden = false;
}

// familiesByEmail inverts the model's family member lists once per load: every
// family a person belongs to, in sorted key order - one for a parent (an adult
// belongs to at most one household), one or more for a kid in two households.
let familiesByEmail = {};

function indexFamilies() {
  familiesByEmail = {};
  for (const key of Object.keys(state.model.families || {}).sort()) {
    const f = state.model.families[key];
    for (const email of [...(f.adultEmails || []), ...(f.kidEmails || [])]) {
      (familiesByEmail[email] = familiesByEmail[email] || []).push(f);
    }
  }
}

function familiesOf(p) {
  return (p && familiesByEmail[p.email]) || [];
}

function familyOf(p) {
  return familiesOf(p)[0];
}

function myFamilyKey() {
  const family = familyOf(byEmail[document.body.dataset.userEmail]);
  return (family && family.key) || '';
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
  updateMobileTitleInset();
}

// .mobile-title is centered by giving it equal left/right insets, so it has to be
// centered on the whole bar rather than just the space between whichever icons
// happen to be showing - a fixed inset sized for the busiest icon cluster (search +
// stale alert + privacy alert + avatar) left the title visibly off-center on every
// page showing fewer icons than that, including the plain back-button pages. Measured
// live because which side is wider varies with the route (back vs. menu button) and
// with per-user alert state (stale info, privacy mismatch).
function updateMobileTitleInset() {
  const bar = document.querySelector('.mobile-top');
  const leftEl = bar.querySelector('#mobile-back:not([hidden]), #mobile-menu-btn:not([hidden])');
  const searchBtn = document.querySelector('#mobile-search-btn');
  if (!bar || !leftEl || !searchBtn) {
    return;
  }
  const barRect = bar.getBoundingClientRect();
  if (!barRect.width) {
    return;
  }
  const leftWidth = leftEl.getBoundingClientRect().right - barRect.left;
  const rightWidth = barRect.right - searchBtn.getBoundingClientRect().left;
  document.documentElement.style.setProperty('--mobile-title-inset', Math.max(leftWidth, rightWidth) + 'px');
}
window.addEventListener('resize', updateMobileTitleInset);

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

// Unlike a person's own photo, a missing family photo needs updating just as much as
// a stale one.
function familyPhotoNeedsUpdate(family) {
  return !family.photoUrl || agedPast(family.photoUrl, family.photoUpdated, staleYears.familyPhoto);
}

function staleItems() {
  const me = byEmail[document.body.dataset.userEmail];
  if (!me) {
    return [];
  }
  const family = familyOf(me);
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
  const family = familyOf(me);
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
      const familyBody = sectionHeading('family', 'My Family', 'heart', todos.length, onFamilyMember);
      if (familyBody) {
        renderItem(familyBody, {path: 'my-family', label: 'My Family'}, familyTodos || (todos.length > familyTodos && 'alert'));
        for (const p of familyPeople) {
          familyBody.append(familyMemberRow(p, me.email, onFamilyMember ? rawSegPerson.email : null));
        }
      }
    }

    const toolsBody = sectionHeading('tools', 'Lists', 'list', 0, false);
    if (toolsBody) {
      for (const item of toolsNavItems) {
        renderItem(toolsBody, item);
      }
      const currentTag = new URLSearchParams(location.search).get('tag');
      for (const name of tagNames()) {
        const a = el('a');
        a.href = '/people?tag=' + encodeURIComponent(name);
        if (seg === 'people' && currentTag === name) {
          a.className = 'active';
        }
        const icon = svg('tag');
        icon.classList.add('nav-icon-tag');
        // Tags are user-created with no fixed set (unlike Directory/Staff/etc.
        // above), so instead of a shared color they each get one hashed from
        // their own name - stable across renders and reasonably legible on
        // the dark sidebar background.
        icon.style.color = `hsl(${hue(name)}, 70%, 72%)`;
        a.append(icon, el('span', '', name));
        toolsBody.append(a);
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
    a.append(svg(item.isListsTab ? 'list' : item.path), el('span', '', item.label));
    if (item.isListsTab) {
      a.addEventListener('click', e => {
        e.preventDefault();
        e.stopPropagation();
        setMobileListsMenu(mobileListsMenu.hidden);
      });
    }
    tabs.append(a);
  }
  if (!mobileListsMenu.hidden) {
    renderMobileListsMenu();
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
    const grade = state.model.grades.find(g => g.name === p.grade);
    return (grade && grade.color) || state.model.staffColor || null;
  }
  if (p.isStaff) {
    return state.model.staffColor || null;
  }
  const family = familyOf(p);
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

// roleWithPronouns is roleLabel's sentence-case counterpart, for spots that
// don't already uppercase via CSS (the profile page's own header) or don't
// want to (a family member's row, where "Parent" reads as plain text next to
// the name rather than a shouty label).
function roleWithPronouns(p) {
  const role = baseRole(p);
  return p.pronouns ? `${role} (${formatPronouns(p.pronouns)})` : role;
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
  const family = familyOf(p);
  if (family) {
    return (family.kidEmails || []).map(e => byEmail[e]?.fullName).filter(Boolean).join(', ');
  }
  return '';
}

function tagMenu(email, onChange) {
  const menu = el('div', 'card-menu tag-menu');
  menu.hidden = true;
  // render() rebuilds every checkbox from scratch on each toggle (simplest way to
  // stay in sync with tagNamesByRecency() gaining/losing/reordering entries), which would otherwise
  // drop keyboard focus back to nothing on every Space press - focusTag puts it
  // back on the same tag's (new) checkbox so arrow keys/Space can keep going.
  menu.focusTag = name => {
    menu.querySelector(`.tag-option input[data-tag-name="${CSS.escape(name)}"]`)?.focus();
  };
  const render = () => {
    menu.replaceChildren();
    for (const name of tagNamesByRecency()) {
      const row = el('label', 'tag-option');
      const box = el('input');
      box.type = 'checkbox';
      box.dataset.tagName = name;
      box.checked = tags[name].includes(email);
      box.addEventListener('change', async () => {
        await setTag(email, name, box.checked);
        render();
        onChange();
        menu.focusTag(name);
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
  // The menu lives inside the card's own <a>, so a plain click here would
  // otherwise bubble up to (or, for a non-self-activating target like the
  // "New tag" input, resolve straight to) the card's link and navigate to
  // the profile page. Checkboxes and their labels already shield themselves
  // from that - they're self-activating - and must keep working natively
  // (preventDefault on them would cancel their own toggle too), so this only
  // steps in for everything else in the menu.
  menu.addEventListener('click', e => {
    e.stopPropagation();
    if (!e.target.closest('.tag-option')) {
      e.preventDefault();
    }
  });
  // Checkboxes only take Tab natively - Up/Down lets a keyboard user walk the
  // list the same way the global search dropdown's results do, so reaching a
  // tag to toggle off never requires the mouse.
  menu.addEventListener('keydown', e => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') {
      return;
    }
    const boxes = [...menu.querySelectorAll('.tag-option input[type="checkbox"]')];
    const current = boxes.indexOf(document.activeElement);
    if (current === -1) {
      return;
    }
    e.preventDefault();
    const delta = e.key === 'ArrowDown' ? 1 : -1;
    boxes[(current + delta + boxes.length) % boxes.length].focus();
  });
  menu.refreshTags = render;
  return menu;
}

// The tag to quick-assign on a click: the last one used, but only if it's
// still one of the user's actual current tags - a remembered name whose last
// person got untagged is gone from tagNames() even though localStorage still
// has it, and reapplying it would resurrect a tag the user no longer has.
// With no current tags at all, "My List" is the starting point.
function mostRecentTag() {
  const existing = tagNames();
  if (!existing.length) {
    return 'My List';
  }
  const last = loadLastTag();
  return existing.includes(last) ? last : existing[0];
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
  button.addEventListener('click', async e => {
    e.preventDefault();
    e.stopPropagation();
    const opening = menu.hidden;
    menu.hidden = !menu.hidden;
    if (!opening) {
      return;
    }
    if (isTagged(email)) {
      // Already has at least one tag - just show them to review or adjust,
      // rather than guessing another one on top.
      menu.querySelector('.tag-option input')?.focus();
      return;
    }
    // Opening the dropdown for someone with no tags yet applies the user's
    // most recently used tag first (creating "My List" the very first time),
    // so tagging someone new is a single click in the common case; the
    // dropdown that comes up right after still shows every tag as a checkbox
    // to adjust or undo the guess.
    const tag = mostRecentTag();
    await setTag(email, tag, true);
    menu.refreshTags();
    button.classList.toggle('active', isTagged(email));
    onChange();
    // Land keyboard focus straight on the tag that was just applied, so a
    // keyboard user's very next keystroke - Space - undoes it without first
    // hunting for it via Tab or the arrow keys.
    menu.focusTag(tag);
  });
  // On a mouse-driven desktop, hovering the button previews the dropdown
  // without the click handler's "apply my most recent tag" side effect -
  // hover is just a look, a click is a commitment. The brief delay before
  // hiding survives the small visual gap between the button and the menu
  // below it, so crossing that gap doesn't flicker the menu shut. Skipped
  // entirely on touch, where there's no hover state to preview with.
  if (window.matchMedia('(hover: hover) and (pointer: fine)').matches) {
    let closeTimer = null;
    wrap.addEventListener('mouseenter', () => {
      clearTimeout(closeTimer);
      menu.hidden = false;
    });
    wrap.addEventListener('mouseleave', () => {
      closeTimer = setTimeout(() => {
        menu.hidden = true;
      }, 150);
    });
  }
  wrap.append(button, menu);
  return wrap;
}

function cardMore(email) {
  return tagControl(email, 'card-more-wrap', 'card-more', () => {});
}

// Pins the personal-tag button to the photo's own corner rather than the
// surrounding card - the card is often much wider than the photo once it's
// centered in a flexible grid column, which left the button floating in
// blank space instead of sitting on the photo.
function photoWithTag(photoEl, email) {
  const wrap = el('div', 'photo-wrap');
  wrap.append(photoEl, cardMore(email));
  return wrap;
}

function personCard(p) {
  const card = el('a', 'person-card');
  card.href = personLink(p);
  card.append(photoWithTag(applyRingColor(photoOrInitials(p.photoUrl, p.fullName, 'person-photo'), p), p.email));
  card.append(el('div', 'role-label', roleLabel(p)));
  card.append(el('div', 'person-name', p.fullName));
  const context = personContext(p);
  if (context) {
    card.append(el('div', 'person-sub', context));
  }
  return card;
}

function everyoneMatches() {
  const q = state.q;
  return state.everyoneOrder.filter(p => {
    const familyNames = familiesOf(p).map(f => f.name).join(' ');
    return `${p.fullName} ${familyNames}`.toLowerCase().includes(q) && matchesFilters(p);
  });
}

function renderEveryone(grid) {
  grid.className = 'people-grid directory-grid';
  const matches = everyoneMatches();
  for (const p of matches) {
    grid.append(personCard(p));
  }
  return matches.length;
}

// The Map view of a tag list - families that have someone matching the same
// filters (tag, relations, search, ...) as Faces/Emails, sharing the map
// plumbing with the standalone Map page.
function renderTagMap(container) {
  const canvas = el('div', 'map-canvas');
  container.append(canvas);
  const q = state.q;
  initFamilyMap(canvas, family => familyMatchesFilters(family.key) && familySearchText(family).includes(q));
}

function renderStudents(grid) {
  grid.className = 'student-grid';
  const matches = state.model.people.filter(p => p.isStudent && p.fullName.toLowerCase().includes(state.q) && matchesFilters(p));
  for (const p of matches) {
    const card = el('a', 'student-card');
    card.href = personLink(p);
    const head = el('div', 'student-head');
    const family = familyOf(p);
    if (family && family.photoUrl) {
      const bg = el('img', 'student-family-photo');
      bg.src = thumbUrl(family.photoUrl);
      bg.loading = 'lazy';
      bg.alt = '';
      head.append(bg);
    }
    head.append(photoOrInitials(p.photoUrl, p.fullName, 'student-photo'));
    head.append(cardMore(p.email));
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
    if (state.staffDeptExcluded.has(dept)) {
      continue;
    }
    grid.append(el('h2', 'staff-section', dept));
    const deptGrid = el('div', 'people-grid directory-grid' + (autoFit ? ' autofit' : ''));
    for (const p of groups.get(dept)) {
      const card = el('a', 'person-card');
      card.href = personLink(p);
      card.append(photoWithTag(applyRingColor(photoOrInitials(p.photoUrl, p.fullName, 'person-photo'), p), p.email));
      card.append(el('div', 'role-label', p.jobTitle || 'Staff'));
      card.append(el('div', 'person-name', p.fullName));
      deptGrid.append(card);
      count++;
    }
    grid.append(deptGrid);
  }
  return count;
}

// Department toggle chips for the Staff page, mirroring roleChips: an
// exclusion set (state.staffDeptExcluded) so every department starts active,
// with stale entries pruned whenever the set of departments actually present
// among staff changes.
function departmentChips(rerender) {
  const departments = state.model.departments || [];
  const present = new Set(state.model.people.filter(p => p.isStaff).map(p => p.department || 'Staff'));
  const ordered = [...present].sort((a, b) => {
    const ia = departments.indexOf(a);
    const ib = departments.indexOf(b);
    return (ia < 0 ? departments.length : ia) - (ib < 0 ? departments.length : ib);
  });
  for (const excluded of [...state.staffDeptExcluded]) {
    if (!ordered.includes(excluded)) {
      state.staffDeptExcluded.delete(excluded);
    }
  }
  const bar = el('div', 'chip-row');
  for (const dept of ordered) {
    const btn = el('button', 'chip-toggle' + (!state.staffDeptExcluded.has(dept) ? ' active' : ''));
    btn.type = 'button';
    btn.append(el('span', '', dept));
    btn.addEventListener('click', () => {
      if (state.staffDeptExcluded.has(dept)) {
        state.staffDeptExcluded.delete(dept);
      } else {
        state.staffDeptExcluded.add(dept);
      }
      btn.classList.toggle('active');
      rerender();
    });
    bar.append(btn);
  }
  return bar;
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
  const header = el('div', 'content-header content-header-solo');
  const controls = el('div', 'controls');
  if (isEveryone) {
    controls.append(roleChips(() => renderGrid()));
  }
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
  const facetFilters = el('div', 'facet-filters');
  facetFilters.append(
    facetDropdown('Grade', gradeOptions(), state.filterGrades, () => renderGrid()),
    facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, () => renderGrid()),
  );
  if (isEveryone && tagNames().length) {
    facetFilters.append(facetDropdown('Tags', tagNames(), state.filterTags, () => renderGrid()));
  }
  controls.append(facetFilters, search);
  // Small-screen stand-in for the Grade/Classroom/Tags dropdowns above: same
  // filters, collapsed into one funnel-icon button so the mobile controls row
  // doesn't have to fit every facet dropdown individually. CSS swaps which of
  // the two is visible per breakpoint (see .facet-filters/.mobile-filter).
  const mobileFilter = filterControl(() => renderGrid(), {role: false, city: false, pronouns: false, newToHelios: false, tags: isEveryone});
  mobileFilter.classList.add('mobile-filter');
  controls.append(mobileFilter);
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

// The unified "list" page: a tag's people (/people?tag=X) or literally
// everyone (/email-list, kept as the URL for continuity) viewed as Faces,
// Emails, or Map - the same filtered data underneath either way, just a
// different display of it. Emails is the fullest-featured view (CSV export,
// per-column copy, a column picker) since that's what this page grew out of.
function renderListPage() {
  const main = resetMain();

  const title = state.filterTags.size ? [...state.filterTags].join(', ') : 'Everyone';

  const pageHeader = el('div', 'page-header container page-header-list');
  const titleWrap = el('div');
  titleWrap.append(el('h1', 'page-title', title));
  pageHeader.append(titleWrap);
  pageHeader.append(tagListViewSwitch(() => renderGrid()));
  main.append(pageHeader);

  const content = el('div', 'content container');
  const header = el('div', 'content-header content-header-solo');
  const controls = el('div', 'controls');
  controls.append(roleChips(() => renderGrid()));
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
  const facetFilters = el('div', 'facet-filters');
  facetFilters.append(
    facetDropdown('Grade', gradeOptions(), state.filterGrades, () => renderGrid()),
    facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, () => renderGrid()),
  );
  if (tagNames().length) {
    facetFilters.append(facetDropdown('Tags', tagNames(), state.filterTags, () => renderGrid()));
  }
  controls.append(facetFilters);
  // Small-screen stand-in for the Grade/Classroom/Tags dropdowns above, same as
  // the Directory page's mobile-filter (see renderPeople) - collapses them into
  // one funnel-icon button so the controls row doesn't wrap across several lines.
  const mobileFilter = filterControl(() => renderGrid(), {role: false, city: false, pronouns: false, newToHelios: false});
  mobileFilter.classList.add('mobile-filter');
  controls.append(mobileFilter);
  // Only meaningful for a single tag - with several selected at once (or
  // none, as on the plain Everyone list) there's no one list to pull
  // relatives in from.
  if (state.filterTags.size === 1) {
    const [activeTag] = state.filterTags;
    controls.append(facetDropdown('Include', tagRelationOptions, state.filterTagRelations, () => {
      saveTagRelations(activeTag, state.filterTagRelations);
      renderGrid();
    }));
  }
  const download = el('a', 'filter-button email-download');
  download.title = 'Download what the list currently shows';
  download.append(svg('download'), el('span', '', 'CSV'));
  controls.append(search, download);
  header.append(controls);
  content.append(header);

  const grid = el('div');
  content.append(grid);
  main.append(content);

  // Emails-view state that should survive a search/filter change, or a trip
  // through Faces/Map and back, rather than resetting on every render.
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

  function renderEmailsTable(container, rows) {
    container.className = 'email-holder';
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
          renderGrid();
        }
      }));
      tr.append(tagCell);
      tbody.append(tr);
    });
    table.append(tbody);
    container.append(table);
  }

  function renderGrid() {
    grid.replaceChildren();
    grid.className = '';
    const rows = emailEntries()
      .filter(r => (r.p.fullName.toLowerCase().includes(state.q) || r.p.email.toLowerCase().includes(state.q)) && matchesFilters(r.p));
    currentRows = rows;
    const csv = [emailColumns.map(c => c.label).join(',')]
      .concat(rows.map(r => emailColumns.map(c => csvField(c.get(r))).join(',')))
      .join('\n');
    download.href = 'data:text/csv;charset=utf-8,' + encodeURIComponent(csv);
    download.download = 'email-list.csv';

    if (state.tagListView === 'map') {
      renderTagMap(grid);
      return;
    }
    if (!rows.length) {
      grid.append(el('div', 'empty', 'No matches.'));
      return;
    }
    if (state.tagListView === 'emails') {
      renderEmailsTable(grid, rows);
      return;
    }
    grid.className = 'people-grid directory-grid';
    for (const r of rows) {
      grid.append(personCard(r.p));
    }
  }
  renderGrid();
  input.focus();
}

// Combines a list of people into one greeting phrase. Two or more people who all
// share a last name are introduced by first name only, with the surname stated once
// at the end ("Alice & Bob McDowell", "Alice, Bob & Carol McDowell"); anyone who
// doesn't share that surname (blended families, a spouse who kept their own name) is
// spelled out in full instead, so nobody's identity is silently merged into someone
// else's. A single person is just their full name.
function joinFamilyNames(people) {
  if (!people.length) {
    return '';
  }
  if (people.length === 1) {
    return people[0].fullName;
  }
  const surname = lastName(people[0].fullName);
  const shared = surname && people.every(p => lastName(p.fullName) === surname);
  const names = people.map(p => shared ? firstName(p.fullName) : p.fullName);
  const line = names.length === 2 ? names.join(' & ') : `${names.slice(0, -1).join(', ')} & ${names[names.length - 1]}`;
  return shared ? `${line} ${surname}` : line;
}

// Like joinFamilyNames, but first names only, with no trailing surname even
// when everyone shares one - what "First name" (as opposed to "Full name")
// means once Include siblings turns a single kid's individual greeting into
// a group of two or more (see buildGreeting).
function joinFirstNames(people) {
  if (!people.length) {
    return '';
  }
  const names = people.map(p => firstName(p.fullName));
  return names.length === 1 ? names[0] : `${names.slice(0, -1).join(', ')} & ${names[names.length - 1]}`;
}

// The _Greetings sheet's Format column is a worked example, not a {{ token }}
// template: it's written against one made-up family - parents Pat and Quinn,
// kids Ali and Bo, everyone surnamed Ender - so writing or editing a greeting
// means describing what it would look like for that one family rather than
// learning template syntax. These are the exact phrases recognized in a
// Format string and what each is replaced with - read from the sheet's own
// "Whole family" / "Kids only" / "Adults only" / "Full name" / "First name"
// rows (via refreshGreetingPhrases, called once loadInviteSystems has the
// greetings) rather than hardcoded here a second time, so the reference
// family lives in exactly one place and editing those rows is all it takes
// to change it. The literals are only a fallback for a sheet missing one of
// those rows - matched longest-first so a bare "Pat" inside "Pat & Quinn
// Ender" doesn't get consumed by the shorter phrase before the longer one
// gets a chance to match.
let GREETING_WHOLE_FAMILY = 'Ali, Bo, Pat & Quinn Ender';
let GREETING_KIDS = 'Ali & Bo Ender';
let GREETING_ADULTS = 'Pat & Quinn Ender';
let GREETING_FULL_NAME = 'Pat Ender';
let GREETING_FIRST_NAME = 'Pat';

function refreshGreetingPhrases() {
  const phraseOf = (name, fallback) => {
    const g = inviteGreetings.find(g => g.name === name);
    return g && g.format ? g.format : fallback;
  };
  GREETING_WHOLE_FAMILY = phraseOf('Whole family', GREETING_WHOLE_FAMILY);
  GREETING_KIDS = phraseOf('Kids only', GREETING_KIDS);
  GREETING_ADULTS = phraseOf('Adults only', GREETING_ADULTS);
  GREETING_FULL_NAME = phraseOf('Full name', GREETING_FULL_NAME);
  GREETING_FIRST_NAME = phraseOf('First name', GREETING_FIRST_NAME);
}

// The standalone Ender token (see substituteNameTokens) always stands for
// the family's own last name, not just one shared by everyone in the
// household - unlike joinFamilyNames' surname (which only appends one when
// the whole group agrees, falling back to full names otherwise), a bare
// "Ender" has nowhere else to fall back to, so it takes whichever surname
// the greeting would naturally be filed under: the primary adult's for a
// family (kids listed first in the group only for name-joining purposes,
// not surname), or the addressed person's own for an individual.
function greetingSurname({kids, adults, person}) {
  if (person) {
    return lastName(person.fullName);
  }
  const primary = adults[0] || kids[0];
  return primary ? lastName(primary.fullName) : '';
}

// splitLastFirst divides a list of people's first names into "everyone but
// the last, comma-joined" and "the last one" - what Ali/Pat and Bo/Quinn
// respectively stand for once they're used loosely rather than as part of
// one of the fixed phrases above. Lines up with joinFamilyNames' own
// comma-then-last shape, so a Format that keeps that same punctuation around
// the tokens reads identically to using the whole-list phrase would.
function splitLastFirst(people) {
  if (!people.length) {
    return {rest: '', last: ''};
  }
  const names = people.map(p => firstName(p.fullName));
  return {rest: names.slice(0, -1).join(', '), last: names[names.length - 1]};
}

// Beyond the fixed phrases buildGreeting checks first (which cover the
// common, cleanly-grouped cases), each individual example name is also
// recognized on its own, wherever it lands in the text - so a Format that
// wraps or rearranges them in its own words (say, "The Ender Family (Ali,
// Bo, Pat & Quinn)") still personalizes per family, instead of only working
// when it reproduces one of the exact phrases verbatim. Runs after those
// fixed-phrase checks, on whatever text they left untouched, so a template
// that does use one of the exact phrases keeps that phrase's own clean
// grouping (correct for 1, 2, or more people) rather than this word-by-word
// fallback's cruder splitting.
// Ali and Pat each stand for "everyone but the last one" in their group,
// which is empty for the common case of a single kid or single parent - a
// plain substitution would then leave whatever separator conventionally
// follows the token (", " in a list, " & " right before the last name)
// stranded at the start (e.g. "(Ali, Bo" -> "(, Bo"). When there's nothing
// to say, this swallows that separator along with the token instead, so it
// reads "(Bo" the way someone writing it by hand would have.
function replaceRestToken(text, token, value) {
  if (value) {
    return text.replace(new RegExp(`\\b${token}\\b`, 'g'), value);
  }
  return text
    .replace(new RegExp(`\\b${token}\\b,\\s*`, 'g'), '')
    .replace(new RegExp(`\\b${token}\\b\\s*&\\s*`, 'g'), '')
    .replace(new RegExp(`\\b${token}\\b`, 'g'), '');
}

function substituteNameTokens(text, {kids, adults, person}) {
  if (!person) {
    const kidsSplit = splitLastFirst(kids);
    const adultsSplit = splitLastFirst(adults);
    text = replaceRestToken(text, 'Ali', kidsSplit.rest);
    text = text.replace(/\bBo\b/g, kidsSplit.last);
    text = replaceRestToken(text, 'Pat', adultsSplit.rest);
    text = text.replace(/\bQuinn\b/g, adultsSplit.last);
  }
  // Also matches "Enders" (the plural a template might use for "the Enders")
  // - only the root swaps for the real surname, so the trailing s (if any)
  // stays put rather than needing its own token.
  const surname = greetingSurname({kids, adults, person});
  return text.replace(/\bEnder(s?)\b/g, `${surname}$1`);
}

// Turns one greeting's Format into real text for a family (kids/adults) or a
// single person (optionally with their siblings folded in too - see
// personInviteParams and Include siblings), by substituting whichever example
// phrases or names actually appear in it - a Format that doesn't use any of
// them at all (a fixed greeting with no names) just passes through
// unchanged. The joined-kids phrase falls back to the adults when there are
// no kids, so a childless couple's "Family of the kids"-style greeting
// doesn't come out blank.
function buildGreeting(format, {kids = [], adults = [], person, siblings = []} = {}) {
  let text = format;
  if (GREETING_WHOLE_FAMILY && text.includes(GREETING_WHOLE_FAMILY)) {
    text = text.replaceAll(GREETING_WHOLE_FAMILY, joinFamilyNames([...kids, ...adults]));
  }
  if (GREETING_KIDS && text.includes(GREETING_KIDS)) {
    text = text.replaceAll(GREETING_KIDS, joinFamilyNames(kids.length ? kids : adults));
  }
  if (GREETING_ADULTS && text.includes(GREETING_ADULTS)) {
    text = text.replaceAll(GREETING_ADULTS, joinFamilyNames(adults));
  }
  if (person) {
    // With no siblings (the common case - an adult's row, or Include siblings
    // off), this is just person's own name either way, same as always.
    const group = [person, ...siblings];
    if (GREETING_FULL_NAME && text.includes(GREETING_FULL_NAME)) {
      text = text.replaceAll(GREETING_FULL_NAME, joinFamilyNames(group));
    }
    if (GREETING_FIRST_NAME && text.includes(GREETING_FIRST_NAME)) {
      text = text.replaceAll(GREETING_FIRST_NAME, joinFirstNames(group));
    }
  }
  return substituteNameTokens(text, {kids, adults, person, siblings});
}

// Individual mode always uses greetings checked Individual - there's no
// family left to build a joint name from once every person gets their own
// row. Group mode uses ones checked Grouped, unless the system itself has no
// way to address a group at all (Punchbowl, Partiful - see
// system.supportsGroups), in which case even a family's one row can only be
// addressed with its primary contact's own name. Grouped and Individual are
// independent (see GreetingTemplate in invites.go) - a greeting can be, and a
// fixed line with no names in it typically would be, both at once.
function greetingFormatsFor(inviteBy, supportsGroups) {
  const wantGrouped = inviteBy === 'group' && supportsGroups;
  return inviteGreetings.filter(g => wantGrouped ? g.grouped : g.individual);
}

// Shared by both invite-by modes: builds the token params a template can pull
// from - primary_email/first/last/phone from whoever is standing in as this
// row's addressee (the family's primary contact in Group mode, or the person
// themself in Individual mode), plus member_1..N, every other person
// flattened into numbered slots ("Name <email>" or just "Name" - what
// Greenvelope/Evite/Punchbowl/Partiful's one-row-per-addressee templates use).
// `people` (addressee first, then members) is the alternate shape a template
// that repeats one row per person (Paperless Post) uses instead - see
// applyInviteTemplate, which decides which shape a given template wants.
function buildInviteParams(addressee, addresseeContact, greeting, members) {
  const params = {
    greeting,
    primary_email: addresseeContact,
    primary_first_name: firstName(addressee.fullName),
    primary_last_name: lastName(addressee.fullName),
    primary_phone: addressee.phoneMasked ? '' : (addressee.phone || ''),
  };
  members.forEach((m, i) => {
    params[`member_${i + 1}`] = m.contact ? `${m.name} <${m.contact}>` : m.name;
  });
  return {
    greeting,
    people: [{name: addressee.fullName, contact: addresseeContact}, ...members],
    params,
  };
}

// A kid's contact, per state.gvKidEmail: off lists them without a way to
// reach them directly, on gives them their own reachable address (blank if
// they don't have one, or it's a masked Veracross placeholder).
function kidContact(kid) {
  return state.gvKidEmail && kid.email && !kid.emailMasked ? kid.email : '';
}

// A family's adults with a real, reachable email - a Veracross placeholder
// address doesn't belong on a mailing list, matching emailEntries - deduped,
// since a family's own adultEmails list is assumed unique but a caller
// merging more than one source (none currently do) shouldn't double-invite
// someone as a result.
function parentContactsOf(adultEmails) {
  const emails = [...new Set(adultEmails || [])];
  return emails.map(e => byEmail[e]).filter(a => a && !a.emailMasked);
}

// A family's kids to actually put on the invite. With Include siblings on,
// that's everyone. Off, it's still not necessarily nobody: if the current
// filters/search single out specific students (a grade, a tag, a name typed
// into search), whichever of this family's kids match are the ones actually
// being invited, not incidental siblings - they stay on regardless, and it's
// only their non-matching siblings that Include siblings is left to add back
// in. With no filter or search narrowing things down at all, there's no
// "invited kid" to distinguish from a sibling, so off just means no kids.
function invitedKids(family) {
  const kids = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
  if (state.gvSiblings) {
    return kids;
  }
  if (!anyFiltersActive() && !state.q) {
    return [];
  }
  return kids.filter(k => matchesFilters(k) &&
    (!state.q || k.fullName.toLowerCase().includes(state.q) || k.email.toLowerCase().includes(state.q)));
}

// Group mode: one row per family. The primary contact is the first listed
// adult with a real, reachable email (matching emailEntries' rule that a
// Veracross placeholder address doesn't belong on a mailing list); everyone
// else (other adults, then kids per invitedKids) becomes a member. Returns
// null for a family with no usable primary contact at all.
function familyInviteParams(family) {
  const adults = (family.adultEmails || []).map(e => byEmail[e]).filter(p => p && !p.emailMasked);
  if (!adults.length) {
    return null;
  }
  const kids = invitedKids(family);
  const [primary, ...otherAdults] = adults;
  const format = inviteGreetings.find(g => g.name === state.gvGreeting) || greetingFormatsFor('group', true)[0];
  const greeting = format ? buildGreeting(format.format, {kids, adults}) : '';
  const members = otherAdults.map(a => ({name: a.fullName, contact: a.email}))
    .concat(kids.map(k => ({name: k.fullName, contact: kidContact(k)})));
  const anchor = kids[0] || primary;
  return {
    ...buildInviteParams(primary, primary.email, greeting, members),
    linkHref: familyLink(family.key),
    sortKey: lastName(anchor.fullName) + ' ' + firstName(anchor.fullName),
  };
}

// One real contact email can only ever appear once in Individual mode's
// output - never two rows to the same inbox, whatever caused the collision
// (two siblings who each separately qualify, a parent who also independently
// qualifies as their own invitee, or - with Include siblings off - two
// separately-matching kids who just happen to share a parent). Rather than
// dropping whichever candidate loses the collision (silently losing a real
// kid's invite), every candidate reaching the same contact is merged into one
// row that names everyone routed there - see mergeCandidates.
//
// A candidate is {contact, person}: person is who this candidate is "about"
// (whose name should count toward the merged greeting), contact is where it
// would be delivered - blank if a student has no parent on record to route
// through (never merged with anything else, so that student's own row still
// shows up rather than silently vanishing).
function individualCandidates(p) {
  if (!p.isStudent) {
    return p.emailMasked ? [] : [{contact: p.email, person: p}];
  }
  const family = familyOf(p);
  // With Include siblings on, every kid in the household is folded into the
  // same set of candidates the first time any of them is reached - the later
  // merge pass then naturally collapses them into one row per parent, same as
  // it would for any other collision.
  let kids = [p];
  if (state.gvSiblings && family) {
    kids = invitedKids(family);
  }
  const parents = family ? parentContactsOf(family.adultEmails) : [];
  if (!parents.length) {
    return kids.map(kid => ({contact: '', person: kid}));
  }
  const candidates = [];
  for (const parent of parents) {
    for (const kid of kids) {
      candidates.push({contact: parent.email, person: kid});
    }
  }
  return candidates;
}

// Turns a merged group of people (everyone whose invite collided on the same
// contact - see individualCandidates) into one row: the first person is the
// addressee (whoever's own name/email columns represent the row - an
// independently-matching adult, if there is one, since candidates are built
// adults-first), everyone else is folded into the greeting as "siblings" (see
// buildGreeting - the name is a holdover from the common case, but this works
// for any group of people sharing one contact, related or not).
function buildMergedEntry(people, contact) {
  const addressee = people[0];
  const format = inviteGreetings.find(g => g.name === state.gvGreeting) || greetingFormatsFor('individual', false)[0];
  const greeting = format ? buildGreeting(format.format, {person: addressee, siblings: people.slice(1)}) : '';
  return {
    ...buildInviteParams(addressee, contact, greeting, []),
    linkHref: personLink(addressee),
    sortKey: lastName(addressee.fullName) + ' ' + firstName(addressee.fullName),
  };
}

// Groups every candidate (see individualCandidates) by contact email and
// builds one merged row per group (see buildMergedEntry) - the one place
// Individual mode's "never the same email twice" rule is actually enforced.
// A blank contact (no parent on record) is never merged with anything else,
// each becomes its own row.
function mergeCandidates(candidates) {
  const byContact = new Map();
  const rows = [];
  for (const c of candidates) {
    if (!c.contact) {
      rows.push(buildMergedEntry([c.person], ''));
      continue;
    }
    if (!byContact.has(c.contact)) {
      byContact.set(c.contact, []);
    }
    const people = byContact.get(c.contact);
    if (!people.some(x => x.email === c.person.email)) {
      people.push(c.person);
    }
  }
  for (const [contact, people] of byContact) {
    rows.push(buildMergedEntry(people, contact));
  }
  return rows;
}

// Every family (Group mode) or person (Individual mode) with a usable
// addressee, filtered and searched the same way the rest of the list pages
// are - familyMatchesFilters/familySearchText for Group mode (the same ones
// the Families tab and the map rely on), plain matchesFilters/name-or-email
// search for Individual mode, matching emailEntries. Sorted by last name -
// the anchor kid's, for a family, matching the order the original Greenvelope
// sheet this page grew out of already used.
function invitesEntries() {
  const rows = [];
  if (state.gvInviteBy === 'individual') {
    const matches = p => matchesFilters(p) && (p.fullName.toLowerCase().includes(state.q) || p.email.toLowerCase().includes(state.q));
    const candidates = [];
    // Adults first, deliberately: when an adult's own invite and their kid's
    // routed-through-them invite land on the same contact and get merged, the
    // adult - added to the candidate list first - is the one buildMergedEntry
    // picks as the addressee.
    for (const p of state.model.people) {
      if (!p.isStudent && matches(p)) {
        candidates.push(...individualCandidates(p));
      }
    }
    // With Include siblings on, a household's candidates all get generated
    // together the first time any of its kids is reached (individualCandidates
    // expands to the whole household); mergedFamilies then skips that
    // household's other kids so they don't generate the same candidates again.
    const mergedFamilies = new Set();
    for (const p of state.model.people) {
      if (!p.isStudent || !matches(p)) {
        continue;
      }
      if (state.gvSiblings) {
        const family = familyOf(p);
        if (family) {
          if (mergedFamilies.has(family.key)) {
            continue;
          }
          mergedFamilies.add(family.key);
        }
      }
      candidates.push(...individualCandidates(p));
    }
    rows.push(...mergeCandidates(candidates));
  } else {
    for (const family of Object.values(state.model.families)) {
      if (!familyMatchesFilters(family.key) || !familySearchText(family).includes(state.q)) {
        continue;
      }
      const entry = familyInviteParams(family);
      if (entry) {
        rows.push(entry);
      }
    }
  }
  rows.sort((a, b) => a.sortKey.localeCompare(b.sortKey));
  return rows;
}

// Fills a template string's {{ token }} placeholders from params; a token with
// no value (including one this app doesn't compute at all, like Paperless
// Post's optional Message column) just renders blank rather than erroring.
function fillTemplate(template, params) {
  return (template || '').replace(/\{\{\s*([\w.]+)\s*\}\}/g, (_, key) => params[key] ?? '');
}

// Whether a template's row(s) use the per-member tokens (member_name/
// member_contact) rather than the numbered member_N slots - a template that
// does (Paperless Post's, so far) wants one output row per family MEMBER, not
// one row per family; see that system's Notes in the Invite List Builder sheet.
function templateRepeatsPerMember(rows) {
  return rows.some(row => row.some(cell => /\{\{\s*member_(name|contact)\s*\}\}/.test(cell || '')));
}

// Whether a template actually uses {{ greeting }} anywhere - the Couple /
// Family greeting picker only matters, and only shows, for a system whose
// template references it. A system that greets some other way (or not at all)
// has no use for the setting.
function templateUsesGreeting(rows) {
  return rows.some(row => row.some(cell => /\{\{\s*greeting\s*\}\}/.test(cell || '')));
}

// Turns one invite system's template (its tab's header + template row, as read
// from the Invite List Builder sheet by the server) into real output rows for
// the given family entries - the one place a sheet template plus computed
// family params actually becomes a CSV. Row shape (one per family, or one per
// family member) is inferred from which tokens the template uses, not
// hardcoded per system, so a new system's tab just needs the right tokens.
function applyInviteTemplate(system, entries) {
  const templateRow = system.rows[0] || [];
  const perMember = templateRepeatsPerMember(system.rows);
  const rows = [];
  for (const entry of entries) {
    if (!perMember) {
      rows.push({entry, cells: system.header.map((_, i) => fillTemplate(templateRow[i], entry.params))});
      continue;
    }
    for (const person of entry.people) {
      const params = {...entry.params, member_name: person.name, member_contact: person.contact};
      rows.push({entry, cells: system.header.map((_, i) => fillTemplate(templateRow[i], params))});
    }
  }
  return rows;
}

// Cached after the first successful fetch - the templates only change when
// someone edits the Invite List Builder sheet, not per page visit.
let inviteSystems = null;
let inviteGreetings = [];
let inviteLoadError = '';

async function loadInviteSystems() {
  if (inviteSystems) {
    return inviteSystems;
  }
  const res = await fetch('/api/directory/invite-templates');
  const body = await res.json().catch(() => ({}));
  inviteSystems = body.systems && body.systems.length ? body.systems : [];
  inviteGreetings = body.greetings || [];
  inviteLoadError = body.error || '';
  if (body.error) {
    console.error('invite templates:', body.error);
  }
  refreshGreetingPhrases();
  return inviteSystems;
}

// The formatted-CSV export page: pick a destination system (Greenvelope, Evite,
// ... - whatever the Invite List Builder sheet's _Services tab lists), then export
// the currently filtered families through that system's own template. One row
// per family, or one row per family member, depending on the template - see
// applyInviteTemplate.
function renderGreenvelopePage() {
  const main = resetMain();

  const pageHeader = el('div', 'page-header container page-header-list');
  const titleWrap = el('div');
  titleWrap.append(el('h1', 'page-title', 'Invites'));
  titleWrap.append(el('div', 'page-subtitle', 'Export a formatted CSV of families, grouped and greeted the way you choose, for invitations.'));
  pageHeader.append(titleWrap);
  main.append(pageHeader);

  const settings = el('div', 'gv-settings container');
  const loading = el('div', 'gv-setting-group', 'Loading invite templates…');
  settings.append(loading);
  main.append(settings);

  const content = el('div', 'content container');
  main.append(content);

  loadInviteSystems().then(systems => {
    if (!systems.length) {
      settings.replaceChildren();
      content.replaceChildren();
      settings.append(el('div', 'gv-setting-group', inviteLoadError
        ? `Couldn't load invite templates: ${inviteLoadError}`
        : 'No invite templates are set up yet - add one to the Invite List Builder sheet\'s _Services tab.'));
      return;
    }
    if (!systems.some(s => s.name === state.gvSystem)) {
      state.gvSystem = systems[0].name;
    }

    // Rebuilds the whole page: needed whenever which system is selected
    // changes, since that decides which other settings (right now, just the
    // Couple / Family greeting picker) even apply. Everything else (role
    // chips, search, Grade/Classroom/Tags) only needs the cheaper renderGrid,
    // wired up below - renderGrid is a hoisted declaration, so it's already
    // callable here even though it's defined later in this same function.
    function renderAll() {
      settings.replaceChildren();
      content.replaceChildren();
      renderInviteSettings(systems, settings, renderAll, renderGrid);

      const header = el('div', 'content-header content-header-solo');
      const controls = el('div', 'controls');
      controls.append(roleChips(renderGrid));
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
        facetDropdown('Grade', gradeOptions(), state.filterGrades, renderGrid),
        facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, renderGrid),
      );
      if (tagNames().length) {
        // Rebuilds the whole page, not just the grid - selecting down to (or
        // away from) exactly one tag changes whether the Include control just
        // below even applies.
        controls.append(facetDropdown('Tags', tagNames(), state.filterTags, renderAll));
      }
      // Who to include beyond the tagged person themselves - only meaningful
      // for a single tag; with several selected at once (or none) there's no
      // one list to pull relatives in from. Mirrors renderListPage's identical
      // control exactly, down to persisting the choice per tag.
      if (state.filterTags.size === 1) {
        const [activeTag] = state.filterTags;
        controls.append(facetDropdown('Include', tagRelationOptions, state.filterTagRelations, () => {
          saveTagRelations(activeTag, state.filterTagRelations);
          renderGrid();
        }));
      }
      const download = el('a', 'filter-button email-download');
      download.title = 'Download what the list currently shows';
      download.append(svg('download'), el('span', '', 'CSV'));
      controls.append(search, download);
      header.append(controls);
      content.append(header);

      const grid = el('div');
      content.append(grid);

      function renderGrid() {
        grid.replaceChildren();
        grid.className = '';
        const system = systems.find(s => s.name === state.gvSystem) || systems[0];
        const entries = invitesEntries();
        const rows = applyInviteTemplate(system, entries);

        const csvLines = rows.map(r => r.cells.map(csvField).join(','));
        if (system.headerRow) {
          csvLines.unshift(system.header.map(csvField).join(','));
        }
        download.href = 'data:text/csv;charset=utf-8,' + encodeURIComponent(csvLines.join('\n'));
        download.download = slugify(system.name) + '.csv';

        if (!rows.length) {
          grid.append(el('div', 'empty', 'No matches.'));
          return;
        }

        grid.className = 'email-holder';
        const table = el('table', 'email-table');
        const thead = el('thead');
        const headRow = el('tr');
        // Whichever column the template maps to {{ greeting }} gets the
        // format picker right in its header, instead of a plain label - the
        // same choice as the Couple / Family greeting setting above (changing
        // either updates both, since both just read/write state.gvGreeting).
        const greetingCol = system.rows[0] ? system.rows[0].findIndex(cell => /\{\{\s*greeting\s*\}\}/.test(cell || '')) : -1;
        const leadTh = el('th', 'email-copy-cell');
        const copyTable = el('button', 'email-copy-columns');
        copyTable.title = 'Copy the whole table to the clipboard';
        copyTable.append(svg('copy'));
        copyTable.addEventListener('click', () => {
          const lines = [system.header.join('\t')].concat(rows.map(r => r.cells.join('\t')));
          navigator.clipboard.writeText(lines.join('\n'));
          copyTable.classList.add('copied');
          copyTable.replaceChildren(svg('check'));
          setTimeout(() => {
            copyTable.classList.remove('copied');
            copyTable.replaceChildren(svg('copy'));
          }, 1200);
        });
        leadTh.append(copyTable);
        headRow.append(leadTh);
        system.header.forEach((label, i) => {
          const th = el('th');
          if (i === greetingCol) {
            const headSelect = el('select', 'gv-select gv-th-select');
            const formats = greetingFormatsFor(state.gvInviteBy, system.supportsGroups);
            const meEmail = document.body.dataset.userEmail;
            const mine = formats.filter(f => f.createdBy && f.createdBy === meEmail);
            const appendOptions = (parent, list) => {
              for (const f of list) {
                const option = el('option', '', f.name);
                option.value = f.name;
                parent.append(option);
              }
            };
            // Only splits into "Yours"/"Everyone's" once there's actually
            // something of the viewer's own to set apart - otherwise it's
            // just the one flat list it's always been.
            if (mine.length) {
              const mineGroup = el('optgroup');
              mineGroup.label = 'Yours';
              appendOptions(mineGroup, mine);
              headSelect.append(mineGroup);
              const everyoneGroup = el('optgroup');
              everyoneGroup.label = 'Everyone’s';
              appendOptions(everyoneGroup, formats.filter(f => !mine.includes(f)));
              headSelect.append(everyoneGroup);
            } else {
              appendOptions(headSelect, formats);
            }
            const divider = el('option', '', '──────────');
            divider.disabled = true;
            headSelect.append(divider);
            const editOption = el('option', '', '(Edit Greetings)');
            editOption.value = '__new__';
            headSelect.append(editOption);
            headSelect.value = state.gvGreeting;
            headSelect.addEventListener('change', () => {
              if (headSelect.value === '__new__') {
                headSelect.value = state.gvGreeting;
                openGreetingDialog(renderGreenvelopePage);
                return;
              }
              state.gvGreeting = headSelect.value;
              renderAll();
            });
            th.append(headSelect);
          } else {
            th.append(el('span', '', label));
          }
          headRow.append(th);
        });
        thead.append(headRow);
        table.append(thead);
        const tbody = el('tbody');
        rows.forEach((r, rowIndex) => {
          const tr = el('tr');
          const num = el('td', 'email-num');
          num.append(el('span', '', String(rowIndex + 1)), copyGlyph(r.cells.join('\t')));
          tr.append(num);
          r.cells.forEach((value, i) => {
            const td = el('td', i === 0 ? 'email-name' : '', i === 0 ? '' : value);
            if (i === 0) {
              const link = el('a', '', value);
              link.href = r.entry.linkHref;
              td.append(link);
            }
            tr.append(td);
          });
          tbody.append(tr);
        });
        table.append(tbody);
        grid.append(table);
      }
      renderGrid();
      input.focus();
    }
    renderAll();
  });
}

// Real per-service icons (small favicons, not full marketing logos) so the
// destination is recognizable at a glance in the Export for picker. Keyed by
// the exact Display Name from the Invite List Builder's _Services tab; a
// system added there without a matching icon here just falls back to a
// colored initial, the same way a person with no photo does elsewhere in
// this app (see photoOrInitials).
const serviceLogos = {
  Greenvelope: '/static/directory/services/greenvelope.png',
  Evite: '/static/directory/services/evite.png',
  'Paperless Post': '/static/directory/services/paperless-post.png',
  Punchbowl: '/static/directory/services/punchbowl.png',
  Partiful: '/static/directory/services/partiful.jpg',
};

function serviceIcon(name) {
  const url = serviceLogos[name];
  if (url) {
    const img = el('img', 'gv-service-logo');
    img.src = url;
    img.alt = '';
    return img;
  }
  const div = el('div', 'gv-service-logo gv-service-fallback', (name[0] || '?').toUpperCase());
  div.style.background = `hsl(${hue(name)} 45% 55%)`;
  return div;
}

// A custom Export for picker in place of a plain <select> - only so each
// system's icon (serviceIcon) can show both on the closed button and in the
// open list, which a native select can't render at all. Built from the app's
// existing .filter-button/.filter-panel/.filter-option classes so the one
// global click-outside handler that already closes every other dropdown in
// the app (see the document click listener near closeFilterPanels) closes
// this one too, for free.
function gvServiceSelect(seg, systems, onChange) {
  const wrap = el('div', 'filter-wrap gv-service-wrap');
  const button = el('button', 'filter-button gv-service-button');
  button.type = 'button';
  const current = systems.find(s => s.name === state.gvSystem) || systems[0];
  button.append(serviceIcon(current.name), el('span', '', current.name), svg('chevron'));
  const panel = el('div', 'filter-panel gv-service-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
  });
  for (const s of systems) {
    const option = el('div', 'filter-option gv-service-option');
    option.append(serviceIcon(s.name), el('span', '', s.name));
    option.addEventListener('click', () => onChange(s.name));
    panel.append(option);
  }
  wrap.append(button, panel);
  seg.append(wrap);
}

// The default starting point for a brand new greeting - a worked example
// that already reads as a real greeting rather than an empty box, using the
// same made-up family (kids Ali & Bo, parents Pat & Quinn, surname Ender) the
// recognized phrases below are drawn from, so it edits cleanly into a real
// one just by trimming or rearranging. Mode-dependent because the phrases
// that actually substitute anything differ: a family's kids/adults are only
// ever populated for a Grouped call, a lone person only for an Individual
// one (see buildGreeting) - defaulting to the wrong kind would silently
// render as flat, unpersonalized text for every row, same as the bug this
// default (and the live preview below) exists to avoid.
function defaultGreetingFormat() {
  return state.gvInviteBy === 'group' ? `The ${GREETING_WHOLE_FAMILY} Family` : `Dear ${GREETING_FIRST_NAME}`;
}

// A second, differently-named made-up family (not Ender) to run a candidate
// Format through live as the person types - proves whether it actually
// personalizes per family rather than just echoing the Ender example back
// unchanged, which would look "correct" without proving the substitution
// fired at all.
const GREETING_PREVIEW_FAMILY = {
  kids: [{fullName: 'Nora Rivera'}, {fullName: 'Theo Rivera'}],
  adults: [{fullName: 'Sam Rivera'}, {fullName: 'Jamie Rivera'}],
};

// The pop-up behind "(Edit Greetings)" at the bottom of the greeting picker
// in the table header - lists whatever custom greetings the viewer has
// already created, lets picking one load it below for editing, and otherwise
// starts a new one. There's no separate name to write: a greeting's Format
// text doubles as its Name (see GreetingTemplate in invites.go), so the only
// field here is the greeting itself. A true modal (built like openCropTool's
// overlay - appended to document.body, closes on backdrop click or Escape)
// rather than an anchored dropdown panel, since the form is too tall to hang
// cleanly off a table header cell. onSaved is called after a successful
// save, with inviteSystems already cleared so the next fetch picks up the
// change.
function openGreetingDialog(onSaved) {
  const overlay = el('div', 'greeting-dialog-overlay');
  const panel = el('div', 'greeting-dialog-panel');

  const close = () => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  };
  function onKey(e) {
    if (e.key === 'Escape') {
      close();
    }
  }
  overlay.addEventListener('click', e => {
    if (e.target === overlay) {
      close();
    }
  });
  document.addEventListener('keydown', onKey);

  const header = el('div', 'greeting-dialog-header');
  const headerIcon = el('div', 'greeting-dialog-icon');
  headerIcon.append(svg('sparkles'));
  header.append(headerIcon);
  const headerText = el('div', 'greeting-dialog-header-text');
  headerText.append(
    el('div', 'greeting-dialog-title', 'Edit Greeting'),
    el('div', 'greeting-dialog-subtitle', 'Create a sample greeting for this family.'),
  );
  header.append(headerText);
  const headerClose = el('button', 'greeting-dialog-close');
  headerClose.type = 'button';
  headerClose.setAttribute('aria-label', 'Close');
  headerClose.append(svg('x'));
  headerClose.addEventListener('click', close);
  header.append(headerClose);
  panel.append(header);

  const form = el('form', 'gv-new-greeting-form');

  // Fixed for the life of this dialog - which kind of call (a family's
  // kids/adults, or one person) this greeting will actually be run through
  // is decided by which mode it's saved under, not by anything typed here.
  const grouped = state.gvInviteBy === 'group';

  // original names the existing _Greetings row currently loaded into the
  // field below (so Save edits it in place), or '' while writing a new one.
  let original = '';
  let cards = [];

  const formatLabel = el('label', 'gv-new-greeting-field');
  formatLabel.append(el('span', '', 'Greeting'));
  const formatInput = el('input');
  formatInput.maxLength = 200;
  formatInput.required = true;
  formatLabel.append(formatInput);

  // Runs the candidate text through buildGreeting against a sample family
  // that isn't Ender, live as the person types - the whole point is to make
  // it obvious when a phrase doesn't actually match (a typo, wrong
  // punctuation, names in the wrong order) instead of silently saving flat
  // text that renders identically for every family, which is exactly what
  // happened before this preview existed.
  const previewValue = el('span', 'gv-new-greeting-preview-value');
  const preview = el('div', 'gv-new-greeting-preview');
  preview.append(el('div', 'gv-new-greeting-preview-label', 'Preview'), previewValue);
  const previewNote = el('div', 'gv-new-greeting-preview-note',
    'No phrase matched, so this will show exactly as typed for every family - fine for a fixed line, not if you meant to personalize it.');
  previewNote.hidden = true;
  function updatePreview() {
    const format = formatInput.value;
    const rendered = grouped
      ? buildGreeting(format, GREETING_PREVIEW_FAMILY)
      : buildGreeting(format, {person: GREETING_PREVIEW_FAMILY.adults[0]});
    previewValue.textContent = rendered || '(empty)';
    previewNote.hidden = rendered !== format;
  }
  formatInput.addEventListener('input', updatePreview);

  // Holds the Greeting field itself plus everything tied to it (preview,
  // hint, Cancel/Save) - hidden until there's actually something to edit, so
  // opening the dialog onto an existing list of greetings doesn't also throw
  // a blank editor at the person before they've picked (or asked for) one.
  const editorSection = el('div', 'gv-greeting-editor');

  // Loads an existing greeting into the field to edit it, or (passed null,
  // from Add another greeting) clears it back to a fresh default - either
  // way this is the one place that changes what Save will do, and what
  // reveals the editor in the first place.
  function loadGreeting(g) {
    original = g ? g.name : '';
    formatInput.value = g ? g.format : defaultGreetingFormat();
    cards.forEach(({card, greeting}) => card.classList.toggle('active', greeting === g));
    editorSection.hidden = false;
    updatePreview();
    formatInput.focus();
    formatInput.select();
  }

  const meEmail = document.body.dataset.userEmail;
  const mine = inviteGreetings.filter(g => g.createdBy && g.createdBy === meEmail);
  if (mine.length) {
    const listWrap = el('div', 'gv-greeting-list');
    listWrap.append(el('div', 'gv-greeting-list-title', 'Your greetings'));
    for (const g of mine) {
      const card = el('div', 'gv-greeting-card');
      card.append(el('span', 'gv-greeting-card-text', g.format));
      const cardActions = el('div', 'gv-greeting-card-actions');
      const editBtn = el('button', 'gv-greeting-card-btn');
      editBtn.type = 'button';
      editBtn.title = 'Edit';
      editBtn.append(svg('pencil'));
      editBtn.addEventListener('click', () => loadGreeting(g));
      const deleteBtn = el('button', 'gv-greeting-card-btn gv-greeting-card-btn-delete');
      deleteBtn.type = 'button';
      deleteBtn.title = 'Delete';
      deleteBtn.append(svg('trash'));
      deleteBtn.addEventListener('click', async () => {
        if (!confirm(`Delete "${g.format}"? This can't be undone.`)) {
          return;
        }
        error.hidden = true;
        deleteBtn.disabled = true;
        try {
          // As a query parameter, not a body - Go's ParseForm only reads a
          // request body for POST/PUT/PATCH, so a DELETE's body would
          // silently go unread server-side and every delete would 400 as
          // "missing name".
          const res = await fetch(`/api/directory/greetings?${new URLSearchParams({name: g.name})}`, {method: 'DELETE'});
          if (!res.ok) {
            throw new Error(await res.text());
          }
          inviteSystems = null;
          close();
          onSaved();
        } catch (err) {
          error.hidden = false;
          error.textContent = 'Couldn’t delete that greeting - try again.';
          deleteBtn.disabled = false;
        }
      });
      cardActions.append(editBtn, deleteBtn);
      card.append(cardActions);
      cards.push({card, greeting: g});
      listWrap.append(card);
    }
    const addButton = el('button', 'gv-greeting-add');
    addButton.type = 'button';
    const addIcon = el('span', 'gv-greeting-add-icon');
    addIcon.append(svg('plus'));
    addButton.append(addIcon, el('span', '', 'Add another greeting'));
    addButton.addEventListener('click', () => loadGreeting(null));
    listWrap.append(addButton);
    form.append(listWrap);
    // Lives inside editorSection (its top edge) rather than out here, so it
    // only shows once there's an editor below it to divide the list from.
    editorSection.append(el('div', 'gv-greeting-divider'));
  }

  formatInput.value = defaultGreetingFormat();
  editorSection.append(formatLabel);
  editorSection.append(preview, previewNote);
  updatePreview();

  // Same worked-example names buildGreeting recognizes - see GreetingTemplate
  // in invites.go for why this is examples, not {{ token }} syntax.
  const hint = el('div', 'gv-new-greeting-hint',
    'Use the family’s actual names (e.g., Pat, Quinn, Ali, Bo) in your own words.');
  editorSection.append(hint);

  const error = el('div', 'gv-new-greeting-error');
  error.hidden = true;
  editorSection.append(error);

  const actions = el('div', 'gv-new-greeting-actions');
  const cancel = el('button', 'gv-new-greeting-cancel', 'Cancel');
  cancel.type = 'button';
  cancel.addEventListener('click', close);
  const save = el('button', 'filter-done gv-new-greeting-save', 'Save');
  save.type = 'submit';
  actions.append(cancel, save);
  editorSection.append(actions);
  // Nothing to browse first (no custom greetings yet) - open straight into
  // the editor instead of showing an empty "Your greetings" list with
  // nothing but an Add button in it.
  editorSection.hidden = mine.length > 0;
  form.append(editorSection);

  form.addEventListener('submit', async e => {
    e.preventDefault();
    const format = formatInput.value.trim();
    if (!format) {
      return;
    }
    error.hidden = true;
    save.disabled = true;
    try {
      // Applies to whichever mode the picker this dialog was opened from is
      // currently showing, not asked for - Group by family vs. Individual is
      // already a choice the person made just above the table, and asking
      // them to repeat it here would just be one more thing to get wrong.
      const body = new URLSearchParams({
        format,
        original,
        grouped: state.gvInviteBy === 'group' ? '1' : '0',
        individual: state.gvInviteBy !== 'group' ? '1' : '0',
      });
      const res = await fetch('/api/directory/greetings', {method: 'POST', body});
      if (!res.ok) {
        throw new Error(await res.text());
      }
      // Cleared so the next loadInviteSystems call refetches instead of
      // serving the cached list, which is missing the row just changed.
      inviteSystems = null;
      state.gvGreeting = format;
      close();
      onSaved();
    } catch (err) {
      error.hidden = false;
      error.textContent = 'Couldn’t save that greeting - try again.';
      save.disabled = false;
    }
  });

  panel.append(form);
  overlay.append(panel);
  document.body.append(overlay);
  formatInput.focus();
  formatInput.select();
}

// A settings-bar segment: a label (with an optional info button explaining a
// less self-evident setting) above whatever control the caller appends next -
// a <select> for Export for, a switch for everything else.
function gvSegment(row, label, infoText) {
  const seg = el('div', 'gv-settings-segment');
  const labelRow = el('div', 'gv-setting-label');
  labelRow.append(el('span', '', label));
  if (infoText) {
    const info = el('button', 'gv-info-btn');
    info.type = 'button';
    info.title = infoText;
    info.setAttribute('aria-label', label + ': ' + infoText);
    info.append(svg('info'));
    labelRow.append(info);
  }
  seg.append(labelRow);
  row.append(seg);
  return seg;
}

function gvSwitch(seg, checked, onChange) {
  const toggle = el('input', 'filter-switch');
  toggle.type = 'checkbox';
  toggle.checked = checked;
  toggle.addEventListener('change', () => onChange(toggle.checked));
  seg.append(toggle);
  return toggle;
}

// One bar - Export for, then whichever of Group by family/Include siblings/
// Send to Kid Emails apply - divided into segments rather than separate
// cards, with the selected system's own description as a single summary line
// underneath instead of scattered per-control. onSystemChange rebuilds this
// whole bar (and the page around it) - needed whenever which system, or Group
// vs. Individual, changes, since those decide which segments even show and
// which greeting options apply (chosen in the table header - see renderGrid's
// greetingCol). onSettingChange just re-runs the grid, for Include siblings/
// Send to Kid Emails, which never change what's shown here.
function renderInviteSettings(systems, settings, onSystemChange, onSettingChange) {
  const system = systems.find(s => s.name === state.gvSystem) || systems[0];
  // A system with no group concept has no Group by family choice to offer -
  // it's always Individual, silently, rather than a switch that doesn't work.
  if (!system.supportsGroups) {
    state.gvInviteBy = 'individual';
  }
  // Keep state.gvGreeting valid for whichever formats currently apply (see
  // greetingFormatsFor) - switching Group by family or system can leave a
  // stale key selected that's no longer one of the current options.
  if (templateUsesGreeting(system.rows)) {
    const formats = greetingFormatsFor(state.gvInviteBy, system.supportsGroups);
    if (formats.length && !formats.some(f => f.name === state.gvGreeting)) {
      state.gvGreeting = formats[0].name;
    }
  }

  const bar = el('div', 'gv-settings-bar');
  const row = el('div', 'gv-settings-row');
  bar.append(row);

  const exportSeg = gvSegment(row, 'Export for');
  gvServiceSelect(exportSeg, systems, name => {
    const next = systems.find(s => s.name === name);
    state.gvSystem = name;
    // Picking a different system resets Group by family to that system's own
    // default (on if it supports one, otherwise off) rather than carrying
    // over whatever the previous system happened to be set to.
    state.gvInviteBy = next && next.supportsGroups ? 'group' : 'individual';
    onSystemChange();
  });

  // Hidden entirely for a system with no group concept (Punchbowl, Partiful) -
  // there's no Group option to offer, so it's always Individual, silently.
  if (system.supportsGroups) {
    row.append(el('div', 'gv-settings-divider'));
    const groupSeg = gvSegment(row, 'Group by family',
      'On: one invite per family, addressed to everyone in the household together. Off: one invite per person.');
    gvSwitch(groupSeg, state.gvInviteBy === 'group', checked => {
      state.gvInviteBy = checked ? 'group' : 'individual';
      onSystemChange();
    });
  }

  row.append(el('div', 'gv-settings-divider'));
  const siblingsSeg = gvSegment(row, 'Include siblings', 'Will include the children or siblings of invitees.');
  gvSwitch(siblingsSeg, state.gvSiblings, checked => {
    state.gvSiblings = checked;
    onSettingChange();
  });

  // Only meaningful in Group mode, where it decides how a sibling appears in
  // the family's member columns (name only, or name plus their own email). In
  // Individual mode a student is never emailed directly at all - see
  // personInviteParams - so there's nothing here for this switch to control.
  if (state.gvInviteBy === 'group') {
    row.append(el('div', 'gv-settings-divider'));
    const emailSeg = gvSegment(row, 'Send to Kid Emails', 'Will include the student emails of the invitees.');
    gvSwitch(emailSeg, state.gvKidEmail, checked => {
      state.gvKidEmail = checked;
      onSettingChange();
    });
  }

  if (system.description) {
    bar.append(el('div', 'gv-fyi gv-settings-summary', system.description));
  }
  settings.append(bar);
}

function personFacets(p, field) {
  if (p.isStudent) {
    return p[field] ? [p[field]] : [];
  }
  if (p.isParent) {
    const family = familyOf(p);
    return ((family && family.kidEmails) || []).map(e => byEmail[e]).filter(Boolean).map(k => k[field]).filter(Boolean);
  }
  return [];
}

function cityOf(p) {
  const family = familyOf(p);
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

// Role chips currently show on the Directory page's Everyone tab, on every
// list page (renderListPage: a tag's list, or the plain Everyone list at
// /email-list), and on the Invites page - not on Directory's Families tab,
// which has no chips of its own to reveal that anything is filtered. Keyed off
// the route rather than state.tab so a stale tab value left over from a
// different page can't make filterRoleExcluded apply (or not) on the wrong
// page.
function roleChipsVisible() {
  const seg = segments();
  if (seg[0] === 'email-list' || seg[0] === 'greenvelope') {
    return true;
  }
  if (seg[0] === 'people') {
    // A tag list (renderListPage) always shows the chips regardless of
    // state.tab, which it doesn't touch - only the plain Directory (renderPeople)
    // gates them on actually being on its own Everyone tab.
    return state.filterTags.size > 0 || state.tab === 'everyone';
  }
  return false;
}

const tagRelationOptions = ['Parents', 'Children', 'Siblings'];

// Whether p only belongs on a single-tag list by way of a selected relation
// to someone directly tagged - a parent of a tagged kid, a kid of a tagged
// parent, or another kid (a sibling) in a tagged kid's family. Only applies
// with exactly one active tag; with several selected at once there's no
// single list to pull relatives in from.
function tagRelatedMatch(p) {
  if (state.filterTags.size !== 1 || !state.filterTagRelations.size) {
    return false;
  }
  const [tag] = state.filterTags;
  const tagged = tags[tag] || [];
  for (const family of familiesOf(p)) {
    if (state.filterTagRelations.has('Parents') && p.isParent &&
      (family.kidEmails || []).some(e => tagged.includes(e))) {
      return true;
    }
    if (state.filterTagRelations.has('Children') && p.isStudent &&
      (family.adultEmails || []).some(e => tagged.includes(e))) {
      return true;
    }
    if (state.filterTagRelations.has('Siblings') && p.isStudent &&
      (family.kidEmails || []).some(e => e !== p.email && tagged.includes(e))) {
      return true;
    }
  }
  return false;
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
  const pronounsOK = !state.filterPronouns.size || (p.pronouns && state.filterPronouns.has(p.pronouns.toLowerCase()));
  const newOK = !state.filterNew || p.isNew;
  const tagOK = !state.filterTags.size || tagsOf(p.email).some(t => state.filterTags.has(t)) || tagRelatedMatch(p);
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
  return [...new Set(state.model.people.map(p => p.pronouns).filter(Boolean).map(p => p.toLowerCase()))].sort();
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

// .filter-panel is CSS-anchored to its wrap's right edge (right:0), which only
// fits on screen when the trigger button sits near the right side of the
// viewport - true for the old lone "Filter" button, not for Grade/Classroom/
// Tags now sitting further left, especially once the controls row wraps on
// mobile. Clamping with an explicit left (converted back to wrap-relative,
// since the panel is absolutely positioned inside its position:relative wrap)
// keeps the panel fully on screen regardless of where its button lands.
function clampFilterPanel(wrap, panel) {
  const margin = 12;
  const wrapRect = wrap.getBoundingClientRect();
  const panelWidth = panel.offsetWidth;
  let left = wrapRect.right - panelWidth;
  left = Math.max(margin, Math.min(left, window.innerWidth - margin - panelWidth));
  panel.style.left = `${left - wrapRect.left}px`;
  panel.style.right = 'auto';
}

// A standalone single-facet dropdown (Grade, Classroom) - the same checkbox
// list a filterControl section would show, but its own button so it doesn't
// need the drill-into-a-section step.
function facetDropdown(label, values, set, rerender) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button facet-button');
  const labelSpan = el('span', '', label);
  button.append(labelSpan, svg('chevron'));
  const panel = el('div', 'filter-panel facet-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
    if (!panel.hidden) {
      clampFilterPanel(wrap, panel);
    }
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
// that it surfaces some other way - the Directory page and every list page
// pull Role out into chips and Grade/Classroom/Tags into their own dropdowns
// (see roleChips and facetDropdown below), while the standalone Map page is
// the last one still using the full panel.
function filterControl(rerender, options = {}) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button');
  button.append(svg('filter'), el('span', '', 'Filter'), svg('chevron'));
  const panel = el('div', 'filter-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
    if (!panel.hidden) {
      clampFilterPanel(wrap, panel);
    }
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
  if (options.pronouns !== false) {
    sections.push({label: 'Pronouns', values: pronounOptions(), set: state.filterPronouns});
  }
  if (options.tags !== false && tagNames().length) {
    sections.push({label: 'Tags', values: tagNames(), set: state.filterTags});
  }
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
    if (options.pronouns !== false) {
      state.filterPronouns.clear();
    }
    if (options.tags !== false) {
      state.filterTags.clear();
    }
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
      return [['Gradebands', classroomsBackOf(from)], [grade.name, back]];
    }
  }
  if (seg[0] === 'classrooms' && seg[1]) {
    const classroom = state.model.classrooms.find(c => slugify(c.name) === seg[1]);
    if (classroom) {
      return [['Gradebands', classroomsBackOf(from)], [classroom.name, back]];
    }
  }
  if (seg[0] === 'people' && !seg[1]) {
    const tag = new URLSearchParams(from.search).get('tag');
    return [[tag || 'People', back]];
  }
  if (seg[0] === 'classrooms') {
    return [['Gradebands', back]];
  }
  if (seg[0] === 'staff') {
    return [['Staff', back]];
  }
  if (seg[0] === 'email-list') {
    return [['Everyone', back]];
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
    const tagWrap = tagControl(tagEmail, 'tag-wrap', 'tag-button', renderTagList);
    tagWrap.querySelector('.tag-button').title = 'Tags (Shift+T)';
    tagArea.append(tagList, tagWrap);
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

// "She/Her" -> "she / her" - forced lowercase regardless of how the source
// data is cased (older records predate saving pronouns lowercase), spaced out
// around the slash for readability in running text.
function formatPronouns(pronouns) {
  return pronouns.toLowerCase().split('/').join(' / ');
}

function pronouncePill(url, name) {
  const btn = el('button', 'pronounce-pill');
  btn.type = 'button';
  btn.title = 'Hear how to pronounce ' + name;
  btn.append(svg('volume'), el('span', '', name));
  btn.addEventListener('click', () => new Audio(url).play());
  return btn;
}

function copyButton(text, label = 'Copy') {
  const btn = iconButton('copy', label, () => {
    navigator.clipboard.writeText(text);
    btn.classList.add('copied');
    btn.title = 'Copied';
    btn.replaceChildren(svg('check'), el('span', '', 'Copied'));
    setTimeout(() => {
      btn.classList.remove('copied');
      btn.title = label;
      btn.replaceChildren(svg('copy'), el('span', '', label));
    }, 1200);
  });
  return btn;
}

function personSummaryText(p, family) {
  const lines = [p.fullName];
  lines.push(p.pronouns ? `${baseRole(p)} · ${formatPronouns(p.pronouns)}` : baseRole(p));
  lines.push('');
  if (p.phone) {
    lines.push('Phone: ' + p.phone);
  }
  if (!p.emailMasked) {
    lines.push('Email: ' + p.email);
  }
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

// fieldEditor is the shared text-field editor behind every simple Overrides
// field (preferred name, phone, address, pronouns...): an input, a Save/Cancel
// pair, and (when opts.allowHide and there's a current value) a Hide button
// that clears it. opts.presets, if given ([{label, value}]), adds a row of
// quick-pick buttons above the input - each just fills it in rather than
// submitting immediately, so picking one still goes through the same explicit
// Save as typing a custom value, and both are always available side by side.
function fieldEditor(anchor, pencil, opts) {
  const box = el('div', 'field-editor');
  const input = el('input');
  input.type = 'text';
  input.value = opts.current || '';
  if (opts.presets) {
    const presets = el('div', 'field-presets');
    for (const preset of opts.presets) {
      const btn = el('button', 'field-preset', preset.label);
      btn.type = 'button';
      btn.addEventListener('click', () => {
        input.value = preset.value;
        input.focus();
      });
      presets.append(btn);
    }
    box.append(presets);
  }
  box.append(input);
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
  box.append(note, buttons, status);
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
  if (p.phone) {
    const actions = el('div', 'fcard-actions');
    actions.append(el('span', 'fcard-phone', p.phone));
    for (const [name, label, scheme] of [['message', 'Text', 'sms:'], ['phone', 'Call', 'tel:']]) {
      actions.append(iconButton(name, label, e => {
        e.preventDefault();
        e.stopPropagation();
        location.href = scheme + p.phone;
      }));
    }
    row.append(actions);
  }
  const chev = el('div', 'fcard-chevron');
  chev.append(svg('chevron-right'));
  row.append(chev);
  return row;
}

function familyCard(p, family) {
  const card = el('div', 'detail-card fcard');
  card.append(el('h2', 'fcard-title', family.name));

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
    img.addEventListener('click', () => openPhotoLightbox(family.photoUrl));
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
      right.append(familyCardRow(adult, roleWithPronouns(adult)));
    }
  }
  const seeChip = el('a', 'fcard-see-chip');
  seeChip.href = familyLink(family.key);
  const shortName = (family.name || '').replace(/ Family$/, '');
  seeChip.append(el('span', '', `See ${shortName} Family`), svg('chevron-right'));
  right.append(seeChip);
  grid.append(right);
  card.append(grid);
  return card;
}

let personEdit = null;
let familyEdit = null;

// The hero photo is always a square crop (same treatment as every other avatar in
// the app), which can crop a photo awkwardly - this is the escape hatch: click it
// to see the whole, uncropped image in an overlay. Closes on click-anywhere or Esc.
function openPhotoLightbox(url) {
  const overlay = el('div', 'photo-lightbox');
  const img = el('img');
  img.src = url;
  img.alt = '';
  overlay.append(img);
  const close = () => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  };
  function onKey(e) {
    if (e.key === 'Escape') {
      close();
    }
  }
  overlay.addEventListener('click', close);
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
}

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
  // Held rather than appended immediately - on mobile the crumb trail itself is
  // hidden (see .crumbs), leaving just the tag button, which reads better below
  // the alert cards and right above the profile than sandwiched between them.
  const crumbsRow = breadcrumbs([...origin, [p.fullName, null]], p.email);

  const editable = canEditPerson(p.email);
  const editing = editable && personEdit === p.email;
  // The "update for the new year" nag (dashed outline + reminder text) is student-only,
  // same as elsewhere. The camera icon is now only for bootstrapping someone with no
  // photo at all - once they have at least one, the photo grid below (with its own
  // "+" tile) is the only way to add more, so there's exactly one way to do it.
  const nagPhoto = editable && photoNeedsUpdate(p);
  const showPhotoEdit = editable && (p.photos || []).length === 0;
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
  main.append(crumbsRow);

  const content = el('div', 'container detail-content');
  const headerCard = el('div', 'detail-card');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  const wrap = el('div', 'photo-wrap' + (nagPhoto ? ' needs-update' : ''));
  // Shared between the hero's own photo menu and photoGrid below, so an action
  // from either place reports success/error in the same spot.
  const status = el('div', 'media-status');
  // Set by photoGrid below, if rendered, so tapping a grid tile to preview a
  // different photo in the hero also redirects the hero's own menu to act on
  // that photo instead of always the primary one - see the comment on
  // photoMenu's getPhoto param.
  let onHeroPreview = null;
  const families = familiesOf(p);
  const family = families[0];
  if (p.photoUrl) {
    // Person.Photos is `omitempty` in the JSON, so p.photos is undefined - not []
    // - whenever nobody has ever uploaded a photo for this person; every other
    // read of it in this file already guards for that except the two below.
    const photos = p.photos || [];
    const img = el('img', 'detail-photo');
    img.src = p.photoUrl;
    img.alt = '';
    wrap.append(img);
    const initialHeroPhoto = photos.length ? photos[0] : {name: '', url: p.photoUrl, originalUrl: p.photoUrl};
    // Reflects whichever photo the hero is currently showing, same as
    // photoMenu's getPhoto - a preview swap (photoGrid's previewPhoto) can put a
    // different photo on screen than the one the page rendered with.
    const updateCropBadge = photo => {
      wrap.querySelector('.photo-crop-badge')?.remove();
      if (photo.url !== photo.originalUrl) {
        wrap.append(cropBadge());
      }
    };
    updateCropBadge(initialHeroPhoto);
    if (editable) {
      let currentHeroPhoto = initialHeroPhoto;
      const menu = photoMenu(p, () => currentHeroPhoto, editing, status);
      img.addEventListener('click', () => togglePhotoMenu(menu));
      wrap.append(menu);
      onHeroPreview = photo => {
        currentHeroPhoto = photo;
        updateCropBadge(photo);
      };
    } else {
      img.addEventListener('click', () => openPhotoLightbox(photos.length ? photos[0].originalUrl : p.photoUrl));
      onHeroPreview = updateCropBadge;
    }
  } else if (family && family.photoUrl) {
    // No uploaded photo of their own: fall back to the family photo rather than a
    // colored-initials placeholder, since that's the more recognizable default for
    // a parent (or a kid) whose own photo hasn't been added yet.
    const img = el('img', 'detail-photo');
    img.src = thumbUrl(family.photoUrl);
    img.alt = '';
    img.addEventListener('click', () => openPhotoLightbox(family.photoUrl));
    wrap.append(img);
  } else {
    // No uploaded photo, own or family's: fall back to the same colored-initials
    // shape the directory grid uses instead of an empty gray box, so a profile
    // never looks broken.
    wrap.append(photoOrInitials(null, p.fullName, 'detail-photo detail-photo-empty'));
  }
  left.append(wrap);
  if (showPhotoEdit) {
    if (nagPhoto) {
      status.textContent = `Add ${self ? 'your' : `${firstName(p.fullName)}'s`} photo for the new year`;
    }
    wrap.append(uploadIcon('camera', 'Upload photo', 'image/*', 'person', p.email, 'photo', status));
  } else if (nagPhoto) {
    left.append(el('div', 'media-status', `Update ${self ? 'your' : `${firstName(p.fullName)}'s`} photo for the new year`));
  }
  if ((p.photos || []).length >= 1) {
    left.append(photoGrid(p, editable, editing, wrap.querySelector('.detail-photo'), status, onHeroPreview));
  }
  if (editable) {
    left.append(status);
  }
  grid.append(left);

  const right = el('div');
  const topRow = el('div', 'detail-top');
  const roleRow = el('div', 'role-label', baseRole(p));
  topRow.append(roleRow);
  const topRight = el('div', 'detail-top-right');
  if (p.isStaff || p.isStudent) {
    if (p.classroom) {
      const classroomChip = el('a', 'tag-chip', p.classroom);
      classroomChip.href = withFrom('/classrooms/' + slugify(p.classroom));
      topRight.append(classroomChip);
    }
    if (p.crew) {
      const crewChip = el('a', 'tag-chip', p.crew);
      crewChip.href = withFrom('/classrooms/' + slugify(p.classroom));
      topRight.append(crewChip);
    }
  }
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
    topActions.append(copyButton(personSummaryText(p, family), 'Copy all info'));
    topRight.append(topActions);
  }
  if (topRight.children.length) {
    topRow.append(topRight);
  }
  right.append(topRow);
  const nameHeader = el('h1', 'detail-name');
  nameHeader.append(el('span', '', p.fullName));
  if (p.pronunciationUrl && !editing) {
    nameHeader.append(pronouncePill(p.pronunciationUrl, firstName(p.fullName)));
  }
  if (editing) {
    const pencil = editPencil('Edit preferred name');
    nameHeader.append(pencil);
    pencil.addEventListener('click', () => fieldEditor(nameHeader, pencil, {
      current: p.preferredName || '',
      submit: (value, status) => submitField(p.email, 'preferred-name', value, status),
    }));
  }
  if (p.pronouns || editing) {
    const pronounSpan = el('span', 'detail-pronouns', p.pronouns ? p.pronouns.toLowerCase().split('/').join(' / ') : (editing ? 'pronouns' : ''));
    nameHeader.append(pronounSpan);
    if (editing) {
      const pronounPencil = editPencil('Edit pronouns');
      nameHeader.append(pronounPencil);
      pronounPencil.addEventListener('click', () => fieldEditor(pronounSpan, pronounPencil, {
        current: p.pronouns || '',
        allowHide: true,
        presets: [
          {label: 'she/her', value: 'she/her'},
          {label: 'he/him', value: 'he/him'},
          {label: 'they/them', value: 'they/them'},
        ],
        submit: (value, status) => submitField(p.email, 'pronouns', value, status),
      }));
    }
  }
  right.append(nameHeader);
  const nickname = displayNameLine(p);
  if (nickname) {
    right.append(el('div', 'detail-sub', nickname));
  }
  if (p.isStudent) {
    const chain = gradeChain(p);
    if (chain) {
      right.append(el('div', 'detail-sub', chain));
    }
  } else if (p.isStaff && p.jobTitle) {
    right.append(el('div', 'detail-sub', p.jobTitle));
  }
  if (!p.emailMasked) {
    const emailValue = el('div', 'contact-value');
    emailValue.append(svg('mail'), el('span', '', p.email));
    right.append(contactRow(emailValue, [
      iconButton('mail', 'Email', 'mailto:' + p.email),
      copyButton(p.email),
    ]));
  }
  if (p.phone) {
    const phoneValue = el('div', 'contact-value');
    phoneValue.append(svg('phone'), el('span', '', p.phone));
    right.append(contactRow(phoneValue, [
      iconButton('message', 'Text', 'sms:' + p.phone),
      iconButton('phone', 'Call', 'tel:' + p.phone),
      copyButton(p.phone),
    ]));
  }
  if (family && family.address) {
    const addressValue = el('div', 'contact-value');
    addressValue.append(svg('map'), el('span', '', family.address));
    right.append(contactRow(addressValue, [
      iconButton('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
      copyButton(family.address),
    ]));
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
    right.append(pronounceEditor('person', p.email, !!p.hasOwnPronunciation));
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
    const text = p.facts ? aboutMeText(textClass, p.facts) : el('div', textClass, placeholder);
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

  main.append(content);

  // One identical card per family - a kid in two households gets both side by
  // side (stacking on narrow screens) rather than one buried below the other,
  // so it reads as "these are the two households" instead of a repeat.
  if (families.length) {
    const wrap = el('div', 'container fcard-wrap' + (families.length > 1 ? ' fcard-wrap-multi' : ''));
    const row = el('div', 'fcard-columns');
    for (const f of families) {
      row.append(familyCard(p, f));
    }
    wrap.append(row);
    main.append(wrap);
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
  const editing = editable && familyEdit === key;
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
    const img = el('img', 'detail-photo');
    img.src = family.photoUrl;
    img.alt = '';
    img.addEventListener('click', () => openPhotoLightbox(img.src));
    wrap.append(img);
  } else {
    wrap.append(photoOrInitials(null, family.name, 'detail-photo detail-photo-empty'));
  }
  left.append(wrap);
  if (editing) {
    const status = el('div', 'media-status');
    wrap.append(uploadIcon('camera', 'Upload family photo', 'image/*', 'family', key, 'photo', status));
    left.append(status);
  }
  if (family.photoCaption || editing) {
    const captionRow = el('div', 'family-caption', family.photoCaption || 'Add a caption');
    left.append(captionRow);
    if (editing) {
      const captionPencil = editPencil('Edit photo caption');
      captionRow.append(captionPencil);
      captionPencil.addEventListener('click', () => fieldEditor(captionRow, captionPencil, {
        current: family.photoCaption || '',
        submit: (value, status) => submitField(key, 'family-photo-caption', value, status),
      }));
    }
  }
  grid.append(left);

  const right = el('div');
  const kids = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
  const adults = (family.adultEmails || []).map(e => byEmail[e]).filter(Boolean);
  const grades = [...new Set(kids.map(k => k.grade).filter(Boolean))];
  const topRow = el('div', 'detail-top');
  topRow.append(el('div', 'role-label', grades.length ? grades.join(', ') : 'Staff'));
  if (editable) {
    const topActions = el('div', 'detail-top-actions');
    const toggle = editing
      ? el('button', 'media-button edit-toggle', 'Done')
      : iconButton('pencil', 'Edit info', () => {
        familyEdit = key;
        renderFamilyDetail(key);
        finishRender();
      });
    if (editing) {
      toggle.addEventListener('click', () => {
        familyEdit = null;
        renderFamilyDetail(key);
        finishRender();
      });
    }
    topActions.append(toggle);
    topRow.append(topActions);
  }
  right.append(topRow);
  const nameHeader = el('h1', 'detail-name');
  nameHeader.append(el('span', '', family.name));
  if (family.pronunciationUrl) {
    nameHeader.append(pronouncePill(family.pronunciationUrl, shortName));
  }
  right.append(nameHeader);
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
  if (editing) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    if (family.pronunciationUrl) {
      const audio = el('audio', 'pronounce-player');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = family.pronunciationUrl;
      right.append(audio);
    }
    right.append(pronounceEditor('family', key, !!family.pronunciationUrl));
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
      adultsCol.append(familyCardRow(a, roleWithPronouns(a)));
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

const LIST_SUB_TRUNCATE_LENGTH = 280;
const LIST_SUB_LINE_PREVIEW = 4;
const BULLET_LINE = /^\s*(?:[*]|-{1,2})\s+(.+)$/;

// The profile page's own "About Me" card: plain text (manual line breaks preserved
// via the .about-text CSS) unless every line is bullet-marked (see parseBullets
// below), in which case it's a real bulleted list instead of showing the literal
// *, -, or -- markers as text.
function aboutMeText(className, facts) {
  const lines = facts.split(/\r?\n/).map(l => l.trim()).filter(Boolean);
  const bullets = lines.length > 1 ? parseBullets(lines) : null;
  if (!bullets) {
    return el('div', className, facts);
  }
  const list = el('ul', className + ' about-bullets');
  for (const item of bullets) {
    list.append(el('li', '', item));
  }
  return list;
}

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

// A row's "About Me" (or similar) text. A bullet list is capped by item count
// (LIST_SUB_LINE_PREVIEW) since each item is a discrete thing to show or hide;
// anything else - a single paragraph or a few manual line breaks alike - is
// capped by total character count instead, so a short bio that merely happens
// to have a couple of line breaks isn't truncated any more eagerly than an
// equally-short single-line one, and a long one is capped regardless of how
// many line breaks it does or doesn't have. Either way, stopPropagation on the
// toggle keeps that click from also triggering the surrounding card's own
// navigation link.
function listSub(text) {
  const wrap = el('div', 'list-sub');
  const lines = text.split(/\r?\n/).map(l => l.trim()).filter(Boolean);
  const bullets = lines.length > 1 ? parseBullets(lines) : null;
  if (bullets) {
    wrap.append(collapsibleLines(bullets, 'ul', 'list-sub-bullets', 'li'));
    return wrap;
  }
  const full = lines.join('\n');
  if (full.length <= LIST_SUB_TRUNCATE_LENGTH) {
    wrap.textContent = full;
    return wrap;
  }
  let short = full.slice(0, LIST_SUB_TRUNCATE_LENGTH);
  const cutAt = Math.max(short.lastIndexOf(' '), short.lastIndexOf('\n'));
  short = cutAt > 0 ? short.slice(0, cutAt) : short;
  const textSpan = el('span', '', short + '… ');
  const toggle = el('button', 'list-sub-more', 'More »');
  toggle.type = 'button';
  let expanded = false;
  toggle.addEventListener('click', e => {
    e.preventDefault();
    e.stopPropagation();
    expanded = !expanded;
    textSpan.textContent = expanded ? full + ' ' : short + '… ';
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
  {key: 'by-classroom', label: 'Classrooms', heading: 'Classrooms'},
  {key: 'by-grade', label: 'Grades', heading: 'Grades'},
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
    const grid = el('div', 'people-grid autofit classroom-grid');
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
    const grid = el('div', 'people-grid autofit classroom-grid');
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

// abbreviateGrade turns a grade's full name ("Kindergarten", "Grade 4") into
// the short form used in a parenthetical like "Harrison (Ravens - 4)".
function abbreviateGrade(name) {
  if (name === 'Kindergarten') {
    return 'K';
  }
  const match = /^Grade (\d+)$/.exec(name || '');
  return match ? match[1] : name;
}

function kidsSummary(parent) {
  const family = familyOf(parent);
  if (!family) {
    return '';
  }
  return (family.kidEmails || [])
    .map(e => byEmail[e])
    .filter(Boolean)
    .map(k => `${firstName(k.fullName)} (${[k.classroom, abbreviateGrade(k.grade)].filter(Boolean).join(' - ')})`)
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
    const grid = el('div', 'people-grid autofit classroom-grid');
    for (const p of parents) {
      const card = el('a', 'person-card');
      card.href = personLink(p);
      card.append(photoWithTag(applyRingColor(photoOrInitials(p.photoUrl, p.fullName, 'person-photo'), p), p.email));
      card.append(el('div', 'person-name', p.fullName));
      card.append(el('div', 'person-sub', kidsSummary(p)));
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
  pageHeader.append(el('h1', 'page-title', 'Gradebands'));
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
  controls.append(departmentChips(renderList));
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
    for (const family of familiesOf(s)) {
      for (const email of family.adultEmails || []) {
        if (!seen.has(email) && byEmail[email]) {
          seen.add(email);
          parents.push(byEmail[email]);
        }
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
  const seen = new Set();
  const names = [];
  for (const family of familiesOf(student)) {
    for (const email of [...(family.kidEmails || []), ...(family.adultEmails || [])]) {
      if (email === student.email || seen.has(email) || !byEmail[email]) {
        continue;
      }
      seen.add(email);
      names.push(byEmail[email].fullName);
    }
  }
  return names.join(', ');
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
  header.append(breadcrumbs([['Gradebands', back], [title, null]]));

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
    const headingRow = el('div', 'roster-heading-row');
    headingRow.append(el('h2', 'roster-heading', `${parents.length} Parents`));
    const parentGroups = groups.map(g => ({header: g.header, chipLabel: g.chipLabel, parents: parentsOf(g.students)}));
    const filterBar = sectionFilterBar(parentGroups, rerender);
    if (filterBar) {
      headingRow.append(filterBar);
    }
    list.append(headingRow);
    const visibleGroups = parentGroups.filter(g => !g.header || !state.rosterSectionExcluded.has(g.chipLabel || g.header));
    const listBody = el('div', 'roster-list grid');
    for (const group of visibleGroups) {
      if (group.header) {
        listBody.append(el('h2', 'group-header', group.header));
      }
      for (const p of sortPeople(group.parents)) {
        listBody.append(listRow(thumbUrl(p.photoUrl), otherFamilyMembers(p).toUpperCase(), p.fullName, p.facts || '', personLink(p)));
      }
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
  const family = familyOf(parent);
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
    // A masked email is a Veracross placeholder nobody can actually reach - it has no
    // place in a mailing list, so this person is left out of it entirely rather than
    // appearing with a blank or fake address.
    if (p.emailMasked || seen.has(p.email)) {
      return;
    }
    seen.add(p.email);
    rows.push({p, role, grade, classroom});
  };
  const students = state.model.people
    .filter(p => p.isStudent)
    .sort((a, b) => a.fullName.localeCompare(b.fullName));
  for (const s of students) {
    add(s, 'Student', s.grade || '', s.classroom || '');
    for (const family of familiesOf(s)) {
      for (const email of family.adultEmails || []) {
        const parent = byEmail[email];
        if (parent) {
          add(parent, 'Parent', kidsField(parent, 'grade'), kidsField(parent, 'classroom'));
        }
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
  const family = familyOf(me);
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

function familySearchText(family) {
  const members = [...(family.kidEmails || []), ...(family.adultEmails || [])]
    .map(e => byEmail[e]).filter(Boolean).map(p => p.fullName);
  return `${family.name || ''} ${members.join(' ')}`.toLowerCase();
}

function familyMapPopup(family) {
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

// Shared by the standalone Map page and a tag list's own Map view: builds the
// map into canvas, showing only families for which familyMatches(family) is
// true, and returns a renderPins() to call again after a filter/search change
// without recreating the map itself.
function initFamilyMap(canvas, familyMatches) {
  let map = null;
  let info = null;
  let markers = [];
  const missing = el('div', 'map-missing');
  canvas.after(missing);

  function renderPins() {
    if (!map) {
      return;
    }
    for (const m of markers) {
      m.setMap(null);
    }
    markers = [];
    const allPeople = new Map();
    const withoutAddress = new Map();
    for (const family of Object.values(state.model.families)) {
      if (!familyMatches(family)) {
        continue;
      }
      const emails = [...(family.kidEmails || []), ...(family.adultEmails || [])];
      const matchingMembers = emails.map(e => byEmail[e]).filter(p => p && matchesFilters(p));
      for (const p of matchingMembers) {
        allPeople.set(p.email, p);
      }
      if (!family.address || family.veracrossAddress === 'partial') {
        for (const p of matchingMembers) {
          withoutAddress.set(p.email, p);
        }
        continue;
      }
      if (!family.lat && !family.lng) {
        continue;
      }
      const marker = new google.maps.Marker({
        map,
        position: {lat: family.lat, lng: family.lng},
        icon: {url: pinIcon, anchor: new google.maps.Point(17, 33)},
        title: family.name,
      });
      marker.addListener('click', () => {
        info.setContent(familyMapPopup(family));
        info.open(map, marker);
      });
      markers.push(marker);
    }
    missing.replaceChildren();
    if (withoutAddress.size > 10) {
      missing.textContent = `${withoutAddress.size} of ${allPeople.size} people not shown — no street address on file`;
    } else if (withoutAddress.size) {
      const names = [...withoutAddress.values()].map(p => p.fullName).sort((a, b) => a.localeCompare(b));
      missing.textContent = `Not shown, no street address on file: ${names.join(', ')}`;
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
      if ((family.lat || family.lng) && family.veracrossAddress !== 'partial' && familyMatches(family)) {
        bounds.extend({lat: family.lat, lng: family.lng});
      }
    }
    map.fitBounds(bounds);
    renderPins();
  });

  return () => renderPins();
}

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
  let renderPins = () => {};
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

  renderPins = initFamilyMap(canvas, family => familyMatchesFilters(family.key) && familySearchText(family).includes(state.q));
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

// submitPhotoOrder posts a person's complete photo order to the reorder-photos
// endpoint - reordering (drag), deleting (order with one name missing), and
// setting a photo primary (order with that name moved to the front) are all just
// this same request, so drag-reorder, delete, and the photo menu's "Set as
// primary" all funnel through it instead of three separate copies of this fetch.
// onError, if given, runs only on failure - drag-reorder uses it to snap the tiles
// back to where they were; delete and "Set as primary" have no DOM order to revert.
async function submitPhotoOrder(key, order, status, onError) {
  status.classList.remove('error');
  const res = await fetch('/api/directory/reorder-photos', {
    method: 'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body: new URLSearchParams({key, order: order.join(',')}),
  });
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    if (onError) {
      onError();
    }
    return false;
  }
  await load();
  return true;
}

// submitCrop posts a cropped square as the crop for one of a person's photos.
async function submitCrop(key, name, blob, status) {
  status.classList.remove('error');
  status.textContent = 'Saving crop…';
  const form = new FormData();
  form.append('key', key);
  form.append('name', name);
  form.append('file', blob, 'crop.jpg');
  const res = await fetch('/api/directory/crop-photo', {method: 'POST', body: form});
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return false;
  }
  await load();
  return true;
}

function canEditPerson(email) {
  const meEmail = document.body.dataset.userEmail;
  if (email === meEmail || state.model.superEdit) {
    return true;
  }
  return familiesOf(byEmail[meEmail]).some(family =>
    [...(family.kidEmails || []), ...(family.adultEmails || [])].includes(email));
}

// togglePhotoMenu opens/closes a photoMenu. Its CSS anchors with `right: 0`,
// which keeps it on screen for a trigger near the right edge (e.g. the "more"
// menu, always top-right) but a photo tile can sit anywhere across a grid - on a
// narrow/mobile viewport a tile nearer the left edge pushes the menu's left side
// past x=0. After opening, nudge it back on screen with a transform once we can
// measure where the pure-CSS position actually landed.
function togglePhotoMenu(menu) {
  menu.hidden = !menu.hidden;
  if (menu.hidden) {
    return;
  }
  menu.rebuild();
  menu.style.transform = '';
  const rect = menu.getBoundingClientRect();
  const margin = 8;
  let shift = 0;
  if (rect.left < margin) {
    shift = margin - rect.left;
  } else if (rect.right > window.innerWidth - margin) {
    shift = window.innerWidth - margin - rect.right;
  }
  if (shift) {
    menu.style.transform = `translateX(${shift}px)`;
  }
}

// cropBadge marks the hero photo as showing a manually cropped square rather
// than the plain auto-crop of the original - grid tiles are small enough that
// it would just add clutter, so this is hero-only. A photo's url differs from
// its originalUrl exactly when a crop is currently applied and resolves (see
// attachBlobs, internal/directory/load.go) - that's the signal used here rather
// than a separate field, since it's already exactly what "has a crop" means.
function cropBadge() {
  const badge = el('div', 'photo-crop-badge');
  badge.title = 'Manually cropped';
  badge.append(svg('zoom-in'));
  return badge;
}

// photoMenu builds the Facebook-style "View photo / Set as primary / Delete
// photo / Crop photo" popup for whichever photo the hero is currently showing.
// Hidden by default; the caller wires a trigger to call togglePhotoMenu(menu),
// which rebuilds the item list fresh on every open (via menu.rebuild, set here)
// and appends the menu inside that trigger's own position:relative container.
// Reuses the app-wide outside-click/Escape closer (below, alongside
// .more-menu/.card-menu) for free - no bespoke close handling here.
//
// getPhoto() returns the photo currently shown in the hero, which isn't always
// p.photos[0]: tapping a grid tile (photoGrid's previewPhoto) swaps the hero's
// preview without touching this menu, so the menu has to ask fresh each time it
// opens rather than close over one photo at build time - otherwise it would
// keep acting on the primary photo even while a different one is on screen.
// "Set as primary" is hidden when the current photo already is; a grid tile's
// own click never opens a menu at all (see photoGrid) - drag it to the front,
// or preview it and use this menu, both end up here. Delete only shows in
// editing mode, same convention as the grid tiles' own delete "x" -
// destructive actions wait for edit mode.
function photoMenu(p, getPhoto, editing, status) {
  const menu = el('div', 'photo-menu');
  menu.hidden = true;
  menu.rebuild = () => {
    menu.replaceChildren();
    const photo = getPhoto();
    const photos = p.photos || [];
    const isPrimary = !photos.length || photo.name === photos[0].name;
    const item = (iconName, label, action) => {
      const btn = el('button', 'photo-menu-item');
      btn.type = 'button';
      btn.append(svg(iconName), el('span', '', label));
      btn.addEventListener('click', e => {
        e.stopPropagation();
        menu.hidden = true;
        action();
      });
      menu.append(btn);
    };
    item('eye', 'View photo', () => openPhotoLightbox(photo.originalUrl));
    if (!isPrimary) {
      item('star', 'Set as primary', () => {
        const order = [photo.name, ...photos.map(ph => ph.name).filter(n => n !== photo.name)];
        submitPhotoOrder(p.email, order, status);
      });
    }
    if (editing) {
      item('trash', 'Delete photo', () => {
        // The school portrait is harder to get back than a self-uploaded photo, so
        // it gets a stronger warning, but either way this is permanent - confirm first.
        const message = photo.source === 'veracross'
          ? 'This is the school portrait from Veracross. Remove it anyway?'
          : 'Remove this photo?';
        if (!confirm(message)) {
          return;
        }
        status.textContent = 'Removing…';
        const order = photos.map(ph => ph.name).filter(name => name !== photo.name);
        submitPhotoOrder(p.email, order, status);
      });
    }
    item('crop', 'Crop photo', () => openCropTool(p, photo, status));
  };
  menu.rebuild();
  return menu;
}

// openCropTool is a full-screen square-crop editor for one of a person's photos.
// There's no stored crop rectangle to restore - only the resulting cropped image
// is saved - so cropping a photo that already has a crop just starts fresh from
// the original and replaces it.
function openCropTool(p, photo, status) {
  const overlay = el('div', 'crop-overlay');
  const panel = el('div', 'crop-panel');
  const stage = el('div', 'crop-stage');
  const img = el('img', 'crop-image');
  img.src = photo.originalUrl;
  img.alt = '';
  const frame = el('div', 'crop-frame');
  const handleEls = ['nw', 'ne', 'sw', 'se'].map(corner => {
    const handle = el('div', 'crop-handle crop-handle-' + corner);
    handle.dataset.corner = corner;
    return handle;
  });
  frame.append(...handleEls);
  const maskTop = el('div', 'crop-mask');
  const maskBottom = el('div', 'crop-mask');
  const maskLeft = el('div', 'crop-mask');
  const maskRight = el('div', 'crop-mask');
  stage.append(img, maskTop, maskLeft, maskRight, maskBottom, frame);
  const actions = el('div', 'crop-actions');
  const cancel = el('button', 'media-button', 'Cancel');
  cancel.type = 'button';
  const save = el('button', 'media-button primary', 'Save crop');
  save.type = 'button';
  actions.append(cancel, save);
  panel.append(stage, actions);
  overlay.append(panel);

  const close = () => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  };
  function onKey(e) {
    if (e.key === 'Escape') {
      close();
    }
  }
  overlay.addEventListener('click', e => {
    // Only the dark backdrop closes on click - not the panel, stage, or frame,
    // which all live inside it and need their own clicks/drags to work.
    if (e.target === overlay) {
      close();
    }
  });
  cancel.addEventListener('click', close);
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);

  const minSide = 60;
  let left = 0;
  let top = 0;
  let side = 0;

  function render() {
    frame.style.left = left + 'px';
    frame.style.top = top + 'px';
    frame.style.width = side + 'px';
    frame.style.height = side + 'px';
    maskTop.style.cssText = `top:0; left:0; right:0; height:${top}px`;
    maskBottom.style.cssText = `top:${top + side}px; left:0; right:0; bottom:0`;
    maskLeft.style.cssText = `top:${top}px; left:0; width:${left}px; height:${side}px`;
    maskRight.style.cssText = `top:${top}px; left:${left + side}px; right:0; height:${side}px`;
  }

  function setFrame(nextLeft, nextTop, nextSide) {
    const stageRect = stage.getBoundingClientRect();
    side = Math.max(minSide, Math.min(nextSide, stageRect.width, stageRect.height));
    left = Math.max(0, Math.min(nextLeft, stageRect.width - side));
    top = Math.max(0, Math.min(nextTop, stageRect.height - side));
    render();
  }

  function init() {
    const stageRect = stage.getBoundingClientRect();
    const initialSide = Math.min(stageRect.width, stageRect.height);
    setFrame((stageRect.width - initialSide) / 2, (stageRect.height - initialSide) / 2, initialSide);
  }
  if (img.complete && img.naturalWidth) {
    init();
  } else {
    img.addEventListener('load', init);
  }

  // Shared pointer-drag wiring for both moving the frame and resizing it from a
  // corner handle - same pointerdown/pointermove/pointerup(+capture) pattern
  // photoGrid's own drag-to-reorder already uses, for mobile/touch reliability.
  function drag(target, onMove) {
    target.addEventListener('pointerdown', e => {
      e.preventDefault();
      e.stopPropagation();
      const pointerId = e.pointerId;
      const startX = e.clientX;
      const startY = e.clientY;
      const startLeft = left;
      const startTop = top;
      const startSide = side;
      target.setPointerCapture(pointerId);
      const move = m => {
        if (m.pointerId !== pointerId) {
          return;
        }
        onMove(m.clientX - startX, m.clientY - startY, startLeft, startTop, startSide);
      };
      const up = u => {
        if (u.pointerId !== pointerId) {
          return;
        }
        target.removeEventListener('pointermove', move);
        target.removeEventListener('pointerup', up);
        target.removeEventListener('pointercancel', up);
      };
      target.addEventListener('pointermove', move);
      target.addEventListener('pointerup', up);
      target.addEventListener('pointercancel', up);
    });
  }

  drag(frame, (dx, dy, startLeft, startTop, startSide) => {
    setFrame(startLeft + dx, startTop + dy, startSide);
  });
  for (const handle of handleEls) {
    const corner = handle.dataset.corner;
    drag(handle, (dx, dy, startLeft, startTop, startSide) => {
      let delta;
      let nextLeft = startLeft;
      let nextTop = startTop;
      if (corner === 'se') {
        delta = Math.max(dx, dy);
      } else if (corner === 'nw') {
        delta = Math.max(-dx, -dy);
        nextLeft = startLeft - delta;
        nextTop = startTop - delta;
      } else if (corner === 'ne') {
        delta = Math.max(dx, -dy);
        nextTop = startTop - delta;
      } else {
        delta = Math.max(-dx, dy);
        nextLeft = startLeft - delta;
      }
      setFrame(nextLeft, nextTop, startSide + delta);
    });
  }

  save.addEventListener('click', () => {
    const stageRect = stage.getBoundingClientRect();
    const scale = img.naturalWidth / stageRect.width;
    const sx = left * scale;
    const sy = top * scale;
    const cropSide = side * scale;
    const outSize = Math.min(Math.round(cropSide), 1200);
    const canvas = document.createElement('canvas');
    canvas.width = outSize;
    canvas.height = outSize;
    canvas.getContext('2d').drawImage(img, sx, sy, cropSide, cropSide, 0, 0, outSize, outSize);
    save.disabled = true;
    save.textContent = 'Saving…';
    canvas.toBlob(async blob => {
      if (!blob) {
        save.disabled = false;
        save.textContent = 'Save crop';
        return;
      }
      const ok = await submitCrop(p.email, photo.name, blob, status);
      if (ok) {
        close();
      } else {
        save.disabled = false;
        save.textContent = 'Save crop';
      }
    }, 'image/jpeg', 0.92);
  });
}

// photoGrid shows up to 5 photo tiles (already primary-first - the server always
// returns them in the order that's shown everywhere). Anyone who may edit the
// record can always drag a tile to reorder it, or add one while under the cap -
// no separate edit mode needed for either. The delete "x" on each tile is the one
// control that waits for edit mode, same convention as every other field on this
// page, since it's destructive. Clicking (rather than dragging) a tile - for
// anyone, editable or not - just shows that photo bigger in the hero image above
// (onPreview, if given, also redirects the hero's own View/Set as primary/Delete/
// Crop menu to that photo - see photoMenu's getPhoto param); it changes nothing
// until a drag actually reorders something, or an action is chosen from that
// menu. Tiles don't get a menu of their own - no room to open one well on a
// narrow screen - so cropping or deleting a non-primary photo means previewing
// or dragging it to the front first, either of which puts it in reach of the
// hero's menu. Reordering is pointer-events based rather than native HTML5
// drag-and-drop, since the latter doesn't work reliably on mobile/touch (iOS
// Safari in particular) and this app is used heavily on phones. status is shared
// with the hero photo's own menu so both report errors in the same place.
function photoGrid(p, editable, editing, heroImg, status, onPreview) {
  const grid = el('div', 'photo-grid');

  const previewPhoto = photo => {
    if (heroImg) {
      heroImg.src = photo.url;
    }
    if (onPreview) {
      onPreview(photo);
    }
  };

  const currentOrder = () => [...grid.querySelectorAll('.photo-slot')].map(t => t.dataset.name);

  const commitOrder = (revertOrder) => {
    submitPhotoOrder(p.email, currentOrder(), status, () => {
      const addTile = grid.querySelector('.photo-slot-add');
      for (const name of revertOrder) {
        grid.insertBefore(grid.querySelector(`.photo-slot[data-name="${CSS.escape(name)}"]`), addTile);
      }
    });
  };

  const deletePhoto = (photo) => {
    status.textContent = 'Removing…';
    submitPhotoOrder(p.email, currentOrder().filter(name => name !== photo.name), status);
  };

  function wireDrag(tile, photo) {
    let pointerId = null;
    let dragging = false;
    let startOrder = null;
    let startX = 0;
    let startY = 0;
    tile.addEventListener('pointerdown', e => {
      if (e.pointerType === 'mouse' && e.button !== 0) {
        return;
      }
      // Without this, holding and moving over the photo also kicks off the browser's
      // own image-drag ghost and text/image selection highlight, fighting visually
      // with the custom drag below.
      e.preventDefault();
      pointerId = e.pointerId;
      startX = e.clientX;
      startY = e.clientY;
    });
    tile.addEventListener('pointermove', e => {
      if (pointerId !== e.pointerId) {
        return;
      }
      if (!dragging) {
        if (Math.hypot(e.clientX - startX, e.clientY - startY) < 6) {
          return;
        }
        dragging = true;
        startOrder = currentOrder();
        tile.setPointerCapture(pointerId);
        tile.classList.add('dragging');
      }
      const addTile = grid.querySelector('.photo-slot-add');
      for (const sib of grid.querySelectorAll('.photo-slot')) {
        if (sib === tile) {
          continue;
        }
        const rect = sib.getBoundingClientRect();
        const mid = rect.left + rect.width / 2;
        const tileIsBefore = Boolean(sib.compareDocumentPosition(tile) & Node.DOCUMENT_POSITION_PRECEDING);
        if (e.clientX < mid && !tileIsBefore) {
          grid.insertBefore(tile, sib);
          break;
        }
        if (e.clientX > mid && tileIsBefore) {
          grid.insertBefore(tile, sib.nextSibling === addTile ? addTile : sib.nextSibling);
          break;
        }
      }
    });
    const finish = e => {
      if (pointerId !== e.pointerId) {
        return;
      }
      if (dragging) {
        tile.classList.remove('dragging');
        const newOrder = currentOrder();
        if (startOrder && newOrder.join(',') !== startOrder.join(',')) {
          commitOrder(startOrder);
        }
      } else {
        // A tap that never crossed the drag threshold: just preview it bigger.
        previewPhoto(photo);
      }
      pointerId = null;
      dragging = false;
      startOrder = null;
    };
    tile.addEventListener('pointerup', finish);
    tile.addEventListener('pointercancel', () => {
      if (dragging && startOrder) {
        const addTile = grid.querySelector('.photo-slot-add');
        for (const name of startOrder) {
          grid.insertBefore(grid.querySelector(`.photo-slot[data-name="${CSS.escape(name)}"]`), addTile);
        }
        tile.classList.remove('dragging');
      }
      pointerId = null;
      dragging = false;
      startOrder = null;
    });
  }

  function addTile() {
    const tile = el('label', 'photo-slot photo-slot-add');
    tile.title = 'Add photo';
    tile.append(el('span', '', '+'));
    const input = el('input');
    input.type = 'file';
    input.accept = 'image/*';
    input.hidden = true;
    input.addEventListener('change', () => {
      if (input.files.length) {
        submitMedia('person', p.email, 'photo', input.files[0], input.files[0].name, status);
      }
    });
    tile.append(input);
    return tile;
  }

  p.photos.forEach((photo, i) => {
    const tile = el('div', 'photo-slot' + (i === 0 ? ' photo-slot-primary' : ''));
    tile.dataset.name = photo.name;
    tile.title = photo.source === 'veracross' ? 'School portrait' : 'Uploaded photo';
    const face = el('img');
    face.src = thumbUrl(photo.url);
    face.alt = '';
    face.draggable = false;
    tile.append(face);
    if (editing) {
      const del = el('button', 'photo-slot-delete', '×');
      del.type = 'button';
      del.title = 'Remove photo';
      del.addEventListener('pointerdown', e => e.stopPropagation());
      del.addEventListener('click', e => {
        e.stopPropagation();
        // The school portrait is harder to get back than a self-uploaded photo, so
        // it gets a stronger warning, but either way this is permanent - confirm first.
        const message = photo.source === 'veracross'
          ? 'This is the school portrait from Veracross. Remove it anyway?'
          : 'Remove this photo?';
        if (!confirm(message)) {
          return;
        }
        deletePhoto(photo);
      });
      tile.append(del);
    }
    if (editable) {
      tile.classList.add('photo-slot-draggable');
      wireDrag(tile, photo);
    } else {
      // No drag wired up here to distinguish a tap from a drag, so a plain click is
      // always just a preview.
      tile.addEventListener('click', () => previewPhoto(photo));
    }
    grid.append(tile);
  });
  if (editable && p.photos.length < 5) {
    grid.append(addTile());
  }

  const wrap = el('div');
  wrap.append(grid);
  return wrap;
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

function deletePronunciationIcon(target, key, status) {
  const button = el('button', 'edit-icon');
  button.type = 'button';
  button.title = 'Delete pronunciation';
  button.append(svg('trash'));
  button.addEventListener('click', () => submitField(key, target === 'family' ? 'family-pronunciation' : 'pronunciation', '', status));
  return button;
}

function pronounceEditor(target, key, hasPronunciation) {
  const box = el('div', 'pronounce-edit');
  const actions = el('div', 'pronounce-actions');
  const status = el('div', 'media-status');
  const preview = el('div', 'record-preview');
  actions.append(recordIcon(target, key, status, preview));
  actions.append(uploadIcon('upload', 'Upload an audio file', 'audio/*', target, key, 'pronunciation', status));
  if (hasPronunciation) {
    actions.append(deletePronunciationIcon(target, key, status));
  }
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
  classrooms: 'Gradebands',
  'my-family': 'My Family',
  staff: 'Staff',
  map: 'Map',
  'email-list': 'Everyone',
  greenvelope: 'Invites',
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
    const tagParam = new URLSearchParams(location.search).get('tag');
    if (tagParam) {
      state.filterTags = new Set([tagParam]);
      state.filterTagRelations = loadTagRelations(tagParam);
      state.tagListView = 'faces';
      renderListPage();
    } else {
      state.tab = tabParam('everyone');
      renderPeople();
    }
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
    state.filterTags = new Set();
    state.filterTagRelations = new Set();
    state.tagListView = 'emails';
    renderListPage();
  } else if (seg[0] === 'greenvelope') {
    state.q = '';
    state.filterTags = new Set();
    state.filterTagRelations = new Set();
    renderGreenvelopePage();
  } else if (seg[0] === 'map') {
    state.q = '';
    renderMapPage();
  } else if (seg[0] === 'my-privacy') {
    renderPrivacyPage();
  }
  finishRender();
}

// Standalone iOS PWAs can settle 100dvh on a shorter value after an in-page
// route change than they reported on first load, leaving fixed bottom bars
// (mobile-tabs) short of the real screen edge - window.innerHeight matches
// what position:fixed elements are actually anchored to, so mirroring it into
// a custom property (see body/main's height: var(--vh100, 100dvh) in
// style.css) keeps them in sync regardless of dvh's own drift.
//
// In standalone mode specifically, innerHeight itself under-reports: it
// comes in ~60pt short of the true screen (no browser chrome exists there to
// explain the gap), and because body's height ends up as fixed elements'
// containing block, mobile-tabs' bottom:0 then stops short of the real edge
// too, leaving a blank strip below it. There's no dynamic toolbar to track in
// standalone mode, so screen.height - the full, stable device height - is
// the correct source there instead.
function syncViewportHeight() {
  const h = runningStandalone() ? window.screen.height : window.innerHeight;
  document.documentElement.style.setProperty('--vh100', h + 'px');
}
window.addEventListener('resize', syncViewportHeight);
window.addEventListener('orientationchange', syncViewportHeight);

// Wraps everything the render just built so it can act as the flexible
// sticky-footer spacer: on a short page it grows to push the art down to the true
// bottom of the viewport, and on a tall page it just yields to scrolling. Called
// both after the top-level render() dispatch and after any in-page tab switch that
// re-renders by calling its render*() function directly instead of going through
// render() - those bypass this otherwise, leaving the footer art stuck from
// whatever page loaded first (or missing it entirely).
function finishRender() {
  syncViewportHeight();
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

// Mobile's counterpart to the desktop topbar avatar above - same menu markup,
// pinned to the top-right of .mobile-top instead of tucked into the drawer, so
// the account entry point sits in the same corner on every breakpoint.
const mobileUserMenu = document.querySelector('#mobile-user-menu');
document.querySelector('#mobile-user').addEventListener('click', e => {
  e.stopPropagation();
  mobileUserMenu.hidden = !mobileUserMenu.hidden;
});

// The stale-count badge (desktop and mobile both) used to link straight to
// /my-family; now it opens a dropdown built from the same todoChecklist used
// on My Family/a person's own page, so what to fix and the link to fix it are
// both right there without leaving the current page.
for (const staleButton of document.querySelectorAll('.stale-alert')) {
  const wrap = staleButton.closest('.stale-wrap');
  const panel = wrap.querySelector('.stale-menu');
  staleButton.addEventListener('click', e => {
    e.stopPropagation();
    const opening = panel.hidden;
    for (const p of document.querySelectorAll('.stale-menu')) {
      p.hidden = true;
    }
    if (opening) {
      panel.replaceChildren(todoChecklist(staleItems()));
      panel.hidden = false;
      clampFilterPanel(wrap, panel);
    }
  });
}

const drawer = document.querySelector('#drawer');
const drawerOverlay = document.querySelector('#drawer-overlay');

const mobileListsMenu = document.querySelector('#mobile-lists-menu');
const mobileListsOverlay = document.querySelector('#mobile-lists-overlay');

function setMobileListsMenu(open) {
  if (open) {
    renderMobileListsMenu();
  }
  mobileListsMenu.hidden = !open;
  mobileListsOverlay.hidden = !open;
}

mobileListsOverlay.addEventListener('click', () => setMobileListsMenu(false));

// The same Everyone/Invites/tag links as the sidebar's "Lists" section
// (buildNavInto above), just laid out as a mobile bottom sheet instead of a
// nav list, since there's no sidebar to hold them on a phone-sized screen.
function renderMobileListsMenu() {
  const body = mobileListsMenu.querySelector('#mobile-lists-body');
  body.replaceChildren();
  const seg = activeSection();
  const currentTag = new URLSearchParams(location.search).get('tag');
  for (const item of toolsNavItems) {
    const a = el('a', 'mobile-lists-item' + (item.path === seg ? ' active' : ''));
    a.href = '/' + item.path;
    a.append(svg(item.path), el('span', '', item.label));
    body.append(a);
  }
  for (const name of tagNames()) {
    const a = el('a', 'mobile-lists-item' + (seg === 'people' && currentTag === name ? ' active' : ''));
    a.href = '/people?tag=' + encodeURIComponent(name);
    const icon = svg('tag');
    icon.style.color = `hsl(${hue(name)}, 65%, 40%)`;
    a.append(icon, el('span', '', name));
    body.append(a);
  }
}

// Both the desktop and mobile user menus carry their own copy of this
// checkbox (admin-only, server-rendered) - a quicker way to flip Super Edit Mode
// than the full admin page, which still has its own toggle too. Wired once here
// since the checkboxes are static; syncSuperEditCheckboxes (called from load())
// keeps their checked state true to the model after every reload, including one
// triggered by a different tab or the admin page.
for (const box of document.querySelectorAll('.super-edit-checkbox')) {
  box.addEventListener('change', () => setSuperEdit(box.checked));
}

function setDrawer(open) {
  drawer.hidden = !open;
  drawerOverlay.hidden = !open;
}

document.querySelector('#mobile-menu-btn').addEventListener('click', () => setDrawer(true));
document.querySelector('#drawer-close').addEventListener('click', () => setDrawer(false));
drawerOverlay.addEventListener('click', () => setDrawer(false));

// Enter jumps straight to the highlighted result, the way a browser's own
// address bar completes on Enter, so search-then-Enter never requires
// reaching for the mouse. Arrow keys move the highlight between results
// first, same as any other combobox.
function goToActiveResult(resultsEl) {
  const active = resultsEl.querySelector('.gsearch-result.active');
  if (active) {
    location.href = active.href;
  }
}

function moveActiveResult(resultsEl, delta) {
  const results = [...resultsEl.querySelectorAll('.gsearch-result')];
  if (!results.length) {
    return;
  }
  const current = results.findIndex(r => r.classList.contains('active'));
  const next = (current + delta + results.length) % results.length;
  if (current >= 0) {
    results[current].classList.remove('active');
  }
  results[next].classList.add('active');
  results[next].scrollIntoView({block: 'nearest'});
}

function handleSearchNavKeys(resultsEl, e) {
  if (e.key === 'Enter') {
    goToActiveResult(resultsEl);
  } else if (e.key === 'ArrowDown') {
    e.preventDefault();
    moveActiveResult(resultsEl, 1);
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    moveActiveResult(resultsEl, -1);
  }
}

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
topbarSearchInput.addEventListener('keydown', e => handleSearchNavKeys(topbarSearchResults, e));

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
mobileSearchInput.addEventListener('keydown', e => handleSearchNavKeys(mobileSearchResults, e));
function closeFilterPanels() {
  for (const panel of document.querySelectorAll('.filter-panel')) {
    panel.hidden = true;
    panel.parentElement.querySelector('.filter-button').classList.remove('open');
  }
}

document.addEventListener('click', e => {
  userMenu.hidden = true;
  mobileUserMenu.hidden = true;
  for (const p of document.querySelectorAll('.stale-menu')) {
    p.hidden = true;
  }
  if (!topbarSearchResults.hidden && !e.target.closest('.topbar-search')) {
    topbarSearchResults.hidden = true;
  }
  for (const menu of document.querySelectorAll('.more-menu, .card-menu, .photo-menu')) {
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
// A bare "/" (no modifiers, and not already typing somewhere) jumps straight to
// search, the way GitHub/Slack do - skipped while any text field, including the
// search box itself, already has focus so a literal "/" can still be typed.
function isEditableTarget(target) {
  return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable;
}

document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    userMenu.hidden = true;
    mobileUserMenu.hidden = true;
    for (const p of document.querySelectorAll('.stale-menu')) {
      p.hidden = true;
    }
    setDrawer(false);
    setMobileSearch(false);
    topbarSearchResults.hidden = true;
    closeFilterPanels();
    for (const menu of document.querySelectorAll('.more-menu, .card-menu, .photo-menu')) {
      menu.hidden = true;
    }
    if (e.target === topbarSearchInput || e.target === mobileSearchInput) {
      e.target.blur();
    }
  } else if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
    e.preventDefault();
    if (isMobile()) {
      setMobileSearch(true);
    } else {
      topbarSearchInput.focus();
    }
  } else if (e.key.toLowerCase() === 't' && e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
    // Same single "the tag button" a person's detail page shows (see
    // breadcrumbs' tagEmail param) - clicking it does the actual work, so the
    // shortcut just replays that click rather than duplicating its logic.
    const tagButton = document.querySelector('.tag-button');
    if (tagButton) {
      e.preventDefault();
      tagButton.click();
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

// setSuperEdit is the one place that actually flips the switch - shared by the
// banner's "Turn off" link and the user menus' checkbox, both of which just
// need to call it and let the reload (which calls syncSuperEditCheckboxes and
// renderSuperEditBanner) bring every copy of the control back in sync.
async function setSuperEdit(enabled) {
  await fetch('/api/admin/super-edit', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({enabled}),
  });
  await load();
}

function syncSuperEditCheckboxes() {
  for (const box of document.querySelectorAll('.super-edit-checkbox')) {
    box.checked = Boolean(state.model.superEdit);
  }
}

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
    await setSuperEdit(false);
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
  const family = familyOf(byEmail[document.body.dataset.userEmail]);
  return family ? privacyWarnings(family) : [];
}

function renderPrivacyMenuAlert() {
  const hasMismatch = myPrivacyWarnings().length > 0;
  const staleCount = staleItems().length;
  const hasStale = staleCount > 0;
  // My Privacy only has anything to show for someone in a family (it's entirely
  // about the family's address/phone visibility) - staff with no family record
  // would just land on an empty page, so hide the link for them instead.
  const hasFamily = !!familyOf(byEmail[document.body.dataset.userEmail]);
  for (const link of document.querySelectorAll('.user-menu-privacy')) {
    link.hidden = !hasFamily;
  }

  for (const badge of document.querySelectorAll('.user-menu-alert')) {
    badge.hidden = !hasMismatch;
    if (hasMismatch && !badge.firstChild) {
      badge.append(svg('alert'));
    }
  }

  // Mobile carries its own copy of both topbar icons now that the avatar lives
  // in the same top-right corner as desktop's, so both share this one loop
  // (over every .stale-alert/.privacy-alert in the page) instead of each
  // querying a single id.
  for (const staleButton of document.querySelectorAll('.stale-alert')) {
    staleButton.hidden = !hasStale;
    staleButton.title = `${staleCount} thing${staleCount === 1 ? '' : 's'} to update for the new year`;
  }
  for (const count of document.querySelectorAll('.stale-count')) {
    count.textContent = String(staleCount);
  }
  for (const privacyButton of document.querySelectorAll('.privacy-alert')) {
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

// Neither iOS Safari nor Chrome expose an install API - the closest thing is
// pointing people at the share sheet's "Add to Home Screen" entry by hand.
// Android/desktop Chrome instead fire `beforeinstallprompt`, which we stash
// here so the card's button can trigger the real native prompt on demand.
let deferredInstallPrompt = null;
const INSTALL_PROMPT_DISMISS_KEY = 'installPromptDismissedAt';
const INSTALL_PROMPT_COOLDOWN_DAYS = 30;

function installPromptDismissedRecently() {
  try {
    const at = Number(localStorage.getItem(INSTALL_PROMPT_DISMISS_KEY));
    return Boolean(at) && Date.now() - at < INSTALL_PROMPT_COOLDOWN_DAYS * 24 * 60 * 60 * 1000;
  } catch (e) {
    return false;
  }
}

function dismissInstallPrompt() {
  try {
    localStorage.setItem(INSTALL_PROMPT_DISMISS_KEY, String(Date.now()));
  } catch (e) {
    // ignore - the card just may reappear sooner than intended
  }
}

function runningStandalone() {
  return window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
}

function iosDevice() {
  return /iphone|ipad|ipod/i.test(navigator.userAgent) ||
    (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
}

// beforeinstallprompt also fires on desktop Chrome/Edge, but "add to home
// screen" is a phone/tablet idea - desktop already has a real taskbar.
function mobileDevice() {
  return iosDevice() || /android/i.test(navigator.userAgent);
}

// iOS 26's Safari redesign folded the toolbar's standalone Share icon into a
// "..." overflow button, so the old "tap Share" instructions point at nothing
// on 26+. Below that, Share still has its own icon in the toolbar.
function iosMajorVersion() {
  const match = navigator.userAgent.match(/OS (\d+)_/) || navigator.userAgent.match(/Version\/(\d+)/);
  return match ? Number(match[1]) : 0;
}

window.addEventListener('beforeinstallprompt', (e) => {
  e.preventDefault();
  deferredInstallPrompt = e;
  maybeShowInstallPrompt();
});

window.addEventListener('appinstalled', () => {
  deferredInstallPrompt = null;
  dismissInstallPrompt();
  document.querySelector('.install-prompt-overlay')?.remove();
});

function maybeShowInstallPrompt() {
  if (!mobileDevice() || runningStandalone() || installPromptDismissedRecently() ||
      document.querySelector('.install-prompt-overlay')) {
    return;
  }
  const isIOS = iosDevice();
  if (!isIOS && !deferredInstallPrompt) {
    return;
  }
  document.body.append(installPromptOverlay(isIOS));
}

function installPromptOverlay(isIOS) {
  const overlay = el('div', 'install-prompt-overlay');
  const card = el('div', 'install-prompt-card');

  const close = el('button', 'install-prompt-close', '×');
  close.type = 'button';
  close.setAttribute('aria-label', 'Dismiss');
  close.addEventListener('click', () => {
    overlay.remove();
    dismissInstallPrompt();
  });
  card.append(close);

  const icon = el('img', 'install-prompt-icon');
  icon.src = '/static/brand/icon-192.png';
  icon.alt = '';
  card.append(icon);

  card.append(el('div', 'install-prompt-title', 'Install Helios Who?'));
  card.append(el('div', 'install-prompt-desc',
    'Add this app to your home screen for easy access and a better experience.'));

  if (isIOS) {
    const hint = el('div', 'install-prompt-hint');
    if (iosMajorVersion() >= 26) {
      const dots = svg('ellipsis');
      dots.classList.add('install-prompt-dots');
      hint.append('Tap ', dots, ' then ', svg('upload'), ' Share, then “Add to Home Screen”');
    } else {
      hint.append('Tap ', svg('upload'), ' then “Add to Home Screen”');
    }
    card.append(hint);
  } else {
    const button = el('button', 'install-prompt-button', 'Install');
    button.type = 'button';
    button.addEventListener('click', async () => {
      overlay.remove();
      dismissInstallPrompt();
      if (deferredInstallPrompt) {
        deferredInstallPrompt.prompt();
        await deferredInstallPrompt.userChoice;
        deferredInstallPrompt = null;
      }
    });
    card.append(button);
  }

  overlay.append(card);
  return overlay;
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
  indexFamilies();
  state.everyoneOrder = shuffled(state.model.people);
  state.familyOrder = shuffled(familyEntries());
  renderSuperEditBanner();
  syncSuperEditCheckboxes();
  renderSpoofBanner();
  renderPrivacyMenuAlert();
  render();
  maybeShowInstallPrompt();
}

load();
