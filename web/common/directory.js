import {signedIn} from '/api.js';
import {appOrigin} from '/appswitch.js';

let asked = null;
let index = null;

async function q(text) {
  const res = await signedIn(await fetch('/api/q', {method: 'QUERY', headers: {'Content-Type': 'text/plain'}, body: text}));
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

function rows(answer, table) {
  return answer.result.map(id => answer.resources[table][id]);
}

function all(answer, table) {
  return Object.values(answer.resources[table] || {});
}

async function load() {
  const [people, emails, photos, members, roles, groups] = await Promise.all([
    q('(from PERSON (where (in id (select EFFECTIVE_MEMBER.person (in group (select GROUP.id (= slug "everyone"))))) (not hidden)) (order name_sort asc))'),
    q('(from PERSON_EMAIL (where primary))'),
    q('(from PHOTO (where ready) (order order asc))'),
    q('(from MEMBER (where (= member "yes") (in group (select GROUP.id (= kind "family")))))'),
    q('(from EFFECTIVE_MEMBER (where (in group (select GROUP.id (in slug "students" "parents" "staff")))) (include group))'),
    q('(from GROUP (where (in kind "grade" "classroom" "department")))'),
  ]);
  const out = {
    people: rows(people, 'PERSON'),
    byId: {},
    byEmail: {},
    email: {},
    photo: {},
    families: {},
    members: {},
    roles: {students: new Set(), parents: new Set(), staff: new Set()},
    groups: {},
  };
  for (const p of out.people) {
    out.byId[p.id] = p;
  }
  for (const e of all(emails, 'PERSON_EMAIL')) {
    out.email[e.person] = e.address;
    if (out.byId[e.person]) {
      out.byEmail[e.address] = out.byId[e.person];
    }
  }
  for (const ph of rows(photos, 'PHOTO')) {
    const of = ph.person || ph.group;
    out.photo[of] = out.photo[of] || `/api/blob/${ph.id}/thumbnail`;
  }
  for (const m of all(members, 'MEMBER')) {
    (out.members[m.group] = out.members[m.group] || []).push(m.person);
    (out.families[m.person] = out.families[m.person] || []).push(m.group);
  }
  const slugs = {};
  for (const g of all(roles, 'GROUP')) {
    slugs[g.id] = g.slug;
  }
  for (const e of all(roles, 'EFFECTIVE_MEMBER')) {
    out.roles[slugs[e.group]].add(e.person);
  }
  for (const g of all(groups, 'GROUP')) {
    out.groups[g.id] = g;
  }
  out.grades = {};
  for (const g of Object.values(out.groups)) {
    if (g.kind === 'grade') {
      out.grades[g.slug] = g.name;
    }
  }
  index = out;
  return out;
}

export function directory() {
  asked = asked || load().catch(err => {
    asked = null;
    throw err;
  });
  return asked;
}

export async function listed() {
  return (await directory()).people;
}

export async function personByEmail(email) {
  await directory();
  return known(email);
}

export function known(email) {
  return index.byEmail[email] || null;
}

export function emailOf(p) {
  return index.email[p.id] || '';
}

export function isStudent(p) {
  return index.roles.students.has(p.id);
}

export function isStaff(p) {
  return index.roles.staff.has(p.id);
}

export function isParent(p) {
  return index.roles.parents.has(p.id);
}

function familiesOf(p) {
  return index.families[p.id] || [];
}

export function photoOf(p) {
  return index.photo[p.id] || familiesOf(p).map(f => index.photo[f]).find(Boolean) || '';
}

export function gradeOf(p) {
  return p.grade ? index.grades['grade-' + p.grade.toLowerCase()] || '' : '';
}

function groupName(id) {
  return id && index.groups[id] ? index.groups[id].name : '';
}

export function placeOf(p) {
  if (isStudent(p)) {
    return [gradeOf(p), groupName(p.classroom)].filter(Boolean).join(' · ');
  }
  return [p.job_title, groupName(p.department)].filter(Boolean).join(' · ');
}

export function wordsOf(p) {
  if (isStaff(p)) {
    return p.job_title || 'Staff';
  }
  if (isStudent(p)) {
    return gradeOf(p) || 'Student';
  }
  return isParent(p) ? 'Parent' : '';
}

function household(p) {
  const out = new Map();
  for (const f of familiesOf(p)) {
    for (const id of index.members[f] || []) {
      if (id !== p.id && index.byId[id]) {
        out.set(id, index.byId[id]);
      }
    }
  }
  return [...out.values()];
}

export function relativesOf(p, relation) {
  const others = household(p);
  switch (relation) {
    case 'parents':
      return isStudent(p) ? others.filter(o => !isStudent(o)) : [];
    case 'children':
      return isStudent(p) ? [] : others.filter(isStudent);
    case 'partners':
      return isStudent(p) ? [] : others.filter(o => !isStudent(o));
    case 'siblings':
      return isStudent(p) ? others.filter(isStudent) : [];
  }
  return [];
}

export function profileLink(p) {
  return appOrigin('who') + '/people/' + encodeURIComponent(p.slug || p.id);
}

function firstName(name) {
  return (name || '').trim().split(/\s+/)[0];
}

export function contactLine(p) {
  if (isStudent(p)) {
    return placeOf(p);
  }
  if (isParent(p)) {
    const kids = relativesOf(p, 'children').map(k => gradeOf(k) ? `${firstName(k.name_show)} (${gradeOf(k)})` : firstName(k.name_show));
    if (kids.length) {
      return 'Parent to ' + kids.join(', ');
    }
  }
  return wordsOf(p);
}
