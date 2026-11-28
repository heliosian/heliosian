import {el, link, svg, button, toast} from '/elements.js';
import {initShell, appSymbol} from '/shell.js';
import {me, findInvite, pagesOf, movePage, dayWords, hourWords, state} from './state.js';
import {openThemeManager} from './thememanager.js';
import {load, navigate} from '/router.js';
import {openPageEditor} from './pageeditor.js';
import {parseWhen, googleCalendarLink} from '/datecard.js';
import {andList, whereWords} from '/envelope.js';
import {inviteHead, madeWith} from '/sidebar.js';

function navLink(href, label, icon, active) {
  const a = link(href, active ? 'is-active' : '');
  a.append(icon ? svg(icon) : appSymbol(), el('span', '', label));
  return a;
}

let dragging = null;

function draggablePage(a, page, inv, list) {
  a.draggable = true;
  a.dataset.page = page.id;
  a.classList.add('is-draggable');
  a.addEventListener('dragstart', e => {
    dragging = a;
    a.classList.add('is-dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', page.id);
  });
  a.addEventListener('dragover', e => {
    if (!dragging || dragging.parentNode !== list) {
      return;
    }
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (dragging === a) {
      return;
    }
    const box = a.getBoundingClientRect();
    list.insertBefore(dragging, e.clientY > box.top + box.height / 2 ? a.nextSibling : a);
  });
  a.addEventListener('drop', e => e.preventDefault());
  a.addEventListener('dragend', async () => {
    a.classList.remove('is-dragging');
    dragging = null;
    const ids = [...list.children].map(x => x.dataset.page);
    if (ids.join() === pagesOf(inv).map(p => p.id).join()) {
      return;
    }
    try {
      await movePage(inv, page.id, ids);
    } catch (err) {
      toast(err.message);
    }
    load();
  });
}

function hostMenu(key, inv, base) {
  const holder = el('div', 'tab-host');
  const menu = el('div', 'tab-host-menu');
  menu.hidden = true;
  const item = (label, icon, run) => {
    const b = button(label, icon, 'tab-host-item', () => {
      menu.hidden = true;
      run();
    });
    menu.append(b);
  };
  item('Questions', 'list', () => navigate(`${base}/questions`));
  item('Notes from guests', 'chat', () => navigate(`${base}/notes`));
  item('Add a page', 'plus', () => openPageEditor(inv, null, key));
  const path = location.pathname;
  const toggle = button('Host', 'gear', 'tab-button' + (path === `${base}/questions` || path === `${base}/notes` ? ' is-active' : ''), () => {
    menu.hidden = !menu.hidden;
  });
  holder.append(menu, toggle);
  openHost = holder;
  return holder;
}

let openHost = null;

document.addEventListener('click', ev => {
  if (openHost && !openHost.contains(ev.target)) {
    openHost.querySelector('.tab-host-menu').hidden = true;
  }
});

function phoneNav(nav, key, inv) {
  const base = `/g/${encodeURIComponent(key)}`;
  const path = location.pathname;
  const rsvp = navLink(base, 'RSVP', 'check', false);
  rsvp.addEventListener('click', ev => {
    ev.preventDefault();
    state.showRsvp = true;
    if (path === base && document.getElementById('rsvp')) {
      document.getElementById('rsvp').scrollIntoView({behavior: 'smooth', block: 'start'});
      return;
    }
    navigate(base);
  });
  nav.append(rsvp, navLink(base, 'Invitation', 'mail', path === base || path.startsWith(`${base}/p/`)));
  if (inv.host || inv.listShown) {
    nav.append(navLink(`${base}/guests`, inv.host ? 'Guest list' : 'Who’s coming', 'groups', path === `${base}/guests`));
  }
  if (inv.host) {
    nav.append(hostMenu(key, inv, base));
  }
}

function inviteNav(nav, key, inv) {
  const base = `/g/${encodeURIComponent(key)}`;
  const path = location.pathname;
  nav.append(navLink(base, 'Invitation', 'mail', path === base));
  const pages = el('div', 'nav-pages');
  for (const page of pagesOf(inv)) {
    const href = `${base}/p/${page.id}`;
    const a = navLink(href, page.title, 'doc', path === href);
    if (inv.host) {
      draggablePage(a, page, inv, pages);
    }
    pages.append(a);
  }
  nav.append(pages);
  if (inv.host || inv.listShown) {
    nav.append(navLink(`${base}/guests`, inv.host ? 'Guest list' : 'Who’s coming', 'groups', path === `${base}/guests`));
  }
  if (inv.host) {
    nav.append(navLink(`${base}/questions`, 'Questions', 'list', path === `${base}/questions`));
    nav.append(navLink(`${base}/notes`, 'Notes from guests', 'chat', path === `${base}/notes`));
    nav.append(button('Add a page', 'plus', 'nav-add', () => openPageEditor(inv, null, key)));
  }
}

const headDay = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'long', day: 'numeric', year: 'numeric'});

function headOf(key, inv) {
  const from = parseWhen(inv.start);
  return inviteHead({
    href: `/g/${encodeURIComponent(key)}`, name: inv.name, look: inv.look, day: from ? headDay.format(from.date) : dayWords(inv), hours: hourWords(inv),
    calendar: googleCalendarLink({title: inv.name, start: inv.start, end: inv.end, allDay: inv.allDay, location: whereWords(inv), details: [inv.description, `${location.origin}/g/${encodeURIComponent(key)}`].filter(Boolean).join('\n\n')}),
    place: inv.place, location: inv.location, hostedBy: inv.hostedBy || andList(inv.hostNames),
  });
}

function brandFor(inv) {
  const brand = document.querySelector('.sidebar .brand');
  brand.hidden = Boolean(inv);
  const bar = document.querySelector('#topbar');
  let line = bar.querySelector('.topbar-subtitle');
  if (!line) {
    line = el('div', 'topbar-subtitle');
    const menu = bar.querySelector('.topbar-menu');
    if (menu) {
      menu.after(line);
    } else {
      bar.prepend(line);
    }
  }
  line.hidden = !inv;
  if (!inv) {
    line.replaceChildren();
    return;
  }
  line.replaceChildren(el('span', 'topbar-title', inv.name));
  if (inv.subtitle) {
    line.append(el('span', 'topbar-subtitle-words', inv.subtitle));
  }
}

function fillNav(nav, sidebar) {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const inv = parts[0] === 'g' && parts[1] ? findInvite(parts[1]) : null;
  brandFor(inv);
  if (inv) {
    if (!sidebar) {
      phoneNav(nav, parts[1], inv);
      return;
    }
    nav.append(headOf(parts[1], inv));
    inviteNav(nav, parts[1], inv);
    nav.append(madeWith());
    return;
  }
  nav.append(navLink('/', 'Invitations', '', true));
  if (state.model.gatherAdmin) {
    const manage = navLink('/', 'Manage themes', 'doc', false);
    manage.addEventListener('click', ev => {
      ev.preventDefault();
      ev.stopPropagation();
      openThemeManager();
    });
    nav.append(manage);
  }
}

export function initChrome() {
  initShell({
    name: 'Helios Gather',
    me,
    search: {placeholder: 'Search invitations…', carry: () => navigate('/')},
    fillNav: nav => fillNav(nav, true),
    fillTabbar: nav => fillNav(nav, false),
  });
}
