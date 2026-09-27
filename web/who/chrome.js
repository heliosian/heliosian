import {state, byEmail} from './state.js';
import {el, svg, segments, hue, firstName, trimMiddle} from './dom.js';
import {saveNavOpen, loadNavScroll, saveNavScroll} from './storage.js';
import {familyOf, myFamilyKey} from './families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl} from './people.js';
import {tagNames, listKeys, listLabel, listApp, sharedKeys, sharedOf, managersOf, tagHref, onTagsChange, onTagsChangeChrome} from './tags.js';
import {closeFilterPanels} from './filters.js';
import {staleItems, familyInfoBanner, familyNavPeople, personTodoCount} from './stale.js';
import {searchResults} from './search.js';
import {privacyMismatchCardDismissed, myPrivacyWarnings, privacyMismatchCard} from './pages/privacy.js';
import {load} from './app.js';
import {onSlash, isEditableTarget} from '/toolbar.js';
import {initShell, renderAccount, searchInput, syncViewportHeight} from '/shell.js';
import {setSuperEdit} from '/superedit.js';

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

export function resetMain(...children) {
  const main = document.querySelector('#main');
  main.replaceChildren();
  onTagsChange(() => {});
  const seg = segments();
  if (seg[0] !== 'my-privacy' && !privacyMismatchCardDismissed()) {
    const warnings = myPrivacyWarnings();
    if (warnings.length) {
      main.append(privacyMismatchCard(warnings));
    }
  }
  const onOwnFamilyPage = seg[0] === 'families' && seg[1] === myFamilyKey();
  const familyEmails = new Set(familyNavPeople().map(fp => fp.email));
  const segPerson = seg[0] === 'people' && seg[1] ? personByKey(seg[1]) : undefined;
  const onOwnFamilyMemberPage = !!segPerson && familyEmails.has(segPerson.email);
  if (!onOwnFamilyPage && !onOwnFamilyMemberPage && seg[0] !== 'my-privacy') {
    const stale = staleItems();
    if (stale.length) {
      main.append(familyInfoBanner(stale));
    }
  }
  main.append(...children);
  return main;
}

function navBadge(count) {
  return el('span', 'nav-badge', String(count));
}

function familyMemberRow(p, meEmail, activeEmail) {
  const a = el('a', 'nav-family-link');
  a.href = personLink(p);
  if (p.email === activeEmail) {
    a.className = 'nav-family-link active';
  }
  a.append(photoOrInitials(personPhotoUrl(p), p.fullName, 'nav-family-avatar'));
  a.append(el('span', 'nav-family-name', p.email === meEmail ? 'Me' : firstName(p.fullName)));
  const count = personTodoCount(p);
  if (count) {
    const badge = navBadge(count);
    badge.title = `${p.email === meEmail ? 'You have' : `${firstName(p.fullName)} has`} ${count} thing${count === 1 ? '' : 's'} to update`;
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

// The rail is laid out before its logo has loaded, so an offset restored then can
// be clamped to nothing; the resize observer re-places it until the person scrolls.
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
  const me = byEmail[document.body.dataset.userEmail];
  const familyPeople = familyNavPeople();
  const familyEmails = new Set(familyPeople.map(p => p.email));
  const rawSegPerson = rawSeg[0] === 'people' && rawSeg[1] ? personByKey(rawSeg[1]) : undefined;
  const onFamilyMember = !!rawSegPerson && familyEmails.has(rawSegPerson.email);

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
      alert.append(svg('alert'));
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
    chevron.append(svg('chevron'));
    const headingIcon = icon === 'app' ? el('span', 'app-symbol') : svg(icon);
    headingIcon.classList.add('nav-heading-icon-' + icon);
    heading.append(chevron, headingIcon, el('span', 'nav-heading-title', title));
    if (indicator === 'alert') {
      const alert = el('span', 'nav-heading-alert');
      alert.title = 'Some family info is missing or out of date';
      alert.append(svg('alert'));
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
        familyBody.append(familyMemberRow(p, me.email, onFamilyMember ? rawSegPerson.email : null));
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
    for (const name of own) {
      listLink('/people?tag=' + encodeURIComponent(name), whiteIcon('tag'), name, params.get('tag') === name);
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

export function finishRender() {
  syncViewportHeight();
  const main = document.querySelector('#main');
  const contentWrap = el('div', 'page-content-wrap');
  contentWrap.append(...main.childNodes);
  main.append(contentWrap);
}

const mobileListsMenu = document.querySelector('#mobile-lists-menu');
const mobileListsOverlay = document.querySelector('#mobile-lists-overlay');

function setMobileListsMenu(open) {
  if (open) {
    renderMobileListsMenu();
  }
  mobileListsMenu.hidden = !open;
  mobileListsOverlay.hidden = !open;
}

const magicTagsTip = 'Automagically created based on events, volunteering and the groups you manage';

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
  const own = tagNames().filter(name => !managersOf(name).length);
  const shared = [
    ...tagNames().filter(name => managersOf(name).length).map(name => ({
      name,
      mine: true,
      href: '/people?tag=' + encodeURIComponent(name),
      title: `${name} - your tag, shared with others`,
      active: params => params.get('tag') === name,
    })),
    ...sharedKeys().map(key => {
      const t = sharedOf(key);
      return {
        name: t.name,
        mine: false,
        href: tagHref(key),
        title: `${t.name} - ${t.ownerName}'s tag, shared with you`,
        active: params => params.get('shared') === `${t.owner}:${t.name}`,
      };
    }),
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
  for (const name of own) {
    const icon = svg('tag');
    icon.style.color = `hsl(${hue(name)}, 65%, 40%)`;
    listItem('/people?tag=' + encodeURIComponent(name), icon, name, params.get('tag') === name);
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

function topBanners() {
  let stack = document.querySelector('#top-banners');
  if (!stack) {
    stack = el('div', '');
    stack.id = 'top-banners';
    document.body.append(stack);
  }
  return stack;
}

function updateBannerOffset() {
  const stack = document.querySelector('#top-banners');
  document.documentElement.style.setProperty('--banner-h', stack ? stack.offsetHeight + 'px' : '0px');
}

export function renderSuperEditBanner() {
  let banner = document.querySelector('.super-edit-banner');
  if (!state.model.superEdit) {
    if (banner) {
      banner.remove();
      updateBannerOffset();
    }
    return;
  }
  if (banner) {
    return;
  }
  banner = el('div', 'super-edit-banner');
  banner.append(el('span', '', 'Super Admin Mode is on — you can edit anyone’s info.'));
  const link = el('a', '', 'Turn off');
  link.href = '#';
  link.addEventListener('click', async e => {
    e.preventDefault();
    setSuperEdit(false);
    await load();
  });
  banner.append(link);
  topBanners().append(banner);
  updateBannerOffset();
}

const privacyRow = el('a', 'user-menu-privacy', 'My Privacy');
privacyRow.href = '/my-privacy';
const privacyAlert = el('span', 'user-menu-alert');
privacyAlert.hidden = true;
privacyRow.append(privacyAlert);

function me() {
  const user = state.model.user;
  const person = personByKey(user.email);
  return {...user, photoUrl: person && personPhotoUrl(person)};
}

function alerts() {
  const capital = s => s.charAt(0).toUpperCase() + s.slice(1);
  return {stale: staleItems().map(item => capital(item.label)), privacy: myPrivacyWarnings()};
}

export function renderUserChrome() {
  renderAccount();
  privacyRow.hidden = !familyOf(byEmail[document.body.dataset.userEmail]);
  const mismatch = myPrivacyWarnings().length > 0;
  privacyAlert.hidden = !mismatch;
  if (mismatch && !privacyAlert.firstChild) {
    privacyAlert.append(svg('alert'));
  }
}

export function initChrome() {
  initShell({
    name: 'Helios Who?',
    me,
    alerts,
    isSystemAdmin: () => state.model.user.isAdmin,
    onSuper: load,
    fillNav,
    fillTabbar,
    search: {placeholder: 'Search by name, student, grade, or classroom…', results: true, own: true},
    menuRows: [privacyRow],
  });
  window.addEventListener('resize', updateMobileTitleInset);
  window.addEventListener('resize', updateBannerOffset);
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
    for (const panel of document.querySelectorAll('.filter-panel')) {
      if (!panel.hidden && !panel.parentElement.contains(e.target)) {
        panel.hidden = true;
        panel.parentElement.querySelector('.filter-button').classList.remove('open');
      }
    }
  });
  onSlash(() => searchInput().focus());
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      searchResults().hidden = true;
      closeFilterPanels();
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
