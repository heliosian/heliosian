import {parseWhen} from '/datecard.js';
import {superEditOn} from '/superedit.js';

export const state = {model: null, showPrevious: false, showHidden: false, year: '', category: ''};

const index = new Map();

export function applyModel(model) {
  state.model = model;
  index.clear();
  const add = (list, parent) => {
    for (const a of list) {
      index.set(a.id, a);
      a.canEdit = Boolean(a.runs) || isAdmin();
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

export function isAdmin() {
  return state.model.user.isAdmin && superEditOn();
}

export function isSystemAdmin() {
  return state.model.user.isAdmin;
}

export function years() {
  return state.model.years;
}

export function activity(id) {
  return index.get(id) || null;
}

export const UNCATEGORIZED = 'uncategorized';

export const ADDING = {yes: 'Yes', approval: 'Approval Needed', no: 'No'};

export function canAdd(thing) {
  return Boolean(thing) && thing.allowAdding !== ADDING.no && Boolean(thing.allowAdding);
}

export function addLabel(thing) {
  return thing && thing.allowAdding === ADDING.approval ? 'Suggest' : 'Add';
}

export function headingChoices(filter) {
  const headings = state.model.categories.filter(c => !c.builtIn && (!filter || filter(c)));
  const choices = headings.map(c => ({label: c.title, value: c.id}));
  choices.push({label: 'Uncategorized', value: UNCATEGORIZED});
  return choices;
}

export function category(id) {
  if (!id) {
    return null;
  }
  const heading = state.model.categories.find(c => c.id === id);
  if (heading) {
    return heading;
  }
  for (const root of state.model.activities) {
    const own = (root.categories || []).find(c => c.id === id);
    if (own) {
      return own;
    }
  }
  return null;
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

export function activityPath(act) {
  const own = act.prettyId ? encodeURIComponent(act.prettyId) : encodeURIComponent(act.id);
  const parent = parentOf(act);
  if (parent) {
    return `${activityPath(parent)}/${own}`;
  }
  return act.prettyId ? `/v/${own}` : `/activities/${own}`;
}

export function byPretty(pretty) {
  const want = (pretty || '').toLowerCase();
  return state.model.activities.find(a => a.prettyId === want) || null;
}

function walkPath(path) {
  const segs = path.split('/').filter(Boolean).map(decodeURIComponent);
  if (segs.length < 2) {
    return null;
  }
  let node = segs[0] === 'v' ? byPretty(segs[1]) : (segs[0] === 'activities' ? activity(segs[1]) : null);
  for (const seg of segs.slice(2)) {
    if (!node) {
      return null;
    }
    const want = seg.toLowerCase();
    node = node.children.find(c => c.id === seg || (c.prettyId && c.prettyId === want)) || null;
  }
  return node;
}

export function resolvePath(path) {
  let at = normalizePath(path);
  for (let hops = 0; hops < 20 && at && !isURL(at); hops++) {
    const live = walkPath(at);
    if (live) {
      return live;
    }
    at = moved(at);
  }
  return null;
}

function isURL(s) {
  return /^https?:\/\//i.test(s);
}

function normalizePath(path) {
  let at = (path || '').replace(/\/+$/, '');
  if (at && !at.startsWith('/')) {
    at = '/v/' + at;
  }
  return at;
}

function moved(at) {
  let to = '';
  let matched = '';
  const lower = at.toLowerCase();
  for (const r of state.model.redirects || []) {
    const old = r.old.toLowerCase();
    if (old === lower) {
      to = r.new;
      matched = at;
    } else if (lower.startsWith(old + '/') && r.old.length > matched.length) {
      to = r.new.replace(/\/$/, '') + at.slice(r.old.length);
      matched = r.old;
    }
  }
  return to;
}

export function redirectTarget(path) {
  const start = normalizePath(path);
  if (!start || walkPath(start)) {
    return '';
  }
  let at = start;
  const seen = new Set([start.toLowerCase()]);
  for (let hops = 0; hops < 20; hops++) {
    const next = moved(at);
    if (!next) {
      break;
    }
    if (isURL(next)) {
      return next;
    }
    const live = walkPath(next);
    if (live) {
      return activityPath(live);
    }
    if (seen.has(next.toLowerCase())) {
      return '';
    }
    seen.add(next.toLowerCase());
    at = next;
  }
  return at === start || !/^\/(?![/\\])/.test(at) ? '' : at;
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

export function isPrevious(node) {
  if (node.status === 'Done') {
    return true;
  }
  const last = parseWhen(node.end) || parseWhen(node.start);
  if (!last) {
    return false;
  }
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return last.date < today;
}

export function listHidden(node) {
  return Boolean(node.volunteersHidden);
}

export function listRevealed(node, editing) {
  return !listHidden(node) || editing || isAdmin() || (state.showHidden && Boolean(node.canEdit));
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
  const user = me();
  return [...(user.spouses || []), ...(user.children || [])];
}

export function isFamily(email) {
  return family().some(c => c.email === email);
}

export function isFull(node) {
  return Boolean(node.volunteersComplete) || (node.spots > 0 && node.taken >= node.spots);
}

export function canJoin(node) {
  if (node.status !== 'Open' || mySignUp(node) || isFull(node)) {
    return false;
  }
  return Boolean(node.directSignUp);
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
  return node.status === 'Hidden' || node.status === 'Pending';
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
  const c = id && state.model.categories.find(c => c.id === id);
  return yearPath() + (c ? '?category=' + encodeURIComponent(categorySlug(c)) : '');
}

export const PRIORITY = 'high-priority';

export function isPriority(node) {
  return (Boolean(node.priority) && !isFull(node)) || (node.children || []).some(isPriority);
}

export function categoryFromAddress() {
  const want = new URLSearchParams(location.search).get('category');
  if (!want) {
    return '';
  }
  if (want === PRIORITY) {
    return PRIORITY;
  }
  const c = state.model.categories.find(c => categorySlug(c) === want || c.id === want);
  return c ? c.id : '';
}

export function listedIn(year) {
  return activitiesIn(year).filter(a =>
    (state.showPrevious || !isPrevious(a)) && revealed(a));
}

export function matches(node, query) {
  if (!query) {
    return true;
  }
  return `${node.title} ${node.description || ''}`.toLowerCase().includes(query);
}
