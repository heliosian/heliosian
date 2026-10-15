import {api} from '/api.js';
import {queries} from '/rail.js';
import {directory, listed, known, contactLine, emailOf, photoOf, wordsOf, gradeOf, isStudent, isParent, isStaff} from '/directory.js';
import {nameRulesWith, ruleOf, ruleRow} from '/listrules.js';
import {keyBetween} from '/order.js';
import {state, me, isParty} from './state.js';

const names = {groups: {}, people: {}};

nameRulesWith({group: id => names.groups[id] || null, person: id => names.people[id] || 'Someone'});

function rowsOf(answer, table) {
  return answer.result.map(id => answer.resources[table][id]);
}

function write(batch) {
  return api('POST', '/api/q', {batch});
}

function stamp() {
  const d = new Date();
  const pad = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function onGuestList(id) {
  return `(or (in id (select MEMBER.person (= group ${id})))
              (in id (select EFFECTIVE_MEMBER.person (= group ${id})))
              (in id (select RECIPIENT.person (in message (select MESSAGE.id (= group ${id}) (in kind "invitation" "skip")))))
              (in id (select MEMBER.person (in group (select GROUP.rsvp_yes (= id ${id})))))
              (in id (select MEMBER.person (in group (select GROUP.rsvp_no (= id ${id})))))
              (in id (select EFFECTIVE_MEMBER.person (in group (select GROUP.managed_by (= id ${id}))))))`;
}

export async function fetchInvites(e) {
  const id = JSON.stringify(e.id);
  const [a] = await Promise.all([queries({
    group: `(from GROUP (where (= id ${id})) (columns kind name managed_by visible_to members_visible_to join eligible flyer auto_invite rsvp_yes rsvp_no added_by status))`,
    runs: `(from GROUP @g (where (= id ${id}) (runs_list @g)) (columns kind))`,
    hostGroup: `(from GROUP (where (in id (select GROUP.managed_by (= id ${id})))) (columns visible_to))`,
    hosts: `(from MEMBER (where (in group (select GROUP.managed_by (= id ${id}))) (= member "yes")) (columns group person))`,
    members: `(from MEMBER (where (= group ${id})) (columns person member opened guest_of added_by added))`,
    paid: `(from PAYMENT (where (= group ${id}) (not (blank for))) (columns for))`,
    effective: `(from EFFECTIVE_MEMBER (where (= group ${id})) (columns person reasons))`,
    rules: `(from RULE (where (= group ${id})) (order order asc) (include target within))`,
    messages: `(from MESSAGE (where (= group ${id}) (in kind "invitation" "skip")) (columns kind from_person created))`,
    copies: `(from RECIPIENT (where (in message (select MESSAGE.id (= group ${id}) (in kind "invitation" "skip")))) (columns message person sent failed detail created))`,
    answers: `(from MEMBER (where (or (in group (select GROUP.rsvp_yes (= id ${id}))) (in group (select GROUP.rsvp_no (= id ${id}))))) (columns group person added added_by))`,
    people: `(from PERSON (where ${onGuestList(id)}) (columns name_show name_short source grade classroom slug))`,
    emails: `(from PERSON_EMAIL (where primary (in person (select PERSON.id ${onGuestList(id)}))) (columns person address))`,
    household: '(from PERSON (where (in id (household @viewer))) (columns name_show))',
    notify: '(from PERSON_SETTING (where (= person @viewer) (= app "when") (= key "notify_hosts")) (columns value))',
  }), directory()]);
  const g = rowsOf(a.group, 'GROUP')[0];
  const viewer = me().id;
  const host = a.runs.result.length > 0;
  const household = new Set([viewer, ...a.household.result]);
  const people = a.people.resources.PERSON || {};
  const emailOfId = Object.fromEntries(rowsOf(a.emails, 'PERSON_EMAIL').map(x => [x.person, x.address]));
  const dir = Object.fromEntries((await listed()).map(p => [p.id, p]));
  const memberOf = Object.fromEntries(rowsOf(a.members, 'MEMBER').map(m => [m.person, m]));
  const paid = new Set(rowsOf(a.paid, 'PAYMENT').map(p => p.for));
  const effective = new Map(rowsOf(a.effective, 'EFFECTIVE_MEMBER').map(x => [x.person, x]));
  const copies = {};
  for (const c of rowsOf(a.copies, 'RECIPIENT')) {
    copies[c.person] = c;
  }
  const answers = {};
  for (const m of rowsOf(a.answers, 'MEMBER')) {
    answers[m.person] = {row: m.id, word: m.group === g.rsvp_yes ? 'yes' : 'no', at: m.added, by: m.added_by};
  }
  const rules = rowsOf(a.rules, 'RULE').map(r => ({...ruleOf(r), order: r.order}));
  for (const r of Object.values(a.rules.resources.GROUP || {})) {
    names.groups[r.id] = r;
  }
  const nameOf = pid => (people[pid] && people[pid].name_show) || (dir[pid] && dir[pid].name_show) || 'Someone';
  for (const pid of Object.keys(people)) {
    names.people[pid] = nameOf(pid);
  }
  const rowFor = pid => {
    const d = dir[pid];
    const p = people[pid] || d || {};
    const m = memberOf[pid];
    const copy = copies[pid];
    const answer = answers[pid];
    const onList = effective.has(pid) || Boolean(copy);
    const outside = p.source === 'guest';
    const email = emailOfId[pid] || (d ? emailOf(d) : '');
    const reasons = (effective.get(pid) || {reasons: ''}).reasons.split(', ').filter(Boolean);
    const byRule = reasons.find(w => w.startsWith('rule'));
    return {
      key: pid, id: pid, email, name: nameOf(pid), photoUrl: d ? photoOf(d) : '',
      line: d ? contactLine(d) : outside ? 'Outside the directory' : '', words: d ? wordsOf(d) : 'Outside the directory', outside,
      isStudent: Boolean(d && isStudent(d)), isParent: Boolean(d && isParent(d)), isStaff: Boolean(d && isStaff(d)),
      grade: d && isStudent(d) ? gradeOf(d) : '', grades: d && isStudent(d) && gradeOf(d) ? [gradeOf(d)] : [], classrooms: [],
      answerGroups: {yes: g.rsvp_yes || '', no: g.rsvp_no || ''},
      answer: answer ? answer.word : '', answerRow: answer ? answer.row : '', answeredAt: answer ? answer.at : '', answeredBy: answer && answer.by && answer.by !== pid ? nameOf(answer.by) : '',
      invited: onList, sent: Boolean(copy), pending: onList && !copy, opened: m ? m.opened || '' : '',
      memberRow: m || null, byRule: Boolean(byRule), byHand: Boolean(m && m.member === 'yes'), via: byRule && !(m && m.member === 'yes') ? 'group:' + byRule : '',
      guestOf: m && m.guest_of ? m.guest_of : '', guestOfName: m && m.guest_of ? nameOf(m.guest_of) : '',
      invitedBy: m && m.added_by && !host ? nameOf(m.added_by) : '', invitedAt: m ? m.added || '' : '',
      mine: host || household.has(pid) || Boolean(m && m.guest_of && household.has(m.guest_of)),
      ticket: isParty(e) && m && m.member === 'yes' ? (paid.has(pid) ? 'ticket' : 'free') : '',
      warning: copy && copy.failed ? 'bounced' : '', warningWords: copy && copy.failed ? `Mail to this address failed: ${copy.detail || 'it could not be delivered'}.` : '',
    };
  };
  const listed_ = [...new Set([...effective.keys(), ...Object.keys(copies), ...Object.keys(answers)])].map(rowFor);
  const hostRows = rowsOf(a.hosts, 'MEMBER');
  const hostGroup = rowsOf(a.hostGroup, 'GROUP')[0];
  const sent = rowsOf(a.messages, 'MESSAGE').some(m => m.kind === 'invitation');
  const mine = listed_.filter(r => (household.has(r.key) || (r.guestOf && household.has(r.guestOf))) && (r.invited || r.answer));
  const notifyRow = rowsOf(a.notify, 'PERSON_SETTING')[0] || null;
  const myRow = memberOf[viewer];
  const mayInvite = host || (g.join === 'others' && (!g.eligible || effective.has(viewer)));
  return {
    id: e.id, group: g, host, sent, rules, mine, mayInvite,
    list: host ? listed_ : listed_.filter(r => r.answer !== 'no'),
    coming: host ? listed_ : listed_.filter(r => r.answer !== 'no'),
    counts: {yes: listed_.filter(r => r.answer === 'yes').length, maybe: 0, no: listed_.filter(r => r.answer === 'no').length},
    hosts: hostRows.map(h => ({...rowFor(h.person), memberRow: h})),
    managedBy: g.managed_by || '', hostsHidden: Boolean(hostGroup) && !hostGroup.visible_to,
    listPrivate: !g.members_visible_to, guests: g.join === 'others', autoInvite: g.auto_invite === 'Yes',
    notifyMe: Boolean(notifyRow && notifyRow.value.split(',').map(s => s.trim()).includes(e.id)), notifyRow,
    flyer: g.flyer ? `/api/blob/${e.id}/flyer` : '',
    party: isParty(e), linked: e.source === 'celebrate' || e.source === 'team',
    can: {opened: Boolean(effective.has(viewer) || copies[viewer]) && !(myRow && myRow.opened)},
    myRow: myRow || null,
  };
}

export async function noteOpened(e, view) {
  const at = stamp();
  await write([view.myRow ? {set: view.myRow.id, cells: {opened: at}} : {insert: 'MEMBER', row: {group: e.id, person: me().id, opened: at}}]);
}

export async function answerFor(row, word) {
  const batch = row.answerRow ? [{delete: row.answerRow}] : [];
  const group = word ? row.answerGroups[word] : '';
  if (word && !group) {
    throw new Error('This guest list takes answers once its invitation goes out.');
  }
  if (group) {
    batch.push({insert: 'MEMBER', row: {group, person: row.key, member: 'yes', added_by: me().id, added: stamp()}});
  }
  if (batch.length) {
    await write(batch);
  }
}

export function sendTo(e, people) {
  return api('POST', '/api/do/invite', {group: e.id, people});
}

export function sendPending(e) {
  return api('POST', '/api/do/invite', {group: e.id});
}

export function skip(e, people) {
  return api('POST', '/api/do/invite', {group: e.id, people, skip: true});
}

export async function addPeople(e, view, people) {
  const helios = [];
  const families = new Map();
  for (const p of people) {
    const d = p.email ? known(p.email) : null;
    if (d) {
      helios.push({person: d.id});
      continue;
    }
    const key = p.household || p.email || p.name;
    families.set(key, [...(families.get(key) || []), {name: p.name, email: p.email || ''}]);
  }
  const send = !view.host;
  if (helios.length) {
    await api('POST', '/api/do/add-guests', {group: e.id, people: helios, send});
  }
  for (const family of families.values()) {
    await api('POST', '/api/do/add-guests', {group: e.id, people: family, family: family.length > 1, send});
  }
}

export function bringGuest(e, body) {
  const d = body.email ? known(body.email) : null;
  const guest = d ? {person: d.id} : {name: body.name, email: body.email || ''};
  return api('POST', '/api/do/add-guests', {group: e.id, people: [guest], guest_of: body.of, answer: body.answer, send: Boolean(body.invite)});
}

export async function takeOff(e, view, row) {
  const batch = [];
  if (row.memberRow && !row.byRule) {
    batch.push({delete: row.memberRow.id});
  } else if (row.memberRow) {
    batch.push({set: row.memberRow.id, cells: {member: 'excluded'}});
  } else {
    batch.push({insert: 'MEMBER', row: {group: e.id, person: row.key, member: 'excluded', added_by: me().id, added: stamp()}});
  }
  if (row.answerRow) {
    batch.push({delete: row.answerRow});
  }
  await write(batch);
}

export function setPublicList(e, on) {
  return write([{set: e.id, cells: {members_visible_to: on ? state.model.everyone : ''}}]);
}

export function setGuestsInvite(e, on) {
  return write([{set: e.id, cells: on ? {join: 'others', eligible: e.id} : {join: '', eligible: ''}}]);
}

export function setAutoInvite(e, on) {
  return write([{set: e.id, cells: {auto_invite: on}}]);
}

export function setFlyer(e, image) {
  return write([{set: e.id, cells: {flyer: image}}]);
}

export async function setNotifyMe(e, view, on) {
  const now = view.notifyRow ? view.notifyRow.value.split(',').map(s => s.trim()).filter(Boolean) : [];
  const next = on ? [...new Set([...now, e.id])] : now.filter(x => x !== e.id);
  if (!view.notifyRow) {
    await write([{insert: 'PERSON_SETTING', row: {person: me().id, app: 'when', key: 'notify_hosts', value: next.join(', ')}}]);
    return;
  }
  await write([{set: view.notifyRow.id, cells: {value: next.join(', ')}}]);
}

export function addHost(e, view, person) {
  return write([{insert: 'MEMBER', row: {group: view.managedBy, person, member: 'yes', added_by: me().id, added: stamp()}}]);
}

export function removeHost(e, view, host) {
  return write([{delete: host.memberRow.id}]);
}

export function setHostsHidden(e, view, hidden) {
  return write([{set: view.managedBy, cells: {visible_to: hidden ? '' : view.managedBy}}]);
}

export async function saveRules(e, view, rules) {
  const batch = view.rules.map(r => ({delete: r.id}));
  let order = '';
  for (const r of rules) {
    order = keyBetween(order, null);
    batch.push({insert: 'RULE', row: ruleRow(e.id, r, order)});
  }
  if (batch.length) {
    await write(batch);
  }
}

export async function addRule(e, view, rule) {
  const last = view.rules.length ? view.rules[view.rules.length - 1].order : '';
  await write([{insert: 'RULE', row: ruleRow(e.id, rule, keyBetween(last || '', null))}]);
}

export function previewRules(e, view, rules) {
  const hand = view.list.filter(r => r.memberRow && r.memberRow.member).map(r => ({person: r.key, member: r.memberRow.member}));
  return api('POST', '/api/do/draft-members', {list: e.id, rules: rules.map(r => ({exclude: r.exclude, target: r.target, replace_with: r.replace_with, within: r.within, person: r.person, search: r.search})), members: hand});
}

export function mailList(e, body) {
  return api('POST', '/api/do/guest-mail', {group: e.id, ...body});
}

export function cancelEvent(e, notify, note) {
  return mailList(e, {kind: 'cancel', notify, text: note || ''});
}

export function sendUpdate(e) {
  return mailList(e, {kind: 'update'});
}

export function deleteEvent(e) {
  return write([{set: e.id, cells: {status: 'closed'}}]);
}
