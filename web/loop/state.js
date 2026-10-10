import {api, signedIn} from '/api.js';
import {me as whoAmI} from '/data.js';
import {listed, known, contactLine, emailOf, photoOf, wordsOf, gradeOf, isStudent, directory} from '/directory.js';
import {navQueries, navLists, viewerListCondition} from '/rail.js';

export const domain = 'loop.heliosian.com';

export const state = {model: null, people: []};

let byId = {};
let bySlug = {};

export async function query(named) {
  const res = await signedIn(await fetch('/api/q', {method: 'QUERY', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({queries: named})}));
  if (!res.ok) {
    throw new Error(await res.text());
  }
  const answer = await res.json();
  const out = {};
  for (const name of Object.keys(named)) {
    out[name] = {result: answer.results[name], resources: answer.resources};
  }
  return out;
}

export function rowsOf(answer, table) {
  return answer.result.map(id => answer.resources[table][id]);
}

export function write(batch) {
  return api('POST', '/api/q', {batch});
}

const lists = `(select GROUP.id @g ${viewerListCondition})`;

function groupQueries(sel) {
  const into = `(or (in group ${sel}) (in group (select GROUP.rsvp_yes (in id ${sel}))))`;
  return {
    groups: `(from GROUP @g (where (in id ${sel})) (order name asc))`,
    runs: `(from GROUP @g (where (in id ${sel}) (runs_list @g)) (columns slug))`,
    members: `(from EFFECTIVE_MEMBER (where ${into}) (columns group person reasons))`,
    hand: `(from MEMBER (where (in group ${sel})) (columns group person member note added))`,
    managers: `(from MEMBER (where (= member "yes") (in group (select GROUP.managed_by (in id ${sel})))) (columns group person))`,
    rules: `(from RULE (where (in group ${sel})) (order order asc) (include target within))`,
    aliases: `(from ALIAS (where (in target ${sel})))`,
    guests: `(from PERSON (where (= source "guest") (or (in id (select MEMBER.person (in group ${sel}))) (in id (select EFFECTIVE_MEMBER.person ${into})))))`,
    guestEmails: `(from PERSON_EMAIL (where guest (in person (select MEMBER.person (in group ${sel})))) (columns person address))`,
    sent: `(from MESSAGE (where (= kind "post") (= direction "in") (= state "sent") (in group ${sel})) (columns group))`,
    unsubscribed: `(from MEMBER (where (= person @viewer) (in group (select GROUP.unsubscribed (in id ${sel})))) (columns group person))`,
  };
}

export function personOf(id) {
  return state.model.people[id] || null;
}

export function personView(id) {
  const p = personOf(id);
  if (!p) {
    return {person: id, email: '', name: 'Someone you cannot see', words: '', context: ''};
  }
  if (p.source === 'guest') {
    const email = state.model.guestEmails[id] || '';
    return {person: id, email, name: p.name_show || email, words: 'Outside the directory', context: email, outside: true};
  }
  const email = emailOf(p);
  return {person: id, email, name: p.name_show, photoUrl: photoOf(p), words: wordsOf(p), grade: isStudent(p) ? gradeOf(p) : '', context: contactLine(p)};
}

function visibility(g, everyone) {
  if (!g.visible_to) {
    return 'hidden';
  }
  return g.visible_to === g.id ? 'members' : g.visible_to === everyone ? 'everyone' : 'hidden';
}

const reasonOf = (rules, word) => {
  if (word === 'member') {
    return {added: true};
  }
  const order = word.replace(/^rule /, '');
  const rule = rules.findIndex(r => r.order === order);
  return rule < 0 ? null : {rule};
};

function buildGroups(answers) {
  const {viewer, everyone, people, guestEmails, groupNames} = state.model;
  const runs = new Set(answers.runs.result);
  for (const p of rowsOf(answers.guests, 'PERSON')) {
    people[p.id] = p;
  }
  for (const e of rowsOf(answers.guestEmails, 'PERSON_EMAIL')) {
    guestEmails[e.person] = e.address;
  }
  for (const g of Object.values(answers.rules.resources.GROUP || {})) {
    groupNames[g.id] = g;
  }
  const by = (answer, table, key) => {
    const out = {};
    for (const row of rowsOf(answer, table)) {
      (out[row[key]] = out[row[key]] || []).push(row);
    }
    return out;
  };
  const members = by(answers.members, 'EFFECTIVE_MEMBER', 'group');
  const hand = by(answers.hand, 'MEMBER', 'group');
  const managers = by(answers.managers, 'MEMBER', 'group');
  const rules = by(answers.rules, 'RULE', 'group');
  const aliases = by({...answers.aliases, result: rowsOf(answers.aliases, 'ALIAS').filter(a => !/[A-Z]/.test(a.alias)).map(a => a.id)}, 'ALIAS', 'target');
  const sent = by(answers.sent, 'MESSAGE', 'group');
  const off = new Set(rowsOf(answers.unsubscribed, 'MEMBER').map(m => m.group));
  return rowsOf(answers.groups, 'GROUP').map(g => {
    const listRules = rules[g.id] || [];
    const source = g.kind === 'event' && g.rsvp_yes ? g.rsvp_yes : g.id;
    const shown = (members[source] || []).map(e => ({...personView(e.person), reasons: e.reasons.split(', ').map(w => reasonOf(listRules, w)).filter(Boolean)}));
    return {
      id: g.id,
      slug: g.slug,
      title: g.name,
      address: g.slug ? `${g.slug}@${domain}` : '',
      description: g.description || '',
      posting: g.posting,
      replying: g.replying,
      visibility: visibility(g, everyone),
      managedBy: g.managed_by,
      aliases: (aliases[g.id] || []).map(a => a.alias),
      aliasRows: aliases[g.id] || [],
      rules: listRules,
      hand: hand[g.id] || [],
      members: shown,
      managers: (managers[g.managed_by] || []).map(m => personView(m.person)),
      managerRows: managers[g.managed_by] || [],
      run: runs.has(g.id),
      member: Boolean(viewer && shown.some(m => m.person === viewer.id)),
      unsubscribed: off.has(g.unsubscribed),
      sent: (sent[g.id] || []).length,
    };
  });
}

function remember(g, ...keys) {
  byId[g.id] = g;
  for (const key of [g.slug, ...keys]) {
    if (key) {
      bySlug[key] = g;
    }
  }
}

export async function loadModel() {
  const [who, answers, people] = await Promise.all([whoAmI(), query({
    viewer: '(from PERSON (where (= id @viewer)))',
    everyone: '(from GROUP (where (= kind "group") (= slug "everyone")) (columns slug))',
    grades: '(from GROUP (where (= kind "grade")) (columns name color))',
    ...groupQueries(lists),
    ...navQueries(),
  }), listed()]);
  await directory();
  const peopleById = {};
  for (const p of people) {
    peopleById[p.id] = p;
  }
  const gradeColors = {};
  for (const g of rowsOf(answers.grades, 'GROUP')) {
    if (g.color) {
      gradeColors[g.name] = g.color;
    }
  }
  state.people = people;
  state.model = {
    nav: navLists(answers),
    viewer: answers.viewer.result.length ? rowsOf(answers.viewer, 'PERSON')[0] : null,
    email: who.email,
    everyone: (answers.everyone.result || [])[0] || '',
    people: peopleById,
    guestEmails: {},
    groupNames: {},
    gradeColors,
    groups: [],
  };
  state.model.groups = buildGroups(answers);
  byId = {};
  bySlug = {};
  for (const g of state.model.groups) {
    remember(g);
  }
}

export async function fetchGroup(key) {
  const column = /^grp[0-9A-Za-z]{11}$/.test(key) ? 'id' : 'slug';
  const [g] = buildGroups(await query(groupQueries(`(select GROUP.id (!= status "closed") (= ${column} ${JSON.stringify(key)}))`)));
  if (g) {
    remember(g, key);
  }
  return g || null;
}

export function person(email) {
  return known(email);
}

export function me() {
  const v = state.model.viewer;
  const name = (v && v.name_show) || state.model.email;
  return {email: state.model.email, name, initial: name[0].toUpperCase(), photoUrl: v ? photoOf(v) : ''};
}

export function group(slug) {
  return bySlug[slug] || byId[slug] || null;
}

export function groupPath(g) {
  return `/groups/${encodeURIComponent(g.slug)}`;
}

export function matches(g, query) {
  if (!query) {
    return true;
  }
  return `${g.title} ${g.address} ${g.description || ''}`.toLowerCase().includes(query);
}
