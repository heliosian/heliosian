import {model, viewer, familyOf, resetPageState} from './state.js';
import {segments, hue, firstName} from './dom.js';
import {navSections, sectionHeading, listName, railRow, fillNav as drawRail, listPageRows, listsOpen, setListsOpen, groupActive, groupPathOf} from '/rail.js';
import {el, svg, link} from '/elements.js';
import {loadNavScroll, saveNavScroll} from './storage.js';
import {myFamily} from './families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl} from './people.js';
import {tagKeys, tagOf, listKeys, listApp, listOf, sharedTag, onTagsChange, onTagsChangeChrome} from './tags.js';
import {staleItems, familyInfoBanner, familyNavPeople, personTodoCount} from './stale.js';
import {searchResults} from './search.js';
import {privacyMismatchCardDismissed, myPrivacyWarnings, privacyMismatchCard} from './pages/privacy.js';
import {initShell, renderAccount, searchInput, syncViewportHeight, onSlash, isEditableTarget} from '/shell.js';

const mobileNavSections = [
  {path: 'people', label: 'Directory'},
  {path: 'classrooms', label: 'Gradebands'},
  {path: 'staff', label: 'Staff'},
  {path: 'my-family', label: 'My Family'},
  {path: 'email-list', label: 'Lists', isListsTab: true},
];

function onMyFamily(seg) {
  const family = myFamily();
  return seg[0] === 'my-family' || (seg[0] === 'families' && Boolean(family) && seg[1] === family.id);
}

function activeSection() {
  const seg = segments();
  if (seg[0] === 'families') {
    return onMyFamily(seg) ? 'my-family' : 'people';
  }
  if (seg[0] === 'grades') {
    return 'classrooms';
  }
  if (seg[0] === 'groups') {
    return 'people';
  }
  return seg[0];
}

export function setChrome(title, backHref) {
  document.querySelector('#mobile-title').textContent = title;
  const back = document.querySelector('#mobile-back');
  back.hidden = !backHref;
  if (backHref) {
    back.href = backHref;
  }
  updateMobileTitleInset();
}

function updateMobileTitleInset() {
  const bar = document.querySelector('.mobile-top');
  const back = document.querySelector('#mobile-back');
  const barRect = bar.getBoundingClientRect();
  if (!barRect.width) {
    return;
  }
  const leftWidth = back.hidden ? 12 : back.getBoundingClientRect().right - barRect.left;
  document.documentElement.style.setProperty('--mobile-title-inset', leftWidth + 'px');
}

export function preparePage(title, entered) {
  if (entered) {
    resetPageState();
    searchInput().value = '';
    searchResults().hidden = true;
    setMobileListsMenu(false);
  }
  renderNav();
  setChrome(title, null);
  onTagsChange(() => {});
}

export function showPage(node) {
  const main = document.querySelector('#main');
  const seg = segments();
  if (seg[0] === 'admin') {
    main.replaceChildren(node);
    return;
  }
  const wrap = el('div', 'page-content-wrap');
  if (seg[0] !== 'my-privacy' && !privacyMismatchCardDismissed()) {
    const warnings = myPrivacyWarnings();
    if (warnings.length) {
      wrap.append(privacyMismatchCard(warnings));
    }
  }
  const onOwnFamilyPage = onMyFamily(seg);
  const familyIds = new Set(familyNavPeople().map(fp => fp.id));
  const segPerson = seg[0] === 'people' && seg[1] ? personByKey(seg[1]) : undefined;
  const onOwnFamilyMemberPage = !!segPerson && familyIds.has(segPerson.id);
  if (!onOwnFamilyPage && !onOwnFamilyMemberPage && seg[0] !== 'my-privacy') {
    const stale = staleItems();
    if (stale.length) {
      wrap.append(familyInfoBanner(stale));
    }
  }
  wrap.append(node);
  main.replaceChildren(wrap);
  syncViewportHeight();
}

function navBadge(count) {
  return el('span', 'nav-badge', String(count));
}

function familyMemberRow(p, meId, activeId) {
  const a = link(personLink(p), 'nav-family-link');
  if (p.id === activeId) {
    a.className = 'nav-family-link active';
  }
  a.append(photoOrInitials(personPhotoUrl(p), p.name_show, 'nav-family-avatar'));
  a.append(el('span', 'nav-family-name', p.id === meId ? 'Me' : firstName(p.name_show)));
  const count = personTodoCount(p);
  if (count) {
    const badge = navBadge(count);
    badge.title = `${p.id === meId ? 'You have' : `${firstName(p.name_show)} has`} ${count} thing${count === 1 ? '' : 's'} to update`;
    a.append(badge);
  }
  return a;
}

function navScrollFade() {
  const nav = document.querySelector('#nav');
  const above = nav.scrollTop > 4;
  const below = nav.scrollTop + nav.clientHeight < nav.scrollHeight - 4;
  nav.style.setProperty('--fade-top', above ? '32px' : '0px');
  nav.style.setProperty('--fade-bottom', below ? '48px' : '0px');
}

let navSettled = false;
let navExpected = 0;

function placeNavScroll(top) {
  const nav = document.querySelector('#nav');
  nav.scrollTop = top;
  [...nav.querySelectorAll('a.active')].at(-1)?.scrollIntoView({block: 'nearest'});
  navExpected = nav.scrollTop;
  navScrollFade();
}

function familyRow(indicator) {
  const a = railRow({href: '/my-family', icon: 'my-family', label: 'My Family'}, activeSection() === 'my-family');
  if (indicator === 'alert') {
    const alert = el('span', 'nav-item-alert');
    alert.title = 'Some family info is missing or out of date';
    alert.append(svg('warn'));
    a.append(alert);
  } else if (indicator) {
    a.append(navBadge(indicator));
  }
  return a;
}

function familySection(people, activeId) {
  const todos = staleItems();
  const familyTodos = todos.filter(i => i.target === 'family').length;
  return {
    after: todos.length ? [navBadge(todos.length)] : [],
    forceOpen: Boolean(activeId),
    fill: body => {
      body.append(familyRow(familyTodos || (todos.length > familyTodos && 'alert')));
      for (const p of people) {
        body.append(familyMemberRow(p, viewer().id, activeId));
      }
    },
  };
}

function fillNav(nav) {
  const seg = activeSection();
  const rawSeg = segments();
  const familyPeople = familyNavPeople();
  const familyIds = new Set(familyPeople.map(p => p.id));
  const rawSegPerson = rawSeg[0] === 'people' && rawSeg[1] ? personByKey(rawSeg[1]) : undefined;
  const onFamilyMember = !!rawSegPerson && familyIds.has(rawSegPerson.id);
  drawRail(nav, {
    here: seg === 'people' && onFamilyMember ? '' : seg,
    lists: listItems(),
    family: familyPeople.length ? familySection(familyPeople, onFamilyMember ? rawSegPerson.id : null) : null,
    redraw: target => {
      if (target.id !== 'nav') {
        fillNav(target);
      }
      renderNav();
    },
  });
}

function listItems() {
  return {
    tags: tagKeys().map(key => ({key, id: tagOf(key).id, name: tagOf(key).name, slug: '', shared: sharedTag(key)})),
    lists: listKeys().map(key => ({...listOf(key), slug: listOf(key).groupSlug})),
  };
}

function toggleLists(section, open) {
  setListsOpen(section, open);
  renderNav();
}

function fillTabbar(bar) {
  const seg = activeSection();
  const familyPeople = familyNavPeople();
  for (const item of mobileNavSections) {
    if (item.path === 'my-family' && !familyPeople.length) {
      continue;
    }
    const a = link('/' + item.path, item.path === seg ? 'active' : '');
    a.append(svg(item.isListsTab ? 'list' : item.path), el('span', '', item.label));
    if (item.isListsTab) {
      a.addEventListener('click', e => {
        e.preventDefault();
        e.stopPropagation();
        setMobileListsMenu(mobileListsMenu.hidden);
      });
    }
    bar.append(a);
  }
}

export function renderNav() {
  const nav = document.querySelector('#nav');
  const top = navSettled ? nav.scrollTop : loadNavScroll();
  fillNav(nav);
  placeNavScroll(top);
  const bar = document.querySelector('#tabbar');
  bar.replaceChildren();
  fillTabbar(bar);
  if (!mobileListsMenu.hidden) {
    renderMobileListsMenu();
  }
}

onTagsChangeChrome(renderNav);

const mobileListsMenu = document.querySelector('#mobile-lists-menu');
const mobileListsOverlay = document.querySelector('#mobile-lists-overlay');

function setMobileListsMenu(open) {
  if (open) {
    renderMobileListsMenu();
  }
  mobileListsMenu.hidden = !open;
  mobileListsOverlay.hidden = !open;
}

function sharedTagIcon(icon) {
  const wrap = el('span', 'magic-tag-icon');
  wrap.append(icon);
  return wrap;
}

function magicTagIcon(key) {
  const mark = el('img', 'magic-tag-mark');
  mark.src = `/brand/apps/${listApp(key)}-outline.png`;
  mark.alt = '';
  return mark;
}

function renderMobileListsMenu() {
  const body = mobileListsMenu.querySelector('#mobile-lists-body');
  body.replaceChildren();
  const seg = activeSection();
  for (const item of listPageRows) {
    const a = link('/' + item.path, 'mobile-lists-item' + (item.path === seg ? ' active' : ''));
    a.append(svg(item.path), el('span', '', item.label));
    body.append(a);
  }
  const itemIcon = item => {
    if (!item.tag) {
      return magicTagIcon(item.key);
    }
    const icon = svg(item.shared ? 'families' : 'tag');
    icon.style.color = `hsl(${hue(item.name)}, 65%, 40%)`;
    return item.shared ? sharedTagIcon(icon) : icon;
  };
  for (const group of navSections(listItems())) {
    if (!sectionHeading(body, group, {className: 'mobile-lists-subheading', open: listsOpen(group), toggle: open => toggleLists(group, open)})) {
      continue;
    }
    for (const item of group.items) {
      const a = link(groupPathOf(item), 'mobile-lists-item' + (groupActive(item) ? ' active' : ''));
      a.append(itemIcon(item), listName(item));
      body.append(a);
    }
  }
}

const privacyRow = link('/my-privacy', 'user-menu-privacy', 'My Privacy');
const privacyAlert = el('span', 'user-menu-alert');
privacyAlert.hidden = true;
privacyRow.append(privacyAlert);

function me() {
  const person = viewer();
  const name = person ? person.name_show : model.email;
  return {email: model.email, name, initial: (name[0] || '').toUpperCase(), photoUrl: person && personPhotoUrl(person)};
}

function alerts() {
  const capital = s => s.charAt(0).toUpperCase() + s.slice(1);
  return {stale: staleItems().map(item => capital(item.label)), privacy: myPrivacyWarnings()};
}

export function renderUserChrome() {
  renderAccount();
  privacyRow.hidden = !familyOf(viewer());
  const mismatch = myPrivacyWarnings().length > 0;
  privacyAlert.hidden = !mismatch;
  if (mismatch && !privacyAlert.firstChild) {
    privacyAlert.append(svg('warn'));
  }
}

export function initChrome() {
  initShell({
    name: 'Helios Who?',
    me,
    alerts,
    fillNav,
    fillTabbar,
    search: {placeholder: 'Search by name, student, grade, or classroom…', results: true, own: true},
    menuRows: [privacyRow],
  });
  window.addEventListener('resize', updateMobileTitleInset);
  const nav = document.querySelector('#nav');
  nav.addEventListener('scroll', () => {
    if (nav.scrollTop !== navExpected) {
      navSettled = true;
    }
    navScrollFade();
  }, {passive: true});
  new ResizeObserver(() => {
    if (navSettled) {
      navScrollFade();
    } else {
      placeNavScroll(loadNavScroll());
    }
  }).observe(nav);
  window.addEventListener('pagehide', () => saveNavScroll(nav.scrollTop));
  mobileListsOverlay.addEventListener('click', () => setMobileListsMenu(false));
  document.addEventListener('click', e => {
    if (!searchResults().hidden && !e.target.closest('.topbar-search')) {
      searchResults().hidden = true;
    }
    for (const menu of document.querySelectorAll('.card-menu, .photo-menu')) {
      if (!menu.hidden && !menu.parentElement.contains(e.target)) {
        menu.hidden = true;
      }
    }
  });
  onSlash(() => searchInput().focus());
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      searchResults().hidden = true;
      for (const menu of document.querySelectorAll('.card-menu, .photo-menu')) {
        menu.hidden = true;
      }
      if (e.target === searchInput()) {
        e.target.blur();
      }
    } else if (e.key.toLowerCase() === 't' && e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
      const tagButton = document.querySelector('.tag-button');
      if (tagButton) {
        e.preventDefault();
        tagButton.click();
      }
    }
  });
}
