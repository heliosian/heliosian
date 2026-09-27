import {state, isAdmin, tagLabelsOf} from './state.js';
import {el, svg, iconOf, toast} from './dom.js';
import {openLinkEditor, openCategoryEditor, openAppEditor, moveApp, moveLink} from './edit.js';
import {appOrigin} from '/toolbar.js';
import {api} from '/api.js';

function editPencil(link, category) {
  if (!isAdmin()) {
    return null;
  }
  const tools = el('div', 'app-tools');
  const siblings = category ? category.links.filter(listed) : [];
  const at = siblings.indexOf(link);
  for (const [by, glyph, words] of [[-1, '\u2039', 'Move earlier'], [1, '\u203a', 'Move later']]) {
    const b = el('button', 'link-edit app-move', glyph);
    b.type = 'button';
    b.title = words;
    b.setAttribute('aria-label', `${words}: ${link.title}`);
    b.disabled = by < 0 ? at <= 0 : at < 0 || at >= siblings.length - 1;
    b.addEventListener('click', e => {
      e.stopPropagation();
      e.preventDefault();
      moveLink(link.title, by);
    });
    tools.append(b);
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
  tools.append(pencil);
  return tools;
}

function categoryGlyph(category, className) {
  const wrap = el('span', className);
  wrap.append(svg(iconOf(category)));
  return wrap;
}

function artwork(link, category, imageClass, initialClass) {
  if (link.imageUrl) {
    const img = el('img', imageClass);
    img.src = link.imageUrl;
    img.alt = '';
    img.loading = 'lazy';
    return img;
  }
  const mark = el('div', initialClass + ' is-icon');
  mark.append(svg(iconOf(category)));
  return mark;
}

function openInNewTab(url) {
  const a = el('a');
  a.href = url;
  a.target = '_blank';
  a.rel = 'noopener';
  return a;
}

function featureCard(link, category) {
  const slot = el('div', 'chip-slot');
  const card = openInNewTab(link.url);
  card.className = 'chip' + (link.visible ? '' : ' is-hidden');
  const disc = el('div', 'chip-disc');
  disc.append(artwork(link, category, 'chip-image', 'chip-initial'));
  card.append(disc);
  const title = el('div', 'chip-title', link.title);
  title.append(...badges(link));
  card.append(title);
  if (link.description) {
    card.append(el('div', 'chip-description', link.description));
  }
  const go = el('span', 'button chip-open');
  go.append(el('span', '', 'Open App'), svg('arrow'));
  card.append(go);
  slot.append(card);
  const pencil = editPencil(link, category);
  if (pencil) {
    slot.append(pencil);
  }
  return slot;
}

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
  title.append(...badges(link));
  body.append(title);
  if (link.description) {
    body.append(el('div', 'tile-description', link.description));
  }
  open.append(body);
  card.append(open);
  const pencil = editPencil(link, category);
  if (pencil) {
    card.append(pencil);
  }
  return card;
}

function singular(title) {
  const t = title.trim();
  return /^[A-Z][a-z]+s$/.test(t) ? t.slice(0, -1) : 'Link';
}

function addCard(category, cards) {
  const card = el('button', cards ? 'chip add-card' : 'tile add-card');
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

function panel(category, links, needle) {
  const cards = category.style === 'cards';
  const wrap = el('div');
  const grid = el('div', cards ? 'chip-grid' : 'tile-grid');
  const {shown, hidden} = limited(category, links, needle);
  for (const link of shown) {
    grid.append(cards ? featureCard(link, category) : tile(link, category));
  }
  if (isAdmin()) {
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

function listed(link) {
  return (link.visible && link.forMe !== false) || isAdmin();
}

function sectionListed(category) {
  return category.style !== 'events' && (category.forMe !== false || isAdmin());
}

export function audienceWords(rules) {
  const parts = (rules || []).map(r => {
    const bits = [...(r.roles || []), ...(r.grades || []), ...(r.classrooms || []), ...tagLabelsOf(r)];
    if (r.search) {
      bits.push(`“${r.search}”`);
    }
    return (r.kind === 'exclude' ? 'not ' : '') + bits.join(' · ');
  });
  const words = parts.join(' / ');
  return words.length > 40 ? `${rules.length} ${rules.length === 1 ? 'rule' : 'rules'}` : words;
}

function badges(link) {
  const out = [];
  if (link.visible === false) {
    out.push(el('span', 'hidden-badge', 'Hidden'));
  }
  if ((link.rules || []).length && isAdmin()) {
    out.push(el('span', 'hidden-badge audience-badge', audienceWords(link.rules)));
  }
  return out;
}

export function renderCategories(query = '') {
  const root = document.querySelector('#categories');
  const needle = query.trim().toLowerCase();
  root.replaceChildren();
  let shown = 0;
  for (const category of state.model.categories) {
    if (!sectionListed(category)) {
      continue;
    }
    const apps = category.style === 'apps';
    const links = apps ? [] : category.links.filter(link => listed(link) && matches(link, needle));
    const count = apps ? appsMatching(needle).length : links.length;
    if (!count && (needle || !isAdmin())) {
      continue;
    }
    shown += count;
    const section = el('section', 'category' + (category.style === 'tiles' ? ' is-compact' : ''));
    section.id = anchorFor(category.title);
    const head = el('div', 'category-head');
    const title = el('h2', 'category-title', category.title);
    title.append(...badges(category));
    head.append(title);
    if (isAdmin()) {
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
    section.append(apps ? appsPanel(category, needle) : panel(category, links, needle));
    root.append(section);
  }
  const empty = document.querySelector('#empty-search');
  empty.hidden = Boolean(shown) || !needle;
  if (!state.model.categories.length) {
    root.append(el('div', 'footnote', 'Nothing here yet.'));
  }
}

export function rsvpButtons(event) {
  const rsvp = el('div', 'event-rsvp');
  if (event.linkApp === 'celebrate') {
    const held = (event.people || []).some(p => p.note !== 'waitlisted');
    if (held) {
      const send = el('button', 'event-invite');
      send.type = 'button';
      const label = () => {
        send.replaceChildren(svg(event.answer === 'yes' ? 'check' : 'calendarAdd'), el('span', '', event.answer === 'yes' ? 'Invite sent' : 'Add to My Calendar'));
        send.classList.toggle('is-sent', event.answer === 'yes');
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
      add.href = appOrigin(event.linkApp) + event.link;
      add.append(svg('ticket'), el('span', '', live ? 'Add Ticket' : event.call || 'See the party'));
      rsvp.append(add);
    }
    return rsvp;
  }
  let changing = false;
  const render = () => {
    rsvp.replaceChildren();
    if ((event.answer === 'yes' || event.answer === 'no' || event.answer === 'maybe') && !changing) {
      const said = el('div', 'event-said');
      said.append(el('span', '', 'You said '), el('strong', '', event.answer === 'yes' ? 'Yes' : event.answer === 'no' ? 'No' : 'Maybe'));
      const change = el('button', 'event-said-change');
      change.type = 'button';
      change.textContent = 'Change';
      change.addEventListener('click', () => {
        changing = true;
        render();
      });
      said.append(change);
      rsvp.append(said);
      return;
    }
    const yes = el('button', 'button button-small event-yes');
    yes.type = 'button';
    yes.append(svg('check'), el('span', '', 'Yes'));
    const no = el('button', 'button button-small event-no');
    no.type = 'button';
    no.append(svg('close'), el('span', '', 'No'));
    yes.classList.toggle('button-secondary', event.answer !== 'yes');
    no.classList.toggle('button-secondary', event.answer !== 'no');
    yes.title = 'Yes, and send me a calendar invite';
    const say = async word => {
      const next = event.answer === word ? '' : word;
      if (await answer(event, next)) {
        event.answer = next;
        changing = false;
        render();
      }
    };
    yes.addEventListener('click', () => say('yes'));
    no.addEventListener('click', () => say('no'));
    rsvp.append(yes, no);
  };
  render();
  return rsvp;
}

export function calendarMark(c) {
  return c.emoji ? el('span', 'category-calendar-emoji', c.emoji) : svg('calendar');
}

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

async function answer(event, word) {
  try {
    await api('POST', '/api/apps/rsvp', {id: event.id, answer: word});
  } catch (err) {
    toast(err.message);
    return false;
  }
  return true;
}

function appsMatching(needle) {
  return (state.model.apps || []).filter(a => sectionListed(a) && (!needle || `${a.name} ${a.tagline}`.toLowerCase().includes(needle)));
}

function appBadge(app) {
  const v = app.visibility;
  if (!isAdmin() || !v || v.visibility !== 'list') {
    return null;
  }
  const who = [];
  if ((v.rules || []).length) {
    who.push(audienceWords(v.rules));
  }
  const named = (v.emails || []).length;
  if (named) {
    who.push(named === 1 ? '1 person' : named + ' people');
  }
  return el('span', 'hidden-badge audience-badge', who.length ? who.join(' · ') : 'Nobody');
}

function appCard(app) {
  const slot = el('div', 'chip-slot');
  const card = el('a', 'chip');
  card.href = appOrigin(app.key);
  const disc = el('div', 'chip-disc');
  const icon = el('img', 'chip-image');
  icon.src = `/brand/apps/${app.key}.png` + (app.mark ? `?v=${app.mark}` : '');
  icon.alt = '';
  disc.append(icon);
  card.append(disc);
  const title = el('div', 'chip-title', app.name);
  const badge = appBadge(app);
  if (badge) {
    title.append(badge);
  }
  card.append(title);
  card.append(el('div', 'chip-description', app.tagline));
  const go = el('span', 'button chip-open');
  go.append(el('span', '', 'Open App'), svg('arrow'));
  card.append(go);
  slot.append(card);
  if (isAdmin() && app.visibility) {
    const tools = el('div', 'app-tools');
    const keys = (state.model.apps || []).map(a => a.key);
    const at = keys.indexOf(app.key);
    for (const [by, glyph, words] of [[-1, '\u2039', 'Move earlier'], [1, '\u203a', 'Move later']]) {
      const b = el('button', 'link-edit app-move', glyph);
      b.type = 'button';
      b.title = words;
      b.setAttribute('aria-label', `${words}: ${app.name}`);
      b.disabled = by < 0 ? at <= 0 : at >= keys.length - 1;
      b.addEventListener('click', e => {
        e.stopPropagation();
        e.preventDefault();
        moveApp(app.key, by);
      });
      tools.append(b);
    }
    const pencil = el('button', 'link-edit');
    pencil.type = 'button';
    pencil.title = 'Edit app';
    pencil.setAttribute('aria-label', `Edit ${app.name}`);
    pencil.append(svg('edit'));
    pencil.addEventListener('click', e => {
      e.stopPropagation();
      e.preventDefault();
      openAppEditor(app);
    });
    tools.append(pencil);
    slot.append(tools);
  }
  return slot;
}

function appsPanel(category, needle) {
  const apps = appsMatching(needle);
  if (!apps.length) {
    return el('div', 'category-empty', needle ? 'No apps match.' : 'No apps to show.');
  }
  const wrap = el('div');
  const grid = el('div', 'chip-grid');
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

function hasSomething(category) {
  if (!sectionListed(category)) {
    return false;
  }
  if (isAdmin()) {
    return true;
  }
  if (category.style === 'apps') {
    return appsMatching('').length > 0;
  }
  return category.links.some(listed);
}

export function renderNav() {
  const nav = document.querySelector('#app-nav');
  nav.replaceChildren();
  let first = true;
  for (const category of state.model.categories.filter(hasSomething)) {
    const item = el('a', first ? 'is-active' : '');
    item.href = '#' + anchorFor(category.title);
    const glyph = first ? el('span', 'app-nav-glyph') : categoryGlyph(category, 'app-nav-glyph');
    if (first) {
      glyph.append(el('span', 'app-symbol'));
    }
    item.append(glyph, el('span', '', category.title));
    nav.append(item);
    first = false;
  }
}

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

