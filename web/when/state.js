import {appOrigin} from '/appswitch.js';
import {parseWhen} from '/datecard.js';
import {queries} from '/rail.js';
import {api} from '/api.js';
import {render} from '/router.js';

const remembered = readFilters();
export const state = {model: null, filters: {classrooms: remembered.classrooms, tags: remembered.tags}, query: '', day: '', month: '', activeFeed: remembered.active, appliedCalendar: '', matching: new Set(), found: new Set()};

export function setActiveFeed(token) {
  state.activeFeed = token;
  saveFilters();
}

function readFilters() {
  const raw = JSON.parse(localStorage.getItem('calendar.filters') || '{}');
  return {classrooms: Array.isArray(raw.classrooms) ? raw.classrooms : null, tags: Array.isArray(raw.tags) ? raw.tags : null, active: typeof raw.active === 'string' ? raw.active : ''};
}

function saveFilters() {
  localStorage.setItem('calendar.filters', JSON.stringify({...state.filters, active: state.activeFeed}));
}

const byId = new Map();
const byDate = new Map();

const kinds = '(in kind "event" "party" "activity")';

const whole = {from: '2000-01-01', to: '2099-12-31'};

const dayRoles = {'Regular': 'regular', 'No School': 'no-school', 'Early Dismissal': 'early-dismissal'};

function rowsOf(answer, table) {
  return answer.result.map(id => answer.resources[table][id]);
}

function by(rows, key) {
  const out = {};
  for (const row of rows) {
    (out[row[key]] = out[row[key]] || []).push(row);
  }
  return out;
}

function datesBetween(start, end) {
  const out = [];
  const last = (end || start).slice(0, 10);
  for (let d = start.slice(0, 10); d <= last; d = addDays(d, 1)) {
    out.push(d);
  }
  return out;
}

function namesWords(names) {
  if (names.length < 2) {
    return names[0] || '';
  }
  return names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1];
}

const callWords = {available: 'Get tickets', waitlist: 'Join the waitlist', 'sold-out': 'Sold out', open: 'Join', full: 'Full'};

function mineWords(e) {
  const names = namesWords(e.mineWho) === 'You' ? '' : namesWords(e.mineWho);
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

export async function loadModel() {
  const a = await queries({
    viewer: '(from PERSON (where (= id @viewer)) (columns name_show name_short))',
    email: '(from PERSON_EMAIL (where (= person @viewer) primary) (columns address))',
    everyone: '(from GROUP (where (= id (slug "everyone"))) (columns slug))',
    staff: '(from EFFECTIVE_MEMBER (where (= group (slug "staff")) (= person @viewer)) (columns group))',
    admin: '(from APP (where (= key "when") (admin_of "when")) (columns key))',
    household: '(from PERSON (where (or (= id @viewer) (in id (household @viewer)))) (columns name_short name_show classroom grade))',
    classrooms: '(from GROUP (where (= kind "classroom")) (order order asc name asc) (include parent) (columns name slug color parent))',
    categories: '(from GROUP (where (= kind "category") (blank parent)) (order order asc name asc) (columns name slug description default))',
    dayTypes: '(from GROUP (where (= kind "category") (= parent.slug "schedule")) (columns name))',
    days: '(from GROUP (where (= kind "day")) (columns name start parent))',
    parts: '(from GROUP (where (= kind "day_part")) (columns name start end parent))',
    dayRules: '(from RULE (where (= group.kind "day")) (columns group target))',
    tree: '(from GROUP (where (in kind "category" "activity" "celebration" "event")) (columns kind parent slug))',
    events: `(from GROUP @e (where ${kinds} (!= status "closed") (not (blank start))) (order start asc) (columns kind name slug start end all_day location description parent status visible_to listed audience rsvp_yes rsvp_no waitlist hidden_for capacity join added_by added))`,
    audiences: `(from RULE (where (in group (select GROUP.audience ${kinds}))) (columns group target))`,
    sources: `(from GROUP_SOURCE (where (in group.kind "event" "party" "activity")) (columns group calendar_event document name))`,
    photos: `(from PHOTO (where (in group.kind "event" "party" "activity" "category")) (order order asc) (columns group ready))`,
    places: `(from MEMBER (where (= member "yes") (in group (select GROUP.id (in kind "party" "activity")))) (columns group person))`,
    mine: '(from MEMBER (where (= member "yes") (or (= person @viewer) (in person (household @viewer)))) (columns group person))',
    hosted: `(from GROUP @g (where ${kinds} (manages @g)) (columns kind))`,
    adders: `(from PERSON (where (in id (select GROUP.added_by ${kinds}))) (columns name_show))`,
    saved: '(from SAVED_SEARCH (where (= person @viewer)) (order order asc) (columns name emoji order default token search))',
    invites: '(from MESSAGE @m (where (= kind "invitation") (exists RECIPIENT (= message @m) (or (= person @viewer) (in person (household @viewer))))) (columns group))',
  });
  const me = rowsOf(a.viewer, 'PERSON')[0];
  const everyone = a.everyone.result[0];
  const household = rowsOf(a.household, 'PERSON');
  const nameOf = id => id === me.id ? 'You' : ((household.find(p => p.id === id) || {}).name_short || 'Someone');
  const classroomRows = rowsOf(a.classrooms, 'GROUP');
  const classrooms = classroomRows.map(c => ({id: c.id, name: c.name, slug: c.slug, band: c.parent ? a.classrooms.resources.GROUP[c.parent].name : '', grades: [], crews: []}));
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
  const admin = a.admin.result.length > 0;
  const adders = Object.fromEntries(rowsOf(a.adders, 'PERSON').map(p => [p.id, p.name_show]));
  const rows = rowsOf(a.events, 'GROUP');
  const under = by(rows.filter(g => g.kind === 'activity'), 'parent');
  const below = id => (under[id] || []).flatMap(c => [c.id, ...below(c.id)]);
  const events = rows.map(g => {
    const source = g.kind === 'party' ? 'celebrate' : g.kind === 'activity' ? 'team' : (sources[g.id] || []).some(s => s.calendar_event) ? 'google' : (sources[g.id] || []).some(s => s.document) ? 'pdf' : 'sheet';
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
      id: g.id, address: g.slug || '', title: g.name, start: g.start, end: g.end || g.start, allDay: g.all_day === 'Yes', dates: datesBetween(g.start, g.end),
      location: g.location || '', description: g.description || '', source, sourceTitle: ((sources[g.id] || [])[0] || {}).name || '', sourceUrl: '',
      classrooms: rooms, tags: [top, ...(audiences[g.audience] || []).map(r => r.target).filter(t => classroomName[t])].filter(Boolean),
      sharing: g.visible_to === everyone ? (g.listed === 'Yes' || g.kind !== 'event' ? 'Public' : 'Link') : 'Invite Only',
      status: g.status, pending: g.status === 'pending', cancelled: g.status === 'cancelled', declined: false,
      link: g.kind === 'party' ? `/p/${encodeURIComponent(g.slug || g.id)}` : g.kind === 'activity' ? `/v/${encodeURIComponent(g.slug || g.id)}` : '',
      linkedId: g.kind === 'event' ? '' : g.id, availability, image: imageOf(g.id), hosts: [], hostNames: [], hosted: hosted.has(g.id),
      mine: holders.length ? 'going' : waiting.length ? 'waitlisted' : '', mineWho: (holders.length ? holders : waiting).map(nameOf), minePeople: [],
      going: going.length ? namesWords(going.map(nameOf)) + (going.length > 1 || going[0] === me.id ? ' are going' : ' is going') : '',
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
  const saved = savedRows.map((s, i) => ({id: s.id, token: s.token, name: s.name, emoji: s.emoji || '', order: s.order || '', position: i, locked: s.default === 'Yes', url: `/open/feed/${s.token}.ics`, ...wordsOf(s.search)}));
  const home = saved.find(s => s.locked) || {id: '', name: 'My Heliosian', emoji: '', position: 0, url: '', classrooms: [], tags: []};
  const mineRooms = [...new Set(household.filter(p => p.id !== me.id && p.grade && p.classroom).map(p => classroomName[p.classroom]).filter(Boolean))];
  applyModel({
    user: {id: me.id, email: (rowsOf(a.email, 'PERSON_EMAIL')[0] || {}).address || '', name: me.name_show, classrooms: mineRooms, saved: home.id ? {classrooms: home.classrooms, tags: home.tags} : null, home, staff: a.staff.result.length > 0},
    settingsId: '',
    imageSearch: false,
    today: formatDate(new Date()),
    classrooms,
    colors: Object.fromEntries(classroomRows.map(c => [c.name, c.color || ''])),
    tags,
    dayTypes,
    years: [],
    days: dayMap,
    events,
    gradeColors: {},
    names: adders,
    feeds: saved.filter(s => !s.locked),
    saved: savedRows,
    admin,
    everyone,
    misc: (tags.find(t => t.slug === 'misc') || {}).id || '',
  });
  await refreshMatches();
}

export function wordsFor(rooms, picked) {
  const lists = [];
  if (rooms.length !== classroomNames().length) {
    const who = state.model.classrooms.filter(c => rooms.includes(c.name) && c.slug).flatMap(c => [c.slug, c.slug + '-parents']);
    lists.push(me().staff && who.length ? [...who, 'staff'] : who);
  }
  if (picked.length !== tagIds().length) {
    lists.push(state.model.tags.filter(t => picked.includes(t.id)).map(t => t.slug));
  }
  return lists;
}

export function searchWords() {
  return wordsFor(selectedClassrooms(), selectedTags());
}

let matchRun = 0;

export async function refreshMatches() {
  const run = ++matchRun;
  const lists = searchWords();
  if (lists.some(l => !l.length)) {
    state.matching = new Set();
    return;
  }
  const answer = await api('POST', '/api/do/saved-search', {search: lists, ...whole});
  if (run === matchRun) {
    state.matching = new Set(answer.result);
  }
}

function rematch() {
  refreshMatches().then(render);
}

let searchRun = 0;

export async function findSearch(query) {
  const run = ++searchRun;
  if (!words(query).length) {
    state.found = new Set();
    return;
  }
  const answer = await api('POST', '/api/do/search', {words: query, limits: {GROUP: 200, PERSON: 0, DOCUMENT: 0}});
  if (run === searchRun) {
    state.found = new Set((answer.GROUP || []).map(h => h.id));
  }
}

function applyModel(model) {
  state.model = model;
  byId.clear();
  byDate.clear();
  for (const e of model.events) {
    byId.set(e.id, e);
    if (e.address) {
      byId.set(e.address, e);
    }
    for (const date of eventDates(e)) {
      if (!byDate.has(date)) {
        byDate.set(date, []);
      }
      byDate.get(date).push(e);
    }
  }
  const names = classroomNames();
  if (state.filters.classrooms) {
    state.filters.classrooms = state.filters.classrooms.filter(c => names.includes(c));
  }
  const tags = tagIds();
  if (state.filters.tags) {
    state.filters.tags = state.filters.tags.filter(t => tags.includes(t));
  }
}

export function me() {
  return state.model.user;
}

export function settingsId() {
  return state.model.settingsId;
}

export function event(id) {
  return byId.get(id) || null;
}

export async function fetchEvent(id) {
  const a = await queries({alias: `(from ALIAS (where (= alias ${JSON.stringify(id.toLowerCase())})) (columns target))`});
  const row = rowsOf(a.alias, 'ALIAS')[0];
  const e = row ? byId.get(row.target) : null;
  if (e) {
    byId.set(id, e);
  }
  return e || null;
}

export function defaultFeedName() {
  const first = (me().name || '').trim().split(/\s+/)[0];
  return unusedFeedName(first ? `${first}\u2019s Heliosian Calendar` : 'Heliosian Calendar');
}

export function unusedFeedName(name) {
  const taken = new Set((state.model.feeds || []).map(f => f.name.toLowerCase()));
  if (!taken.has(name.toLowerCase())) {
    return name;
  }
  for (let n = 2; ; n++) {
    if (!taken.has(`${name} ${n}`.toLowerCase())) {
      return `${name} ${n}`;
    }
  }
}

export function feedURL(path) {
  return location.origin + path;
}

export function webcalURL(path) {
  return `webcal://${location.host}${path}`;
}

export const MY_HELIOSIAN = 'my-heliosian';

export function myHeliosian() {
  const home = me().home;
  return {id: home.id, url: home.url || '', token: MY_HELIOSIAN, name: home.name || 'My Heliosian', emoji: home.emoji || '', order: home.order || '', locked: true, position: home.position || 0, classrooms: [], tags: []};
}

export function allCalendars() {
  const feeds = [...(state.model.feeds || [])];
  const home = myHeliosian();
  feeds.splice(Math.min(Math.max(home.position, 0), feeds.length), 0, home);
  return feeds;
}

export function feedClassrooms(f) {
  if (f.locked) {
    return builtinClassrooms();
  }
  return f.classrooms.length ? f.classrooms : classroomNames();
}

export function feedTags(f) {
  if (f.locked) {
    return builtinTags();
  }
  return f.tags.length ? f.tags : tagIds();
}

function sameSet(a, b) {
  return a.length === b.length && a.every(x => b.includes(x));
}

export function showsFeed(f) {
  return sameSet(selectedClassrooms(), feedClassrooms(f)) && sameSet(selectedTags(), feedTags(f));
}

export function savedAlready() {
  return allCalendars().some(showsFeed);
}

export function activeFeed() {
  const feeds = allCalendars();
  const shown = feeds.find(showsFeed);
  if (shown) {
    if (state.activeFeed !== shown.token) {
      setActiveFeed(shown.token);
    }
    return shown;
  }
  return feeds.find(f => f.token === state.activeFeed) || defaultFeed();
}

export function classroomNames() {
  return state.model.classrooms.map(c => c.name);
}

export function tagGroups() {
  const groups = [];
  let loose = null;
  for (const t of state.model.tags) {
    if (!t.group) {
      loose = loose || {name: '', tags: []};
      loose.tags.push(t);
      continue;
    }
    let g = groups.find(g => g.name === t.group);
    if (!g) {
      g = {name: t.group, tags: []};
      groups.push(g);
    }
    g.tags.push(t);
  }
  return loose ? [...groups, loose] : groups;
}

export function tagIds() {
  return state.model.tags.map(t => t.id);
}

export function tagName(id) {
  const tag = state.model.tags.find(t => t.id === id) || state.model.classrooms.find(c => c.id === id);
  return tag ? tag.name : '';
}

export function roleTag(role) {
  const tag = state.model.tags.find(t => t.role === role);
  return tag ? tag.id : '';
}

export function classroomIds() {
  return state.model.classrooms.map(c => c.id);
}

export function bands() {
  const out = [];
  for (const c of state.model.classrooms) {
    let band = out.find(b => b.name === (c.band || c.name));
    if (!band) {
      band = {name: c.band || c.name, classrooms: []};
      out.push(band);
    }
    band.classrooms.push(c);
  }
  return out;
}

export function myClassrooms() {
  return me().classrooms;
}

export function savedView() {
  return me().saved || null;
}

export function defaultFeed() {
  return allCalendars()[0];
}

export function builtinClassrooms() {
  const saved = savedView();
  if (saved && saved.classrooms.length) {
    return saved.classrooms;
  }
  return myClassrooms().length ? myClassrooms() : classroomNames();
}

export function builtinTags() {
  const saved = savedView();
  if (saved) {
    return saved.tags;
  }
  return state.model.tags.filter(t => t.default).map(t => t.id);
}

export function defaultClassrooms() {
  return feedClassrooms(defaultFeed());
}

export function selectedClassrooms() {
  return state.filters.classrooms || defaultClassrooms();
}

export function setClassrooms(list) {
  state.filters.classrooms = list;
  saveFilters();
  rematch();
}

export function toggleClassroom(name) {
  const current = selectedClassrooms();
  setClassrooms(current.includes(name) ? current.filter(c => c !== name) : classroomNames().filter(c => c === name || current.includes(c)));
}

export function defaultTags() {
  return feedTags(defaultFeed());
}

export function selectedTags() {
  return state.filters.tags || defaultTags();
}

export function setTags(list) {
  state.filters.tags = list;
  saveFilters();
  rematch();
}

export function toggleTag(id) {
  const current = selectedTags();
  setTags(current.includes(id) ? current.filter(t => t !== id) : tagIds().filter(t => t === id || current.includes(t)));
}

export function filtersAreDefault() {
  return !state.filters.classrooms && !state.filters.tags;
}

export function resetFilters() {
  state.filters = {classrooms: null, tags: null};
  saveFilters();
  rematch();
}

export function categoryTags(e) {
  const rooms = classroomIds();
  return e.tags.filter(t => !rooms.includes(t));
}

export function eventVisible(e) {
  if (e.sharing === 'Link' && !answerOf(e) && !e.hosted && !e.mine) {
    return false;
  }
  return state.matching.has(e.id);
}

export function hiddenMatches(tag) {
  if (!state.query) {
    return 0;
  }
  return state.model.events.filter(e => e.tags.includes(tag) && isMatch(e) && !eventVisible(e)).length;
}

export function hiddenClassroomMatches(classroom) {
  if (!state.query) {
    return 0;
  }
  return state.model.events.filter(e => e.classrooms.includes(classroom) && isMatch(e) && !eventVisible(e)).length;
}

export function visibleEvents() {
  return state.model.events.filter(eventVisible);
}

function words(query) {
  return (query || '').toLowerCase().split(/\s+/).filter(Boolean);
}

export function dayTypeMatches(name, query) {
  const hay = name.toLowerCase();
  return words(query).every(w => hay.includes(w));
}

export function eventsOn(date) {
  return (byDate.get(date) || []).filter(eventVisible);
}

export function isMatch(e) {
  return Boolean(state.query) && state.found.has(e.id);
}

export function searchResults(query) {
  if (!words(query).length) {
    return [];
  }
  return state.model.events.filter(e => state.found.has(e.id)).sort((a, b) => a.start.localeCompare(b.start)).map(e => ({event: e, hidden: !eventVisible(e)}));
}

export function parseDate(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(s || '');
  if (!m) {
    return null;
  }
  return new Date(+m[1], m[2] - 1, +m[3]);
}

export function formatDate(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

export function addDays(date, n) {
  const d = parseDate(date);
  d.setDate(d.getDate() + n);
  return formatDate(d);
}

export function today() {
  return state.model.today;
}

export function monthOf(date) {
  return date.slice(0, 7);
}

export function shiftMonth(month, n) {
  const d = parseDate(month + '-01');
  d.setMonth(d.getMonth() + n);
  return formatDate(d).slice(0, 7);
}

export function weekStart(date) {
  return addDays(date, -parseDate(date).getDay());
}

export function eventDates(e) {
  return e.dates;
}

export function spansDays(e) {
  return Boolean(e.end) && e.end.slice(0, 10) !== e.start.slice(0, 10);
}

const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
const longDayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric', year: 'numeric'});
const shortDayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric', year: 'numeric'});
const monthDayFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});
const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long', year: 'numeric'});
const timeFormat = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});

export function dayLabel(date) {
  return dayFormat.format(parseDate(date));
}

export function longDayLabel(date) {
  return longDayFormat.format(parseDate(date));
}

export function shortDayLabel(date) {
  return shortDayFormat.format(parseDate(date));
}

export function weekdayLong(date) {
  return parseDate(date).toLocaleDateString('en-US', {weekday: 'long'});
}

export function monthLabel(month) {
  return monthFormat.format(parseDate(month + '-01'));
}

export function weekdayShort(date) {
  return parseDate(date).toLocaleDateString('en-US', {weekday: 'short'});
}

export function clock(hhmm) {
  const [h, m] = hhmm.split(':').map(Number);
  const out = timeFormat.format(new Date(2000, 0, 1, h, m));
  return out.length < 8 ? ' ' + out : out;
}

export function timeRange(from, to) {
  const a = clock(from);
  const b = clock(to).trimStart();
  if (a.slice(-2) === b.slice(-2)) {
    return `${a.slice(0, -3)}–${b}`;
  }
  return `${a}–${b}`;
}

export function whenLine(e) {
  if (e.allDay) {
    return daysLine(e);
  }
  if (spansDays(e)) {
    return timeLine(e);
  }
  return `${daysLine(e)} · ${timeLine(e)}`;
}

export function daysLine(e) {
  const start = parseWhen(e.start);
  if (spansDays(e)) {
    return `${monthDayFormat.format(start.date)} – ${monthDayFormat.format(parseWhen(e.end).date)}`;
  }
  return dayFormat.format(start.date);
}

export function timeLine(e, date) {
  return timeColumn(e, date).trimStart();
}

export function startTime(e, date) {
  if (e.allDay) {
    return 'All day';
  }
  if (spansDays(e) && date !== e.start.slice(0, 10)) {
    return timeLine(e, date);
  }
  return clock(e.start.slice(11)).trimStart();
}

export function timeColumn(e, date) {
  if (e.allDay) {
    return 'All day';
  }
  if (e.end === e.start) {
    return clock(e.start.slice(11));
  }
  if (spansDays(e)) {
    const first = e.start.slice(0, 10);
    const last = e.end.slice(0, 10);
    if (date === first) {
      return `From ${clock(e.start.slice(11)).trimStart()}`;
    }
    if (date === last) {
      return `Until ${clock(e.end.slice(11)).trimStart()}`;
    }
    if (date) {
      return 'All day';
    }
    const at = w => `${dayFormat.format(w.date)} ${clock(w.date.toTimeString().slice(0, 5)).trimStart()}`;
    return `${at(parseWhen(e.start))} – ${at(parseWhen(e.end))}`;
  }
  return timeRange(e.start.slice(11), e.end.slice(11));
}

export function eventPath(e) {
  if (e.address) {
    return '/e/' + encodeURIComponent(e.address);
  }
  return '/e/' + e.id.split('/').map(encodeURIComponent).join('/');
}

export function dayType(id) {
  return state.model.dayTypes.find(d => d.id === id) || null;
}

export function dayTypeName(id) {
  const type = dayType(id);
  return type ? type.name : '';
}

export function plan(date, classrooms) {
  const byClassroom = state.model.days[date];
  if (!byClassroom) {
    return [];
  }
  const groups = [];
  for (const c of classrooms || selectedClassrooms()) {
    const id = byClassroom[c];
    if (!id) {
      continue;
    }
    let group = groups.find(g => g.id === id);
    if (!group) {
      const type = dayType(id);
      group = {id, name: type.name, type, classrooms: []};
      groups.push(group);
    }
    group.classrooms.push(c);
  }
  const order = state.model.dayTypes.map(d => d.id);
  groups.sort((a, b) => order.indexOf(a.id) - order.indexOf(b.id));
  return groups;
}

export function isSchoolDay(date) {
  return Boolean(state.model.days[date]);
}

export function groupWords(group) {
  return group.classrooms.length === selectedClassrooms().length ? group.name : `${group.name} (${group.classrooms.length})`;
}

export function specials(date) {
  return plan(date).filter(g => g.type.role !== 'regular');
}

export function scheduleOn() {
  const schedule = roleTag('schedule');
  return !schedule || selectedTags().includes(schedule);
}

export function colorOf(classroom) {
  return state.model.colors[classroom] || '';
}

export function eventColors(e) {
  if (!e.classrooms.length || e.classrooms.length === classroomNames().length) {
    return [];
  }
  const out = [];
  for (const c of e.classrooms) {
    const color = colorOf(c);
    let entry = out.find(o => o.color === color);
    if (!entry) {
      entry = {color, classrooms: []};
      out.push(entry);
    }
    entry.classrooms.push(c);
  }
  return out;
}

export function eventTint(e) {
  const first = eventColors(e).find(c => c.color);
  if (first) {
    return first.color;
  }
  if (e.source === 'celebrate') {
    return '#d94a7c';
  }
  if (e.source === 'team') {
    return '#7b56c9';
  }
  return '#3b7dc4';
}

export function audienceWords(e) {
  if (!e.classrooms.length || e.classrooms.length === classroomNames().length) {
    return ['Everyone'];
  }
  const out = [];
  for (const band of bands()) {
    const mine = band.classrooms.filter(c => e.classrooms.includes(c.name));
    if (!mine.length) {
      continue;
    }
    if (mine.length === band.classrooms.length && band.classrooms.length > 1) {
      out.push(band.name);
    } else {
      out.push(...mine.map(c => c.name));
    }
  }
  return out;
}

export function sourceWords(e) {
  switch (e.source) {
    case 'google':
      return "From the school's Google Calendar";
    case 'pdf':
      return "From the school's year calendar (PDF)";
    case 'celebrate':
      return 'A Helios Celebrate party';
    case 'team':
      return 'An HCA-Team event';
  }
  return 'Added by the community';
}

export function linkedApp(e) {
  return e.source === 'celebrate' ? 'celebrate' : 'team';
}

export function linkURL(e) {
  return appOrigin(linkedApp(e)) + e.link;
}

export function answerOf(e) {
  return e.me.answer || '';
}

export function isHidden(e) {
  return answerOf(e) === 'hidden';
}

export function isGray(e) {
  const word = answerOf(e);
  return word === 'hidden' || word === 'no';
}

const answerGroups = {yes: 'rsvp_yes', no: 'rsvp_no', hidden: 'hidden_for'};

export async function answer(e, word) {
  const batch = Object.values(e.myRows).filter(Boolean).map(id => ({delete: id}));
  if (word) {
    batch.push({insert: 'MEMBER', row: {group: e.groups[answerGroups[word]], person: me().id, member: 'yes', added_by: me().id}});
  }
  if (batch.length) {
    await api('POST', '/api/q', {batch});
  }
  await loadModel();
  Object.assign(e, event(e.id) || {});
}

export function eventImage(e) {
  if (e.image) {
    return e.image;
  }
  for (const id of e.tags) {
    const tag = state.model.tags.find(t => t.id === id);
    if (tag && tag.imageUrl) {
      return tag.imageUrl;
    }
  }
  return '/brand/default-header.jpg';
}

export function isParty(e) {
  return e.source === 'celebrate';
}

export function myEvents() {
  const day = today();
  const upcoming = state.model.events.filter(e => eventDates(e)[eventDates(e).length - 1] >= day);
  const pending = e => e.pending && !e.declined && e.addedBy === me().id;
  return {
    hosted: upcoming.filter(e => e.hosted && !pending(e)),
    pending: upcoming.filter(pending),
    waiting: upcoming.filter(e => !e.hosted && e.invited && !answerOf(e)),
    going: upcoming.filter(e => !e.hosted && answerOf(e) === 'yes'),
  };
}
