import {state, model, isStudent, isStaff, familiesOf, familyOf, kidsOf, adultsOf, membersOf, photosOf, photoUrl, thumbOf, gradeOf, classroomOf, groupById, roomParentsOf, gradeByKey, classroomByKey, gradePath, classroomPath} from '../state.js';
import {withFrom, ordinal, firstName, listSub, paletteColor} from '../dom.js';
import {el, svg} from '/elements.js';
import {tabStrip, tabHref} from '/tabs.js';
import {personLink, photoWithTag, applyRingColor, photoOrInitials, personPhotoUrl, sortPeople} from '../people.js';
import {fromURL, breadcrumbs} from '../crumbs.js';
import {render, notFound} from '/router.js';

function gradesOf(band) {
  return model.grades.filter(g => g.parent === band.id);
}

function bandGroups() {
  const groups = model.bands
    .map(band => ({band, grades: gradesOf(band)}))
    .filter(group => group.grades.length)
    .sort((a, b) => model.grades.indexOf(a.grades[0]) - model.grades.indexOf(b.grades[0]));
  for (const group of groups) {
    group.label = group.grades.map(g => g.slug === 'grade-k' ? 'K' : ordinal(g.name)).join(' / ');
  }
  return groups;
}

export function gradeImage(g) {
  const photo = photosOf(g.id)[0];
  if (photo) {
    return photoUrl(photo);
  }
  return '/brand/classrooms/' + g.slug + '.jpg';
}

export function classroomImage(c) {
  const photo = photosOf(c.id)[0];
  if (photo) {
    return photoUrl(photo);
  }
  return '/brand/classrooms/classroom-' + c.name.toLowerCase() + '.jpg';
}

function studentsOf(filter) {
  return model.people.filter(p => isStudent(p) && filter(p));
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

function counted(n) {
  return `${n} student${n === 1 ? '' : 's'}`;
}

function renderClassroomsList(list) {
  const q = state.q;
  let count = 0;
  for (const group of bandGroups()) {
    const rows = model.classrooms
      .filter(c => c.parent === group.band.id)
      .filter(c => c.name.toLowerCase().includes(q));
    if (!rows.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit classroom-grid');
    for (const c of rows) {
      const students = studentsOf(p => p.classroom === c.id).length;
      grid.append(badgeCard(thumbOf(photosOf(c.id)[0]) || classroomImage(c), counted(students), c.name, withFrom(classroomPath(c)), c.color));
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
    const rows = group.grades.filter(g => g.name.toLowerCase().includes(q));
    if (!rows.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit classroom-grid');
    for (const g of rows) {
      const students = studentsOf(p => gradeOf(p) === g).length;
      grid.append(badgeCard(gradeImage(g), counted(students), g.name, withFrom(gradePath(g)), g.color));
      count++;
    }
    list.append(grid);
  }
  return count;
}

function abbreviateGrade(g) {
  if (!g) {
    return '';
  }
  return g.slug === 'grade-k' ? 'K' : g.slug.replace('grade-', '');
}

function kidsSummary(parent) {
  const family = familyOf(parent);
  if (!family) {
    return '';
  }
  return kidsOf(family)
    .map(k => `${firstName(k.name_show)} (${[(classroomOf(k) || {}).name, abbreviateGrade(gradeOf(k))].filter(Boolean).join(' - ')})`)
    .join(' • ');
}

function renderRoomParents(list) {
  const q = state.q;
  let count = 0;
  for (const group of bandGroups()) {
    const parents = roomParentsOf(group.band).filter(p => p.name_show.toLowerCase().includes(q));
    if (!parents.length) {
      continue;
    }
    list.append(el('h2', 'staff-section', group.label));
    const grid = el('div', 'people-grid autofit classroom-grid');
    for (const p of parents) {
      const card = el('a', 'person-card');
      card.href = personLink(p);
      card.append(photoWithTag(applyRingColor(photoOrInitials(personPhotoUrl(p), p.name_show, 'person-photo'), p), p.id));
      card.append(el('div', 'person-name', p.name_show));
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
  const out = new Map();
  for (const s of students) {
    for (const family of familiesOf(s)) {
      for (const a of adultsOf(family)) {
        out.set(a.id, a);
      }
    }
  }
  return [...out.values()];
}

function teachersOf(classrooms) {
  const rooms = new Set(classrooms.map(c => c.id));
  return model.people.filter(p => isStaff(p) && rooms.has(p.classroom));
}

function otherFamilyMembers(person) {
  const out = new Map();
  for (const family of familiesOf(person)) {
    for (const m of membersOf(family)) {
      if (m.id !== person.id) {
        out.set(m.id, m);
      }
    }
  }
  return [...out.values()].map(p => p.name_show).join(', ');
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

function rosterPage(title, image, groups, sectionColorFor) {
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
  const teachers = teachersOf([...new Map(allStudents.map(classroomOf).filter(Boolean).map(c => [c.id, c])).values()]);
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
  const row = p => listRow(thumbOf(photosOf(p.id)[0]), otherFamilyMembers(p).toUpperCase(), p.name_show, p.facts || '', personLink(p));
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
        listBody.append(row(s));
      }
    }
    list.append(listBody);
  } else if (state.rosterTab === 'staff') {
    list.append(el('h2', 'roster-heading', `${teachers.length} Staff`));
    const listBody = el('div', 'roster-list grid');
    for (const person of sortPeople(teachers)) {
      listBody.append(listRow(thumbOf(photosOf(person.id)[0]), (person.job_title || '').toUpperCase(), person.name_show, person.facts || '', personLink(person)));
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
        listBody.append(row(p));
      }
    }
    list.append(listBody);
  }
  content.append(list);
  page.append(content);
  return page;
}

export function gradePage(key) {
  const grade = gradeByKey(key);
  if (!grade) {
    return notFound('That grade');
  }
  const students = studentsOf(p => gradeOf(p) === grade);
  const classrooms = [...new Map(students.map(classroomOf).filter(Boolean).map(c => [c.id, c])).values()].sort((a, b) => a.name.localeCompare(b.name));
  const groups = classrooms.length
    ? classrooms.map(c => ({header: c.name, students: students.filter(s => s.classroom === c.id)}))
    : [{header: '', students}];
  return rosterPage(grade.name, gradeImage(grade), groups, name => (model.classrooms.find(c => c.name === name) || {}).color);
}

export function classroomPage(key) {
  const classroom = classroomByKey(key);
  if (!classroom) {
    return notFound('That classroom');
  }
  const students = studentsOf(p => p.classroom === classroom.id);
  const crews = [...new Map(students.map(s => groupById[s.crew]).filter(Boolean).map(c => [c.id, c])).values()].sort((a, b) => a.name.localeCompare(b.name));
  const groups = crews.length
    ? crews.map(crew => ({header: `${crew.name} ${classroom.name}`, chipLabel: crew.name, students: students.filter(s => s.crew === crew.id)}))
    : [{header: '', students}];
  return rosterPage(classroom.name, classroomImage(classroom), groups);
}
