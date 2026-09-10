import {state, byEmail, tags} from './state.js';
import {el, svg, segments} from './dom.js';
import {familyOf, familiesOf} from './families.js';
import {tagsOf, tagNames} from './tags.js';

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

// Role chips currently show on the Directory page's Everyone tab, on every
// list page (renderListPage: a tag's list, or the plain Everyone list at
// /email-list), and on the Invites page - not on Directory's Families tab,
// which has no chips of its own to reveal that anything is filtered. Keyed off
// the route rather than state.tab so a stale tab value left over from a
// different page can't make filterRoleExcluded apply (or not) on the wrong
// page.
function roleChipsVisible() {
  const seg = segments();
  if (seg[0] === 'email-list' || seg[0] === 'greenvelope') {
    return true;
  }
  if (seg[0] === 'people') {
    // A tag list (renderListPage) always shows the chips regardless of
    // state.tab, which it doesn't touch - only the plain Directory (renderPeople)
    // gates them on actually being on its own Everyone tab.
    return state.filterTags.size > 0 || state.tab === 'everyone';
  }
  return false;
}

export const tagRelationOptions = ['Parents', 'Children', 'Siblings'];

// Whether p only belongs on a single-tag list by way of a selected relation
// to someone directly tagged - a parent of a tagged kid, a kid of a tagged
// parent, or another kid (a sibling) in a tagged kid's family. Only applies
// with exactly one active tag; with several selected at once there's no
// single list to pull relatives in from.
function tagRelatedMatch(p) {
  if (state.filterTags.size !== 1 || !state.filterTagRelations.size) {
    return false;
  }
  const [tag] = state.filterTags;
  const tagged = tags[tag] || [];
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
  const tagOK = !state.filterTags.size || tagsOf(p.email).some(t => state.filterTags.has(t)) || tagRelatedMatch(p);
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

// Role as always-visible toggle chips, all on by default. state.filterRoleExcluded
// tracks only the deselected roles (mirroring rosterSectionExcluded's exclusion-set
// approach) - kept separate from state.filterRoles, the inclusion set the Role
// checkboxes elsewhere use, so the two can't fight over what an empty set means.
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

// .filter-panel is CSS-anchored to its wrap's right edge (right:0), which only
// fits on screen when the trigger button sits near the right side of the
// viewport - true for the old lone "Filter" button, not for Grade/Classroom/
// Tags now sitting further left, especially once the controls row wraps on
// mobile. Clamping with an explicit left (converted back to wrap-relative,
// since the panel is absolutely positioned inside its position:relative wrap)
// keeps the panel fully on screen regardless of where its button lands.
export function clampFilterPanel(wrap, panel) {
  const margin = 12;
  const wrapRect = wrap.getBoundingClientRect();
  const panelWidth = panel.offsetWidth;
  let left = wrapRect.right - panelWidth;
  left = Math.max(margin, Math.min(left, window.innerWidth - margin - panelWidth));
  panel.style.left = `${left - wrapRect.left}px`;
  panel.style.right = 'auto';
}

// A standalone single-facet dropdown (Grade, Classroom) - the same checkbox
// list a filterControl section would show, but its own button so it doesn't
// need the drill-into-a-section step.
export function facetDropdown(label, values, set, rerender) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button facet-button');
  const labelSpan = el('span', '', label);
  button.append(labelSpan, svg('chevron'));
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
    const row = el('label', 'filter-option');
    const box = el('input');
    box.type = 'checkbox';
    box.checked = set.has(v);
    box.addEventListener('change', () => {
      if (box.checked) {
        set.add(v);
      } else {
        set.delete(v);
      }
      updateLabel();
      rerender();
    });
    row.append(el('span', '', v), box);
    body.append(row);
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

// options lets a caller opt out of a section (or the "New to Helios" toggle)
// that it surfaces some other way - the Directory page and every list page
// pull Role out into chips and Grade/Classroom/Tags into their own dropdowns
// (see roleChips and facetDropdown above), while the standalone Map page is
// the last one still using the full panel.
export function filterControl(rerender, options = {}) {
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button');
  button.append(svg('filter'), el('span', '', 'Filter'), svg('chevron'));
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
    sections.push({label: 'Class', values: state.model.classrooms.map(c => c.name), set: state.filterClassrooms});
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
  if (options.tags !== false && tagNames().length) {
    sections.push({label: 'Tags', values: tagNames(), set: state.filterTags});
  }
  for (const s of sections) {
    const head = el('div', 'filter-section');
    head.append(el('span', '', s.label), svg('chevron'));
    const body = el('div', 'filter-options');
    body.hidden = true;
    head.addEventListener('click', () => {
      body.hidden = !body.hidden;
      head.classList.toggle('open', !body.hidden);
    });
    for (const v of s.values) {
      const row = el('label', 'filter-option');
      const box = el('input');
      box.type = 'checkbox';
      box.checked = s.set.has(v);
      box.addEventListener('change', () => {
        if (box.checked) {
          s.set.add(v);
        } else {
          s.set.delete(v);
        }
        rerender();
      });
      row.append(el('span', '', v), box);
      body.append(row);
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
    rerender();
  });
  const done = el('button', 'filter-done', 'Done');
  done.addEventListener('click', () => {
    panel.hidden = true;
    button.classList.remove('open');
  });
  footer.append(clear, done);
  panel.append(footer);

  wrap.append(button, panel);
  return wrap;
}

export function closeFilterPanels() {
  for (const panel of document.querySelectorAll('.filter-panel')) {
    panel.hidden = true;
    panel.parentElement.querySelector('.filter-button').classList.remove('open');
  }
}
