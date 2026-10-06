import {state, page, pagePath, editPath, childrenOf, hasChildren, trail, under, mine, sidesOf, body, save, remove} from './state.js';
import {el, svg, link, button, toast} from '/elements.js';
import {setTitle, setSearch} from '/shell.js';
import {navigate, load, notFound} from '/router.js';
import {render} from '/markdown.js';
import {imageTools} from '/images.js';

function head(title, actions) {
  const top = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', title));
  top.append(main);
  if (actions) {
    actions.classList.add('page-actions');
    top.append(actions);
  }
  return top;
}

function pathWords(p) {
  return trail(p).map(a => a.name).join(' › ');
}

function pageCard(p, withPath) {
  const card = link(pagePath(p), 'wiki-card');
  const icon = el('span', 'wiki-card-icon');
  icon.append(svg(childrenOf(p.id).length ? 'book' : 'doc'));
  const words = el('span', 'wiki-card-words');
  words.append(el('span', 'wiki-card-title', p.name));
  const path = withPath && pathWords(p);
  if (path) {
    words.append(el('span', 'wiki-card-path', path));
  }
  card.append(icon, words);
  const count = childrenOf(p.id).length;
  if (count) {
    card.append(el('span', 'wiki-card-count', `${count} ${count === 1 ? 'page' : 'pages'}`));
  }
  return card;
}

function crumbs(p) {
  const nav = el('nav', 'wiki-crumbs');
  nav.append(link('/', '', 'Wiki'));
  for (const a of trail(p)) {
    nav.append(el('span', 'wiki-crumb-sep', '›'), link(pagePath(a), '', a.name));
  }
  return nav;
}

export function listPage() {
  setTitle('Helios Wiki');
  const out = el('div');
  const make = link('/new', 'button');
  make.append(svg('plus'), el('span', '', 'New Page'));
  const actions = el('div');
  actions.append(make);
  out.append(head('Helios Wiki', actions), el('p', 'page-lead', 'Parent-to-parent info: what families have learned, written down for the next ones.'));
  const list = el('div', 'wiki-list');
  out.append(list);
  const show = q => {
    const shown = q ? state.pages.filter(p => p.name.toLowerCase().includes(q)).sort((a, b) => a.name.localeCompare(b.name)) : childrenOf('');
    list.replaceChildren(...shown.map(p => pageCard(p, Boolean(q))));
    if (!shown.length) {
      list.append(el('p', 'panel-empty', state.pages.length ? 'No page has that in its title.' : 'No pages yet. Start the first one.'));
    }
  };
  show('');
  setSearch('Search pages…', show);
  return out;
}

const tools = imageTools('/api/wiki', {state: {}});

function anchorFor(text, taken) {
  const base = text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'section';
  let id = base;
  for (let n = 2; taken.has(id); n++) {
    id = `${base}-${n}`;
  }
  taken.add(id);
  return id;
}

function outline(content, card) {
  const headings = [...content.querySelectorAll('h3')];
  card.hidden = !headings.length;
  const list = el('nav', 'wiki-outline');
  const taken = new Set();
  for (const h of headings) {
    h.id = anchorFor(h.textContent, taken);
    const a = el('a', 'wiki-outline-item', h.textContent);
    a.href = `#${h.id}`;
    a.addEventListener('click', e => {
      e.preventDefault();
      h.scrollIntoView({behavior: 'smooth', block: 'start'});
    });
    list.append(a);
  }
  card.replaceChildren(el('div', 'side-card-title', 'On this page'), list);
}

async function deletePage(p) {
  if (hasChildren(p)) {
    toast('Move or delete its sub-pages first.');
    return;
  }
  if (!confirm(`Delete “${p.name}”? This can’t be undone.`)) {
    return;
  }
  try {
    await remove(p);
    await load();
    navigate(p.parent ? pagePath(page(p.parent)) : '/');
    toast('Page deleted');
  } catch (err) {
    toast(err.message);
  }
}

export function viewPage(id) {
  const p = page(id);
  if (!p) {
    return notFound('That page');
  }
  setTitle(p.name);
  const out = el('div', 'wiki-page');
  const top = el('div', 'detail-top');
  const actions = el('div', 'detail-tools');
  const edit = link(editPath(p), 'button button-small button-secondary');
  edit.append(svg('edit'), el('span', '', 'Edit Page'));
  const add = link(`/new?parent=${p.id}`, 'button button-small button-secondary');
  add.append(svg('plus'), el('span', '', 'Add Sub-Page'));
  top.append(crumbs(p), actions);
  const children = childrenOf(p.id);
  actions.append(add, edit);
  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const side = el('div', 'detail-side');
  const contents = el('div', 'side-card');
  contents.hidden = true;
  side.append(contents);
  for (const card of sidesOf(p)) {
    const box = el('div', 'side-card');
    const text = el('div', 'wiki-body side-card-body');
    box.append(el('div', 'side-card-title', card.name), text);
    side.append(box);
    body(card).then(md => text.replaceChildren(render(md))).catch(err => text.replaceChildren(el('p', 'panel-empty', err.message)));
  }
  main.append(el('h1', 'detail-title', p.name));
  const content = el('article', 'wiki-body');
  content.append(el('p', 'panel-empty', 'Loading…'));
  main.append(content);
  body(p).then(text => {
    content.replaceChildren(text.trim() ? render(text) : el('p', 'panel-empty', children.length ? 'This page holds the pages below.' : 'This page is empty. Edit it to add something.'));
    outline(content, contents);
  }).catch(err => {
    content.replaceChildren(el('p', 'panel-empty', err.message));
  });
  if (children.length) {
    const list = el('div', 'wiki-list');
    list.append(...children.map(c => pageCard(c, false)));
    main.append(el('h2', 'wiki-section', 'Pages in this section'), list);
  }
  cols.append(main, side);
  out.append(top, cols);
  return out;
}

function parentPicker(p, chosen) {
  const select = el('select', 'wiki-parent');
  const top = el('option', '', 'Top level');
  top.value = '';
  select.append(top);
  const add = (parent, depth) => {
    for (const c of childrenOf(parent)) {
      if (p && (c === p || under(c, p))) {
        continue;
      }
      const option = el('option', '', `${'   '.repeat(depth + 1)}${c.name}`);
      option.value = c.id;
      select.append(option);
      add(c.id, depth + 1);
    }
  };
  add('', 0);
  select.value = chosen;
  return select;
}

function imageInserter(textarea, label) {
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  const insert = button(label, 'image', label ? 'button button-small button-secondary' : 'icon-button', () => file.click());
  insert.setAttribute('aria-label', 'Insert image');
  file.addEventListener('change', async () => {
    const picked = file.files[0];
    file.value = '';
    if (!picked) {
      return;
    }
    insert.disabled = true;
    try {
      const {name} = await tools.uploadImage(picked);
      const markdown = `![](/api/wiki/picture/${name.split('/').pop()})`;
      const at = textarea.selectionStart;
      const before = textarea.value.slice(0, at);
      const after = textarea.value.slice(textarea.selectionEnd);
      const lead = before && !before.endsWith('\n') ? '\n' : '';
      const tail = after.startsWith('\n') ? '' : '\n';
      textarea.value = before + lead + markdown + tail + after;
      textarea.focus();
      textarea.selectionStart = textarea.selectionEnd = (before + lead + markdown).length;
    } catch (err) {
      toast(err.message);
    }
    insert.disabled = false;
  });
  const wrap = el('span', 'wiki-insert');
  wrap.append(insert, file);
  return wrap;
}

function cardsEditor(p) {
  const cards = [];
  const wrap = el('div', 'wiki-cards');
  const list = el('div', 'wiki-cards-list');
  const draw = () => {
    list.replaceChildren(...cards.map((card, i) => {
      const row = el('div', 'wiki-card-edit');
      const top = el('div', 'wiki-card-edit-top');
      const tools = el('div', 'wiki-card-edit-tools');
      const up = button('', 'up', 'icon-button', () => {
        cards.splice(i - 1, 0, cards.splice(i, 1)[0]);
        draw();
      });
      up.disabled = i === 0;
      up.setAttribute('aria-label', 'Move up');
      const down = button('', 'down', 'icon-button', () => {
        cards.splice(i + 1, 0, cards.splice(i, 1)[0]);
        draw();
      });
      down.disabled = i === cards.length - 1;
      down.setAttribute('aria-label', 'Move down');
      const drop = button('', 'trash', 'icon-button', () => {
        cards.splice(i, 1);
        draw();
      });
      drop.setAttribute('aria-label', 'Remove card');
      tools.append(imageInserter(card.text, ''), up, down, drop);
      top.append(card.title, tools);
      row.append(top, card.text);
      return row;
    }));
  };
  const addCard = (id, name, md) => {
    const title = el('input', 'wiki-card-title-input');
    title.placeholder = 'Card title';
    title.value = name;
    const text = el('textarea', 'wiki-card-text');
    text.placeholder = 'What the card says, in Markdown.';
    text.value = md;
    cards.push({id, title, text});
  };
  const more = button('Add Card', 'plus', 'button button-small button-secondary', () => {
    addCard('', '', '');
    draw();
    cards[cards.length - 1].title.focus();
  });
  more.type = 'button';
  wrap.append(el('div', 'wiki-cards-head', 'Side Cards'), el('p', 'wiki-hint', 'Cards down the right of the page, under On this page, in this order.'), list, more);
  const existing = p ? sidesOf(p) : [];
  const ready = Promise.all(existing.map(card => body(card))).then(texts => {
    existing.forEach((card, i) => addCard(card.id, card.name, texts[i]));
    draw();
  });
  const value = () => cards
    .map(card => ({id: card.id, name: card.title.value, body: card.text.value}))
    .filter(card => card.name.trim() || card.body.trim());
  return {wrap, ready, value};
}

export function editPage(id) {
  const p = id ? page(id) : null;
  if (id && !p) {
    return notFound('That page');
  }
  const asked = new URLSearchParams(location.search).get('parent') || '';
  const parentId = p ? p.parent : (page(asked) ? asked : '');
  setTitle(p ? `Edit ${p.name}` : 'New Page');
  const out = el('div');
  out.append(head(p ? 'Edit Page' : 'New Page'));
  const form = el('form', 'wiki-editor');
  const title = el('input', 'wiki-title-input');
  title.placeholder = 'Title';
  title.required = true;
  title.value = p ? p.name : '';
  const parent = parentPicker(p, parentId);
  const where = el('label', 'wiki-parent-field');
  where.append(el('span', '', 'Under'), parent);
  const slug = el('input', 'wiki-slug');
  slug.value = p ? p.slug : '';
  slug.placeholder = 'its-address';
  slug.pattern = '[a-z0-9]+(-[a-z0-9]+)*';
  slug.maxLength = 40;
  slug.title = 'Lowercase letters, digits and single hyphens';
  const address = el('label', 'wiki-parent-field');
  address.append(el('span', '', 'Address'), el('span', 'wiki-slug-host', `${location.host}/p/`), slug);
  const text = el('textarea', 'wiki-text');
  text.placeholder = 'Write the page here.';
  text.disabled = Boolean(p);
  const hint = el('p', 'wiki-hint', 'Markdown works: # Heading, **bold**, *italic*, - a list, [words](https://a.link); Insert Image puts a picture at the cursor.');
  const saveButton = el('button', 'button', 'Save');
  saveButton.type = 'submit';
  const back = p ? pagePath(p) : (parentId ? pagePath(page(parentId)) : '/');
  const cancel = link(back, 'button button-secondary', 'Cancel');
  const bar = el('div', 'wiki-actions');
  bar.append(saveButton, cancel);
  if (p && (mine(p) || state.admin)) {
    bar.append(button('Delete', 'trash', 'button button-secondary wiki-delete', () => deletePage(p)));
  }
  const cards = cardsEditor(p);
  const main = el('div', 'wiki-editor-main');
  const textTools = el('div', 'wiki-text-tools');
  textTools.append(hint, imageInserter(text, 'Insert Image'));
  main.append(title, where);
  if (!p || mine(p) || state.admin) {
    main.append(address);
  }
  main.append(text, textTools, bar);
  const side = el('div', 'detail-side wiki-editor-side');
  side.append(cards.wrap);
  form.append(main, side);
  out.append(form);
  if (p) {
    body(p).then(t => {
      text.value = t;
      text.disabled = false;
    }).catch(err => toast(err.message));
  }
  form.addEventListener('submit', async e => {
    e.preventDefault();
    saveButton.disabled = true;
    try {
      await cards.ready;
      const saved = await save(p ? p.id : '', parent.value, slug.value.trim(), title.value, text.value, cards.value());
      await load();
      navigate(pagePath(page(saved.result[0])));
      toast('Saved');
    } catch (err) {
      saveButton.disabled = false;
      toast(err.message);
    }
  });
  setTimeout(() => (p ? text : title).focus());
  return out;
}
