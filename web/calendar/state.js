// The model as the server rendered it for the viewer, plus the page's own
// choices: which classrooms and tags are showing, remembered per browser, the
// search words, which day the rail shows (the last one opened, today until
// then), and which month the grid is open to. A null filter list means the
// viewer's own classrooms, or every tag.
import {appOrigin} from '/toolbar.js';

// activeFeed is the token of the saved calendar the viewer last opened from
// the rail (or that the filters matched on load): Save Calendar saves the
// filters back onto it.
const remembered = readFilters();
export const state = {model: null, filters: {classrooms: remembered.classrooms, tags: remembered.tags}, query: '', day: '', month: '', activeFeed: remembered.active};

// setActiveFeed marks the saved calendar the viewer is working from.
export function setActiveFeed(token) {
  state.activeFeed = token;
  saveFilters();
}

function readFilters() {
  try {
    const raw = JSON.parse(localStorage.getItem('calendar.filters') || '{}');
    return {classrooms: Array.isArray(raw.classrooms) ? raw.classrooms : null, tags: Array.isArray(raw.tags) ? raw.tags : null, active: typeof raw.active === 'string' ? raw.active : ''};
  } catch (err) {
    return {classrooms: null, tags: null, active: ''};
  }
}

// The filters and the saved calendar being worked from are kept together,
// so a reload picks up where the viewer was.
function saveFilters() {
  try {
    localStorage.setItem('calendar.filters', JSON.stringify({...state.filters, active: state.activeFeed}));
  } catch (err) {
    // A browser that refuses storage just forgets the choice on reload.
  }
}

const byId = new Map();
const byDate = new Map();

export function applyModel(model) {
  state.model = model;
  byId.clear();
  byDate.clear();
  for (const e of model.events) {
    byId.set(e.id, e);
    for (const date of eventDates(e)) {
      if (!byDate.has(date)) {
        byDate.set(date, []);
      }
      byDate.get(date).push(e);
    }
  }
  // Every filter is checked against the live vocabulary, so a classroom
  // renamed since the choice was made drops out rather than hiding everything.
  const names = classroomNames();
  if (state.filters.classrooms) {
    state.filters.classrooms = state.filters.classrooms.filter(c => names.includes(c));
  }
  const tags = tagNames();
  if (state.filters.tags) {
    state.filters.tags = state.filters.tags.filter(t => tags.includes(t));
  }
}

export function me() {
  return state.model.user;
}

export function event(id) {
  return byId.get(id) || null;
}

// defaultFeedName is the name a saved calendar starts with: the viewer's
// own first name on it, "Sam's Heliosian Calendar" - the name their
// calendar app lists its feed by.
export function defaultFeedName() {
  const first = (me().name || '').trim().split(/\s+/)[0];
  return unusedFeedName(first ? `${first}\u2019s Heliosian Calendar` : 'Heliosian Calendar');
}

// unusedFeedName is a name none of the viewer's saved calendars has: the
// one given, or it with the first free number after it - "… 2", "… 3".
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

// feedURL is a feed's address for a calendar app; webcalURL the same as
// webcal://, which Apple's and most others open straight into a
// subscription.
export function feedURL(token) {
  return `${location.origin}/feed/${token}.ics`;
}

export function webcalURL(token) {
  return `webcal://${location.host}/feed/${token}.ics`;
}

// feedClassrooms and feedTags are a saved calendar's filter as the
// calendar's own: every classroom or tag where it carries no filter.
export function feedClassrooms(f) {
  return f.classrooms.length ? f.classrooms : classroomNames();
}

export function feedTags(f) {
  return f.tags.length ? f.tags : tagNames();
}

function sameSet(a, b) {
  return a.length === b.length && a.every(x => b.includes(x));
}

// showsFeed says whether the calendar's filters are one saved calendar's
// exactly; savedAlready whether they are any saved calendar's.
export function showsFeed(f) {
  return sameSet(selectedClassrooms(), feedClassrooms(f)) && sameSet(selectedTags(), feedTags(f));
}

export function savedAlready() {
  return (state.model.feeds || []).some(showsFeed);
}

// activeFeed is the saved calendar the viewer is working from: the one the
// filters are exactly, else the one last opened from the rail if it still
// exists, else none.
export function activeFeed() {
  const feeds = state.model.feeds || [];
  const shown = feeds.find(showsFeed);
  if (shown) {
    if (state.activeFeed !== shown.token) {
      setActiveFeed(shown.token);
    }
    return shown;
  }
  return feeds.find(f => f.token === state.activeFeed) || null;
}

export function classroomNames() {
  return state.model.classrooms.map(c => c.name);
}

// tagGroups are the tags by the group the sheet files them under, in the
// order the groups first occur; the tags with no group come last as one
// unnamed group.
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

export function tagNames() {
  return state.model.tags.map(t => t.name);
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

// savedView is the view this person kept, on the server, for every device
// and for Heliosian's Upcoming Events - or nothing.
export function savedView() {
  return me().saved || null;
}

// defaultFeed is the viewer's default calendar: the first of their saved
// calendars, as the rail lists them; null with none.
export function defaultFeed() {
  return (state.model.feeds || [])[0] || null;
}

// defaultClassrooms are the classrooms on for someone who has not chosen
// today: their default calendar's, else the ones they saved, else their
// own, else - for someone with none - every classroom.
export function defaultClassrooms() {
  const first = defaultFeed();
  if (first) {
    return feedClassrooms(first);
  }
  const saved = savedView();
  if (saved && saved.classrooms.length) {
    return saved.classrooms;
  }
  return myClassrooms().length ? myClassrooms() : classroomNames();
}

// selectedClassrooms is the filter in force: the viewer's choice, else the
// default.
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

// defaultTags are the categories on for someone who has not chosen today:
// the ones they saved, else the ones the Tags tab (and Admin Tools) mark
// on by default.
export function defaultTags() {
  const first = defaultFeed();
  if (first) {
    return feedTags(first);
  }
  const saved = savedView();
  if (saved) {
    return saved.tags;
  }
  return state.model.tags.filter(t => t.default).map(t => t.name);
}

export function selectedTags() {
  return state.filters.tags || defaultTags();
}

export function setTags(list) {
  state.filters.tags = list;
  saveFilters();
}

export function toggleTag(name) {
  const current = selectedTags();
  setTags(current.includes(name) ? current.filter(t => t !== name) : tagNames().filter(t => t === name || current.includes(t)));
}

export function filtersAreDefault() {
  return !state.filters.classrooms && !state.filters.tags;
}

export function resetFilters() {
  state.filters = {classrooms: null, tags: null};
  saveFilters();
}

export function categoryTags(e) {
  const names = classroomNames();
  return e.tags.filter(t => !names.includes(t));
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
  return classroomsAdmit(e) && tagsAdmit(e);
}

// hiddenMatches counts the events the search words find under a tag that is
// switched off - what the reader would see if they turned it on.
export function hiddenMatches(tag) {
  if (!state.query) {
    return 0;
  }
  return state.model.events.filter(e => e.tags.includes(tag) && matches(e, state.query) && classroomsAdmit(e) && !tagsAdmit(e)).length;
}

// hiddenClassroomMatches is the same for a classroom that is switched off.
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

// matches is the search: every word typed is found somewhere in the title,
// the description, the place, the tags, or the hidden keywords.
export function matches(e, query) {
  const hay = `${e.title} ${e.description || ''} ${e.location || ''} ${e.tags.join(' ')} ${(e.keywords || []).join(' ')} ${e.dayType || ''}`.toLowerCase();
  return words(query).every(w => hay.includes(w));
}

// dayTypeMatches is the search over the schedule: a day type lights up
// while every word typed is in its name.
export function dayTypeMatches(name, query) {
  const hay = name.toLowerCase();
  return words(query).every(w => hay.includes(w));
}

export function eventsOn(date) {
  return (byDate.get(date) || []).filter(eventVisible);
}

// isMatch says the search words find this event - the highlight the page
// gives it while there are words.
export function isMatch(e) {
  return Boolean(state.query) && matches(e, state.query);
}

// searchResults are every event the words find, whatever the filters say,
// in date order - what the search box lists. hidden marks one the filters
// keep off the page.
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

// weekStart is the Sunday on or before a date, the way a US wall calendar
// starts its rows.
export function weekStart(date) {
  return addDays(date, -parseDate(date).getDay());
}

// eventDates lists every day an event touches, so a camp-out that runs from
// Friday evening to Sunday noon sits on all three days, not just the first.
export function eventDates(e) {
  const out = [];
  const start = e.start.slice(0, 10);
  const end = (e.end || e.start).slice(0, 10);
  for (let d = start; d <= end; d = addDays(d, 1)) {
    out.push(d);
  }
  return out;
}

// spansDays says whether an event's end falls on a later day than its start.
export function spansDays(e) {
  return Boolean(e.end) && e.end.slice(0, 10) !== e.start.slice(0, 10);
}

export function parseWhen(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})(?: (\d{2}):(\d{2}))?$/.exec(s || '');
  if (!m) {
    return null;
  }
  return {date: new Date(+m[1], m[2] - 1, +m[3], m[4] ? +m[4] : 0, m[5] ? +m[5] : 0), hasTime: Boolean(m[4])};
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

// shortDayLabel is the rail's date line: "Sun, Sep 13, 2026".
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

// clock is one wall-clock time as every page writes it: "8:15 AM". A
// one-digit hour is padded with a figure space, so times line up in a column
// and a day's hours keep their place when the digits change.
export function clock(hhmm) {
  const [h, m] = hhmm.split(':').map(Number);
  const out = timeFormat.format(new Date(2000, 0, 1, h, m));
  return out.length < 8 ? ' ' + out : out;
}

// timeRange is two times as one span, the AM or PM written once when both
// share it: "8:00–8:15 AM", "11:30 AM–12:15 PM".
export function timeRange(from, to) {
  const a = clock(from);
  const b = clock(to).trimStart();
  if (a.slice(-2) === b.slice(-2)) {
    return `${a.slice(0, -3)}–${b}`;
  }
  return `${a}–${b}`;
}

// whenLine is an event's date line: "Mon Sep 7" for a day, "Sep 9 – Sep 11"
// for a span, "Thu Sep 24 · 4:00–6:00 PM" with hours, and "Fri Oct 2 4:00 PM
// – Sun Oct 4 12:00 PM" for hours that run across days.
export function whenLine(e) {
  if (e.allDay) {
    return daysLine(e);
  }
  if (spansDays(e)) {
    return timeLine(e);
  }
  return `${daysLine(e)} · ${timeLine(e)}`;
}

// daysLine is the day or days alone: "Mon Sep 7", or "Sep 9 – Sep 11".
export function daysLine(e) {
  const start = parseWhen(e.start);
  if (spansDays(e)) {
    return `${monthDayFormat.format(start.date)} – ${monthDayFormat.format(parseWhen(e.end).date)}`;
  }
  return dayFormat.format(start.date);
}

// timeLine is an event's hours, for a line of its own. For hours that run
// across days it is both ends with their days - or, given the date of the
// row it sits in, what that day sees of it: "From 4:00 PM", "All day",
// "Until 12:00 PM".
export function timeLine(e, date) {
  return timeColumn(e, date).trimStart();
}

// timeColumn is the same words padded as clock pads them, so a column of
// them lines up - the event list's.
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
  return '/events/' + e.id.split('/').map(encodeURIComponent).join('/');
}

export function dayType(name) {
  return state.model.dayTypes.find(d => d.name === name) || null;
}

// plan is the day as the selected classrooms have it: the classrooms grouped
// by day type, in the day types' order, or nothing outside the school year.
export function plan(date, classrooms) {
  const byClassroom = state.model.days[date];
  if (!byClassroom) {
    return [];
  }
  const groups = [];
  for (const c of classrooms || selectedClassrooms()) {
    const name = byClassroom[c];
    if (!name) {
      continue;
    }
    let group = groups.find(g => g.name === name);
    if (!group) {
      group = {name, type: dayType(name), classrooms: []};
      groups.push(group);
    }
    group.classrooms.push(c);
  }
  const order = state.model.dayTypes.map(d => d.name);
  groups.sort((a, b) => order.indexOf(a.name) - order.indexOf(b.name));
  return groups;
}

export function isSchoolDay(date) {
  return Boolean(state.model.days[date]);
}

export function specials(date) {
  return plan(date).filter(g => g.name !== 'Regular');
}

const scheduleTag = 'Schedule';

// scheduleOn says the Upcoming panel lists the days that are not regular:
// the Schedule tag is on, or the sheet has no such tag to switch them off.
export function scheduleOn() {
  return !tagNames().includes(scheduleTag) || selectedTags().includes(scheduleTag);
}

// nextSpecials walks forward from a date to the next n days that are not
// regular for the selected classrooms.
export function nextSpecials(from, n) {
  const out = [];
  const dates = Object.keys(state.model.days).filter(d => d > from).sort();
  for (const date of dates) {
    const groups = specials(date);
    if (groups.length) {
      out.push({date, groups});
      if (out.length >= n) {
        break;
      }
    }
  }
  return out;
}

const knownTypes = {'Regular': 'dt-regular', 'No School': 'dt-no-school', 'Early Dismissal': 'dt-early', 'No Aftercare': 'dt-no-aftercare'};

export function dayTypeClass(name) {
  return knownTypes[name] || 'dt-other';
}

export function colorOf(classroom) {
  return state.model.colors[classroom] || '';
}

// eventColors is who an event is for as colors: one per distinct classroom
// color among its classrooms, each naming the classrooms it stands for, and
// none for an event that is everyone's.
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

// eventTint is the one color an event wears in the month's pills, the
// rail's timeline and the upcoming rows: its first classroom color, or for
// an event that is everyone's, a color for where it comes from - Celebrate's
// parties pink, HCA's events purple, the school's own blue.
export function eventTint(e) {
  const first = eventColors(e).find(c => c.color);
  if (first) {
    return first.color;
  }
  if (e.tags.includes('Celebrate')) {
    return '#d94a7c';
  }
  if (e.tags.includes('HCA')) {
    return '#7b56c9';
  }
  return '#3b7dc4';
}

// audienceWords compresses an event's classrooms: every classroom is
// "Everyone", both classrooms of a band are the band, the rest are named.
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

// linkURL is a linked event's page on the app that runs it, on this page's
// own tier.
// linkedApp is the app that runs a linked event: Celebrate for a party,
// HCA-Team for everything else with a way in - including the school's own
// listing of an HCA event, which keeps the school as its source once the
// two are folded together.
function linkedApp(e) {
  return e.source === 'celebrate' ? 'celebrate' : 'team';
}

export function linkURL(e) {
  return appOrigin(linkedApp(e)) + e.link;
}

// answerOf is the viewer's word on an event: yes, no, hidden, or nothing.
export function answerOf(e) {
  return (me().answers || {})[e.id] || '';
}

export function isHidden(e) {
  return answerOf(e) === 'hidden';
}

// isGray says whether the month and the timeline show an event in plain
// gray rather than a pill: one the viewer hid, or said no to.
export function isGray(e) {
  const word = answerOf(e);
  return word === 'hidden' || word === 'no';
}

// answer tells the calendar the viewer's word on an event and keeps it in
// the model at once, so the page redraws without a reload: a yes puts the
// event under Going, as the server files it, and taking the yes back lifts
// it unless a ticket keeps it there.
export async function answer(e, word) {
  const res = await fetch('/api/calendar/rsvp', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: e.id, answer: word})});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  const answers = {...(me().answers || {})};
  if (word) {
    answers[e.id] = word;
  } else {
    delete answers[e.id];
  }
  me().answers = answers;
  const going = word === 'yes' || e.mine === 'going';
  if (going && !e.tags.includes('Going')) {
    e.tags = [...e.tags, 'Going'];
  } else if (!going) {
    e.tags = e.tags.filter(t => t !== 'Going');
  }
}

// eventImage is the picture across the top of an event's page: the event's
// own, where the app that runs it has one (fetched from that app, on this
// page's own tier); else the image of the first of its tags that has one,
// in the order the event carries them; else the calendar's own header.
export function eventImage(e) {
  if (e.image) {
    return e.link ? appOrigin(linkedApp(e)) + e.image : e.image;
  }
  for (const name of e.tags) {
    const tag = state.model.tags.find(t => t.name === name);
    if (tag && tag.imageUrl) {
      return tag.imageUrl;
    }
  }
  return '/brand/default-header.jpg';
}

const callWords = {available: 'Get tickets', waitlist: 'Join the waitlist', 'sold-out': 'Sold out', open: 'Join', full: 'Full'};

// isParty says which app runs a linked event: a Celebrate party, else an
// HCA-Team event - the school's own event when one is folded into it.
export function isParty(e) {
  return e.source === 'celebrate';
}

// mineWords is where the viewer's household stands with a linked event, as
// the row and the page say it in place of the way in.
// mineWords is the household's standing in words: the viewer's own as
// "you", another member's by name - "Sam is going", "Sam and Alex are
// waitlisted", "Sam signed up".
export function mineWords(e) {
  const who = e.mineWho || [];
  const names = who.length > 1 ? who.slice(0, -1).join(', ') + ' and ' + who[who.length - 1] : who[0] || '';
  const verb = who.length > 1 ? 'are' : 'is';
  if (e.mine === 'waitlisted') {
    return names ? `${names} ${verb} waitlisted` : 'Waitlisted';
  }
  if (e.mine === 'going') {
    if (isParty(e)) {
      return names ? `${names} ${verb} going` : "You're going";
    }
    return names ? `${names} signed up` : 'Signed up';
  }
  return '';
}

// call is what a linked event's row and page say about signing up: the
// household's own standing first, else the way in, else nothing once it has
// passed or is closed.
export function call(e) {
  return mineWords(e) || callWords[e.availability] || '';
}

export function calendarLink(e) {
  const from = parseWhen(e.start);
  const to = parseWhen(e.end);
  const stamp = d => `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}`
    + (from.hasTime ? `T${String(d.getHours()).padStart(2, '0')}${String(d.getMinutes()).padStart(2, '0')}00` : '');
  let until = to && e.end !== e.start ? to.date : new Date(from.date.getTime() + 60 * 60 * 1000);
  if (!from.hasTime) {
    until = new Date((to ? to.date : from.date).getTime() + 24 * 60 * 60 * 1000);
  }
  const params = new URLSearchParams({
    action: 'TEMPLATE',
    text: e.title,
    dates: `${stamp(from.date)}/${stamp(until)}`,
    details: [e.description, location.origin + eventPath(e)].filter(Boolean).join('\n\n'),
    location: e.location || '',
  });
  return `https://calendar.google.com/calendar/render?${params}`;
}
