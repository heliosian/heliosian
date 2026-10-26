import {googleCalendarLink, parseWhen} from '/datecard.js';
import {api} from '/api.js';
import {queries, rowsOf} from '/rail.js';
import {availabilityOf} from '/calendar.js';
import {directory, emailOf, photoOf, isStudent, isParent, isStaff, gradeOf, relativesOf} from '/directory.js';
import {chipsOf, withoutChips, plain} from '/markdown.js';

export const state = {model: null, celebration: '', tab: 'upcoming', hostingTab: 'mine', category: '', showPast: true};

export function familyShown(p) {
  return state.showPast || p.availability !== 'past';
}

const byId = new Map();

function groupBy(rows, key) {
  const out = {};
  for (const row of rows) {
    (out[row[key]] = out[row[key]] || []).push(row);
  }
  return out;
}

function statusOf(g) {
  if (!g.visible_to) {
    return 'Hidden';
  }
  return g.status === 'pending' ? 'Pending' : 'Open';
}

export function summaryOf(description) {
  const first = withoutChips(description).split(/\n\s*\n/).map(s => s.trim()).find(s => s && !s.startsWith('>')) || '';
  return plain(first);
}

function linksOf(markdown) {
  return [...(markdown || '').matchAll(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g)].map(m => ({text: m[1], url: m[2]}));
}

export async function loadModel() {
  const [a, dir] = await Promise.all([queries({
    viewer: '(from PERSON (where (= id @viewer)))',
    household: '(from PERSON (where (in id (household @viewer))) (columns name_show))',
    admin: '(from APP (where (= key "celebrate") (admin_of "celebrate")) (columns key))',
    students: '(from GROUP (where (= id (slug "students"))) (columns slug))',
    adults: '(from GROUP (where (= id (slug "adults"))) (columns slug))',
    runs: '(from GROUP @g (where (in kind "party" "celebration" "category") (runs_list @g)) (columns kind))',
    celebrations: '(from GROUP (where (= kind "celebration") (!= status "closed")) (order start desc))',
    categories: '(from GROUP (where (= kind "category") (in parent (select GROUP.id (= kind "celebration")))) (order order asc))',
    parties: '(from GROUP (where (= kind "party") (!= status "closed")) (order start asc))',
    tickets: '(from MEMBER (where (or (in group (select GROUP.id (= kind "party"))) (in group (select GROUP.waitlist (= kind "party"))))) (include person))',
    hosts: '(from MEMBER (where (in group (select GROUP.managed_by (= kind "party")))) (columns group person member))',
    yes: '(from MEMBER (where (in group (select GROUP.rsvp_yes (= kind "party")))) (columns group person))',
    no: '(from MEMBER (where (in group (select GROUP.rsvp_no (= kind "party")))) (columns group person))',
    payments: '(from PAYMENT (where (in group (select GROUP.id (in kind "party" "celebration")))) (include person for))',
    top: '(from GROUP (where (= kind "category") (= slug "celebrate")) (columns slug visible_to members_visible_to))',
    invitations: '(from MESSAGE (where (= kind "invitation") (in group (select GROUP.id (= kind "party")))) (columns group))',
    eligible: '(from EFFECTIVE_MEMBER (where (in group (select GROUP.eligible (= kind "party"))) (in person (household @viewer))) (columns group person))',
    photos: '(from PHOTO (where ready (in group (select GROUP.id (in kind "party" "celebration")))) (order order asc) (columns group))',
    families: '(from GROUP (where (= kind "family")) (columns name slug who_href))',
    guests: '(from PERSON (where (= source "guest")) (order name_sort asc) (columns name_show))',
    settings: '(from SETTING (where (= app "celebrate")))',
  }), directory()]);
  const viewer = rowsOf(a.viewer, 'PERSON')[0];
  const admin = a.admin.result.length > 0;
  const students = a.students.result[0] || '';
  const runs = new Set(a.runs.result);
  const now = new Date();
  const people = {...(a.tickets.resources.PERSON || {}), ...(a.payments.resources.PERSON || {})};
  const nameOf = id => (dir.byId[id] && dir.byId[id].name_show) || (people[id] && people[id].name_show) || '';
  const emailOfId = id => (id ? emailOf({id}) : '');
  const settings = {};
  for (const s of rowsOf(a.settings, 'SETTING')) {
    settings[s.key] = s.value;
  }
  const picture = {};
  for (const ph of rowsOf(a.photos, 'PHOTO')) {
    picture[ph.group] = picture[ph.group] || ph.id;
  }
  const imageOf = id => (picture[id] ? `/api/blob/${picture[id]}/image` : '');
  const families = {};
  for (const f of rowsOf(a.families, 'GROUP')) {
    families[f.id] = f;
  }
  const household = new Set([viewer.id, ...a.household.result]);
  const viewerIsStudent = isStudent(viewer);

  const celebrations = rowsOf(a.celebrations, 'GROUP').map(c => ({
    id: c.id, code: c.slug || c.id, title: c.name, subtitle: c.subtitle || '', start: c.start || '', end: c.end || '',
    location: c.location || '', address: c.address || '', description: c.description || '', links: linksOf(c.description),
    image: picture[c.id] || '', imageUrl: imageOf(c.id), current: c.id === settings['Current'], adding: c.adding || '', row: c,
  }));
  const celebrationIds = new Set(celebrations.map(c => c.id));
  const categoryRows = rowsOf(a.categories, 'GROUP');
  const categoryIds = new Set(categoryRows.map(c => c.id));
  const categories = categoryRows.map(c => ({id: c.id, title: c.name, celebration: c.parent, order: c.order, row: c}));
  const categoryParent = Object.fromEntries(categoryRows.map(c => [c.id, c.parent]));

  const members = groupBy(rowsOf(a.tickets, 'MEMBER'), 'group');
  const hostsOf = groupBy(rowsOf(a.hosts, 'MEMBER'), 'group');
  const said = {};
  for (const [answer, word] of [[a.yes, 'yes'], [a.no, 'no']]) {
    for (const m of rowsOf(answer, 'MEMBER')) {
      said[m.group + '/' + m.person] = word;
    }
  }
  const paymentsOf = groupBy(rowsOf(a.payments, 'PAYMENT'), 'group');
  const invited = new Set(rowsOf(a.invitations, 'MESSAGE').map(m => m.group));
  const eligibleHas = new Set(rowsOf(a.eligible, 'EFFECTIVE_MEMBER').map(e => e.group + '/' + e.person));

  const familyOf = id => {
    const f = (dir.families[id] || []).map(fid => families[fid]).find(Boolean);
    return f ? {key: f.slug || f.id, name: f.name, href: f.who_href, photo: photoOf({id: f.id})} : null;
  };
  const kindOf = d => (!d ? 'Guest' : isStudent(d) ? 'Student' : isStaff(d) ? 'Staff' : isParent(d) ? 'Parent' : 'Guest');
  const lineOf = (d, m) => {
    switch (kindOf(d)) {
      case 'Student':
        return gradeOf(d);
      case 'Staff':
        return d.job_title || 'Staff';
      case 'Parent': {
        const kids = relativesOf(d, 'children').map(k => (gradeOf(k) ? `${k.name_show} (${gradeOf(k)})` : k.name_show));
        return kids.length ? 'Parent to ' + kids.join(', ') : 'Parent';
      }
    }
    return m.guest_of && m.guest_of !== m.person ? 'Guest of ' + nameOf(m.guest_of) : 'Guest';
  };

  const parties = [];
  for (const g of rowsOf(a.parties, 'GROUP')) {
    const under = categoryIds.has(g.parent) ? categoryParent[g.parent] : g.parent;
    if (!celebrationIds.has(under)) {
      continue;
    }
    const tickets = (members[g.id] || []).filter(m => m.member === 'yes');
    const waits = (members[g.waitlist] || []).filter(m => m.member === 'yes');
    const payments = (paymentsOf[g.id] || []).filter(p => p.status !== 'cancelled');
    const paidFor = {};
    for (const p of payments) {
      if (p.for && (!paidFor[p.for] || p.added > paidFor[p.for].added)) {
        paidFor[p.for] = p;
      }
    }
    const hostRows = (hostsOf[g.managed_by] || []).filter(m => m.member === 'yes');
    const hostIds = hostRows.map(m => m.person);
    const edit = runs.has(g.id);
    const sold = tickets.length;
    const capacity = Number(g.capacity) || 0;
    const availability = availabilityOf(g, sold, now);
    const isInvited = invited.has(g.id);
    const attendee = (m, status) => {
      const d = dir.byId[m.person] || null;
      const pay = status === 'Ticket' ? paidFor[m.person] : null;
      const family = familyOf(m.person) || (m.guest_of ? familyOf(m.guest_of) : null);
      const mine = household.has(m.person) || household.has(m.guest_of) || household.has(m.added_by) || Boolean(pay && household.has(pay.person));
      const sees = edit || admin || mine;
      const rsvp = said[g.rsvp_yes + '/' + m.person] || said[g.rsvp_no + '/' + m.person] || 'none';
      return {
        ticketId: m.id, person: m.person, email: emailOfId(m.person), name: nameOf(m.person) || (m.guest_of ? `Guest of ${nameOf(m.guest_of)}` : 'Guest'),
        photoUrl: d ? photoOf(d) : '', kind: kindOf(d), grade: d && isStudent(d) ? gradeOf(d) : '', line: lineOf(d, m),
        status, quantity: 1, added: m.added || '', mine, guestOf: m.guest_of || '', addedById: m.added_by || '',
        purchaser: sees && pay ? emailOfId(pay.person) : '', purchaserName: sees && pay ? nameOf(pay.person) : '',
        price: sees && pay ? Number(pay.amount) || 0 : 0, note: sees ? m.note || '' : '', addedBy: sees && m.added_by ? nameOf(m.added_by) : '',
        rsvp: isInvited ? rsvp : '',
        household: family ? family.key : m.guest_of || m.person, familyHref: family ? family.href : '', familyName: family ? family.name : '', familyPhoto: family ? family.photo : '',
        can: {reassign: edit || mine, delete: edit || mine},
      };
    };
    const unnamed = m => !nameOf(m.person) && m.guest_of;
    const waiting = [];
    const byRequester = {};
    for (const m of [...waits].sort((x, y) => (x.added || '').localeCompare(y.added || ''))) {
      if (unnamed(m) && byRequester[m.guest_of]) {
        byRequester[m.guest_of].quantity++;
        byRequester[m.guest_of].extra.push({id: m.id, person: m.person});
        continue;
      }
      const w = {...attendee(m, 'Waitlist'), extra: []};
      byRequester[m.person] = w;
      waiting.push(w);
    }
    const chips = chipsOf(g.description);
    const description = withoutChips(g.description);
    const p = {
      id: g.id, partyId: g.id, prettyId: g.slug || '', path: g.path,
      title: g.name, subtitle: g.subtitle || '', description, summary: summaryOf(g.description), chips, audience: chips.join(' · '),
      hosts: '', hostRows, hostPeople: hostIds.map(id => ({id, email: emailOfId(id), name: nameOf(id), photoUrl: dir.byId[id] ? photoOf(dir.byId[id]) : ''})),
      category: categoryIds.has(g.parent) ? g.parent : '', celebration: under,
      start: g.start || '', end: g.end || '', location: g.location || '', address: g.address || '',
      image: picture[g.id] || '', imageUrl: imageOf(g.id), flyer: g.flyer || '', flyerUrl: g.flyer ? `/api/blob/${g.id}/flyer` : '',
      price: Number(g.price) || 0, unit: g.unit || '', capacity, minimum: Number(g.minimum) || 0,
      ticketsOpen: g.join === 'direct', waitlist: Boolean(g.waitlist), waitlistGroup: g.waitlist || '',
      adults: g.eligible !== students, students: !g.eligible || g.eligible === students,
      dropOff: g.drop_off_allowed === 'Yes', parentTicket: g.parent_ticket_required === 'Yes',
      status: statusOf(g), availability, sold, waiting: waits.length,
      raised: payments.reduce((n, x) => n + (Number(x.amount) || 0), 0), remaining: capacity ? Math.max(0, capacity - sold) : 0,
      attendees: tickets.map(m => attendee(m, 'Ticket')), waitlisted: waiting,
      hosting: hostIds.includes(viewer.id), started: isInvited, invited: isInvited,
      admits: [...household].filter(id => !g.eligible || eligibleHas.has(g.eligible + '/' + id)).map(emailOfId).filter(Boolean),
      can: {
        edit, status: admin,
        buy: !viewerIsStudent && (edit || availability === 'available'),
        'join-waitlist': !viewerIsStudent && availability === 'waitlist',
      },
      row: g,
    };
    parties.push(p);
  }

  const person = id => {
    const d = dir.byId[id];
    return {id, email: emailOfId(id), name: nameOf(id), photoUrl: d ? photoOf(d) : '', isStudent: Boolean(d && isStudent(d)), isParent: Boolean(d && isParent(d)), isStaff: Boolean(d && isStaff(d))};
  };
  const others = a.household.result.filter(id => id !== viewer.id).map(person);
  const user = {...person(viewer.id), name: viewer.name_show, initial: (viewer.name_show || '?')[0].toUpperCase(),
    adults: others.filter(o => !o.isStudent), children: others.filter(o => o.isStudent)};
  const current = settings['Current'] || '';
  const currentCelebration = celebrations.find(c => c.id === current);
  applyModel({
    user,
    admin,
    roleGroups: {adults: a.adults.result[0] || '', students},
    allowances: admin ? ['celebrate.see-all', 'celebrate.curate', 'celebrate.configure', 'celebrate.act-as-host'] : [],
    can: {host: admin || Boolean(currentCelebration && currentCelebration.adding === 'approval')},
    settings: {partiesIntro: settings['Parties Intro'] || '', ticketNote: settings['Ticket Note'] || ''},
    settingRows: rowsOf(a.settings, 'SETTING'),
    current,
    banner: settings['Banner'] || '',
    imageSearch: true,
    billable: [user, ...user.adults].filter(x => !x.isStudent && x.email).map(x => x.email),
    celebrations,
    categories,
    parties,
    guests: rowsOf(a.guests, 'PERSON').filter(g => g.name_show).map(g => ({id: g.id, name: g.name_show})),
    top: rowsOf(a.top, 'GROUP')[0] || null,
    payments: rowsOf(a.payments, 'PAYMENT').map(x => ({...x, payerName: nameOf(x.person), payerEmail: emailOfId(x.person), forName: nameOf(x.for)})),
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
  const want = (code || '').toLowerCase();
  return state.model.celebrations.find(c => c.code.toLowerCase() === want || c.id === code) || null;
}

export function currentCelebration() {
  return celebration(state.celebration);
}

export function category(id) {
  return state.model.categories.find(c => c.id === id) || null;
}

export function categoriesOf(celebrationId) {
  return state.model.categories.filter(c => c.celebration === celebrationId);
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
  const answer = await api('QUERY', '/api/q', ['from', 'ALIAS', ['where', ['=', {n: 'alias'}, segment]], ['columns', 'target']]);
  for (const id of answer.result) {
    const p = party(answer.resources.ALIAS[id].target);
    if (p) {
      return p;
    }
  }
  return null;
}

export function household() {
  const user = me();
  return [user, ...user.adults, ...user.children];
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
  const people = [me(), ...me().adults];
  return state.model.billable.map(email => people.find(p => p.email === email));
}

export function admits(p, person) {
  return p.admits.includes(person.email);
}

export function inAudience(p, person) {
  return ((isParent(person) || isStaff(person)) && p.adults) || (isStudent(person) && p.students);
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
  return p.availability === 'waitlist' ? p.can['join-waitlist'] : p.can.buy;
}

export function canHost() {
  return state.model.can.host;
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
  const hosts = p.hostPeople.map(h => h.name).join(' ');
  return `${p.title} ${p.subtitle || ''} ${p.summary || ''} ${hosts} ${p.audience || ''} ${filed ? filed.title : ''} ${p.location || ''}`.toLowerCase().includes(query);
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
  return googleCalendarLink({title, start: c.start, end: c.end, details: [plain(c.description), location.origin].filter(Boolean).join('\n\n'), location: c.address || c.location || ''});
}

export function celebrationUpcoming(c) {
  const start = parseWhen(c.start);
  return Boolean(start) && start.date > new Date();
}
