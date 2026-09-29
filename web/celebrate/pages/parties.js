import {state, parties, celebration, whenParts, currentCelebration, celebrationCalendarLink} from '../state.js';
import {selectPill} from '../dom.js';
import {el, svg} from '/elements.js';
import {tabStrip} from '/tabs.js';
import {inTab, listTabs, listPath, listTab, listCategory, searchParties} from '../chrome.js';
import {setTitle, setSearch, renderChrome} from '/shell.js';
import {render} from '/router.js';
import {partyCard} from '../cards.js';

function shown(id) {
  return parties(id).filter(p => inTab(p, state.tab) && (!state.category || p.category === state.category));
}

export function celebrationBand(c) {
  const band = el('div', 'gala-band' + (c.imageUrl ? ' has-image' : ''));
  if (c.imageUrl) {
    band.style.backgroundImage = `url("${c.imageUrl}")`;
  }
  const words = el('div', 'gala-words');
  if (c.subtitle) {
    words.append(el('div', 'gala-kicker', c.subtitle));
  }
  words.append(el('div', 'gala-title', c.title));
  const when = whenParts(c);
  const line = [when.day, when.time, c.location].filter(Boolean).join(' · ');
  if (line) {
    words.append(el('div', 'gala-when', line));
  }
  band.append(words);
  const href = c.buttonUrl === 'calendar' ? celebrationCalendarLink(c) : c.buttonUrl;
  if (href && c.buttonText) {
    const a = el('a', 'button button-small gala-button');
    if (c.buttonUrl === 'calendar') {
      a.append(svg('calendar'));
    }
    a.append(el('span', '', c.buttonText));
    a.href = href;
    a.target = '_blank';
    a.rel = 'noopener';
    band.append(a);
  }
  return band;
}

function grid(id) {
  const wrap = el('div');
  const items = shown(id);
  if (!items.length) {
    const panel = el('div', 'panel');
    const words = {
      available: 'No parties have tickets right now - check the waitlist, or see all upcoming parties.',
      waitlist: 'No party is taking a waitlist right now.',
      upcoming: 'No parties coming up yet.',
      past: 'No party has happened yet.',
    };
    panel.append(el('div', 'panel-empty', state.category ? 'Nothing matches.' : words[state.tab]));
    wrap.append(panel);
    return wrap;
  }
  const g = el('div', 'card-grid');
  for (const p of items) {
    g.append(partyCard(p));
  }
  wrap.append(g);
  return wrap;
}

export function partiesPage(id) {
  if (id && celebration(id)) {
    state.celebration = id;
  } else if (state.model.current) {
    state.celebration = state.model.current;
  }
  state.tab = listTab();
  state.category = listCategory();
  const c = currentCelebration();
  setTitle('Parties');
  const page = el('div');
  const banner = celebration(state.model.banner) || c;
  if (banner) {
    page.append(celebrationBand(banner));
  }
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Community Fun(d)raiser Parties'));
  if (state.model.settings.partiesIntro) {
    main.append(el('p', 'page-intro', state.model.settings.partiesIntro));
  }
  head.append(main);
  const options = state.model.celebrations
    .filter(x => x.id === state.celebration || parties(x.id).length)
    .map(x => ({key: x.id, label: x.title}));
  if (options.length > 1) {
    head.append(selectPill('calendar', options, state.celebration, picked => {
      state.celebration = picked;
      history.replaceState(null, '', picked === state.model.current ? '/' : `/celebrations/${encodeURIComponent(celebration(picked).code)}`);
      render();
    }));
  }
  page.append(head);

  const bar = el('div', 'list-bar');
  const list = el('div');
  const paint = () => list.replaceChildren(grid(state.celebration));
  const counts = {};
  for (const t of listTabs) {
    counts[t.key] = parties().filter(p => inTab(p, t.key)).length;
  }
  const paintTabs = () => {
    bar.replaceChildren();
    bar.append(tabStrip(listTabs.map(t => ({...t, count: counts[t.key]})), state.tab, 2, key => {
      state.tab = key;
      history.pushState(null, '', listPath(state.tab, state.category));
      paintTabs();
      paint();
      renderChrome();
    }));
    const filter = el('label', 'select-pill filter-pill');
    filter.append(svg('list'));
    const sel = el('select');
    sel.append(new Option('All categories', ''));
    for (const cat of state.model.categories) {
      const o = new Option(cat.title, cat.id);
      o.selected = state.category === cat.id;
      sel.append(o);
    }
    sel.addEventListener('change', () => {
      state.category = sel.value;
      history.pushState(null, '', listPath(state.tab, state.category));
      paint();
    });
    filter.append(sel, svg('chevron-down'));
    bar.append(filter);
  };
  paintTabs();
  paint();
  page.append(bar, list);
  setSearch('Search parties by title, host, or keyword…', searchParties);
  return page;
}
