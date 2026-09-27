import {googleCalendarLink, parseWhen} from '/datecard.js';
import {superEditOn} from '/superedit.js';

export const state = {model: null, celebration: '', tab: 'available', hostingTab: 'mine', category: '', showPast: true};

export function familyShown(p) {
  return state.showPast || p.availability !== 'past';
}

export function isSystemAdmin() {
  return state.model.user.isAdmin;
}

const byId = new Map();

export function applyModel(model) {
  state.model = model;
  model.allParties = model.allParties || model.parties;
  model.parties = model.allParties.filter(p => p.status === 'Open' || p.hosting || isAdmin());
  byId.clear();
  for (const p of findable()) {
    byId.set(p.id, p);
    // The server's canEdit counts the admin hat; here it counts only when
    // the hat is on. A host edits their own party either way.
    p.canEdit = Boolean(p.hosting) || isAdmin();
  }
  if (!state.celebration || !model.celebrations.some(c => c.code === state.celebration)) {
    state.celebration = model.current || (model.celebrations[0] ? model.celebrations[0].code : '');
  }
}

export function me() {
  return state.model.user;
}

export function isAdmin() {
  return state.model.user.isAdmin && superEditOn();
}

export function settings() {
  return state.model.settings;
}

export function party(id) {
  return byId.get(id) || null;
}

export function celebration(code) {
  return state.model.celebrations.find(c => c.code === code) || null;
}

export function currentCelebration() {
  return celebration(state.celebration);
}

export function parties(code) {
  return state.model.parties.filter(p => p.celebration === (code || state.celebration));
}

export function partyPath(p) {
  return p.prettyId ? `/p/${encodeURIComponent(p.prettyId)}` : `/parties/${encodeURIComponent(p.id)}`;
}

function walkPath(path) {
  const segs = path.split('/').filter(Boolean).map(decodeURIComponent);
  if (segs.length !== 2) {
    return null;
  }
  if (segs[0] === 'p') {
    const want = segs[1].toLowerCase();
    return findable().find(p => p.prettyId === want) || null;
  }
  if (segs[0] === 'parties') {
    return party(segs[1]);
  }
  return null;
}

export function resolvePath(path) {
  let at = path.replace(/\/+$/, '');
  if (!at.startsWith('/')) {
    at = '/p/' + at;
  }
  for (let hops = 0; hops < 20 && at; hops++) {
    const p = walkPath(at);
    if (p) {
      return p;
    }
    let moved = '';
    for (const r of state.model.redirects || []) {
      if (r.old.toLowerCase() === at.toLowerCase()) {
        moved = r.new;
      }
    }
    at = moved;
  }
  return null;
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
  const out = [];
  if (!user.isStudent) {
    out.push({email: user.email, name: user.name, photoUrl: user.photoUrl});
  }
  return out.concat(user.adults);
}

export function admits(p, person) {
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

export function isKid() {
  const user = me();
  return Boolean(user.isStudent && !user.isParent && !user.isStaff) && !isAdmin();
}

export function canHost() {
  return Boolean(state.model.settings && state.model.settings.hostingOpen) || isAdmin();
}

export function isHosting(p) {
  return Boolean(p.hosting);
}

function findable() {
  const model = state.model;
  if (!isSystemAdmin() || isAdmin()) {
    return model.parties;
  }
  return model.parties.concat(model.allParties.filter(p => p.status === 'Pending' && !p.hosting));
}

export function pendingParties() {
  const list = isSystemAdmin() ? state.model.allParties : state.model.parties;
  return list.filter(p => p.status === 'Pending');
}

export function canApprove(p) {
  return isSystemAdmin() && p.status === 'Pending';
}

export function hostedParties() {
  return state.model.parties.filter(p => p.hosting);
}

export function matches(p, query) {
  if (!query) {
    return true;
  }
  return `${p.title} ${p.subtitle || ''} ${p.summary || ''} ${p.hosts || ''} ${p.audience || ''} ${p.category || ''} ${p.location || ''}`.toLowerCase().includes(query);
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
