import {state, years, allYears, sortByStart, matches, selectedYear, listedIn} from '../state.js';
import {el, toggle, selectPill, thumb} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {activityCard, categoryClass} from '../cards.js';

let query = '';

function shownIn(year) {
  return sortByStart(listedIn(year)).filter(a =>
    matches(a, query) && (!state.category || a.category === state.category));
}

// Cards are grouped under their category, in the order the Categories tab lists
// them. An activity naming a category that tab doesn't have is refused at load
// (internal/events/load.go), so nothing can fall outside these groups.
function yearGrid(year) {
  const root = el('div');
  const items = shownIn(year);
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
    root.append(head);
    const grid = el('div', 'card-grid');
    for (const act of inGroup) {
      grid.append(activityCard(act));
    }
    root.append(grid);
  }
  return root;
}

function yearContent(year, thisYear) {
  const content = el('div');
  const chips = el('div', 'chip-row');
  const list = el('div');
  const paint = () => list.replaceChildren(yearGrid(year));

  // The chip row filters the grid in place: "All" plus one chip per category
  // that has something in it this year, each in that category's own tint. The
  // chips follow the year and the switches but not the search or the chosen
  // chip, so filtering never makes the other chips disappear.
  const paintChips = () => {
    chips.replaceChildren();
    const present = new Set(listedIn(year).map(a => a.category));
    const add = (id, label) => {
      const chip = el('button', 'chip ' + (id ? categoryClass(id) : 'chip-all') + (state.category === id ? ' is-on' : ''));
      chip.type = 'button';
      chip.textContent = label;
      chip.addEventListener('click', () => {
        state.category = state.category === id ? '' : id;
        paintChips();
        paint();
      });
      chips.append(chip);
    };
    add('', 'All');
    for (const c of state.model.categories) {
      if (present.has(c.id)) {
        add(c.id, c.title);
      }
    }
  };

  const head = el('div', 'section-head');
  head.append(el('h2', '', thisYear ? 'Upcoming Opportunities' : `Opportunities for ${year}`));
  head.append(toggle('Show Completed Events', state.showPrevious, on => {
    state.showPrevious = on;
    paint();
  }));

  paintChips();
  paint();
  content.append(chips, head, list);
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
    state.category = '';
    history.replaceState(null, '', picked === years().current ? '/' : `/years/${encodeURIComponent(picked)}`);
    document.dispatchEvent(new CustomEvent('hca:refresh'));
  }));

  page.append(head, body);
  render();
  return page;
}
