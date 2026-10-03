import {state, isAdmin, feed, readUpcoming, readToDos, holdsApps, inGrid, widgetGrid, categoryOf, fraction, clock} from './state.js';
import {appOrigin} from '/appswitch.js';
import {searchInput} from '/shell.js';
import {el, svg, toast} from '/elements.js';
import {act} from '/data.js';
import {calendarMark, calendarMenu, categoryItems, dropdown, opensOutside, shownTo} from './cards.js';
import {iconOf, categoryMark} from './dom.js';
import {dayBar, dayChip, dayRow, eventRow, standing} from '/dayrows.js';
import {rsvpPanel} from '/rsvps.js';

function parseDate(date) {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(y, m - 1, d);
}

function tint(event) {
  return event.linkApp === 'celebrate' ? 'is-celebrate' : event.linkApp === 'team' ? 'is-team' : 'is-school';
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
  renderNav();
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

function appIcon(app, src) {
  const icon = el('img', 'widget-icon');
  const mark = (state.model.apps.find(a => a.key === app) || {}).mark;
  icon.src = src || `/brand/apps/${app}.png` + (mark ? `?v=${mark}` : '');
  icon.alt = '';
  return icon;
}

function widgetTitle(app, words, src) {
  const title = el('h2', 'widget-title');
  title.append(appIcon(app, src), el('span', '', words));
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

function picture(className, image, letter, tone = '') {
  const pic = el('span', className);
  if (image) {
    const img = el('img');
    img.src = image;
    img.alt = '';
    img.loading = 'lazy';
    pic.append(img);
    return pic;
  }
  pic.classList.add('is-letter', ...tone.split(' ').filter(Boolean));
  pic.append(el('span', '', letter));
  return pic;
}

function initial(title) {
  return title.trim().charAt(0).toUpperCase();
}

function pictureRow({href, image, title, line, pill, tone, mark}) {
  const row = el('li', 'wg-party' + (tone ? ' ' + tone : ''));
  row.dataset.row = '';
  const pic = picture('wg-party-pic' + (mark ? ' is-mark' : ''), image, initial(title));
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
  const charity = (item.chosen ? '' : 'Default: ') + item.charity;
  const line = all ? [item.assignee || 'Unassigned', charity] : [stepWords[item.next.step] + ' ' + shortDay(item.next.day), charity];
  const pills = el('span', 'wg-stage-pills');
  pills.append(el('span', 'wg-stage stage-' + item.stage.toLowerCase().replace(/[^a-z]+/g, '-'), stageNames[item.stage] || item.stage));
  if (item.urgency) {
    pills.append(el('span', 'wg-pill', item.urgency === 'late' ? 'Late' : 'Due today'));
  }
  const tone = item.urgency === 'late' ? 'is-red' : item.urgency === 'today' ? 'is-gold' : teamTones[i % teamTones.length];
  return pictureRow({href: appOrigin('birthday') + item.path, image: item.photo, title: item.name, line: line.join(' · '), pill: pills, tone});
}

function birthdayWidget() {
  const birthday = state.model.birthday;
  const card = el('article', 'widget widget-birthday');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('birthday', 'Birthdays'));
  const tabs = [['mine', 'Mine'], ['all', 'All']].filter(([key]) => birthday[key].length > 0);
  if (!tabs.some(([key]) => key === birthdayChip)) {
    birthdayChip = tabs[0][0];
  }
  const chips = el('div', 'wg-chips');
  for (const [key, label] of tabs) {
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
  if (all) {
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

let schoolOpen = null;

function emailAsk(email) {
  const day = parseDate(email.date).toLocaleDateString('en-US', {weekday: 'long', month: 'long', day: 'numeric'});
  return appOrigin('ask') + '/?q=' + encodeURIComponent(`What should I know from the school email "${email.title}" sent ${day}?`);
}

function pointItem(email, words, point) {
  const repeated = email.repeats[point];
  const todo = state.model.todos.find(t => t.email === email.id && t.point === point) || state.model.todos.find(t => t.id === repeated);
  if (!todo) {
    return el('li', '', words);
  }
  const done = todo.me.state === 'done';
  const item = el('li', 'wg-point-todo' + (done ? ' is-done' : ''));
  const box = todoButton('Done: ' + todo.title, done ? 'Mark not done' : 'Mark done', done, 'wg-check', svg('check'), () => setTodo(todo, done ? 'clear' : 'complete'));
  const text = el('span', 'wg-point-words', todo.title);
  text.title = words;
  if (todo.due) {
    text.append(el('span', 'wg-point-due', ' · ' + parseDate(todo.due).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})));
  }
  item.append(box, text);
  return item;
}

function emailTodos(email) {
  const repeated = Object.values(email.repeats);
  return state.model.todos.filter(t => t.email === email.id || repeated.includes(t.id));
}

function reminderCount(n) {
  return `${n} ${n === 1 ? 'reminder' : 'reminders'}`;
}

function emailRow(email, n, open) {
  const date = parseDate(email.date);
  const title = el('button', 'wg-title', email.title);
  title.type = 'button';
  const reminders = emailTodos(email).length;
  if (reminders) {
    title.append(el('span', 'wg-title-count', reminderCount(reminders)));
  }
  title.setAttribute('aria-expanded', String(open));
  const to = el('span', 'wg-email-to', email.to);
  to.title = email.to;
  const body = el('div', 'wg-email-body');
  if (email.points.length) {
    const points = el('ul', 'wg-points');
    email.points.forEach((p, i) => points.append(pointItem(email, p, i + 1)));
    body.append(points);
  } else {
    body.append(el('div', 'wg-sub', 'Key points on their way.'));
  }
  const ask = el('a', 'wg-ask');
  ask.href = emailAsk(email);
  ask.append(el('span', '', 'Ask about this'), svg('chevron-right'));
  body.append(ask);
  const after = document.createDocumentFragment();
  if (email.summary) {
    const summary = el('div', 'wg-email-summary', email.summary);
    summary.title = email.summary;
    after.append(summary);
  }
  after.append(body);
  const row = dayRow('wg-email ' + schoolTones[n % schoolTones.length], {title, chips: [to], time: email.time ? clock(email.time) : '', after}, e => {
    if (e.target.closest('.wg-email-body')) {
      return;
    }
    const opening = !row.classList.contains('is-open');
    row.classList.toggle('is-open', opening);
    title.setAttribute('aria-expanded', String(opening));
    if (opening) {
      schoolOpen.add(email.key);
    } else {
      schoolOpen.delete(email.key);
    }
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
      schoolOpen = null;
      renderWidgets(searchInput().value);
    });
    menu.append(item);
  }
  dropdown(toggle, menu);
  wrap.append(toggle, menu);
  return wrap;
}

function dayCount(emails) {
  const reminders = new Set(emails.flatMap(e => emailTodos(e).map(t => t.id))).size;
  const count = `${emails.length} ${emails.length === 1 ? 'email' : 'emails'}`;
  if (!reminders) {
    return count;
  }
  return `${count} · ${reminderCount(reminders)}`;
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
  const groups = dayGroups(shown, e => e.date, e => e.time || '').reverse().map(g => ({...g, rows: g.rows.reverse()}));
  if (!schoolOpen) {
    schoolOpen = new Set(groups.length ? [groups[0].rows[0].key] : []);
  }
  card.append(...grouped(name, groups, g => dayBar(g.day, [el('span', 'wg-day-count', dayCount(g.rows))]), (e, g, n) => emailRow(e, n, schoolOpen.has(e.key))));
  const foot = widgetFoot(name, shown.length, null);
  if (foot) {
    card.append(foot);
  }
  return card;
}

let todoChip = 'open';

const todoChips = [['open', 'Recent'], ['saved', 'Saved'], ['done', 'Done']];

function todosUnder(chip) {
  const todos = state.model.todos;
  if (chip === 'saved') {
    return todos.filter(t => t.me.state === 'saved');
  }
  if (chip === 'done') {
    return todos.filter(t => t.me.state === 'done');
  }
  return todos.filter(t => t.me.state !== 'done');
}

async function setTodo(todo, action) {
  try {
    await act('to-dos', todo.id, action);
    await readToDos();
  } catch (err) {
    toast(err.message);
  }
  renderNav();
  renderWidgets(searchInput().value);
}

function todoAsk(todo) {
  const day = parseDate(todo.source.date).toLocaleDateString('en-US', {weekday: 'long', month: 'long', day: 'numeric'});
  return appOrigin('ask') + '/?q=' + encodeURIComponent(`Tell me more about "${todo.title}" from the school email "${todo.source.title}" sent ${day}. What do I need to do, and by when?`);
}

function todoButton(label, tip, pressed, className, content, onClick) {
  const b = el('button', className);
  b.type = 'button';
  b.setAttribute('aria-label', label);
  b.setAttribute('aria-pressed', String(pressed));
  b.title = tip;
  b.append(content);
  b.addEventListener('click', () => {
    b.disabled = true;
    onClick();
  });
  return b;
}

function todoTip(todo) {
  const sent = parseDate(todo.source.date).toLocaleDateString('en-US', {month: 'short', day: 'numeric'});
  return `${todo.summary}\n\n${todo.details}\n\nFrom “${todo.source.title}” to ${todo.source.to}, ${sent}`;
}

function todoRow(todo) {
  const mark = todo.me.state || '';
  const row = el('li', 'wg-check-row' + (mark ? ' is-' + mark : ''));
  row.dataset.row = '';
  const done = mark === 'done';
  const box = todoButton('Done: ' + todo.title, done ? 'Mark not done' : 'Mark done', done, 'wg-check', svg('check'), () => setTodo(todo, done ? 'clear' : 'complete'));
  const title = el('a', 'wg-check-title', todo.title);
  title.href = todoAsk(todo);
  title.title = todoTip(todo);
  row.append(box, title);
  if (todo.link) {
    const open = opensOutside(el('a', 'wg-check-icon'));
    open.href = todo.link;
    open.setAttribute('aria-label', 'Open the link for ' + todo.title);
    open.title = 'Open the link';
    open.append(svg('open'));
    row.append(open);
  }
  if (!done) {
    const saved = mark === 'saved';
    row.append(todoButton('Save: ' + todo.title, saved ? 'Saved - click to unsave' : 'Save so it stays on the list', saved, 'wg-check-icon wg-check-save', svg('star'), () => setTodo(todo, saved ? 'clear' : 'save')));
  }
  const due = el('span', 'wg-check-due' + (todo.due && todo.due < state.model.today && !done ? ' is-late' : ''));
  if (todo.due) {
    due.textContent = parseDate(todo.due).toLocaleDateString('en-US', {month: 'short', day: 'numeric'});
  }
  row.append(due);
  return row;
}

function byDue(a, b) {
  return (a.due || '9999').localeCompare(b.due || '9999');
}

function todoWidget() {
  const card = el('article', 'widget widget-todo');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('ask', 'Reminders', widgetMarks.todo));
  const chips = el('div', 'wg-chips');
  for (const [key, label] of todoChips) {
    const chip = el('button', 'wg-chip' + (todoChip === key ? ' is-on' : ''), label);
    chip.type = 'button';
    chip.append(el('span', 'wg-chip-count', String(todosUnder(key).length)));
    chip.addEventListener('click', () => {
      todoChip = key;
      renderWidgets(searchInput().value);
    });
    chips.append(chip);
  }
  card.append(head, chips);
  const items = todosUnder(todoChip);
  const name = 'todo-' + todoChip;
  if (!items.length) {
    const empty = {open: 'Nothing to do from the school’s email just now.', saved: 'Save a to-do to keep it here after its day passes.', done: 'Nothing checked off yet.'}[todoChip];
    card.append(el('p', 'wg-empty', empty));
    return card;
  }
  const list = el('ul', 'wg-checklist');
  items.slice().sort(byDue).slice(0, shownCount(name)).forEach(todo => list.append(todoRow(todo)));
  card.append(list);
  const foot = widgetFoot(name, items.length, null);
  if (foot) {
    card.append(foot);
  }
  return card;
}

function categoryTitle(category) {
  const title = el('h2', 'widget-title');
  const mark = el('span', 'widget-icon widget-mark');
  mark.append(categoryMark(iconOf(category)));
  title.append(mark, el('span', '', category.title));
  return title;
}

function linkIcon(item, className, tone) {
  return picture(className, item.image, initial(item.title), tone);
}

function linkGrid(items) {
  const grid = el('ul', 'wg-grid');
  items.forEach((item, i) => {
    const a = el('a', 'wg-grid-item');
    if (item.external) {
      opensOutside(a);
    }
    a.href = item.href;
    a.title = item.tip || item.title;
    a.append(linkIcon(item, 'wg-grid-icon', teamTones[i % teamTones.length]), el('span', 'wg-grid-title', item.title));
    if (item.line) {
      a.append(el('span', 'wg-grid-line', item.line));
    }
    const li = el('li');
    li.append(a);
    grid.append(li);
  });
  return grid;
}

function compactList(items) {
  const list = el('ul', 'wg-links');
  items.forEach((item, i) => {
    const li = el('li', 'wg-link');
    const a = opensOutside(el('a', 'wg-link-main'));
    a.href = item.href;
    const text = el('span', 'wg-link-text');
    text.append(el('span', 'wg-link-title', item.title));
    if (item.line) {
      text.append(el('span', 'wg-link-sub', item.line));
    }
    a.title = item.tip || item.title;
    a.append(linkIcon(item, 'wg-link-icon', teamTones[i % teamTones.length]), text);
    li.append(a);
    list.append(li);
  });
  return list;
}

function categoryWidget(key) {
  const category = categoryOf(key);
  if (!category) {
    return null;
  }
  const items = categoryItems(category);
  if (!items.length && !isAdmin()) {
    return null;
  }
  const card = el('article', 'widget widget-category');
  const head = el('header', 'widget-head');
  head.append(categoryTitle(category));
  card.append(head);
  if (!items.length) {
    card.append(el('p', 'wg-empty', holdsApps(category) ? 'No apps to show.' : 'No links yet.'));
    return card;
  }
  if (inGrid(category)) {
    card.append(linkGrid(items));
    return card;
  }
  if (!holdsApps(category)) {
    card.append(compactList(items));
    return card;
  }
  const name = 'category-' + category.id;
  card.append(pictureList(name, items, (item, i) => pictureRow({...item, tone: teamTones[i % teamTones.length]})));
  const foot = widgetFoot(name, items.length, null);
  if (foot) {
    card.append(foot);
  }
  return card;
}

const widgetMakers = {when: whenWidget, team: teamWidget, celebrate: celebrateWidget, school: schoolWidget, birthday: birthdayWidget, todo: todoWidget};

const widgetNames = {when: 'Upcoming', team: 'Team', celebrate: 'Celebrate', school: 'Inbox', birthday: 'Birthdays', todo: 'Reminders'};

const widgetApps = {when: 'when', team: 'team', celebrate: 'celebrate', school: 'ask', birthday: 'birthday', todo: 'ask'};

const widgetMarks = {todo: '/brand/reminders.png'};

function hasWork() {
  const birthday = state.model.birthday;
  return birthday.mine.length > 0 || birthday.all.length > 0;
}

export function widgetRows() {
  return state.model.widgets.map(w => {
    const meta = [shownTo(w.rules), w.me.shown ? '' : 'Hidden from you'].filter(Boolean).join(' · ');
    const category = categoryOf(w.key);
    if (category) {
      return {widget: w, name: category.title, mark: categoryMark(iconOf(category)), meta};
    }
    return {widget: w, name: widgetNames[w.key], mark: appIcon(widgetApps[w.key], widgetMarks[w.key]), meta};
  });
}

const sidebarCount = 5;

function navPic(item) {
  if (item.todo) {
    return el('span', 'app-nav-dot');
  }
  if (item.icon && !item.image) {
    const pic = el('span', 'app-nav-pic');
    pic.append(svg(item.icon));
    return pic;
  }
  return picture('app-nav-pic', item.image, item.letter || initial(item.title));
}

function navLink(item) {
  const a = el('a', 'app-nav-link');
  a.href = item.href;
  a.title = item.tip || item.title;
  if (item.external) {
    opensOutside(a);
  }
  const pic = navPic(item);
  pic.setAttribute('aria-hidden', 'true');
  a.append(pic, el('span', 'app-nav-link-title', item.title));
  return a;
}

function upcomingItems() {
  const today = state.model.today;
  const seen = new Set();
  const out = [];
  for (const g of dayGroups(currentUpcoming().events, event => event.dates.filter(d => d >= today), sortKey)) {
    for (const event of g.rows) {
      if (seen.has(event.path)) {
        continue;
      }
      seen.add(event.path);
      out.push({href: appOrigin('when') + event.path, title: event.title, letter: String(parseDate(g.day).getDate()), tip: `${event.title}, ${shortDay(g.day)}`});
    }
  }
  return out;
}

function schoolItems() {
  return dayGroups(state.model.school, e => e.date, e => e.time || '').reverse().flatMap(g => g.rows.reverse()).map(email => ({
    href: emailAsk(email), title: email.title, icon: 'mail', tip: `${email.title} · ${email.to}`,
  }));
}

const sidebarItems = {
  when: upcomingItems,
  todo: () => todosUnder('open').slice().sort(byDue).map(todo => ({href: todoAsk(todo), title: todo.title, todo, tip: todo.summary})),
  team: () => state.model.team.open.map(item => ({href: appOrigin('team') + item.path, title: item.title, image: item.image ? appOrigin('team') + item.image : ''})),
  celebrate: () => partiesUnder('upcoming').map(p => ({href: appOrigin('celebrate') + p.link, title: p.title, image: p.image ? appOrigin(p.imageApp) + p.image : ''})),
  school: schoolItems,
  birthday: () => {
    const birthday = state.model.birthday;
    return (birthday.mine.length ? birthday.mine : birthday.all).map(item => ({href: appOrigin('birthday') + item.path, title: item.name, image: item.photo}));
  },
};

function viewable(w, admin) {
  return (w.me.shown || admin) && (w.key !== 'birthday' || hasWork());
}

export function renderNav() {
  const nav = document.querySelector('#app-nav');
  nav.replaceChildren();
  const admin = isAdmin();
  for (const row of widgetRows()) {
    const w = row.widget;
    if (!w.sidebar || !viewable(w, admin)) {
      continue;
    }
    const category = categoryOf(w.key);
    const items = (category ? categoryItems(category) : sidebarItems[w.key]()).slice(0, sidebarCount);
    if (!items.length) {
      continue;
    }
    const glyph = el('span', 'app-nav-glyph');
    glyph.setAttribute('aria-hidden', 'true');
    glyph.append(row.mark);
    const head = el('div', 'app-nav-heading');
    head.id = 'nav-' + w.key.replace(/[^a-z0-9]+/g, '-');
    head.append(glyph, el('span', '', row.name));
    if (!w.me.shown) {
      head.append(el('span', 'hidden-badge', 'Hidden'));
    }
    const group = el('div', 'app-nav-group');
    group.setAttribute('role', 'group');
    group.setAttribute('aria-labelledby', head.id);
    const body = el('div', 'app-nav-items');
    body.append(...items.map(navLink));
    group.append(head, body);
    nav.append(group);
  }
  nav.hidden = !nav.children.length;
}

function hiddenBadge(card) {
  card.querySelector('.widget-head .widget-title').append(el('span', 'hidden-badge', 'Hidden'));
}

function drawWidget(w, admin) {
  if (!w || !viewable(w, admin)) {
    return null;
  }
  const widget = w.key.startsWith('category:') ? categoryWidget(w.key) : widgetMakers[w.key]();
  if (widget && !w.me.shown) {
    hiddenBadge(widget);
  }
  return widget;
}

function fitWidgets(query) {
  let changed = false;
  for (const card of document.querySelectorAll('#widgets .widget')) {
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
  for (const row of widgetGrid()) {
    const line = el('div', 'widget-row');
    const shares = [];
    for (const slot of row.slots) {
      const widget = drawWidget(state.model.widgets.find(w => w.key === slot.key), admin);
      if (widget) {
        line.append(widget);
        shares.push(slot.share);
      }
    }
    if (!shares.length) {
      continue;
    }
    line.dataset.count = String(shares.length);
    const parts = shares.map(fraction);
    const total = parts.reduce((a, b) => a + b, 0);
    let left = 6;
    [...line.children].forEach((widget, i) => {
      const span = i === parts.length - 1 ? left : Math.round(6 * parts[i] / total);
      left -= span;
      widget.style.setProperty('--span', String(span));
    });
    root.append(line);
  }
  root.hidden = !root.children.length;
  if (!fitted && !root.hidden) {
    fitWidgets(query);
  }
}
