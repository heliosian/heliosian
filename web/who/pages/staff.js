import {state, model, isStaff, departmentName} from '../state.js';
import {paletteColor} from '../dom.js';
import {el, svg} from '/elements.js';
import {personLink, photoWithTag, applyRingColor, photoOrInitials, personPhotoUrl} from '../people.js';
import {matchesFilters} from '../filters.js';

function departmentOrder() {
  return model.departments.map(d => d.name);
}

function byDepartment(a, b) {
  const departments = departmentOrder();
  const ia = departments.indexOf(a);
  const ib = departments.indexOf(b);
  return (ia < 0 ? departments.length : ia) - (ib < 0 ? departments.length : ib);
}

export function renderStaff(grid, autoFit) {
  grid.className = '';
  const staff = model.people.filter(p =>
    isStaff(p) && `${p.name_show} ${p.job_title || ''}`.toLowerCase().includes(state.q) && matchesFilters(p));
  const groups = new Map();
  for (const p of staff) {
    const dept = departmentName(p) || 'Staff';
    if (!groups.has(dept)) {
      groups.set(dept, []);
    }
    groups.get(dept).push(p);
  }
  let count = 0;
  for (const dept of [...groups.keys()].sort(byDepartment)) {
    if (state.staffDeptExcluded.has(dept)) {
      continue;
    }
    grid.append(el('h2', 'staff-section', dept));
    const deptGrid = el('div', 'people-grid directory-grid' + (autoFit ? ' autofit' : ''));
    for (const p of groups.get(dept)) {
      const card = el('a', 'person-card');
      card.href = personLink(p);
      card.append(photoWithTag(applyRingColor(photoOrInitials(personPhotoUrl(p), p.name_show, 'person-photo'), p), p.id));
      card.append(el('div', 'role-label role-label-staff', p.job_title || 'Staff'));
      card.append(el('div', 'person-name', p.name_show));
      deptGrid.append(card);
      count++;
    }
    grid.append(deptGrid);
  }
  return count;
}

function departmentChips(rerender) {
  const present = new Set(model.people.filter(isStaff).map(p => departmentName(p) || 'Staff'));
  const ordered = [...present].sort(byDepartment);
  for (const excluded of [...state.staffDeptExcluded]) {
    if (!ordered.includes(excluded)) {
      state.staffDeptExcluded.delete(excluded);
    }
  }
  const bar = el('div', 'chip-row');
  for (const dept of ordered) {
    const btn = el('button', 'chip-toggle' + (!state.staffDeptExcluded.has(dept) ? ' active' : ''));
    btn.type = 'button';
    btn.style.setProperty('--chip-color', paletteColor(dept));
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

export function staffPage() {
  const page = document.createDocumentFragment();

  const pageHeader = el('div', 'page-header-plain container');
  pageHeader.append(el('h1', 'page-title', 'Staff'));
  page.append(pageHeader);

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
  page.append(content);

  function renderList() {
    list.replaceChildren();
    if (renderStaff(list, true) === 0) {
      list.append(el('div', 'empty', 'No matches.'));
    }
  }
  renderList();
  return page;
}
