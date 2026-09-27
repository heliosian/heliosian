import {state, byEmail, lists, tags} from '../state.js';
import {csvField, copyGlyph} from '../dom.js';
import {el, svg} from '/elements.js';
import {familiesOf, familyOf, familySearchText} from '../families.js';
import {personCard, personLink, guestCard, guestPerson} from '../people.js';
import {tagControl, onTagsChange, listLabel, tagFacetOptions, listSource, sharedOf, managersOf, manageControl, selectedGuests} from '../tags.js';
import {saveTagRelations} from '../storage.js';
import {matchesFilters, familyMatchesFilters, roleChips, facetDropdown, gradeOptions, filterControl, tagRelationOptionsFor, familyDropdown} from '../filters.js';
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

function emailEntries() {
  const rows = [];
  const seen = new Set();
  const add = (p, role, grade, classroom) => {
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

export function listPage() {
  const page = document.createDocumentFragment();

  const title = state.filterTags.size ? [...state.filterTags].map(listLabel).join(', ') : 'Everyone';
  const smart = state.filterTags.size === 1 ? lists[[...state.filterTags][0]] : null;
  const ownTag = state.filterTags.size === 1 && !smart && tags[[...state.filterTags][0]] ? [...state.filterTags][0] : null;
  const sharedTag = state.filterTags.size === 1 ? sharedOf([...state.filterTags][0]) : null;

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
  const chip = email => {
    const p = byEmail[email];
    const a = el('a', 'tag-chip person-chip', p ? p.fullName : email);
    if (p) {
      a.href = personLink(p);
    }
    return a;
  };
  const chips = (target, emails, joiner) => {
    emails.forEach((email, i) => {
      if (i) {
        target.append(el('span', '', i === emails.length - 1 ? ` ${joiner} ` : ', '));
      }
      target.append(chip(email));
    });
  };
  const ownership = el('div', 'page-subtitle tag-ownership');
  const paintOwnership = () => {
    ownership.replaceChildren();
    if (sharedTag) {
      const others = sharedTag.managers.filter(e => e !== state.model.user.email);
      ownership.append(svg('families'), chip(sharedTag.owner), el('span', '', "'s tag, shared with you" + (others.length ? ' and ' : '')));
      chips(ownership, others, 'and');
    } else if (ownTag && managersOf(ownTag).length) {
      ownership.append(svg('families'), el('span', '', 'Managed with '));
      chips(ownership, managersOf(ownTag), 'and');
    } else {
      ownership.hidden = true;
      return;
    }
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
  const familyOptions = state.filterTags.size === 1 ? tagRelationOptionsFor([...state.filterTags][0]) : [];
  if (familyOptions.length) {
    const [activeTag] = state.filterTags;
    lead.append(familyDropdown(familyOptions, state.filterTagRelations, () => {
      saveTagRelations(activeTag, state.filterTagRelations);
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
      facetDropdown('Grade', gradeOptions(), state.filterGrades, () => renderGrid()),
      facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, () => renderGrid()),
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
      tagsFacet = facetDropdown('Tags', tagFacetOptions(), state.filterTags, () => renderGrid());
      facetFilters.append(tagsFacet);
    }
    const next = filterControl(() => renderGrid(), {role: false, city: false, pronouns: false, newToHelios: false});
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
  if (ownTag || sharedTag) {
    lead.append(manageControl([...state.filterTags][0], paintOwnership));
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
      if (!r.p.guest) {
        tagCell.append(tagControl(r.p.email, 'tag-wrap', 'row-tag', () => {
          if (state.filterTags.size) {
            renderGrid();
          }
        }));
      }
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
      .filter(r => (r.p.fullName.toLowerCase().includes(state.q) || r.p.email.toLowerCase().includes(state.q)) && matchesFilters(r.p))
      .concat(selectedGuests().map(g => ({p: guestPerson(g), role: 'Guest', grade: '', classroom: ''})));
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
      grid.append(r.p.guest ? guestCard(r.p.guest) : personCard(r.p));
    }
  }
  renderGrid();
  // The router mounts the page after this returns.
  queueMicrotask(() => input.focus());
  return page;
}
