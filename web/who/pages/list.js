import {state, model, lists, tags, peopleOf, emailOf, isStudent, isStaff, familiesOf, familyOf, kidsOf, adultsOf, gradeName, classroomName} from '../state.js';
import {csvField} from '../dom.js';
import {dataGrid} from '/datagrid.js';
import {el, svg} from '/elements.js';
import {familySearchText} from '../families.js';
import {personCard, personLink} from '../people.js';
import {tagControl, onTagsChange, tagLabel, tagFacetOptions, listSource, sharedTag, otherManagers, manageControl, selectedGuests} from '../tags.js';
import {saveTagRelations} from '../storage.js';
import {matchesFilters, familyMatchesFilters, roleChips, gradeOptions, classroomOptions, directoryFilter, tagRelationOptionsFor} from '../filters.js';
import {facetDropdown, familyDropdown} from '/rules.js';
import {initFamilyMap} from './map.js';

const tagListViews = [
  {key: 'faces', label: 'Profiles', icon: 'everyone'},
  {key: 'emails', label: 'Email List', icon: 'email-list'},
  {key: 'map', label: 'Map', icon: 'map'},
];

function tagListViewSwitch(rerender) {
  const bar = el('div', 'view-switch');
  for (const v of tagListViews) {
    const btn = el('button', 'view-switch-btn' + (state.tagListView === v.key ? ' active' : ''));
    btn.type = 'button';
    btn.title = v.label;
    btn.append(svg(v.icon));
    btn.addEventListener('click', () => {
      if (state.tagListView === v.key) {
        return;
      }
      state.tagListView = v.key;
      for (const sibling of bar.querySelectorAll('.view-switch-btn')) {
        sibling.classList.remove('active');
      }
      btn.classList.add('active');
      rerender();
    });
    bar.append(btn);
  }
  return bar;
}

function renderTagMap(container) {
  const canvas = el('div', 'map-canvas');
  container.append(canvas);
  const q = state.q;
  initFamilyMap(canvas, family => familyMatchesFilters(family) && familySearchText(family).includes(q));
}

const emailColumns = [
  {label: 'Full Name', get: r => r.p.name_show},
  {label: 'Email', get: r => emailOf(r.p)},
  {label: 'Role', get: r => r.role},
  {label: 'Grade', get: r => r.grade},
  {label: 'Classroom', get: r => r.classroom},
];

function kidsField(parent, of) {
  const family = familyOf(parent);
  const values = (family ? kidsOf(family) : []).map(of).filter(Boolean);
  return [...new Set(values)].join(', ');
}

function emailEntries() {
  const rows = [];
  const seen = new Set();
  const add = (p, role, grade, classroom) => {
    if (!emailOf(p) || seen.has(p.id)) {
      return;
    }
    seen.add(p.id);
    rows.push({p, role, grade, classroom});
  };
  const byName = (a, b) => a.name_show.localeCompare(b.name_show);
  for (const s of model.people.filter(isStudent).sort(byName)) {
    add(s, 'Student', gradeName(s), classroomName(s));
    for (const family of familiesOf(s)) {
      for (const parent of adultsOf(family)) {
        add(parent, 'Parent', kidsField(parent, gradeName), kidsField(parent, classroomName));
      }
    }
  }
  for (const s of model.people.filter(isStaff).sort(byName)) {
    add(s, 'Staff', '', '');
  }
  return rows;
}

export function listPage() {
  const page = document.createDocumentFragment();

  const title = state.filterTags.size ? [...state.filterTags].map(tagLabel).join(', ') : 'Everyone';
  const only = state.filterTags.size === 1 ? [...state.filterTags][0] : '';
  const smart = only && lists[only] ? lists[only] : null;
  const ownTag = only && tags[only] ? only : '';

  const pageHeader = el('div', 'page-header container page-header-list');
  const titleWrap = el('div');
  titleWrap.append(el('h1', 'page-title', title));
  if (smart) {
    const line = el('div', 'page-subtitle magic-source');
    const source = listSource(smart.key);
    if (source) {
      line.append(svg('sparkle'), el('span', '', 'Magic Tag from '));
      const link = el('a');
      link.href = source.href;
      const mark = el('img');
      mark.src = `/brand/apps/${source.app}.png`;
      mark.alt = '';
      link.append(mark, el('span', '', `${source.name} - open this ${source.thing}`));
      line.append(link);
    } else {
      line.append(svg('sparkle'), el('span', '', 'Magic Tag from the directory - the families of the grades you are a room parent for'));
    }
    titleWrap.append(line);
  }
  const chip = p => {
    const a = el('a', 'tag-chip person-chip', p.name_show);
    a.href = personLink(p);
    return a;
  };
  const ownership = el('div', 'page-subtitle tag-ownership');
  const paintOwnership = () => {
    ownership.replaceChildren();
    const others = ownTag ? peopleOf(otherManagers(ownTag)) : [];
    if (!ownTag || !sharedTag(ownTag)) {
      ownership.hidden = true;
      return;
    }
    ownership.append(svg('families'), el('span', '', 'Managed with '));
    others.forEach((p, i) => {
      if (i) {
        ownership.append(el('span', '', i === others.length - 1 ? ' and ' : ', '));
      }
      ownership.append(chip(p));
    });
    ownership.hidden = false;
  };
  paintOwnership();
  titleWrap.append(ownership);
  pageHeader.append(titleWrap);
  pageHeader.append(tagListViewSwitch(() => renderGrid()));
  page.append(pageHeader);

  const content = el('div', 'content container');
  const header = el('div', 'content-header content-header-solo');
  const controls = el('div', 'controls');
  const lead = el('div', 'controls-lead');
  const familyOptions = only ? tagRelationOptionsFor(only) : [];
  if (familyOptions.length) {
    lead.append(familyDropdown(familyOptions, state.filterTagRelations, () => {
      saveTagRelations(only, state.filterTagRelations);
      renderGrid();
    }));
  }
  if (lead.children.length) {
    controls.classList.add('controls-spread');
    controls.append(lead);
  }
  controls.append(roleChips(() => renderGrid()));
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
  const onTagPage = state.filterTags.size > 0;
  const facetFilters = el('div', 'facet-filters');
  if (!onTagPage) {
    facetFilters.append(
      facetDropdown('Grade', null, gradeOptions(), state.filterGrades, () => renderGrid()),
      facetDropdown('Classroom', null, classroomOptions(), state.filterClassrooms, () => renderGrid()),
    );
    controls.append(facetFilters);
  }
  let tagsFacet = null;
  let mobileFilter = null;
  const buildTagFilters = () => {
    if (tagsFacet) {
      tagsFacet.remove();
      tagsFacet = null;
    }
    if (!onTagPage && tagFacetOptions().length) {
      tagsFacet = facetDropdown('Tags', null, tagFacetOptions(), state.filterTags, () => renderGrid());
      facetFilters.append(tagsFacet);
    }
    const next = directoryFilter(() => renderGrid(), {role: false, city: false, pronouns: false});
    if (!onTagPage) {
      next.classList.add('mobile-filter');
    }
    if (mobileFilter) {
      mobileFilter.replaceWith(next);
    } else {
      controls.append(next);
    }
    mobileFilter = next;
  };
  buildTagFilters();
  onTagsChange(buildTagFilters);
  if (ownTag) {
    lead.append(manageControl(ownTag, paintOwnership));
    if (!lead.parentElement) {
      controls.classList.add('controls-spread');
      controls.prepend(lead);
    }
  }
  const download = el('a', 'filter-button email-download');
  download.title = 'Download what the list currently shows';
  download.append(svg('download'), el('span', '', 'CSV'));
  controls.append(search, download);
  header.append(controls);
  content.append(header);

  const grid = el('div');
  content.append(grid);
  page.append(content);

  function renderEmailsTable(container, rows) {
    const columns = emailColumns.map((c, i) => (i === 0 ? {...c, show: r => {
      const link = el('a', '', r.p.name_show);
      link.href = personLink(r.p);
      return link;
    }} : c));
    const trailing = r => (r.p.source === 'guest' ? el('span') : tagControl(r.p.id, 'tag-wrap', 'row-tag', () => {
      if (state.filterTags.size) {
        renderGrid();
      }
    }));
    container.append(dataGrid({columns, rows, trailing}).wrap);
  }

  function renderGrid() {
    grid.replaceChildren();
    grid.className = '';
    const rows = emailEntries()
      .filter(r => (r.p.name_show.toLowerCase().includes(state.q) || emailOf(r.p).toLowerCase().includes(state.q)) && matchesFilters(r.p))
      .concat(selectedGuests().map(g => ({p: g, role: 'Guest', grade: '', classroom: ''})));
    const csv = [emailColumns.map(c => c.label).join(',')]
      .concat(rows.map(r => emailColumns.map(c => csvField(c.get(r))).join(',')))
      .join('\n');
    download.href = 'data:text/csv;charset=utf-8,' + encodeURIComponent(csv);
    download.download = 'email-list.csv';

    if (state.tagListView === 'map') {
      renderTagMap(grid);
      return;
    }
    if (!rows.length) {
      grid.append(el('div', 'empty', 'No matches.'));
      return;
    }
    if (state.tagListView === 'emails') {
      renderEmailsTable(grid, rows);
      return;
    }
    grid.className = 'people-grid directory-grid';
    for (const r of rows) {
      grid.append(personCard(r.p));
    }
  }
  renderGrid();
  // The router mounts the page after this returns.
  queueMicrotask(() => input.focus());
  return page;
}
