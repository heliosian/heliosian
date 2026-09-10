import {state, colors} from '../state.js';
import {el, svg, thumbUrl, firstName, tabStrip, tabHref} from '../dom.js';
import {familiesOf, familyOf} from '../families.js';
import {personCard, personLink, photoOrInitials, cardMore, gradeChain} from '../people.js';
import {tagNames} from '../tags.js';
import {matchesFilters, familyMatchesFilters, roleChips, facetDropdown, gradeOptions, filterControl} from '../filters.js';
import {resetMain, finishRender} from '../chrome.js';
import {renderStaff} from './staff.js';
import {familyDetailChip} from './family.js';

const peopleTabs = [
  {key: 'everyone', label: 'Everyone'},
  {key: 'families', label: 'Families'},
];

function everyoneMatches() {
  const q = state.q;
  return state.everyoneOrder.filter(p => {
    const familyNames = familiesOf(p).map(f => f.name).join(' ');
    return `${p.fullName} ${familyNames}`.toLowerCase().includes(q) && matchesFilters(p);
  });
}

function renderEveryone(grid) {
  grid.className = 'people-grid directory-grid';
  const matches = everyoneMatches();
  for (const p of matches) {
    grid.append(personCard(p));
  }
  return matches.length;
}

function renderStudents(grid) {
  grid.className = 'student-grid';
  const matches = state.model.people.filter(p => p.isStudent && p.fullName.toLowerCase().includes(state.q) && matchesFilters(p));
  for (const p of matches) {
    const card = el('a', 'student-card');
    card.href = personLink(p);
    const head = el('div', 'student-head');
    const family = familyOf(p);
    if (family && family.photoUrl) {
      const bg = el('img', 'student-family-photo');
      bg.src = thumbUrl(family.photoUrl);
      bg.loading = 'lazy';
      bg.alt = '';
      head.append(bg);
    }
    head.append(photoOrInitials(p.photoUrl, p.fullName, 'student-photo'));
    head.append(cardMore(p.email));
    card.append(head);
    card.append(el('div', 'student-first', firstName(p.fullName)));
    card.append(el('div', 'student-last', p.fullName.replace(firstName(p.fullName), '').trim()));
    card.append(el('div', 'student-line', gradeChain(p)));
    if (p.pronouns) {
      card.append(el('div', 'student-pronouns', p.pronouns));
    }
    grid.append(card);
  }
  return matches.length;
}

// Shares .person-card/.person-photo/.photo-wrap-peek with personCard (see
// ../people.js), same as badgeCard in classrooms.js - a family tile looks
// like every other card in the app rather than its own older, plainer style.
function renderFamilies(grid) {
  grid.className = 'people-grid directory-grid';
  const matches = state.familyOrder.filter(f =>
    `${f.name} ${f.members.join(' ')}`.toLowerCase().includes(state.q) && familyMatchesFilters(f.key));
  for (const f of matches) {
    const card = el('a', 'person-card');
    card.href = f.href;
    const photo = photoOrInitials(f.photoUrl, f.name, 'person-photo');
    // A family can span more than one grade - the first kid's grade color
    // stands in as the one representative color for the photo/peek, same as
    // ringColorFor does for a parent (picks one kid's grade rather than
    // trying to blend several).
    const color = f.grades.length ? colors.grades[f.grades[0]] : null;
    const wrap = el('div', 'photo-wrap photo-wrap-peek');
    if (color) {
      photo.style.setProperty('--ring-color', color);
      if (photo.tagName === 'DIV') {
        photo.style.background = `color-mix(in srgb, ${color} 65%, white)`;
      }
      wrap.style.setProperty('--peek-color', color);
    }
    wrap.append(photo);
    if (f.grades.length) {
      // Pinned to the photo's own top-left corner (photoWithTag's cardMore
      // tag button owns the top-right one) instead of sitting as its own row
      // below the photo, same corner-badge treatment as gradeBadge does for
      // an individual student's card.
      const chipRow = el('div', 'chip-row chip-row-overlay');
      for (const g of f.grades) {
        chipRow.append(familyDetailChip(g, colors.grades[g]));
      }
      wrap.append(chipRow);
    }
    card.append(wrap);
    card.append(el('div', 'person-name', f.name));
    if (f.members.length) {
      card.append(el('div', 'person-sub', f.members.join(', ')));
    }
    grid.append(card);
  }
  return matches.length;
}

const tabRenderers = {
  everyone: renderEveryone,
  students: renderStudents,
  families: renderFamilies,
  staff: renderStaff,
};

export function renderPeople() {
  const main = resetMain();

  const pageHeader = el('div', 'page-header container');
  pageHeader.append(el('h1', 'page-title', 'Directory'));
  pageHeader.append(el('div', 'page-subtitle', 'Find and connect with the Helios community.'));
  main.append(pageHeader);

  const items = peopleTabs.map(t => ({...t, icon: t.key === 'staff' ? 'staff-tab' : t.key}));
  main.append(tabStrip(items, state.tab, 2, key => {
    state.tab = key;
    state.q = '';
    // The Tags dropdown only exists on the Everyone tab - clear it on every
    // switch so a filter set there can't silently keep narrowing results on a
    // tab with no control showing it's active. Role chips get the same
    // treatment without losing the selection: matchesFilters only applies
    // filterRoleExcluded while state.tab is 'everyone' (see below), so
    // switching to Families and back restores whatever was toggled off.
    state.filterTags.clear();
    history.replaceState(null, '', tabHref(key));
    renderPeople();
    finishRender();
  }));

  const content = el('div', 'content container');
  const isEveryone = state.tab === 'everyone';
  const header = el('div', 'content-header content-header-solo');
  const controls = el('div', 'controls');
  if (isEveryone) {
    controls.append(roleChips(() => renderGrid()));
  }
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    renderGrid();
  });
  search.append(input);
  const facetFilters = el('div', 'facet-filters');
  facetFilters.append(
    facetDropdown('Grade', gradeOptions(), state.filterGrades, () => renderGrid()),
    facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, () => renderGrid()),
  );
  if (isEveryone && tagNames().length) {
    facetFilters.append(facetDropdown('Tags', tagNames(), state.filterTags, () => renderGrid()));
  }
  controls.append(facetFilters, search);
  // Small-screen stand-in for the Grade/Classroom/Tags dropdowns above: same
  // filters, collapsed into one funnel-icon button so the mobile controls row
  // doesn't have to fit every facet dropdown individually. CSS swaps which of
  // the two is visible per breakpoint (see .facet-filters/.mobile-filter).
  const mobileFilter = filterControl(() => renderGrid(), {role: false, city: false, pronouns: false, newToHelios: false, tags: isEveryone});
  mobileFilter.classList.add('mobile-filter');
  controls.append(mobileFilter);
  header.append(controls);
  content.append(header);

  const grid = el('div');
  content.append(grid);
  main.append(content);

  function renderGrid() {
    grid.replaceChildren();
    if (tabRenderers[state.tab](grid) === 0) {
      grid.append(el('div', 'empty', 'No matches.'));
    }
  }
  renderGrid();
  input.focus();
}
