import {state, me, isAdmin, pendingItems, selectedYear, listedIn, years, resolvePath, rootOf, eventCategories, descendants, activityPath, isSystemAdmin, setSuperEdit} from './state.js';
import {el, svg, link, button} from './dom.js';
import {githubBadge} from '/github-badge.js';
import {openActivity} from './edit.js';

// The rail and the drawer show these; the mobile tab bar drops the admin ones.
// /admin is deliberately absent - Admin Tools is reached from the account menu.
const primary = [
  {href: '/', icon: 'signup', label: 'Opportunities'},
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

// yearPath is the opportunities page for the chosen year: the root for the
// current year, /years/... for any other, matching what the year dropdown puts
// in the address bar.
function yearPath() {
  const year = selectedYear();
  return year === years().current ? '/' : `/years/${encodeURIComponent(year)}`;
}

function navLink(item) {
  const href = item.href === '/' ? yearPath() : item.href;
  const a = link(href, active(item.href) ? 'is-active' : '');
  if (item.href === '/') {
    // Opportunities is the whole year: it drops whatever category a chip or
    // the rail had narrowed the page to, but keeps the year.
    a.addEventListener('click', () => {
      state.category = '';
    });
  }
  a.append(svg(item.icon), el('span', '', item.label));
  const n = item.count ? item.count() : 0;
  if (n) {
    a.append(el('span', 'nav-count', String(n)));
  }
  return a;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.user-menu')) {
    menu.hidden = true;
  }
}

// Suggesting an idea is the one thing anyone can start from anywhere, so it sits
// under the nav rather than on the opportunities page. edit.js reaches app.js
// through a dynamic import, so importing it here makes no cycle.
function suggestButton() {
  // Ideas file under whichever category is named "Just an Idea", if there is
  // one; otherwise the editor's default category stands.
  return button('Suggest an Idea', 'idea', 'button nav-action', () => {
    const ideas = state.model.categories.find(c => c.title === 'Just an Idea');
    openActivity(null, {category: ideas ? ideas.id : ''});
  });
}

// The categories sit under Opportunities as a sub-list, each with how many
// activities it holds for the chosen year. The count comes from listedIn(), the
// same helper the grid filters through, so a category showing 3 here cannot show
// a different number of cards - and both follow the Show Previous / Show Hidden
// switches. Clicking one filters the grid rather than opening a page of its own.
function categoryLinks() {
  const wrap = el('div', 'nav-sub');
  const counts = new Map();
  for (const a of listedIn(selectedYear())) {
    counts.set(a.category, (counts.get(a.category) || 0) + 1);
  }
  for (const c of state.model.categories) {
    const n = counts.get(c.id) || 0;
    // An empty category is nothing to click through to, so it drops out of the
    // rail until the year or the switches bring something back into it.
    if (!n) {
      continue;
    }
    const on = state.category === c.id;
    const item = el('button', 'nav-sub-item' + (on ? ' is-on' : ''));
    item.type = 'button';
    item.append(el('span', 'nav-sub-name', c.title), el('span', 'nav-sub-count', String(n)));
    item.addEventListener('click', async () => {
      state.category = on ? '' : c.id;
      const {navigate} = await import('./app.js');
      navigate(location.pathname === '/' ? location.pathname : '/');
    });
    wrap.append(item);
  }
  return wrap.children.length ? wrap : null;
}

// currentActivity is the thing whose page is open, by either of its addresses,
// or null on every other page.
function currentActivity() {
  const first = location.pathname.split('/').filter(Boolean)[0];
  return first === 'activities' || first === 'v' ? resolvePath(location.pathname) : null;
}

// Which of an event's category groups are open in the rail, by root id and
// category id. Kept across renders - every navigation redraws the rail - so a
// group someone opened stays open while they move around the event.
const openGroups = new Set();

// signUps is a thing's sign-ups and those of everything under it - the number
// the rail shows against a committee. Taken counts a private list too, so the
// number is right for whoever looks.
function signUps(node) {
  return node.taken + descendants(node).reduce((n, d) => n + d.taken, 0);
}

// countLabel is "3", or "2 of 5" when the thing itself is after a set number.
function countLabel(node) {
  if (node.spots) {
    return `${node.taken} of ${node.spots}`;
  }
  return String(signUps(node));
}

// eventTree is the rail's entry for the event whose page is open: the event
// itself, then its committees grouped by the event's categories. Every group,
// and every committee with things under it, starts closed - the counts say
// what is inside - and stays as it was toggled while moving around the event.
// Hidden and pending things appear only with Show Hidden Things on.
function eventTree(current) {
  const root = rootOf(current);
  const wrap = el('div', 'nav-event');
  const head = link(activityPath(root), 'nav-event-link' + (current === root ? ' is-active' : ''));
  head.append(svg('join'), el('span', '', root.title));
  wrap.append(head);
  const shown = n => state.showHidden || (n.status !== 'Hidden' && n.status !== 'Pending');
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
  // A disclosure chevron opens a group or a committee with things under it;
  // a leaf gets a blank of the same width so titles line up.
  const disclosure = (key, open) => {
    const b = el('button', 'nav-tree-toggle' + (open ? ' is-open' : ''));
    b.type = 'button';
    b.setAttribute('aria-expanded', String(open));
    b.setAttribute('aria-label', open ? 'Collapse' : 'Expand');
    b.append(svg('chevron'));
    b.addEventListener('click', e => {
      e.preventDefault();
      e.stopPropagation();
      flip(key);
    });
    return b;
  };
  // Rows under a group start where the group's title starts (past its chevron)
  // and step in again for each level below that; without groups, from the edge.
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
    toggle.append(svg('chevron'), el('span', 'nav-sub-name', group.title),
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

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
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
  }
  nav.append(suggestButton());
}

function renderTabbar() {
  const bar = document.querySelector('#tabbar');
  bar.replaceChildren();
  for (const item of primary.filter(i => !i.admin)) {
    bar.append(navLink(item));
  }
}

function renderDrawer() {
  const drawer = document.querySelector('#drawer');
  drawer.replaceChildren();
  const head = el('div', 'drawer-head');
  const icon = el('img');
  icon.src = '/brand/logo-mark.png';
  icon.alt = '';
  const close = el('button', 'icon-button');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  close.append(svg('close'));
  close.addEventListener('click', closeDrawer);
  head.append(icon, el('span', '', 'HCA-Team'), close);
  drawer.append(head);
  for (const item of navItems()) {
    drawer.append(navLink(item));
  }
  drawer.append(suggestButton());
  const user = el('div', 'drawer-user');
  user.append(el('div', 'name', me().name), el('div', 'email', me().email));
  const form = el('form');
  form.method = 'post';
  form.action = '/auth/logout';
  form.append(el('button', 'button button-secondary button-small', 'Sign Out'));
  user.append(form);
  drawer.append(user);
}

export function openDrawer() {
  renderDrawer();
  document.querySelector('#drawer-overlay').hidden = false;
}

export function closeDrawer() {
  document.querySelector('#drawer-overlay').hidden = true;
}

function renderUser() {
  const user = me();
  for (const avatar of document.querySelectorAll('.user-avatar')) {
    avatar.replaceChildren();
    if (user.photoUrl) {
      const img = el('img');
      img.src = user.photoUrl;
      img.alt = '';
      avatar.append(img);
    } else {
      avatar.textContent = user.initial;
    }
  }
  document.querySelector('.user-name').textContent = user.name;
  for (const line of document.querySelectorAll('.user-menu-email')) {
    line.textContent = user.email;
  }
  // The switch and Admin Tools go with being on the admin list; the rest of
  // the admin rows come and go with the hat.
  for (const row of document.querySelectorAll('.user-menu-super, .user-menu-system')) {
    row.hidden = !isSystemAdmin();
  }
  for (const box of document.querySelectorAll('.super-edit-checkbox')) {
    box.checked = state.superEdit;
  }
  for (const admin of document.querySelectorAll('.user-menu-admin')) {
    admin.hidden = !isAdmin();
  }
  for (const box of document.querySelectorAll('.show-hidden-checkbox')) {
    box.checked = state.showHidden;
  }
}

// One search box, in the top bar, and each page says what it filters. Pages that
// filter nothing leave it hidden; app.js clears the binding on every route
// change, so a stale handler can never outlive the page that set it.
let onSearch = null;

export function setSearch(placeholder, handler) {
  onSearch = handler;
  for (const id of ['#search-input', '#mobile-search-input']) {
    const input = document.querySelector(id);
    input.value = '';
    input.placeholder = placeholder || 'Search';
  }
  document.querySelector('#topbar-search').hidden = false;
  document.querySelector('#mobile-search-btn').hidden = false;
}

export function clearSearch() {
  onSearch = null;
  document.querySelector('#topbar-search').hidden = true;
  document.querySelector('#mobile-search-btn').hidden = true;
  closeMobileSearch();
}

function openMobileSearch() {
  document.querySelector('#mobile-search-overlay').hidden = false;
  document.querySelector('#mobile-search-input').focus();
}

function closeMobileSearch() {
  document.querySelector('#mobile-search-overlay').hidden = true;
}

export function setTitle(title) {
  document.querySelector('#mobile-title').textContent = title;
  document.title = title === 'HCA-Team' ? title : `${title} · HCA-Team`;
}

export function renderChrome() {
  renderUser();
  renderNav();
  renderTabbar();
}

export function initChrome() {
  document.querySelector('#site-footer').append(githubBadge());
  document.querySelector('#menu-button').append(svg('menu'));
  document.querySelector('#menu-button').addEventListener('click', openDrawer);
  document.querySelector('#mobile-search-btn').append(svg('search'));
  document.querySelector('#mobile-search-close').append(svg('close'));
  document.querySelector('#mobile-search-btn').addEventListener('click', openMobileSearch);
  document.querySelector('#mobile-search-close').addEventListener('click', closeMobileSearch);
  for (const id of ['#search-input', '#mobile-search-input']) {
    const input = document.querySelector(id);
    input.addEventListener('input', () => {
      if (onSearch) {
        onSearch(input.value.trim().toLowerCase());
      }
    });
  }
  document.querySelector('#drawer-overlay').addEventListener('click', e => {
    if (e.target === e.currentTarget) {
      closeDrawer();
    }
  });
  document.querySelector('#drawer').addEventListener('click', e => {
    if (e.target.closest('a')) {
      closeDrawer();
    }
  });
  for (const [button, menu] of [['#user', '#user-menu'], ['#mobile-user', '#mobile-user-menu']]) {
    const panel = document.querySelector(menu);
    document.querySelector(button).addEventListener('click', e => {
      e.stopPropagation();
      const opening = panel.hidden;
      closeMenus();
      panel.hidden = !opening;
    });
  }
  // The switch exists twice (rail menu and mobile menu), so a change on either
  // updates the other. app.js listens for the repaint rather than chrome.js
  // importing render, which would make the two modules import each other.
  for (const box of document.querySelectorAll('.show-hidden-checkbox')) {
    box.addEventListener('change', () => {
      state.showHidden = box.checked;
      for (const other of document.querySelectorAll('.show-hidden-checkbox')) {
        other.checked = box.checked;
      }
      document.dispatchEvent(new CustomEvent('hca:refresh'));
    });
  }
  // Super Edit Mode puts a system admin's hat on or takes it off; the page
  // repaints as the other kind of user.
  for (const box of document.querySelectorAll('.super-edit-checkbox')) {
    box.addEventListener('change', () => {
      setSuperEdit(box.checked);
      document.dispatchEvent(new CustomEvent('hca:refresh'));
    });
  }
  document.addEventListener('click', closeMenus);
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeMenus();
      closeDrawer();
      closeMobileSearch();
    }
  });
}
