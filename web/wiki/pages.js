import {state, page, pagePath, segmentOf, editPath, childrenOf, pageCount, hasChildren, trail, under, mine, sidesOf, body, save, remove, picturePath, splitHeader, joinHeader, setHeader, firstSentence, headerFor, listed, setHidden} from './state.js';
import {card} from '/cardgrid.js';
import {el, svg, link, button, toast, imageThumb} from '/elements.js';
import {setTitle, setSearch} from '/shell.js';
import {navigate, load, notFound} from '/router.js';
import {render} from '/markdown.js';
import {imageTools} from '/images.js';
import {detailHero, heroImageBar} from '/heroimage.js';
import {openModal, closeModal} from '/modal.js';
import {field, text as textInput} from '/form.js';

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

function wikiCard(p, md, imageUrl, withPath) {
  const count = pageCount(p.id);
  const open = link(pagePath(p), 'button button-secondary button-small', 'Open');
  const yours = el('span', 'card-chip card-chip-mine');
  yours.append(svg('star'), el('span', '', 'Yours'));
  const chips = p.hidden ? [el('span', 'card-chip card-chip-hidden', 'Hidden')] : [];
  if (count) {
    chips.push(`${count} ${count === 1 ? 'page' : 'pages'}`);
  }
  return card({
    href: pagePath(p),
    imageUrl,
    title: p.name,
    subtitle: withPath ? pathWords(p) : '',
    text: firstSentence(md) || childrenOf(p.id).map(c => c.name).join(' · '),
    media: mine(p) ? [yours] : [],
    chips,
    foot: [open],
  });
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
  out.append(head('Wiki', actions));
  const list = el('div');
  out.append(list);
  let asked = 0;
  const show = async q => {
    const ask = ++asked;
    const shown = q ? state.pages.filter(p => listed(p) && p.name.toLowerCase().includes(q)).sort((a, b) => a.name.localeCompare(b.name)) : childrenOf('');
    if (!shown.length) {
      list.replaceChildren(el('p', 'panel-empty', state.pages.length ? 'No page has that in its title.' : 'No pages yet. Start the first one.'));
      return;
    }
    try {
      const [texts, images] = await Promise.all([Promise.all(shown.map(p => body(p))), Promise.all(shown.map(headerFor))]);
      if (ask !== asked) {
        return;
      }
      const grid = el('div', 'card-grid');
      grid.append(...shown.map((p, i) => wikiCard(p, texts[i], images[i], Boolean(q))));
      list.replaceChildren(grid);
    } catch (err) {
      list.replaceChildren(el('p', 'panel-empty', err.message));
    }
  };
  show('');
  setSearch('Search pages…', show);
  return out;
}

const tools = imageTools('/api/wiki', {state: {model: {imageSearch: true}}});

function headerSaver(p, hero) {
  return async name => {
    hero.querySelector('.hero-image-bar').replaceChildren(el('span', 'hero-image-status', 'Saving…'));
    try {
      await setHeader(p.id, name ? picturePath + name.split('/').pop() : '');
      await load();
      toast(name ? 'Header image saved' : 'Header image removed');
    } catch (err) {
      toast(err.message);
      await load();
    }
  };
}

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

function sectionList(children) {
  const list = el('nav', 'wiki-outline');
  list.append(...children.map(c => link(pagePath(c), 'wiki-outline-item', c.name)));
  return list;
}

function sectionCards(children) {
  const list = el('nav', 'wiki-list');
  for (const c of children) {
    const row = link(pagePath(c), 'wiki-card');
    const icon = el('span', 'wiki-card-icon');
    const count = pageCount(c.id);
    icon.append(svg(count ? 'book' : 'doc'));
    row.append(icon, el('span', 'wiki-card-title', c.name));
    if (count) {
      row.append(el('span', 'wiki-card-count', `${count} ${count === 1 ? 'page' : 'pages'}`));
    }
    list.append(row);
  }
  return list;
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
  if (mine(p) || state.admin) {
    const visibility = button(p.hidden ? 'Hidden' : 'Public', p.hidden ? 'eye-off' : 'eye', 'button button-small button-secondary', async () => {
      const hide = !p.hidden;
      try {
        await setHidden(p.id, hide);
        await load();
        toast(hide ? 'Hidden: kept out of the lists of pages' : 'Public: in the lists of pages');
      } catch (err) {
        toast(err.message);
      }
    });
    visibility.title = p.hidden ? 'Hidden: click to make it public' : 'Public: click to hide it from the lists';
    actions.append(visibility);
  }
  actions.append(add, edit);
  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const side = el('div', 'detail-side');
  const contents = el('div', 'side-card');
  contents.hidden = true;
  side.append(contents);
  const section = el('div', 'side-card');
  section.hidden = !children.length;
  section.append(el('div', 'side-card-title', 'Pages in this section'), sectionList(children));
  side.append(section);
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
  const hero = el('div');
  const md = body(p);
  md.then(md => {
    const {text} = splitHeader(md);
    if (!text.trim() && children.length) {
      content.replaceChildren(el('h2', 'wiki-section', 'Pages in this section'), sectionCards(children));
      return;
    }
    content.replaceChildren(text.trim() ? render(text) : el('p', 'panel-empty', 'This page is empty. Edit it to add something.'));
    outline(content, contents);
  }).catch(err => {
    content.replaceChildren(el('p', 'panel-empty', err.message));
  });
  Promise.all([md, headerFor(p)]).then(([md, imageUrl]) => {
    const edit = {image: splitHeader(md).header, tools, save: headerSaver(p, hero)};
    hero.replaceChildren(detailHero({imageUrl, title: p.name, path: pagePath(p), edit}));
  }).catch(err => toast(err.message));
  cols.append(main, side);
  out.append(top, hero, cols);
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

function replaceRange(textarea, start, end, text, selectStart, selectEnd) {
  textarea.focus();
  textarea.setSelectionRange(start, end);
  document.execCommand('insertText', false, text);
  textarea.setSelectionRange(start + selectStart, start + selectEnd);
}

function wrapSelection(textarea, mark) {
  const {selectionStart: start, selectionEnd: end, value} = textarea;
  const words = value.slice(start, end);
  replaceRange(textarea, start, end, mark + words + mark, mark.length, mark.length + words.length);
}

function selectedLines(textarea) {
  const {selectionStart, selectionEnd, value} = textarea;
  const start = value.lastIndexOf('\n', selectionStart - 1) + 1;
  const last = selectionEnd > selectionStart && value[selectionEnd - 1] === '\n' ? selectionEnd - 1 : selectionEnd;
  const newline = value.indexOf('\n', last);
  const end = newline === -1 ? value.length : newline;
  return {start, end, lines: value.slice(start, end).split('\n')};
}

function prefixLines(textarea, first, prefix, blank) {
  const {start, end, lines} = selectedLines(textarea);
  const text = first + (lines.length === 1 && !lines[0] ? prefix : lines.map(line => line ? prefix + line : blank).join('\n'));
  replaceRange(textarea, start, end, text, text.length, text.length);
}

const listItem = /^\s*([-*]|\d+[.)])\s+/;

function indentLists(textarea) {
  textarea.addEventListener('keydown', e => {
    if (e.key !== 'Tab' || e.ctrlKey || e.metaKey || e.altKey) {
      return;
    }
    const {start, end, lines} = selectedLines(textarea);
    if (!lines.some(line => listItem.test(line))) {
      return;
    }
    e.preventDefault();
    const shifted = lines.map(line => {
      if (!listItem.test(line)) {
        return line;
      }
      return e.shiftKey ? line.replace(/^( {1,2}|\t)/, '') : '  ' + line;
    });
    const text = shifted.join('\n');
    if (lines.length === 1 && textarea.selectionStart === textarea.selectionEnd) {
      const caret = Math.max(0, textarea.selectionStart - start + text.length - lines[0].length);
      replaceRange(textarea, start, end, text, caret, caret);
      return;
    }
    replaceRange(textarea, start, end, text, 0, text.length);
  });
}

function linkPopup(textarea) {
  const {selectionStart: start, selectionEnd: end, value} = textarea;
  const words = textInput(value.slice(start, end), {placeholder: 'What the link says', required: true});
  const address = textInput('', {type: 'url', placeholder: 'https://', required: true});
  openModal('Add Link', [field('Words', words), field('Address', address)], {
    saveLabel: 'Add Link',
    stay: true,
    submit: async () => {
      const markdown = `[${words.value.trim()}](${address.value.trim()})`;
      closeModal();
      replaceRange(textarea, start, end, markdown, markdown.length, markdown.length);
    },
  });
  if (words.value) {
    address.focus();
  }
}

function textToolbar(textarea) {
  const tool = (icon, label, onClick) => {
    const node = button('', icon, 'icon-button', onClick);
    node.setAttribute('aria-label', label);
    node.title = label;
    return node;
  };
  const callout = el('select', 'wiki-callout-pick');
  callout.setAttribute('aria-label', 'Callout');
  callout.append(el('option', '', 'Callout'));
  for (const kind of ['Quote', 'Tip', 'Important', 'Warning', 'Caution', 'Question']) {
    const option = el('option', '', kind);
    option.value = kind.toUpperCase();
    callout.append(option);
  }
  const firsts = {QUOTE: '', QUESTION: '> [!QUESTION] Question?\n'};
  callout.addEventListener('change', () => {
    const kind = callout.value;
    callout.selectedIndex = 0;
    prefixLines(textarea, kind in firsts ? firsts[kind] : `> [!${kind}]\n`, '> ', '>');
  });
  const bar = el('div', 'wiki-text-tools');
  bar.append(
    tool('bold', 'Bold', () => wrapSelection(textarea, '**')),
    tool('italic', 'Italic', () => wrapSelection(textarea, '*')),
    tool('list', 'List', () => prefixLines(textarea, '', '- ', '')),
    tool('link', 'Link', () => linkPopup(textarea)),
    callout,
    imageInserter(textarea, 'Insert Image'),
  );
  return bar;
}

function headerEditor(query) {
  let header = '';
  const wrap = el('div', 'detail-hero wiki-header-edit');
  const draw = () => {
    const picture = header ? imageThumb(header, '', 'detail-hero-image') : el('div', 'wiki-header-empty', 'No header image');
    const save = async name => {
      header = name ? picturePath + name.split('/').pop() : '';
      draw();
    };
    wrap.replaceChildren(picture, heroImageBar({image: header, imageUrl: header, query: query(), tools, save}));
  };
  draw();
  return {
    wrap,
    set: value => {
      header = value;
      draw();
    },
    value: () => header,
  };
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
  slug.pattern = '[a-z0-9]+(-[a-z0-9]+)*';
  slug.maxLength = 40;
  slug.title = 'Lowercase letters, digits and single hyphens; left blank, the address follows the title';
  const host = el('span', 'wiki-slug-host');
  const showAddress = () => {
    host.textContent = `${location.host}${parent.value ? decodeURIComponent(pagePath(page(parent.value))) : '/p'}/`;
    slug.placeholder = segmentOf({slug: '', name: title.value, id: ''}) || 'its-address';
  };
  showAddress();
  parent.addEventListener('change', showAddress);
  title.addEventListener('input', showAddress);
  const address = el('label', 'wiki-parent-field');
  address.append(el('span', '', 'Address'), host, slug);
  const text = el('textarea', 'wiki-text');
  text.placeholder = 'Write the page here.';
  text.disabled = Boolean(p);
  indentLists(text);
  const hint = el('p', 'wiki-hint', 'Markdown works: # Heading, **bold**, *italic*, - a list (Tab and Shift+Tab indent and outdent its lines for sub-bullets), [words](https://a.link), > [!NOTE] (or TIP, IMPORTANT, WARNING, CAUTION) for a callout, > [!QUESTION] and the question, then > lines of answer, for a question and its answer; Insert Image puts a picture at the cursor.');
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
  const header = headerEditor(() => title.value);
  const main = el('div', 'wiki-editor-main');
  main.append(header.wrap, title, where);
  if (!p || mine(p) || state.admin) {
    main.append(address);
  }
  main.append(textToolbar(text), text, hint, bar);
  const side = el('div', 'detail-side wiki-editor-side');
  side.append(cards.wrap);
  form.append(main, side);
  out.append(form);
  if (p) {
    body(p).then(md => {
      const split = splitHeader(md);
      header.set(split.header);
      text.value = split.text;
      text.disabled = false;
    }).catch(err => toast(err.message));
  }
  form.addEventListener('submit', async e => {
    e.preventDefault();
    saveButton.disabled = true;
    try {
      await cards.ready;
      const saved = await save(p ? p.id : '', parent.value, slug.value.trim(), title.value, joinHeader(header.value(), text.value), cards.value());
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
