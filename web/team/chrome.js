import {state, me, isAdmin, pendingItems, selectedYear, yearPath, categoryPath, listedIn, years, resolvePath, rootOf, eventCategories, descendants, activityPath, family, myRows, isPrevious, revealed, runsAnything, parentOf, shownVolunteers, activitiesIn, allYears, sortByStart, matches} from './state.js';
import {parseWhen} from '/datecard.js';
import {el, svg, link, button, imageThumb} from '/elements.js';
import {initShell, appSymbol, searchInput} from '/shell.js';
import {navigate, render, setPath} from '/router.js';
import {openActivity} from './edit.js';

const primary = [
  {href: '/', icon: 'app', label: 'Opportunities'},
  {href: '/my', icon: 'star', label: 'My Sign Ups'},
  {href: '/calendar', icon: 'calendar', label: 'Calendar'},
  {href: '/approvals', icon: 'join', label: 'Approval Needed', admin: true, count: () => pendingItems().length},
];

function navItems() {
  return primary.filter(item => !item.admin || isAdmin());
}

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return path === '/' || path.startsWith('/years/') || path.startsWith('/activities/') || path.startsWith('/v/');
  }
  return path === href || path.startsWith(href + '/');
}

function navLink(item) {
  const href = item.href === '/' ? yearPath() : item.href;
  const a = link(href, active(item.href) ? 'is-active' : '');
  a.append(item.icon === 'app' ? appSymbol() : svg(item.icon), el('span', '', item.label));
  const n = item.count ? item.count() : 0;
  if (n) {
    a.append(el('span', 'nav-count', String(n)));
  }
  return a;
}

function suggestButton() {
  return button('Suggest an Idea', 'bulb', 'button nav-action', () => {
    const ideas = state.model.categories.find(c => c.title === 'Just an Idea');
    openActivity(null, {category: ideas ? ideas.id : ''});
  });
}

function categoryLinks() {
  const wrap = el('div', 'nav-sub');
  const counts = new Map();
  for (const a of listedIn(selectedYear())) {
    counts.set(a.category, (counts.get(a.category) || 0) + 1);
  }
  for (const c of state.model.categories) {
    const n = counts.get(c.id) || 0;
    if (!n) {
      continue;
    }
    const on = state.category === c.id;
    const item = el('button', 'nav-sub-item' + (on ? ' is-on' : ''));
    item.type = 'button';
    item.append(el('span', 'nav-sub-name', c.title), el('span', 'nav-sub-count', String(n)));
    item.addEventListener('click', () => navigate(categoryPath(on ? '' : c.id)));
    wrap.append(item);
  }
  return wrap.children.length ? wrap : null;
}

function familyLinks() {
  const people = family();
  if (!people.length) {
    return null;
  }
  const wrap = el('div', 'nav-sub');
  const current = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const on = current[0] === 'my' ? current[1] || '' : null;
  const count = email => myRows(email).filter(r => r.act.year === years().current && (state.showPrevious || !isPrevious(r.act))).length;
  const add = (href, label, email, self) => {
    const a = link(href, 'nav-sub-item nav-family-item' + (on === (self ? '' : email) ? ' is-on' : ''));
    a.append(el('span', 'nav-sub-name', label), el('span', 'nav-sub-count', String(count(email))));
    wrap.append(a);
  };
  add('/my', 'Me', me().email, true);
  for (const c of people) {
    add(`/my/${encodeURIComponent(c.email)}`, c.name, c.email, false);
  }
  return wrap;
}

function currentActivity() {
  const first = location.pathname.split('/').filter(Boolean)[0];
  return first === 'activities' || first === 'v' ? resolvePath(location.pathname) : null;
}

const openGroups = new Set();

function signUps(node) {
  return node.taken + descendants(node).reduce((n, d) => n + d.taken, 0);
}

function countLabel(node) {
  if (node.spots) {
    return `${node.taken} of ${node.spots}`;
  }
  return String(signUps(node));
}

function eventTree(current) {
  const root = rootOf(current);
  const wrap = el('div', 'nav-event');
  const head = link(activityPath(root), 'nav-event-link' + (current === root ? ' is-active' : ''));
  head.append(svg('join'), el('span', '', root.title));
  wrap.append(head);
  const shown = revealed;
  const list = el('div', 'nav-sub nav-tree');
  const grouped = eventCategories(root).length > 0;
  const flip = key => {
    if (openGroups.has(key)) {
      openGroups.delete(key);
    } else {
      openGroups.add(key);
    }
    renderNav();
  };
  const disclosure = (key, open) => {
    const b = el('button', 'nav-tree-toggle' + (open ? ' is-open' : ''));
    b.type = 'button';
    b.setAttribute('aria-expanded', String(open));
    b.setAttribute('aria-label', open ? 'Collapse' : 'Expand');
    b.append(svg('chevron-right'));
    b.addEventListener('click', e => {
      e.preventDefault();
      e.stopPropagation();
      flip(key);
    });
    return b;
  };
  const item = (node, depth) => {
    const row = el('div', 'nav-tree-row');
    row.style.paddingLeft = `${(grouped ? 22 : 0) + depth * 14}px`;
    const kids = node.children.filter(shown);
    const key = 'node/' + node.id;
    const open = kids.length > 0 && openGroups.has(key);
    row.append(kids.length ? disclosure(key, open) : el('span', 'nav-tree-toggle is-leaf'));
    const a = link(activityPath(node), 'nav-sub-item nav-tree-item' + (node === current ? ' is-on' : ''));
    a.append(el('span', 'nav-sub-name', node.title), el('span', 'nav-sub-count', countLabel(node)));
    row.append(a);
    list.append(row);
    if (open) {
      kids.forEach(k => item(k, depth + 1));
    }
  };
  const groups = [...eventCategories(root).map(c => ({id: c.id, title: c.title})), {id: '', title: 'Uncategorized'}];
  const children = root.children.filter(shown);
  for (const group of groups) {
    const members = children.filter(c => (c.category || '') === group.id);
    if (!members.length) {
      continue;
    }
    if (!grouped) {
      members.forEach(m => item(m, 0));
      continue;
    }
    const key = root.id + '/' + group.id;
    const open = openGroups.has(key);
    const toggle = el('button', 'nav-sub-item nav-tree-group' + (open ? ' is-open' : ''));
    toggle.type = 'button';
    toggle.setAttribute('aria-expanded', String(open));
    toggle.append(svg('chevron-right'), el('span', 'nav-sub-name', group.title),
      el('span', 'nav-sub-count', String(members.reduce((n, m) => n + signUps(m), 0))));
    toggle.addEventListener('click', () => flip(key));
    list.append(toggle);
    if (open) {
      members.forEach(m => item(m, 0));
    }
  }
  if (list.children.length) {
    wrap.append(list);
  }
  return wrap;
}

function fillNav(nav) {
  const current = currentActivity();
  for (const item of navItems()) {
    nav.append(navLink(item));
    if (item.href === '/') {
      const categories = categoryLinks();
      if (categories) {
        nav.append(categories);
      }
      if (current) {
        nav.append(eventTree(current));
      }
    }
    if (item.href === '/my') {
      const people = familyLinks();
      if (people) {
        nav.append(people);
      }
    }
  }
  nav.append(suggestButton());
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  fillNav(nav);
}

function fillTabbar(bar) {
  for (const item of primary.filter(i => !i.admin)) {
    bar.append(navLink(item));
  }
}

const hiddenRow = el('label', 'user-menu-toggle user-menu-hidden');
const hiddenBox = el('input', 'show-hidden-checkbox');
hiddenBox.type = 'checkbox';
hiddenRow.hidden = true;
hiddenRow.append(el('span', '', 'Show Hidden Things'), hiddenBox);
hiddenBox.addEventListener('change', () => {
  state.showHidden = hiddenBox.checked;
  render();
});

function renderHiddenRow() {
  hiddenRow.hidden = !isAdmin() && !runsAnything();
  hiddenBox.checked = state.showHidden;
}

let resultRows = [];
let activeRow = -1;
const dayFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});

function resultsNode() {
  return document.querySelector('#search-results');
}

function closeResults() {
  const node = resultsNode();
  node.hidden = true;
  node.replaceChildren();
  resultRows = [];
  activeRow = -1;
}

function startOf(node) {
  return node.start || rootOf(node).start;
}

function past(node) {
  return isPrevious(node) || isPrevious(rootOf(node));
}

function found(year, query) {
  const out = [];
  const walk = list => {
    for (const node of list.filter(revealed)) {
      if (matches(node, query)) {
        out.push({start: startOf(node), node});
      }
      for (const person of shownVolunteers(node)) {
        if ((person.name || '').toLowerCase().includes(query)) {
          out.push({start: startOf(node), node, person});
        }
      }
      walk(node.children || []);
    }
  };
  walk(activitiesIn(year));
  return sortByStart(out);
}

function resultRow({node: act, person}) {
  const row = link(activityPath(act), 'search-row');
  const when = parseWhen(startOf(act));
  row.append(person ? imageThumb(person.photoUrl, person.name, 'search-row-pic') : imageThumb(act.imageUrl || rootOf(act).imageUrl, act.title, 'search-row-pic'));
  row.append(el('span', 'search-row-day', when ? dayFormat.format(when.date) : 'All Year'));
  const body = el('span', 'search-row-body');
  body.append(el('span', 'search-row-title', person ? person.name : act.title));
  const parent = parentOf(act);
  const c = state.model.categories.find(c => c.id === act.category);
  const where = person ? `${person.position || 'Volunteer'} · ${act.title}` : parent ? parent.title : c ? c.title : '';
  const line = [where, past(act) ? 'Past' : ''].filter(Boolean).join(' · ');
  if (line) {
    body.append(el('span', 'search-row-line', line));
  }
  row.append(body);
  row.addEventListener('mousedown', e => e.preventDefault());
  row.addEventListener('click', closeResults);
  row.addEventListener('mouseenter', () => setActive(resultRows.indexOf(row)));
  resultRows.push(row);
  return row;
}

export function showResults(query) {
  const node = resultsNode();
  closeResults();
  if (!query) {
    return;
  }
  const year = selectedYear();
  const options = allYears();
  const earlier = options[options.indexOf(year) + 1] || '';
  const hits = found(year, query);
  const current = [...hits.filter(hit => !past(hit.node)), ...hits.filter(hit => past(hit.node))];
  const older = earlier ? found(earlier, query) : [];
  if (!current.length && !older.length) {
    node.append(el('div', 'search-empty', 'Nothing matches.'));
    node.hidden = false;
    return;
  }
  for (const hit of current) {
    node.append(resultRow(hit));
  }
  if (older.length) {
    const divider = el('div', 'search-divider');
    divider.append(el('span', '', earlier));
    node.append(divider);
  }
  for (const hit of older) {
    const row = resultRow(hit);
    row.classList.add('is-earlier');
    node.append(row);
  }
  node.hidden = false;
}

function setActive(index) {
  activeRow = index;
  resultRows.forEach((row, i) => row.classList.toggle('is-active', i === index));
  if (index >= 0) {
    resultRows[index].scrollIntoView({block: 'nearest'});
  }
}

function onSearchKey(e) {
  if (resultsNode().hidden) {
    return;
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault();
    if (!resultRows.length) {
      return;
    }
    const step = e.key === 'ArrowDown' ? 1 : -1;
    setActive((activeRow + step + resultRows.length) % resultRows.length);
  } else if (e.key === 'Enter') {
    if (activeRow >= 0) {
      e.preventDefault();
      resultRows[activeRow].click();
    }
  } else if (e.key === 'Escape') {
    e.preventDefault();
    closeResults();
    searchInput().blur();
  }
}

export function initChrome() {
  initShell({
    name: 'HCA-Team',
    me,
    fillNav,
    fillTabbar,
    search: {
      placeholder: 'Search opportunities…',
      results: true,
      carry: () => setPath(yearPath()),
    },
    menuRows: [hiddenRow],
    afterRender: renderHiddenRow,
  });
  searchInput().addEventListener('keydown', onSearchKey);
  searchInput().addEventListener('blur', closeResults);
}
