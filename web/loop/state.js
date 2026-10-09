import {batch, me as whoAmI} from '/data.js';
import {listed, known, contactLine, emailOf, photoOf, wordsOf, gradeOf, isStudent} from '/directory.js';

export const state = {model: null, people: []};

const byName = new Map();
const byEmail = new Map();

export function personView(p) {
  const row = known(p.email);
  if (!row) {
    return {email: p.email, name: p.fullName, photoUrl: p.heroPhotoUrl, words: p.words, grade: p.isStudent ? p.grade : '', context: p.words};
  }
  return {email: emailOf(row), name: row.name_show, photoUrl: photoOf(row), words: wordsOf(row), grade: isStudent(row) ? gradeOf(row) : '', context: contactLine(row)};
}

export function memberView(m, person) {
  if (person) {
    return {...personView(person), reasons: m.reasons || []};
  }
  return {email: m.email, name: m.name, words: 'Outside the directory', outside: true, reasons: m.reasons || []};
}

function groupView(read, g) {
  return {
    ...g,
    rules: g.rules || [],
    additions: g.additions || [],
    excluded: g.excluded || [],
    sent: g.sent || 0,
    managers: [...read.follow(g, 'managers').map(personView), ...(g.managersOutside || []).map(email => ({email, name: email}))],
    members: read.follow(g, 'members').map(m => memberView(m, read.follow(m, 'person'))),
    mine: g.me.managing,
    member: g.me.member,
    unsubscribed: g.me.unsubscribed,
    archived: g.me.archived,
  };
}

export async function loadModel() {
  const [read, viewer, people] = await Promise.all([batch({
    settings: '/api/loop-settings?include=viewer',
    lists: '/api/email-lists?include=managers,members.person',
    suggestions: '/api/email-list-suggestions?include=managers',
    classrooms: '/api/classrooms',
    grades: '/api/grades?enrolled',
    tags: '/api/tags',
    magicTags: '/api/magic-tags?include=parent',
  }), whoAmI(), listed()]);
  const s = read.get(read.result.settings[0]);
  const person = read.follow(s, 'viewer') || {};
  const name = person.fullName || viewer.email;
  state.model = {
    user: {email: viewer.email, name, initial: name[0].toUpperCase(), photoUrl: person.heroPhotoUrl},
    allowances: viewer.allowances,
    domain: s.domain,
    gradeColors: s.gradeColors || {},
    options: {
      classrooms: read.result.classrooms.map(id => read.get(id).name),
      grades: read.result.grades.map(id => read.get(id).name),
      tags: read.result.tags.map(id => read.get(id)).map(t => ({key: `tag:${t.id}`, name: t.me.mine ? t.name : `${t.name} (${t.ownerName}'s)`})),
      lists: read.result.magicTags.map(read.get).filter(t => !t.archived)
        .map(t => ({key: t.key, name: t.name, kind: t.kind, parent: (t.parent && read.get(t.parent) || {}).key || ''}))
        .sort((a, b) => a.name.localeCompare(b.name)),
      roles: s.roles,
      relations: s.relations,
    },
    groups: read.result.lists.map(id => groupView(read, read.get(id))),
    suggestions: read.result.suggestions.map(id => read.get(id)).map(sg => ({key: sg.key, name: sg.name, kind: sg.kind, mine: sg.mine, managers: read.follow(sg, 'managers').map(personView)})),
  };
  state.people = people;
  byName.clear();
  for (const g of state.model.groups) {
    byName.set(g.name, g);
  }
  byEmail.clear();
  for (const p of state.people) {
    byEmail.set(emailOf(p), p);
  }
}

export function person(email) {
  return byEmail.get(email) || null;
}

export function me() {
  return state.model.user;
}

export function isAdmin() {
  return state.model.allowances.includes('loop.see-all');
}

export function managed(g) {
  return g.can.edit;
}

export function group(name) {
  return byName.get(name) || null;
}

export function groupPath(g) {
  return `/groups/${encodeURIComponent(g.name)}`;
}

export function options() {
  return state.model.options;
}

export function matches(g, query) {
  if (!query) {
    return true;
  }
  return `${g.title} ${g.address} ${g.description || ''}`.toLowerCase().includes(query);
}
