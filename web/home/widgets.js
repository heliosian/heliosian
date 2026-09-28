import {state, isAdmin} from './state.js';
import {appOrigin} from '/appswitch.js';
import {searchInput} from '/shell.js';
import {api} from '/api.js';
import {el, svg} from '/elements.js';
import {calendarMark, calendarMenu, dropdown, audienceWords} from './cards.js';
import {openWidgetAudience, moveWidget} from './edit.js';
import {dayBar, dayChip, dayRow, eventRow, standing} from '/dayrows.js';
import {rsvpPanel} from '/rsvps.js';

function parseDate(date) {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(y, m - 1, d);
}

const pad = n => String(n).padStart(2, '0');

function tint(event) {
  return event.linkApp === 'celebrate' ? 'is-celebrate' : event.linkApp === 'team' ? 'is-team' : 'is-school';
}

function clock(time) {
  const [h, m] = time.split(':').map(Number);
  return new Date(2000, 0, 1, h, m).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
}

function startTime(event, date) {
  const [day, time] = event.startAt.split(' ');
  if (!time || day !== date) {
    return 'All day';
  }
  return clock(time);
}

function sortKey(event, date) {
  const [day, time] = event.startAt.split(' ');
  return !time || day !== date ? '' : time;
}

function shown(event) {
  return event.answer !== 'no' && event.answer !== 'hidden';
}

let picked = null;
let base = null;

function currentMonth() {
  if (base !== state.model.calendar) {
    base = state.model.calendar;
    picked = null;
  }
  return picked || base;
}

async function pick(token) {
  const month = currentMonth();
  try {
    picked = await api('GET', '/api/apps/calendar?month=' + month.today.slice(0, 7) + '&calendar=' + encodeURIComponent(token));
  } catch {
    return;
  }
  nextMonth = null;
  renderWidgets(searchInput().value);
}

function calendarPick(month) {
  const cal = state.model.upcomingCalendar;
  const list = (cal && cal.calendars) || [];
  if (!list.length) {
    return null;
  }
  const current = list.find(c => c.token === month.calendar) || list[0];
  const chosen = list.find(c => c.token === cal.default) || list[0];
  const wrap = el('div', 'category-calendar widget-calendar');
  const toggle = el('button', 'category-calendar-toggle');
  toggle.type = 'button';
  toggle.title = current.locked ? 'The calendar\u2019s own view, for everyone' : 'The saved calendar these events come from';
  toggle.append(calendarMark(current), el('span', '', current.name), svg('chevron-right'));
  const menu = calendarMenu(list, current, chosen, c => pick(c.token));
  dropdown(toggle, menu);
  wrap.append(toggle, menu);
  return wrap;
}

let nextMonth = null;

function allEvents(month) {
  const events = [...(month.events || [])];
  if (nextMonth && nextMonth.month !== month.month && nextMonth.calendar === month.calendar) {
    for (const e of nextMonth.events || []) {
      if (!events.some(x => x.id === e.id)) {
        events.push(e);
      }
    }
  }
  return events.filter(shown);
}

function dayInfo(month, day) {
  const here = (month.days || {})[day];
  if (here || !nextMonth || nextMonth.calendar !== month.calendar) {
    return here || {};
  }
  return (nextMonth.days || {})[day] || {};
}

let nextAsked = null;

async function fetchNext(month) {
  const [y, m] = month.month.split('-').map(Number);
  const after = m === 12 ? `${y + 1}-01` : `${y}-${pad(m + 1)}`;
  const want = after + '|' + (month.calendar || '');
  if (nextAsked === want) {
    return;
  }
  nextAsked = want;
  try {
    nextMonth = await api('GET', '/api/apps/calendar?month=' + after + '&calendar=' + encodeURIComponent(month.calendar || ''));
  } catch {
    return;
  }
  renderWidgets(searchInput().value);
}

function dayGroups(items, dayOf, order) {
  const byDay = new Map();
  for (const item of items) {
    for (const day of [dayOf(item)].flat()) {
      if (!day) {
        continue;
      }
      if (!byDay.has(day)) {
        byDay.set(day, []);
      }
      byDay.get(day).push(item);
    }
  }
  return [...byDay.keys()].sort().map(d => ({day: d, rows: byDay.get(d).sort((a, b) => order(a, d).localeCompare(order(b, d)))}));
}

const pageRows = 3;

const shownCounts = new Map();

const fills = new Map();

function shownCount(name) {
  return (shownCounts.get(name) || pageRows) + (fills.get(name) || 0);
}

function widgetFoot(name, total, seeAll) {
  const foot = el('footer', 'widget-foot');
  foot.dataset.list = name;
  foot.dataset.total = String(total);
  const count = Math.min(shownCount(name), total);
  const buttons = el('div', 'wg-more-row');
  const act = (words, next, dir) => {
    const b = el('button', 'wg-more ' + dir);
    b.type = 'button';
    b.append(el('span', '', words), svg('chevron-right'));
    b.addEventListener('click', () => {
      shownCounts.set(name, next);
      renderWidgets(searchInput().value);
    });
    buttons.append(b);
  };
  if (count < total) {
    act(`Show ${Math.min(pageRows, total - count)} more`, count + pageRows, 'is-more');
  }
  if ((shownCounts.get(name) || pageRows) > pageRows) {
    act('Show less', pageRows, 'is-less');
  }
  if (buttons.children.length) {
    foot.append(buttons);
  }
  if (seeAll) {
    foot.append(moreLink(seeAll.words || 'See all', seeAll.href));
  }
  return foot.children.length ? foot : null;
}

function grouped(name, groups, bar, row) {
  const out = [];
  const limit = shownCount(name);
  let n = 0;
  for (const g of groups) {
    if (n >= limit) {
      break;
    }
    const list = el('ol', 'wg-list');
    for (const r of g.rows) {
      if (n >= limit) {
        break;
      }
      list.append(row(r, g, n++));
    }
    out.push(bar(g), list);
  }
  return out;
}

function widgetTitle(app, words) {
  const title = el('h2', 'widget-title');
  const icon = el('img', 'widget-icon');
  const mark = ((state.model.apps || []).find(a => a.key === app) || {}).mark;
  icon.src = `/brand/apps/${app}.png` + (mark ? `?v=${mark}` : '');
  icon.alt = '';
  title.append(icon, el('span', '', words));
  return title;
}

function moreLink(words, href) {
  const a = el('a', 'widget-more');
  a.href = href;
  a.append(el('span', '', words), svg('chevron-right'));
  return a;
}

let rsvps = [];
let rsvpsFor = null;

async function fetchRsvps() {
  rsvpsFor = state.model;
  try {
    rsvps = (await api('GET', '/api/apps/rsvp')).waiting || [];
  } catch {
    return;
  }
  renderWidgets(searchInput().value);
}

function whenWidget() {
  const month = currentMonth();
  if (!month || !month.today) {
    return null;
  }
  if (rsvpsFor !== state.model) {
    fetchRsvps();
  }
  const events = allEvents(month);
  const card = el('article', 'widget widget-when');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('when', 'Upcoming'));
  const choose = calendarPick(month);
  if (choose) {
    head.append(choose);
  }
  card.append(head);
  if (rsvps.length) {
    card.append(rsvpPanel(rsvps, appOrigin('when')));
  }
  const groups = dayGroups(events, event => event.dates.filter(d => d >= month.today), sortKey);
  if (!groups.length) {
    card.append(el('p', 'wg-empty', 'Nothing coming up on the calendar.'));
  }
  card.append(...grouped('when', groups,
    g => dayBar(g.day, (dayInfo(month, g.day).kinds || []).map(k => dayChip(k.name, k.words))),
    (event, g) => eventRow(event, {base: appOrigin('when'), time: startTime(event, g.day), className: tint(event)})));
  const total = groups.reduce((n, g) => n + g.rows.length, 0);
  card.append(widgetFoot('when', total, {href: appOrigin('when')}));
  fetchNext(month);
  return card;
}

let team = null;
let teamFor = null;

async function fetchTeam() {
  teamFor = state.model;
  try {
    team = await api('GET', '/api/apps/team');
  } catch {
    return;
  }
  renderWidgets(searchInput().value);
}

let teamChip = 'all';

const teamTones = ['is-red', 'is-blue', 'is-gold', 'is-green'];

function teamWidget() {
  if (teamFor !== state.model) {
    team = null;
    fetchTeam();
  }
  if (!team) {
    return null;
  }
  const card = el('article', 'widget widget-team');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('team', 'Team'));
  const chips = el('div', 'wg-chips');
  const body = el('div');
  const foot = el('div', 'widget-foot-slot');
  const paint = () => {
    chips.replaceChildren();
    const priority = (team.priority || []).length > 0;
    if (!priority && teamChip === 'priority') {
      teamChip = 'all';
    }
    for (const [key, label] of [['all', 'All'], ['priority', 'Priority'], ['mine', 'Mine']].filter(([key]) => key !== 'priority' || priority)) {
      const chip = el('button', 'wg-chip tone-' + key + (teamChip === key ? ' is-on' : ''), label);
      chip.type = 'button';
      const count = key === 'priority' ? team.priority.length : key === 'mine' ? team.mine.length : null;
      if (count !== null) {
        chip.append(el('span', 'wg-chip-count', String(count)));
      }
      chip.addEventListener('click', () => {
        teamChip = key;
        renderWidgets(searchInput().value);
      });
      chips.append(chip);
    }
    body.replaceChildren();
    const mine = teamChip === 'mine';
    const items = mine ? team.mine : teamChip === 'priority' ? team.priority || [] : team.open;
    const end = widgetFoot('team-' + teamChip, items.length, {href: appOrigin('team')});
    foot.replaceChildren(...(end ? [end] : []));
    if (!items.length) {
      body.append(el('p', 'wg-empty', mine ? 'You\u2019re not signed up for anything coming up.' : 'Nothing needs hands just now.'));
      return;
    }
    body.append(pictureList('team-' + teamChip, items, (item, i) => {
      const href = appOrigin('team') + item.path;
      const day = item.start ? parseDate(item.start).toLocaleDateString('en-US', {weekday: 'short', month: 'short', day: 'numeric'}) : item.timing;
      const line = mine ? [day, item.under, item.position] : [day, item.note];
      let pill = null;
      if (!mine) {
        pill = el('a', 'wg-pill', 'Sign up');
        pill.href = href;
      }
      return pictureRow({
        href, image: item.image ? appOrigin('team') + item.image : '', title: item.title,
        line: line.filter(Boolean).join(' \u00b7 '), pill, tone: teamTones[i % teamTones.length],
      });
    }));
  };
  paint();
  card.append(head, chips, body, foot);
  return card;
}

let parties = null;
let partiesFor = null;

async function fetchParties() {
  partiesFor = state.model;
  try {
    parties = (await api('GET', '/api/apps/celebrate')).parties || [];
  } catch {
    return;
  }
  renderWidgets(searchInput().value);
}

let partyChip = 'upcoming';

function partiesUnder(chip) {
  if (chip === 'mine') {
    return parties.filter(p => p.mine);
  }
  if (chip === 'upcoming') {
    return parties.filter(p => p.mine || p.availability === 'available');
  }
  return parties;
}

function partyPill(p) {
  const held = standing(p);
  if (held) {
    return held;
  }
  if (p.call && (p.availability === 'available' || p.availability === 'waitlist')) {
    const pill = el('a', 'wg-pill', p.call);
    pill.href = appOrigin('celebrate') + p.link;
    return pill;
  }
  return null;
}

function pictureRow({href, image, title, line, pill, tone}) {
  const row = el('li', 'wg-party' + (tone ? ' ' + tone : ''));
  row.dataset.row = '';
  const pic = el('span', 'wg-party-pic');
  if (image) {
    const img = el('img');
    img.src = image;
    img.alt = '';
    img.loading = 'lazy';
    pic.append(img);
  } else {
    pic.classList.add('is-letter');
    pic.append(el('span', '', title.trim().charAt(0).toUpperCase()));
  }
  const text = el('div', 'wg-text');
  const name = el('a', 'wg-title', title);
  name.href = href;
  text.append(name);
  if (line) {
    text.append(el('div', 'wg-sub', line));
  }
  if (pill) {
    const under = el('div', 'wg-party-pill');
    under.append(pill);
    text.append(under);
  }
  const go = el('a', 'wg-go');
  go.href = href;
  go.setAttribute('aria-label', 'Open ' + title);
  go.append(svg('chevron-right'));
  row.append(pic, text, go);
  row.addEventListener('click', e => {
    if (!e.target.closest('a')) {
      location.href = href;
    }
  });
  return row;
}

function partyRow(p) {
  const day = parseDate(p.start).toLocaleDateString('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
  return pictureRow({
    href: appOrigin('celebrate') + p.link, image: p.image ? appOrigin(p.imageApp) + p.image : '',
    title: p.title, line: day + ' · ' + startTime(p, p.start), pill: partyPill(p),
  });
}

function pictureList(name, items, row) {
  const list = el('ol', 'wg-parties');
  items.slice(0, shownCount(name)).forEach((item, i) => list.append(row(item, i)));
  return list;
}

function celebrateWidget() {
  if (partiesFor !== state.model) {
    parties = null;
    fetchParties();
  }
  if (!parties) {
    return null;
  }
  const card = el('article', 'widget widget-celebrate');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('celebrate', 'Celebrate'));
  const chips = el('div', 'wg-chips');
  const body = el('div');
  const foot = el('div', 'widget-foot-slot');
  const paint = () => {
    chips.replaceChildren();
    for (const [key, label] of [['upcoming', 'Upcoming'], ['all', 'All'], ['mine', 'Mine']]) {
      const chip = el('button', 'wg-chip' + (partyChip === key ? ' is-on' : ''), label);
      chip.type = 'button';
      chip.addEventListener('click', () => {
        partyChip = key;
        renderWidgets(searchInput().value);
      });
      chips.append(chip);
    }
    body.replaceChildren();
    const items = partiesUnder(partyChip);
    const end = widgetFoot('celebrate-' + partyChip, items.length, {href: appOrigin('celebrate')});
    foot.replaceChildren(...(end ? [end] : []));
    if (!items.length) {
      const empty = {mine: 'Your household has no tickets to anything coming up.', upcoming: 'Nothing with tickets to be had or held just now.'}[partyChip] || 'No parties coming up.';
      body.append(el('p', 'wg-empty', empty));
      return;
    }
    body.append(pictureList('celebrate-' + partyChip, items, partyRow));
  };
  paint();
  card.append(head, chips, body, foot);
  return card;
}

let school = null;
let schoolFor = null;

async function fetchSchool() {
  schoolFor = state.model;
  try {
    school = (await api('GET', '/api/apps/school')).emails || [];
  } catch {
    return;
  }
  renderWidgets(searchInput().value);
}

const listNames = {
  newsletter: 'Newsletter', parentsandstaff: 'Parents & staff', parentsonly: 'Parents', parentsandstudents: 'Parents & students',
  community: 'Community', parents: 'All parents', newstudentfamilies: 'New families', 'new.parents': 'New families',
};

function sentTo(email) {
  const cap = w => w.charAt(0).toUpperCase() + w.slice(1);
  if (email.kind !== 'list') {
    if (!email.audience || email.audience === 'Everyone') {
      return 'All families';
    }
    const named = email.audience.split(',').map(r => r.trim()).filter(Boolean);
    const isGrade = n => /^Grade \d+$/.test(n) || n === 'Kindergarten';
    const and = list => list.length > 1 ? list.slice(0, -1).join(', ') + ' & ' + list[list.length - 1] : list.join('');
    const rooms = named.filter(n => !isGrade(n));
    const grades = named.filter(isGrade);
    const numbers = grades.filter(g => g !== 'Kindergarten').map(g => g.slice(6));
    const gradeWords = [];
    if (grades.includes('Kindergarten')) {
      gradeWords.push(numbers.length ? 'K' : 'Kindergarten');
    }
    if (numbers.length) {
      gradeWords.push(...numbers);
    }
    const gradePart = !grades.length ? '' : grades.length === 1 ? grades[0] : `Grades ${and(gradeWords)}`;
    return [rooms.length ? and(rooms) : '', gradePart].filter(Boolean).join(' \u00b7 ');
  }
  if (listNames[email.channel]) {
    return listNames[email.channel];
  }
  const [room, who] = email.channel.split('.');
  if (who) {
    return `${cap(room)} ${who}`;
  }
  return room.split('and').map(cap).join(' & ');
}

const schoolTones = ['is-teal', 'is-lime', 'is-pink', 'is-blue'];

let schoolOpen;

function emailRow(email, n, open) {
  const date = parseDate(email.date);
  const title = el('button', 'wg-title', email.title);
  title.type = 'button';
  title.setAttribute('aria-expanded', String(open));
  const to = el('span', 'wg-email-to', sentTo(email));
  to.title = sentTo(email);
  const body = el('div', 'wg-email-body');
  if (email.points.length) {
    const points = el('ul', 'wg-points');
    for (const p of email.points) {
      points.append(el('li', '', p));
    }
    body.append(points);
  } else {
    body.append(el('div', 'wg-sub', 'Key points on their way.'));
  }
  const day = date.toLocaleDateString('en-US', {weekday: 'long', month: 'long', day: 'numeric'});
  const ask = el('a', 'wg-ask');
  ask.href = appOrigin('ask') + '/?q=' + encodeURIComponent(`What should I know from the school email "${email.title}" sent ${day}?`);
  ask.append(el('span', '', 'Ask about this'), svg('chevron-right'));
  body.append(ask);
  const row = dayRow('wg-email ' + schoolTones[n % schoolTones.length], {title, chips: [to], time: email.time ? clock(email.time) : '', after: body}, e => {
    if (e.target.closest('.wg-email-body')) {
      return;
    }
    const opening = !row.classList.contains('is-open');
    for (const other of row.closest('.widget').querySelectorAll('.wg-email')) {
      other.classList.remove('is-open');
      other.querySelector('.wg-title').setAttribute('aria-expanded', 'false');
    }
    row.classList.toggle('is-open', opening);
    title.setAttribute('aria-expanded', String(opening));
    schoolOpen = opening ? email.key : null;
  });
  row.classList.toggle('is-open', open);
  return row;
}

let schoolType = null;

function typePick() {
  const counts = new Map();
  for (const e of school) {
    counts.set(sentTo(e), (counts.get(sentTo(e)) || 0) + 1);
  }
  const wrap = el('div', 'category-calendar widget-calendar wg-type');
  const toggle = el('button', 'category-calendar-toggle');
  toggle.type = 'button';
  toggle.title = 'Show one kind of email';
  toggle.append(el('span', '', schoolType || 'All'), svg('chevron-right'));
  const menu = el('div', 'category-calendar-menu');
  menu.hidden = true;
  for (const [type, n] of [[null, school.length], ...counts]) {
    const item = el('button', 'category-calendar-item' + (type === schoolType ? ' is-on' : ''));
    item.type = 'button';
    item.append(el('span', '', type || 'All'), el('span', 'category-calendar-tail wg-type-count', String(n)));
    item.addEventListener('click', () => {
      menu.hidden = true;
      schoolType = type;
      renderWidgets(searchInput().value);
    });
    menu.append(item);
  }
  dropdown(toggle, menu);
  wrap.append(toggle, menu);
  return wrap;
}

function schoolWidget() {
  if (schoolFor !== state.model) {
    school = null;
    fetchSchool();
  }
  if (!school) {
    return null;
  }
  const card = el('article', 'widget widget-school');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('ask', 'Inbox'));
  card.append(head);
  if (!school.length) {
    card.append(el('p', 'wg-empty', 'No school email in the last two weeks.'));
    return card;
  }
  if (schoolType && !school.map(sentTo).includes(schoolType)) {
    schoolType = null;
  }
  head.append(typePick());
  const shown = !schoolType ? school : school.filter(e => sentTo(e) === schoolType);
  const name = 'school-' + (schoolType || 'all');
  const groups = dayGroups(shown, e => e.date, e => e.time || '').reverse();
  const first = groups.length ? groups[0].rows[0].key : null;
  const openKey = schoolOpen === undefined || (schoolOpen && !shown.some(e => e.key === schoolOpen)) ? first : schoolOpen;
  card.append(...grouped(name, groups, g => dayBar(g.day, [el('span', 'wg-day-count', `${g.rows.length} ${g.rows.length === 1 ? 'email' : 'emails'}`)]), (e, g, n) => emailRow(e, n, e.key === openKey)));
  const foot = widgetFoot(name, shown.length, null);
  if (foot) {
    card.append(foot);
  }
  return card;
}

const widgetMakers = {when: whenWidget, team: teamWidget, celebrate: celebrateWidget, school: schoolWidget};

const widgetNames = {when: 'Upcoming', team: 'Team', celebrate: 'Celebrate', school: 'Inbox'};

function adminTools(card, key) {
  const head = card.querySelector('.widget-head');
  const v = (state.model.widgets || {})[key] || {forMe: true, rules: []};
  if (v.forMe === false) {
    head.querySelector('.widget-title').append(el('span', 'hidden-badge', 'Hidden'));
  }
  if ((v.rules || []).length) {
    head.querySelector('.widget-title').append(el('span', 'hidden-badge audience-badge', audienceWords(v.rules)));
  }
  const edit = el('button', 'category-edit widget-edit');
  edit.type = 'button';
  edit.title = 'Who sees this widget';
  edit.setAttribute('aria-label', `Who sees ${widgetNames[key]}`);
  edit.append(svg('edit'));
  edit.addEventListener('click', () => openWidgetAudience(key, widgetNames[key]));
  head.insertBefore(edit, head.children[1] || null);
  const order = widgetOrder();
  const at = order.indexOf(key);
  const moves = el('span', 'widget-moves');
  for (const [by, glyph, words] of [[-1, '\u2039', 'Move earlier'], [1, '\u203a', 'Move later']]) {
    const b = el('button', 'category-edit widget-move', glyph);
    b.type = 'button';
    b.title = words;
    b.setAttribute('aria-label', `${words}: ${widgetNames[key]}`);
    b.disabled = by < 0 ? at <= 0 : at >= order.length - 1;
    b.addEventListener('click', () => moveWidget(key, by));
    moves.append(b);
  }
  head.insertBefore(moves, edit.nextSibling);
}

function widgetOrder() {
  const makers = Object.keys(widgetMakers);
  const set = (state.model.widgetOrder || []).filter(k => makers.includes(k));
  return [...set, ...makers.filter(k => !set.includes(k))];
}

function fitWidgets(query) {
  let changed = false;
  for (const card of document.querySelector('#widgets').children) {
    const foot = card.querySelector('.widget-foot[data-list]');
    const rows = card.querySelectorAll('[data-row]');
    if (!foot || !rows.length) {
      continue;
    }
    const name = foot.dataset.list;
    const left = Number(foot.dataset.total) - rows.length;
    if (left <= 0) {
      continue;
    }
    const first = card.querySelector('.wg-day, [data-row]').getBoundingClientRect();
    const last = rows[rows.length - 1].getBoundingClientRect();
    const per = (last.bottom - first.top) / rows.length;
    const room = foot.getBoundingClientRect().top - last.bottom - 24;
    const more = Math.min(left, Math.floor(room / per));
    if (per > 0 && more > 0) {
      fills.set(name, (fills.get(name) || 0) + more);
      changed = true;
    }
  }
  if (changed) {
    renderWidgets(query, true);
  }
}

let resizing = null;
window.addEventListener('resize', () => {
  clearTimeout(resizing);
  resizing = setTimeout(() => renderWidgets(searchInput().value), 150);
});

export function renderWidgets(query = '', fitted = false) {
  if (!fitted) {
    fills.clear();
  }
  const root = document.querySelector('#widgets');
  root.replaceChildren();
  root.hidden = Boolean(query.trim());
  if (root.hidden) {
    return;
  }
  const admin = isAdmin();
  for (const key of widgetOrder()) {
    const make = widgetMakers[key];
    const forMe = ((state.model.widgets || {})[key] || {}).forMe !== false;
    if (!forMe && !admin) {
      continue;
    }
    const widget = make();
    if (!widget) {
      continue;
    }
    if (admin) {
      adminTools(widget, key);
    }
    root.append(widget);
  }
  root.hidden = !root.children.length;
  if (!fitted && !root.hidden) {
    fitWidgets(query);
  }
}
