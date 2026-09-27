import {state, me, isSystemAdmin, applySuperEdit, pendingParties, hostedParties, parties, household, familyMember, myPath, myTickets, canHost, familyShown} from './state.js';
import {el, svg, link, button} from './dom.js';
import {initShell, appSymbol} from '/shell.js';
import {navigate, render, setPath} from '/router.js';
import {openParty} from './edit.js';

const primary = [
  {href: '/', icon: 'app', label: 'Parties'},
  {href: '/my', icon: 'home', label: "My Family's Parties"},
  {href: '/hosting', icon: 'star', label: 'Hosting', count: () => hostedParties().length},
  {href: '/approvals', icon: 'hourglass', label: 'Approval Needed', admin: true, count: () => pendingParties().length},
];

function navItems() {
  return primary.filter(item => !item.admin || isSystemAdmin());
}

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return path === '/' || path.startsWith('/parties/') || path.startsWith('/celebrations/');
  }
  if (href === '/approvals') {
    return path === '/hosting' && hostingShown() === 'approvals';
  }
  if (href === '/hosting') {
    return path === '/hosting' && hostingShown() !== 'approvals';
  }
  return path === href || path.startsWith(href + '/');
}

export function partiesPath() {
  const code = state.celebration;
  return code === state.model.current ? '/' : `/celebrations/${encodeURIComponent(code)}`;
}

function slug(words) {
  return words.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
}

export function listPath(tab, category) {
  const q = new URLSearchParams();
  if (tab && tab !== listTabs[0].key) {
    q.set('show', tab);
  }
  if (category) {
    q.set('category', slug(category));
  }
  const rest = q.toString();
  return partiesPath() + (rest ? '?' + rest : '');
}

export function listTab() {
  const want = new URLSearchParams(location.search).get('show');
  return listTabs.some(t => t.key === want) ? want : listTabs[0].key;
}

export function listCategory() {
  const want = new URLSearchParams(location.search).get('category');
  return (want && state.model.categories.find(c => slug(c) === want)) || '';
}

export function hostingShown() {
  return new URLSearchParams(location.search).get('show') || 'mine';
}

function navLink(item) {
  let href = item.href === '/' ? partiesPath() : item.href;
  if (item.href === '/approvals') {
    href = '/hosting?show=approvals';
  }
  const a = link(href, active(item.href) ? 'is-active' : '');
  a.append(item.icon === 'app' ? appSymbol() : svg(item.icon), el('span', '', item.label));
  const n = item.count ? item.count() : 0;
  if (n) {
    a.append(el('span', item.admin ? 'nav-count is-alert' : 'nav-count', String(n)));
  }
  return a;
}

function hostButton() {
  return button('Host a Party', 'plus', 'button nav-action', () => openParty(null));
}

export const listTabs = [
  {key: 'available', label: 'Available'},
  {key: 'waitlist', label: 'Waitlist'},
  {key: 'upcoming', label: 'All Upcoming'},
  {key: 'past', label: 'Past Parties'},
];

export function inTab(p, tab) {
  switch (tab) {
    case 'available':
      return p.availability === 'available' && p.status === 'Open';
    case 'waitlist':
      return p.availability === 'waitlist' && p.status === 'Open';
    case 'past':
      return p.availability === 'past';
  }
  return p.availability !== 'past';
}

function tabLinks() {
  const wrap = el('div', 'nav-sub');
  const onList = location.pathname === '/' || location.pathname.startsWith('/celebrations/');
  for (const t of listTabs) {
    const n = parties().filter(p => inTab(p, t.key)).length;
    const on = onList && state.tab === t.key;
    const item = el('button', 'nav-sub-item' + (on ? ' is-on' : ''));
    item.type = 'button';
    item.append(el('span', 'nav-sub-name', t.label), el('span', 'nav-sub-count', String(n)));
    item.addEventListener('click', () => navigate(listPath(t.key, onList ? state.category : '')));
    wrap.append(item);
  }
  return wrap;
}

function familyLinks() {
  const people = household();
  if (people.length < 2) {
    return null;
  }
  const wrap = el('div', 'nav-sub');
  const current = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const shown = current[0] === 'my' && current[1] ? familyMember(current[1]) : null;
  const entry = (href, name, n, on) => {
    const a = link(href, 'nav-sub-item nav-family-item' + (on ? ' is-on' : ''));
    a.append(el('span', 'nav-sub-name', name), el('span', 'nav-sub-count', String(n)));
    wrap.append(a);
  };
  const listed = state.model.parties.filter(familyShown);
  entry('/my', 'My Family', listed.filter(p => myTickets(p).length).length, current[0] === 'my' && !current[1]);
  people.forEach((person, i) => {
    const n = listed.filter(p => [...p.attendees, ...p.waitlisted].some(a => a.email === person.email)).length;
    entry(myPath(person), i === 0 ? 'Me' : person.name, n, Boolean(shown && shown.email === person.email));
  });
  return wrap;
}

function fillNav(nav) {
  for (const item of navItems()) {
    nav.append(navLink(item));
    if (item.href === '/') {
      nav.append(tabLinks());
    }
    if (item.href === '/my') {
      const people = familyLinks();
      if (people) {
        nav.append(people);
      }
    }
  }
  if (canHost()) {
    nav.append(hostButton());
  }
}

function fillTabbar(bar) {
  for (const item of primary.filter(i => !i.admin)) {
    bar.append(navLink(item));
  }
}

export function initChrome() {
  initShell({
    name: 'Helios Celebrate',
    me,
    isSystemAdmin,
    onSuper: () => {
      applySuperEdit();
      render();
    },
    fillNav,
    fillTabbar,
    search: {
      placeholder: 'Search parties…',
      carry: () => setPath(listPath(state.tab, state.category)),
    },
  });
}
