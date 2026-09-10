import {state, byEmail} from './state.js';
import {el, svg, segments, isMobile, hue, firstName} from './dom.js';
import {saveNavOpen} from './storage.js';
import {familyOf, myFamilyKey} from './families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl} from './people.js';
import {tagNames} from './tags.js';
import {clampFilterPanel, closeFilterPanels} from './filters.js';
import {staleItems, familyInfoBanner, todoChecklist, familyNavPeople, personTodoCount} from './stale.js';
import {topbarSearchInput, topbarSearchResults, mobileSearchInput, setMobileSearch} from './search.js';
import {privacyMismatchCardDismissed, myPrivacyWarnings, privacyMismatchCard} from './pages/privacy.js';
import {load} from './app.js';

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
  const menuBtn = document.querySelector('#mobile-menu-btn');
  back.hidden = !backHref;
  menuBtn.hidden = Boolean(backHref);
  if (backHref) {
    back.href = backHref;
  }
  updateMobileTitleInset();
}

// .mobile-title is centered by giving it equal left/right insets, so it has to be
// centered on the whole bar rather than just the space between whichever icons
// happen to be showing - a fixed inset sized for the busiest icon cluster (search +
// stale alert + privacy alert + avatar) left the title visibly off-center on every
// page showing fewer icons than that, including the plain back-button pages. Measured
// live because which side is wider varies with the route (back vs. menu button) and
// with per-user alert state (stale info, privacy mismatch).
function updateMobileTitleInset() {
  const bar = document.querySelector('.mobile-top');
  const leftEl = bar.querySelector('#mobile-back:not([hidden]), #mobile-menu-btn:not([hidden])');
  const searchBtn = document.querySelector('#mobile-search-btn');
  if (!bar || !leftEl || !searchBtn) {
    return;
  }
  const barRect = bar.getBoundingClientRect();
  if (!barRect.width) {
    return;
  }
  const leftWidth = leftEl.getBoundingClientRect().right - barRect.left;
  const rightWidth = barRect.right - searchBtn.getBoundingClientRect().left;
  document.documentElement.style.setProperty('--mobile-title-inset', Math.max(leftWidth, rightWidth) + 'px');
}

export function resetMain(...children) {
  const main = document.querySelector('#main');
  main.replaceChildren();
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
      const currentTag = new URLSearchParams(location.search).get('tag');
      for (const name of tagNames()) {
        const a = el('a');
        a.href = '/people?tag=' + encodeURIComponent(name);
        if (seg === 'people' && currentTag === name) {
          a.className = 'active';
        }
        const icon = svg('tag');
        icon.classList.add('nav-icon-tag');
        // Plain white, not a per-name hashed color: the hash occasionally
        // landed near the sidebar's own dark teal, making that tag's icon
        // nearly invisible against the background it's sitting on.
        icon.style.color = '#fff';
        a.append(icon, el('span', '', name));
        toolsBody.append(a);
      }
    }
  }

  buildNavInto(document.querySelector('#nav'));
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
  main.append(contentWrap, el('div', 'page-footer-art'));
}

const userMenu = document.querySelector('#user-menu');
const mobileUserMenu = document.querySelector('#mobile-user-menu');

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

// The same Everyone/Invites/tag links as the sidebar's "Lists" section
// (buildNavInto above), just laid out as a mobile bottom sheet instead of a
// nav list, since there's no sidebar to hold them on a phone-sized screen.
function renderMobileListsMenu() {
  const body = mobileListsMenu.querySelector('#mobile-lists-body');
  body.replaceChildren();
  const seg = activeSection();
  const currentTag = new URLSearchParams(location.search).get('tag');
  for (const item of toolsNavItems) {
    const a = el('a', 'mobile-lists-item' + (item.path === seg ? ' active' : ''));
    a.href = '/' + item.path;
    a.append(svg(item.path), el('span', '', item.label));
    body.append(a);
  }
  for (const name of tagNames()) {
    const a = el('a', 'mobile-lists-item' + (seg === 'people' && currentTag === name ? ' active' : ''));
    a.href = '/people?tag=' + encodeURIComponent(name);
    const icon = svg('tag');
    icon.style.color = `hsl(${hue(name)}, 65%, 40%)`;
    a.append(icon, el('span', '', name));
    body.append(a);
  }
}

function setDrawer(open) {
  drawer.hidden = !open;
  drawerOverlay.hidden = !open;
}

// A bare "/" (no modifiers, and not already typing somewhere) jumps straight to
// search, the way GitHub/Slack do - skipped while any text field, including the
// search box itself, already has focus so a literal "/" can still be typed.
function isEditableTarget(target) {
  return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable;
}

// Both banners live in one fixed-position stack so they pile up in normal flow
// instead of both claiming top:0 and hiding one another — which is exactly what
// happened once spoofing stopped being mutually exclusive with Super Edit Mode.
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
  banner.append(el('span', '', 'Super Edit Mode is on — you can edit anyone’s info.'));
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

export function renderSpoofBanner() {
  let banner = document.querySelector('.spoof-banner');
  if (!state.model.spoofingAs) {
    if (banner) {
      banner.remove();
      updateBannerOffset();
    }
    return;
  }
  if (banner) {
    banner.querySelector('.spoof-banner-name').textContent = state.model.spoofingAs;
    updateBannerOffset();
    return;
  }
  banner = el('div', 'spoof-banner');
  banner.append(el('span', '', 'Viewing as '));
  banner.append(el('span', 'spoof-banner-name', state.model.spoofingAs));
  const link = el('a', '', 'Stop');
  link.href = '#';
  link.addEventListener('click', async e => {
    e.preventDefault();
    await fetch('/api/admin/spoof', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({email: ''}),
    });
    await load();
  });
  banner.append(link);
  topBanners().append(banner);
  updateBannerOffset();
}

// Fills the user chrome (both avatars, the name label, the profile links, and
// the admin-only menu items) from the model's signed-in identity.
export function renderUserChrome() {
  const user = state.model.user;
  for (const avatar of document.querySelectorAll('.user-avatar')) {
    avatar.textContent = user.initial;
  }
  document.querySelector('.user-name').textContent = user.name;
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

  // Mobile carries its own copy of both topbar icons now that the avatar lives
  // in the same top-right corner as desktop's, so both share this one loop
  // (over every .stale-alert/.privacy-alert in the page) instead of each
  // querying a single id.
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
  window.addEventListener('resize', updateMobileTitleInset);
  window.addEventListener('resize', syncViewportHeight);
  window.addEventListener('orientationchange', syncViewportHeight);
  window.addEventListener('resize', updateBannerOffset);

  // The topbar only has room for the avatar (no name label), so - unlike the old
  // sidebar row, which let a name click open the menu and an avatar click jump
  // straight to the profile - the avatar's only job now is opening the menu, whose
  // first item is "View Profile".
  document.querySelector('#user').addEventListener('click', e => {
    e.stopPropagation();
    userMenu.hidden = !userMenu.hidden;
  });

  // Mobile's counterpart to the desktop topbar avatar above - same menu markup,
  // pinned to the top-right of .mobile-top instead of tucked into the drawer, so
  // the account entry point sits in the same corner on every breakpoint.
  document.querySelector('#mobile-user').addEventListener('click', e => {
    e.stopPropagation();
    mobileUserMenu.hidden = !mobileUserMenu.hidden;
  });

  // The stale-count badge (desktop and mobile both) used to link straight to
  // /my-family; now it opens a dropdown built from the same todoChecklist used
  // on My Family/a person's own page, so what to fix and the link to fix it are
  // both right there without leaving the current page.
  for (const staleButton of document.querySelectorAll('.stale-alert')) {
    const wrap = staleButton.closest('.stale-wrap');
    const panel = wrap.querySelector('.stale-menu');
    staleButton.addEventListener('click', e => {
      e.stopPropagation();
      const opening = panel.hidden;
      for (const p of document.querySelectorAll('.stale-menu')) {
        p.hidden = true;
      }
      if (opening) {
        panel.replaceChildren(todoChecklist(staleItems()));
        panel.hidden = false;
        clampFilterPanel(wrap, panel);
      }
    });
  }

  mobileListsOverlay.addEventListener('click', () => setMobileListsMenu(false));

  // Both the desktop and mobile user menus carry their own copy of this
  // checkbox (shown to admins only) - a quicker way to flip Super Edit Mode
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
    mobileUserMenu.hidden = true;
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

  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      userMenu.hidden = true;
      mobileUserMenu.hidden = true;
      for (const p of document.querySelectorAll('.stale-menu')) {
        p.hidden = true;
      }
      setDrawer(false);
      setMobileSearch(false);
      topbarSearchResults.hidden = true;
      closeFilterPanels();
      for (const menu of document.querySelectorAll('.more-menu, .card-menu, .photo-menu')) {
        menu.hidden = true;
      }
      if (e.target === topbarSearchInput || e.target === mobileSearchInput) {
        e.target.blur();
      }
    } else if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
      e.preventDefault();
      if (isMobile()) {
        setMobileSearch(true);
      } else {
        topbarSearchInput.focus();
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
