import {queries, rowsOf, write} from '/rail.js';
import {sheetStamp} from '/stamp.js';
import {parseWhen, timeRange, googleCalendarLink} from '/datecard.js';
import {el, svg} from '/elements.js';
import {host} from '/guesthost.js';
import {answerButtons} from '/inviteparts.js';
import {questionQueries, questionParts} from '/rsvpquestions.js';
import {replyRows} from '/questionfields.js';
import {whereWords} from '/envelope.js';
import {inviteHead} from '/sidebar.js';
import {lookWords, lookFrom} from '/themes.js';

export const invitationColumns = '(columns name subtitle managed_by start end all_day location place description theme envelope seal liner design card_picture backdrop_picture hosted_by path slug status rsvp_yes rsvp_no rsvp_maybe maybe members_visible_to when_href)';
export const mineOrHousehold = '(or (= person @viewer) (in person (household @viewer)))';
export const gatherInvitation = '(= kind "event") (not (blank theme))';

export async function invitationRow(key) {
  const lower = JSON.stringify(key.toLowerCase());
  const a = await queries({
    group: `(from GROUP @g (where (= kind "event") (or (= id ${JSON.stringify(key)}) (= slug ${lower}))) ${invitationColumns})`,
    alias: `(from ALIAS (where (= alias ${lower})) (columns target))`,
  });
  const named = rowsOf(a.group, 'GROUP')[0];
  if (named) {
    return named;
  }
  const alias = rowsOf(a.alias, 'ALIAS')[0];
  if (!alias) {
    return null;
  }
  const b = await queries({group: `(from GROUP @g (where (= kind "event") (= id ${JSON.stringify(alias.target)})) ${invitationColumns})`});
  return rowsOf(b.group, 'GROUP')[0] || null;
}

export function guestListShown(inv) {
  return inv.host || inv.listShown;
}

export function deckOf(inv, pages) {
  return [{id: '', title: inv.name, card: true}, ...pages];
}

export function cardWords(inv, details) {
  return {
    look: inv.look, name: inv.name, subtitle: inv.subtitle, cancelled: inv.status === 'cancelled', start: inv.start, hours: hourWords(inv),
    location: inv.location, place: inv.place, description: inv.description, hosts: details.hosts, hostedBy: inv.hostedBy,
  };
}

function pictureOf(g, column) {
  return g[column] ? `/api/blob/${g.id}/${column}?c=${g[column]}` : '';
}

export function inviteOf(g, hosting) {
  return {
    id: g.id, name: g.name || 'Untitled invitation', subtitle: g.subtitle || '', start: g.start || '', end: g.end || '', allDay: g.all_day === 'Yes',
    location: g.location || '', place: g.place || '', description: g.description || '', theme: g.theme || '', hostedBy: g.hosted_by || '',
    look: lookFrom({theme: g.theme, envelope: g.envelope, seal: g.seal, liner: g.liner, design: g.design, cardPicture: pictureOf(g, 'card_picture'), backdrop: pictureOf(g, 'backdrop_picture')}),
    path: g.path || `/g/${g.id}`,
    slug: g.slug || '', status: g.status || '', rsvpYes: g.rsvp_yes || '', rsvpNo: g.rsvp_no || '', rsvpMaybe: g.rsvp_maybe || '', maybe: g.maybe === 'Yes' && Boolean(g.rsvp_maybe),
    listShown: Boolean(g.members_visible_to), whenHref: g.when_href || '', host: hosting, managers: g.managed_by || '', hostNames: [], pageWords: '',
  };
}

export function lookThemes(inv) {
  return [inv.look.theme, inv.look.envelope, inv.look.seal, inv.look.liner];
}

export function asEvent(inv) {
  const day = inv.start.slice(0, 10);
  return {
    id: inv.id, title: inv.name, source: 'sheet', allDay: inv.allDay, location: inv.location, where: whereWords(inv), cancelled: inv.status === 'cancelled',
    start: inv.start, end: inv.end, dates: day ? [day] : [], path: inv.path, can: {edit: inv.host}, hostsAreGuests: true, answerWords: lookWords(inv.look),
  };
}

export function guestListWords(inv) {
  return inv.host ? 'Guest list' : lookWords(inv.look).coming;
}

function answerGroupsOf(inv) {
  return {yes: inv.rsvpYes, no: inv.rsvpNo, maybe: inv.maybe ? inv.rsvpMaybe : ''};
}

export async function answerAs(inv, word) {
  const groups = [inv.rsvpYes, inv.rsvpNo, inv.rsvpMaybe].filter(Boolean).map(g => JSON.stringify(g)).join(' ');
  const a = await queries({mine: `(from MEMBER (where (in group ${groups}) (= person @viewer)) (columns group))`});
  const batch = rowsOf(a.mine, 'MEMBER').map(m => ({delete: m.id}));
  const group = answerGroupsOf(inv)[word];
  if (group) {
    batch.push({insert: 'MEMBER', row: {group, person: host.me().id, member: 'yes', added_by: host.me().id, added: sheetStamp()}});
  }
  if (batch.length) {
    await write(batch);
  }
}

export async function loadDetails(inv) {
  const id = JSON.stringify(inv.id);
  const answerGroups = `(or (in group (select GROUP.rsvp_yes (= id ${id}))) (in group (select GROUP.rsvp_no (= id ${id}))) (in group (select GROUP.rsvp_maybe (= id ${id}))))`;
  const a = await queries({
    hosts: `(from PERSON (where (in id (select EFFECTIVE_MEMBER.person (in group (select GROUP.managed_by (= id ${id})))))) (order name_sort asc) (columns name_show))`,
    household: `(from PERSON (where (or (= id @viewer) (in id (household @viewer))) (in id (select EFFECTIVE_MEMBER.person (= group ${id})))) (order name_sort asc) (columns name_show name_short child))`,
    answers: `(from MEMBER (where ${answerGroups} ${mineOrHousehold}) (columns group person))`,
    mine: `(from MEMBER (where (= group ${id}) (= person @viewer)) (columns opened))`,
    guests: `(from EFFECTIVE_MEMBER (where (= group ${id})) (columns person))`,
    tally: `(from MEMBER (where ${answerGroups}) (columns group))`,
    ...questionQueries(inv.id),
    students: `(from PERSON (where (or (= id @viewer) (in id (household @viewer))) (student id)) (columns name_show))`,
    repliers: `(from PERSON (where (in id (select ANSWER.person (in question (select QUESTION.id (= group ${id})))))) (columns name_show))`,
    families: `(from GROUP (where (in id (select ANSWER.family (in question (select QUESTION.id (= group ${id})))))) (columns name))`,
  });
  const answers = {};
  for (const m of rowsOf(a.answers, 'MEMBER')) {
    answers[m.person] = {row: m.id, word: m.group === inv.rsvpYes ? 'yes' : m.group === inv.rsvpMaybe ? 'maybe' : 'no'};
  }
  const tally = rowsOf(a.tally, 'MEMBER');
  const students = new Set(a.students.result);
  const me = host.me();
  return {
    ...questionParts(a),
    hosts: rowsOf(a.hosts, 'PERSON').map(p => p.name_show),
    household: rowsOf(a.household, 'PERSON').map(p => ({
      id: p.id, name: p.name_show, short: p.name_short || p.name_show, answer: answers[p.id] || null, going: Boolean(answers[p.id] && answers[p.id].word === 'yes'),
      kid: students.has(p.id) || p.child === 'Yes',
    })).sort((x, y) => (y.id === me.id) - (x.id === me.id)),
    myRow: rowsOf(a.mine, 'MEMBER')[0] || null,
    counts: {guests: a.guests.result.length, yes: tally.filter(m => m.group === inv.rsvpYes).length, no: tally.filter(m => m.group === inv.rsvpNo).length},
    names: Object.fromEntries([...rowsOf(a.repliers, 'PERSON').map(p => [p.id, p.name_show]), ...rowsOf(a.families, 'GROUP').map(g => [g.id, g.name])]),
  };
}

export async function fetchPages(inv) {
  const a = await queries({pages: `(from INVITE_PAGE (where (= group ${JSON.stringify(inv.id)})) (order order asc) (columns order title body background picture))`});
  return rowsOf(a.pages, 'INVITE_PAGE').map(pageOf);
}

export async function noteOpened(inv, details) {
  if (details.myRow && details.myRow.opened) {
    return;
  }
  const me = host.me();
  if (!details.household.some(p => p.id === me.id)) {
    return;
  }
  const at = sheetStamp();
  await write([details.myRow ? {set: details.myRow.id, cells: {opened: at}} : {insert: 'MEMBER', row: {group: inv.id, person: me.id, opened: at}}]);
}

const longDay = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric', year: 'numeric'});
const shortDay = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
const clock = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});

export function dayWords(inv, short) {
  const from = parseWhen(inv.start);
  if (!from) {
    return 'Date to come';
  }
  return (short ? shortDay : longDay).format(from.date);
}

export function hourWords(inv) {
  const from = parseWhen(inv.start);
  if (!from || !from.hasTime || inv.allDay) {
    return '';
  }
  const to = parseWhen(inv.end);
  if (!to || !to.hasTime || to.date <= from.date) {
    return clock.format(from.date);
  }
  return timeRange(from.date, to.date);
}

function calendarLinkOf(inv, url) {
  return googleCalendarLink({title: inv.name, start: inv.start, end: inv.end, allDay: inv.allDay, location: whereWords(inv), details: [inv.description, url].filter(Boolean).join('\n\n')});
}

export function invitationHead(inv, href) {
  return inviteHead({
    href, name: inv.name, look: inv.look, start: inv.start, hours: hourWords(inv), calendar: calendarLinkOf(inv, location.origin + href),
    place: inv.place, location: inv.location, hostedBy: inv.hostedBy, hosts: inv.hostNames,
  });
}

export function topbarLine(inv) {
  document.querySelector('.sidebar .brand').hidden = Boolean(inv);
  const bar = document.querySelector('#topbar');
  let line = bar.querySelector('.topbar-subtitle');
  if (!line) {
    line = el('div', 'topbar-subtitle');
    const menu = bar.querySelector('.topbar-menu');
    if (menu) {
      menu.after(line);
    } else {
      bar.prepend(line);
    }
  }
  line.hidden = !inv;
  if (!inv) {
    line.replaceChildren();
    return;
  }
  line.replaceChildren(el('span', 'topbar-title', inv.name));
  if (inv.subtitle) {
    line.append(el('span', 'topbar-subtitle-words', inv.subtitle));
  }
}

export function pageOf(row) {
  return {...row, background: row.background || '', pictureUrl: row.picture ? `/api/blob/${row.id}/picture?c=${row.picture}` : ''};
}

function answerRow(inv, person, redraw) {
  const row = el('div', 'reply-row');
  row.append(el('span', 'reply-name', person.id === host.me().id ? `${person.name} (you)` : person.name));
  const guest = {
    key: person.id, answer: person.answer ? person.answer.word : '', answerRow: person.answer ? person.answer.row : '', mine: true,
    answerGroups: answerGroupsOf(inv),
  };
  row.append(answerButtons(guest, asEvent(inv), () => redraw(), {small: false}));
  return row;
}

export function rsvpPanel(inv, details, redraw) {
  const panel = el('section', 'side-card reply-panel');
  panel.id = 'rsvp';
  const waiting = details.household.filter(p => !p.answer).length;
  panel.append(el('div', 'side-title', waiting ? 'Will you be there?' : 'Your RSVP'));
  if (!inv.rsvpYes || !inv.rsvpNo) {
    panel.append(el('p', 'hint', 'This invitation takes answers once it goes out.'));
    return panel;
  }
  panel.append(...replyRows(details, person => answerRow(inv, person, redraw), redraw));
  const add = calendarLinkOf(inv, location.href);
  if (add) {
    const a = el('a', 'button button-secondary reply-calendar');
    a.href = add;
    a.target = '_blank';
    a.rel = 'noopener';
    a.append(svg('calendar-plus'), el('span', '', 'Add to Calendar'));
    panel.append(a);
  }
  return panel;
}
