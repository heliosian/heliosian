import {state, parties, matches, celebration, whenParts, currentCelebration} from '../state.js';
import {el, selectPill, tabs, svg} from '../dom.js';
import {setTitle, setSearch, inTab} from '../chrome.js';
import {partyCard} from '../cards.js';

let query = '';

const listTabs = [
  {key: 'available', label: 'Available'},
  {key: 'waitlist', label: 'Waitlist'},
  {key: 'all', label: 'All Parties'},
];

function shown(code) {
  return parties(code).filter(p => inTab(p, state.tab) && matches(p, query) && (!state.category || p.category === state.category));
}

// celebrationBand is the year's gala itself, across the top of the parties
// page: its picture as the background under a teal wash, title, theme, when
// and where, and its button - Learn More, to the celebration's own site.
// Admins edit it from Admin Tools › Banner.
export function celebrationBand(c) {
  const band = el('div', 'gala-band' + (c.imageUrl ? ' has-image' : ''));
  if (c.imageUrl) {
    band.style.backgroundImage = `url("${c.imageUrl}")`;
  }
  const words = el('div', 'gala-words');
  words.append(el('div', 'gala-kicker', c.title));
  if (c.subtitle) {
    words.append(el('div', 'gala-title', c.subtitle));
  }
  const when = whenParts(c);
  const line = [when.day, when.time, c.location].filter(Boolean).join(' · ');
  if (line) {
    words.append(el('div', 'gala-when', line));
  }
  band.append(words);
  if (c.buttonUrl && c.buttonText) {
    const a = el('a', 'button button-small gala-button', c.buttonText);
    a.href = c.buttonUrl;
    a.target = '_blank';
    a.rel = 'noopener';
    band.append(a);
  }
  return band;
}

function grid(code) {
  const wrap = el('div');
  const items = shown(code);
  if (!items.length) {
    const panel = el('div', 'panel');
    const words = {
      available: 'No parties have tickets right now - check the waitlist, or see all parties.',
      waitlist: 'No party is taking a waitlist right now.',
      all: 'No parties yet.',
    };
    panel.append(el('div', 'panel-empty', query || state.category ? 'Nothing matches.' : words[state.tab]));
    wrap.append(panel);
    return wrap;
  }
  if (state.tab === 'all') {
    // Everything, in two groups: what is still to come, then what has been.
    const upcoming = items.filter(p => p.availability !== 'past');
    const past = items.filter(p => p.availability === 'past');
    for (const [heading, list] of [['Upcoming', upcoming], ['Past Parties', past]]) {
      if (!list.length) {
        continue;
      }
      wrap.append(el('h3', 'group-name', heading));
      const g = el('div', 'card-grid');
      for (const p of list) {
        g.append(partyCard(p));
      }
      wrap.append(g);
    }
    return wrap;
  }
  const g = el('div', 'card-grid');
  for (const p of items) {
    g.append(partyCard(p));
  }
  wrap.append(g);
  return wrap;
}

export function partiesPage(code) {
  if (code && celebration(code)) {
    state.celebration = code;
  } else if (state.model.current) {
    state.celebration = state.model.current;
  }
  const c = currentCelebration();
  setTitle('Parties');
  const page = el('div');
  if (c) {
    page.append(celebrationBand(c));
  }
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Community Fun(d)raiser Parties'));
  if (state.model.settings.partiesIntro) {
    main.append(el('p', 'page-intro', state.model.settings.partiesIntro));
  }
  head.append(main);
  const options = state.model.celebrations.map(x => ({key: x.code, label: x.title}));
  if (options.length > 1) {
    head.append(selectPill('calendar', options, state.celebration, picked => {
      state.celebration = picked;
      query = '';
      state.category = '';
      history.replaceState(null, '', picked === state.model.current ? '/' : `/celebrations/${encodeURIComponent(picked)}`);
      document.dispatchEvent(new CustomEvent('celebrate:refresh'));
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
    bar.append(tabs(listTabs.map(t => ({...t, count: counts[t.key]})), state.tab, key => {
      state.tab = key;
      paintTabs();
      paint();
    }));
    const filter = el('label', 'select-pill filter-pill');
    filter.append(svg('list'));
    const sel = el('select');
    sel.append(new Option('All categories', ''));
    for (const cat of state.model.categories) {
      const o = new Option(cat, cat);
      o.selected = state.category === cat;
      sel.append(o);
    }
    sel.addEventListener('change', () => {
      state.category = sel.value;
      paint();
    });
    filter.append(sel, svg('caret'));
    bar.append(filter);
  };
  paintTabs();
  paint();
  page.append(bar, list);
  setSearch('Search parties by title, host, or keyword…', q => {
    query = q;
    paint();
  });
  return page;
}
