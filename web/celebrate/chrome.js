import {state, me, anyAllowance, pendingParties, hostedParties, parties, household, familyMember, myPath, myTickets, ticketFor, canHost, familyShown, celebration, category, categoriesOf, matches, partyPath, partiesPath, invitationPath, invitationPageId, shownCelebration, shownInvitation, shownLook, celebrationPath, guestsPath} from './state.js';
import {el, svg, link, button} from '/elements.js';
import {initShell, appSymbol} from '/shell.js';
import {initResults, showResults, closeResults, resultDay} from '/searchmenu.js';
import {parseWhen} from '/datecard.js';
import {navigate, setPath} from '/router.js';
import {openParty} from './edit.js';
import {invitationHead, topbarLine, guestListWords, guestListShown} from '/invitation.js';
import {dropMenu} from '/invites.js';
import {themed, sealMark} from '/themes.js';

const primary = [
  {href: '/', icon: 'app', label: 'Parties'},
  {href: '/my', icon: 'home', label: "My Family's Parties", count: () => familyParties().length, quiet: true},
  {href: '/hosting', icon: 'star', label: 'Hosting', count: () => hostedParties().length},
  {href: '/approvals', icon: 'hourglass', label: 'Approval Needed', admin: true, count: () => pendingParties().length},
];

function familyParties() {
  return state.model.parties.filter(p => familyShown(p) && myTickets(p).length);
}

function navItems() {
  return primary.map(item => ({...item, n: item.count ? item.count() : 0})).filter(item => (!item.admin || anyAllowance()) && (!item.count || item.n));
}

export function shownTabs(onList) {
  return listTabs.map(t => ({...t, count: parties().filter(p => inTab(p, t.key)).length})).filter(t => t.count || (onList && state.tab === t.key));
}

export function themedPage(className = '') {
  return el('div', [className, shownLook() ? 'themed-panel' : ''].filter(Boolean).join(' '));
}

export function pageHeadMain(title, kicker) {
  const main = el('div', 'page-head-main');
  if (shownLook() && kicker) {
    main.append(el('p', 'page-kicker', kicker));
  }
  main.append(el('h1', 'page-title', title));
  return main;
}

export function siteName() {
  const c = celebration(shownCelebration());
  return c ? c.title : '';
}

function onPartiesList() {
  return location.pathname === partiesPath();
}

function onInvitation() {
  return Boolean(shownInvitation()) && location.pathname === invitationPath('');
}

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return onPartiesList() || path.startsWith('/g/');
  }
  if (href === '/approvals') {
    return path === '/hosting' && hostingShown() === 'approvals';
  }
  if (href === '/hosting') {
    return path === '/hosting' && hostingShown() !== 'approvals';
  }
  return path === href || path.startsWith(href + '/');
}

function slug(words) {
  return words.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
}

export function listPath(tab, categoryId) {
  const q = new URLSearchParams();
  if (tab && tab !== listTabs[0].key) {
    q.set('show', tab);
  }
  const filed = category(categoryId);
  if (filed) {
    q.set('category', slug(filed.title));
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
  const found = want && categoriesOf(state.celebration).find(c => slug(c.title) === want);
  return found ? found.id : '';
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
  if (item.n && !item.quiet) {
    a.append(el('span', item.admin ? 'nav-count is-alert' : 'nav-count', String(item.n)));
  }
  return a;
}

function hostButton() {
  return button('Host a Party', 'plus', 'button nav-action', () => openParty(null));
}

export const listTabs = [
  {key: 'upcoming', label: 'Upcoming'},
  {key: 'available', label: 'Available'},
  {key: 'waitlist', label: 'Waitlist'},
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
  const onList = onPartiesList();
  for (const t of shownTabs(onList)) {
    const on = onList && state.tab === t.key;
    const item = el('button', 'nav-sub-item' + (on ? ' is-on' : ''));
    item.type = 'button';
    item.append(el('span', 'nav-sub-name', t.label), el('span', 'nav-sub-count', String(t.count)));
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
  const listed = familyParties();
  entry('/my', 'My Family', listed.length, current[0] === 'my' && !current[1]);
  people.forEach((person, i) => {
    const n = listed.filter(p => ticketFor(p, person.email)).length;
    if (!n) {
      return;
    }
    entry(myPath(person), i === 0 ? 'Me' : person.name, n, Boolean(shown && shown.email === person.email));
  });
  return wrap;
}

function sticker(c) {
  if (!c.look) {
    return null;
  }
  const mark = themed(el('span', 'year-sticker'), c.look);
  mark.append(el('span', 'env-seal', sealMark(c.look, c.title)));
  return mark;
}

function yearSwitch() {
  const toggle = el('button', 'year-switch');
  toggle.type = 'button';
  toggle.title = 'Another year';
  toggle.append(svg('chevron-down'));
  const items = state.model.celebrations.map(c => ({
    icon: 'calendar',
    mark: sticker(c),
    words: c.title,
    note: c.start.slice(0, 4) || 'Date to come',
    cls: c.id === state.celebration ? 'is-on' : '',
    run: () => navigate(celebrationPath(c.id)),
  }));
  const holder = dropMenu(toggle, items);
  holder.classList.add('year-switch-holder');
  return holder;
}

function invitationNav(nav, inv) {
  const head = invitationHead(inv, invitationPath(''));
  if (state.model.celebrations.length > 1) {
    head.prepend(yearSwitch());
  }
  nav.append(head);
  const pages = el('div', 'nav-pages');
  const card = link(invitationPath(''), onInvitation() && !invitationPageId() ? 'is-active' : '');
  card.append(svg('mail'), el('span', '', 'Invitation'));
  pages.append(card);
  for (const page of inv.pages) {
    const a = link(invitationPath(page.id), onInvitation() && invitationPageId() === page.id ? 'is-active' : '');
    a.append(svg('doc'), el('span', '', page.title));
    pages.append(a);
  }
  if (guestListShown(inv)) {
    const a = link(guestsPath(), location.pathname === guestsPath() ? 'is-active' : '');
    a.append(svg('groups'), el('span', '', guestListWords(inv)));
    pages.append(a);
  }
  nav.append(pages);
}

function fillNav(nav) {
  const inv = shownInvitation();
  topbarLine(inv);
  if (inv) {
    invitationNav(nav, inv);
  }
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
  for (const item of navItems().filter(i => !i.admin)) {
    bar.append(navLink(item));
  }
}

function found(celebrationId, query) {
  const hits = [];
  for (const p of sortByStart(parties(celebrationId))) {
    if (matches(p, query)) {
      hits.push({p});
    }
    const seen = new Set();
    const people = [
      ...p.hostPeople.map(person => ({person, role: 'Host'})),
      ...p.attendees.map(person => ({person, role: 'Guest'})),
      ...p.waitlisted.map(person => ({person, role: 'Waitlist'})),
    ];
    for (const {person, role} of people) {
      const key = role + '|' + person.name;
      if (seen.has(key) || !(person.name || '').toLowerCase().includes(query)) {
        continue;
      }
      seen.add(key);
      hits.push({p, person, role});
    }
  }
  return hits;
}

function resultItem({p, person, role}) {
  const filed = category(p.category);
  const where = person ? `${role} · ${p.title}` : filed ? filed.title : '';
  return {
    href: partyPath(p),
    image: person ? person.photoUrl : p.imageUrl,
    day: resultDay(p.start),
    title: person ? person.name : p.title,
    line: [where, p.availability === 'past' ? 'Past' : ''].filter(Boolean).join(' · '),
  };
}

function startTime(x) {
  const when = parseWhen(x.start);
  return when ? when.date.getTime() : 0;
}

function sortByStart(list) {
  return [...list].sort((a, b) => startTime(a) - startTime(b));
}

function earlierCelebration() {
  const current = celebration(state.celebration);
  const before = state.model.celebrations.filter(c => current && startTime(c) < startTime(current));
  return before.sort((a, b) => startTime(b) - startTime(a))[0] || null;
}

export function searchParties(query) {
  if (!query) {
    closeResults();
    return;
  }
  const hits = found(state.celebration, query);
  const current = [...hits.filter(hit => hit.p.availability !== 'past'), ...hits.filter(hit => hit.p.availability === 'past')];
  const earlier = earlierCelebration();
  const older = earlier ? found(earlier.id, query) : [];
  showResults(current.map(resultItem), earlier ? earlier.title : '', older.map(resultItem));
}

export function initChrome() {
  initShell({
    name: 'Helios Celebrate',
    me,
    fillNav,
    fillTabbar,
    search: {
      placeholder: 'Search parties…',
      results: true,
      carry: () => setPath(listPath(state.tab, state.category)),
    },
  });
  initResults();
}
