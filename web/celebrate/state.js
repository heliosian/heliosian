// The model as the server rendered it for the viewer, plus the page's own
// choices: which celebration's parties are showing, the list's tab, the
// Hosting page's tab, the category filter.
// superEdit is a system admin's hat, as HCA-Team has it: off, they see and
// can do what any parent can (plus whatever they host); on, every admin
// control comes back. It is remembered per browser. The server keeps
// enforcing by the admin list either way; the switch is about what the page
// shows and offers.
// showPast is the switch on My Family's Parties and the pages under it: on,
// as it starts, the parties that have been are listed after those to come;
// off, only what is still ahead. It lasts the visit and comes back on.
export const state = {model: null, celebration: '', tab: 'available', hostingTab: 'mine', category: '', superEdit: readSuperEdit(), showPast: true};

// familyShown says whether a party belongs on the family's pages with the
// past switch as it is.
export function familyShown(p) {
  return state.showPast || p.availability !== 'past';
}

function readSuperEdit() {
  try {
    return localStorage.getItem('celebrate.superEdit') === '1';
  } catch (err) {
    return false;
  }
}

export function setSuperEdit(on) {
  state.superEdit = on;
  try {
    localStorage.setItem('celebrate.superEdit', on ? '1' : '0');
  } catch (err) {
    // A browser that refuses storage just forgets the choice on reload.
  }
  applyModel(state.model);
}

// isSystemAdmin says the person is on the admin list, hat or no hat.
export function isSystemAdmin() {
  return state.model.user.isAdmin;
}

const byId = new Map();

// applyModel keeps the server's parties as allParties and lists in parties
// what the viewer sees with the hat as it is: the server sends an admin
// every party, but with the hat off one that is Pending or Hidden shows
// only to its hosts, as it would to any parent - off the lists and counts.
// Approvals are the exception: a Pending party waits on whoever is on the
// admin list, so it is still found by its address, hat or no hat. A Hidden
// one is not.
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
  return state.model.user.isAdmin && state.superEdit;
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
    return findable().find(p => p.prettyId === want) || null;
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

// admits says whether a party's audience rules let this person hold a
// ticket: adults are parents and staff alike.
export function admits(p, person) {
  return ((person.isParent || person.isStaff) && p.adults) || (person.isStudent && p.students);
}

// audienceWords is the party's rule in words: "adults and students".
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

// myTickets is the household's tickets on a party - sold and waiting - by
// the Mine flag the server set.
export function myTickets(p) {
  return [...p.attendees, ...p.waitlisted].filter(a => a.mine);
}

export function ticketFor(p, email) {
  return [...p.attendees, ...p.waitlisted].find(a => a.email === email) || null;
}

// isKid says the viewer is a student and nothing else: they browse and see
// who is coming, but tickets are taken and passed on by a parent.
export function isKid() {
  const user = me();
  return Boolean(user.isStudent && !user.isParent && !user.isStaff) && !isAdmin();
}

// canHost says whether the viewer may post a party: anyone while the
// Hosting Open setting is on, an admin regardless.
export function canHost() {
  return Boolean(state.model.settings && state.model.settings.hostingOpen) || isAdmin();
}

export function isHosting(p) {
  return Boolean(p.hosting);
}

// findable is what the viewer can reach by address: the listed parties,
// and for anyone on the admin list every Pending one too.
function findable() {
  const model = state.model;
  if (!isSystemAdmin() || isAdmin()) {
    return model.parties;
  }
  return model.parties.concat(model.allParties.filter(p => p.status === 'Pending' && !p.hosting));
}

// pendingParties is what waits for approval: every Pending party for anyone
// on the admin list, hat or no hat, and otherwise the viewer's own.
export function pendingParties() {
  const list = isSystemAdmin() ? state.model.allParties : state.model.parties;
  return list.filter(p => p.status === 'Pending');
}

// canApprove says the viewer can approve or turn down this party: anyone on
// the admin list, hat or no hat, while it is Pending.
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
// calendarLink is a Google Calendar "add this" address for anything with a
// start, an optional end, a name and a place: the event lands in the
// reader's calendar with the site's page in its notes.
export function calendarLink({title, start, end, details, location, address}) {
  const from = parseWhen(start);
  if (!from) {
    return '';
  }
  const to = parseWhen(end);
  const stamp = d => `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}`
    + (from.hasTime ? `T${String(d.getHours()).padStart(2, '0')}${String(d.getMinutes()).padStart(2, '0')}00` : '');
  let until = to ? to.date : new Date(from.date.getTime() + 2 * 60 * 60 * 1000);
  if (!from.hasTime) {
    until = new Date((to ? to.date : from.date).getTime() + 24 * 60 * 60 * 1000);
  }
  const params = new URLSearchParams({
    action: 'TEMPLATE',
    text: title,
    dates: `${stamp(from.date)}/${stamp(until)}`,
    details: details || '',
    location: address || location || '',
  });
  return `https://calendar.google.com/calendar/render?${params}`;
}

export function googleCalendarLink(p) {
  return calendarLink({title: p.title, start: p.start, end: p.end, details: [p.summary, location.origin + partyPath(p)].filter(Boolean).join('\n\n'), location: p.location, address: p.address});
}

// celebrationCalendarLink is the banner's Save the Date: the gala itself,
// by its title and theme.
export function celebrationCalendarLink(c) {
  const title = c.subtitle ? `${c.title} · ${c.subtitle}` : c.title;
  return calendarLink({title, start: c.start, end: c.end, details: [c.description, location.origin].filter(Boolean).join('\n\n'), location: c.location, address: c.address});
}
