import {queries, rowsOf} from '/rail.js';
import {teamQueries, teamWidget} from './team.js';
import {partyQueries, partiesWidget} from './parties.js';
import {birthdayWork} from '/birthdays.js';
import {loadCalendar, buildCalendar, builtinWords, wordsFor, wordsCondition, matchingQuery, roomsOn, by} from '/calendar.js';

export const state = {model: null};

const pad = n => String(n).padStart(2, '0');

export const layouts = ['1/3 1/3 1/3', '2/3 1/3', '1/3 2/3', '1/2 1/2', '1'];

const mailDays = 14;

export function isAdmin() {
  return Boolean(state.model && state.model.admin);
}

export function holdsApps(category) {
  return category.style === 'apps' || category.style === 'apps-grid';
}

export function inGrid(category) {
  return category.style === 'cards' || category.style === 'apps-grid';
}

export function widgetKey(category) {
  return 'category:' + category.id;
}

export function categoryOf(key) {
  return state.model.categories.find(c => widgetKey(c) === key);
}

export function linkCategories() {
  const place = c => state.model.widgets.findIndex(w => w.key === widgetKey(c));
  return state.model.categories.slice().sort((a, b) => place(a) - place(b));
}

export function fraction(share) {
  const [top, bottom] = share.split('/').map(Number);
  return top / (bottom || 1);
}

export function clock(time) {
  const [h, m] = time.split(':').map(Number);
  return new Date(2000, 0, 1, h, m).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
}

export function widgetGrid() {
  const layout = state.model.layout;
  const keys = state.model.widgets.map(w => w.key);
  const rows = [];
  let at = 0;
  for (let i = 0; at < keys.length || i < layout.length; i++) {
    const row = layout[i] || layouts[0];
    const columns = row.split(' ');
    rows.push({layout: row, set: i < layout.length, slots: columns.map((share, n) => ({share, key: keys[at + n] || ''}))});
    at += columns.length;
  }
  return rows;
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

function dateOf(d) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function eventCard(e) {
  const linked = e.source === 'celebrate' || e.source === 'team';
  return {
    id: e.id, title: e.title, path: e.whenHref, start: e.start.slice(0, 10), startAt: e.start, endAt: e.end || e.start, allDay: e.allDay,
    dates: e.dates, link: linked ? e.link : '', linkApp: linked ? (e.source === 'celebrate' ? 'celebrate' : 'team') : '',
    call: linked ? e.call || '' : '', mine: linked ? e.mine || '' : '', availability: linked ? e.availability || '' : '',
    answer: e.me.answer || '', going: e.going || '', invited: Boolean(e.invited), hosted: Boolean(e.hosted), cancelled: Boolean(e.cancelled),
    event: e,
  };
}

function eventsIn(calendar, from, to) {
  return state.model.events.filter(e => calendar.matching.has(e.id) && e.dates.some(d => d >= from && d <= to)).map(eventCard);
}

export function readMonth(ym, calendar) {
  return {month: ym, calendar, events: eventsIn(feed(calendar), ym + '-01', lastOfMonth(ym))};
}

export function readUpcoming(calendar) {
  const today = state.model.today;
  return {calendar, events: eventsIn(feed(calendar), today, lastOfMonth(shiftMonth(today.slice(0, 7), 1)))};
}

function daysFor(cal, rooms) {
  const out = {};
  for (const date of Object.keys(cal.dayMap)) {
    const groups = roomsOn(cal, date, rooms);
    out[date] = groups.filter(g => g.type.role !== 'regular').map(g => ({
      name: g.name, words: g.classrooms.length === rooms.length ? g.name : `${g.name} · ${g.classrooms.join(', ')}`,
    }));
  }
  return out;
}

function calendarsOf(cal) {
  const all = cal.classrooms.map(c => c.name);
  const builtinRooms = cal.home.classrooms.length ? cal.home.classrooms : cal.mineRooms.length ? cal.mineRooms : all;
  const home = cal.home.id ? cal.saved.find(s => s.id === cal.home.id) : null;
  const list = [{
    id: home ? home.id : 'my-heliosian', token: home ? home.token : 'my-heliosian', name: cal.home.name || 'My Heliosian', emoji: cal.home.emoji || '', locked: true,
    condition: home ? home.condition : wordsCondition(builtinWords(cal)), rooms: builtinRooms,
  }];
  for (const s of cal.saved.filter(s => !s.locked)) {
    list.push({id: s.id, token: s.token, name: s.name, emoji: s.emoji, locked: false, condition: s.condition || wordsCondition(wordsFor(cal, s.classrooms.length ? s.classrooms : all, s.tags.length ? s.tags : cal.tags.map(t => t.id))), rooms: s.classrooms.length ? s.classrooms : all});
  }
  for (const c of list) {
    c.days = daysFor(cal, c.rooms);
  }
  return list;
}

const homeQueries = since => ({
  homeAdmin: '(from APP (where (= key "home") (admin_of "home")) (columns key))',
  superAdmin: '(from APP (where (= key "home") (super_admin)) (columns key))',
  homePhoto: '(from PHOTO (where (= person @viewer) ready) (order order asc) (columns person))',
  layout: '(from SETTING (where (= app "home") (= key "layout")) (columns value))',
  apps: '(from APP (order order asc) (columns key name subtitle visible_to order))',
  appsShown: '(from APP (where (in visible_to (select EFFECTIVE_MEMBER.group (= person @viewer)))) (columns key))',
  widgets: '(from WIDGET (order order asc) (columns key group name icon style descriptions sidebar visible_to order))',
  widgetsShown: '(from WIDGET (where (in visible_to (select EFFECTIVE_MEMBER.group (= person @viewer)))) (columns key))',
  links: '(from GROUP (where (in parent (select WIDGET.group))) (order order asc) (columns parent name description url visible_to order))',
  linksShown: '(from GROUP (where (in parent (select WIDGET.group)) (in visible_to (select EFFECTIVE_MEMBER.group (= person @viewer)))) (columns parent))',
  photos: '(from PHOTO (where ready (or (in group (select WIDGET.group)) (in group (select GROUP.id (in parent (select WIDGET.group)))))) (order order asc) (columns group))',
  audiences: '(from GROUP (where (or (in id (select WIDGET.visible_to)) (in id (select APP.visible_to)) (in id (select GROUP.visible_to (in parent (select WIDGET.group)))))) (columns name slug kind listed added_by))',
  audienceMembers: '(from MEMBER (where (= member "yes") (or (in group (select WIDGET.visible_to)) (in group (select APP.visible_to)) (in group (select GROUP.visible_to (in parent (select WIDGET.group)))))) (include person) (columns group person))',
  audienceRules: '(from RULE (where (or (in group (select WIDGET.visible_to)) (in group (select APP.visible_to)) (in group (select GROUP.visible_to (in parent (select WIDGET.group)))))) (order order asc) (columns group order exclude target replace_with within person search))',
  mail: `(from DOCUMENT (where (= kind "mail") (blank parent) (>= published "${since}")) (order published desc) (columns name published summary key_points repeats message))`,
  marks: '(from TODO_MARK (where (= person @viewer)) (columns todo state changed))',
  todos: `(from TODO @t (where (or (in document (select DOCUMENT.id (= kind "mail") (blank parent) (>= published "${since}"))) (in id (select TODO_MARK.todo (= person @viewer))))) (include document) (columns document point title summary details link due order))`,
  sentTo: `(from DOCUMENT_GROUP (where (= relation "sent_to") (or (in document (select DOCUMENT.id (= kind "mail") (blank parent) (>= published "${since}"))) (in document (select TODO.document (in id (select TODO_MARK.todo (= person @viewer))))))) (include group) (columns document group))`,
});

function linkSection(w) {
  return w.key === 'links' || w.key === 'apps';
}

function styleOf(w) {
  if (w.key === 'apps') {
    return w.style === 'grid' ? 'apps-grid' : 'apps';
  }
  return w.style === 'grid' ? 'cards' : 'tiles';
}

function homeModel(h, cal, everyone) {
  const ruleRows = by(rowsOf(h.audienceRules, 'RULE'), 'group');
  const audience = group => ({visibleTo: group || '', rules: group && group !== everyone ? ruleRows[group] || [] : []});
  // Only Heliosian's admins read rows not shown to them; anyone else reads an audience group they can't see as blank.
  const admin = h.homeAdmin.result.length > 0;
  const shown = (set, id) => !admin || set.has(id);
  const appsShown = new Set(h.appsShown.result);
  const apps = rowsOf(h.apps, 'APP').filter(a => a.key !== 'home').map(a => ({
    id: a.id, key: a.key, name: a.name, tagline: a.subtitle || '', order: a.order || '', forMe: shown(appsShown, a.id), mark: '', ...audience(a.visible_to),
  }));
  const photos = {};
  for (const p of rowsOf(h.photos, 'PHOTO')) {
    photos[p.group] = photos[p.group] || p.id;
  }
  const linksShown = new Set(h.linksShown.result);
  const links = by(rowsOf(h.links, 'GROUP'), 'parent');
  const widgetsShown = new Set(h.widgetsShown.result);
  const widgetRows = rowsOf(h.widgets, 'WIDGET');
  const categories = widgetRows.filter(linkSection).map(w => {
    const photo = photos[w.group] || '';
    return {
      id: w.id, group: w.group || '', title: w.name || '', emoji: photo ? 'image:' : w.icon || '', icon: w.icon || '', photo, imageUrl: photo ? `/api/blob/${photo}/image` : '',
      style: styleOf(w), descriptions: w.descriptions === 'Yes', ...audience(w.visible_to),
      links: (links[w.group] || []).map(g => ({
        id: g.id, category: w.id, title: g.name, description: g.description || '', url: g.url || '', order: g.order || '',
        photo: photos[g.id] || '', imageUrl: photos[g.id] ? `/api/blob/${photos[g.id]}/image` : '',
        visible: !admin || Boolean(g.visible_to), forMe: !g.visible_to || shown(linksShown, g.id), ...audience(g.visible_to),
      })),
    };
  });
  const widgets = widgetRows.map(w => ({
    id: w.id, key: linkSection(w) ? 'category:' + w.id : w.key, kind: w.key, sidebar: w.sidebar === 'Yes', order: w.order || '',
    me: {shown: shown(widgetsShown, w.id)}, ...audience(w.visible_to),
  }));
  const layoutRow = rowsOf(h.layout, 'SETTING')[0];
  const audiences = Object.fromEntries(rowsOf(h.audiences, 'GROUP').map(g => [g.id, g]));
  const audienceMembers = by(rowsOf(h.audienceMembers, 'MEMBER'), 'group');
  const people = h.audienceMembers.resources.PERSON || {};
  return {
    apps, categories, widgets, audiences, audienceRules: ruleRows, audienceMembers, people,
    layoutSetting: layoutRow ? layoutRow.id : '', layout: layoutRow ? layoutRow.value.split(';').map(s => s.trim()).filter(Boolean) : [],
  };
}

function sentToNames(h) {
  const groups = h.sentTo.resources.GROUP || {};
  const out = {};
  for (const link of rowsOf(h.sentTo, 'DOCUMENT_GROUP')) {
    const g = groups[link.group];
    if (g) {
      (out[link.document] = out[link.document] || []).push(g);
    }
  }
  return out;
}

function mailModel(h, today) {
  const sent = sentToNames(h);
  const to = id => (sent[id] || []).map(g => g.name).sort().join(' · ');
  const docs = h.todos.resources.DOCUMENT || {};
  const marks = Object.fromEntries(rowsOf(h.marks, 'TODO_MARK').map(m => [m.todo, m]));
  const since = dateOf(new Date(Date.now() - mailDays * 86400000));
  const todos = rowsOf(h.todos, 'TODO').map(t => {
    const doc = docs[t.document] || {};
    return {
      id: t.id, email: t.document, point: Number(t.point) || 0, title: t.title, summary: t.summary || '', details: t.details || '', link: t.link || '', due: t.due || '',
      order: t.order || '', mark: marks[t.id] || null, me: {state: marks[t.id] ? marks[t.id].state : ''},
      source: {title: doc.name || '', date: (doc.published || '').slice(0, 10), time: (doc.published || '').slice(11, 16), to: to(t.document)},
    };
  }).filter(t => {
    const current = t.due ? t.due >= today : t.source.date >= since;
    return current || t.me.state === 'saved';
  });
  todos.sort((a, b) => (a.due || '9999').localeCompare(b.due || '9999') || b.source.date.localeCompare(a.source.date) || a.title.localeCompare(b.title));
  const school = rowsOf(h.mail, 'DOCUMENT').filter(d => !d.message).map(d => {
    const repeats = {};
    for (const line of (d.repeats || '').split('\n')) {
      const [point, todo] = line.split(':').map(s => s.trim());
      if (point && todo) {
        repeats[Number(point)] = todo;
      }
    }
    return {
      id: d.id, key: d.id, title: d.name || '', date: (d.published || '').slice(0, 10), time: (d.published || '').slice(11, 16), to: to(d.id),
      summary: d.summary || '', points: (d.key_points || '').split('\n').map(s => s.trim()).filter(Boolean), repeats,
    };
  });
  return {school, todos};
}

function birthdayItem(sv) {
  return {
    charity: sv.charity, chosen: sv.donation, name: sv.name, photo: sv.photo, path: sv.path, stage: sv.stage, next: sv.next,
    newsletter: sv.newsletterDate, birthday: sv.birthdayThisYear, urgency: sv.urgency ? sv.urgency.when : '', assignee: sv.assignee,
  };
}

function birthdayLists(work) {
  const soon = new Set(work.issues);
  const mine = work.items.filter(sv => sv.next && sv.next.day && sv.next.step !== 'newsletter' && sv.assignedToId === work.viewer && soon.has(sv.newsletterDate))
    .sort((x, y) => x.next.day.localeCompare(y.next.day));
  const all = work.admin ? work.items.filter(sv => soon.has(sv.newsletterDate)).sort((x, y) => x.newsletterDate.localeCompare(y.newsletterDate)) : [];
  return {mine: mine.map(birthdayItem), all: all.map(birthdayItem)};
}

export async function readToDos() {
  const since = dateOf(new Date(Date.now() - mailDays * 86400000));
  const h = await queries({marks: homeQueries(since).marks, todos: homeQueries(since).todos, sentTo: homeQueries(since).sentTo, mail: homeQueries(since).mail});
  state.model.todos = mailModel(h, state.model.today).todos;
}

export async function loadModel() {
  const now = new Date();
  const today = dateOf(now);
  const since = dateOf(new Date(now.getTime() - mailDays * 86400000));
  const [a, data, work] = await Promise.all([loadCalendar(), queries({...teamQueries, ...partyQueries, ...homeQueries(since)}), birthdayWork()]);
  const cal = buildCalendar(a);
  const calendars = calendarsOf(cal);
  const asked = calendars.filter(c => c.condition);
  const matched = asked.length ? await queries(Object.fromEntries(asked.map((c, i) => ['match' + i, matchingQuery(c.condition)]))) : {};
  for (const c of calendars) {
    const i = asked.indexOf(c);
    c.matching = new Set(i >= 0 ? matched['match' + i].result : []);
  }
  const home = homeModel(data, cal, cal.everyone);
  const mail = mailModel(data, today);
  const photo = rowsOf(data.homePhoto, 'PHOTO')[0];
  const name = cal.me.name_show || cal.email;
  const upcoming = cal.events.filter(e => e.dates[e.dates.length - 1] >= today);
  state.model = {
    user: {id: cal.me.id, email: cal.email, name, initial: (name || '?')[0].toUpperCase(), photoUrl: photo ? `/api/blob/${photo.id}/thumbnail` : ''},
    admin: data.homeAdmin.result.length > 0,
    superAdmin: data.superAdmin.result.length > 0,
    imageSearch: true,
    everyone: cal.everyone,
    layout: home.layout,
    layoutSetting: home.layoutSetting,
    layouts,
    today,
    now: `${today} ${pad(now.getHours())}:${pad(now.getMinutes())}`,
    categories: home.categories,
    apps: home.apps,
    widgets: home.widgets,
    audiences: home.audiences,
    audienceRules: home.audienceRules,
    audienceMembers: home.audienceMembers,
    people: home.people,
    calendars,
    calendar: cal,
    events: cal.events,
    dayTypes: cal.dayTypes,
    parties: partiesWidget(data),
    waiting: upcoming.filter(e => !e.hosted && e.invited && !e.me.answer && !e.cancelled).sort((x, y) => x.start.localeCompare(y.start)).map(e => ({title: e.title, start: e.start, path: e.whenHref})),
    team: teamWidget(data, `${today} 00:00`),
    school: mail.school,
    todos: mail.todos,
    birthday: birthdayLists(work),
  };
  state.model.month = readMonth(today.slice(0, 7), calendars[0].id);
  state.model.upcoming = readUpcoming(calendars[0].id);
}
