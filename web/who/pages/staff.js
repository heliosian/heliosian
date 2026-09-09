import {state} from '../state.js';
import {el, svg} from '../dom.js';
import {personLink, photoWithTag, applyRingColor, photoOrInitials, personPhotoUrl} from '../people.js';
import {matchesFilters} from '../filters.js';
import {resetMain} from '../chrome.js';

export function renderStaff(grid, autoFit) {
  grid.className = '';
  const staff = state.model.people.filter(p =>
    p.isStaff && `${p.fullName} ${p.jobTitle || ''}`.toLowerCase().includes(state.q) && matchesFilters(p));
  const departments = state.model.departments || [];
  const groups = new Map();
  for (const p of staff) {
    const dept = p.department || 'Staff';
    if (!groups.has(dept)) {
      groups.set(dept, []);
    }
    groups.get(dept).push(p);
  }
  const ordered = [...groups.keys()].sort((a, b) => {
    const ia = departments.indexOf(a);
    const ib = departments.indexOf(b);
    return (ia < 0 ? departments.length : ia) - (ib < 0 ? departments.length : ib);
  });
  let count = 0;
  for (const dept of ordered) {
    if (state.staffDeptExcluded.has(dept)) {
      continue;
    }
    grid.append(el('h2', 'staff-section', dept));
    const deptGrid = el('div', 'people-grid directory-grid' + (autoFit ? ' autofit' : ''));
    for (const p of groups.get(dept)) {
      const card = el('a', 'person-card');
      card.href = personLink(p);
      card.append(photoWithTag(applyRingColor(photoOrInitials(personPhotoUrl(p), p.fullName, 'person-photo'), p), p.email));
      card.append(el('div', 'role-label', p.jobTitle || 'Staff'));
      card.append(el('div', 'person-name', p.fullName));
      deptGrid.append(card);
      count++;
    }
    grid.append(deptGrid);
  }
  return count;
}

// Department toggle chips for the Staff page, mirroring roleChips: an
// exclusion set (state.staffDeptExcluded) so every department starts active,
// with stale entries pruned whenever the set of departments actually present
// among staff changes.
function departmentChips(rerender) {
  const departments = state.model.departments || [];
  const present = new Set(state.model.people.filter(p => p.isStaff).map(p => p.department || 'Staff'));
  const ordered = [...present].sort((a, b) => {
    const ia = departments.indexOf(a);
    const ib = departments.indexOf(b);
    return (ia < 0 ? departments.length : ia) - (ib < 0 ? departments.length : ib);
  });
  for (const excluded of [...state.staffDeptExcluded]) {
    if (!ordered.includes(excluded)) {
      state.staffDeptExcluded.delete(excluded);
    }
  }
  const bar = el('div', 'chip-row');
  for (const dept of ordered) {
    const btn = el('button', 'chip-toggle' + (!state.staffDeptExcluded.has(dept) ? ' active' : ''));
    btn.type = 'button';
    btn.append(el('span', '', dept));
    btn.addEventListener('click', () => {
      if (state.staffDeptExcluded.has(dept)) {
        state.staffDeptExcluded.delete(dept);
      } else {
        state.staffDeptExcluded.add(dept);
      }
      btn.classList.toggle('active');
      rerender();
    });
    bar.append(btn);
  }
  return bar;
}

export function renderStaffPage() {
  const main = resetMain();

  const pageHeader = el('div', 'page-header-plain container');
  pageHeader.append(el('h1', 'page-title', 'Staff'));
  main.append(pageHeader);

  const content = el('div', 'content container');
  const header = el('div', 'content-header content-header-solo');
  const controls = el('div', 'controls');
  controls.append(departmentChips(renderList));
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    renderList();
  });
  search.append(input);
  controls.append(search);
  header.append(controls);
  content.append(header);

  const list = el('div');
  content.append(list);
  main.append(content);

  function renderList() {
    list.replaceChildren();
    if (renderStaff(list, true) === 0) {
      list.append(el('div', 'empty', 'No matches.'));
    }
  }
  renderList();
}
