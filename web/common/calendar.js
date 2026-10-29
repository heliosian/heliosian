import {queries, rowsOf, write} from '/rail.js';
import {parseWhen} from '/datecard.js';
import {andList} from '/andlist.js';

export const eventKinds = '(in kind "event" "party" "activity")';

const dayRoles = {'Regular': 'regular', 'No School': 'no-school', 'Early Dismissal': 'early-dismissal'};

export function by(rows, key) {
  const out = {};
  for (const row of rows) {
    (out[row[key]] = out[row[key]] || []).push(row);
  }
  return out;
}

function addDay(date, n) {
  const [y, m, d] = date.split('-').map(Number);
  const next = new Date(y, m - 1, d + n);
  return `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, '0')}-${String(next.getDate()).padStart(2, '0')}`;
}

function datesBetween(start, end) {
  const out = [];
  const last = (end || start).slice(0, 10);
  for (let d = start.slice(0, 10); d <= last; d = addDay(d, 1)) {
    out.push(d);
  }
  return out;
}

export function availabilityOf(g, sold, now) {
  const last = parseWhen(g.end || g.start);
  if (last && last.date < now) {
    return 'past';
  }
  if (g.status !== 'open' || !g.visible_to || g.join !== 'direct') {
    return 'closed';
  }
  const capacity = Number(g.capacity) || 0;
  if (!capacity || sold < capacity) {
    return 'available';
  }
  return g.waitlist ? 'waitlist' : 'sold-out';
}

const callWords = {available: 'Get tickets', waitlist: 'Join the waitlist', 'sold-out': 'Sold out', open: 'Join', full: 'Full'};

function mineWords(e) {
  const names = andList(e.mineWho) === 'You' ? '' : andList(e.mineWho);
  if (e.mine === 'waitlisted') {
    return names ? `${names} ${e.mineWho.length > 1 ? 'are' : 'is'} waitlisted` : 'Waitlisted';
  }
  if (e.mine !== 'going') {
    return '';
  }
  if (e.source === 'celebrate') {
    if (e.mineWho.length > 1) {
      return names + ' have tickets';
    }
    return names ? names + ' has a ticket' : 'You have a ticket';
  }
  return names ? names + ' signed up' : 'Signed up';
}

function dayWins(next, held, roles) {
  if (!held) {
    return true;
  }
  return roles[next] === 'no-school' || roles[held] === 'regular';
}

export const calendarQueries = {
  viewer: '(from PERSON (where (= id @viewer)) (columns name_show name_short))',
  email: '(from PERSON_EMAIL (where (= person @viewer) primary) (columns address))',
  everyone: '(from GROUP (where (= id (slug "everyone"))) (columns slug))',
  staff: '(from EFFECTIVE_MEMBER (where (= group (slug "staff")) (= person @viewer)) (columns group))',
  whenAdmin: '(from APP (where (= key "when") (admin_of "when")) (columns key))',
  household: '(from PERSON (where (or (= id @viewer) (in id (household @viewer)))) (columns name_short name_show classroom grade))',
  classrooms: '(from GROUP (where (= kind "classroom")) (order order asc name asc) (include parent) (columns name slug color parent))',
  categories: '(from GROUP (where (= kind "category") (blank parent)) (order order asc name asc) (columns name slug description default))',
  dayTypes: '(from GROUP (where (= kind "category") (= parent.slug "schedule")) (columns name))',
  days: '(from GROUP (where (= kind "day")) (columns name start parent))',
  parts: '(from GROUP (where (= kind "day_part")) (columns name start end parent))',
  dayRules: '(from RULE (where (in group (select GROUP.id (= kind "day")))) (columns group target))',
  tree: '(from GROUP (where (in kind "category" "activity" "celebration" "event")) (columns kind parent slug))',
  events: `(from GROUP @e (where ${eventKinds} (!= status "closed") (not (blank start))) (order start asc) (columns kind name slug start end all_day location description parent status visible_to listed audience rsvp_yes rsvp_no waitlist hidden_for capacity join added_by added path when_href celebrate_href team_href))`,
  audiences: `(from RULE (where (in group (select GROUP.audience ${eventKinds}))) (columns group target))`,
  sources: `(from GROUP_SOURCE (where (in group (select GROUP.id ${eventKinds}))) (columns group calendar_event document name))`,
  photos: `(from PHOTO (where (in group (select GROUP.id (in kind "event" "party" "activity" "category")))) (order order asc) (columns group ready))`,
  places: `(from MEMBER (where (= member "yes") (in group (select GROUP.id (in kind "party" "activity") (not (blank capacity))))) (columns group person))`,
  mine: '(from MEMBER (where (= member "yes") (or (= person @viewer) (in person (household @viewer)))) (columns group person))',
  hosted: `(from GROUP @g (where ${eventKinds} (manages @g)) (columns kind))`,
  adders: `(from PERSON (where (in id (select GROUP.added_by ${eventKinds}))) (columns name_show))`,
  saved: '(from SAVED_SEARCH (where (= person @viewer)) (order order asc) (columns name emoji order default token search condition))',
  invites: '(from MESSAGE @m (where (= kind "invitation") (exists RECIPIENT (= message @m) (or (= person @viewer) (in person (household @viewer))))) (columns group))',
};

export function buildCalendar(a) {
  const me = rowsOf(a.viewer, 'PERSON')[0];
  const everyone = a.everyone.result[0];
  const household = rowsOf(a.household, 'PERSON');
  const nameOf = id => id === me.id ? 'You' : ((household.find(p => p.id === id) || {}).name_short || 'Someone');
  const classroomRows = rowsOf(a.classrooms, 'GROUP');
  const classrooms = classroomRows.map(c => ({id: c.id, name: c.name, slug: c.slug, gradeband: c.parent ? a.classrooms.resources.GROUP[c.parent].name : '', grades: [], crews: []}));
  const classroomName = Object.fromEntries(classrooms.map(c => [c.id, c.name]));
  const photos = by(rowsOf(a.photos, 'PHOTO'), 'group');
  const imageOf = id => photos[id] ? `/api/blob/${photos[id][0].id}/image` : '';
  const tags = rowsOf(a.categories, 'GROUP').map(c => ({id: c.id, name: c.name, slug: c.slug, description: c.description || '', group: '', default: c.default === 'Yes', role: c.slug, image: imageOf(c.id), imageUrl: imageOf(c.id), photo: photos[c.id] ? photos[c.id][0].id : ''}));
  tags.push({id: 'going', name: 'Going', slug: 'going', description: 'Events you said yes to, parties your household holds tickets to, and activities someone in it signed up for.', group: '', default: true, builtIn: true, role: 'going', image: '', imageUrl: ''});
  tags.push({id: 'waitlisted', name: 'Waitlisted', slug: 'waitlisted', description: 'Parties your household is on the waitlist for.', group: '', default: true, builtIn: true, role: 'waitlisted', image: '', imageUrl: ''});
  const tree = a.tree.resources.GROUP;
  const topOf = id => {
    let g = tree[id];
    let top = '';
    for (let n = 0; g && n < 20; n++, g = tree[g.parent]) {
      if (g.kind === 'category') {
        top = g.id;
      }
    }
    return top;
  };
  const partsOf = by(rowsOf(a.parts, 'GROUP'), 'parent');
  const days = rowsOf(a.days, 'GROUP');
  const dayTypes = rowsOf(a.dayTypes, 'GROUP').map(t => {
    const sample = days.find(d => d.parent === t.id);
    const blocks = ((sample && partsOf[sample.id]) || []).map(p => ({name: p.name, start: p.start.slice(11), end: (p.end || p.start).slice(11)}));
    return {id: t.id, name: t.name, role: dayRoles[t.name] || '', blocks};
  });
  const order = ['regular', 'no-school', 'early-dismissal'];
  dayTypes.sort((x, y) => ((order.indexOf(x.role) + 1) || 9) - ((order.indexOf(y.role) + 1) || 9));
  const roles = Object.fromEntries(dayTypes.map(t => [t.id, t.role]));
  const dayRules = by(rowsOf(a.dayRules, 'RULE'), 'group');
  const dayMap = {};
  for (const d of days) {
    const rooms = dayRules[d.id] ? dayRules[d.id].map(r => classroomName[r.target]).filter(Boolean) : classrooms.map(c => c.name);
    const date = d.start.slice(0, 10);
    dayMap[date] = dayMap[date] || {};
    for (const room of rooms) {
      if (dayWins(d.parent, dayMap[date][room], roles)) {
        dayMap[date][room] = d.parent;
      }
    }
  }
  const audiences = by(rowsOf(a.audiences, 'RULE'), 'group');
  const sources = by(rowsOf(a.sources, 'GROUP_SOURCE'), 'group');
  const places = by(rowsOf(a.places, 'MEMBER'), 'group');
  const mine = by(rowsOf(a.mine, 'MEMBER'), 'group');
  const hosted = new Set(a.hosted.result);
  const invitedTo = new Set(rowsOf(a.invites, 'MESSAGE').map(m => m.group));
  const admin = a.whenAdmin.result.length > 0;
  const adders = Object.fromEntries(rowsOf(a.adders, 'PERSON').map(p => [p.id, p.name_show]));
  const rows = rowsOf(a.events, 'GROUP');
  const under = by(rows.filter(g => g.kind === 'activity'), 'parent');
  const below = id => (under[id] || []).flatMap(c => [c.id, ...below(c.id)]);
  const events = rows.map(g => {
    const source = g.kind === 'party' ? 'celebrate' : g.kind === 'activity' ? 'team' : (sources[g.id] || []).some(s => s.calendar_event?.startsWith('c_cmoccvs0uksquupa6g68mf2cs0@group.calendar.google.com/')) ? 'counseling' : (sources[g.id] || []).some(s => s.calendar_event) ? 'google' : (sources[g.id] || []).some(s => s.document) ? 'pdf' : 'sheet';
    const rooms = (audiences[g.audience] || []).map(r => classroomName[r.target]).filter(Boolean);
    const top = topOf(g.parent);
    const own = group => ((group && mine[group]) || []).find(m => m.person === me.id);
    const myRows = {yes: (own(g.rsvp_yes) || {}).id || '', no: (own(g.rsvp_no) || {}).id || '', hidden: (own(g.hidden_for) || {}).id || ''};
    const answer = myRows.yes ? 'yes' : myRows.no ? 'no' : myRows.hidden ? 'hidden' : '';
    const holders = [...new Set([g.id, ...(g.kind === 'activity' ? below(g.id) : [])].flatMap(id => (mine[id] || []).map(m => m.person)))];
    const waiting = [...new Set((mine[g.waitlist] || []).map(m => m.person))];
    const going = [...new Set((mine[g.rsvp_yes] || []).map(m => m.person))];
    const taken = (places[g.id] || []).length;
    const full = Boolean(g.capacity) && taken >= Number(g.capacity);
    const availability = g.kind === 'party' ? (!full ? 'available' : g.waitlist ? 'waitlist' : 'sold-out') : g.kind === 'activity' ? (g.status === 'done' ? 'done' : g.join === 'none' || full ? 'full' : 'open') : '';
    const e = {
      id: g.id, address: g.slug || '', path: g.path, whenHref: g.when_href, title: g.name, start: g.start, end: g.end || g.start, allDay: g.all_day === 'Yes', dates: datesBetween(g.start, g.end),
      location: g.location || '', description: g.description || '', source, sourceTitle: ((sources[g.id] || [])[0] || {}).name || '', sourceUrl: '',
      classrooms: rooms, tags: [top, ...(audiences[g.audience] || []).map(r => r.target).filter(t => classroomName[t])].filter(Boolean),
      sharing: g.visible_to === everyone ? (g.listed === 'Yes' || g.kind !== 'event' ? 'Public' : 'Link') : 'Invite Only',
      status: g.status, pending: g.status === 'pending', cancelled: g.status === 'cancelled', declined: false,
      link: g.celebrate_href || g.team_href,
      linkedId: g.kind === 'event' ? '' : g.id, availability, image: imageOf(g.id), hosts: [], hostNames: [], hosted: hosted.has(g.id),
      mine: holders.length ? 'going' : waiting.length ? 'waitlisted' : '', mineWho: (holders.length ? holders : waiting).map(nameOf), minePeople: [],
      going: going.length ? andList(going.map(nameOf)) + (going.length > 1 || going[0] === me.id ? ' are going' : ' is going') : '',
      me: {answer}, invited: invitedTo.has(g.id), invitation: invitedTo.has(g.id), dayType: '', keywords: [], addedBy: g.added_by || '', added: g.added || '',
      myRows, groups: {rsvp_yes: g.rsvp_yes || '', rsvp_no: g.rsvp_no || '', hidden_for: g.hidden_for || '', audience: g.audience || ''},
      audienceRules: (audiences[g.audience] || []).map(r => r.id), photo: photos[g.id] ? photos[g.id][0].id : '', category: top,
      can: {edit: hosted.has(g.id) || admin, correct: admin, image: hosted.has(g.id) || admin, approve: admin && g.status === 'pending', decline: admin && g.status === 'pending', keywords: false, opened: false},
    };
    if (e.mine || answer === 'yes') {
      e.tags.push('going');
    }
    if (e.mine === 'waitlisted') {
      e.tags.push('waitlisted');
    }
    e.mineWords = mineWords(e);
    e.call = e.mineWords || callWords[availability] || '';
    return e;
  });
  const wordsOf = search => {
    const lists = JSON.parse(search || '[]');
    const tagOf = Object.fromEntries(tags.map(t => [t.slug, t.id]));
    const roomOf = Object.fromEntries(classrooms.map(c => [c.slug, c.name]));
    const out = {classrooms: [], tags: []};
    for (const list of lists) {
      for (const w of list) {
        if (tagOf[w]) {
          out.tags.push(tagOf[w]);
        } else if (roomOf[w]) {
          out.classrooms.push(roomOf[w]);
        }
      }
    }
    return out;
  };
  const savedRows = rowsOf(a.saved, 'SAVED_SEARCH');
  const saved = savedRows.map((s, i) => ({id: s.id, token: s.token, name: s.name, emoji: s.emoji || '', order: s.order || '', position: i, locked: s.default === 'Yes', url: `/open/feed/${s.token}.ics`, condition: s.condition || '', ...wordsOf(s.search)}));
  const home = saved.find(s => s.locked) || {id: '', name: 'My Heliosian', emoji: '', position: 0, url: '', classrooms: [], tags: []};
  const mineRooms = [...new Set(household.filter(p => p.id !== me.id && p.grade && p.classroom).map(p => classroomName[p.classroom]).filter(Boolean))];
  return {
    me, email: (rowsOf(a.email, 'PERSON_EMAIL')[0] || {}).address || '', staff: a.staff.result.length > 0, everyone, household,
    classrooms, classroomRows, tags, dayTypes, dayMap, events, adders, admin, saved, savedRows, home, mineRooms,
  };
}

export function builtinWords(cal) {
  const rooms = cal.home.classrooms.length ? cal.home.classrooms : cal.mineRooms.length ? cal.mineRooms : cal.classrooms.map(c => c.name);
  const picked = cal.home.id ? cal.home.tags : cal.tags.filter(t => t.default).map(t => t.id);
  return wordsFor(cal, rooms, picked);
}

export function wordsFor(cal, rooms, picked) {
  const lists = [];
  if (rooms.length !== cal.classrooms.length) {
    const who = cal.classrooms.filter(c => rooms.includes(c.name) && c.slug).flatMap(c => [c.slug, c.slug + '-parents']);
    lists.push(cal.staff && who.length ? [...who, 'staff'] : who);
  }
  if (picked.length !== cal.tags.length) {
    lists.push(cal.tags.filter(t => picked.includes(t.id)).map(t => t.slug));
  }
  return lists;
}

export function wordsCondition(lists) {
  if (lists.some(l => !l.length)) {
    return '';
  }
  const word = w => w === 'going' ? '(going @e)' : w === 'waitlisted' ? '(waitlisted @e)' : `(searched_for @e ${JSON.stringify(w)})`;
  return [...lists.map(l => `(or ${l.map(word).join(' ')})`), '(not (hidden_by_viewer @e))'].join(' ');
}

export function matchingQuery(condition) {
  return `(from GROUP @e (where ${eventKinds} (!= status "closed") (not (blank start)) ${condition}) (columns status))`;
}

export function roomsOn(cal, date, classrooms) {
  const byClassroom = cal.dayMap[date];
  if (!byClassroom) {
    return null;
  }
  const groups = [];
  for (const c of classrooms) {
    const id = byClassroom[c];
    if (!id) {
      continue;
    }
    let group = groups.find(g => g.id === id);
    if (!group) {
      const type = cal.dayTypes.find(d => d.id === id);
      group = {id, name: type.name, type, classrooms: []};
      groups.push(group);
    }
    group.classrooms.push(c);
  }
  const order = cal.dayTypes.map(d => d.id);
  groups.sort((x, y) => order.indexOf(x.id) - order.indexOf(y.id));
  return groups;
}

const answerGroups = {yes: 'rsvp_yes', no: 'rsvp_no', hidden: 'hidden_for'};

export async function answerEvent(e, word, viewer) {
  const batch = Object.values(e.myRows).filter(Boolean).map(id => ({delete: id}));
  if (word) {
    batch.push({insert: 'MEMBER', row: {group: e.groups[answerGroups[word]], person: viewer, member: 'yes', added_by: viewer}});
  }
  if (batch.length) {
    await write(batch);
  }
}

export function loadCalendar(extra = {}) {
  return queries({...calendarQueries, ...extra});
}
