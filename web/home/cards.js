import {state, isAdmin} from './state.js';
import {el, svg, toast} from './dom.js';
import {openLinkEditor, openCategoryEditor} from './edit.js';

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
  if (isAdmin()) {
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

// A link without its own image borrows its category's, which is how a whole
// category of chats shares one mark; with neither, the title's initial stands in.
function artwork(link, category, imageClass, initialClass) {
  const url = link.imageUrl || category.imageUrl;
  if (url) {
    const img = el('img', imageClass);
    img.src = url;
    img.alt = '';
    img.loading = 'lazy';
    return img;
  }
  return el('div', initialClass, link.title.slice(0, 1).toUpperCase());
}

function featureCard(link, category) {
  const card = el('div', 'feature' + (link.visible ? '' : ' is-hidden'));
  card.append(artwork(link, category, 'feature-image', 'feature-initial'));

  const body = el('div', 'feature-body');
  const title = el('div', 'feature-title', link.title);
  if (!link.visible) {
    title.append(el('span', 'hidden-badge', 'Hidden'));
  }
  body.append(title);
  if (link.description) {
    body.append(el('div', 'feature-description', link.description));
  }
  const go = el('a', 'button');
  go.href = link.url;
  go.target = '_blank';
  go.rel = 'noopener';
  go.append(el('span', '', 'Open App'), svg('go'));
  body.append(go);
  card.append(body, moreMenu(link));
  return card;
}

function tile(link, category) {
  const card = el('div', 'tile' + (link.visible ? '' : ' is-hidden'));
  // The anchor holds only the content; the overflow button is a sibling, since
  // a button nested inside an anchor is invalid and swallows its own clicks.
  const open = el('a', 'tile-link');
  open.href = link.url;
  open.target = '_blank';
  open.rel = 'noopener';
  open.append(artwork(link, category, 'tile-image', 'tile-initial'));
  open.append(el('div', 'tile-title', link.title));
  if (!link.visible) {
    open.append(el('span', 'hidden-badge', 'Hidden'));
  }
  card.append(open, moreMenu(link));
  return card;
}

// The sheet's Style column decides the shape of each section; the loader has
// already refused anything that is not one of these two.
function panel(category, links) {
  const cards = category.style === 'cards';
  const grid = el('div', cards ? 'card-grid' : 'tile-grid');
  for (const link of links) {
    grid.append(cards ? featureCard(link, category) : tile(link, category));
  }
  return grid;
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

export function renderCategories(query = '') {
  const root = document.querySelector('#categories');
  const needle = query.trim().toLowerCase();
  root.replaceChildren();
  let shown = 0;
  for (const category of state.model.categories) {
    const links = category.links.filter(link => matches(link, needle));
    // A search hides empty sections outright; without one, an admin still sees
    // an empty category so it can be edited or filled.
    if (!links.length && (needle || !isAdmin())) {
      continue;
    }
    shown += links.length;
    const section = el('section', 'category');
    section.id = anchorFor(category.title);
    const title = el('h2', 'category-title', category.title);
    if (isAdmin()) {
      const edit = el('button', 'category-edit');
      edit.type = 'button';
      edit.title = 'Edit category';
      edit.setAttribute('aria-label', `Edit the ${category.title} category`);
      edit.append(svg('edit'));
      edit.addEventListener('click', () => openCategoryEditor(category));
      title.append(edit);
    }
    section.append(title);
    section.append(links.length ? panel(category, links) : el('div', 'category-empty', 'No links yet.'));
    root.append(section);
  }
  const empty = document.querySelector('#empty-search');
  empty.hidden = Boolean(shown) || !needle;
  if (!state.model.categories.length) {
    root.append(el('div', 'footnote', 'Nothing here yet.'));
  }
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
    // The category's own image is what distinguishes it everywhere else on the
    // page, so the nav uses it too; the generic mark only stands in when the
    // sheet has not given the category a picture.
    if (category.imageUrl) {
      const img = el('img', 'app-nav-image');
      img.src = category.imageUrl;
      img.alt = '';
      img.loading = 'lazy';
      item.append(img);
    } else {
      item.append(svg('section'));
    }
    item.append(el('span', '', category.title));
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
