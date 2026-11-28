import {state, parties, whenParts, currentCelebration, celebrationCalendarLink, celebrationUpcoming, categoriesOf, partiesPath, invitationPath, invitationPageId, shownInvitation, celebrationLabel} from '../state.js';
import {el, svg} from '/elements.js';
import {selectPill} from '/form.js';
import {tabStrip} from '/tabs.js';
import {inTab, listPath, listTab, listCategory, searchParties, shownTabs, themedPage, pageHeadMain} from '../chrome.js';
import {setTitle, setSearch, renderChrome} from '/shell.js';
import {render} from '/router.js';
import {partyCard} from '../cards.js';
import {themed} from '/themes.js';
import {inviteCard} from '/envelope.js';
import {openSheet, fillDeck} from '/sheets.js';
import {loadDetails, noteOpened, rsvpPanel, deckOf, cardWords} from '/invitation.js';

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
  const buttons = c.links.map(l => ({text: l.text, href: l.url}));
  if (celebrationUpcoming(c)) {
    buttons.push({text: 'Add to Calendar', href: celebrationCalendarLink(c), icon: 'calendar'});
  }
  for (const b of buttons) {
    const a = el('a', 'button button-small gala-button');
    if (b.icon) {
      a.append(svg(b.icon));
    }
    a.append(el('span', '', b.text));
    a.href = b.href;
    a.target = '_blank';
    a.rel = 'noopener';
    band.append(a);
  }
  return band;
}

function invitationSection(inv) {
  const section = themed(el('section', 'invitation-section'), inv.look);
  const column = el('div', 'invitation-column invite-column');
  const above = el('div', 'invite-sheets is-above');
  const stage = el('div', 'invitation-stage invite-stage');
  const below = el('div', 'invite-sheets is-below');
  const side = el('div', 'invitation-side');
  column.append(above, stage, below);
  section.append(column, side);
  const showDeck = details => {
    const item = fillDeck(inv.look, deckOf(inv, inv.pages), invitationPageId(), above, below, picked => {
      history.replaceState(null, '', invitationPath(picked.card ? '' : picked.id));
      showDeck(details);
      renderChrome();
    });
    stage.replaceChildren(item.card ? inviteCard(cardWords(inv, details)) : openSheet(inv.look, item));
  };
  const fillSide = details => {
    side.replaceChildren();
    if (details.household.length && inv.status !== 'cancelled') {
      side.append(rsvpPanel(inv, details, redraw));
    }
  };
  const redraw = async () => fillSide(await loadDetails(inv));
  loadDetails(inv).then(details => {
    showDeck(details);
    fillSide(details);
    noteOpened(inv, details);
  });
  return section;
}

export function invitationPage() {
  const inv = shownInvitation();
  setTitle(inv.name);
  return invitationSection(inv);
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

export function partiesPage() {
  state.tab = listTab();
  state.category = listCategory();
  const c = currentCelebration();
  setTitle('Parties');
  const page = themedPage();
  if (c && !c.inv) {
    page.append(celebrationBand(c));
  }
  const head = el('div', 'page-head');
  const main = pageHeadMain('Community Fun(d)raiser Parties', c ? c.title : '');
  if (state.model.settings.partiesIntro) {
    main.append(el('p', 'page-intro', state.model.settings.partiesIntro));
  }
  head.append(main);
  const options = state.model.celebrations
    .filter(x => x.id === state.celebration || parties(x.id).length)
    .map(x => ({key: x.id, label: celebrationLabel(x)}));
  if (options.length > 1 && !(c && c.inv)) {
    head.append(selectPill('calendar', options, state.celebration, picked => {
      state.celebration = picked;
      history.replaceState(null, '', partiesPath());
      render();
    }));
  }
  page.append(head);

  const bar = el('div', 'list-bar');
  const list = el('div');
  const paint = () => list.replaceChildren(grid(state.celebration));
  const paintTabs = () => {
    bar.replaceChildren();
    bar.append(tabStrip(shownTabs(true), state.tab, 2, key => {
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
    for (const cat of categoriesOf(state.celebration)) {
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
