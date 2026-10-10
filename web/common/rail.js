import {el, svg, link} from '/elements.js';
import {appOrigin, currentApp} from '/appswitch.js';

export const tagCondition = '(own_group @g) (not (blank managed_by)) (!= status "closed") (not (exists GROUP (= managed_by @g) (!= id @g))) (manages @g)';
const viewerGroups = '(select EFFECTIVE_MEMBER.group (= person @viewer))';
export const runCondition = '(sidebar_group @g) (manages @g)';
export const joinedCondition = `(sidebar_group @g) (not (manages @g)) (or (in id ${viewerGroups}) (in rsvp_yes ${viewerGroups}))`;
export const viewerListCondition = `(sidebar_group @g) (or (manages @g) (in id ${viewerGroups}) (in rsvp_yes ${viewerGroups}))`;
export const listIncludes = '(include parent parent.parent parent.parent.parent)';

export const listKinds = {party: 'party', activity: 'activity', event: 'event', admins: 'admins'};

export function navQueries() {
  return {
    navTags: `(from GROUP @g (where ${tagCondition}) (include managed_by) (columns name managed_by))`,
    navTagManagers: `(from MEMBER (where (= member "yes") (in group (select GROUP.managed_by @g ${tagCondition}))) (columns group person))`,
    navManaged: `(from GROUP @g (where ${runCondition}) (order name asc) ${listIncludes})`,
    navJoined: `(from GROUP @g (where ${joinedCondition}) (order name asc) ${listIncludes})`,
    navMine: '(from EFFECTIVE_MEMBER (where (= person @viewer)) (columns group))',
    navRooms: '(from GROUP (where (= kind "group") (= parent.kind "band") (in id (select EFFECTIVE_MEMBER.group (= person @viewer)))) (include parent) (columns name slug parent))',
  };
}

function rowsOf(answer, table) {
  return answer.result.map(id => answer.resources[table][id]);
}

function all(answer, table) {
  return Object.values(answer.resources[table] || {});
}

export function today() {
  return new Date().toLocaleDateString('en-CA');
}

export function datedFrom(g, groups) {
  for (let at = g; at; at = groups[at.parent]) {
    if (at.start) {
      return at;
    }
  }
  return null;
}

export function liveLists(rows, groups) {
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

export function navLists(answers) {
  const tags = [];
  const tagRows = rowsOf(answers.navTags, 'GROUP');
  const managers = {};
  for (const m of rowsOf(answers.navTagManagers, 'MEMBER')) {
    (managers[m.group] = managers[m.group] || new Set()).add(m.person);
  }
  for (const t of tagRows) {
    if (tagRows.some(other => other.managed_by === t.id && t.name === other.name + ' Managers')) {
      continue;
    }
    tags.push({key: 'tag:' + t.id, id: t.id, name: t.name, slug: '', shared: (managers[t.managed_by] || new Set()).size > 1});
  }
  const groups = {};
  for (const g of [...all(answers.navManaged, 'GROUP'), ...all(answers.navJoined, 'GROUP')]) {
    groups[g.id] = g;
  }
  const run = liveLists(rowsOf(answers.navManaged, 'GROUP'), groups);
  const running = new Set(run.map(g => g.id));
  const mine = new Set(rowsOf(answers.navMine, 'EFFECTIVE_MEMBER').map(e => e.group));
  const lists = [...run, ...liveLists(rowsOf(answers.navJoined, 'GROUP'), groups)].map(g => {
    const dated = datedFrom(g, groups);
    return {
      key: 'list:' + g.id,
      id: g.id,
      name: g.name,
      kind: listKinds[g.kind] || 'group',
      slug: g.slug || '',
      run: running.has(g.id),
      member: mine.has(g.kind === 'event' ? g.rsvp_yes : g.id),
      start: dated ? dated.start : '',
    };
  });
  const bands = answers.navRooms.resources.GROUP || {};
  const rooms = rowsOf(answers.navRooms, 'GROUP');
  const shownElsewhere = new Set([...rooms, ...tagRows].map(g => g.id));
  const kept = lists.filter(l => !shownElsewhere.has(l.id));
  for (const g of rooms) {
    const band = bands[g.parent];
    kept.push({key: 'list:' + g.id, id: g.id, name: `${band ? band.name : g.name} Families`, kind: 'room', slug: g.slug || '', run: true, member: true, start: ''});
  }
  return {tags, lists: kept};
}

export function groupPathOf(item) {
  return '/groups/' + encodeURIComponent(item.slug || item.id);
}

export function navSections({tags, lists}) {
  const backstage = l => l.run && !l.member && l.kind !== 'event';
  const shown = lists.filter(l => !backstage(l));
  const byName = (a, b) => a.name.localeCompare(b.name);
  const tagItems = tags.map(t => ({...t, tag: true, title: t.shared ? `${t.name} - shared with others` : t.name, run: false, start: ''}));
  const listItem = dated => l => ({...l, title: l.name, run: dated && l.run, start: dated ? l.start : ''});
  return [
    {key: 'running', title: 'Running', open: true, items: [...tagItems, ...shown.filter(l => l.run && !l.start).map(listItem(false))].sort(byName)},
    {key: 'upcoming', title: 'Coming Up', open: true, items: shown.filter(l => l.start).sort((a, b) => a.start.localeCompare(b.start)).map(listItem(true))},
    {key: 'joined', title: 'Joined', open: true, items: shown.filter(l => !l.run && !l.start).sort(byName).map(listItem(false))},
    {key: 'managing', title: 'Managing', open: false, items: lists.filter(backstage).sort(byName).map(listItem(false))},
  ].filter(section => section.items.length);
}

const dayFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});

function trimMiddle(text, max) {
  if (text.length <= max) {
    return text;
  }
  const head = Math.ceil((max - 1) * 0.55);
  return text.slice(0, head).trimEnd() + '…' + text.slice(text.length - (max - 1 - head)).trimStart();
}

export function listName(item) {
  const label = el('span', 'nav-list-label');
  label.append(el('span', '', trimMiddle(item.name, 40)));
  if (item.start) {
    label.append(el('span', 'nav-list-date', dayFormat.format(new Date(item.start.slice(0, 10) + 'T00:00'))));
  }
  const marks = el('span', 'nav-list-marks');
  if (item.run) {
    marks.append(el('span', 'nav-list-run', '★'));
  }
  const row = el('span', 'nav-list-row');
  row.append(label, marks);
  return row;
}

function railHeading(nav, {title, icon, open, toggle, after = []}) {
  const heading = el('div', 'nav-heading nav-heading-toggle' + (open ? ' open' : ''));
  const chevron = el('span', 'nav-chevron');
  chevron.append(svg('chevron-down'));
  const mark = icon === 'who' ? el('span', 'app-symbol app-symbol-who') : svg(icon);
  mark.classList.add('nav-heading-icon-' + icon);
  heading.append(chevron, mark, el('span', 'nav-heading-title', title), ...after);
  heading.addEventListener('click', toggle);
  nav.append(heading);
  if (!open) {
    return null;
  }
  const body = el('div', 'nav-section-body');
  nav.append(body);
  queueMicrotask(() => heading.classList.toggle('active', Boolean(body.querySelector('a.active'))));
  return body;
}

function railLink(href, className) {
  const a = link(href, className);
  if (/^https?:/.test(href)) {
    a.removeAttribute('data-link');
  }
  return a;
}

export function railRow(item, active) {
  const a = railLink(item.href, active ? 'active' : '');
  const icon = svg(item.icon);
  icon.classList.add('nav-icon-' + item.icon);
  a.append(icon, el('span', '', item.label));
  return a;
}

export function sectionHeading(container, section, {className, open, toggle}) {
  const heading = el('div', className + ' nav-subheading-toggle' + (open ? ' open' : ''));
  const chevron = el('span', 'nav-chevron');
  chevron.append(svg('chevron-down'));
  heading.append(el('span', '', section.title));
  if (!open) {
    heading.append(el('span', 'nav-subheading-count', String(section.items.length)));
  }
  heading.append(chevron);
  heading.addEventListener('click', () => toggle(!open));
  container.append(heading);
  return open;
}

const directoryRows = [
  {path: 'people', label: 'Directory'},
  {path: 'classrooms', label: 'Gradebands'},
  {path: 'staff', label: 'Staff'},
];

export const listPageRows = [
  {path: 'email-list', label: 'Everyone'},
  {path: 'greenvelope', label: 'Invites'},
];

function navOpen() {
  const raw = localStorage.getItem('navOpen');
  return raw ? JSON.parse(raw) : {directory: true, family: false, tools: true};
}

function setNavOpen(key, open) {
  localStorage.setItem('navOpen', JSON.stringify({...navOpen(), [key]: open}));
}

export function listsOpen(section) {
  return navOpen()['lists-' + section.key] ?? section.open;
}

export function setListsOpen(section, open) {
  setNavOpen('lists-' + section.key, open);
}

export function whoPath(path) {
  return (currentApp() === 'who' ? '' : appOrigin('who')) + '/' + path;
}

export function groupActive(item) {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  return parts[0] === 'groups' && Boolean(parts[1]) && (parts[1] === item.id || parts[1].toLowerCase() === item.slug.toLowerCase());
}

export function fillNav(nav, {here = '', lists, family = null, redraw}) {
  const open = navOpen();
  const toggle = key => {
    setNavOpen(key, !open[key]);
    redraw(nav);
  };
  const pageRow = (body, item) => body.append(railRow({href: whoPath(item.path), icon: item.path, label: item.label}, item.path === here));
  nav.replaceChildren();
  const directory = railHeading(nav, {title: 'Directory', icon: 'who', open: open.directory, toggle: () => toggle('directory')});
  if (directory) {
    for (const item of directoryRows) {
      pageRow(directory, item);
    }
  }
  if (family) {
    const body = railHeading(nav, {title: 'My Family', icon: 'heart', after: family.after, open: open.family || family.forceOpen, toggle: () => toggle('family')});
    if (body) {
      family.fill(body);
    }
  }
  const tools = railHeading(nav, {title: 'Lists', icon: 'list', open: open.tools, toggle: () => toggle('tools')});
  if (!tools) {
    return;
  }
  for (const item of listPageRows) {
    pageRow(tools, item);
  }
  drawNavSections(tools, navSections(lists), {
    href: groupPathOf,
    active: groupActive,
    isOpen: listsOpen,
    toggle: (section, next) => {
      setListsOpen(section, next);
      redraw(nav);
    },
  });
}

export function drawNavSections(container, sections, {href, active, isOpen, toggle}) {
  for (const section of sections) {
    const open = isOpen(section);
    if (!sectionHeading(container, section, {className: 'nav-subheading', open, toggle: next => toggle(section, next)})) {
      continue;
    }
    for (const item of section.items) {
      const a = railLink(href(item));
      a.title = item.title;
      if (active(item)) {
        a.className = 'active';
      }
      a.append(listName(item));
      container.append(a);
    }
  }
}
