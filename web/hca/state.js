export const state = {model: null, showPrevious: false, showHidden: false, year: '', category: ''};

const index = new Map();

// The index holds every activity, root or child, by id - that is what makes a
// child reachable by its own URL, and what lets two things share a title.
export function applyModel(model) {
  state.model = model;
  index.clear();
  const add = list => {
    for (const a of list) {
      index.set(a.id, a);
      add(a.children);
    }
  };
  add(model.activities);
}

export function me() {
  return state.model.user;
}

export function isAdmin() {
  return state.model.user.isAdmin;
}

export function years() {
  return state.model.years;
}

export function activity(id) {
  return index.get(id) || null;
}

// category finds one by id wherever it lives: a page heading, or one of some
// root event's own. The page's list and every root's list are both searched.
// UNCATEGORIZED is the built-in heading a root lands under when its category is
// blank or names nothing. The server adds it to the model only when something
// needs it, so pickers use headingChoices() to always offer it.
export const UNCATEGORIZED = 'uncategorized';

// headingChoices is the page's headings as select options, ending with
// Uncategorized whether or not the model currently carries it.
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

// eventCategories are a root event's own categories - what the things under it
// are grouped by. Empty for anything that is not a root.
export function eventCategories(root) {
  // Look the event up again rather than trusting the object handed in: a modal
  // that reopens itself after a save still holds the node from before the
  // reload, whose categories are the old ones.
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

// descendants walks everything under an activity, in row order.
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

// rootOf climbs to the top of the tree - the thing whose category and image the
// whole branch inherits.
export function rootOf(node) {
  let top = node;
  for (let up = parentOf(top); up; up = parentOf(top)) {
    top = up;
  }
  return top;
}

// activityPath is the address a link uses, the same one the server builds
// (Model.PathOf): /v/{pretty} for a root with a friendly address, otherwise
// /activities/{id}; and under an event each thing adds its own segment, its
// friendly address or its id - /v/inight/poland.
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

// walkPath follows one path to a thing, or null: the first segment names a
// root by friendly address (/v/) or id (/activities/), each further segment one
// of the children by friendly address or id. A child's bare /activities/{id}
// still resolves, so old links keep working.
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

// resolvePath is walkPath plus the Redirects tab, mirroring Model.Resolve: an
// address that has since changed is followed through the chain of renames,
// and a redirect of an event's own address carries the rest of the path along.
export function resolvePath(path) {
  let at = (path || '').replace(/\/+$/, '');
  if (at && !at.startsWith('/')) {
    at = '/v/' + at;
  }
  for (let hops = 0; hops < 20 && at; hops++) {
    const live = walkPath(at);
    if (live) {
      return live;
    }
    let moved = '';
    const lower = at.toLowerCase();
    for (const r of state.model.redirects || []) {
      const old = r.old.toLowerCase();
      if (old === lower) {
        moved = r.new;
      } else if (lower.startsWith(old + '/') && r.old.length > moved.length) {
        moved = r.new + at.slice(r.old.length);
      }
    }
    at = moved;
  }
  return null;
}

export function parseWhen(s) {
  if (!s) {
    return null;
  }
  const m = /^(\d{4})-(\d{2})-(\d{2})(?: (\d{2}):(\d{2}))?$/.exec(s);
  if (!m) {
    return null;
  }
  return {
    date: new Date(+m[1], m[2] - 1, +m[3], m[4] ? +m[4] : 0, m[5] ? +m[5] : 0),
    hasTime: Boolean(m[4]),
  };
}

const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
const dateFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});
const timeFormat = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});
const longFormat = new Intl.DateTimeFormat('en-US', {month: 'long', day: 'numeric', year: 'numeric'});

function sameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

// whenLabel is the small-caps line above a title: the date and time when one
// is set, the free-text timing otherwise.
export function whenLabel(node) {
  const start = parseWhen(node.start);
  if (!start) {
    return node.timing || '';
  }
  const end = parseWhen(node.end);
  if (end && !sameDay(start.date, end.date)) {
    return `${dateFormat.format(start.date)} – ${dateFormat.format(end.date)}`;
  }
  const day = dayFormat.format(start.date);
  return start.hasTime ? `${day} @ ${timeFormat.format(start.date)}` : day;
}

// formatWhen writes a Date back in the shape the sheet holds: "YYYY-MM-DD", or
// "YYYY-MM-DD HH:MM" when this end of a range carries a time.
export function formatWhen(date, withTime) {
  const two = n => String(n).padStart(2, '0');
  const day = `${date.getFullYear()}-${two(date.getMonth() + 1)}-${two(date.getDate())}`;
  return withTime ? `${day} ${two(date.getHours())}:${two(date.getMinutes())}` : day;
}

// shiftedEnd moves an end by however far a start just moved, so editing when
// something begins keeps how long it lasts. Returns '' when there is nothing to
// move or nothing to move it by.
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

// isPrevious tells the Show Previous Events toggle what to hide: anything done,
// or whose last day has passed.
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

// listHidden says whether a thing's volunteer list is private: its own switch,
// or one on anything above it, the way the server reads it for people who do
// not run the event.
export function listHidden(node) {
  for (let n = node; n; n = parentOf(n)) {
    if (n.volunteersHidden) {
      return true;
    }
  }
  return false;
}

// shownVolunteers is who the page lists. The server sends an organizer the
// whole of a private list, but the page shows an organizer what everyone else
// sees - the co-chairs and themselves - unless they are editing or have Show
// Hidden Things on, so the page they look at is the page people get.
export function shownVolunteers(node, editing) {
  if (!listHidden(node) || editing || state.showHidden) {
    return node.volunteers;
  }
  const mine = me().email;
  return node.volunteers.filter(v => v.position === 'Co-Chair' || v.email === mine);
}

export function coChairs(node) {
  return node.volunteers.filter(v => v.position === 'Co-Chair');
}

export function mySignUp(node) {
  return node.volunteers.find(v => v.email === me().email) || null;
}

export function isFull(node) {
  return node.spots > 0 && node.taken >= node.spots;
}

// A root that does not take sign-ups itself sends people to the things under it;
// anything with a parent always takes them.
export function canJoin(node) {
  if (node.status !== 'Open' || mySignUp(node) || isFull(node)) {
    return false;
  }
  return node.parent ? true : node.directSignUp;
}

// myRows lists every sign-up of the viewer's, activity-level and role-level.
export function myRows() {
  const rows = [];
  for (const root of state.model.activities) {
    for (const node of [root, ...descendants(root)]) {
      const v = mySignUp(node);
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

// Hidden and pending are both kept off the grid: an idea nobody has approved yet
// is no more public than one an admin has hidden.
export function isUnlisted(node) {
  return node.status === 'Hidden' || node.status === 'Pending';
}

// selectedYear is the school year the opportunities page is showing, falling
// back to the current one when nothing is chosen or the choice went stale.
export function selectedYear() {
  const options = allYears();
  return state.year && options.includes(state.year) ? state.year : years().current;
}

// listedIn is what that page shows for a year before the search box and the
// category chips narrow it further. The rail's per-category counts and the grid
// itself both go through here, so the numbers cannot drift from the cards.
export function listedIn(year) {
  return activitiesIn(year).filter(a =>
    (state.showPrevious || !isPrevious(a)) && (state.showHidden || !isUnlisted(a)));
}

export function matches(node, query) {
  if (!query) {
    return true;
  }
  return `${node.title} ${node.description || ''}`.toLowerCase().includes(query);
}
