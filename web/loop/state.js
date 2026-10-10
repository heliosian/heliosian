import {api, signedIn} from '/api.js';
import {me as whoAmI} from '/data.js';
import {listed, known, contactLine, emailOf, photoOf, wordsOf, gradeOf, isStudent, directory} from '/directory.js';

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

const listCondition = '(mail_list @g) (!= status "closed")';
const lists = `(select GROUP.id @g ${listCondition})`;
const runsDirectly = '(in managed_by (select EFFECTIVE_MEMBER.group (= person @viewer)))';

function suggestions(answers, named) {
  const today = new Date().toLocaleDateString('en-CA');
  const running = rowsOf(answers.running, 'GROUP');
  const byId = {};
  for (const g of running) {
    byId[g.id] = g;
  }
  const runners = {};
  for (const m of rowsOf(answers.runners, 'MEMBER')) {
    (runners[m.group] = runners[m.group] || []).push(m.person);
  }
  const under = root => running.filter(g => {
    for (let at = byId[g.parent]; at; at = byId[at.parent]) {
      if (at === root) {
        return true;
      }
    }
    return false;
  });
  const out = [];
  for (const g of running) {
    const past = (g.end || g.start || '9999').slice(0, 10) < today;
    if (byId[g.parent] || past || named.has(g.id)) {
      continue;
    }
    out.push({id: g.id, name: g.name, kind: g.kind, targets: [g.id, ...under(g).map(x => x.id)], managers: runners[g.managed_by] || []});
  }
  for (const g of rowsOf(answers.tags, 'GROUP')) {
    if (!named.has(g.id)) {
      out.push({id: g.id, name: g.name, kind: 'tag', targets: [g.id], managers: []});
    }
  }
  return out;
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

export async function loadModel() {
  const [who, answers, people] = await Promise.all([whoAmI(), query({
    viewer: '(from PERSON (where (= id @viewer)))',
    admin: '(from PERSON (where (= id @viewer) (admin_of "loop")) (columns source))',
    everyone: '(from GROUP (where (= kind "group") (= slug "everyone")) (columns slug))',
    lists: `(from GROUP @g (where ${listCondition}) (order name asc))`,
    runs: `(from GROUP @g (where ${listCondition} (runs_list @g)) (columns slug))`,
    members: `(from EFFECTIVE_MEMBER (where (in group ${lists})) (columns group person reasons))`,
    hand: `(from MEMBER (where (in group ${lists})) (columns group person member note added))`,
    managers: `(from MEMBER (where (= member "yes") (in group (select GROUP.managed_by @g ${listCondition}))) (columns group person))`,
    rules: `(from RULE (where (in group ${lists})) (order order asc) (include target within))`,
    aliases: `(from ALIAS (where (in target ${lists})))`,
    guests: `(from PERSON (where (= source "guest") (or (in id (select MEMBER.person (in group ${lists}))) (in id (select EFFECTIVE_MEMBER.person (in group ${lists}))))))`,
    guestEmails: `(from PERSON_EMAIL (where guest (in person (select MEMBER.person (in group ${lists})))) (columns person address))`,
    sent: `(from MESSAGE (where (= kind "post") (= direction "in") (= state "sent") (in group ${lists})) (columns group))`,
    grades: '(from GROUP (where (= kind "grade")) (columns name color))',
    running: `(from GROUP @g (where (in kind "activity" "party") (not (blank parent)) (!= status "closed") ${runsDirectly}) (order name asc) (columns name kind parent start end managed_by))`,
    tags: `(from GROUP @g (where (own_group @g) (not (blank managed_by)) (!= status "closed") ${runsDirectly} (not (exists GROUP (= managed_by @g) (!= id @g)))) (order name asc) (columns name kind managed_by))`,
    runners: `(from MEMBER (where (= member "yes") (in group (select GROUP.managed_by @g (in kind "activity" "party") ${runsDirectly}))) (columns group person))`,
  }), listed()]);
  await directory();
  const everyone = (answers.everyone.result || [])[0] || '';
  const viewer = answers.viewer.result.length ? rowsOf(answers.viewer, 'PERSON')[0] : null;
  const runs = new Set(answers.runs.result);
  const peopleById = {};
  for (const p of people) {
    peopleById[p.id] = p;
  }
  for (const p of rowsOf(answers.guests, 'PERSON')) {
    peopleById[p.id] = p;
  }
  const guestEmails = {};
  for (const e of rowsOf(answers.guestEmails, 'PERSON_EMAIL')) {
    guestEmails[e.person] = e.address;
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
  const aliases = by(answers.aliases, 'ALIAS', 'target');
  const sent = by(answers.sent, 'MESSAGE', 'group');
  const groupNames = {};
  for (const g of Object.values(answers.rules.resources.GROUP || {})) {
    groupNames[g.id] = g;
  }
  const gradeColors = {};
  for (const g of rowsOf(answers.grades, 'GROUP')) {
    if (g.color) {
      gradeColors[g.name] = g.color;
    }
  }
  state.people = people;
  const named = new Set(rowsOf(answers.rules, 'RULE').map(r => r.target).filter(Boolean));
  state.model = {
    suggestions: suggestions(answers, named),
    viewer,
    email: who.email,
    admin: answers.admin.result.length > 0,
    everyone,
    people: peopleById,
    guestEmails,
    groupNames,
    gradeColors,
    groups: [],
  };
  state.model.groups = rowsOf(answers.lists, 'GROUP').map(g => {
      const listRules = rules[g.id] || [];
      const shown = (members[g.id] || []).map(e => ({...personView(e.person), reasons: e.reasons.split(', ').map(w => reasonOf(listRules, w)).filter(Boolean)}));
      const own = (hand[g.id] || []).find(m => viewer && m.person === viewer.id);
      return {
        id: g.id,
        slug: g.slug,
        title: g.name,
        address: `${g.slug}@${domain}`,
        description: g.description || '',
        posting: g.posting || 'everyone',
        replying: g.replying || 'everyone',
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
        unsubscribed: Boolean(own && own.member === 'excluded'),
        ownRow: own || null,
        sent: (sent[g.id] || []).length,
      };
  });
  byId = {};
  bySlug = {};
  for (const g of state.model.groups) {
    byId[g.id] = g;
    bySlug[g.slug] = g;
  }
}

export function person(email) {
  return known(email);
}

export function me() {
  const v = state.model.viewer;
  const name = (v && v.name_show) || state.model.email;
  return {email: state.model.email, name, initial: name[0].toUpperCase(), photoUrl: v ? photoOf(v) : ''};
}

export function isAdmin() {
  return state.model.admin;
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
