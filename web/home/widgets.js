import {state, superOn} from './state.js';
import {appOrigin} from '/toolbar.js';
import {el, svg} from './dom.js';
import {whenOrigin, calendarMark, calendarMenu, dropdown, audienceWords} from './cards.js';
import {openWidgetAudience, moveWidget} from './edit.js';
import {dayTypeClass} from '/daytype.js';

// The widgets across the top of the page, each a card with one app's view
// of what matters now: Helios When's week, what HCA-Team needs people for,
// and Helios Celebrate's parties.

// A YYYY-MM-DD as a local date, without the time zone shifting it.
function parseDate(date) {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(y, m - 1, d);
}

const pad = n => String(n).padStart(2, '0');

function dateOf(d) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function addDays(date, n) {
  const d = parseDate(date);
  d.setDate(d.getDate() + n);
  return dateOf(d);
}

// tint is where an event comes from, as When colours it: a party pink, an
// HCA event purple, the school's own blue.
function tint(event) {
  return event.linkApp === 'celebrate' ? 'is-celebrate' : event.linkApp === 'team' ? 'is-team' : 'is-school';
}

// startTime is when an event starts on a day, as the widget's time column
// says it: "3:30 PM" on its first day, "All day" for a whole-day event or a
// day after its first.
function startTime(event, date) {
  const [day, time] = event.startAt.split(' ');
  if (!time || day !== date) {
    return 'All day';
  }
  const [h, m] = time.split(':').map(Number);
  return new Date(2000, 0, 1, h, m).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
}

// sortKey puts a day's whole-day events first, then the timed ones by
// their start.
function sortKey(event, date) {
  const [day, time] = event.startAt.split(' ');
  return !time || day !== date ? '' : time;
}

// shown is what the widget lists: everything the month carries but what
// the viewer said no to.
function shown(event) {
  return event.answer !== 'no' && event.answer !== 'hidden';
}

// picked is the month read under the calendar picked in the widget's
// dropdown, or null for the rail's - the viewer's default calendar;
// base is the rail's month it was picked over, so a fresh model (after
// Make default) starts the widget over from its own.
let picked = null;
let base = null;

function currentMonth() {
  if (base !== state.model.calendar) {
    base = state.model.calendar;
    picked = null;
  }
  return picked || base;
}

// pick reads this month under another saved calendar and draws the
// widget from it.
async function pick(token) {
  const month = currentMonth();
  try {
    const res = await fetch('/api/apps/calendar?month=' + month.today.slice(0, 7) + '&calendar=' + encodeURIComponent(token));
    if (!res.ok) {
      return;
    }
    picked = await res.json();
  } catch {
    return;
  }
  nextMonth = null;
  renderWidgets(document.querySelector('#search').value);
}

// calendarPick is the saved calendar the widget reads, as a quiet dropdown
// beside the date - the list Upcoming Events and the rail's month offer.
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
  toggle.append(calendarMark(current), el('span', '', current.name), svg('chevron'));
  const menu = calendarMenu(list, current, chosen, c => pick(c.token));
  dropdown(toggle, menu);
  wrap.append(toggle, menu);
  return wrap;
}

// allEvents are the month's events - with the next month's once the week
// runs into it - from today through six days on.
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

// fetchNext asks for the month after, under the same calendar, when the
// week reaches into it, and draws the widget again once it is in.
async function fetchNext(month) {
  const last = addDays(month.today, 6);
  if (last.slice(0, 7) === month.month || (nextMonth && nextMonth.month === last.slice(0, 7) && nextMonth.calendar === month.calendar)) {
    return;
  }
  try {
    const res = await fetch('/api/apps/calendar?month=' + last.slice(0, 7) + '&calendar=' + encodeURIComponent(month.calendar || ''));
    if (!res.ok) {
      return;
    }
    nextMonth = await res.json();
  } catch {
    return;
  }
  renderWidgets(document.querySelector('#search').value);
}

// dayBar is a day's heading inside a widget: a pale bar with the weekday
// and the date - or a word in their place - and how many things fall on
// it at its far end, and any chips after the date.
function dayBar(day, count, noun, chips = []) {
  const bar = el('div', 'wg-day');
  const label = el('span', 'wg-day-label');
  if (day) {
    const date = parseDate(day);
    label.append(el('span', 'wg-day-weekday', date.toLocaleDateString('en-US', {weekday: 'short'})), el('span', 'wg-day-sep', '\u00b7'), el('span', '', date.toLocaleDateString('en-US', {month: 'short', day: 'numeric'})));
  } else {
    label.append(el('span', '', 'Ongoing'));
  }
  label.append(...chips);
  const plural = noun.endsWith('y') ? noun.slice(0, -1) + 'ies' : noun + 's';
  bar.append(label, el('span', 'wg-day-count', `${count} ${count === 1 ? noun : plural}`));
  return bar;
}

// wgRow is one row under a day: a dot in its colour on a line running down
// the day, what it is, and a chevron, the whole row opening href but for
// any link inside it, which goes where it says.
function wgRow(className, href, parts) {
  const row = el('li', 'wg-row ' + className);
  row.append(el('span', 'wg-dot'), ...parts);
  const go = el('a', 'wg-go');
  go.href = href;
  go.setAttribute('aria-label', 'Open');
  go.append(svg('chevron'));
  row.append(go);
  row.addEventListener('click', e => {
    if (!e.target.closest('a')) {
      location.href = href;
    }
  });
  return row;
}

// standing is the household's part in an event another app runs, as When
// words it - "John has a ticket", "You have a ticket" - on an outlined pill
// behind a tick.
function standing(event) {
  if (!event.mine || !event.call) {
    return null;
  }
  const pill = el('span', 'wg-standing');
  pill.append(svg('check'), el('span', '', event.call));
  return pill;
}

// action is the pill when the viewer has something to do: RSVP, in green,
// on an invitation still waiting on their word, to its page on When; else,
// for an event another app runs that is still open to them and where the
// household has no standing yet, the way in as When words it - Join, Get
// tickets, Join the waitlist - in the event's colour, to its page there.
function action(event) {
  if (event.invited && !event.answer) {
    const pill = el('a', 'wg-pill is-rsvp', 'RSVP');
    pill.href = whenOrigin('calendar') + event.path;
    pill.title = 'You\u2019re invited - answer on its page';
    return pill;
  }
  if (event.link && event.call && !event.mine && ['available', 'open', 'waitlist'].includes(event.availability)) {
    const pill = el('a', 'wg-pill', event.call);
    pill.href = whenOrigin(event.linkApp) + event.link;
    return pill;
  }
  return null;
}

// eventRow is one event on one day: its start, its title with the
// household's standing or a pill to act on beside it.
function eventRow(event, day) {
  const href = whenOrigin('calendar') + event.path;
  const title = el('a', 'wg-title', event.title);
  title.href = href;
  const main = el('div', 'wg-main');
  main.append(title);
  const extra = standing(event) || action(event);
  if (extra) {
    main.append(extra);
  }
  return wgRow(tint(event), href, [el('span', 'wg-time', startTime(event, day)), main]);
}

// weekGroups is the week from today on: every day with something on it,
// each event once, on the first of the seven days it falls on, whole-day
// ones first and then by start.
function weekGroups(events, today) {
  const days = [0, 1, 2, 3, 4, 5, 6].map(n => addDays(today, n));
  const byDay = new Map(days.map(d => [d, []]));
  for (const event of events) {
    const first = days.find(d => event.dates.includes(d));
    if (first) {
      byDay.get(first).push(event);
    }
  }
  return days.filter(d => byDay.get(d).length).map(d => ({day: d, events: byDay.get(d).sort((a, b) => sortKey(a, d).localeCompare(sortKey(b, d)))}));
}

// widgetRows is how many rows a widget shows before its More button.
const widgetRows = 4;

// expanded holds the lists whose More button was pressed, by name, kept
// while the widgets redraw.
const expanded = new Set();

// grouped draws groups of rows under their day bars, the first few rows
// only until More is pressed: a group cut short keeps its bar, with the
// day's whole count on it; a group past the cut is left out. More says how
// many are held back, and Show less folds them away again.
function grouped(name, groups, bar, row) {
  const out = [];
  const total = groups.reduce((n, g) => n + g.rows.length, 0);
  const open = expanded.has(name);
  let n = 0;
  for (const g of groups) {
    if (!open && n >= widgetRows) {
      break;
    }
    const list = el('ol', 'wg-list');
    for (const r of g.rows) {
      if (!open && n >= widgetRows) {
        break;
      }
      list.append(row(r, g, n++));
    }
    out.push(bar(g), list);
  }
  if (total > widgetRows) {
    const more = el('button', 'wg-more', open ? 'Show less' : `Show ${total - widgetRows} more`);
    more.type = 'button';
    more.addEventListener('click', () => {
      if (open) {
        expanded.delete(name);
      } else {
        expanded.add(name);
      }
      renderWidgets(document.querySelector('#search').value);
    });
    out.push(more);
  }
  return out;
}

// widgetTitle is a widget's heading: the mark of the app its things come
// from - the same the app switch and the page's app cards show, kept
// fresh by the app's mark - then its words.
function widgetTitle(app, words) {
  const title = el('h2', 'widget-title');
  const icon = el('img', 'widget-icon');
  const mark = ((state.model.apps || []).find(a => a.key === app) || {}).mark;
  icon.src = `/brand/apps/${app}.png` + (mark ? `?v=${mark}` : '');
  icon.alt = '';
  title.append(icon, el('span', '', words));
  return title;
}

// moreLink is a widget's way across: words and an arrow.
function moreLink(words, href) {
  const a = el('a', 'widget-more');
  a.href = href;
  a.append(el('span', '', words), svg('chevron'));
  return a;
}

// whenWidget is Helios When's card, This Week: the calendar picker at the
// heading's end, then today and the six days after, each day with
// something on it under its bar - the weekday, the date, any kind of day
// it is for the viewer's classrooms when not simply regular, and how many
// events - and its events under it; and at its foot View full calendar
// across to When. It reads the month the rail's calendar was first drawn
// from - the viewer's default calendar - so the two agree, until another
// is picked from the dropdown.
function whenWidget() {
  const month = currentMonth();
  if (!month || !month.today) {
    return null;
  }
  const events = allEvents(month);
  const card = el('article', 'widget widget-when');
  const head = el('header', 'widget-head');
  head.append(widgetTitle('calendar', 'This Week'));
  const choose = calendarPick(month);
  if (choose) {
    head.append(choose);
  }
  card.append(head);
  const groups = weekGroups(events, month.today);
  if (!groups.length) {
    card.append(el('p', 'wg-empty', 'Nothing on the calendar this week.'));
  }
  card.append(...grouped('when', groups.map(g => ({day: g.day, rows: g.events})),
    g => dayBar(g.day, g.rows.length, 'event', (((month.days || {})[g.day] || {}).kinds || []).map(k => el('span', 'widget-kind ' + dayTypeClass(k.name), k.words))),
    (event, g) => eventRow(event, g.day)));
  const foot = el('footer', 'widget-foot');
  foot.append(moreLink('View full calendar', whenOrigin('calendar')));
  card.append(foot);
  fetchNext(month);
  return card;
}

// HCA-Team's lists come from Heliosian's host (/api/apps/team), asked once
// per model - the page's load - and kept while the widgets are drawn again.
let team = null;
let teamFor = null;

async function fetchTeam() {
  teamFor = state.model;
  try {
    const res = await fetch('/api/apps/team');
    if (!res.ok) {
      return;
    }
    team = await res.json();
  } catch {
    return;
  }
  renderWidgets(document.querySelector('#search').value);
}

// teamChip is the chip picked - All, what needs people; Priority, what an
// admin marked as most needing hands; or Mine, what the viewer is on - kept
// while the widgets redraw.
let teamChip = 'all';

// The dots' and pills' colours, in turn down the list: red, blue, gold,
// green.
const teamTones = ['is-red', 'is-blue', 'is-gold', 'is-green'];

// teamWidget is HCA-Team's card, headed Team: View all across to the
// portal at the heading's end; three chips - All, what needs people that
// the viewer is not on, each saying what it still wants with Sign up;
// Priority, what an admin marked a priority, the same way, a chip only
// while something is; and Mine, what they are on that is still ahead, with
// the event it sits under and their position - each a picture list, as
// Celebrate's, the first three until More.
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
  head.append(widgetTitle('team', 'Team'), moreLink('View all', appOrigin('team')));
  const chips = el('div', 'wg-chips');
  const body = el('div');
  const paint = () => {
    chips.replaceChildren();
    // Priority is a chip only while an admin has marked something.
    const priority = (team.priority || []).length > 0;
    if (!priority && teamChip === 'priority') {
      teamChip = 'all';
    }
    for (const [key, label] of [['all', 'All'], ['priority', 'Priority'], ['mine', 'Mine']].filter(([key]) => key !== 'priority' || priority)) {
      const chip = el('button', 'wg-chip tone-' + key + (teamChip === key ? ' is-on' : ''), label);
      chip.type = 'button';
      chip.addEventListener('click', () => {
        teamChip = key;
        paint();
      });
      chips.append(chip);
    }
    body.replaceChildren();
    const mine = teamChip === 'mine';
    const items = mine ? team.mine : teamChip === 'priority' ? team.priority || [] : team.open;
    if (!items.length) {
      body.append(el('p', 'wg-empty', mine ? 'You\u2019re not signed up for anything coming up.' : 'Nothing needs hands just now.'));
      return;
    }
    body.append(...pictureList('team-' + teamChip, items, (item, i) => {
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
  card.append(head, chips, body);
  return card;
}

// Helios Celebrate's parties come from Heliosian's host
// (/api/apps/celebrate), asked once per model and kept while the widgets
// redraw, as HCA-Team's are.
let parties = null;
let partiesFor = null;

async function fetchParties() {
  partiesFor = state.model;
  try {
    const res = await fetch('/api/apps/celebrate');
    if (!res.ok) {
      return;
    }
    parties = (await res.json()).parties || [];
  } catch {
    return;
  }
  renderWidgets(document.querySelector('#search').value);
}

// partyChip is the chip picked - All, every party ahead; or Mine, those
// the household holds a ticket to or waits for - kept while the widgets
// redraw.
let partyChip = 'all';

// partyCards is how many rows a picture list - Celebrate's, Team's - shows
// before More.
const partyCards = 3;

// partyPill is a party's standing or way in: the household's tickets as
// When words them, else Get tickets or Join the waitlist while there is a
// way in, to its page on Celebrate; nothing for one sold out or closed.
function partyPill(p) {
  const held = standing(p);
  if (held) {
    return held;
  }
  if (p.call && (p.availability === 'available' || p.availability === 'waitlist')) {
    const pill = el('a', 'wg-pill', p.call);
    pill.href = whenOrigin('celebrate') + p.link;
    return pill;
  }
  return null;
}

// pictureRow is one row of a picture list - Celebrate's parties, Team's
// needs: the picture (else the title's first letter on a pale tile), the
// title over a line, a pill under them when there is one, and a chevron,
// the whole row opening href but for a link inside it.
function pictureRow({href, image, title, line, pill, tone}) {
  const row = el('li', 'wg-party' + (tone ? ' ' + tone : ''));
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
  go.append(svg('chevron'));
  row.append(pic, text, go);
  row.addEventListener('click', e => {
    if (!e.target.closest('a')) {
      location.href = href;
    }
  });
  return row;
}

// partyRow is one party: its picture, its title over its day and hours,
// and its pill.
function partyRow(p) {
  const day = parseDate(p.start).toLocaleDateString('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
  return pictureRow({
    href: whenOrigin('celebrate') + p.link, image: p.image ? whenOrigin(p.imageApp) + p.image : '',
    title: p.title, line: day + ' · ' + startTime(p, p.start), pill: partyPill(p),
  });
}

// pictureList is a widget's list of picture rows, the first three until
// Show N more, which lists the rest and then folds them away again.
function pictureList(name, items, row) {
  const open = expanded.has(name);
  const list = el('ol', 'wg-parties');
  (open ? items : items.slice(0, partyCards)).forEach((item, i) => list.append(row(item, i)));
  const out = [list];
  if (items.length > partyCards) {
    const more = el('button', 'wg-more', open ? 'Show less' : `Show ${items.length - partyCards} more`);
    more.type = 'button';
    more.addEventListener('click', () => {
      if (open) {
        expanded.delete(name);
      } else {
        expanded.add(name);
      }
      renderWidgets(document.querySelector('#search').value);
    });
    out.push(more);
  }
  return out;
}

// celebrateWidget is Helios Celebrate's card: View all across to it at the
// heading's end; All and Mine; then the next parties, each with its
// picture, the first three until More.
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
  head.append(widgetTitle('celebrate', 'Celebrate'), moreLink('View all', appOrigin('celebrate')));
  const chips = el('div', 'wg-chips');
  const body = el('div');
  const paint = () => {
    chips.replaceChildren();
    for (const [key, label] of [['all', 'All'], ['mine', 'Mine']]) {
      const chip = el('button', 'wg-chip' + (partyChip === key ? ' is-on' : ''), label);
      chip.type = 'button';
      chip.addEventListener('click', () => {
        partyChip = key;
        paint();
      });
      chips.append(chip);
    }
    body.replaceChildren();
    const mine = partyChip === 'mine';
    const items = mine ? parties.filter(p => p.mine) : parties;
    if (!items.length) {
      body.append(el('p', 'wg-empty', mine ? 'Your household has no tickets to anything coming up.' : 'No parties coming up.'));
      return;
    }
    body.append(...pictureList('celebrate-' + partyChip, items, partyRow));
  };
  paint();
  card.append(head, chips, body);
  return card;
}

// School email comes from Heliosian's host (/api/apps/school): the last
// week's, each with its key points; and the invitations still waiting for
// the viewer's reply from the same host (/api/apps/rsvp, the toolbar's
// badge's list) - both asked once per model and kept while the widgets
// redraw.
let school = null;
let rsvps = [];
let schoolFor = null;

async function fetchSchool() {
  schoolFor = state.model;
  try {
    const [mail, waiting] = await Promise.all([fetch('/api/apps/school'), fetch('/api/apps/rsvp')]);
    if (!mail.ok) {
      return;
    }
    school = (await mail.json()).emails || [];
    rsvps = waiting.ok ? (await waiting.json()).waiting || [] : [];
  } catch {
    return;
  }
  renderWidgets(document.querySelector('#search').value);
}

// listNames are the school's lists as the widget says them.
const listNames = {
  newsletter: 'Newsletter', parentsandstaff: 'Parents & staff', parentsonly: 'Parents', parentsandstudents: 'Parents & students',
  community: 'Community', parents: 'All parents', newstudentfamilies: 'New families', 'new.parents': 'New families',
};

// sentTo is whom an email went to, as its chip says it. A list's mail
// says so by its list: every family's ("Parents & staff"), a classroom's
// parents or students ("Jays parents"), a grade band ("Jays & Ravens").
// Mail the school sends through Veracross - the newsletter, a teacher's
// note - says nothing of it, so it goes by whom it was judged to be
// written to: the classrooms it names, else every family.
function sentTo(email) {
  const cap = w => w.charAt(0).toUpperCase() + w.slice(1);
  if (email.kind !== 'list') {
    if (!email.audience || email.audience === 'Everyone') {
      return 'All families';
    }
    return email.audience.split(',').map(r => r.trim()).filter(Boolean).join(' & ');
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

// schoolTones are the colours Inbox's rows wear in turn, the dot
// on the line and the chip alike.
const schoolTones = ['is-teal', 'is-lime', 'is-pink', 'is-blue'];

// schoolOpen is the email whose key points are showing, by key: undefined
// for the newest, as the widget first opens; null once the viewer folds
// it away. Only one is open at a time, and it stays open while the widgets
// redraw.
let schoolOpen;

// emailRow is one school email on the widget's line: a dot in its colour,
// the day it came, its subject, a chip saying whom it went to, and a
// chevron; a click opens its key points - or word that they are on their
// way - and Ask about this, which opens Helios Ask on a question about it,
// folding away whichever was open before.
function emailRow(email, n, open) {
  const row = el('li', 'wg-email ' + schoolTones[n % schoolTones.length]);
  row.classList.toggle('is-open', open);
  const date = parseDate(email.date);
  const head = el('button', 'wg-email-head');
  head.type = 'button';
  head.setAttribute('aria-expanded', String(open));
  const when = `${date.toLocaleDateString('en-US', {weekday: 'short'})}, ${date.toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}`;
  head.append(el('span', 'wg-dot'), el('span', 'wg-email-date', when), el('span', 'wg-email-title', email.title), el('span', 'wg-email-to', sentTo(email)), svg('chevron'));
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
  ask.append(el('span', '', 'Ask about this'), svg('chevron'));
  body.append(ask);
  head.addEventListener('click', () => {
    const opening = !row.classList.contains('is-open');
    for (const other of row.parentNode.children) {
      other.classList.remove('is-open');
      other.querySelector('.wg-email-head').setAttribute('aria-expanded', 'false');
    }
    row.classList.toggle('is-open', opening);
    head.setAttribute('aria-expanded', String(opening));
    schoolOpen = opening ? email.key : null;
  });
  row.append(head, body);
  return row;
}

// schoolRows is how many emails the Inbox shows at first, the most
// recent, and how many more each Show more adds; schoolCount is how many
// it shows now, kept while the widgets redraw and back to the first three
// when the dropdown picks another kind or Show less is pressed.
const schoolRows = 3;
let schoolCount = schoolRows;

// rsvpType is the Inbox dropdown's name for the invitations waiting on a
// reply.
const rsvpType = 'RSVP needed';

// rsvpPanel is the invitations waiting on the viewer's reply, at the top
// of the Inbox until they answer, set apart from the email in a green
// panel: how many need an RSVP and View all across to My Events' RSVP on
// When, then a white card of them, each its day, its title and an RSVP
// pill, the whole row opening its page on When, where they answer.
function rsvpPanel(waiting) {
  const panel = el('section', 'wg-rsvps');
  const head = el('div', 'wg-rsvps-head');
  const n = waiting.length;
  head.append(svg('calendar'), el('span', 'wg-rsvps-words', `${n} ${n === 1 ? 'event needs' : 'events need'} your RSVP`), moreLink('View all', whenOrigin('calendar') + '/mine/rsvp'));
  const list = el('ol', 'wg-rsvp-list');
  for (const rsvp of waiting) {
    const row = el('li');
    const a = el('a', 'wg-rsvp');
    a.href = whenOrigin('calendar') + rsvp.path;
    a.title = 'You\u2019re invited - answer on its page';
    const date = parseDate(rsvp.start.split(' ')[0]);
    const when = `${date.toLocaleDateString('en-US', {weekday: 'short'})}, ${date.toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}`;
    a.append(el('span', 'wg-rsvp-date', when), el('span', 'wg-rsvp-title', rsvp.title), el('span', 'wg-rsvp-pill', 'RSVP'));
    row.append(a);
    list.append(row);
  }
  panel.append(head, list);
  return panel;
}

// schoolType is the chip the Inbox's dropdown narrows it to - one whom
// the email went to ("Parents & staff") - or null for all of them, kept
// while the widgets redraw.
let schoolType = null;

// typePick is the Inbox's dropdown at its heading: All, then each whom the
// week's email went to as its chips say it, with how many; picking one
// shows only those.
function typePick() {
  const counts = new Map();
  if (rsvps.length) {
    counts.set(rsvpType, rsvps.length);
  }
  for (const e of school) {
    counts.set(sentTo(e), (counts.get(sentTo(e)) || 0) + 1);
  }
  const wrap = el('div', 'category-calendar widget-calendar wg-type');
  const toggle = el('button', 'category-calendar-toggle');
  toggle.type = 'button';
  toggle.title = 'Show one kind of email';
  toggle.append(el('span', '', schoolType || 'All'), svg('chevron'));
  const menu = el('div', 'category-calendar-menu');
  menu.hidden = true;
  for (const [type, n] of [[null, rsvps.length + school.length], ...counts]) {
    const item = el('button', 'category-calendar-item' + (type === schoolType ? ' is-on' : ''));
    item.type = 'button';
    item.append(el('span', '', type || 'All'), el('span', 'category-calendar-tail wg-type-count', String(n)));
    item.addEventListener('click', () => {
      menu.hidden = true;
      schoolType = type;
      schoolCount = schoolRows;
      renderWidgets(document.querySelector('#search').value);
    });
    menu.append(item);
  }
  dropdown(toggle, menu);
  wrap.append(toggle, menu);
  return wrap;
}

// schoolWidget is Inbox: every invitation still waiting on the viewer's
// reply, soonest first, until they answer it; then the last week of the
// school's email - the newsletter, the lists to every family, and the
// viewer's own classrooms' - newest first down a line, each its day,
// subject and whom it went to, the first opened on its key points. A
// dropdown at the heading narrows it to the invitations or to one whom the
// email went to. The three most recent emails show, and each Show more
// adds the next three; once all show, Show less folds back to three.
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
  if (!school.length && !rsvps.length) {
    card.append(el('p', 'wg-empty', 'No school email this week.'));
    return card;
  }
  const types = [...(rsvps.length ? [rsvpType] : []), ...school.map(sentTo)];
  if (schoolType && !types.includes(schoolType)) {
    schoolType = null;
  }
  head.append(typePick());
  const waiting = !schoolType || schoolType === rsvpType ? rsvps : [];
  const shown = !schoolType ? school : school.filter(e => sentTo(e) === schoolType);
  if (waiting.length) {
    card.append(rsvpPanel(waiting));
  }
  const count = Math.min(schoolCount, shown.length);
  const list = el('ol', 'wg-emails');
  if (shown.length) {
    const openKey = schoolOpen === undefined || (schoolOpen && !shown.some(e => e.key === schoolOpen)) ? shown[0].key : schoolOpen;
    shown.slice(0, count).forEach((e, n) => list.append(emailRow(e, n, e.key === openKey)));
  }
  card.append(list);
  if (shown.length > schoolRows) {
    const left = shown.length - count;
    const more = el('button', 'wg-more', left ? `Show ${Math.min(left, schoolRows)} more` : 'Show less');
    more.type = 'button';
    more.addEventListener('click', () => {
      schoolCount = left ? count + schoolRows : schoolRows;
      renderWidgets(document.querySelector('#search').value);
    });
    card.append(more);
  }
  return card;
}

// widgetMakers draw each widget, by its name.
const widgetMakers = {when: whenWidget, team: teamWidget, celebrate: celebrateWidget, school: schoolWidget};

// widgetNames are the widgets' names as an admin's pencil says them.
const widgetNames = {when: 'This Week', team: 'Team', celebrate: 'Celebrate', school: 'Inbox'};

// adminTools puts, in Super Admin Mode, a pencil at a widget's heading that
// opens who it is for, and says so beside the title: Hidden when the admin
// is not among them, else the rules in short.
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

// widgetOrder is the widgets' names in the order the page draws them, as
// an admin set it for everyone.
function widgetOrder() {
  const makers = Object.keys(widgetMakers);
  const set = (state.model.widgetOrder || []).filter(k => makers.includes(k));
  return [...set, ...makers.filter(k => !set.includes(k))];
}

// renderWidgets draws the row in the order an admin set, or leaves it
// empty while a search is filtering the page below. A widget not for the viewer - its rules, set
// by an admin, leave them out - is left out; in Super Admin Mode every
// widget shows, with the pencil that sets who it is for.
export function renderWidgets(query = '') {
  const root = document.querySelector('#widgets');
  root.replaceChildren();
  root.hidden = Boolean(query.trim());
  if (root.hidden) {
    return;
  }
  const admin = superOn();
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
}
