import {googleCalendarLink, parseWhen} from '/datecard.js';
import {batch, query, me as whoAmI} from '/data.js';

export const state = {model: null, celebration: '', tab: 'available', hostingTab: 'mine', category: '', showPast: true};

export function familyShown(p) {
  return state.showPast || p.availability !== 'past';
}

const byId = new Map();

function partyView(read, p) {
  const withCan = a => ({...a, can: read.get(a.ticketId).can});
  return {...p, attendees: p.attendees.map(withCan), waitlisted: p.waitlisted.map(withCan)};
}

export async function loadModel() {
  const [read, viewer] = await Promise.all([batch({
    settings: '/api/celebrate-settings',
    parties: '/api/parties?include=tickets',
    celebrations: '/api/celebrations',
    categories: '/api/party-categories',
  }), whoAmI()]);
  const s = read.get(read.result.settings[0]);
  applyModel({
    user: s.user,
    allowances: viewer.allowances,
    settingsId: s.id,
    can: s.can,
    today: s.today,
    settings: s.settings,
    current: s.current || '',
    banner: s.banner || '',
    invoicing: s.invoicing || [],
    imageSearch: s.imageSearch,
    billable: s.billable,
    celebrations: read.result.celebrations.map(read.get),
    categories: read.result.categories.map(read.get),
    parties: read.result.parties.map(id => partyView(read, read.get(id))),
  });
}

function applyModel(model) {
  state.model = model;
  byId.clear();
  for (const p of model.parties) {
    byId.set(p.id, p);
  }
  if (!state.celebration || !model.celebrations.some(c => c.id === state.celebration)) {
    state.celebration = model.current || (model.celebrations[0] ? model.celebrations[0].id : '');
  }
}

export function me() {
  return state.model.user;
}

export function allows(name) {
  return state.model.allowances.includes(name);
}

export function anyAllowance() {
  return state.model.allowances.some(a => a.startsWith('celebrate.'));
}

export function settingsId() {
  return state.model.settingsId;
}

export function settings() {
  return state.model.settings;
}

export function party(id) {
  return byId.get(id) || null;
}

export function celebration(id) {
  return state.model.celebrations.find(c => c.id === id) || null;
}

export function celebrationByCode(code) {
  return state.model.celebrations.find(c => c.code === code) || null;
}

export function currentCelebration() {
  return celebration(state.celebration);
}

export function category(id) {
  return state.model.categories.find(c => c.id === id) || null;
}

export function parties(id) {
  return state.model.parties.filter(p => p.celebration === (id || state.celebration));
}

export function partyPath(p) {
  return p.path;
}

export function partyAt(segment) {
  const want = segment.toLowerCase();
  return state.model.parties.find(p => p.partyId === segment || p.prettyId === want) || null;
}

export async function fetchParty(segment) {
  try {
    const read = await query('/api/parties/' + encodeURIComponent(segment));
    return read.get(read.result);
  } catch (err) {
    return null;
  }
}

export function household() {
  const user = me();
  const self = {email: user.email, name: user.name, photoUrl: user.photoUrl, isStudent: user.isStudent, isParent: user.isParent, isStaff: user.isStaff};
  return [self, ...user.adults, ...user.children];
}

export function isFamily(email) {
  return household().some(p => p.email === email);
}

export function familyMember(seg) {
  const want = (seg || '').toLowerCase();
  return household().find(p => p.email === want || p.email.split('@')[0] === want) || null;
}

export function myPath(person) {
  return `/my/${encodeURIComponent(person.email.split('@')[0])}`;
}

export function billable() {
  const user = me();
  const people = [{email: user.email, name: user.name, photoUrl: user.photoUrl}, ...user.adults];
  return state.model.billable.map(email => people.find(p => p.email === email));
}

export function admits(p, person) {
  return p.admits.includes(person.email);
}

export function inAudience(p, person) {
  return ((person.isParent || person.isStaff) && p.adults) || (person.isStudent && p.students);
}

export function audienceWords(p) {
  const words = [];
  if (p.adults) {
    words.push('adults');
  }
  if (p.students) {
    words.push('students');
  }
  return words.join(' and ');
}

export function myTickets(p) {
  return [...p.attendees, ...p.waitlisted].filter(a => a.mine);
}

export function ticketFor(p, email) {
  return [...p.attendees, ...p.waitlisted].find(a => a.email === email) || null;
}

export function mayTake(p) {
  if (p.offered) {
    return p.can.buy;
  }
  return p.availability === 'waitlist' ? p.can['join-waitlist'] : p.can.buy;
}

export function canHost() {
  return state.model.can.host;
}

export function isHosting(p) {
  return Boolean(p.hosting);
}

export function pendingParties() {
  return state.model.parties.filter(p => p.status === 'Pending');
}

export function canApprove(p) {
  return p.can.status && p.status === 'Pending';
}

export function hostedParties() {
  return state.model.parties.filter(p => p.hosting);
}

export function matches(p, query) {
  if (!query) {
    return true;
  }
  const filed = category(p.category);
  return `${p.title} ${p.subtitle || ''} ${p.summary || ''} ${p.hosts || ''} ${p.audience || ''} ${filed ? filed.title : ''} ${p.location || ''}`.toLowerCase().includes(query);
}


const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric', year: 'numeric'});
const longDayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric', year: 'numeric'});
const timeFormat = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});

function sameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

export function whenParts(p) {
  const start = parseWhen(p.start);
  if (!start) {
    return {};
  }
  const end = parseWhen(p.end);
  const out = {day: dayFormat.format(start.date), longDay: longDayFormat.format(start.date)};
  if (end && !sameDay(start.date, end.date)) {
    out.day = `${dayFormat.format(start.date)} - ${dayFormat.format(end.date)}`;
  }
  if (start.hasTime) {
    out.time = timeFormat.format(start.date);
    if (end && end.hasTime) {
      out.time += ` - ${timeFormat.format(end.date)}`;
    }
  }
  return out;
}

export function whenLine(p) {
  const w = whenParts(p);
  return [w.day, w.time].filter(Boolean).join(' · ');
}

export function money(n) {
  return Number.isInteger(n) ? `$${n}` : `$${n.toFixed(2)}`;
}

export function priceLine(p) {
  if (!p.price) {
    return 'Free';
  }
  if (!p.unit) {
    return money(p.price);
  }
  if (/^\d|\bper\b/i.test(p.unit)) {
    return `${money(p.price)} · ${p.unit}`;
  }
  return `${money(p.price)} per ${p.unit.toLowerCase()}`;
}

export function availabilityLabel(p) {
  switch (p.availability) {
    case 'available':
      return 'Tickets available';
    case 'waitlist':
      return 'Waitlist';
    case 'sold-out':
      return 'Sold out';
    case 'past':
      return 'Past';
  }
  return 'Tickets closed';
}

export function partyCalendarLink(p) {
  return googleCalendarLink({title: p.title, start: p.start, end: p.end, details: [p.summary, location.origin + partyPath(p)].filter(Boolean).join('\n\n'), location: p.address || p.location || ''});
}

export function celebrationCalendarLink(c) {
  const title = c.subtitle ? `${c.title} · ${c.subtitle}` : c.title;
  return googleCalendarLink({title, start: c.start, end: c.end, details: [c.description, location.origin].filter(Boolean).join('\n\n'), location: c.address || c.location || ''});
}
