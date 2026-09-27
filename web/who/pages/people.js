import {state, colors} from '../state.js';
import {el, svg, thumbUrl, firstName} from '../dom.js';
import {tabStrip, tabHref} from '/tabs.js';
import {familiesOf, familyOf} from '../families.js';
import {personCard, personLink, photoOrInitials, cardMore, gradeChain, guestCard} from '../people.js';
import {tagFacetOptions, onTagsChange, selectedGuests} from '../tags.js';
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
  const guests = selectedGuests();
  for (const g of guests) {
    grid.append(guestCard(g));
  }
  return matches.length + guests.length;
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

function renderFamilies(grid) {
  grid.className = 'people-grid directory-grid';
  const matches = state.familyOrder.filter(f =>
    `${f.name} ${f.members.join(' ')}`.toLowerCase().includes(state.q) && familyMatchesFilters(f.key));
  for (const f of matches) {
    const card = el('a', 'person-card');
    card.href = f.href;
    const photo = photoOrInitials(f.photoUrl, f.name, 'person-photo');
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

  const items = peopleTabs.map(t => ({...t, icon: svg(t.key === 'staff' ? 'staff-tab' : t.key)}));
  const strip = tabStrip(items, state.tab, 2, key => {
    state.tab = key;
    state.q = '';
    state.filterTags.clear();
    history.replaceState(null, '', tabHref(key));
    renderPeople();
    finishRender();
  });
  strip.classList.add('container');
  main.append(strip);

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
  controls.append(facetFilters, search);
  let tagsFacet = null;
  let mobileFilter = null;
  const buildTagFilters = () => {
    if (tagsFacet) {
      tagsFacet.remove();
      tagsFacet = null;
    }
    if (isEveryone && tagFacetOptions().length) {
      tagsFacet = facetDropdown('Tags', tagFacetOptions(), state.filterTags, () => renderGrid());
      facetFilters.append(tagsFacet);
    }
    const next = filterControl(() => renderGrid(), {role: false, city: false, pronouns: false, newToHelios: false, tags: isEveryone});
    next.classList.add('mobile-filter');
    if (mobileFilter) {
      mobileFilter.replaceWith(next);
    } else {
      controls.append(next);
    }
    mobileFilter = next;
  };
  buildTagFilters();
  onTagsChange(() => {
    buildTagFilters();
    if (state.filterTags.size) {
      renderGrid();
    }
  });
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
