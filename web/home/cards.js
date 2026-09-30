import {state, isAdmin, tagLabelsOf} from './state.js';
import {iconOf, categoryMark} from './dom.js';
import {el, svg, toast} from '/elements.js';
import {appOrigin} from '/appswitch.js';
import {act} from '/data.js';

function categoryGlyph(category, className) {
  const wrap = el('span', className);
  wrap.append(categoryMark(iconOf(category)));
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
  mark.append(categoryMark(iconOf(category)));
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
  return card;
}

const expanded = new Set();

function limited(category, items, needle) {
  if (!category.max || needle || expanded.has(category.id) || items.length <= category.max) {
    return {shown: items, hidden: 0};
  }
  return {shown: items.slice(0, category.max), hidden: items.length - category.max};
}

function seeMore(category, hidden, rerender) {
  const button = el('button', 'button button-secondary see-more');
  button.type = 'button';
  button.append(el('span', '', `See more (${hidden})`), svg('chevron-right'));
  button.addEventListener('click', () => {
    expanded.add(category.id);
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

function sectionListed(category) {
  return category.style !== 'events';
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

export function shownTo(rules, emails = []) {
  const who = [];
  if ((rules || []).length) {
    who.push(audienceWords(rules));
  }
  if (emails.length) {
    who.push(emails.length === 1 ? '1 person' : `${emails.length} people`);
  }
  return who.length ? `Shown to ${who.join(' · ')}` : 'Shown to everyone';
}

function badges(link) {
  const out = [];
  if (link.visible === false) {
    out.push(el('span', 'hidden-badge', 'Hidden'));
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
    const links = apps ? [] : category.links.filter(link => matches(link, needle));
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
        send.replaceChildren(svg(event.answer === 'yes' ? 'check' : 'calendar-plus'), el('span', '', event.answer === 'yes' ? 'Invite sent' : 'Add to My Calendar'));
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
    await act('events', event.id, 'answer', {answer: word});
  } catch (err) {
    toast(err.message);
    return false;
  }
  return true;
}

function appsMatching(needle) {
  return state.model.apps.filter(a => (a.forMe || isAdmin()) && (!needle || `${a.name} ${a.tagline}`.toLowerCase().includes(needle)));
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
  card.append(el('div', 'chip-title', app.name));
  card.append(el('div', 'chip-description', app.tagline));
  const go = el('span', 'button chip-open');
  go.append(el('span', '', 'Open App'), svg('arrow'));
  card.append(go);
  slot.append(card);
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
  return category.links.length > 0;
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

