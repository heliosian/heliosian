import {state} from './state.js';
import {el, svg} from '/elements.js';
import {chipToggle, facetDropdown, filterControl} from '/rules.js';
import {today, datedFrom} from '/rail.js';
import {setSearch, searchInput} from '/shell.js';

const comingUp = 'Coming up';
const past = 'Past';
const noDate = 'No date';
const hasShortName = 'Has a short name';
const noShortName = 'No short name';

const filters = {
  excluded: new Set(),
  when: new Set([comingUp, noDate]),
  partOf: new Set(),
  shortName: new Set(),
  query: '',
};

function rows() {
  return state.model.groupRows;
}

function whenOf(g) {
  const dated = datedFrom(rows()[g.id], rows());
  if (!dated) {
    return noDate;
  }
  return (dated.end || dated.start).slice(0, 10) >= today() ? comingUp : past;
}

function ancestorsOf(g) {
  const out = [];
  for (let at = rows()[rows()[g.id].parent]; at; at = rows()[at.parent]) {
    out.push(at.id);
  }
  return out;
}

function matchesQuery(g) {
  return !filters.query || `${g.title} ${g.address} ${g.description}`.toLowerCase().includes(filters.query);
}

export function shown(g) {
  const relationOK = (g.run && !filters.excluded.has('manager')) || (g.member && !filters.excluded.has('member'));
  const whenOK = !filters.when.size || filters.when.has(whenOf(g));
  const partOfOK = !filters.partOf.size || ancestorsOf(g).some(id => filters.partOf.has(id));
  const shortNameOK = !filters.shortName.size || filters.shortName.has(g.slug ? hasShortName : noShortName);
  return relationOK && whenOK && partOfOK && shortNameOK && matchesQuery(g);
}

function partOfOptions() {
  const above = new Set(state.model.groups.flatMap(ancestorsOf));
  const children = {};
  const roots = [];
  for (const id of above) {
    const parent = rows()[id].parent;
    if (above.has(parent)) {
      (children[parent] = children[parent] || []).push(id);
    } else {
      roots.push(id);
    }
  }
  const byName = (a, b) => rows()[a].name.localeCompare(rows()[b].name);
  const out = [];
  const walk = (ids, depth) => {
    for (const id of ids.sort(byName)) {
      out.push({value: id, label: rows()[id].name, depth});
      walk(children[id] || [], depth + 1);
    }
  };
  walk(roots, 0);
  return out;
}

const relationChips = [
  ['manager', 'I\'m a manager'],
  ['member', 'I\'m a member'],
];

export function groupFilters(rerender) {
  const controls = el('div', 'group-controls');
  const chips = el('div', 'chip-row');
  for (const [key, label] of relationChips) {
    chips.append(chipToggle(label, !filters.excluded.has(key), on => {
      if (on) {
        filters.excluded.delete(key);
      } else {
        filters.excluded.add(key);
      }
      rerender();
    }, key));
  }
  const sections = [
    {label: 'When', values: [comingUp, past, noDate], chosen: filters.when},
    {label: 'Part of', values: partOfOptions(), chosen: filters.partOf},
    {label: 'Short name', values: [hasShortName, noShortName], chosen: filters.shortName},
  ];
  const facets = el('div', 'facet-filters');
  for (const s of sections) {
    facets.append(facetDropdown(s.label, null, s.values, s.chosen, rerender));
  }
  const search = el('div', 'search');
  const input = el('input');
  input.placeholder = 'Search';
  input.value = filters.query;
  input.addEventListener('input', () => {
    searchInput().value = input.value;
    filters.query = input.value.trim().toLowerCase();
    rerender();
  });
  setSearch('', q => {
    input.value = searchInput().value;
    filters.query = q;
    rerender();
  });
  searchInput().value = input.value;
  search.append(svg('search'), input);
  const mobile = filterControl(sections, rerender);
  mobile.classList.add('mobile-filter');
  controls.append(chips, facets, search, mobile);
  return controls;
}
