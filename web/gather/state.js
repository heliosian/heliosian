import {queries, rowsOf, write} from '/rail.js';
import {sheetStamp} from '/stamp.js';
import {api} from '/api.js';
import {parseWhen, timeRange} from '/datecard.js';
import {gradebandsOf} from '/gradebands.js';
import {loadThemes, refreshThemes, forgetThemes} from '/themes.js';
import {keyBetween} from '/order.js';
import {whereWords} from '/envelope.js';
import {questionQueries, questionParts} from '/rsvpquestions.js';

export const state = {model: null, pages: {}};

const byKey = new Map();
const columns = '(columns name subtitle managed_by start end all_day location place description theme envelope seal liner design card_picture backdrop_picture hosted_by path slug status rsvp_yes rsvp_no rsvp_maybe maybe members_visible_to when_href)';
const mineOrHousehold = '(or (= person @viewer) (in person (household @viewer)))';
const hostedByMe = '(in managed_by (select EFFECTIVE_MEMBER.group (= person @viewer)))';

function pictureOf(g, column) {
  return g[column] ? `/api/blob/${g.id}/${column}?c=${g[column]}` : '';
}

function inviteOf(g, host) {
  return {
    id: g.id, name: g.name || 'Untitled invitation', subtitle: g.subtitle || '', start: g.start || '', end: g.end || '', allDay: g.all_day === 'Yes',
    location: g.location || '', place: g.place || '', description: g.description || '', theme: g.theme || '', hostedBy: g.hosted_by || '',
    look: {
      theme: g.theme || '', envelope: g.envelope || '', seal: g.seal || '', liner: g.liner || '', design: g.design ? JSON.parse(g.design) : {},
      cardPicture: pictureOf(g, 'card_picture'), backdrop: pictureOf(g, 'backdrop_picture'),    },
    path: g.path || `/g/${g.id}`,
    slug: g.slug || '', status: g.status || '', rsvpYes: g.rsvp_yes || '', rsvpNo: g.rsvp_no || '', rsvpMaybe: g.rsvp_maybe || '', maybe: g.maybe === 'Yes' && Boolean(g.rsvp_maybe),
    listShown: Boolean(g.members_visible_to), whenHref: g.when_href || '', host, managers: g.managed_by || '', hostNames: [], pageWords: '',
  };
}

function lookThemes(inv) {
  return [inv.look.theme, inv.look.envelope, inv.look.seal, inv.look.liner];
}

export async function uploadTheme(files) {
  const form = new FormData();
  for (const f of files) {
    form.append('file', f);
  }
  const {result} = await api('POST', '/api/do/theme', form);
  await refreshThemes([result[0]]);
  if (!state.model.usable.includes(result[0])) {
    state.model.usable.push(result[0]);
  }
  return result[0];
}

export async function removeTheme(id) {
  await write([{delete: id}]);
  forgetThemes([id]);
  state.model.usable = state.model.usable.filter(x => x !== id);
}

const gatherHosts ='(select GROUP.managed_by (= kind "event") (not (blank theme)))';

function remember(inv) {
  byKey.set(inv.id, inv);
  if (inv.slug) {
    byKey.set(inv.slug, inv);
  }
}

export async function loadModel() {
  const [a] = await Promise.all([queries({
    me: '(from PERSON (where (= id @viewer)) (columns name_show))',
    email: '(from PERSON_EMAIL (where (= person @viewer) primary) (columns address))',
    photo: '(from PHOTO (where (= person @viewer) ready) (order order asc) (columns person))',
    hosting: `(from GROUP @g (where (= kind "event") (not (blank theme)) (!= status "closed") ${hostedByMe}) (order start asc) ${columns})`,
    invited: `(from GROUP @g (where (= kind "event") (not (blank theme)) (in status "open" "cancelled") (not ${hostedByMe}) (in id (select EFFECTIVE_MEMBER.group ${mineOrHousehold}))) (order start asc) ${columns})`,
    misc: '(from GROUP (where (= kind "category") (= slug "misc")) (columns name))',
    everyone: '(from GROUP (where (= slug "everyone")) (columns name))',
    classrooms: '(from GROUP (where (= kind "classroom")) (order order asc) (columns name color parent))',
    gradebands: '(from GROUP (where (= kind "gradeband")) (columns name))',
    hostRows: `(from EFFECTIVE_MEMBER (where (in group ${gatherHosts})) (columns group person))`,
    pageWords: '(from INVITE_PAGE (where (in group (select GROUP.id (= kind "event") (not (blank theme))))) (columns group title body))',
    hostPeople: `(from PERSON (where (in id (select EFFECTIVE_MEMBER.person (in group ${gatherHosts})))) (order name_sort asc) (columns name_show))`,
    usable: '(from THEME (where (or (blank added_by) (= added_by @viewer))) (columns key))',
    gatherAdmin: '(from APP (where (= key "gather") (admin_of "gather")) (columns key))',
  })]);
  const person = rowsOf(a.me, 'PERSON')[0] || {};
  const email = rowsOf(a.email, 'PERSON_EMAIL')[0];
  const photo = rowsOf(a.photo, 'PHOTO')[0];
  const name = person.name_show || '';
  byKey.clear();
  const hosting = rowsOf(a.hosting, 'GROUP').map(g => inviteOf(g, true));
  const invited = rowsOf(a.invited, 'GROUP').map(g => inviteOf(g, false));
  const people = rowsOf(a.hostPeople, 'PERSON');
  const rows = rowsOf(a.hostRows, 'EFFECTIVE_MEMBER');
  const pageRows = rowsOf(a.pageWords, 'INVITE_PAGE');
  await loadThemes([...hosting, ...invited].flatMap(lookThemes));
  for (const inv of [...hosting, ...invited]) {
    const ids = new Set(rows.filter(r => r.group === inv.managers).map(r => r.person));
    inv.hostNames = people.filter(p => ids.has(p.id)).map(p => p.name_show);
    inv.pageWords = pageRows.filter(p => p.group === inv.id).map(p => `${p.title} ${p.body || ''}`).join(' ');
    remember(inv);
  }
  state.model = {
    user: {id: person.id, name, email: email ? email.address : '', initial: (name || '?')[0].toUpperCase(), photoUrl: photo ? `/api/blob/${photo.id}/thumbnail` : ''},
    hosting, invited, usable: rowsOf(a.usable, 'THEME').map(t => t.id),
    misc: (rowsOf(a.misc, 'GROUP')[0] || {}).id || '', gatherAdmin: rowsOf(a.gatherAdmin, 'APP').length > 0,
    everyone: (rowsOf(a.everyone, 'GROUP')[0] || {}).id || '',
    classrooms: rowsOf(a.classrooms, 'GROUP').map(c => ({id: c.id, name: c.name, color: c.color || '', gradeband: (rowsOf(a.gradebands, 'GROUP').find(b => b.id === c.parent) || {}).name || ''})),
  };
}

export function gradebands() {
  return gradebandsOf(state.model.classrooms);
}

export function asEvent(inv) {
  const day = inv.start.slice(0, 10);
  return {
    id: inv.id, title: inv.name, source: 'sheet', allDay: inv.allDay, location: inv.location, where: whereWords(inv), cancelled: inv.status === 'cancelled',
    start: inv.start, end: inv.end, dates: day ? [day] : [], path: inv.path, can: {edit: inv.host}, hostsAreGuests: true,
  };
}

function answerGroupOf(inv, word) {
  return {yes: inv.rsvpYes, no: inv.rsvpNo, maybe: inv.maybe ? inv.rsvpMaybe : ''}[word] || '';
}

export async function answerAsMe(e, word) {
  const inv = findInvite(e.id);
  const groups = [inv.rsvpYes, inv.rsvpNo, inv.rsvpMaybe].filter(Boolean).map(g => JSON.stringify(g)).join(' ');
  const a = await queries({mine: `(from MEMBER (where (in group ${groups}) (= person @viewer)) (columns group))`});
  const batch = rowsOf(a.mine, 'MEMBER').map(m => ({delete: m.id}));
  if (answerGroupOf(inv, word)) {
    batch.push({insert: 'MEMBER', row: {group: answerGroupOf(inv, word), person: me().id, member: 'yes', added_by: me().id, added: sheetStamp()}});
  }
  if (batch.length) {
    await write(batch);
  }
}

export function me() {
  return state.model.user;
}

export function findInvite(key) {
  return byKey.get(key) || null;
}

export async function fetchInvite(key) {
  const k = JSON.stringify(key);
  const a = await queries({
    group: `(from GROUP @g (where (= kind "event") (or (= id ${k}) (= slug ${k}))) ${columns})`,
    alias: `(from ALIAS (where (= alias ${JSON.stringify(key.toLowerCase())})) (columns target))`,
  });
  let g = rowsOf(a.group, 'GROUP')[0];
  if (!g) {
    const alias = rowsOf(a.alias, 'ALIAS')[0];
    if (!alias) {
      return null;
    }
    const b = await queries({group: `(from GROUP @g (where (= id ${JSON.stringify(alias.target)})) ${columns})`});
    g = rowsOf(b.group, 'GROUP')[0];
  }
  if (!g) {
    return null;
  }
  const runs = await queries({runs: `(from GROUP @g (where (= id ${JSON.stringify(g.id)}) (runs_list @g)) (columns kind))`});
  const inv = inviteOf(g, runs.runs.result.length > 0);
  await loadThemes(lookThemes(inv));
  remember(inv);
  byKey.set(key, inv);
  return inv;
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
  return {
    ...questionParts(a),
    hosts: rowsOf(a.hosts, 'PERSON').map(p => p.name_show),
    household: rowsOf(a.household, 'PERSON').map(p => ({
      id: p.id, name: p.name_show, short: p.name_short || p.name_show, answer: answers[p.id] || null, going: Boolean(answers[p.id] && answers[p.id].word === 'yes'),
      kid: students.has(p.id) || p.child === 'Yes',
    })).sort((x, y) => (y.id === me().id) - (x.id === me().id)),
    myRow: rowsOf(a.mine, 'MEMBER')[0] || null,
    counts: {guests: a.guests.result.length, yes: tally.filter(m => m.group === inv.rsvpYes).length, no: tally.filter(m => m.group === inv.rsvpNo).length},
    names: Object.fromEntries([...rowsOf(a.repliers, 'PERSON').map(p => [p.id, p.name_show]), ...rowsOf(a.families, 'GROUP').map(g => [g.id, g.name])]),
  };
}

export async function loadPages(inv) {
  const a = await queries({pages: `(from INVITE_PAGE (where (= group ${JSON.stringify(inv.id)})) (order order asc) (columns order title body background picture))`});
  state.pages[inv.id] = rowsOf(a.pages, 'INVITE_PAGE').map(p => ({...p, background: p.background || '', pictureUrl: p.picture ? `/api/blob/${p.id}/picture?c=${p.picture}` : ''}));
  return state.pages[inv.id];
}

export function pagesOf(inv) {
  return state.pages[inv.id] || [];
}

export async function savePage(inv, page, body) {
  const cells = {title: body.title, body: body.body, background: body.background};
  if (page) {
    await write([{set: page.id, cells}]);
    return page.id;
  }
  const list = pagesOf(inv);
  const after = list.length ? list[list.length - 1].order : undefined;
  const {result: [id]} = await write([{insert: 'INVITE_PAGE', row: {...cells, group: inv.id, order: keyBetween(after, undefined)}}]);
  return id;
}

export async function setPagePicture(page, file) {
  const form = new FormData();
  form.append('page', page.id);
  form.append('picture', file);
  await api('POST', '/api/do/page-picture', form);
}

export async function clearPagePicture(page) {
  await write([{set: page.id, cells: {picture_original: ''}}]);
}

export async function movePage(inv, id, ids) {
  const byId = new Map(pagesOf(inv).map(p => [p.id, p]));
  const at = ids.indexOf(id);
  const before = at > 0 ? byId.get(ids[at - 1]).order : undefined;
  const after = at < ids.length - 1 ? byId.get(ids[at + 1]).order : undefined;
  const order = keyBetween(before, after);
  byId.get(id).order = order;
  state.pages[inv.id] = ids.map(x => byId.get(x));
  await write([{set: id, cells: {order}}]);
}

export async function setDesignPicture(inv, part, file) {
  const form = new FormData();
  form.append('group', inv.id);
  form.append('part', part);
  form.append('picture', file);
  await api('POST', '/api/do/design-picture', form);
}

export async function clearDesignPicture(inv, part) {
  await write([{set: inv.id, cells: {[`${part}_picture_original`]: ''}}]);
}

export async function removePage(page) {
  await write([{delete: page.id}]);
}

function questionCells(body) {
  return {text: body.text, kind: body.kind, choices: body.choices, asked: body.asked, required: body.required, going_only: body.goingOnly};
}

export async function addQuestion(inv, body, after) {
  await write([{insert: 'QUESTION', row: {...questionCells(body), group: inv.id, order: keyBetween(after, undefined)}}]);
}

export async function editQuestion(q, body) {
  await write([{set: q.id, cells: questionCells(body)}]);
}

export async function removeQuestion(q) {
  await write([{delete: q.id}]);
}

export async function noteOpened(inv, details) {
  if (details.myRow && details.myRow.opened) {
    return;
  }
  if (!details.household.some(p => p.id === me().id)) {
    return;
  }
  const at = sheetStamp();
  await write([details.myRow ? {set: details.myRow.id, cells: {opened: at}} : {insert: 'MEMBER', row: {group: inv.id, person: me().id, opened: at}}]);
}

function inviteCells(body) {
  return {
    name: body.name, subtitle: body.subtitle, start: body.start, end: body.end || body.start, all_day: Boolean(body.start) && !body.start.includes(' '),
    location: body.location, place: body.place, description: body.description, theme: body.theme, envelope: body.envelope, seal: body.seal, liner: body.liner,
    design: Object.keys(body.design).length ? JSON.stringify(body.design) : '', hosted_by: body.hostedBy,
  };
}

export async function createInvite(body) {
  const {result: [id]} = await write([
    {insert: 'GROUP', as: 'event', row: {...inviteCells(body), kind: 'event', parent: state.model.misc, status: 'open', added_by: me().id, listed: false, members_visible_to: state.model.everyone}},
    {insert: 'GROUP', as: 'hosts', row: {kind: 'group', name: body.name + ' Managers', status: 'open', added_by: me().id}},
    {set: '@hosts', cells: {managed_by: '@hosts'}},
    {insert: 'MEMBER', row: {group: '@hosts', person: me().id, member: 'yes'}},
    {set: '@event', cells: {managed_by: '@hosts'}},
    {set: '@event', cells: {visible_to: '@event', resend_days: '3'}},
    {insert: 'MEMBER', row: {group: '@event', person: me().id, member: 'yes', added_by: me().id, added: sheetStamp()}},
  ]);
  return id;
}

export async function editInvite(inv, body) {
  await write([{set: inv.id, cells: inviteCells(body)}]);
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
