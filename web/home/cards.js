import {state, isAdmin} from './state.js';
import {el, svg, toast, categoryIcon} from './dom.js';
import {openLinkEditor, openCategoryEditor} from './edit.js';
import {appOrigin} from '/toolbar.js';

function closeMenus() {
  for (const menu of document.querySelectorAll('.more-menu')) {
    menu.hidden = true;
  }
}

async function copyLink(link) {
  await navigator.clipboard.writeText(link.url);
  toast('Link copied');
}

function openLink(link) {
  window.open(link.url, '_blank', 'noopener');
}

function menuItem(icon, label, action) {
  const button = el('button', '', '');
  button.type = 'button';
  button.append(svg(icon), el('span', '', label));
  button.addEventListener('click', e => {
    e.stopPropagation();
    closeMenus();
    action();
  });
  return button;
}

// Every link carries the same overflow menu in both styles; it is the only
// route to Edit, and on a tile it is the only route to Copy Link as well.
function moreMenu(link) {
  const wrap = el('div', 'more-wrap');
  const more = el('button', 'more-button');
  more.type = 'button';
  more.setAttribute('aria-label', `More for ${link.title}`);
  more.append(svg('more'));
  const menu = el('div', 'more-menu');
  menu.hidden = true;
  menu.append(menuItem('go', 'Go!', () => openLink(link)));
  menu.append(menuItem('copy', 'Copy Link', () => copyLink(link)));
  if (isAdmin() && state.superAdmin) {
    menu.append(menuItem('edit', 'Edit', () => openLinkEditor(link)));
  }
  more.addEventListener('click', e => {
    e.stopPropagation();
    e.preventDefault();
    const opening = menu.hidden;
    closeMenus();
    menu.hidden = !opening;
  });
  wrap.append(more, menu);
  return wrap;
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
  card.append(corner, moreMenu(link));
  return card;
}

// A tile: the picture, then the initial badge over the title and description.
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
  body.append(el('div', 'tile-initial', link.title.slice(0, 1).toUpperCase()));
  const title = el('div', 'tile-title', link.title);
  if (!link.visible) {
    title.append(el('span', 'hidden-badge', 'Hidden'));
  }
  body.append(title);
  if (link.description) {
    body.append(el('div', 'tile-description', link.description));
  }
  open.append(body);
  card.append(open, moreMenu(link));
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
    // than from links, and its heading gets a way across to the portal.
    const events = category.style === 'events';
    const links = events ? [] : category.links.filter(link => listed(link) && matches(link, needle));
    const count = events ? eventsMatching(needle) : links.length;
    // A section with nothing to show stays off the page - except in Super
    // Admin Mode, where it appears empty so it can be filled or edited.
    if (!count && (needle || !isAdmin() || !state.superAdmin)) {
      continue;
    }
    shown += count;
    const section = el('section', 'category' + (events ? ' upcoming' : ''));
    section.id = anchorFor(category.title);
    const head = el('div', 'category-head');
    const title = el('h2', 'category-title', category.title);
    head.append(categoryGlyph(category, 'category-icon'), title);
    if (events) {
      const all = el('a', 'category-more');
      all.href = appOrigin('team');
      all.append(el('span', '', 'See all in HCA-Team'), svg('chevron'));
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
    section.append(events ? eventsPanel(category, needle) : panel(category, links, needle));
    root.append(section);
  }
  const empty = document.querySelector('#empty-search');
  empty.hidden = Boolean(shown) || !needle;
  if (!state.model.categories.length) {
    root.append(el('div', 'footnote', 'Nothing here yet.'));
  }
}

// A Google Calendar "add this" link for an event: an all-day entry for a
// bare day (or a span of days), a timed one when the sheet gives times - two
// hours long when it gives no end - pinned to the school's time zone. The
// community lives in Google Workspace, so this beats an .ics download on
// every device it has.
function calendarLink(event) {
  const compact = cell => cell.replace(/[-: ]/g, '');
  const timed = event.startAt.length > 10;
  let dates;
  if (timed) {
    let end = event.endAt && event.endAt.length > 10 ? event.endAt : '';
    if (!end) {
      const start = new Date(event.startAt.replace(' ', 'T'));
      start.setHours(start.getHours() + 2);
      const pad = n => String(n).padStart(2, '0');
      end = `${start.getFullYear()}-${pad(start.getMonth() + 1)}-${pad(start.getDate())} ${pad(start.getHours())}:${pad(start.getMinutes())}`;
    }
    dates = `${compact(event.startAt)}00/${compact(end)}00`;
  } else {
    // All-day entries end on the day after the last day.
    const last = new Date((event.endAt || event.startAt).slice(0, 10) + 'T00:00:00');
    last.setDate(last.getDate() + 1);
    const pad = n => String(n).padStart(2, '0');
    dates = `${compact(event.startAt.slice(0, 10))}/${last.getFullYear()}${pad(last.getMonth() + 1)}${pad(last.getDate())}`;
  }
  const params = new URLSearchParams({
    action: 'TEMPLATE', text: event.title, dates, ctz: 'America/Los_Angeles',
    details: [event.description, appOrigin('team') + event.path].filter(Boolean).join('\n\n'),
  });
  if (event.location) {
    params.set('location', event.location);
  }
  return 'https://calendar.google.com/calendar/render?' + params.toString();
}

// Upcoming Events: the portal's next few open, dated events, each a card that
// opens its page in HCA-Team, with Add to Calendar and Volunteer (the event's
// page, where the sign-up is) under it. The search box filters them by title
// like the links; with none ahead, or none matching, the section stays off
// the page.
function eventCard(event) {
  const card = el('div', 'event-card');
  const open = el('a', 'event-open');
  open.href = appOrigin('team') + event.path;
  const art = el('div', 'event-art');
  // The picture lives on the portal's host, like the page it opens.
  if (event.imageUrl) {
    const img = el('img');
    img.src = appOrigin('team') + event.imageUrl;
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
  if (event.when) {
    // "Saturday, March 6 · 5:30–10:00 PM" reads as two lines beside a calendar.
    const [day, time] = event.when.split(' · ');
    const when = el('div', 'event-when');
    const lines = el('div', 'event-when-lines');
    lines.append(el('span', '', day));
    if (time) {
      lines.append(el('span', '', time.replace('–', ' – ')));
    }
    when.append(svg('calendar'), lines);
    body.append(when);
  }
  open.append(art, body);
  // One row: a calendar-with-plus icon button, and Volunteer.
  const actions = el('div', 'event-actions');
  const calendar = el('a', 'button button-secondary button-small event-calendar');
  calendar.href = calendarLink(event);
  calendar.target = '_blank';
  calendar.rel = 'noopener';
  calendar.title = 'Add to Calendar';
  calendar.setAttribute('aria-label', `Add ${event.title} to your calendar`);
  calendar.append(svg('calendarAdd'));
  const volunteer = el('a', 'button button-small');
  volunteer.href = open.href;
  volunteer.append(svg('volunteer'), el('span', '', 'Volunteer'));
  actions.append(calendar, volunteer);
  card.append(open, actions);
  return card;
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

export function renderNav() {
  const nav = document.querySelector('#app-nav');
  nav.replaceChildren();
  const home = el('a', 'is-active');
  home.href = '#';
  home.append(svg('home'), el('span', '', 'Home'));
  nav.append(home);
  for (const category of state.model.categories) {
    const item = el('a', '');
    item.href = '#' + anchorFor(category.title);
    item.append(categoryGlyph(category, 'app-nav-glyph'), el('span', '', category.title));
    nav.append(item);
  }
  nav.addEventListener('click', e => {
    const link = e.target.closest('a');
    if (!link) {
      return;
    }
    for (const a of nav.querySelectorAll('a')) {
      a.classList.toggle('is-active', a === link);
    }
  });
}

document.addEventListener('click', closeMenus);
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    closeMenus();
  }
});
