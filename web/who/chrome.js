import {state, byEmail} from './state.js';
import {el, svg, segments, hue, firstName, thumbUrl, trimMiddle} from './dom.js';
import {saveNavOpen, loadNavScroll, saveNavScroll} from './storage.js';
import {familyOf, myFamilyKey} from './families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl} from './people.js';
import {tagNames, listKeys, listLabel, listIcon, onTagsChange, onTagsChangeChrome} from './tags.js';
import {clampFilterPanel, closeFilterPanels} from './filters.js';
import {staleItems, familyInfoBanner, todoChecklist, familyNavPeople, personTodoCount} from './stale.js';
import {topbarSearchInput, topbarSearchResults} from './search.js';
import {privacyMismatchCardDismissed, myPrivacyWarnings, privacyMismatchCard, privacyMismatchText} from './pages/privacy.js';
import {load} from './app.js';
import {renderAvatars, onSlash, isEditableTarget, initAppSwitch, initUserMenu, initSpoof, hoverMenu, hoverClick, alertMenu, alertCard, markSuper} from '/toolbar.js';

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
  // Not a real page - opens the Lists popup instead of navigating (see the
  // isListsTab handling in renderNav below). Keeps the "email-list" path so
  // it still lands on Everyone as a fallback (JS disabled, middle-click,
  // "open in new tab") and so the tab lights up whenever that page is open.
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

// .mobile-title is centered by giving it equal left/right insets, so it has to
// be centered on the whole strip rather than just the space beside the back
// arrow, which is there on detail pages and not on the rest. Measured live.
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
  // My Privacy already shows its own, more detailed version of this per field, so the
  // summary card here would just repeat what's right below it on that page.
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

// The rail is laid out before its logo has loaded, so an offset restored
// then can be clamped to nothing; the nav's resize observer re-places it
// until the person scrolls the rail themselves (navSettled).
let navSettled = false;
let navExpected = 0;

function placeNavScroll(top) {
  const nav = document.querySelector('#nav');
  nav.scrollTop = top;
  // The last active link: on a list page the Directory item is lit as well,
  // and it sits at the top, so keeping it in view would scroll the list out.
  [...nav.querySelectorAll('a.active')].at(-1)?.scrollIntoView({block: 'nearest'});
  navExpected = nav.scrollTop;
  navScrollFade();
}

export function renderNav() {
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

  function buildNavInto(nav) {
    nav.replaceChildren();

    function sectionHeading(key, title, icon, indicator, forceOpen) {
      const open = state.navOpen[key] || forceOpen;
      const heading = el('div', 'nav-heading nav-heading-toggle' + (open ? ' open' : ''));
      const chevron = el('span', 'nav-chevron');
      chevron.append(svg('chevron'));
      const headingIcon = svg(icon);
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
        renderNav();
      });
      nav.append(heading);
      if (!open) {
        return null;
      }
      const body = el('div', 'nav-section-body');
      nav.append(body);
      return body;
    }

    const directoryBody = sectionHeading('directory', 'Directory', 'people', 0, false);
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
      const listLink = (href, iconName, name, active, auto) => {
        const a = el('a');
        a.href = href;
        a.title = name;
        if (seg === 'people' && active) {
          a.className = 'active';
        }
        const icon = svg(iconName);
        icon.classList.add('nav-icon-tag');
        // Plain white, not a per-name hashed color: the hash occasionally
        // landed near the sidebar's own dark teal, making that tag's icon
        // nearly invisible against the background it's sitting on.
        icon.style.color = '#fff';
        a.append(auto ? magicTagIcon(icon) : icon, el('span', '', trimMiddle(name, 40)));
        toolsBody.append(a);
      };
      for (const name of tagNames()) {
        listLink('/people?tag=' + encodeURIComponent(name), 'tag', name, params.get('tag') === name, false);
      }
      if (listKeys().length) {
        toolsBody.append(magicTagsHeading('nav-subheading'));
      }
      for (const key of listKeys()) {
        listLink('/people?list=' + encodeURIComponent(key), listIcon(key), listLabel(key), params.get('list') === key, true);
      }
    }
  }

  const nav = document.querySelector('#nav');
  const top = navSettled ? nav.scrollTop : loadNavScroll();
  buildNavInto(nav);
  placeNavScroll(top);
  const drawerNav = document.querySelector('#drawer-nav');
  if (drawerNav) {
    buildNavInto(drawerNav);
  }

  const tabs = document.querySelector('#mobile-tabs');
  tabs.replaceChildren();
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
    tabs.append(a);
  }
  if (!mobileListsMenu.hidden) {
    renderMobileListsMenu();
  }
}

onTagsChangeChrome(renderNav);

// Standalone iOS PWAs can settle 100dvh on a shorter value after an in-page
// route change than they reported on first load, leaving fixed bottom bars
// (mobile-tabs) short of the real screen edge - window.innerHeight matches
// what position:fixed elements are actually anchored to, so mirroring it into
// a custom property (see body/main's height: var(--vh100, 100dvh) in
// style.css) keeps them in sync regardless of dvh's own drift.
//
// In standalone mode specifically, innerHeight itself under-reports: it
// comes in ~60pt short of the true screen (no browser chrome exists there to
// explain the gap), and because body's height ends up as fixed elements'
// containing block, mobile-tabs' bottom:0 then stops short of the real edge
// too, leaving a blank strip below it. There's no dynamic toolbar to track in
// standalone mode, so screen.height - the full, stable device height - is
// the correct source there instead.
function syncViewportHeight() {
  const standalone = window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
  const height = standalone ? screen.height : window.innerHeight;
  document.documentElement.style.setProperty('--vh100', height + 'px');
}

// Wraps everything the render just built so it can act as the flexible
// sticky-footer spacer: on a short page it grows to push the art down to the true
// bottom of the viewport, and on a tall page it just yields to scrolling. Called
// both after the top-level render() dispatch and after any in-page tab switch that
// re-renders by calling its render*() function directly instead of going through
// render() - those bypass this otherwise, leaving the footer art stuck from
// whatever page loaded first (or missing it entirely).
export function finishRender() {
  syncViewportHeight();
  const main = document.querySelector('#main');
  const contentWrap = el('div', 'page-content-wrap');
  contentWrap.append(...main.childNodes);
  const footer = el('div', 'page-footer');
  footer.append(el('div', 'page-footer-art'));
  main.append(contentWrap, footer);
}

const userMenu = document.querySelector('#user-menu');

const drawer = document.querySelector('#drawer');
const drawerOverlay = document.querySelector('#drawer-overlay');

const mobileListsMenu = document.querySelector('#mobile-lists-menu');
const mobileListsOverlay = document.querySelector('#mobile-lists-overlay');

function setMobileListsMenu(open) {
  if (open) {
    renderMobileListsMenu();
  }
  mobileListsMenu.hidden = !open;
  mobileListsOverlay.hidden = !open;
}

// The smart lists (a party's guests, an activity's roster, a room parent's
// families) sit under the user's own tags in both the sidebar and the phone
// Lists sheet, set apart as "Magic Tags" so it's clear they're made and kept
// up by the app, not something the user typed in - the (i) explains on
// hover, since the name alone doesn't say where they come from or why they
// can't be edited.
const magicTagsTip = 'Magic Tags appear on their own, made from what the directory already knows about you: the guests of a party you are hosting, the roster of an activity you run, or the families a room parent looks after. They update themselves as those things change, so there is nothing to keep up. They cannot be edited.';

function magicTagsHeading(className) {
  const heading = el('div', className);
  const tip = el('span', 'magic-tags-info');
  tip.title = magicTagsTip;
  tip.append(svg('info'));
  heading.append(el('span', '', 'Magic Tags'), tip);
  return heading;
}

// The list's own kind icon with a little sparkle pinned to its corner, the
// same "made by the app" mark wherever it appears.
function magicTagIcon(icon) {
  const wrap = el('span', 'magic-tag-icon');
  const spark = svg('sparkles');
  spark.classList.add('magic-tag-spark');
  wrap.append(icon, spark);
  return wrap;
}

// The same Everyone/Invites/tag links as the sidebar's "Lists" section
// (buildNavInto above), just laid out as a mobile bottom sheet instead of a
// nav list, since there's no sidebar to hold them on a phone-sized screen.
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
  const listItem = (href, iconName, name, active, auto) => {
    const a = el('a', 'mobile-lists-item' + (seg === 'people' && active ? ' active' : ''));
    a.href = href;
    const icon = svg(iconName);
    icon.style.color = `hsl(${hue(name)}, 65%, 40%)`;
    a.append(auto ? magicTagIcon(icon) : icon, el('span', '', name));
    body.append(a);
  };
  for (const name of tagNames()) {
    listItem('/people?tag=' + encodeURIComponent(name), 'tag', name, params.get('tag') === name, false);
  }
  if (listKeys().length) {
    body.append(magicTagsHeading('mobile-lists-subheading'));
  }
  for (const key of listKeys()) {
    listItem('/people?list=' + encodeURIComponent(key), listIcon(key), listLabel(key), params.get('list') === key, true);
  }
}

function setDrawer(open) {
  drawer.hidden = !open;
  drawerOverlay.hidden = !open;
}

// The banners live in one fixed-position stack so they pile up in normal flow
// instead of each claiming top:0 and hiding one another.
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

// setSuperEdit is the one place that actually flips the switch - shared by the
// banner's "Turn off" link and the user menus' checkbox, both of which just
// need to call it and let the reload (which calls syncSuperEditCheckboxes and
// renderSuperEditBanner) bring every copy of the control back in sync.
async function setSuperEdit(enabled) {
  await fetch('/api/admin/super-edit', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({enabled}),
  });
  await load();
}

export function syncSuperEditCheckboxes() {
  markSuper(state.model.superEdit);
  for (const box of document.querySelectorAll('.super-edit-checkbox')) {
    box.checked = Boolean(state.model.superEdit);
  }
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
    await setSuperEdit(false);
  });
  banner.append(link);
  topBanners().append(banner);
  updateBannerOffset();
}

// Fills the user chrome (both avatars, the profile links, and the admin-only
// menu items) from the model's signed-in identity.
export function renderUserChrome() {
  const user = state.model.user;
  // The same hero photo the profile page shows (own photo, else family's), with
  // the initial standing in when there isn't one - the same avatar every
  // Heliosian app's toolbar shows.
  const person = personByKey(user.email);
  const photoUrl = person && personPhotoUrl(person);
  renderAvatars({photoUrl: photoUrl && thumbUrl(photoUrl), initial: user.initial});
  for (const line of document.querySelectorAll('.user-menu-email')) {
    line.textContent = user.email;
  }
  for (const link of document.querySelectorAll('.user-menu-profile')) {
    link.href = '/people/' + encodeURIComponent(user.slug);
  }
  for (const item of document.querySelectorAll('.user-menu-admin')) {
    item.hidden = !user.isAdmin;
  }
}

export function renderPrivacyMenuAlert() {
  const hasMismatch = myPrivacyWarnings().length > 0;
  const staleCount = staleItems().length;
  const hasStale = staleCount > 0;
  // My Privacy only has anything to show for someone in a family (it's entirely
  // about the family's address/phone visibility) - staff with no family record
  // would just land on an empty page, so hide the link for them instead.
  const hasFamily = !!familyOf(byEmail[document.body.dataset.userEmail]);
  for (const link of document.querySelectorAll('.user-menu-privacy')) {
    link.hidden = !hasFamily;
  }

  for (const badge of document.querySelectorAll('.user-menu-alert')) {
    badge.hidden = !hasMismatch;
    if (hasMismatch && !badge.firstChild) {
      badge.append(svg('alert'));
    }
  }

  // One loop over every .stale-alert/.privacy-alert in the page, however many
  // bars carry them.
  for (const staleButton of document.querySelectorAll('.stale-alert')) {
    staleButton.hidden = !hasStale;
    staleButton.title = `${staleCount} thing${staleCount === 1 ? '' : 's'} to update for the new year`;
  }
  for (const count of document.querySelectorAll('.stale-count')) {
    count.textContent = String(staleCount);
  }
  for (const privacyButton of document.querySelectorAll('.privacy-alert')) {
    privacyButton.hidden = !hasMismatch;
  }
}

// Wires the static chrome (menus, drawer, search, stale badge, global
// click/keyboard handlers) exactly once, from app.js, so no module does DOM
// work just by being imported.
export function initChrome() {
  initAppSwitch();
  window.addEventListener('resize', updateMobileTitleInset);
  window.addEventListener('resize', syncViewportHeight);
  window.addEventListener('orientationchange', syncViewportHeight);
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

  // The topbar only has room for the avatar (no name label), so - unlike the old
  // sidebar row, which let a name click open the menu and an avatar click jump
  // straight to the profile - the avatar's only job now is opening the menu, whose
  // first item is "View Profile".
  initUserMenu();
  initSpoof();

  // The stale-count badge (desktop and mobile both) used to link straight to
  // /my-family; now it opens a dropdown built from the same todoChecklist used
  // on My Family/a person's own page, so what to fix and the link to fix it are
  // both right there without leaving the current page. It opens on hover the
  // way the bar's other menus do, a mouse click leaving it open and a tap
  // toggling it.
  for (const staleButton of document.querySelectorAll('.stale-alert')) {
    const wrap = staleButton.closest('.stale-wrap');
    const panel = wrap.querySelector('.stale-menu');
    const open = () => {
      for (const p of document.querySelectorAll('.stale-menu')) {
        p.hidden = true;
      }
      panel.replaceChildren(todoChecklist(staleItems()));
      panel.hidden = false;
      clampFilterPanel(wrap, panel);
    };
    const close = () => {
      panel.hidden = true;
    };
    hoverMenu(staleButton, panel, open, close);
    staleButton.addEventListener('click', e => {
      e.stopPropagation();
      if (panel.hidden) {
        open();
      } else if (!hoverClick(e)) {
        close();
      }
    });
  }

  // The privacy triangle stays a link to My Privacy, and on hover says what
  // the mismatch is - the sentence the dismissable banner uses - over the
  // link, so the page need not be left to learn which detail it is.
  for (const badge of document.querySelectorAll('.privacy-alert')) {
    alertMenu(badge, () => alertCard('Privacy Settings Mismatch', privacyMismatchText(myPrivacyWarnings()), 'See Details', '/my-privacy'));
  }

  mobileListsOverlay.addEventListener('click', () => setMobileListsMenu(false));

  // Both the desktop and mobile user menus carry their own copy of this
  // checkbox (shown to admins only) - a quicker way to flip Super Admin Mode
  // than the full admin page, which still has its own toggle too. Wired once here
  // since the checkboxes are static; syncSuperEditCheckboxes (called from load())
  // keeps their checked state true to the model after every reload, including one
  // triggered by a different tab or the admin page.
  for (const box of document.querySelectorAll('.super-edit-checkbox')) {
    box.addEventListener('change', () => setSuperEdit(box.checked));
  }

  document.querySelector('#mobile-menu-btn').addEventListener('click', () => setDrawer(true));
  document.querySelector('#drawer-close').addEventListener('click', () => setDrawer(false));
  drawerOverlay.addEventListener('click', () => setDrawer(false));

  document.addEventListener('click', e => {
    userMenu.hidden = true;
    for (const p of document.querySelectorAll('.stale-menu')) {
      p.hidden = true;
    }
    if (!topbarSearchResults.hidden && !e.target.closest('.topbar-search')) {
      topbarSearchResults.hidden = true;
    }
    for (const menu of document.querySelectorAll('.more-menu, .card-menu, .photo-menu')) {
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

  onSlash(() => topbarSearchInput.focus());

  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      userMenu.hidden = true;
      for (const p of document.querySelectorAll('.stale-menu')) {
        p.hidden = true;
      }
      setDrawer(false);
      topbarSearchResults.hidden = true;
      closeFilterPanels();
      for (const menu of document.querySelectorAll('.more-menu, .card-menu, .photo-menu')) {
        menu.hidden = true;
      }
      if (e.target === topbarSearchInput) {
        e.target.blur();
      }
    } else if (e.key.toLowerCase() === 't' && e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
      // Same single "the tag button" a person's detail page shows (see
      // breadcrumbs' tagEmail param) - clicking it does the actual work, so the
      // shortcut just replays that click rather than duplicating its logic.
      const tagButton = document.querySelector('.tag-button');
      if (tagButton) {
        e.preventDefault();
        tagButton.click();
      }
    }
  });
}
