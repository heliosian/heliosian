// The model as the server rendered it for the viewer, plus the page's own
// choices: which classrooms and tags are showing, remembered per browser, the
// search words, and which month the grid is open to. A null filter list means
// the viewer's own classrooms, or every tag.
import {appOrigin} from '/toolbar.js';

export const state = {model: null, filters: readFilters(), query: '', month: ''};

function readFilters() {
  try {
    const raw = JSON.parse(localStorage.getItem('calendar.filters') || '{}');
    return {classrooms: Array.isArray(raw.classrooms) ? raw.classrooms : null, tags: Array.isArray(raw.tags) ? raw.tags : null};
  } catch (err) {
    return {classrooms: null, tags: null};
  }
}

function saveFilters() {
  try {
    localStorage.setItem('calendar.filters', JSON.stringify(state.filters));
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

export function classroomNames() {
  return state.model.classrooms.map(c => c.name);
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

// selectedClassrooms is the filter in force: the viewer's choice, else their
// own classrooms, else - for someone with none - every classroom.
export function selectedClassrooms() {
  if (state.filters.classrooms) {
    return state.filters.classrooms;
  }
  return myClassrooms().length ? myClassrooms() : classroomNames();
}

export function setClassrooms(list) {
  state.filters.classrooms = list;
  saveFilters();
}

export function toggleClassroom(name) {
  const current = selectedClassrooms();
  setClassrooms(current.includes(name) ? current.filter(c => c !== name) : classroomNames().filter(c => c === name || current.includes(c)));
}

export function selectedTags() {
  return state.filters.tags || tagNames();
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

// dayTypeMatches is the search over the schedule: a day type stays on the
// page while every word typed is in its name.
export function dayTypeMatches(name, query) {
  const hay = name.toLowerCase();
  return words(query).every(w => hay.includes(w));
}

export function eventsOn(date) {
  return (byDate.get(date) || []).filter(eventVisible).filter(e => matches(e, state.query));
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
const monthDayFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});
const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long', year: 'numeric'});
const timeFormat = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});

export function dayLabel(date) {
  return dayFormat.format(parseDate(date));
}

export function longDayLabel(date) {
  return longDayFormat.format(parseDate(date));
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

// timeLine is an event's hours. For hours that run across days it is both
// ends with their days - or, given the date of the row it sits in, what that
// day sees of it: "From 4:00 PM", "All day", "Until 12:00 PM".
export function timeLine(e, date) {
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
  return plan(date).filter(g => g.name !== 'Regular' && dayTypeMatches(g.name, state.query));
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
      return "From the school's calendar feed";
    case 'pdf':
      return "From the school's year calendar";
    case 'celebrate':
      return 'A Helios Celebrate party';
    case 'team':
      return 'An HCA-Team event';
  }
  return 'Added by the community';
}

// linkURL is a linked event's page on the app that runs it, on this page's
// own tier.
export function linkURL(e) {
  return appOrigin(e.source) + e.link;
}

const callWords = {available: 'Get tickets', waitlist: 'Join the waitlist', 'sold-out': 'Sold out', open: 'Join', full: 'Full'};

// isParty says which app runs a linked event: a Celebrate party, else an
// HCA-Team event - the school's own event when one is folded into it.
export function isParty(e) {
  return e.source === 'celebrate';
}

// mineWords is where the viewer's household stands with a linked event, as
// the row and the page say it in place of the way in.
export function mineWords(e) {
  if (e.mine === 'waitlisted') {
    return 'Waitlisted';
  }
  if (e.mine === 'going') {
    return isParty(e) ? "You're going" : 'Signed up';
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
