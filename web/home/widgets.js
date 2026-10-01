import {state, isAdmin, feed, readUpcoming} from './state.js';
import {appOrigin} from '/appswitch.js';
import {searchInput} from '/shell.js';
import {el, svg} from '/elements.js';
import {calendarMark, calendarMenu, dropdown, shownTo} from './cards.js';
import {dayBar, dayChip, dayRow, eventRow, standing} from '/dayrows.js';
import {rsvpPanel} from '/rsvps.js';

function parseDate(date) {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(y, m - 1, d);
}

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

let picked = null;
let base = null;

function currentUpcoming() {
  if (base !== state.model.upcoming) {
    base = state.model.upcoming;
    picked = null;
  }
  return picked || base;
}

async function pick(calendar) {
  try {
    picked = await readUpcoming(calendar);
  } catch {
    return;
  }
  renderWidgets(searchInput().value);
}

function calendarPick(upcoming) {
  const list = state.model.calendars;
  const current = feed(upcoming.calendar);
  const wrap = el('div', 'category-calendar widget-calendar');
  const toggle = el('button', 'category-calendar-toggle');
  toggle.type = 'button';
  toggle.title = current.locked ? 'The calendar\u2019s own view, for everyone' : 'The saved calendar these events come from';
  toggle.append(calendarMark(current), el('span', '', current.name), svg('chevron-right'));
  const menu = calendarMenu(list, current, list[0], c => pick(c.id));
  dropdown(toggle, menu);
  wrap.append(toggle, menu);
  return wrap;
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
  const mark = (state.model.apps.find(a => a.key === app) || {}).mark;
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

function whenWidget() {
  const upcoming = currentUpcoming();
  const today = state.model.today;
  const days = feed(upcoming.calendar).days || {};
  const card = el('article', 'widget widget-when');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('when', 'Upcoming'), calendarPick(upcoming));
  card.append(head);
  if (state.model.waiting.length) {
    card.append(rsvpPanel(state.model.waiting, appOrigin('when')));
  }
  const groups = dayGroups(upcoming.events, event => event.dates.filter(d => d >= today), sortKey);
  if (!groups.length) {
    card.append(el('p', 'wg-empty', 'Nothing coming up on the calendar.'));
  }
  card.append(...grouped('when', groups,
    g => dayBar(g.day, (days[g.day] || []).map(k => dayChip(k.name, k.words))),
    (event, g) => eventRow(event, {base: appOrigin('when'), time: startTime(event, g.day), className: tint(event)})));
  const total = groups.reduce((n, g) => n + g.rows.length, 0);
  card.append(widgetFoot('when', total, {href: appOrigin('when')}));
  return card;
}

let teamChip = 'all';

const teamTones = ['is-red', 'is-blue', 'is-gold', 'is-green'];

function teamWidget() {
  const team = state.model.team;
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

let partyChip = 'upcoming';

function partiesUnder(chip) {
  const parties = state.model.parties;
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

let birthdayChip = 'mine';

const stageNames = {'Wait': 'Scheduled', 'Awaiting Outreach': 'Ready to Contact', 'Awaiting Response': 'Waiting for Reply', 'Awaiting Newsletter': 'Ready for Newsletter', 'Complete': 'Complete'};

const stepWords = {outreach: 'Ask by', info: 'Charity due', newsletter: 'Newsletter'};

function shortDay(date) {
  return parseDate(date).toLocaleDateString('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
}

function birthdayRow(item, i, all) {
  const line = all
    ? [stageNames[item.stage] || item.stage, item.assignee || 'Unassigned']
    : [stepWords[item.next.step] + ' ' + shortDay(item.next.day), stageNames[item.stage] || item.stage];
  let pill = null;
  if (item.urgency) {
    pill = el('span', 'wg-pill', item.urgency === 'late' ? 'Late' : 'Due today');
  }
  const tone = item.urgency === 'late' ? 'is-red' : item.urgency === 'today' ? 'is-gold' : teamTones[i % teamTones.length];
  return pictureRow({href: appOrigin('birthday') + item.path, image: item.photo, title: item.name, line: line.join(' · '), pill, tone});
}

function birthdayWidget() {
  const birthday = state.model.birthday;
  const card = el('article', 'widget widget-birthday');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('birthday', 'Birthdays'));
  if (!birthday.admin) {
    birthdayChip = 'mine';
  }
  const chips = el('div', 'wg-chips');
  for (const [key, label] of [['mine', 'Mine'], ['all', 'All']].filter(([key]) => key === 'mine' || birthday.admin)) {
    const chip = el('button', 'wg-chip' + (birthdayChip === key ? ' is-on' : ''), label);
    chip.type = 'button';
    chip.append(el('span', 'wg-chip-count', String(birthday[key].length)));
    chip.addEventListener('click', () => {
      birthdayChip = key;
      renderWidgets(searchInput().value);
    });
    chips.append(chip);
  }
  card.append(head, chips);
  const all = birthdayChip === 'all';
  const items = birthday[birthdayChip];
  const name = 'birthday-' + birthdayChip;
  if (!items.length) {
    card.append(el('p', 'wg-empty', all ? 'Nobody in the next two newsletters.' : 'None of yours in the next two newsletters need anything.'));
  } else if (all) {
    const groups = dayGroups(items, b => b.newsletter, () => '');
    card.append(...grouped(name, groups,
      g => dayBar(g.day, [el('span', 'wg-day-count', `${g.rows.length} ${g.rows.length === 1 ? 'birthday' : 'birthdays'}`)]),
      (item, g, n) => birthdayRow(item, n, true)));
  } else {
    card.append(pictureList(name, items, (item, i) => birthdayRow(item, i, false)));
  }
  const foot = widgetFoot(name, items.length, {href: appOrigin('birthday')});
  if (foot) {
    card.append(foot);
  }
  return card;
}

const schoolTones = ['is-teal', 'is-lime', 'is-pink', 'is-blue'];

let schoolOpen;

function emailRow(email, n, open) {
  const date = parseDate(email.date);
  const title = el('button', 'wg-title', email.title);
  title.type = 'button';
  title.setAttribute('aria-expanded', String(open));
  const to = el('span', 'wg-email-to', email.to);
  to.title = email.to;
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

function typePick(school) {
  const counts = new Map();
  for (const e of school) {
    counts.set(e.to, (counts.get(e.to) || 0) + 1);
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
  const school = state.model.school;
  const card = el('article', 'widget widget-school');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('ask', 'Inbox'));
  card.append(head);
  if (!school.length) {
    card.append(el('p', 'wg-empty', 'No school email in the last two weeks.'));
    return card;
  }
  if (schoolType && !school.some(e => e.to === schoolType)) {
    schoolType = null;
  }
  head.append(typePick(school));
  const shown = !schoolType ? school : school.filter(e => e.to === schoolType);
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

const widgetMakers = {when: whenWidget, team: teamWidget, celebrate: celebrateWidget, school: schoolWidget, birthday: birthdayWidget};

const widgetNames = {when: 'Upcoming', team: 'Team', celebrate: 'Celebrate', school: 'Inbox', birthday: 'Birthdays'};

const widgetApps = {when: 'when', team: 'team', celebrate: 'celebrate', school: 'ask', birthday: 'birthday'};

function hasWork() {
  const birthday = state.model.birthday;
  return birthday.mine.length > 0 || birthday.all.length > 0;
}

export function widgetRows() {
  return state.model.widgets.map(w => {
    const icon = widgetTitle(widgetApps[w.key], '').querySelector('img');
    const meta = [shownTo(w.rules), w.me.shown ? '' : 'Hidden from you'].filter(Boolean).join(' · ');
    return {widget: w, name: widgetNames[w.key], mark: icon, meta};
  });
}

function hiddenBadge(card) {
  card.querySelector('.widget-head .widget-title').append(el('span', 'hidden-badge', 'Hidden'));
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
  const admin = isAdmin();
  if (root.hidden) {
    return;
  }
  for (const w of state.model.widgets) {
    if ((!w.me.shown && !admin) || (w.key === 'birthday' && !hasWork())) {
      continue;
    }
    const widget = widgetMakers[w.key]();
    if (!w.me.shown) {
      hiddenBadge(widget);
    }
    root.append(widget);
  }
  root.hidden = !root.children.length;
  if (!fitted && !root.hidden) {
    fitWidgets(query);
  }
}
