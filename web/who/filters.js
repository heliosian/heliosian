import {state, byEmail} from './state.js';
import {segments} from './dom.js';
import {el, svg} from '/elements.js';
import {familyOf, familiesOf} from './families.js';
import {members, tagFacetOptions} from './tags.js';

function personFacets(p, field) {
  if (p.isStudent) {
    return p[field] ? [p[field]] : [];
  }
  if (p.isParent) {
    const family = familyOf(p);
    return ((family && family.kidEmails) || []).map(e => byEmail[e]).filter(Boolean).map(k => k[field]).filter(Boolean);
  }
  return [];
}

function cityOf(p) {
  const family = familyOf(p);
  if (!family || !family.address) {
    return '';
  }
  const parts = family.address.split(',').map(s => s.trim());
  return parts.length >= 2 ? parts[parts.length - 2] : parts[0];
}

export function anyFiltersActive() {
  return Boolean(state.filterGrades.size || state.filterClassrooms.size || state.filterRoles.size ||
    state.filterRoleExcluded.size || state.filterCities.size || state.filterPronouns.size ||
    state.filterTags.size || state.filterNew);
}

function roleChipsVisible() {
  const seg = segments();
  if (seg[0] === 'email-list' || seg[0] === 'greenvelope') {
    return true;
  }
  if (seg[0] === 'people') {
    return state.filterTags.size > 0 || state.tab === 'everyone';
  }
  return false;
}

export function tagRelationOptionsFor(tag) {
  const tagged = members(tag).map(e => byEmail[e]).filter(Boolean);
  const hasKids = tagged.some(p => p.isStudent);
  const hasParents = tagged.some(p => p.isParent);
  const options = [];
  if (hasKids) {
    options.push('Parents');
  }
  if (hasParents) {
    options.push('Children');
  }
  if (hasKids) {
    options.push('Siblings');
  }
  for (const chosen of [...state.filterTagRelations]) {
    if (!options.includes(chosen)) {
      state.filterTagRelations.delete(chosen);
    }
  }
  return options;
}

function tagRelatedMatch(p) {
  if (state.filterTags.size !== 1 || !state.filterTagRelations.size) {
    return false;
  }
  const [tag] = state.filterTags;
  const tagged = members(tag);
  for (const family of familiesOf(p)) {
    if (state.filterTagRelations.has('Parents') && p.isParent &&
      (family.kidEmails || []).some(e => tagged.includes(e))) {
      return true;
    }
    if (state.filterTagRelations.has('Children') && p.isStudent &&
      (family.adultEmails || []).some(e => tagged.includes(e))) {
      return true;
    }
    if (state.filterTagRelations.has('Siblings') && p.isStudent &&
      (family.kidEmails || []).some(e => e !== p.email && tagged.includes(e))) {
      return true;
    }
  }
  return false;
}

export function matchesFilters(p) {
  const gradeOK = !state.filterGrades.size || personFacets(p, 'grade').some(g => state.filterGrades.has(g));
  const classOK = !state.filterClassrooms.size || personFacets(p, 'classroom').some(c => state.filterClassrooms.has(c));
  const roleOK = (!state.filterRoles.size ||
    (state.filterRoles.has('Student') && p.isStudent) ||
    (state.filterRoles.has('Parent') && p.isParent) ||
    (state.filterRoles.has('Staff') && p.isStaff)) &&
    (!roleChipsVisible() ||
    (p.isStudent && !state.filterRoleExcluded.has('Student')) ||
    (p.isParent && !state.filterRoleExcluded.has('Parent')) ||
    (p.isStaff && !state.filterRoleExcluded.has('Staff')));
  const cityOK = !state.filterCities.size || state.filterCities.has(cityOf(p));
  const pronounsOK = !state.filterPronouns.size || (p.pronouns && state.filterPronouns.has(p.pronouns.toLowerCase()));
  const newOK = !state.filterNew || p.isNew;
  const tagOK = !state.filterTags.size || [...state.filterTags].some(k => members(k).includes(p.email)) || tagRelatedMatch(p);
  return gradeOK && classOK && roleOK && cityOK && pronounsOK && newOK && tagOK;
}

export function familyMatchesFilters(key) {
  const family = state.model.families[key];
  if (!family) {
    return !anyFiltersActive();
  }
  const members = [...(family.kidEmails || []), ...(family.adultEmails || [])].map(e => byEmail[e]).filter(Boolean);
  return !anyFiltersActive() || members.some(matchesFilters);
}

export function gradeOptions() {
  const present = new Set(state.model.people.filter(p => p.isStudent).map(p => p.grade).filter(Boolean));
  return state.model.grades.map(g => g.name).filter(n => present.has(n));
}

function cityOptions() {
  return [...new Set(state.model.people.map(cityOf).filter(Boolean))].sort();
}

function pronounOptions() {
  return [...new Set(state.model.people.map(p => p.pronouns).filter(Boolean).map(p => p.toLowerCase()))].sort();
}

const roleChipFacets = [
  ['Student', 'Students'],
  ['Parent', 'Parents'],
  ['Staff', 'Staff'],
];

export function roleChips(rerender) {
  const bar = el('div', 'chip-row');
  for (const [key, label] of roleChipFacets) {
    const btn = el('button', 'chip-toggle chip-toggle-' + key.toLowerCase() + (!state.filterRoleExcluded.has(key) ? ' active' : ''));
    btn.type = 'button';
    btn.append(el('span', '', label));
    btn.addEventListener('click', () => {
      if (state.filterRoleExcluded.has(key)) {
        state.filterRoleExcluded.delete(key);
      } else {
        state.filterRoleExcluded.add(key);
      }
      btn.classList.toggle('active');
      rerender();
    });
    bar.append(btn);
  }
  return bar;
}

export function clampFilterPanel(wrap, panel) {
  const margin = 12;
  const wrapRect = wrap.getBoundingClientRect();
  const panelWidth = panel.offsetWidth;
  const column = wrap.closest('.container');
  const minLeft = Math.max(margin, column ? column.getBoundingClientRect().left + parseFloat(getComputedStyle(column).paddingLeft) : 0);
  let left = wrapRect.right - panelWidth;
  left = Math.max(minLeft, Math.min(left, window.innerWidth - margin - panelWidth));
  panel.style.left = `${left - wrapRect.left}px`;
  panel.style.right = 'auto';
}

function optionRow(v, set, onChange) {
  const {value, label, icon} = typeof v === 'string' ? {value: v, label: v} : v;
  const row = el('label', 'filter-option');
  const box = el('input');
  box.type = 'checkbox';
  box.checked = set.has(value);
  box.addEventListener('change', () => {
    if (box.checked) {
      set.add(value);
    } else {
      set.delete(value);
    }
    onChange();
  });
  if (icon) {
    row.append(svg(icon));
  }
  row.append(el('span', '', label), box);
  return row;
}

export function familyDropdown(values, set, onChange) {
  const toggle = value => {
    if (set.has(value)) {
      set.delete(value);
    } else {
      set.add(value);
    }
    onChange();
  };
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button facet-button');
  const labelSpan = el('span', '', 'Add');
  button.append(svg('families'), labelSpan, svg('chevron-down'));
  const panel = el('div', 'filter-panel family-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
    if (!panel.hidden) {
      clampFilterPanel(wrap, panel);
    }
  });
  const updateLabel = () => {
    labelSpan.textContent = set.size ? `Add (${set.size})` : 'Add';
  };

  const head = el('div', 'family-head');
  head.append(svg('families'));
  const text = el('div');
  text.append(el('div', 'family-title', 'Also add family members'),
    el('div', 'family-desc', 'Add parents, children, or siblings of the people already matched above.'));
  head.append(text);
  panel.append(head);

  const body = el('div', 'family-options');
  for (const value of values) {
    const row = el('label', 'family-option');
    const box = el('input');
    box.type = 'checkbox';
    box.checked = set.has(value);
    box.addEventListener('change', () => {
      toggle(value);
      updateLabel();
    });
    row.append(box, el('span', '', `Their ${value.toLowerCase()}`));
    body.append(row);
  }
  panel.append(body);

  const note = el('div', 'family-note');
  note.append(svg('info'), el('span', '', 'Adds these relatives to the people already matched above.'));
  panel.append(note);

  updateLabel();
  wrap.append(button, panel);
  return wrap;
}

export function facetDropdown(label, values, set, rerender) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button facet-button');
  const labelSpan = el('span', '', label);
  button.append(labelSpan, svg('chevron-down'));
  const panel = el('div', 'filter-panel facet-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
    if (!panel.hidden) {
      clampFilterPanel(wrap, panel);
    }
  });

  const updateLabel = () => {
    labelSpan.textContent = set.size ? `${label} (${set.size})` : label;
  };

  const body = el('div', 'filter-options');
  for (const v of values) {
    body.append(optionRow(v, set, () => {
      updateLabel();
      rerender();
    }));
  }
  panel.append(body);

  const footer = el('div', 'filter-footer');
  const clear = el('button', 'filter-clear', 'Clear');
  clear.addEventListener('click', () => {
    set.clear();
    for (const box of body.querySelectorAll('input')) {
      box.checked = false;
    }
    updateLabel();
    rerender();
  });
  const done = el('button', 'filter-done', 'Done');
  done.addEventListener('click', () => {
    panel.hidden = true;
    button.classList.remove('open');
  });
  footer.append(clear, done);
  panel.append(footer);

  updateLabel();
  wrap.append(button, panel);
  return wrap;
}

export function filterControl(rerender, options = {}) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button');
  const labelSpan = el('span', '', 'Filter');
  button.append(svg('filter'), labelSpan, svg('chevron-down'));
  const panel = el('div', 'filter-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
    if (!panel.hidden) {
      clampFilterPanel(wrap, panel);
    }
  });

  const sections = [];
  if (options.role !== false) {
    sections.push({label: 'Role', values: ['Student', 'Parent', 'Staff'], set: state.filterRoles});
  }
  if (options.classroom !== false) {
    sections.push({label: 'Classroom', values: state.model.classrooms.map(c => c.name), set: state.filterClassrooms});
  }
  if (options.grade !== false) {
    sections.push({label: 'Grade', values: gradeOptions(), set: state.filterGrades});
  }
  if (options.city !== false) {
    sections.push({label: 'City', values: cityOptions(), set: state.filterCities});
  }
  if (options.pronouns !== false) {
    sections.push({label: 'Pronouns', values: pronounOptions(), set: state.filterPronouns});
  }
  if (options.tags !== false && tagFacetOptions().length) {
    sections.push({label: 'Tags', values: tagFacetOptions(), set: state.filterTags});
  }
  const updateLabels = () => {
    let total = 0;
    for (const s of sections) {
      total += s.set.size;
      s.labelSpan.textContent = s.set.size ? `${s.label} (${s.set.size})` : s.label;
    }
    labelSpan.textContent = total ? `Filter (${total})` : 'Filter';
  };
  const changed = () => {
    updateLabels();
    rerender();
  };
  for (const s of sections) {
    const head = el('div', 'filter-section');
    s.labelSpan = el('span', '', s.label);
    head.append(s.labelSpan, svg('chevron-down'));
    const body = el('div', 'filter-options');
    body.hidden = true;
    head.addEventListener('click', () => {
      body.hidden = !body.hidden;
      head.classList.toggle('open', !body.hidden);
    });
    for (const v of s.values) {
      body.append(optionRow(v, s.set, changed));
    }
    panel.append(head, body);
  }

  if (options.newToHelios !== false) {
    const toggleRow = el('label', 'filter-toggle-row');
    toggleRow.append(el('span', '', 'New to Helios'));
    const toggle = el('input', 'filter-switch');
    toggle.type = 'checkbox';
    toggle.checked = state.filterNew;
    toggle.addEventListener('change', () => {
      state.filterNew = toggle.checked;
      rerender();
    });
    toggleRow.append(toggle);
    panel.append(toggleRow);
  }

  const footer = el('div', 'filter-footer');
  const clear = el('button', 'filter-clear', 'Clear all');
  clear.addEventListener('click', () => {
    if (options.grade !== false) {
      state.filterGrades.clear();
    }
    if (options.classroom !== false) {
      state.filterClassrooms.clear();
    }
    if (options.role !== false) {
      state.filterRoles.clear();
    }
    if (options.city !== false) {
      state.filterCities.clear();
    }
    if (options.pronouns !== false) {
      state.filterPronouns.clear();
    }
    if (options.tags !== false) {
      state.filterTags.clear();
    }
    if (options.newToHelios !== false) {
      state.filterNew = false;
    }
    for (const box of panel.querySelectorAll('input')) {
      box.checked = false;
    }
    changed();
  });
  const done = el('button', 'filter-done', 'Done');
  done.addEventListener('click', () => {
    panel.hidden = true;
    button.classList.remove('open');
  });
  footer.append(clear, done);
  panel.append(footer);

  updateLabels();
  wrap.append(button, panel);
  return wrap;
}

export function closeFilterPanels() {
  for (const panel of document.querySelectorAll('.filter-panel')) {
    panel.hidden = true;
    panel.parentElement.querySelector('.filter-button').classList.remove('open');
  }
}
