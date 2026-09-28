import {appOrigin} from '/appswitch.js';
import {api} from '/api.js';
import {parseWhen} from '/datecard.js';

const remembered = readFilters();
export const state = {model: null, filters: {classrooms: remembered.classrooms, tags: remembered.tags}, query: '', day: '', month: '', activeFeed: remembered.active, appliedCalendar: ''};

export function isAdmin() {
  return Boolean(state.model && state.model.user.isAdmin);
}

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

export function applyModel(model) {
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

export function postedAndHosting(e) {
  return e.source === 'sheet' && e.addedBy === state.model.user.email && !e.posterLeft;
}

export function event(id) {
  if (byId.has(id)) {
    return byId.get(id);
  }
  const [source, rest] = id.split('/', 2);
  if (source === 'team' && rest) {
    return state.model.events.find(e => e.linkedId === rest && e.link) || null;
  }
  return null;
}

export async function fetchEvent(id) {
  try {
    const e = await api('GET', '/api/when/event?id=' + encodeURIComponent(id));
    byId.set(e.id, e);
    if (e.address) {
      byId.set(e.address, e);
    }
    return e;
  } catch (err) {
    return null;
  }
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

export function feedURL(token) {
  return `${location.origin}/open/feed/${token}.ics`;
}

export function webcalURL(token) {
  return `webcal://${location.host}/open/feed/${token}.ics`;
}

export const MY_HELIOSIAN = 'my-heliosian';

export function myHeliosian() {
  const home = me().home || {};
  return {token: MY_HELIOSIAN, name: home.name || 'My Heliosian', emoji: home.emoji || '', locked: true, position: home.position || 0, classrooms: [], tags: []};
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
}

export function categoryTags(e) {
  const rooms = classroomIds();
  return e.tags.filter(t => !rooms.includes(t));
}

function overlaps(a, b) {
  return a.some(x => b.includes(x));
}

function classroomsAdmit(e) {
  return !e.classrooms.length || overlaps(e.classrooms, selectedClassrooms());
}

function tagsAdmit(e) {
  const categories = categoryTags(e);
  return !categories.length || overlaps(categories, selectedTags());
}

export function eventVisible(e) {
  if (e.invited || (answerOf(e) === 'yes' && selectedTags().includes(roleTag('going')))) {
    return true;
  }
  return classroomsAdmit(e) && tagsAdmit(e);
}

export function hiddenMatches(tag) {
  if (!state.query) {
    return 0;
  }
  return state.model.events.filter(e => e.tags.includes(tag) && matches(e, state.query) && classroomsAdmit(e) && !tagsAdmit(e)).length;
}

export function hiddenClassroomMatches(classroom) {
  if (!state.query) {
    return 0;
  }
  return state.model.events.filter(e => e.classrooms.includes(classroom) && matches(e, state.query) && tagsAdmit(e) && !classroomsAdmit(e)).length;
}

export function visibleEvents() {
  return state.model.events.filter(eventVisible);
}

function words(query) {
  return (query || '').toLowerCase().split(/\s+/).filter(Boolean);
}

export function matches(e, query) {
  const hay = `${e.title} ${e.description || ''} ${e.location || ''} ${e.tags.map(tagName).join(' ')} ${(e.keywords || []).join(' ')} ${dayTypeName(e.dayType)}`.toLowerCase();
  return words(query).every(w => hay.includes(w));
}

export function dayTypeMatches(name, query) {
  const hay = name.toLowerCase();
  return words(query).every(w => hay.includes(w));
}

export function eventsOn(date) {
  return (byDate.get(date) || []).filter(eventVisible);
}

export function isMatch(e) {
  return Boolean(state.query) && matches(e, state.query);
}

export function searchResults(query) {
  if (!words(query).length) {
    return [];
  }
  return state.model.events.filter(e => matches(e, query)).sort((a, b) => a.start.localeCompare(b.start)).map(e => ({event: e, hidden: !eventVisible(e)}));
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
  if (e.tags.includes(roleTag('celebrate'))) {
    return '#d94a7c';
  }
  if (e.tags.includes(roleTag('hca'))) {
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
  return (me().answers || {})[e.id] || '';
}

export function isHidden(e) {
  return answerOf(e) === 'hidden';
}

export function isGray(e) {
  const word = answerOf(e);
  return word === 'hidden' || word === 'no';
}

export async function answer(e, word) {
  await api('POST', '/api/when/rsvp', {id: e.id, answer: word});
  const answers = {...(me().answers || {})};
  if (word) {
    answers[e.id] = word;
  } else {
    delete answers[e.id];
  }
  me().answers = answers;
  const going = word === 'yes' || e.mine === 'going';
  const tag = roleTag('going');
  if (going && !e.tags.includes(tag)) {
    e.tags = [...e.tags, tag];
  } else if (!going) {
    e.tags = e.tags.filter(t => t !== tag);
  }
}

export function eventImage(e) {
  if (e.image) {
    return e.link ? '/open/banner/' + e.id.split('/').map(encodeURIComponent).join('/') : e.image;
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
  const pending = e => e.pending && !e.declined && e.addedBy === me().email;
  return {
    hosted: upcoming.filter(e => e.hosted && !pending(e)),
    pending: upcoming.filter(pending),
    waiting: upcoming.filter(e => !e.hosted && e.invited && !answerOf(e)),
    going: upcoming.filter(e => !e.hosted && answerOf(e) === 'yes'),
  };
}
