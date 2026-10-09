import {state, model, tagKey, listKey, viewer, familyOf, resetPageState} from './state.js';
import {segments, hue, firstName, trimMiddle} from './dom.js';
import {el, svg, link} from '/elements.js';
import {saveNavOpen, loadNavScroll, saveNavScroll} from './storage.js';
import {myFamily} from './families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl} from './people.js';
import {tagKeys, tagLabel, listApp, listOf, listSections, sharedTag, tagHref, onTagsChange, onTagsChangeChrome} from './tags.js';
import {staleItems, familyInfoBanner, familyNavPeople, personTodoCount} from './stale.js';
import {searchResults} from './search.js';
import {privacyMismatchCardDismissed, myPrivacyWarnings, privacyMismatchCard} from './pages/privacy.js';
import {initShell, renderAccount, searchInput, syncViewportHeight, onSlash, isEditableTarget} from '/shell.js';

const primaryNavItems = [
  {path: 'people', label: 'Directory'},
  {path: 'classrooms', label: 'Gradebands'},
  {path: 'staff', label: 'Staff'},
];

const toolsNavItems = [
  {path: 'email-list', label: 'Everyone'},
  {path: 'greenvelope', label: 'Invites'},
];

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

function fillNav(nav) {
  const seg = activeSection();
  const rawSeg = segments();
  const me = viewer();
  const familyPeople = familyNavPeople();
  const familyIds = new Set(familyPeople.map(p => p.id));
  const rawSegPerson = rawSeg[0] === 'people' && rawSeg[1] ? personByKey(rawSeg[1]) : undefined;
  const onFamilyMember = !!rawSegPerson && familyIds.has(rawSegPerson.id);

  function renderItem(container, item, indicator) {
    const a = link('/' + item.path);
    if (item.path === seg && !(item.path === 'people' && onFamilyMember)) {
      a.className = 'active';
    }
    const icon = svg(item.path);
    icon.classList.add('nav-icon-' + item.path);
    a.append(icon, el('span', '', item.label));
    if (indicator === 'alert') {
      const alert = el('span', 'nav-item-alert');
      alert.title = 'Some family info is missing or out of date';
      alert.append(svg('warn'));
      a.append(alert);
    } else if (indicator) {
      a.append(navBadge(indicator));
    }
    container.append(a);
  }

  function sectionHeading(key, title, icon, indicator, forceOpen) {
    const open = state.navOpen[key] || forceOpen;
    const heading = el('div', 'nav-heading nav-heading-toggle' + (open ? ' open' : ''));
    const chevron = el('span', 'nav-chevron');
    chevron.append(svg('chevron-down'));
    const headingIcon = icon === 'app' ? el('span', 'app-symbol') : svg(icon);
    headingIcon.classList.add('nav-heading-icon-' + icon);
    heading.append(chevron, headingIcon, el('span', 'nav-heading-title', title));
    if (indicator === 'alert') {
      const alert = el('span', 'nav-heading-alert');
      alert.title = 'Some family info is missing or out of date';
      alert.append(svg('warn'));
      heading.append(alert);
    } else if (indicator) {
      heading.append(navBadge(indicator));
    }
    heading.addEventListener('click', () => {
      state.navOpen[key] = !state.navOpen[key];
      saveNavOpen(state.navOpen);
      if (nav.id !== 'nav') {
        fillNav(nav);
      }
      renderNav();
    });
    nav.append(heading);
    if (!open) {
      return null;
    }
    const body = el('div', 'nav-section-body');
    nav.append(body);
    queueMicrotask(() => heading.classList.toggle('active', Boolean(body.querySelector('a.active'))));
    return body;
  }

  nav.replaceChildren();
  const directoryBody = sectionHeading('directory', 'Directory', 'app', 0, false);
  if (directoryBody) {
    for (const item of primaryNavItems) {
      renderItem(directoryBody, item);
    }
  }

  if (familyPeople.length) {
    const todos = staleItems();
    const familyTodos = todos.filter(i => i.target === 'family').length;
    const familyBody = sectionHeading('family', 'My Family', 'heart', todos.length, onFamilyMember);
    if (familyBody) {
      renderItem(familyBody, {path: 'my-family', label: 'My Family'}, familyTodos || (todos.length > familyTodos && 'alert'));
      for (const p of familyPeople) {
        familyBody.append(familyMemberRow(p, me.id, onFamilyMember ? rawSegPerson.id : null));
      }
    }
  }

  const toolsBody = sectionHeading('tools', 'Lists', 'list', 0, false);
  if (toolsBody) {
    for (const item of toolsNavItems) {
      renderItem(toolsBody, item);
    }
    for (const group of listGroups()) {
      if (!listsHeading(toolsBody, group, 'nav-subheading')) {
        continue;
      }
      for (const item of group.items) {
        const a = link(item.href);
        a.title = item.title;
        if (seg === 'people' && item.active) {
          a.className = 'active';
        }
        a.append(listName(item));
        toolsBody.append(a);
      }
    }
  }
}

function listGroups() {
  const params = new URLSearchParams(location.search);
  const {own, shared} = groupedTags();
  const {running, upcoming, joined, managing} = listSections();
  const tagItems = [...own, ...shared].map(entry => ({href: entry.href, shared: entry.shared, name: entry.name, title: entry.title, active: entry.active(params), run: false, mail: false, start: ''}));
  const listItems = (keys, dated) => keys.map(key => ({
    href: tagHref(key),
    list: key,
    name: tagLabel(key),
    title: tagLabel(key),
    active: listActive(params, key),
    run: dated && listOf(key).run,
    mail: listOf(key).mail,
    start: dated ? listOf(key).start : '',
  }));
  const byName = (a, b) => a.name.localeCompare(b.name);
  return [
    {key: 'running', title: 'Running', open: true, items: [...tagItems, ...listItems(running)].sort(byName)},
    {key: 'upcoming', title: 'Coming Up', open: true, items: listItems(upcoming, true)},
    {key: 'joined', title: 'Joined', open: true, items: listItems(joined)},
    {key: 'managing', title: 'Managing', open: false, items: listItems(managing)},
  ].filter(group => group.items.length);
}

const dayFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});

function listName(item) {
  const label = el('span', 'nav-list-label');
  label.append(el('span', '', trimMiddle(item.name, 40)));
  if (item.start) {
    label.append(el('span', 'nav-list-date', dayFormat.format(new Date(item.start.slice(0, 10) + 'T00:00'))));
  }
  const marks = el('span', 'nav-list-marks');
  if (item.mail) {
    const mark = svg('mail');
    mark.classList.add('nav-list-mail');
    marks.append(mark);
  }
  if (item.run) {
    marks.append(el('span', 'nav-list-run', '★'));
  }
  const row = el('span', 'nav-list-row');
  row.append(label, marks);
  return row;
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

function listsHeading(container, group, className) {
  const key = 'lists-' + group.key;
  const open = state.navOpen[key] ?? group.open;
  const heading = el('div', className + ' nav-subheading-toggle' + (open ? ' open' : ''));
  const chevron = el('span', 'nav-chevron');
  chevron.append(svg('chevron-down'));
  heading.append(el('span', '', group.title));
  if (!open) {
    heading.append(el('span', 'nav-subheading-count', String(group.items.length)));
  }
  heading.append(chevron);
  heading.addEventListener('click', () => {
    state.navOpen[key] = !open;
    saveNavOpen(state.navOpen);
    renderNav();
  });
  container.append(heading);
  return open;
}

function listActive(params, key) {
  return params.has('list') && listKey(params.get('list')) === key;
}

function groupedTags() {
  const entry = (key, title, shared) => ({
    name: tagLabel(key),
    href: tagHref(key),
    title,
    shared,
    active: params => params.has('tag') && tagKey(params.get('tag')) === key,
  });
  const own = tagKeys().filter(key => !sharedTag(key)).map(key => entry(key, tagLabel(key), false));
  const shared = tagKeys().filter(sharedTag).map(key => entry(key, `${tagLabel(key)} - shared with others`, true));
  return {own, shared};
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
  for (const item of toolsNavItems) {
    const a = link('/' + item.path, 'mobile-lists-item' + (item.path === seg ? ' active' : ''));
    a.append(svg(item.path), el('span', '', item.label));
    body.append(a);
  }
  const itemIcon = item => {
    if (item.list) {
      return magicTagIcon(item.list);
    }
    const icon = svg(item.shared ? 'families' : 'tag');
    icon.style.color = `hsl(${hue(item.name)}, 65%, 40%)`;
    return item.shared ? sharedTagIcon(icon) : icon;
  };
  for (const group of listGroups()) {
    if (!listsHeading(body, group, 'mobile-lists-subheading')) {
      continue;
    }
    for (const item of group.items) {
      const a = link(item.href, 'mobile-lists-item' + (seg === 'people' && item.active ? ' active' : ''));
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
