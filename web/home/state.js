import {batch, query, me as whoAmI} from '/data.js';
import {eventPath} from '/dayrows.js';

export const state = {model: null};

const pad = n => String(n).padStart(2, '0');

export function isAdmin() {
  return Boolean(state.model && state.model.allowances.includes('home.configure'));
}

export function tagLabelsOf(rule) {
  return rule.tagLabels || rule.tags || [];
}

export function linkCategories() {
  return state.model.categories.filter(c => c.style !== 'events');
}

export function feed(id) {
  return state.model.calendars.find(c => c.id === id);
}

export function lastOfMonth(ym) {
  const [y, m] = ym.split('-').map(Number);
  return `${ym}-${pad(new Date(y, m, 0).getDate())}`;
}

export function shiftMonth(ym, by) {
  const [y, m] = ym.split('-').map(Number);
  const d = new Date(y, m - 1 + by, 1);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}`;
}

function eventsPath(from, to, calendar) {
  return `/api/events?from=${from}&to=${to}&calendar=${encodeURIComponent(calendar)}`;
}

function upcomingPath(today, calendar) {
  return eventsPath(today, lastOfMonth(shiftMonth(today.slice(0, 7), 1)), calendar);
}

function monthPath(ym, calendar) {
  return eventsPath(ym + '-01', lastOfMonth(ym), calendar);
}

export function eventCard(e) {
  const linked = e.app !== 'when';
  return {
    id: e.id, title: e.title, path: eventPath(e), start: e.start.slice(0, 10), startAt: e.start, endAt: e.end || e.start, allDay: e.allDay,
    dates: e.dates, image: '/open/banner/' + encodeURIComponent(e.id), imageApp: 'when',
    link: linked ? e.path : '', linkApp: linked ? e.app : '',
    call: linked ? e.call || '' : '', mine: linked ? e.mine || '' : '', availability: linked ? e.availability || '' : '', people: linked ? e.minePeople || [] : [],
    answer: e.me.answer || '', going: e.going || '', invited: Boolean(e.invited), hosted: Boolean(e.hosted), cancelled: Boolean(e.cancelled),
  };
}

function cards(read, name) {
  return read.result[name].map(id => eventCard(read.get(id)));
}

export async function readMonth(ym, calendar) {
  const read = await query(monthPath(ym, calendar));
  return {month: ym, calendar, events: read.result.map(id => eventCard(read.get(id)))};
}

export async function readUpcoming(calendar) {
  const read = await query(upcomingPath(state.model.today, calendar));
  return {calendar, events: read.result.map(id => eventCard(read.get(id)))};
}

function teamItem(a) {
  return {title: a.title, under: a.under, start: a.day, timing: a.dayTiming, position: a.me.position, note: a.wants, path: a.path, image: a.picture};
}

function birthdayItem(read, b, fallback) {
  const person = read.follow(b, 'person') || {};
  const assignee = read.follow(b, 'assignee');
  const donation = read.follow(b, 'donation');
  const chosen = donation && read.follow(donation, 'charity');
  return {
    charity: chosen ? chosen.name : fallback.name, chosen: Boolean(chosen),
    name: person.fullName || b.email, photo: person.photoUrl || '', path: b.path, stage: b.stage, next: b.next,
    newsletter: b.newsletterDate, urgency: b.urgency ? b.urgency.when : '', assignee: assignee ? assignee.fullName : b.assignedTo || '',
  };
}

function options(read) {
  return {
    classrooms: read.result.classrooms.map(id => read.get(id).name),
    grades: read.result.grades.map(id => read.get(id).name),
    tags: read.result.tags.map(id => read.get(id)).map(t => ({key: `tag:${t.id}`, name: t.me.mine ? t.name : `${t.name} (${t.ownerName}'s)`})),
    lists: read.result.magicTags.map(read.get).filter(t => !t.archived)
      .map(t => ({key: t.key, name: t.name, kind: t.kind, parent: (t.parent && read.get(t.parent) || {}).key || ''}))
      .sort((a, b) => a.name.localeCompare(b.name)),
  };
}

export async function loadModel() {
  const [read, viewer] = await Promise.all([batch({
    settings: '/api/home-settings?include=viewer',
    categories: '/api/link-categories?include=links',
    apps: '/api/apps',
    widgets: '/api/home-widgets',
    feeds: '/api/calendar-feeds',
    waiting: '/api/events?waiting',
    team: '/api/team-settings?include=mine,needed,priority',
    school: '/api/school-emails',
    birthday: '/api/birthday-settings?include=mine.person,mine.donation.charity,all.person,all.assignee,all.donation.charity,default-charity',
  }), whoAmI()]);
  const admin = viewer.allowances.includes('home.configure');
  const today = read.now.slice(0, 10);
  const calendars = read.result.feeds.map(read.get);
  const calendar = calendars[0].id;
  const later = await batch({
    month: monthPath(today.slice(0, 7), calendar),
    upcoming: upcomingPath(today, calendar),
    parties: `/api/events?app=celebrate&from=${today}`,
    ...(admin ? {
      classrooms: '/api/classrooms',
      grades: '/api/grades?enrolled',
      tags: '/api/tags',
      magicTags: '/api/magic-tags?include=parent',
    } : {}),
  });
  const s = read.get(read.result.settings[0]);
  const person = read.follow(s, 'viewer') || {};
  const name = person.fullName || viewer.email;
  const team = read.get(read.result.team[0]);
  const birthday = read.get(read.result.birthday[0]);
  const fallback = read.follow(birthday, 'default-charity');
  state.model = {
    user: {email: viewer.email, name, initial: name[0].toUpperCase(), photoUrl: person.heroPhotoUrl},
    allowances: viewer.allowances,
    imageSearch: s.imageSearch,
    today,
    categories: read.result.categories.map(read.get).map(c => ({
      ...c,
      rules: c.rules || [],
      links: read.follow(c, 'links').map(l => ({...l, category: c.id, rules: l.rules || []})),
    })),
    apps: read.result.apps.map(read.get).filter(a => a.key !== 'home').map(a => ({...a, emails: a.emails || [], rules: a.rules || [], forMe: a.me.listed})),
    widgets: read.result.widgets.map(read.get).map(w => ({...w, rules: w.rules || []})),
    calendars,
    month: {month: today.slice(0, 7), calendar, events: cards(later, 'month')},
    upcoming: {calendar, events: cards(later, 'upcoming')},
    parties: cards(later, 'parties'),
    waiting: read.result.waiting.map(read.get).sort((a, b) => a.start.localeCompare(b.start)).map(e => ({title: e.title, start: e.start, path: eventPath(e)})),
    team: {mine: read.follow(team, 'mine').map(teamItem), open: read.follow(team, 'needed').map(teamItem), priority: read.follow(team, 'priority').map(teamItem)},
    school: read.result.school.map(read.get),
    birthday: {
      admin: viewer.allowances.includes('birthday.configure'),      mine: read.follow(birthday, 'mine').map(b => birthdayItem(read, b, fallback)),
      all: read.follow(birthday, 'all').map(b => birthdayItem(read, b, fallback)),
    },
    options: admin ? {...options(later), roles: s.roles, relations: s.relations} : null,
  };
}
