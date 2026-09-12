// The model as the server rendered it for the viewer, plus the page's own
// choices: which celebration's parties are showing, the list's tab, the
// category filter.
export const state = {model: null, celebration: '', tab: 'available', category: ''};

const byId = new Map();

export function applyModel(model) {
  state.model = model;
  byId.clear();
  for (const p of model.parties) {
    byId.set(p.id, p);
  }
  if (!state.celebration || !model.celebrations.some(c => c.code === state.celebration)) {
    state.celebration = model.current || (model.celebrations[0] ? model.celebrations[0].code : '');
  }
}

export function me() {
  return state.model.user;
}

export function isAdmin() {
  return state.model.user.isAdmin;
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

// parties is the chosen celebration's parties, in the server's date order.
export function parties(code) {
  return state.model.parties.filter(p => p.celebration === (code || state.celebration));
}

// partyPath is a party's address: /p/{pretty} with a friendly address,
// /parties/{id} without.
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
    return state.model.parties.find(p => p.prettyId === want) || null;
  }
  if (segs[0] === 'parties') {
    return party(segs[1]);
  }
  return null;
}

// resolvePath is walkPath plus the Redirects tab, mirroring Model.Resolve: an
// old friendly address lands on the party it was renamed to.
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

// household is everyone in the viewer's family, the viewer first.
export function household() {
  const user = me();
  const self = {email: user.email, name: user.name, photoUrl: user.photoUrl, isStudent: user.isStudent, isParent: user.isParent, isStaff: user.isStaff};
  return [self, ...user.adults, ...user.children];
}

export function isFamily(email) {
  return household().some(p => p.email === email);
}

// familyMember finds someone in the household by the address's local part -
// /my/ella.whitfield - or, for an older link, the whole address.
export function familyMember(seg) {
  const want = (seg || '').toLowerCase();
  return household().find(p => p.email === want || p.email.split('@')[0] === want) || null;
}

// myPath is a household member's own page: the address's local part.
export function myPath(person) {
  return `/my/${encodeURIComponent(person.email.split('@')[0])}`;
}

// billable is who may be invoiced: the viewer when they are not a student,
// and the other adults of the household.
export function billable() {
  const user = me();
  const out = [];
  if (!user.isStudent) {
    out.push({email: user.email, name: user.name, photoUrl: user.photoUrl});
  }
  return out.concat(user.adults);
}

// admits says whether a party's audience rules let this person hold a ticket.
export function admits(p, person) {
  return (person.isParent && p.parents) || (person.isStudent && p.students) || (person.isStaff && p.staff);
}

// audienceWords is the party's rule in words: "parents and staff".
export function audienceWords(p) {
  const words = [];
  if (p.parents) {
    words.push('parents');
  }
  if (p.staff) {
    words.push('staff');
  }
  if (p.students) {
    words.push('students');
  }
  return words.join(' and ');
}

// myTickets is the household's tickets on a party - sold and waiting - by
// the Mine flag the server set.
export function myTickets(p) {
  return [...p.attendees, ...p.waitlisted].filter(a => a.mine);
}

export function ticketFor(p, email) {
  return [...p.attendees, ...p.waitlisted].find(a => a.email === email) || null;
}

export function isHosting(p) {
  return Boolean(p.hosting);
}

export function pendingParties() {
  return state.model.parties.filter(p => p.status === 'Pending');
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

// parseWhen reads the sheet's "YYYY-MM-DD" or "YYYY-MM-DD HH:MM" as a local
// wall-clock time.
export function parseWhen(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})(?: (\d{2}):(\d{2}))?$/.exec(s || '');
  if (!m) {
    return null;
  }
  return {date: new Date(+m[1], m[2] - 1, +m[3], m[4] ? +m[4] : 0, m[5] ? +m[5] : 0), hasTime: Boolean(m[4])};
}

const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric', year: 'numeric'});
const longDayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric', year: 'numeric'});
const timeFormat = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});

function sameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

// whenParts is a party's date line in pieces: the day, and the time span.
// "Sat Sep 19, 2026" and "5:00 PM - 9:00 PM".
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

// priceLine is "$65 per person": the price and what one ticket covers.
export function priceLine(p) {
  if (!p.price) {
    return 'Free';
  }
  if (!p.unit) {
    return money(p.price);
  }
  // A unit written as a whole phrase - "1 ticket per child (parents free)",
  // the way the old site's hosts typed it - stands on its own.
  if (/^\d|\bper\b/i.test(p.unit)) {
    return `${money(p.price)} · ${p.unit}`;
  }
  return `${money(p.price)} per ${p.unit.toLowerCase()}`;
}

// availabilityLabel is the badge over a card: what the ticket button will say.
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

// googleCalendarLink builds the Add to Google Calendar address for a party.
export function googleCalendarLink(p) {
  const start = parseWhen(p.start);
  if (!start) {
    return '';
  }
  const end = parseWhen(p.end);
  const stamp = d => `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}`
    + (start.hasTime ? `T${String(d.getHours()).padStart(2, '0')}${String(d.getMinutes()).padStart(2, '0')}00` : '');
  let until = end ? end.date : new Date(start.date.getTime() + 2 * 60 * 60 * 1000);
  if (!start.hasTime) {
    until = new Date((end ? end.date : start.date).getTime() + 24 * 60 * 60 * 1000);
  }
  const params = new URLSearchParams({
    action: 'TEMPLATE',
    text: p.title,
    dates: `${stamp(start.date)}/${stamp(until)}`,
    details: [p.summary, location.origin + partyPath(p)].filter(Boolean).join('\n\n'),
    location: p.address || p.location || '',
  });
  return `https://calendar.google.com/calendar/render?${params}`;
}
