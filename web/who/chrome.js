import {state, tagKey, viewer} from './state.js';
import {segments, hue, firstName, trimMiddle} from './dom.js';
import {el, svg} from '/elements.js';
import {saveNavOpen, loadNavScroll, saveNavScroll} from './storage.js';
import {familyOf, myFamilyKey} from './families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl} from './people.js';
import {tagKeys, tagLabel, listKeys, listLabel, listApp, sharedKeys, sharedOf, managersOf, tagHref, onTagsChange, onTagsChangeChrome} from './tags.js';
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

function activeSection() {
  const seg = segments();
  if (seg[0] === 'families') {
    return seg[1] === myFamilyKey() ? 'my-family' : 'people';
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

export function preparePage(title) {
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
  const onOwnFamilyPage = seg[0] === 'families' && seg[1] === myFamilyKey();
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
  const a = el('a', 'nav-family-link');
  a.href = personLink(p);
  if (p.id === activeId) {
    a.className = 'nav-family-link active';
  }
  a.append(photoOrInitials(personPhotoUrl(p), p.fullName, 'nav-family-avatar'));
  a.append(el('span', 'nav-family-name', p.id === meId ? 'Me' : firstName(p.fullName)));
  const count = personTodoCount(p);
  if (count) {
    const badge = navBadge(count);
    badge.title = `${p.id === meId ? 'You have' : `${firstName(p.fullName)} has`} ${count} thing${count === 1 ? '' : 's'} to update`;
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
    const a = el('a');
    a.href = '/' + item.path;
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
    const params = new URLSearchParams(location.search);
    const listLink = (href, icon, name, active, title) => {
      const a = el('a');
      a.href = href;
      a.title = title || name;
      if (seg === 'people' && active) {
        a.className = 'active';
      }
      a.append(icon, el('span', '', trimMiddle(name, 40)));
      toolsBody.append(a);
    };
    const whiteIcon = name => {
      const icon = svg(name);
      icon.classList.add('nav-icon-tag');
      icon.style.color = '#fff';
      return icon;
    };
    const {own, shared} = groupedTags();
    for (const entry of own) {
      listLink(entry.href, whiteIcon('tag'), entry.name, entry.active(params), entry.title);
    }
    if (shared.length) {
      toolsBody.append(sharedTagsHeading('nav-subheading'));
    }
    for (const entry of shared) {
      listLink(entry.href, sharedTagIcon(whiteIcon('families'), entry.mine), entry.name, entry.active(params), entry.title);
    }
    if (listKeys().length) {
      toolsBody.append(magicTagsHeading('nav-subheading'));
    }
    for (const key of listKeys()) {
      listLink('/people?list=' + encodeURIComponent(key), magicTagIcon(key), listLabel(key), params.get('list') === key);
    }
  }
}

function fillTabbar(bar) {
  const seg = activeSection();
  const familyPeople = familyNavPeople();
  for (const item of mobileNavSections) {
    if (item.path === 'my-family' && !familyPeople.length) {
      continue;
    }
    const a = el('a', item.path === seg ? 'active' : '');
    a.href = '/' + item.path;
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

const magicTagsTip = 'Automagically created based on events, volunteering and the email lists you manage';

function hintIcon(text) {
  const tip = el('span', 'magic-tags-info');
  tip.append(svg('info'));
  let box = null;
  tip.addEventListener('mouseenter', () => {
    box = el('div', 'hint-box', text);
    document.body.append(box);
    const at = tip.getBoundingClientRect();
    box.style.left = `${at.right + 8}px`;
    box.style.top = `${at.top + at.height / 2 - box.offsetHeight / 2}px`;
  });
  tip.addEventListener('mouseleave', () => {
    box?.remove();
    box = null;
  });
  return tip;
}

function magicTagsHeading(className) {
  const heading = el('div', className);
  heading.append(el('span', '', 'Magic Tags'), hintIcon(magicTagsTip));
  return heading;
}

function groupedTags() {
  const entry = (key, mine, title) => ({
    name: tagLabel(key),
    mine,
    href: tagHref(key),
    title,
    active: params => params.has('tag') && tagKey(params.get('tag')) === key,
  });
  const own = tagKeys().filter(key => !managersOf(key).length).map(key => entry(key, true, tagLabel(key)));
  const shared = [
    ...tagKeys().filter(key => managersOf(key).length).map(key => entry(key, true, `${tagLabel(key)} - your tag, shared with others`)),
    ...sharedKeys().map(key => entry(key, false, `${tagLabel(key)} - ${sharedOf(key).ownerName}'s tag, shared with you`)),
  ].sort((a, b) => a.name.localeCompare(b.name));
  return {own, shared};
}

function sharedTagIcon(icon, mine) {
  if (!mine) {
    return icon;
  }
  const wrap = el('span', 'magic-tag-icon');
  const star = svg('star');
  star.classList.add('owner-star');
  wrap.append(icon, star);
  return wrap;
}

const sharedTagsTip = 'Tags more than one person manages. A star marks yours.';

function sharedTagsHeading(className) {
  const heading = el('div', className);
  heading.append(el('span', '', 'Shared Tags'), hintIcon(sharedTagsTip));
  return heading;
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
  const params = new URLSearchParams(location.search);
  for (const item of toolsNavItems) {
    const a = el('a', 'mobile-lists-item' + (item.path === seg ? ' active' : ''));
    a.href = '/' + item.path;
    a.append(svg(item.path), el('span', '', item.label));
    body.append(a);
  }
  const listItem = (href, icon, name, active) => {
    const a = el('a', 'mobile-lists-item' + (seg === 'people' && active ? ' active' : ''));
    a.href = href;
    a.append(icon, el('span', '', name));
    body.append(a);
  };
  const {own, shared} = groupedTags();
  for (const entry of own) {
    const icon = svg('tag');
    icon.style.color = `hsl(${hue(entry.name)}, 65%, 40%)`;
    listItem(entry.href, icon, entry.name, entry.active(params));
  }
  if (shared.length) {
    body.append(sharedTagsHeading('mobile-lists-subheading'));
  }
  for (const entry of shared) {
    const icon = svg('families');
    icon.style.color = `hsl(${hue(entry.name)}, 65%, 40%)`;
    listItem(entry.href, sharedTagIcon(icon, entry.mine), entry.name, entry.active(params));
  }
  if (listKeys().length) {
    body.append(magicTagsHeading('mobile-lists-subheading'));
  }
  for (const key of listKeys()) {
    listItem('/people?list=' + encodeURIComponent(key), magicTagIcon(key), listLabel(key), params.get('list') === key);
  }
}

const privacyRow = el('a', 'user-menu-privacy', 'My Privacy');
privacyRow.href = '/my-privacy';
const privacyAlert = el('span', 'user-menu-alert');
privacyAlert.hidden = true;
privacyRow.append(privacyAlert);

function me() {
  const person = viewer();
  return {...state.model.user, photoUrl: person && personPhotoUrl(person)};
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
