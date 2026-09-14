import {state, isAdmin} from './state.js';
import {el, svg, categoryIcon, toast} from './dom.js';
import {openLinkEditor, openCategoryEditor} from './edit.js';
import {appOrigin} from '/toolbar.js';

// In Super Admin Mode every link wears a pencil in its corner, the way into
// its editor; a card has no other menu.
function editPencil(link) {
  if (!isAdmin() || !state.superAdmin) {
    return null;
  }
  const pencil = el('button', 'link-edit');
  pencil.type = 'button';
  pencil.title = 'Edit link';
  pencil.setAttribute('aria-label', `Edit ${link.title}`);
  pencil.append(svg('edit'));
  pencil.addEventListener('click', e => {
    e.stopPropagation();
    e.preventDefault();
    openLinkEditor(link);
  });
  return pencil;
}

// The glyph a category goes by: its emoji when the sheet gives it one, else
// an outline read off its title.
function categoryGlyph(category, className) {
  if (category.emoji) {
    return el('span', className + ' is-emoji', category.emoji);
  }
  const wrap = el('span', className);
  wrap.append(svg(categoryIcon(category.title)));
  return wrap;
}

// A link without its own image shows its category's emoji, which is how a
// whole category of chats shares one mark; with neither, the title's initial
// stands in.
function artwork(link, category, imageClass, initialClass) {
  if (link.imageUrl) {
    const img = el('img', imageClass);
    img.src = link.imageUrl;
    img.alt = '';
    img.loading = 'lazy';
    return img;
  }
  if (category.emoji) {
    return el('div', initialClass + ' is-emoji', category.emoji);
  }
  return el('div', initialClass, link.title.slice(0, 1).toUpperCase());
}

function openInNewTab(url) {
  const a = el('a');
  a.href = url;
  a.target = '_blank';
  a.rel = 'noopener';
  return a;
}

// A feature card: the picture in a disc, the words, the Open App button, and
// the same picture again large and faint behind the right edge, with a round
// chevron in the corner that opens the link too.
function featureCard(link, category) {
  const card = el('div', 'feature' + (link.visible ? '' : ' is-hidden'));
  const disc = el('div', 'feature-disc');
  disc.append(artwork(link, category, 'feature-image', 'feature-initial'));
  card.append(disc);

  const body = el('div', 'feature-body');
  const title = el('div', 'feature-title', link.title);
  if (!link.visible) {
    title.append(el('span', 'hidden-badge', 'Hidden'));
  }
  body.append(title);
  if (link.description) {
    body.append(el('div', 'feature-description', link.description));
  }
  const go = openInNewTab(link.url);
  go.className = 'button';
  go.append(el('span', '', 'Open App'));
  body.append(go);
  card.append(body);

  if (link.imageUrl) {
    card.append(artwork(link, category, 'feature-watermark', ''));
  }
  const corner = openInNewTab(link.url);
  corner.className = 'feature-corner';
  corner.setAttribute('aria-label', `Open ${link.title}`);
  corner.append(svg('chevron'));
  card.append(corner);
  const pencil = editPencil(link);
  if (pencil) {
    card.append(pencil);
  }
  return card;
}

// A tile: the picture, then the title and description.
// The anchor holds only the content; the overflow button is a sibling, since
// a button nested inside an anchor is invalid and swallows its own clicks.
function tile(link, category) {
  const card = el('div', 'tile' + (link.visible ? '' : ' is-hidden'));
  const open = openInNewTab(link.url);
  open.className = 'tile-link';
  if (link.imageUrl) {
    open.append(artwork(link, category, 'tile-image', ''));
  } else {
    const blank = el('div', 'tile-image tile-image-blank');
    blank.append(categoryGlyph(category, 'tile-glyph'));
    open.append(blank);
  }
  const body = el('div', 'tile-body');
  const title = el('div', 'tile-title', link.title);
  if (!link.visible) {
    title.append(el('span', 'hidden-badge', 'Hidden'));
  }
  body.append(title);
  if (link.description) {
    body.append(el('div', 'tile-description', link.description));
  }
  open.append(body);
  card.append(open);
  const pencil = editPencil(link);
  if (pencil) {
    card.append(pencil);
  }
  return card;
}

// "Events" adds an Event and "Chats" a Chat; a category whose title does not
// end that way adds a Link.
function singular(title) {
  const t = title.trim();
  return /^[A-Z][a-z]+s$/.test(t) ? t.slice(0, -1) : 'Link';
}

// The admin's way in: a dashed card at the end of every category that opens
// the link editor with that category already chosen. It comes and goes with
// Super Admin Mode, so the page an admin reads by default is the page
// everyone else gets.
function addCard(category, cards) {
  const card = el('button', cards ? 'feature add-card' : 'tile add-card');
  card.type = 'button';
  const disc = el('div', 'add-card-disc');
  disc.append(svg('plus'));
  const body = el('div', 'add-card-body');
  body.append(el('div', 'add-card-title', `Add ${singular(category.title)}`));
  body.append(el('div', 'add-card-sub', `Link something under ${category.title}`));
  card.append(disc, body);
  card.addEventListener('click', () => openLinkEditor(null, category.title));
  return card;
}

// A category's Max caps what shows until See More; the choice to see more
// lasts until the next reload, and a search shows every match regardless.
const expanded = new Set();

function limited(category, items, needle) {
  if (!category.max || needle || expanded.has(category.title) || items.length <= category.max) {
    return {shown: items, hidden: 0};
  }
  return {shown: items.slice(0, category.max), hidden: items.length - category.max};
}

function seeMore(category, hidden, rerender) {
  const button = el('button', 'button button-secondary see-more');
  button.type = 'button';
  button.append(el('span', '', `See more (${hidden})`), svg('chevron'));
  button.addEventListener('click', () => {
    expanded.add(category.title);
    rerender();
  });
  return button;
}

// The sheet's Style column decides the shape of each section; the loader has
// already refused anything that is not one of these two.
function panel(category, links, needle) {
  const cards = category.style === 'cards';
  const wrap = el('div');
  const grid = el('div', cards ? 'card-grid' : 'tile-grid');
  const {shown, hidden} = limited(category, links, needle);
  for (const link of shown) {
    grid.append(cards ? featureCard(link, category) : tile(link, category));
  }
  if (isAdmin() && state.superAdmin) {
    grid.append(addCard(category, cards));
  }
  wrap.append(grid);
  if (hidden) {
    wrap.append(seeMore(category, hidden, () => renderCategories(needle)));
  }
  return wrap;
}

export function anchorFor(title) {
  return 'section-' + title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
}

function matches(link, query) {
  if (!query) {
    return true;
  }
  return `${link.title} ${link.description || ''} ${link.url}`.toLowerCase().includes(query);
}

// The server sends hidden links to admins only; the page shows them only while
// the admin's Super Admin Mode switch is on, so what an admin looks at by
// default is what everyone else gets.
function listed(link) {
  return link.visible || state.superAdmin;
}

export function renderCategories(query = '') {
  const root = document.querySelector('#categories');
  const needle = query.trim().toLowerCase();
  root.replaceChildren();
  let shown = 0;
  for (const category of state.model.categories) {
    // The events section is the portal's; its cards come from there rather
    // than from links, and its heading gets a way across to the portal. The
    // apps section is the community apps themselves, from the model.
    const events = category.style === 'events';
    const apps = category.style === 'apps';
    const links = events || apps ? [] : category.links.filter(link => listed(link) && matches(link, needle));
    const count = events ? eventsMatching(needle) : apps ? appsMatching(needle).length : links.length;
    // A section with nothing to show stays off the page - except in Super
    // Admin Mode, where it appears empty so it can be filled or edited.
    if (!count && (needle || !isAdmin() || !state.superAdmin)) {
      continue;
    }
    shown += count;
    const section = el('section', 'category' + (events ? ' upcoming' : ''));
    section.id = anchorFor(category.title);
    // The heading is the title alone; the category's emoji marks it in the
    // rail, not here.
    const head = el('div', 'category-head');
    const title = el('h2', 'category-title', category.title);
    head.append(title);
    // Upcoming Events names the saved calendar it is read under beside
    // the heading, as a quiet dropdown of every saved calendar; one that
    // is not the default gets a Make default beside it.
    if (events && state.model.upcomingCalendar) {
      head.append(calendarPicker(needle));
    }
    if (events) {
      const all = el('a', 'category-more');
      all.href = whenOrigin('calendar');
      all.append(el('span', '', 'See all in Helios When'), svg('chevron'));
      head.append(all);
    }
    if (isAdmin() && state.superAdmin) {
      const edit = el('button', 'category-edit');
      edit.type = 'button';
      edit.title = 'Edit category';
      edit.setAttribute('aria-label', `Edit the ${category.title} category`);
      edit.append(svg('edit'));
      edit.addEventListener('click', () => openCategoryEditor(category));
      const more = head.querySelector('.category-more');
      head.insertBefore(edit, more);
    }
    section.append(head);
    // An empty category still gets its grid for the admin's add card.
    section.append(events ? eventsPanel(category, needle) : apps ? appsPanel(category, needle) : panel(category, links, needle));
    root.append(section);
  }
  const empty = document.querySelector('#empty-search');
  empty.hidden = Boolean(shown) || !needle;
  if (!state.model.categories.length) {
    root.append(el('div', 'footnote', 'Nothing here yet.'));
  }
}

// Upcoming Events: Helios When's next few events for this person - the
// school's, HCA-Team's and Celebrate's as the calendar lists them - each a
// card that opens its page in When, with Add to Calendar under it and, for an
// event another app runs, the way in there: Join, Get tickets, or where the
// household already stands. The search box filters them by title like the
// links; with none ahead, or none matching, the section stays off the page.
// whenOrigin is an app's origin on this tier, the calendar's under the name
// it answers to rather than the one that redirects there.
export function whenOrigin(app) {
  return appOrigin(app === 'calendar' ? 'when' : app);
}

// eventCard is one of the events coming up, from Helios Calendar - the
// school's, HCA-Team's and Celebrate's as the calendar lists them - opening
// its page on When, with Yes and No under it, a cross to hide it, and for
// an event another app runs, the way in as When's pill words it.
function eventCard(event) {
  const card = el('div', 'event-card');
  const open = el('a', 'event-open');
  open.href = whenOrigin('calendar') + event.path;
  const art = el('div', 'event-art');
  // The picture is the one the event's page wears, fetched from whichever
  // app serves it - the calendar, or the app that runs a linked event.
  if (event.image) {
    const img = el('img');
    img.src = whenOrigin(event.imageApp) + event.image;
    img.alt = '';
    img.loading = 'lazy';
    art.append(img);
  } else {
    art.append(svg('calendar'));
  }
  // The stamp reads the day off the YYYY-MM-DD start without the browser's
  // time zone shifting it.
  const [, month, day] = event.start.split('-');
  const stamp = el('div', 'event-stamp');
  stamp.append(el('span', 'event-stamp-month', new Date(2000, Number(month) - 1, 1).toLocaleDateString('en-US', {month: 'short'})));
  stamp.append(el('span', 'event-stamp-day', String(Number(day))));
  art.append(stamp);
  const body = el('div', 'event-body');
  body.append(el('div', 'event-title', event.title));
  // The stamp on the picture is the day; under the title, the hours alone.
  const [, time] = (event.when || '').split(' · ');
  if (time) {
    const when = el('div', 'event-when');
    const lines = el('div', 'event-when-lines');
    lines.append(el('span', '', time));
    when.append(svg('clock'), lines);
    body.append(when);
  }
  open.append(art, body);
  // The cross at the corner hides the event from this person's lists -
  // still on the calendar's month, in gray, and in its search.
  const hide = el('button', 'event-hide');
  hide.type = 'button';
  hide.title = 'Hide this event';
  hide.setAttribute('aria-label', 'Hide this event');
  hide.append(svg('close'));
  hide.addEventListener('click', async e => {
    e.preventDefault();
    if (await answer(event, 'hidden')) {
      card.remove();
    }
  });
  art.append(hide);
  // One row: Yes and No (a party's Send Invite or Add Ticket), and beside
  // them, when the event has a way in and nobody is in yet, a button
  // saying what When's pill says - teal while it is open, plain once it
  // is full.
  const actions = el('div', 'event-actions');
  actions.append(rsvpButtons(event));
  // The household's part in a linked event is a line above the buttons -
  // each ticket holder, each volunteer with their role - rather than a
  // button; the way in stays a button only while nobody is in yet.
  if (event.people && event.people.length) {
    card.append(open, peopleList(event), actions);
    return card;
  }
  if (event.call && event.linkApp !== 'celebrate') {
    const live = event.availability === 'open' || event.availability === 'available';
    const go = el('a', 'button button-small' + (live ? '' : ' button-secondary'));
    go.href = whenOrigin(event.linkApp) + event.link;
    go.append(svg('volunteer'), el('span', '', event.call));
    actions.append(go);
  }
  card.append(open, actions);
  return card;
}

// rsvpButtons is the event's answer as buttons: Yes and No, the one given
// filled - a yes brings a calendar invite by email. A party has no yes or
// no: with a ticket in the household, Send Invite puts it on this person's
// calendar; without one, Add Ticket goes to the party page. The rail's
// day card and the Upcoming cards share it.
export function rsvpButtons(event) {
  const rsvp = el('div', 'event-rsvp');
  if (event.linkApp === 'celebrate') {
    const held = (event.people || []).some(p => p.note !== 'waitlisted');
    if (held) {
      const send = el('button', 'button button-small event-invite' + (event.answer === 'yes' ? ' button-secondary' : ''));
      send.type = 'button';
      const label = () => {
        send.replaceChildren(svg(event.answer === 'yes' ? 'check' : 'calendarAdd'), el('span', '', event.answer === 'yes' ? 'Invite sent' : 'Send Invite'));
        send.classList.toggle('button-secondary', event.answer === 'yes');
        send.title = event.answer === 'yes' ? 'Sent to your email - click to send it again' : 'Email me a calendar invite';
      };
      label();
      send.addEventListener('click', async () => {
        if (await answer(event, 'yes')) {
          event.answer = 'yes';
          label();
          toast('A calendar invite is on its way to your email');
        }
      });
      rsvp.append(send);
    } else {
      const live = event.availability === 'available';
      const add = el('a', 'button button-small' + (live ? '' : ' button-secondary'));
      add.href = whenOrigin(event.linkApp) + event.link;
      add.append(svg('ticket'), el('span', '', live ? 'Add Ticket' : event.call || 'See the party'));
      rsvp.append(add);
    }
    return rsvp;
  }
  const yes = el('button', 'button button-small event-yes');
  yes.type = 'button';
  yes.append(svg('check'), el('span', '', 'Yes'));
  const no = el('button', 'button button-small event-no');
  no.type = 'button';
  no.append(svg('close'), el('span', '', 'No'));
  const mark = () => {
    yes.classList.toggle('button-secondary', event.answer !== 'yes');
    no.classList.toggle('button-secondary', event.answer !== 'no');
    yes.title = event.answer === 'yes' ? 'You said yes - the invite is in your email' : 'Yes, and send me a calendar invite';
  };
  mark();
  yes.addEventListener('click', async () => {
    const next = event.answer === 'yes' ? '' : 'yes';
    if (await answer(event, next)) {
      event.answer = next;
      mark();
    }
  });
  no.addEventListener('click', async () => {
    const next = event.answer === 'no' ? '' : 'no';
    if (await answer(event, next)) {
      event.answer = next;
      mark();
    }
  });
  rsvp.append(yes, no);
  return rsvp;
}

// calendarMark is a saved calendar's mark: its emoji, else the calendar
// outline.
export function calendarMark(c) {
  return c.emoji ? el('span', 'category-calendar-emoji', c.emoji) : svg('calendar');
}

// calendarMenu lists every saved calendar to pick from, the current one
// lit, the default and My Heliosian's lock marked at the end of their
// rows; picking one closes the menu and hands it to onPick. Upcoming
// Events and the rail's month share it.
export function calendarMenu(list, current, chosen, onPick) {
  const menu = el('div', 'category-calendar-menu');
  menu.hidden = true;
  for (const c of list) {
    const item = el('button', 'category-calendar-item' + (c.token === current.token ? ' is-on' : ''));
    item.type = 'button';
    item.append(calendarMark(c), el('span', '', c.name));
    const tail = el('span', 'category-calendar-tail');
    if (c.token === chosen.token) {
      tail.append(el('span', 'category-calendar-default', 'default'));
    }
    if (c.locked) {
      tail.append(el('span', 'category-calendar-lock', '\ud83d\udd12'));
    }
    if (tail.childElementCount) {
      item.append(tail);
    }
    item.addEventListener('click', () => {
      menu.hidden = true;
      onPick(c);
    });
    menu.append(item);
  }
  return menu;
}

// dropdown opens the menu under its toggle and closes it on the next
// click anywhere else.
export function dropdown(toggle, menu) {
  toggle.addEventListener('click', e => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
    if (!menu.hidden) {
      document.addEventListener('click', () => {
        menu.hidden = true;
      }, {once: true});
    }
  });
  menu.addEventListener('click', e => e.stopPropagation());
}

// calendarPicker is the saved calendar Upcoming Events is read under, as
// a small dropdown beside the heading: picking another re-reads the
// events under it, and Make default beside it makes that one the default
// on Helios When - the one this page and its rail open to.
function calendarPicker(needle) {
  const cal = state.model.upcomingCalendar;
  const list = cal.calendars || [];
  const current = list.find(c => c.token === cal.calendar) || list[0];
  const chosen = list.find(c => c.token === cal.default) || list[0];
  const wrap = el('div', 'category-calendar');
  const toggle = el('button', 'category-calendar-toggle');
  toggle.type = 'button';
  toggle.title = 'The saved calendar these events come from';
  toggle.append(calendarMark(current), el('span', '', current.name), svg('chevron'));
  if (current.locked) {
    toggle.title = 'The calendar\u2019s own view, for everyone';
  }
  const menu = calendarMenu(list, current, chosen, async c => {
    const res = await fetch('/api/apps/upcoming?calendar=' + encodeURIComponent(c.token));
    if (!res.ok) {
      toast(await res.text());
      return;
    }
    const ahead = await res.json();
    state.model.upcoming = ahead.events;
    state.model.upcomingCalendar = {calendar: ahead.calendar, default: ahead.default, calendars: ahead.calendars};
    renderCategories(needle);
  });
  dropdown(toggle, menu);
  wrap.append(toggle, menu);
  if (current.token !== chosen.token) {
    const make = el('button', 'category-calendar-make');
    make.type = 'button';
    make.append(svg('star'), el('span', '', 'Make default'));
    make.title = 'Open Heliosian and Helios When to this calendar from now on - it moves to the top of the rail';
    make.addEventListener('click', async () => {
      const res = await fetch('/api/apps/calendar/default', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({token: current.token})});
      if (!res.ok) {
        toast(await res.text());
        return;
      }
      toast(`${current.name} is your default calendar now`);
      const {load} = await import('./app.js');
      await load();
    });
    wrap.append(make);
  }
  return wrap;
}

// peopleList is the household's part in a linked event on one line behind
// one icon: the names with a dot between each, and a note - a role, a
// waitlist place, a guest still to be named - in parentheses after its name.
function peopleList(event) {
  const line = el('div', 'event-people');
  line.append(svg(event.linkApp === 'celebrate' ? 'ticket' : 'volunteer'));
  const names = el('span', 'event-people-names');
  event.people.forEach((p, i) => {
    if (i) {
      names.append(el('span', 'event-people-sep', '\u2022'));
    }
    names.append(el('span', 'event-person', p.name));
    if (p.note) {
      names.append(el('span', 'event-person-note', `(${p.note})`));
    }
  });
  line.append(names);
  return line;
}

// answer tells the calendar this person's word on an event - yes, no,
// hidden, or nothing - and says whether it took.
async function answer(event, word) {
  const res = await fetch('/api/apps/rsvp', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: event.id, answer: word})});
  if (!res.ok) {
    toast(await res.text());
    return false;
  }
  return true;
}

// The events section's body: the portal's next few, or a word when nothing is
// ahead; the search box filters them by title like the links.
function eventsPanel(category, needle) {
  const events = (state.model.upcoming || []).filter(e => !needle || e.title.toLowerCase().includes(needle));
  if (!events.length) {
    return el('div', 'category-empty', needle ? 'No events match.' : 'Nothing coming up yet.');
  }
  const wrap = el('div');
  const grid = el('div', 'event-grid');
  const {shown, hidden} = limited(category, events, needle);
  for (const event of shown) {
    grid.append(eventCard(event));
  }
  wrap.append(grid);
  if (hidden) {
    wrap.append(seeMore(category, hidden, () => renderCategories(needle)));
  }
  return wrap;
}

function eventsMatching(needle) {
  return (state.model.upcoming || []).filter(e => !needle || e.title.toLowerCase().includes(needle)).length;
}

// The apps section: the community apps this person sees, as the model lists
// them - each a feature card with its mark, name and tagline, opening the app
// on this tier in the same tab, the way the toolbar's switch does. The search
// box filters them by name and tagline like the links.
function appsMatching(needle) {
  return (state.model.apps || []).filter(a => !needle || `${a.name} ${a.tagline}`.toLowerCase().includes(needle));
}

function appCard(app) {
  const card = el('div', 'feature');
  const disc = el('div', 'feature-disc');
  const icon = el('img', 'feature-image');
  icon.src = `/brand/apps/${app.key}.png` + (app.mark ? `?v=${app.mark}` : '');
  icon.alt = '';
  disc.append(icon);
  card.append(disc);
  const body = el('div', 'feature-body');
  body.append(el('div', 'feature-title', app.name));
  body.append(el('div', 'feature-description', app.tagline));
  const go = el('a', 'button');
  go.href = appOrigin(app.host || app.key);
  go.append(el('span', '', 'Open App'));
  body.append(go);
  card.append(body);
  const watermark = el('img', 'feature-watermark');
  watermark.src = icon.src;
  watermark.alt = '';
  card.append(watermark);
  const corner = el('a', 'feature-corner');
  corner.href = go.href;
  corner.setAttribute('aria-label', `Open ${app.name}`);
  corner.append(svg('chevron'));
  card.append(corner);
  return card;
}

function appsPanel(category, needle) {
  const apps = appsMatching(needle);
  if (!apps.length) {
    return el('div', 'category-empty', needle ? 'No apps match.' : 'No apps to show.');
  }
  const wrap = el('div');
  const grid = el('div', 'card-grid');
  const {shown, hidden} = limited(category, apps, needle);
  for (const app of shown) {
    grid.append(appCard(app));
  }
  wrap.append(grid);
  if (hidden) {
    wrap.append(seeMore(category, hidden, () => renderCategories(needle)));
  }
  return wrap;
}

// Whether a category has anything on the page for the reader: a visible
// link, an event ahead, or an app to show - and, in Super Admin Mode,
// anything at all.
function hasSomething(category) {
  if (isAdmin() && state.superAdmin) {
    return true;
  }
  if (category.style === 'events') {
    return eventsMatching('') > 0;
  }
  if (category.style === 'apps') {
    return appsMatching('').length > 0;
  }
  return category.links.some(listed);
}

export function renderNav() {
  const nav = document.querySelector('#app-nav');
  nav.replaceChildren();
  const home = el('a', 'is-active');
  home.href = '#';
  home.append(svg('home'), el('span', '', 'Home'));
  nav.append(home);
  // The rail lists the sections the page shows, so an empty one stays off
  // it too.
  for (const category of state.model.categories.filter(hasSomething)) {
    const item = el('a', '');
    item.href = '#' + anchorFor(category.title);
    item.append(categoryGlyph(category, 'app-nav-glyph'), el('span', '', category.title));
    nav.append(item);
  }
}

// The rail marks the section last chosen; wired once, since the rail is
// rebuilt on every load and mode change.
document.querySelector('#app-nav').addEventListener('click', e => {
  const nav = e.currentTarget;
  const link = e.target.closest('a');
  if (!link) {
    return;
  }
  for (const a of nav.querySelectorAll('a')) {
    a.classList.toggle('is-active', a === link);
  }
});

