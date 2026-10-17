import {parseWhen} from '/datecard.js';
import {api} from '/api.js';
import {queries} from '/rail.js';
import {directory, emailOf, photoOf} from '/directory.js';

export const state = {model: null, showPrevious: false, showHidden: false, year: '', category: ''};

const index = new Map();

export const ADDING = {yes: 'Yes', approval: 'Approval Needed', no: 'No'};

const addingWords = {anyone: ADDING.yes, approval: ADDING.approval, managers: ADDING.no};

export function addingValue(words) {
  return Object.keys(addingWords).find(k => addingWords[k] === words) || 'managers';
}

export function write(batch) {
  return api('POST', '/api/q', {batch});
}

function rowsOf(answer, table) {
  return answer.result.map(id => answer.resources[table][id]);
}

function schoolYears(now) {
  const start = now.getMonth() >= 6 ? now.getFullYear() : now.getFullYear() - 1;
  const span = y => `${y} - ${y + 1}`;
  return {current: span(start), last: span(start - 1), next: span(start + 1)};
}

function today() {
  const d = new Date();
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

function statusOf(g) {
  if (!g.visible_to) {
    return 'Hidden';
  }
  return {pending: 'Pending', done: 'Done'}[g.status] || 'Open';
}

export async function loadModel() {
  const [answers, dir] = await Promise.all([queries({
    viewer: '(from PERSON (where (= id @viewer)))',
    admin: '(from APP (where (= key "team") (admin_of "team")) (columns key))',
    everyone: '(from GROUP (where (= id (slug "everyone"))) (columns slug))',
    activities: '(from GROUP (where (= kind "activity") (!= status "closed")) (order order asc))',
    headings: '(from GROUP (where (= kind "category") (= parent.kind "activity")) (order order asc))',
    runs: '(from GROUP @g (where (in kind "activity" "category") (runs_list @g)) (columns kind))',
    members: '(from MEMBER (where (in group (select GROUP.id (= kind "activity")))))',
    managers: '(from MEMBER (where (= member "yes") (in group (select GROUP.managed_by (= kind "activity")))) (columns group person))',
    yes: '(from MEMBER (where (in group (select GROUP.rsvp_yes (= kind "activity")))) (columns group person))',
    no: '(from MEMBER (where (in group (select GROUP.rsvp_no (= kind "activity")))) (columns group person))',
    offers: '(from MEMBER (where (= member "yes") (in group (select GROUP.chair_offers (= kind "activity")))) (columns group person))',
    leads: '(from MEMBER (where (= member "yes") (in group (select GROUP.leads (= kind "activity")))) (columns group person))',
    photos: '(from PHOTO (where (in group (select GROUP.id (in kind "activity" "category")))) (order order asc) (columns group ready))',
    settings: '(from SETTING (where (= app "team")))',
    rules: '(from RULE (where under (= target group) (in group (select GROUP.id (= kind "activity")))) (columns group replace_with))',
  }), directory()]);
  const viewer = rowsOf(answers.viewer, 'PERSON')[0];
  const admin = answers.admin.result.length > 0;
  const runs = new Set(answers.runs.result);
  const picture = {};
  for (const ph of rowsOf(answers.photos, 'PHOTO')) {
    picture[ph.group] = picture[ph.group] || ph;
  }
  const by = (answer, table, key) => {
    const out = {};
    for (const row of rowsOf(answer, table)) {
      (out[row[key]] = out[row[key]] || []).push(row);
    }
    return out;
  };
  const members = by(answers.members, 'MEMBER', 'group');
  const managers = by(answers.managers, 'MEMBER', 'group');
  const said = {};
  for (const [answer, word] of [[answers.yes, 'Yes'], [answers.no, 'No']]) {
    for (const m of rowsOf(answer, 'MEMBER')) {
      said[m.group + '/' + m.person] = word;
    }
  }
  const offers = by(answers.offers, 'MEMBER', 'group');
  const leads = by(answers.leads, 'MEMBER', 'group');
  const reaching = new Set(rowsOf(answers.rules, 'RULE').map(r => r.group));
  const groups = {};
  for (const g of [...rowsOf(answers.activities, 'GROUP'), ...rowsOf(answers.headings, 'GROUP')]) {
    groups[g.id] = g;
  }
  const isYear = g => g.kind === 'activity' && (!g.parent || (groups[g.parent] === undefined));
  const nameOf = id => (dir.byId[id] && dir.byId[id].name_show) || '';
  const settings = {};
  for (const s of rowsOf(answers.settings, 'SETTING')) {
    settings[s.key] = s.value;
  }
  const nodes = new Map();
  const headings = new Map();
  const yearOf = {};
  for (const g of Object.values(groups)) {
    if (isYear(g)) {
      yearOf[g.id] = g.name;
    }
  }
  const now = today();
  const roots = [];
  const yearHeadings = [];
  for (const g of rowsOf(answers.headings, 'GROUP')) {
    const owner = groups[g.parent];
    const h = {
      id: g.id, title: g.name, description: g.description || '', eventId: isYear(owner) ? '' : owner.id, year: isYear(owner) ? owner.name : '',
      image: picture[g.id] ? picture[g.id].id : '', imageUrl: picture[g.id] ? `/api/blob/${picture[g.id].id}/image` : '',
      allowAdding: addingWords[g.adding] || ADDING.no, allowAddingOwn: addingWords[g.adding] || ADDING.no,
      showOnMain: g.listed !== 'No', hidden: !g.visible_to, volunteersHidden: !g.members_visible_to, order: g.order,
      can: {add: runs.has(g.id) || g.adding === 'anyone' || g.adding === 'approval', delete: runs.has(g.id)}, row: g,
    };
    headings.set(g.id, h);
    if (!h.eventId) {
      yearHeadings.push(h);
    }
  }
  const placeOf = g => {
    let parent = groups[g.parent];
    let category = '';
    if (parent && parent.kind === 'category') {
      category = parent.id;
      parent = groups[parent.parent];
    }
    if (!parent || isYear(parent)) {
      return {parent: '', category, year: parent ? parent.name : ''};
    }
    return {parent: parent.id, category};
  };
  for (const g of rowsOf(answers.activities, 'GROUP')) {
    if (isYear(g)) {
      continue;
    }
    const rows = members[g.id] || [];
    const taken = rows.filter(m => m.member === 'yes').length;
    const spots = Number(g.capacity) || 0;
    const chairs = new Set((managers[g.managed_by] || []).map(m => m.person));
    const offerRows = offers[g.chair_offers] || [];
    const offered = new Map(offerRows.map(m => [m.person, m.id]));
    const led = new Map((leads[g.leads] || []).map(m => [m.person, m.id]));
    const volunteers = rows.filter(m => m.member === 'yes').map(m => ({
      id: m.id, person: m.person, email: emailOf({id: m.person}), name: nameOf(m.person) || 'Someone',
      photoUrl: dir.byId[m.person] ? photoOf(dir.byId[m.person]) : '',
      position: led.has(m.person) || chairs.has(m.person) ? 'Co-Chair' : offered.has(m.person) ? 'Open to Co-Chair' : 'Volunteer',
      offer: offered.get(m.person) || '', lead: led.get(m.person) || '',
      note: m.note || '', added: m.added || '', addedBy: m.added_by ? emailOf({id: m.added_by}) : '', addedByName: nameOf(m.added_by),
      rsvp: said[g.rsvp_yes + '/' + m.person] || said[g.rsvp_no + '/' + m.person] || (g.rsvp_yes ? 'No RSVP yet' : ''),
      can: {delete: runs.has(g.id) || m.person === viewer.id || (dir.families[m.person] || []).some(f => (dir.families[viewer.id] || []).includes(f))},
    }));
    volunteers.sort((a, b) => (b.position === 'Co-Chair') - (a.position === 'Co-Chair'));
    const where = placeOf(g);
    const status = statusOf(g);
    const node = {
      id: g.id, title: g.name, description: g.description || '', status, row: g,
      parent: where.parent, category: where.category, year: where.year || '',
      image: picture[g.id] ? picture[g.id].id : '', imageUrl: picture[g.id] ? `/api/blob/${picture[g.id].id}/image` : '',
      flyer: g.flyer || '', flyerUrl: g.flyer ? `/api/blob/${g.id}/flyer` : '',
      timing: g.timing || '', start: g.start || '', end: g.end || '', location: g.location || '',
      spots, taken, volunteersComplete: g.join === 'none', full: g.join === 'none' || (spots > 0 && taken >= spots),
      directSignUp: g.join === 'direct' || g.join === 'others', directSignUpOwn: g.join === 'direct' || g.join === 'others' ? 'Yes' : 'No',
      volunteersHidden: !g.members_visible_to, volunteersHiddenOwn: g.members_visible_to ? 'No' : 'Yes',
      allowAdding: addingWords[g.adding] || ADDING.no, allowAddingOwn: addingWords[g.adding] || ADDING.no,
      priority: g.priority === 'Yes', prettyId: g.slug || '', added: g.added || '', addedBy: nameOf(g.added_by),
      reachesUnder: reaching.has(g.id), path: `/v/${encodeURIComponent(g.slug || g.id)}`,
      canEdit: runs.has(g.id), runs: runs.has(g.id), children: [], volunteers, managerRows: managers[g.managed_by] || [],
      offersOpen: Boolean(g.chair_offers), offerRows, leadRows: leads[g.leads] || [],
      can: {
        edit: runs.has(g.id), delete: runs.has(g.id) && !rows.length,
        add: runs.has(g.id) || g.adding === 'anyone' || g.adding === 'approval',
        'sign-up': (g.join === 'direct' || g.join === 'others') && status === 'Open',
      },
    };
    nodes.set(g.id, node);
  }
  for (const node of nodes.values()) {
    if (node.parent && nodes.has(node.parent)) {
      nodes.get(node.parent).children.push(node);
    } else {
      roots.push(node);
    }
  }
  const year = node => node.year || (node.parent && nodes.has(node.parent) ? year(nodes.get(node.parent)) : '');
  for (const node of nodes.values()) {
    node.year = year(node);
    node.categories = [...headings.values()].filter(h => h.eventId === node.id);
    const parentOk = !node.parent || (nodes.get(node.parent) && nodes.get(node.parent).runs);
    node.can.approve = node.status === 'Pending' && (admin || parentOk && Boolean(node.parent));
    node.can.decline = node.can.approve;
  }
  for (const node of nodes.values()) {
    node.categoryHidden = Boolean(node.category && headings.get(node.category) && headings.get(node.category).hidden);
    node.past = pastOf(node, nodes, now);
  }
  const family = [];
  for (const f of dir.families[viewer.id] || []) {
    for (const p of dir.members[f] || []) {
      if (p !== viewer.id && dir.byId[p] && !family.some(x => x.id === p)) {
        family.push({id: p, email: emailOf(dir.byId[p]), name: dir.byId[p].name_show});
      }
    }
  }
  applyModel({
    user: {id: viewer.id, email: emailOf(viewer), name: viewer.name_show, family},
    allowances: admin ? ['team.see-all', 'team.curate', 'team.configure', 'team.act-as-cochair'] : [],
    everyone: answers.everyone.result[0] || '',
    years: schoolYears(new Date()),
    settings: {intro: settings['Intro'] || '', expenseFormUrl: settings['Expense Form URL'] || ''},
    settingRows: rowsOf(answers.settings, 'SETTING'),
    rules: rowsOf(answers.rules, 'RULE'),
    yearGroups: Object.values(groups).filter(isYear),
    gradeColors: {},
    imageSearch: true,
    categories: yearHeadings,
    headings,
    activities: roots,
  });
}

function pastOf(node, nodes, now) {
  if (node.status === 'Done') {
    return true;
  }
  let dated = node;
  while (!dated.start && !dated.timing && dated.parent && nodes.has(dated.parent)) {
    dated = nodes.get(dated.parent);
  }
  const last = parseWhen(dated.end || dated.start);
  return Boolean(last) && last.date < now;
}

function applyModel(model) {
  state.model = model;
  index.clear();
  const add = (list, parent) => {
    for (const a of list) {
      index.set(a.id, a);
      a.own = {start: a.start || '', end: a.end || '', timing: a.timing || ''};
      a.whenFrom = null;
      if (parent && !a.own.start && !a.own.timing) {
        a.start = parent.start || '';
        a.end = parent.end || '';
        a.timing = parent.timing || '';
        a.whenFrom = parent.whenFrom || parent;
      }
      add(a.children, a);
    }
  };
  add(model.activities, null);
}

export function me() {
  return state.model.user;
}

export function allows(name) {
  return state.model.allowances.includes(name);
}

export function years() {
  return state.model.years;
}

export function activity(id) {
  return index.get(id) || null;
}

export function yearGroup(year) {
  return state.model.yearGroups.find(g => g.name === year) || null;
}

export function addLabel(thing) {
  return thing && thing.allowAdding === ADDING.approval ? 'Suggest' : 'Add';
}

export function headingChoices(year, filter) {
  return state.model.categories.filter(c => c.year === year && (!filter || filter(c))).map(c => ({label: c.title, value: c.id}));
}

export function category(id) {
  return (id && state.model.headings.get(id)) || null;
}

export function eventCategories(root) {
  const fresh = activity(root.id) || root;
  return fresh.categories || [];
}

export function activitiesIn(year) {
  return state.model.activities.filter(a => a.year === year);
}

export function allYears() {
  const set = new Set(state.model.activities.map(a => a.year));
  set.add(years().current);
  return [...set].sort().reverse();
}

export function descendants(node) {
  const out = [];
  const walk = list => {
    for (const c of list) {
      out.push(c);
      walk(c.children);
    }
  };
  walk(node.children);
  return out;
}

export function parentOf(node) {
  return node.parent ? activity(node.parent) : null;
}

export function rootOf(node) {
  let top = node;
  for (let up = parentOf(top); up; up = parentOf(top)) {
    top = up;
  }
  return top;
}

export function resolvePath(path) {
  const segs = path.split('/').filter(Boolean).map(decodeURIComponent);
  if (segs.length < 2 || (segs[0] !== 'v' && segs[0] !== 'activities')) {
    return null;
  }
  const want = segs[segs.length - 1].toLowerCase();
  for (const node of index.values()) {
    if (node.id === segs[segs.length - 1] || node.prettyId === want) {
      return node;
    }
  }
  return null;
}

export async function resolveAlias(path) {
  const segs = path.split('/').filter(Boolean).map(decodeURIComponent);
  const last = segs[segs.length - 1] || '';
  const answer = await api('QUERY', '/api/q', ['from', 'ALIAS', ['where', ['=', {n: 'alias'}, last]], ['columns', 'target']]);
  for (const id of answer.result) {
    const node = activity(answer.resources.ALIAS[id].target);
    if (node) {
      return node;
    }
  }
  return null;
}

const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
const dateFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});
const timeFormat = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});
const longFormat = new Intl.DateTimeFormat('en-US', {month: 'long', day: 'numeric', year: 'numeric'});

function sameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

export function whenParts(node) {
  if (node.timing) {
    return {words: node.timing};
  }
  const start = parseWhen(node.start);
  if (!start) {
    return {};
  }
  const end = parseWhen(node.end);
  if (end && !sameDay(start.date, end.date)) {
    return {day: `${dateFormat.format(start.date)} – ${dateFormat.format(end.date)}`};
  }
  return {day: dayFormat.format(start.date), time: start.hasTime ? timeFormat.format(start.date) : ''};
}

export function whenLabel(node) {
  if (node.timing) {
    return node.timing;
  }
  const start = parseWhen(node.start);
  if (!start) {
    return '';
  }
  const end = parseWhen(node.end);
  if (end && !sameDay(start.date, end.date)) {
    return `${dateFormat.format(start.date)} – ${dateFormat.format(end.date)}`;
  }
  const day = dayFormat.format(start.date);
  return start.hasTime ? `${day} @ ${timeFormat.format(start.date)}` : day;
}

export function formatWhen(date, withTime) {
  const two = n => String(n).padStart(2, '0');
  const day = `${date.getFullYear()}-${two(date.getMonth() + 1)}-${two(date.getDate())}`;
  return withTime ? `${day} ${two(date.getHours())}:${two(date.getMinutes())}` : day;
}

export function shiftedEnd(was, now, end, withTime) {
  const from = parseWhen(was);
  const to = parseWhen(now);
  const current = parseWhen(end);
  if (!from || !to || !current) {
    return '';
  }
  return formatWhen(new Date(current.date.getTime() + (to.date - from.date)), withTime);
}

export function defaultEnd(start) {
  const s = parseWhen(start);
  if (!s) {
    return '';
  }
  if (!s.hasTime) {
    return formatWhen(s.date, false);
  }
  return formatWhen(new Date(s.date.getTime() + 2 * 60 * 60 * 1000), true);
}

export function longDate(s) {
  const when = parseWhen(s);
  return when ? longFormat.format(when.date) : s;
}

export function sortByStart(list) {
  return [...list].sort((a, b) => {
    const sa = parseWhen(a.start);
    const sb = parseWhen(b.start);
    if (sa && sb) {
      return sa.date - sb.date;
    }
    if (sa || sb) {
      return sa ? -1 : 1;
    }
    return 0;
  });
}

export function listHidden(node) {
  return Boolean(node.volunteersHidden);
}

export function listRevealed(node, editing) {
  return !listHidden(node) || editing || allows('team.see-all') || (state.showHidden && Boolean(node.canEdit));
}

export function shownVolunteers(node, editing) {
  if (listRevealed(node, editing)) {
    return node.volunteers;
  }
  const mine = me().email;
  return node.volunteers.filter(v => v.position === 'Co-Chair' || v.email === mine || isFamily(v.email));
}

export function coChairs(node) {
  return node.volunteers.filter(v => v.position === 'Co-Chair');
}

export function mySignUp(node) {
  return signUpOf(node, me().email);
}

export function signUpOf(node, email) {
  return node.volunteers.find(v => v.email === email) || null;
}

export function family() {
  return me().family;
}

export function isFamily(email) {
  return family().some(c => c.email === email);
}

export function canJoin(node) {
  return node.status === 'Open' && !node.full && Boolean(node.directSignUp) && !mySignUp(node) && Boolean(node.can['sign-up']);
}

export function myRows(email = me().email) {
  const rows = [];
  for (const root of state.model.activities) {
    for (const node of [root, ...descendants(root)]) {
      const v = signUpOf(node, email);
      if (v) {
        rows.push({act: node, volunteer: v});
      }
    }
  }
  return rows;
}

export function pendingItems() {
  const out = [];
  for (const root of state.model.activities) {
    for (const node of [root, ...descendants(root)]) {
      if (node.status === 'Pending') {
        out.push({act: node});
      }
    }
  }
  return out;
}

export function isUnlisted(node) {
  return node.status === 'Hidden' || node.status === 'Pending' || Boolean(node.categoryHidden);
}

export function revealed(node) {
  return !isUnlisted(node) || (state.showHidden && Boolean(node.canEdit));
}

export function runsAnything() {
  for (const node of index.values()) {
    if (node.runs) {
      return true;
    }
  }
  return false;
}

export function selectedYear() {
  const options = allYears();
  return state.year && options.includes(state.year) ? state.year : years().current;
}

export function yearPath() {
  const year = selectedYear();
  return year === years().current ? '/' : `/years/${encodeURIComponent(year)}`;
}

function categorySlug(c) {
  return c.title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || c.id;
}

export function categoryPath(id) {
  if (id === PRIORITY) {
    return yearPath() + '?category=' + PRIORITY;
  }
  const c = category(id);
  return yearPath() + (c ? '?category=' + encodeURIComponent(categorySlug(c)) : '');
}

export const PRIORITY = 'high-priority';

export function isPriority(node) {
  return (Boolean(node.priority) && !node.full) || (node.children || []).some(isPriority);
}

export function categoryFromAddress() {
  const want = new URLSearchParams(location.search).get('category');
  if (!want) {
    return '';
  }
  if (want === PRIORITY) {
    return PRIORITY;
  }
  const c = state.model.categories.find(c => c.year === selectedYear() && (categorySlug(c) === want || c.id === want));
  return c ? c.id : '';
}

export function listedIn(year) {
  return activitiesIn(year).filter(a =>
    (state.showPrevious || (!a.past && !a.full)) && revealed(a));
}

export function matches(node, query) {
  if (!query) {
    return true;
  }
  return `${node.title} ${node.description || ''}`.toLowerCase().includes(query);
}
