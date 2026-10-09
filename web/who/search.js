import {model, emailOf, isStudent, isStaff, familyOf, kidsOf, gradeName, gradePath, classroomPath} from './state.js';
import {withFrom, firstName} from './dom.js';
import {el} from '/elements.js';
import {personLink, photoOrInitials, personPhotoUrl, roleLabel, gradeChain} from './people.js';
import {gradeImage, classroomImage} from './pages/classrooms.js';
import {searchInput} from '/shell.js';

function personSearchSubtitle(p) {
  if (isStudent(p)) {
    return gradeChain(p);
  }
  if (isStaff(p)) {
    return p.job_title || roleLabel(p);
  }
  const family = familyOf(p);
  const kids = family ? kidsOf(family) : [];
  if (kids.length) {
    const names = kids.map(k => firstName(k.name_show)).join(', ');
    const grades = [...new Set(kids.map(gradeName).filter(Boolean))].join(', ');
    return `Parent to ${names}${grades ? ` (${grades})` : ''}`;
  }
  return roleLabel(p);
}

function renderGlobalSearchResults(resultsEl, query) {
  const q = query.trim().toLowerCase();
  resultsEl.replaceChildren();
  if (!q || !model.people.length) {
    resultsEl.hidden = true;
    return;
  }
  const people = model.people
    .filter(p => p.name_show.toLowerCase().includes(q) || emailOf(p).toLowerCase().includes(q))
    .slice(0, 8);
  const grades = model.grades
    .filter(g => g.name.toLowerCase().includes(q))
    .slice(0, 5);
  const classrooms = model.classrooms
    .filter(c => c.name.toLowerCase().includes(q))
    .slice(0, 5);

  if (!people.length && !grades.length && !classrooms.length) {
    resultsEl.append(el('div', 'gsearch-empty', 'No matches.'));
    resultsEl.hidden = false;
    return;
  }

  function group(label, items, buildRow) {
    if (!items.length) {
      return;
    }
    resultsEl.append(el('div', 'gsearch-label', label));
    for (const item of items) {
      resultsEl.append(buildRow(item));
    }
  }

  group('People', people, p => {
    const row = el('a', 'gsearch-result');
    row.href = personLink(p);
    row.append(photoOrInitials(personPhotoUrl(p), p.name_show, 'gsearch-avatar'));
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', p.name_show));
    const sub = personSearchSubtitle(p);
    if (sub) {
      info.append(el('div', 'gsearch-sub', sub));
    }
    row.append(info);
    return row;
  });

  group('Grades', grades, g => {
    const row = el('a', 'gsearch-result');
    row.href = withFrom(gradePath(g));
    row.append(photoOrInitials(gradeImage(g), g.name, 'gsearch-avatar'));
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', g.name));
    row.append(info);
    return row;
  });

  group('Gradebands', classrooms, c => {
    const row = el('a', 'gsearch-result');
    row.href = withFrom(classroomPath(c));
    row.append(photoOrInitials(classroomImage(c), c.name, 'gsearch-avatar'));
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', c.name));
    row.append(info);
    return row;
  });

  const first = resultsEl.querySelector('.gsearch-result');
  if (first) {
    first.classList.add('active');
  }

  resultsEl.hidden = false;
}

function goToActiveResult(resultsEl) {
  const active = resultsEl.querySelector('.gsearch-result.active');
  if (active) {
    location.href = active.href;
  }
}

function moveActiveResult(resultsEl, delta) {
  const results = [...resultsEl.querySelectorAll('.gsearch-result')];
  if (!results.length) {
    return;
  }
  const current = results.findIndex(r => r.classList.contains('active'));
  const next = (current + delta + results.length) % results.length;
  if (current >= 0) {
    results[current].classList.remove('active');
  }
  results[next].classList.add('active');
  results[next].scrollIntoView({block: 'nearest'});
}

function handleSearchNavKeys(resultsEl, e) {
  if (e.key === 'Enter') {
    goToActiveResult(resultsEl);
  } else if (e.key === 'ArrowDown') {
    e.preventDefault();
    moveActiveResult(resultsEl, 1);
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    moveActiveResult(resultsEl, -1);
  }
}

export function searchResults() {
  return document.querySelector('#search-results');
}

export function initSearch() {
  const input = searchInput();
  input.addEventListener('input', () => {
    renderGlobalSearchResults(searchResults(), input.value);
  });
  input.addEventListener('focus', () => {
    if (input.value.trim()) {
      renderGlobalSearchResults(searchResults(), input.value);
    }
  });
  input.addEventListener('keydown', e => handleSearchNavKeys(searchResults(), e));
}
