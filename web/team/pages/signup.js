import {state, isAdmin, years, allYears, sortByStart, matches, selectedYear, listedIn, canAdd, addLabel, categoryPath, categoryFromAddress, PRIORITY, isPriority, descendants} from '../state.js';
import {el, toggle, selectPill, thumb, button, svg} from '../dom.js';
import {setTitle, setSearch, renderChrome} from '/shell.js';
import {render} from '/router.js';
import {activityCard, categoryClass, priorityRow, wanted} from '../cards.js';
import {openActivity} from '../edit.js';

let query = '';

function shownIn(year) {
  return sortByStart(listedIn(year)).filter(a =>
    matches(a, query) && (!state.category || (state.category === PRIORITY ? isPriority(a) : a.category === state.category)));
}

function yearGrid(year) {
  const root = el('div');
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

function priorityItems(year) {
  const out = [];
  for (const a of listedIn(year)) {
    out.push(...[a, ...descendants(a)].filter(wanted));
  }
  return sortByStart(out);
}

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
  const paint = () => {
    const onPriority = state.category === PRIORITY;
    const panel = !state.category || onPriority ? priorityPanel(year) : null;
    top.replaceChildren(...(panel ? [panel] : []));
    list.replaceChildren(...(onPriority ? [] : [yearGrid(year)]));
  };

  const paintChips = () => {
    chips.replaceChildren();
    const present = new Set(listedIn(year).map(a => a.category));
    const add = (id, label) => {
      const chip = el('button', 'chip ' + (id === PRIORITY ? 'chip-priority' : id ? categoryClass(id) : 'chip-all') + (state.category === id ? ' is-on' : ''));
      chip.type = 'button';
      chip.textContent = label;
      if (id === PRIORITY) {
        chip.append(el('span', 'chip-count', String(priorityItems(year).length)));
      }
      chip.addEventListener('click', () => pick(id));
      chips.append(chip);
    };
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
  state.category = categoryFromAddress();

  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Volunteer Opportunities'));
  head.append(main);

  const body = el('div');
  const paintYear = () => {
    setTitle(year);
    body.replaceChildren(yearContent(year, year === years().current));
  };
  head.append(selectPill('calendar', options.map(y => ({key: y, label: y})), year, picked => {
    state.year = picked;
    query = '';
    history.replaceState(null, '', picked === years().current ? '/' : `/years/${encodeURIComponent(picked)}`);
    render();
  }));

  page.append(head, body);
  paintYear();
  return page;
}
