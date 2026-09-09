import {state, isAdmin} from './state.js';
import {el, svg, displayURL, toast} from './dom.js';
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

// A link without its own image borrows its category's, which is how a whole
// category of chats shares one mark; with neither, the title's initial stands in.
function linkImage(link, category) {
  const url = link.imageUrl || category.imageUrl;
  if (url) {
    const img = el('img', 'link-image');
    img.src = url;
    img.alt = '';
    img.loading = 'lazy';
    return img;
  }
  return el('div', 'link-initial', link.title.slice(0, 1).toUpperCase());
}

function linkCard(link, category) {
  const card = el('div', 'link-card' + (link.visible ? '' : ' is-hidden'));
  card.append(linkImage(link, category));

  const body = el('div', 'link-body');
  const label = el('div', 'link-label', link.title);
  if (!link.visible) {
    label.append(el('span', 'hidden-badge', 'Hidden'));
  }
  body.append(label);
  const urlLine = el('div', 'link-url');
  const anchor = el('a', '', displayURL(link.url));
  anchor.href = link.url;
  anchor.target = '_blank';
  anchor.rel = 'noopener';
  urlLine.append(anchor);
  body.append(urlLine);
  if (link.description) {
    body.append(el('div', 'link-description', link.description));
  }
  card.append(body);

  const actions = el('div', 'link-actions');
  const go = el('button', 'button');
  go.type = 'button';
  go.append(svg('go'), el('span', '', 'Go!'));
  go.addEventListener('click', () => openLink(link));
  const copy = el('button', 'button button-secondary');
  copy.type = 'button';
  copy.append(svg('copy'), el('span', '', 'Copy Link'));
  copy.addEventListener('click', () => copyLink(link));
  actions.append(go, copy);

  const wrap = el('div', 'more-wrap');
  const more = el('button', 'more-button');
  more.type = 'button';
  more.setAttribute('aria-label', 'More');
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
    const opening = menu.hidden;
    closeMenus();
    menu.hidden = !opening;
  });
  wrap.append(more, menu);
  actions.append(wrap);
  card.append(actions);
  return card;
}

export function renderCategories() {
  const root = document.querySelector('#categories');
  root.replaceChildren();
  for (const category of state.model.categories) {
    if (!category.links.length && !isAdmin()) {
      continue;
    }
    const title = el('h2', 'category-title', category.title);
    if (isAdmin()) {
      const edit = el('button', 'category-edit');
      edit.type = 'button';
      edit.title = 'Edit category';
      edit.append(svg('edit'));
      edit.addEventListener('click', () => openCategoryEditor(category));
      title.append(edit);
    }
    root.append(title);
    const panel = el('div', 'category-panel');
    if (!category.links.length) {
      panel.append(el('div', 'category-empty', 'No links yet.'));
    }
    for (const link of category.links) {
      panel.append(linkCard(link, category));
    }
    root.append(panel);
  }
  if (!state.model.categories.length) {
    root.append(el('div', 'footnote', 'Nothing here yet.'));
  }
}

document.addEventListener('click', closeMenus);
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    closeMenus();
  }
});
