import {state, byEmail} from '../state.js';
import {el, svg, csvField, copyGlyph} from '../dom.js';
import {familiesOf, familyOf, familySearchText} from '../families.js';
import {personCard, personLink} from '../people.js';
import {tagNames, tagControl} from '../tags.js';
import {saveTagRelations} from '../storage.js';
import {matchesFilters, familyMatchesFilters, roleChips, facetDropdown, gradeOptions, filterControl, tagRelationOptions} from '../filters.js';
import {resetMain} from '../chrome.js';
import {initFamilyMap} from './map.js';

const tagListViews = [
  {key: 'faces', label: 'Profiles', icon: 'everyone'},
  {key: 'emails', label: 'Email List', icon: 'email-list'},
  {key: 'map', label: 'Map', icon: 'map'},
];

// Single-select, icon-only segmented control for how to view a list - it's
// the same filtered data underneath either way, just displayed differently,
// so it reads as a display-mode switch (like a grid/list view toggle) rather
// than a top-level tab or another filter chip.
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

// The Map view of a tag list - families that have someone matching the same
// filters (tag, relations, search, ...) as Faces/Emails, sharing the map
// plumbing with the standalone Map page.
function renderTagMap(container) {
  const canvas = el('div', 'map-canvas');
  container.append(canvas);
  const q = state.q;
  initFamilyMap(canvas, family => familyMatchesFilters(family.key) && familySearchText(family).includes(q));
}

const emailColumns = [
  {label: 'Full Name', get: r => r.p.fullName},
  {label: 'Email', get: r => r.p.email},
  {label: 'Role', get: r => r.role},
  {label: 'Grade', get: r => r.grade},
  {label: 'Classroom', get: r => r.classroom},
];

function kidsField(parent, field) {
  const family = familyOf(parent);
  const values = ((family && family.kidEmails) || [])
    .map(e => byEmail[e])
    .filter(Boolean)
    .map(k => k[field])
    .filter(Boolean);
  return [...new Set(values)].join(', ');
}

// Every student, their parents, and staff, deduped by email (a parent who's
// also staff keeps whichever role they were added under first). Role/grade/
// classroom filtering down from this full set happens the same way the
// Directory page's Everyone tab does it - via matchesFilters, driven by the
// role chips and the Grade/Classroom/Tags dropdowns.
function emailEntries() {
  const rows = [];
  const seen = new Set();
  const add = (p, role, grade, classroom) => {
    // A masked email is a Veracross placeholder nobody can actually reach - it has no
    // place in a mailing list, so this person is left out of it entirely rather than
    // appearing with a blank or fake address.
    if (p.emailMasked || seen.has(p.email)) {
      return;
    }
    seen.add(p.email);
    rows.push({p, role, grade, classroom});
  };
  const students = state.model.people
    .filter(p => p.isStudent)
    .sort((a, b) => a.fullName.localeCompare(b.fullName));
  for (const s of students) {
    add(s, 'Student', s.grade || '', s.classroom || '');
    for (const family of familiesOf(s)) {
      for (const email of family.adultEmails || []) {
        const parent = byEmail[email];
        if (parent) {
          add(parent, 'Parent', kidsField(parent, 'grade'), kidsField(parent, 'classroom'));
        }
      }
    }
  }
  const staff = state.model.people
    .filter(p => p.isStaff)
    .sort((a, b) => a.fullName.localeCompare(b.fullName));
  for (const s of staff) {
    add(s, 'Staff', '', '');
  }
  return rows;
}

// The unified "list" page: a tag's people (/people?tag=X) or literally
// everyone (/email-list, kept as the URL for continuity) viewed as Faces,
// Emails, or Map - the same filtered data underneath either way, just a
// different display of it. Emails is the fullest-featured view (CSV export,
// per-column copy, a column picker) since that's what this page grew out of.
export function renderListPage() {
  const main = resetMain();

  const title = state.filterTags.size ? [...state.filterTags].join(', ') : 'Everyone';

  const pageHeader = el('div', 'page-header container page-header-list');
  const titleWrap = el('div');
  titleWrap.append(el('h1', 'page-title', title));
  pageHeader.append(titleWrap);
  pageHeader.append(tagListViewSwitch(() => renderGrid()));
  main.append(pageHeader);

  const content = el('div', 'content container');
  const header = el('div', 'content-header content-header-solo');
  const controls = el('div', 'controls');
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
  const facetFilters = el('div', 'facet-filters');
  facetFilters.append(
    facetDropdown('Grade', gradeOptions(), state.filterGrades, () => renderGrid()),
    facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, () => renderGrid()),
  );
  if (tagNames().length) {
    facetFilters.append(facetDropdown('Tags', tagNames(), state.filterTags, () => renderGrid()));
  }
  controls.append(facetFilters);
  // Small-screen stand-in for the Grade/Classroom/Tags dropdowns above, same as
  // the Directory page's mobile-filter (see renderPeople) - collapses them into
  // one funnel-icon button so the controls row doesn't wrap across several lines.
  const mobileFilter = filterControl(() => renderGrid(), {role: false, city: false, pronouns: false, newToHelios: false});
  mobileFilter.classList.add('mobile-filter');
  controls.append(mobileFilter);
  // Only meaningful for a single tag - with several selected at once (or
  // none, as on the plain Everyone list) there's no one list to pull
  // relatives in from.
  if (state.filterTags.size === 1) {
    const [activeTag] = state.filterTags;
    controls.append(facetDropdown('Include', tagRelationOptions, state.filterTagRelations, () => {
      saveTagRelations(activeTag, state.filterTagRelations);
      renderGrid();
    }));
  }
  const download = el('a', 'filter-button email-download');
  download.title = 'Download what the list currently shows';
  download.append(svg('download'), el('span', '', 'CSV'));
  controls.append(search, download);
  header.append(controls);
  content.append(header);

  const grid = el('div');
  content.append(grid);
  main.append(content);

  // Emails-view state that should survive a search/filter change, or a trip
  // through Faces/Map and back, rather than resetting on every render.
  const selectedColumns = new Set(emailColumns.map((c, i) => i));
  let currentRows = [];
  const copyColumns = el('button', 'email-copy-columns');
  copyColumns.title = 'Copy the checked columns to the clipboard';
  copyColumns.append(svg('copy'));
  copyColumns.addEventListener('click', () => {
    const cols = emailColumns.filter((c, i) => selectedColumns.has(i));
    if (!cols.length || !currentRows.length) {
      return;
    }
    const text = [cols.map(c => c.label).join('\t')]
      .concat(currentRows.map(r => cols.map(c => c.get(r)).join('\t')))
      .join('\n');
    navigator.clipboard.writeText(text);
    copyColumns.classList.add('copied');
    copyColumns.replaceChildren(svg('check'));
    setTimeout(() => {
      copyColumns.classList.remove('copied');
      copyColumns.replaceChildren(svg('copy'));
    }, 1200);
  });

  function renderEmailsTable(container, rows) {
    container.className = 'email-holder';
    const table = el('table', 'email-table');
    const thead = el('thead');
    const headRow = el('tr');
    const leadTh = el('th', 'email-copy-cell');
    leadTh.append(copyColumns);
    headRow.append(leadTh);
    emailColumns.forEach((c, i) => {
      const th = el('th');
      const pick = el('label', 'column-pick');
      const checkbox = el('input');
      checkbox.type = 'checkbox';
      checkbox.checked = selectedColumns.has(i);
      checkbox.addEventListener('change', () => {
        if (checkbox.checked) {
          selectedColumns.add(i);
        } else {
          selectedColumns.delete(i);
        }
      });
      pick.append(el('span', '', c.label), checkbox);
      th.append(pick, copyGlyph(rows.map(c.get).filter(Boolean).join('\n')));
      headRow.append(th);
    });
    headRow.append(el('th'));
    thead.append(headRow);
    table.append(thead);
    const tbody = el('tbody');
    rows.forEach((r, i) => {
      const tr = el('tr');
      const num = el('td', 'email-num');
      num.append(el('span', '', String(i + 1)), copyGlyph(emailColumns.map(c => c.get(r)).join('\t')));
      tr.append(num);
      const nameCell = el('td', 'email-name');
      const nameLink = el('a', '', r.p.fullName);
      nameLink.href = personLink(r.p);
      nameCell.append(nameLink, copyGlyph(r.p.fullName));
      tr.append(nameCell);
      for (const c of emailColumns.slice(1)) {
        const td = el('td', '', c.get(r));
        if (c.get(r)) {
          td.append(copyGlyph(c.get(r)));
        }
        tr.append(td);
      }
      const tagCell = el('td', 'email-tag');
      tagCell.append(tagControl(r.p.email, 'tag-wrap', 'row-tag', () => {
        if (state.filterTags.size) {
          renderGrid();
        }
      }));
      tr.append(tagCell);
      tbody.append(tr);
    });
    table.append(tbody);
    container.append(table);
  }

  function renderGrid() {
    grid.replaceChildren();
    grid.className = '';
    const rows = emailEntries()
      .filter(r => (r.p.fullName.toLowerCase().includes(state.q) || r.p.email.toLowerCase().includes(state.q)) && matchesFilters(r.p));
    currentRows = rows;
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
  input.focus();
}
