import {state, colors, peopleOf} from '../state.js';
import {withFrom, slugify, ordinal, thumbUrl, firstName, listSub, paletteColor} from '../dom.js';
import {el, svg} from '/elements.js';
import {tabStrip, tabHref} from '/tabs.js';
import {familyOf, familiesOf} from '../families.js';
import {personLink, photoWithTag, applyRingColor, photoOrInitials, personPhotoUrl, sortPeople} from '../people.js';
import {fromURL, breadcrumbs} from '../crumbs.js';
import {render} from '/router.js';

function bandGroups() {
  const groups = [];
  for (const g of state.model.grades) {
    if (!g.band || g.band === 'Eggs' || g.band === 'Alum' || g.name === 'PreK' || g.name === 'Grade 9') {
      continue;
    }
    let group = groups.find(x => x.band === g.band);
    if (!group) {
      group = {band: g.band, grades: []};
      groups.push(group);
    }
    group.grades.push(g.name);
  }
  for (const group of groups) {
    group.label = group.band === 'Hummingbirds' ? 'K' : group.grades.map(ordinal).join(' / ');
  }
  return groups;
}

export function gradeImage(gradeName) {
  const grade = state.model.grades.find(g => g.name === gradeName);
  if (grade && grade.imageUrl) {
    return grade.imageUrl;
  }
  const suffix = gradeName === 'Kindergarten' ? 'k' : gradeName.split(' ')[1];
  return '/brand/classrooms/grade-' + suffix + '.jpg';
}

function studentsOf(filter) {
  return state.model.people.filter(p => p.isStudent && filter(p));
}

function classroomBand(name) {
  const student = state.model.people.find(p => p.isStudent && p.classroom === name);
  if (!student) {
    return '';
  }
  const grade = state.model.grades.find(g => g.name === student.grade);
  return grade ? grade.band : '';
}

function listRow(image, label, title, sub, href) {
  const row = el('a', 'list-row');
  row.href = href;
  if (image) {
    const img = el('img', 'list-tile');
    img.src = image;
    img.loading = 'lazy';
    img.alt = '';
    row.append(img);
  } else {
    row.append(el('div', 'list-tile'));
  }
  const info = el('div', 'list-info');
  if (label) {
    info.append(el('div', 'role-label', label));
  }
  info.append(el('div', 'list-title', title));
  if (sub) {
    info.append(listSub(sub));
  }
  row.append(info);
  return row;
}

function badgeCard(imageUrl, label, name, href, color) {
  const card = el('a', 'person-card');
  card.href = href;
  const photo = imageUrl ? el('img', 'person-photo') : el('div', 'person-photo');
  if (imageUrl) {
    photo.src = imageUrl;
    photo.loading = 'lazy';
    photo.alt = '';
  }
  const wrap = el('div', 'photo-wrap photo-wrap-peek');
  if (color) {
    photo.style.setProperty('--ring-color', color);
    wrap.style.setProperty('--peek-color', color);
  }
  wrap.append(photo);
  card.append(wrap);
  card.append(el('div', 'role-label', label));
  card.append(el('div', 'person-name', name));
  return card;
}

const classroomsTabs = [
  {key: 'by-classroom', label: 'Classrooms', heading: 'Classrooms'},
  {key: 'by-grade', label: 'Grades', heading: 'Grades'},
  {key: 'room-parents', label: 'Room Parents', heading: ''},
];

function renderClassroomsList(list) {
  const q = state.q;
  let count = 0;
  for (const group of bandGroups()) {
    const rows = state.model.classrooms
      .filter(c => classroomBand(c.name) === group.band)
      .filter(c => c.name.toLowerCase().includes(q));
    if (!rows.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit classroom-grid');
    for (const c of rows) {
      const students = studentsOf(p => p.classroom === c.name).length;
      grid.append(badgeCard(c.imageUrl, `${students} student${students === 1 ? '' : 's'}`, c.name,
        withFrom('/classrooms/' + slugify(c.name)), colors.classrooms[c.name]));
      count++;
    }
    list.append(grid);
  }
  return count;
}

function renderGradesList(list) {
  const q = state.q;
  let count = 0;
  for (const group of bandGroups()) {
    const rows = group.grades.filter(name => name.toLowerCase().includes(q));
    if (!rows.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit classroom-grid');
    for (const name of rows) {
      const students = studentsOf(p => p.grade === name).length;
      grid.append(badgeCard(gradeImage(name), `${students} student${students === 1 ? '' : 's'}`, name,
        withFrom('/grades/' + slugify(name)), colors.grades[name]));
      count++;
    }
    list.append(grid);
  }
  return count;
}

function abbreviateGrade(name) {
  if (name === 'Kindergarten') {
    return 'K';
  }
  const match = /^Grade (\d+)$/.exec(name || '');
  return match ? match[1] : name;
}

function kidsSummary(parent) {
  const family = familyOf(parent);
  if (!family) {
    return '';
  }
  return peopleOf(family.kids)
    .map(k => `${firstName(k.fullName)} (${[k.classroom, abbreviateGrade(k.grade)].filter(Boolean).join(' - ')})`)
    .join(' • ');
}

function renderRoomParents(list) {
  const q = state.q;
  let count = 0;
  for (const group of bandGroups()) {
    const grades = state.model.grades.filter(g => g.band === group.band);
    const parents = peopleOf([...new Set(grades.flatMap(g => g['room-parents']))])
      .filter(p => p.fullName.toLowerCase().includes(q));
    if (!parents.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit classroom-grid');
    for (const p of parents) {
      const card = el('a', 'person-card');
      card.href = personLink(p);
      card.append(photoWithTag(applyRingColor(photoOrInitials(personPhotoUrl(p), p.fullName, 'person-photo'), p), p.id));
      card.append(el('div', 'person-name', p.fullName));
      card.append(el('div', 'person-sub', kidsSummary(p)));
      grid.append(card);
      count++;
    }
    list.append(grid);
  }
  return count;
}

const classroomsTabRenderers = {
  'by-classroom': renderClassroomsList,
  'by-grade': renderGradesList,
  'room-parents': renderRoomParents,
};

export function classroomsPage() {
  const page = document.createDocumentFragment();

  const pageHeader = el('div', 'page-header container');
  pageHeader.append(el('h1', 'page-title', 'Gradebands'));
  page.append(pageHeader);

  const strip = tabStrip(classroomsTabs, state.classTab, 1, key => {
    state.classTab = key;
    state.q = '';
    history.replaceState(null, '', tabHref(key));
    render();
  });
  strip.classList.add('container');
  page.append(strip);

  const content = el('div', 'content container');
  const header = el('div', 'content-header');
  const heading = classroomsTabs.find(t => t.key === state.classTab).heading;
  header.append(el('h1', '', heading));
  const controls = el('div', 'controls');
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
    if (classroomsTabRenderers[state.classTab](list) === 0) {
      list.append(el('div', 'empty', 'No matches.'));
    }
  }
  renderList();
  return page;
}

function parentsOf(students) {
  const ids = [];
  for (const s of students) {
    for (const family of familiesOf(s)) {
      ids.push(...family.adults);
    }
  }
  return peopleOf([...new Set(ids)]);
}

function teachersOf(classroomNames) {
  const rooms = state.model.classrooms.filter(c => classroomNames.includes(c.name)).map(c => c.id);
  const ids = state.model.crews.filter(crew => rooms.includes(crew.classroom)).flatMap(crew => crew.teachers);
  return peopleOf([...new Set(ids)]);
}

function otherFamilyMembers(student) {
  const ids = [];
  for (const family of familiesOf(student)) {
    ids.push(...family.kids, ...family.adults);
  }
  return peopleOf([...new Set(ids)].filter(id => id !== student.id)).map(p => p.fullName).join(', ');
}

function sectionFilterBar(groups, rerender, colorFor) {
  const sections = groups.filter(g => g.header);
  if (sections.length < 2) {
    return null;
  }
  const keys = sections.map(g => g.chipLabel || g.header);
  for (const excluded of [...state.rosterSectionExcluded]) {
    if (!keys.includes(excluded)) {
      state.rosterSectionExcluded.delete(excluded);
    }
  }
  const bar = el('div', 'chip-row');
  for (const g of sections) {
    const key = g.chipLabel || g.header;
    const active = !state.rosterSectionExcluded.has(key);
    const btn = el('button', 'chip-toggle' + (active ? ' active' : ''));
    btn.type = 'button';
    btn.style.setProperty('--chip-color', (colorFor && colorFor(key)) || paletteColor(key));
    btn.append(el('span', '', g.header));
    btn.addEventListener('click', () => {
      if (active) {
        state.rosterSectionExcluded.add(key);
      } else {
        state.rosterSectionExcluded.delete(key);
      }
      rerender();
    });
    bar.append(btn);
  }
  return bar;
}

function rosterPage(title, image, groups, backLabel, sectionColorFor) {
  const page = document.createDocumentFragment();
  const from = fromURL();
  const back = from && from.pathname === '/classrooms' ? from.pathname + from.search : '/classrooms';

  const header = el('div', 'roster-header');
  header.append(breadcrumbs([['Gradebands', back], [title, null]]));

  const headWrap = el('div', 'container');
  const head = el('div', 'class-head');
  if (image) {
    const img = el('img', 'class-tile');
    img.src = image;
    img.alt = '';
    head.append(img);
  }
  head.append(el('h1', 'class-title', title));
  headWrap.append(head);
  header.append(headWrap);

  const allStudents = groups.flatMap(g => g.students);
  const teachers = teachersOf([...new Set(allStudents.map(s => s.classroom).filter(Boolean))]);
  const parents = parentsOf(allStudents);
  const memberTabs = [
    {key: 'students', label: 'Students', icon: svg('students'), count: allStudents.length},
    {key: 'staff', label: 'Staff', icon: svg('staff'), count: teachers.length},
    {key: 'parents', label: 'Parents', icon: svg('families'), count: parents.length},
  ];

  const strip = tabStrip(memberTabs, state.rosterTab, 2, key => {
    state.rosterTab = key;
    history.replaceState(null, '', tabHref(key));
    render();
  });
  strip.classList.add('container', 'roster-tabs');
  header.append(strip);
  page.append(header);

  const content = el('div', 'container detail-content');
  const list = el('div');
  if (state.rosterTab === 'students') {
    const headingRow = el('div', 'roster-heading-row');
    headingRow.append(el('h2', 'roster-heading', `${allStudents.length} Students`));
    const filterBar = sectionFilterBar(groups, render, sectionColorFor);
    if (filterBar) {
      headingRow.append(filterBar);
    }
    list.append(headingRow);
    const visibleGroups = groups.filter(g => !g.header || !state.rosterSectionExcluded.has(g.chipLabel || g.header));
    const listBody = el('div', 'roster-list grid');
    for (const group of visibleGroups) {
      if (group.header) {
        listBody.append(el('h2', 'group-header', group.header));
      }
      for (const s of sortPeople(group.students)) {
        listBody.append(listRow(thumbUrl(s.photoUrl), otherFamilyMembers(s).toUpperCase(), s.fullName, s.facts || '', personLink(s)));
      }
    }
    list.append(listBody);
  } else if (state.rosterTab === 'staff') {
    list.append(el('h2', 'roster-heading', `${teachers.length} Staff`));
    const listBody = el('div', 'roster-list grid');
    for (const person of sortPeople(teachers)) {
      listBody.append(listRow(thumbUrl(person.photoUrl), (person.jobTitle || '').toUpperCase(), person.fullName, person.facts || '', personLink(person)));
    }
    list.append(listBody);
  } else {
    const headingRow = el('div', 'roster-heading-row');
    headingRow.append(el('h2', 'roster-heading', `${parents.length} Parents`));
    const parentGroups = groups.map(g => ({header: g.header, chipLabel: g.chipLabel, parents: parentsOf(g.students)}));
    const filterBar = sectionFilterBar(parentGroups, render, sectionColorFor);
    if (filterBar) {
      headingRow.append(filterBar);
    }
    list.append(headingRow);
    const visibleGroups = parentGroups.filter(g => !g.header || !state.rosterSectionExcluded.has(g.chipLabel || g.header));
    const listBody = el('div', 'roster-list grid');
    for (const group of visibleGroups) {
      if (group.header) {
        listBody.append(el('h2', 'group-header', group.header));
      }
      for (const p of sortPeople(group.parents)) {
        listBody.append(listRow(thumbUrl(p.photoUrl), otherFamilyMembers(p).toUpperCase(), p.fullName, p.facts || '', personLink(p)));
      }
    }
    list.append(listBody);
  }
  content.append(list);
  page.append(content);
  return page;
}

export function gradePage(slug) {
  const grade = state.model.grades.find(g => slugify(g.name) === slug);
  if (!grade) {
    return el('div', 'empty', 'Not found.');
  }
  const students = studentsOf(p => p.grade === grade.name);
  const classrooms = [...new Set(students.map(s => s.classroom).filter(Boolean))].sort();
  const groups = classrooms.length
    ? classrooms.map(name => ({header: name, students: students.filter(s => s.classroom === name)}))
    : [{header: '', students}];
  return rosterPage(grade.name, gradeImage(grade.name), groups, undefined, name => colors.classrooms[name]);
}

export function classroomPage(slug) {
  const classroom = state.model.classrooms.find(c => slugify(c.name) === slug);
  if (!classroom) {
    return el('div', 'empty', 'Not found.');
  }
  const students = studentsOf(p => p.classroom === classroom.name);
  const crews = [...new Set(students.map(s => s.crew).filter(Boolean))].sort();
  const groups = crews.length
    ? crews.map(name => ({header: `${name} ${classroom.name}`, chipLabel: name, students: students.filter(s => s.crew === name)}))
    : [{header: '', students}];
  return rosterPage(classroom.name, classroom.imageUrl, groups);
}
