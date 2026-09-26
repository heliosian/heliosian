import {state, isAdmin, years, allYears, sortByStart, matches, selectedYear, listedIn, canAdd, addLabel, categoryPath, categoryFromAddress, PRIORITY, isPriority, descendants} from '../state.js';
import {el, toggle, selectPill, thumb, button, svg} from '../dom.js';
import {setTitle, setSearch, renderChrome} from '../chrome.js';
import {activityCard, categoryClass, priorityRow, wanted} from '../cards.js';
import {openActivity} from '../edit.js';

let query = '';

function shownIn(year) {
  return sortByStart(listedIn(year)).filter(a =>
    matches(a, query) && (!state.category || (state.category === PRIORITY ? isPriority(a) : a.category === state.category)));
}

// Cards are grouped under their category, in the order the Categories tab lists
// them. An activity naming a category that tab doesn't have lands under the
// built-in Uncategorized heading (internal/team/load.go), so nothing can fall
// outside these groups.
function yearGrid(year) {
  const root = el('div');
  // A heading kept off the main page only appears when it is the one asked
  // for - from the rail, or its chip.
  const items = shownIn(year).filter(a => {
    const c = state.model.categories.find(c => c.id === a.category);
    return !c || c.showOnMain || state.category === c.id || state.category === PRIORITY;
  });
  if (!items.length) {
    const panel = el('div', 'panel');
    panel.append(el('div', 'panel-empty', query || state.category ? 'Nothing matches.' : 'Nothing to sign up for yet.'));
    root.append(panel);
    return root;
  }
  for (const c of state.model.categories) {
    const inGroup = items.filter(a => a.category === c.id);
    if (!inGroup.length) {
      continue;
    }
    const head = el('div', 'group-head');
    if (c.imageUrl) {
      head.append(thumb(c.imageUrl, c.title, 'group-image'));
    }
    const heading = el('div', 'group-heading');
    heading.append(el('h3', 'group-name', c.title));
    if (c.description) {
      heading.append(el('p', 'group-note', c.description));
    }
    head.append(heading);
    // An Add button on the heading, when the category takes additions from
    // people, or always for an admin, so a new thing lands in the right place;
    // the built-in Uncategorized heading takes none.
    if (!c.builtIn && (canAdd(c) || isAdmin())) {
      const add = button(isAdmin() ? 'Add' : addLabel(c), 'plus', 'button button-secondary button-small group-add',
        () => openActivity(null, {category: c.id}));
      add.title = `Add to ${c.title}`;
      head.append(add);
    }
    root.append(head);
    const grid = el('div', 'card-grid');
    for (const act of inGroup) {
      grid.append(activityCard(act));
    }
    root.append(grid);
  }
  return root;
}

// priorityItems are the things marked a priority this year that still want
// people - an event, or something at any depth under one, each itself -
// among what the page lists, soonest first.
function priorityItems(year) {
  const out = [];
  for (const a of listedIn(year)) {
    out.push(...[a, ...descendants(a)].filter(wanted));
  }
  return sortByStart(out);
}

// priorityPanel is High Priority, between the chips and the grid: a pale
// red panel with its heading over a card of rows, one for each thing
// marked a priority - the thing itself, so a role under an event is its
// own row, naming what it is part of.
function priorityPanel(year) {
  const items = priorityItems(year).filter(n => matches(n, query));
  if (!items.length) {
    return null;
  }
  const panel = el('section', 'prio-panel');
  const head = el('div', 'prio-head');
  const mark = el('span', 'prio-mark');
  mark.append(svg('bolt'));
  const words = el('div', 'prio-head-words');
  words.append(el('h2', '', 'High Priority'), el('p', '', 'These need your help soon.'));
  head.append(mark, words);
  const list = el('div', 'prio-list');
  for (const node of items) {
    list.append(priorityRow(node));
  }
  panel.append(head, list);
  return panel;
}

function yearContent(year, thisYear) {
  const content = el('div');
  const chips = el('div', 'chip-row');
  const list = el('div');
  const top = el('div');
  const pick = id => {
    state.category = state.category === id ? '' : id;
    history.pushState(null, '', categoryPath(state.category));
    paintChips();
    paint();
    renderChrome();
  };
  // The panel leads the page with All, and stands alone under the High
  // Priority chip; another chip narrows the page to its heading.
  const paint = () => {
    const onPriority = state.category === PRIORITY;
    const panel = !state.category || onPriority ? priorityPanel(year) : null;
    top.replaceChildren(...(panel ? [panel] : []));
    list.replaceChildren(...(onPriority ? [] : [yearGrid(year)]));
  };

  // The chip row filters the grid in place: High Priority first, when an
  // admin has marked anything this year, then "All" plus one chip per category
  // that has something in it this year, each in that category's own tint. The
  // chips follow the year and the switches but not the search or the chosen
  // chip, so filtering never makes the other chips disappear.
  const paintChips = () => {
    chips.replaceChildren();
    const present = new Set(listedIn(year).map(a => a.category));
    const add = (id, label) => {
      const chip = el('button', 'chip ' + (id === PRIORITY ? 'chip-priority' : id ? categoryClass(id) : 'chip-all') + (state.category === id ? ' is-on' : ''));
      chip.type = 'button';
      chip.textContent = label;
      // High Priority says how many things are marked one.
      if (id === PRIORITY) {
        chip.append(el('span', 'chip-count', String(priorityItems(year).length)));
      }
      // A chip is the rail's pick too, kept in the address the same way,
      // but repaints in place so the search typed so far stands.
      chip.addEventListener('click', () => pick(id));
      chips.append(chip);
    };
    // High Priority leads, while anything this year is marked one.
    if (priorityItems(year).length) {
      add(PRIORITY, 'High Priority');
    }
    add('', 'All');
    for (const c of state.model.categories) {
      if (present.has(c.id) && (c.showOnMain || state.category === c.id)) {
        add(c.id, c.title);
      }
    }
  };

  const head = el('div', 'section-head');
  head.append(el('h2', '', thisYear ? 'Upcoming Opportunities' : `Opportunities for ${year}`));
  head.append(toggle('Show Past Events', state.showPrevious, on => {
    state.showPrevious = on;
    paint();
  }));

  paintChips();
  paint();
  content.append(chips, top, head, list);
  setSearch('Search opportunities by title, event, or keyword…', q => {
    query = q;
    paint();
  });
  return content;
}

export function signUpPage(yearParam) {
  const options = allYears();
  if (yearParam && options.includes(yearParam)) {
    state.year = yearParam;
  }
  const year = selectedYear();
  state.year = year;
  // The category is the address's: ?category= names it, and none is all.
  state.category = categoryFromAddress();

  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Volunteer Opportunities'));
  head.append(main);

  const body = el('div');
  const render = () => {
    setTitle(year);
    body.replaceChildren(yearContent(year, year === years().current));
  };
  // Switching year is a filter on this page, not a new destination, so the URL
  // is replaced rather than pushed - back still leaves the page rather than
  // walking every year the reader looked at.
  head.append(selectPill('calendar', options.map(y => ({key: y, label: y})), year, picked => {
    state.year = picked;
    query = '';
    history.replaceState(null, '', picked === years().current ? '/' : `/years/${encodeURIComponent(picked)}`);
    document.dispatchEvent(new CustomEvent('hca:refresh'));
  }));

  page.append(head, body);
  render();
  return page;
}
