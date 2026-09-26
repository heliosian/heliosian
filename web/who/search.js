import {state, byEmail} from './state.js';
import {el, withFrom, slugify, firstName} from './dom.js';
import {familyOf} from './families.js';
import {personLink, photoOrInitials, personPhotoUrl, roleLabel, gradeChain} from './people.js';
import {gradeImage} from './pages/classrooms.js';
import {searchInput} from '/shell.js';

function personSearchSubtitle(p) {
  if (p.isStudent) {
    return gradeChain(p);
  }
  if (p.isStaff) {
    return p.jobTitle || roleLabel(p);
  }
  const family = familyOf(p);
  const kids = ((family && family.kidEmails) || []).map(e => byEmail[e]).filter(Boolean);
  if (kids.length) {
    const names = kids.map(k => firstName(k.fullName)).join(', ');
    const grades = [...new Set(kids.map(k => k.grade).filter(Boolean))].join(', ');
    return `Parent to ${names}${grades ? ` (${grades})` : ''}`;
  }
  return roleLabel(p);
}

function renderGlobalSearchResults(resultsEl, query) {
  const q = query.trim().toLowerCase();
  resultsEl.replaceChildren();
  if (!q || !state.model) {
    resultsEl.hidden = true;
    return;
  }
  const people = state.model.people
    .filter(p => p.fullName.toLowerCase().includes(q) || p.email.toLowerCase().includes(q))
    .slice(0, 8);
  const grades = state.model.grades
    .filter(g => g.name.toLowerCase().includes(q))
    .slice(0, 5);
  const classrooms = state.model.classrooms
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
    row.append(photoOrInitials(personPhotoUrl(p), p.fullName, 'gsearch-avatar'));
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', p.fullName));
    const sub = personSearchSubtitle(p);
    if (sub) {
      info.append(el('div', 'gsearch-sub', sub));
    }
    row.append(info);
    return row;
  });

  group('Grades', grades, g => {
    const row = el('a', 'gsearch-result');
    row.href = withFrom('/grades/' + slugify(g.name));
    row.append(photoOrInitials(gradeImage(g.name), g.name, 'gsearch-avatar'));
    const info = el('div', 'gsearch-info');
    info.append(el('div', 'gsearch-title', g.name));
    row.append(info);
    return row;
  });

  group('Gradebands', classrooms, c => {
    const row = el('a', 'gsearch-result');
    row.href = withFrom('/classrooms/' + slugify(c.name));
    row.append(photoOrInitials(c.imageUrl, c.name, 'gsearch-avatar'));
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
