import {queries, rowsOf, write} from '/rail.js';
import {sheetStamp} from '/stamp.js';
import {api} from '/api.js';
import {gradebandsOf} from '/gradebands.js';
import {loadThemes, forgetThemes} from '/themes.js';
import {keyBetween} from '/order.js';
import {postTheme} from '/themeupload.js';
import {invitationColumns as columns, mineOrHousehold, gatherInvitation, invitationRow, inviteOf, lookThemes, answerAs, fetchPages} from '/invitation.js';

export const state = {model: null, pages: {}};

const byKey = new Map();
const hostedByMe = '(in managed_by (select EFFECTIVE_MEMBER.group (= person @viewer)))';

export async function uploadTheme(files) {
  const id = await postTheme(files);
  if (!state.model.usable.includes(id)) {
    state.model.usable.push(id);
  }
  return id;
}

export async function removeTheme(id) {
  await write([{delete: id}]);
  forgetThemes([id]);
  state.model.usable = state.model.usable.filter(x => x !== id);
}

const gatherHosts = `(select GROUP.managed_by ${gatherInvitation})`;

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
    hosting: `(from GROUP @g (where ${gatherInvitation} (!= status "closed") ${hostedByMe}) (order start asc) ${columns})`,
    invited: `(from GROUP @g (where ${gatherInvitation} (in status "open" "cancelled") (not ${hostedByMe}) (in id (select EFFECTIVE_MEMBER.group ${mineOrHousehold}))) (order start asc) ${columns})`,
    misc: '(from GROUP (where (= kind "category") (= slug "misc")) (columns name))',
    everyone: '(from GROUP (where (= slug "everyone")) (columns name))',
    classrooms: '(from GROUP (where (= kind "classroom")) (order order asc) (columns name color parent))',
    gradebands: '(from GROUP (where (= kind "gradeband")) (columns name))',
    hostRows: `(from EFFECTIVE_MEMBER (where (in group ${gatherHosts})) (columns group person))`,
    pageWords: `(from INVITE_PAGE (where (in group (select GROUP.id ${gatherInvitation}))) (columns group title body))`,
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

export function answerAsMe(e, word) {
  return answerAs(findInvite(e.id), word);
}

export function me() {
  return state.model.user;
}

export function findInvite(key) {
  return byKey.get(key) || null;
}

export async function fetchInvite(key) {
  const g = await invitationRow(key);
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

export async function loadPages(inv) {
  state.pages[inv.id] = await fetchPages(inv);
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
    {set: '@event', cells: {visible_to: body.anyoneWithLink ? state.model.everyone : '@event', resend_days: '3'}},
    {insert: 'MEMBER', row: {group: '@event', person: me().id, member: 'yes', added_by: me().id, added: sheetStamp()}},
  ]);
  return id;
}

export async function editInvite(inv, body) {
  await write([{set: inv.id, cells: {...inviteCells(body), visible_to: body.anyoneWithLink ? state.model.everyone : inv.id}}]);
}
