import {api, signedIn} from '/api.js';
import {me as whoAmI} from '/data.js';
import {loadNavOpen} from './storage.js';

export const state = {everyoneOrder: [], familyOrder: [], tab: 'everyone', classTab: 'by-classroom', rosterTab: 'students', rosterSectionExcluded: new Set(), q: '', filterGrades: new Set(), filterClassrooms: new Set(), filterRoles: new Set(), filterRoleExcluded: new Set(), filterCities: new Set(), filterPronouns: new Set(), filterTags: new Set(), filterTagRelations: new Set(), staffDeptExcluded: new Set(), tagListView: 'faces', navOpen: loadNavOpen(), gvGreeting: '', gvSiblings: true, gvKidEmail: false, gvInviteBy: 'group', gvSystem: ''};

export const model = {
  viewer: null,
  email: '',
  allowances: [],
  admin: false,
  people: [],
  families: [],
  classrooms: [],
  grades: [],
  bands: [],
  crews: [],
  departments: [],
  roomParents: [],
  settings: {},
  moved: '',
};

export let byId = {};
export let bySlug = {};
export let groupById = {};
export let tags = {};
export let lists = {};

let emails = {};
let photos = {};
let geocodes = {};
let familiesByPerson = {};
let membersByGroup = {};
let roles = {students: new Set(), parents: new Set(), staff: new Set()};

export async function q(text) {
  const res = await signedIn(await fetch('/api/q', {method: 'QUERY', headers: {'Content-Type': 'text/plain'}, body: text}));
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

async function qs(named) {
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

function all(answer, table) {
  return Object.values(answer.resources[table] || {});
}

export function write(batch) {
  return api('POST', '/api/q', {batch});
}

export function tagKey(id) {
  return 'tag:' + id;
}

export function listKey(id) {
  return 'list:' + id;
}

const tagCondition = '(own_group @g) (not (blank managed_by)) (!= status "closed") (not (exists GROUP (= managed_by @g) (!= id @g))) (manages @g)';
const listKindCondition = '(!= status "closed") (or (in kind "party" "activity") mail (and (= kind "event") (not (blank rsvp_yes))))';
const viewerGroups = '(select MEMBER.group (= person @viewer) (= member "yes"))';
const runCondition = `${listKindCondition} (manages @g)`;
const joinedCondition = `${listKindCondition} (not (manages @g)) (or (in id ${viewerGroups}) (in rsvp_yes ${viewerGroups}))`;
const listCondition = `${listKindCondition} (or (manages @g) (in id ${viewerGroups}) (in rsvp_yes ${viewerGroups}))`;
const listIncludes = '(include parent parent.parent parent.parent.parent)';
const personColumns = 'source vc_legal_name name_long_override name_long name_short_override name_short name_sort_override name_sort name_show slug ' +
  'grade_override grade classroom_override classroom crew_override crew department_override department job_title_override job_title ' +
  'phone phone_consent address_consent pronouns pronunciation facts facts_updated photo_updated';

function oldName() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const params = new URLSearchParams(location.search);
  if (parts[0] === 'people' && parts[1]) {
    return parts[1].startsWith('guest:') ? {page: 'guest', key: parts[1].slice('guest:'.length)} : {page: 'person', key: parts[1].toLowerCase()};
  }
  if (parts[0] === 'people' && params.get('tag')) {
    return {page: 'tag', key: params.get('tag')};
  }
  if (parts[0] === 'people' && params.get('list')) {
    const list = params.get('list');
    return {page: 'list', key: list.startsWith('room:') ? list : list.slice(list.indexOf(':') + 1)};
  }
  if (['families', 'classrooms', 'grades'].includes(parts[0]) && parts[1]) {
    return {page: parts[0], key: parts[1]};
  }
  return null;
}

async function one(table, column, value) {
  const answer = await api('QUERY', '/api/q', {from: table, where: [{'=': [{path: column}, value]}]});
  return rowsOf(answer, table)[0];
}

async function oldTarget(name) {
  if (!name) {
    return '';
  }
  if (name.page === 'guest' && name.key.includes(':')) {
    const email = await one('PERSON_EMAIL', 'address', name.key.slice(name.key.indexOf(':') + 1).toLowerCase());
    return email ? email.person : '';
  }
  const alias = await one('ALIAS', 'alias', name.key);
  if (!alias || !alias.target.startsWith('mem')) {
    return alias ? alias.target : '';
  }
  const ticket = await one('MEMBER', 'id', alias.target);
  return ticket ? ticket.person : '';
}

function movedTo(name, target) {
  if (!target) {
    return '';
  }
  if (target.startsWith('per')) {
    return '/people/' + encodeURIComponent((byId[target] && byId[target].slug) || target);
  }
  const group = groupById[target];
  const path = {
    tag: '/people?tag=' + encodeURIComponent(target),
    list: '/people?list=' + encodeURIComponent(target),
    families: '/families/' + encodeURIComponent(target),
    classrooms: group && group.slug ? '/classrooms/' + encodeURIComponent(group.slug) : '',
    grades: group && group.slug ? '/grades/' + encodeURIComponent(group.slug) : '',
  }[name.page] || '';
  return path === location.pathname + location.search ? '' : path;
}

export async function loadModel() {
  const name = oldName();
  const [who, answers, oldId] = await Promise.all([
    whoAmI(),
    qs({
      viewer: '(from PERSON (where (= id @viewer)))',
      admin: '(from PERSON (where (= id @viewer) (admin_of "who")) (columns source))',
      people: `(from PERSON (where (in id (select EFFECTIVE_MEMBER.person (in group (select GROUP.id (= slug "everyone"))))) (not hidden)) (order name_sort asc) (columns ${personColumns}))`,
      addresses: '(from PERSON_EMAIL (where primary) (columns person address))',
      pictures: '(from PHOTO (where ready (or (not (blank person)) (in group (select GROUP.id (in kind "family" "classroom" "grade"))))) (order order asc) (columns person group order reencode crop_left crop_top crop_width crop_height))',
      groups: '(from GROUP @g (where (!= status "closed") (or (in kind "family" "classroom" "grade" "band" "crew" "department") (and (= kind "group") (= parent.kind "band")))) (order order asc name asc))',
      members: '(from MEMBER (where (= member "yes") (in group (select GROUP.id (= kind "family")))) (columns group person))',
      effective: '(from EFFECTIVE_MEMBER (where (in group (select GROUP.id (or (in slug "students" "parents" "staff") (and (= kind "group") (= parent.kind "band")))))) (include group) (columns group person))',
      coords: '(from GEOCODE)',
      settings: '(from SETTING (where (= app "platform")))',
      tagged: `(from GROUP @g (where ${tagCondition}) (include managed_by))`,
      tagMembers: `(from MEMBER (where (= member "yes") (in group (select GROUP.id @g ${tagCondition}))) (columns group person))`,
      tagManagers: `(from MEMBER (where (= member "yes") (in group (select GROUP.managed_by @g ${tagCondition}))) (columns group person))`,
      managed: `(from GROUP @g (where ${runCondition}) (order name asc) ${listIncludes})`,
      joined: `(from GROUP @g (where ${joinedCondition}) (order name asc) ${listIncludes})`,
      listMembers: `(from EFFECTIVE_MEMBER (where (in group (select GROUP.id @g ${listCondition}))) (columns group person))`,
      rsvpMembers: `(from EFFECTIVE_MEMBER (where (in group (select GROUP.rsvp_yes @g ${listCondition}))) (columns group person))`,
      guests: `(from PERSON (where (= source "guest") (in id (select EFFECTIVE_MEMBER.person (in group (select GROUP.id @g ${listCondition}))))) (columns ${personColumns}))`,
    }),
    oldTarget(name),
  ]);
  const {viewer, admin, people, addresses, pictures, groups, members, effective, coords, settings, tagged, tagMembers, tagManagers, managed, joined, listMembers, rsvpMembers, guests} = answers;
  model.email = who.email;
  model.allowances = who.allowances;
  model.viewer = viewer.result.length ? rowsOf(viewer, 'PERSON')[0] : null;
  model.admin = admin.result.length > 0;
  model.people = rowsOf(people, 'PERSON');
  byId = {};
  bySlug = {};
  for (const p of model.people) {
    byId[p.id] = p;
    if (p.slug) {
      bySlug[p.slug] = p;
    }
  }
  if (model.viewer && !byId[model.viewer.id]) {
    byId[model.viewer.id] = model.viewer;
  }
  emails = {};
  for (const e of rowsOf(addresses, 'PERSON_EMAIL')) {
    emails[e.person] = e.address;
  }
  photos = {};
  for (const ph of rowsOf(pictures, 'PHOTO')) {
    const of = ph.person || ph.group;
    (photos[of] = photos[of] || []).push(ph);
  }
  geocodes = {};
  for (const g of rowsOf(coords, 'GEOCODE')) {
    geocodes[g.address] = {lat: Number(g.lat), lng: Number(g.lng)};
  }
  model.settings = {};
  for (const s of rowsOf(settings, 'SETTING')) {
    model.settings[s.key] = s.value;
  }
  groupById = {};
  const groupRows = rowsOf(groups, 'GROUP');
  for (const g of groupRows) {
    groupById[g.id] = g;
  }
  const ofKind = kind => groupRows.filter(g => g.kind === kind);
  model.families = ofKind('family');
  model.classrooms = ofKind('classroom');
  model.grades = ofKind('grade').sort((a, b) => gradeRank(a) - gradeRank(b));
  model.bands = ofKind('band');
  model.crews = ofKind('crew');
  model.departments = ofKind('department');
  model.roomParents = groupRows.filter(g => g.kind === 'group');
  membersByGroup = {};
  familiesByPerson = {};
  for (const m of rowsOf(members, 'MEMBER')) {
    if (!byId[m.person]) {
      continue;
    }
    (membersByGroup[m.group] = membersByGroup[m.group] || []).push(m.person);
  }
  for (const f of model.families) {
    for (const id of membersByGroup[f.id] || []) {
      (familiesByPerson[id] = familiesByPerson[id] || []).push(f);
    }
  }
  roles = {students: new Set(), parents: new Set(), staff: new Set()};
  const slugs = {};
  for (const g of all(effective, 'GROUP')) {
    slugs[g.id] = g.slug;
  }
  for (const e of rowsOf(effective, 'EFFECTIVE_MEMBER')) {
    const slug = slugs[e.group];
    if (roles[slug]) {
      roles[slug].add(e.person);
      continue;
    }
    if (groupById[e.group]) {
      (membersByGroup[e.group] = membersByGroup[e.group] || []).push(e.person);
    }
  }
  loadTags(tagged, [tagMembers, tagManagers]);
  loadLists(managed, joined, [listMembers, rsvpMembers], guests);
  model.moved = movedTo(name, oldId);
}

function loadTags(tagged, memberAnswers) {
  tags = {};
  const rows = rowsOf(tagged, 'GROUP');
  if (!rows.length) {
    return;
  }
  const included = {};
  for (const g of all(tagged, 'GROUP')) {
    included[g.id] = g;
  }
  const rowsOfGroup = {};
  for (const m of memberAnswers.flatMap(a => rowsOf(a, 'MEMBER'))) {
    (rowsOfGroup[m.group] = rowsOfGroup[m.group] || {})[m.person] = m.id;
  }
  for (const t of rows) {
    if (rows.some(other => other.managed_by === t.id && t.name === other.name + ' Managers')) {
      continue;
    }
    const managing = included[t.managed_by] || {};
    tags[tagKey(t.id)] = {
      id: t.id,
      name: t.name,
      memberRows: rowsOfGroup[t.id] || {},
      managerRows: rowsOfGroup[t.managed_by] || {},
      managersGroup: t.managed_by,
      ownManagers: managing.name === t.name + ' Managers',
    };
  }
}

export function tagPeople(t) {
  return Object.keys(t.memberRows);
}

export function tagManagers(t) {
  return Object.keys(t.managerRows);
}

const listKinds = {party: 'party', activity: 'activity', event: 'event'};

export function today() {
  return new Date().toLocaleDateString('en-CA');
}

function datedFrom(g, groups) {
  for (let at = g; at; at = groups[at.parent]) {
    if (at.start) {
      return at;
    }
  }
  return null;
}

function liveLists(rows, groups) {
  const now = today();
  const live = rows.filter(g => {
    const dated = datedFrom(g, groups);
    return !dated || (dated.end || dated.start).slice(0, 10) >= now;
  });
  const ids = new Set(live.map(g => g.id));
  const instance = g => g.kind === 'event' && groups[g.parent] && groups[g.parent].kind === 'event';
  const next = {};
  for (const g of live.filter(instance)) {
    if (!next[g.parent] || g.start < next[g.parent].start) {
      next[g.parent] = g;
    }
  }
  return live.filter(g => !ids.has(g.parent) && (!instance(g) || next[g.parent] === g));
}

function loadLists(managed, joined, memberAnswers, guestRows) {
  lists = {};
  const groups = {};
  for (const g of [...all(managed, 'GROUP'), ...all(joined, 'GROUP')]) {
    groups[g.id] = g;
  }
  const run = liveLists(rowsOf(managed, 'GROUP'), groups);
  const rows = [...run, ...liveLists(rowsOf(joined, 'GROUP'), groups)];
  const running = new Set(run.map(g => g.id));
  const room = model.roomParents.filter(g => (membersByGroup[g.id] || []).includes(model.viewer && model.viewer.id));
  if (!rows.length && !room.length) {
    return;
  }
  const guests = {};
  for (const p of rowsOf(guestRows, 'PERSON')) {
    guests[p.id] = p;
    byId[p.id] = byId[p.id] || p;
  }
  const people = {};
  const seen = new Set();
  for (const e of memberAnswers.flatMap(a => rowsOf(a, 'EFFECTIVE_MEMBER'))) {
    if (seen.has(e.id)) {
      continue;
    }
    seen.add(e.id);
    (people[e.group] = people[e.group] || []).push(e.person);
  }
  for (const g of rows) {
    const ids = people[g.kind === 'event' ? g.rsvp_yes : g.id] || [];
    const dated = datedFrom(g, groups);
    lists[listKey(g.id)] = {
      key: listKey(g.id),
      id: g.id,
      name: g.name,
      kind: listKinds[g.kind] || 'group',
      slug: g.slug || g.id,
      run: running.has(g.id),
      member: (people[g.id] || []).includes(model.viewer && model.viewer.id),
      mail: g.mail === 'Yes',
      start: dated ? dated.start : '',
      people: ids.filter(id => !guests[id]),
      guests: ids.filter(id => guests[id]).map(id => guests[id]),
    };
  }
  for (const g of room) {
    const band = groupById[g.parent];
    const kids = model.people.filter(p => isStudent(p) && bandOf(p) === band);
    const ids = new Set();
    for (const k of kids) {
      ids.add(k.id);
      for (const f of familiesOf(k)) {
        for (const a of adultsOf(f)) {
          ids.add(a.id);
        }
      }
    }
    lists[listKey(g.id)] = {key: listKey(g.id), id: g.id, name: `${band.name} Families`, kind: 'room', slug: '', run: true, member: true, mail: false, start: '', people: [...ids], guests: []};
  }
}

function gradeRank(g) {
  const code = (g.slug || '').replace('grade-', '');
  return code === 'k' ? 0 : Number(code);
}

export function viewer() {
  return model.viewer;
}

export function viewerId() {
  return model.viewer ? model.viewer.id : '';
}

export function peopleOf(ids) {
  return ids.map(id => byId[id]).filter(Boolean);
}

export function emailOf(p) {
  return emails[p.id] || '';
}

export function photosOf(id) {
  return photos[id] || [];
}

export function photoUrl(ph) {
  return ph ? `/api/blob/${ph.id}/image` : '';
}

export function thumbOf(ph) {
  return ph ? `/api/blob/${ph.id}/thumbnail` : '';
}

export function fullOf(ph) {
  return ph ? `/api/blob/${ph.id}/${ph.reencode ? 'reencode' : 'image'}` : '';
}

export function pronunciationUrl(row) {
  return row.pronunciation ? `/api/blob/${row.id}/pronunciation` : '';
}

export function isStudent(p) {
  return roles.students.has(p.id);
}

export function isParent(p) {
  return roles.parents.has(p.id);
}

export function isStaff(p) {
  return roles.staff.has(p.id);
}

export function familiesOf(p) {
  return (p && familiesByPerson[p.id]) || [];
}

export function familyOf(p) {
  return familiesOf(p)[0];
}

export function membersOf(group) {
  return peopleOf(membersByGroup[group.id] || []);
}

export function kidsOf(family) {
  return membersOf(family).filter(isStudent);
}

export function adultsOf(family) {
  return membersOf(family).filter(p => !isStudent(p));
}

export function familyName(family) {
  return family.name || '';
}

export function familyShortName(family) {
  return familyName(family).replace(/ Family$/, '');
}

export function geocodeOf(address) {
  return geocodes[address] || null;
}

export function gradeOf(p) {
  if (!p.grade) {
    return null;
  }
  return model.grades.find(g => g.slug === 'grade-' + p.grade.toLowerCase()) || null;
}

export function gradeName(p) {
  const g = gradeOf(p);
  return g ? g.name : '';
}

export function classroomOf(p) {
  return groupById[p.classroom] || null;
}

export function classroomName(p) {
  const c = classroomOf(p);
  return c ? c.name : '';
}

export function crewName(p) {
  const c = groupById[p.crew];
  return c ? c.name : '';
}

export function departmentName(p) {
  const d = groupById[p.department];
  return d ? d.name : '';
}

export function bandOf(p) {
  const g = gradeOf(p);
  return g ? groupById[g.parent] || null : null;
}

export function groupKey(g) {
  return g.slug || g.id;
}

export function classroomByKey(key) {
  return model.classrooms.find(c => groupKey(c) === key) || null;
}

export function gradeByKey(key) {
  return model.grades.find(g => groupKey(g) === key) || null;
}

export function classroomPath(c) {
  return '/classrooms/' + encodeURIComponent(groupKey(c));
}

export function gradePath(g) {
  return '/grades/' + encodeURIComponent(groupKey(g));
}

export function roomParentsOf(band) {
  const group = model.roomParents.find(g => g.parent === band.id);
  return group ? peopleOf(membersByGroup[group.id] || []) : [];
}

export function roomParentsGroup(band) {
  return model.roomParents.find(g => g.parent === band.id) || null;
}

export function gradeNameColor(name) {
  const g = model.grades.find(x => x.name === name);
  return g ? g.color || '' : '';
}

export function classroomNameColor(name) {
  const c = model.classrooms.find(x => x.name === name);
  return c ? c.color || '' : '';
}

export function staffColor() {
  return model.settings['Staff Color'] || '';
}

export function staleYears() {
  return {
    photo: Number(model.settings['Photo Stale Years']),
    facts: Number(model.settings['Facts Stale Years']),
    familyPhoto: Number(model.settings['Family Photo Stale Years']),
  };
}

export function privacyLinks() {
  return {veracrossPreferences: model.settings['Veracross Preferences URL'] || '', heliosWhoOptIn: model.settings['Helios Who Opt-In URL'] || ''};
}

export function inHousehold(p) {
  const me = viewer();
  if (!me) {
    return false;
  }
  return p.id === me.id || familiesOf(me).some(f => (membersByGroup[f.id] || []).includes(p.id));
}

export function canEdit(p) {
  return model.admin || inHousehold(p);
}

export function canEditFamily(family) {
  return model.admin || (membersByGroup[family.id] || []).includes(viewerId());
}
