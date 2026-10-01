import {state, allowed} from '../state.js';
import {el} from '/elements.js';
import {adminPage as buildAdminPage, adminsCard, appAdmins} from '/admin.js';
import {createPersonPicker} from '/picker.js';
import {popup} from '/modal.js';
import {batch, act, create, remove} from '/data.js';
import {dataGrid} from '/datagrid.js';
import {storeMedia} from '../edit.js';

let data = null;
const painters = [];

async function fetchState() {
  const read = await batch({
    records: '/api/person-records?include=person',
    classrooms: '/api/classrooms',
    grades: '/api/grades',
    crews: '/api/crews?include=classroom',
    departments: '/api/departments',
    settings: '/api/who-settings',
  });
  const all = name => read.result[name].map(read.get);
  const records = all('records');
  const grades = all('grades');
  data = {
    people: records.filter(r => !r.hidden).sort((a, b) => a.fullName < b.fullName ? -1 : a.fullName > b.fullName ? 1 : 0),
    hidden: records.filter(r => r.hidden),
    classrooms: all('classrooms'),
    grades,
    bands: [...new Set(grades.map(g => g.band).filter(Boolean))],
    crews: all('crews').filter(c => c.name).map(c => ({name: c.name, classroom: (read.follow(c, 'classroom') || {}).name || ''})),
    departments: all('departments').map(d => d.name),
    config: read.get(read.result.settings[0]),
  };
}

async function refresh() {
  await fetchState();
  for (const paint of painters) {
    paint();
  }
}

function say(status, text, error) {
  status.textContent = text;
  status.classList.toggle('error', Boolean(error));
}

function actionButton(label, className, onClick) {
  const b = el('button', className, label);
  b.type = 'button';
  b.addEventListener('click', onClick);
  return b;
}

function settingsAct(action, body) {
  return act('who-settings', data.config.id, action, body);
}

async function setColor(kind, name, input, status) {
  say(status, 'Saving…');
  try {
    await settingsAct('color', {kind, name, color: input.value});
    say(status, '');
  } catch (err) {
    say(status, err.message, true);
  }
}

function card(title, hint) {
  const node = el('div', 'card');
  node.append(el('h2', '', title));
  if (hint) {
    node.append(el('div', 'hint', hint));
  }
  return node;
}

function swatch(title, value) {
  const input = el('input', 'color-swatch');
  input.type = 'color';
  input.title = title;
  input.value = value;
  return input;
}

function staffColorCard() {
  const node = card('Staff color', 'Used for staff, and for anyone else (like a parent) with no grade-band color to inherit.');
  const row = el('div', 'image-row');
  const status = el('span', 'status');
  const input = swatch('Staff color', data.config.staffColor);
  input.addEventListener('change', () => setColor('staff', '', input, status));
  row.append(input, el('div', 'name', 'Staff'), status);
  node.append(row);
  return node;
}

async function uploadImage(type, item, input, status) {
  if (!input.files.length) {
    return;
  }
  say(status, 'Uploading…');
  try {
    const name = await storeMedia('photo', input.files[0], input.files[0].name);
    await act(type, item.id, 'image', {name});
  } catch (err) {
    say(status, err.message, true);
    return;
  }
  say(status, '');
  await refresh();
}

function imagesCard(title, hint, kind, items, colors) {
  const node = card(title, hint);
  const list = el('div');
  node.append(list);
  painters.push(() => {
    list.replaceChildren();
    for (const item of data[items]) {
      const row = el('div', 'image-row');
      if (item.imageUrl) {
        const img = el('img');
        img.src = item.imageUrl;
        img.alt = '';
        row.append(img);
      } else {
        row.append(el('div', 'placeholder'));
      }
      const status = el('span', 'status');
      const color = swatch(`Hover color for ${item.name}`, data.config[colors][item.name] || '#8a939b');
      color.addEventListener('change', () => setColor(kind, item.name, color, status));
      const replace = el('label', 'button button-secondary button-small', 'Replace');
      const file = el('input');
      file.type = 'file';
      file.accept = 'image/*';
      file.hidden = true;
      file.addEventListener('change', () => uploadImage(items, item, file, status));
      replace.append(file);
      row.append(el('div', 'name', item.name), status, color, replace);
      list.append(row);
    }
  });
  return node;
}

function imagesPanel() {
  const wrap = el('div');
  wrap.append(
    staffColorCard(),
    imagesCard('Classroom images & colors', 'Shown in the directory wherever a classroom is listed. Replacing one takes effect immediately.', 'classroom', 'classrooms', 'classroomColors'),
    imagesCard('Grade images & colors', 'Shown in Explore by Grade and on each grade’s page.', 'grade', 'grades', 'gradeColors'),
  );
  return wrap;
}

function fieldRow(label, input) {
  const row = el('label', 'field-row');
  row.append(el('span', '', label), input);
  return row;
}

function savingCard(title, hint, fields, action, body) {
  const node = card(title, hint);
  const status = el('span', 'save-status');
  const save = actionButton('Save', 'button', async () => {
    save.disabled = true;
    say(status, 'Saving…');
    try {
      await settingsAct(action, body());
      say(status, 'Saved.');
      await refresh();
    } catch (err) {
      say(status, err.message, true);
    }
    save.disabled = false;
  });
  const actions = el('div', 'add-row');
  actions.append(save, status);
  node.append(...fields, actions);
  return node;
}

function thresholdsCard() {
  const years = {};
  const fields = [['photo', 'Person photo'], ['facts', 'Student facts'], ['familyPhoto', 'Family photo']].map(([key, label]) => {
    const input = el('input');
    input.type = 'number';
    input.step = '0.05';
    input.min = '0.05';
    input.value = data.config.staleYears[key];
    years[key] = input;
    return fieldRow(label, input);
  });
  return savingCard('Update thresholds', 'How many years old a photo or facts entry can get before the directory asks someone to refresh it.', fields, 'stale-years',
    () => Object.fromEntries(Object.entries(years).map(([key, input]) => [key, Number(input.value)])));
}

function privacyCard() {
  const links = {};
  const fields = [['veracrossPreferences', 'Veracross directory preferences'], ['heliosWhoOptIn', 'Helios Who opt-in form']].map(([key, label]) => {
    const input = el('input');
    input.type = 'url';
    input.value = data.config.privacyLinks[key];
    links[key] = input;
    return fieldRow(label, input);
  });
  return savingCard('Privacy links', 'Where My Privacy sends people to fix a Veracross/Helios Who mismatch. Update these if either URL ever changes.', fields, 'privacy-links',
    () => Object.fromEntries(Object.entries(links).map(([key, input]) => [key, input.value.trim()])));
}

function fillSelect(select, values, current) {
  select.replaceChildren(new Option('— none —', ''));
  for (const value of current && !values.includes(current) ? [current, ...values] : values) {
    select.append(new Option(value, value));
  }
  select.value = current || '';
}

function crewNamesFor(classroom) {
  const crews = classroom ? data.crews.filter(c => c.classroom === classroom) : data.crews;
  return [...new Set(crews.map(c => c.name))];
}

function overridesPanel({title, hint, filter, fields}) {
  const wrap = el('div');
  const find = card(title, hint);
  const mount = el('div');
  const picker = createPersonPicker(mount, {people: () => data.people.filter(filter)});
  const bar = el('div', 'add-row');
  const form = el('div', 'card');
  form.hidden = true;
  const heading = el('h2');
  const address = el('div', 'hint');
  const status = el('span', 'save-status');
  const inputs = {};
  const currents = {};
  const sources = {
    classroom: () => data.classrooms.map(c => c.name),
    crew: () => crewNamesFor(inputs.classroom ? inputs.classroom.value : ''),
    department: () => data.departments,
    grade: () => data.grades.map(g => g.name),
    band: () => data.bands,
  };
  form.append(heading, address);
  for (const f of fields) {
    const input = f.kind === 'textarea' ? el('textarea') : sources[f.kind] ? el('select') : el('input');
    if (input.tagName === 'INPUT') {
      input.type = 'text';
    }
    if (f.max) {
      input.maxLength = f.max;
    }
    inputs[f.key] = input;
    currents[f.key] = el('div', 'field-current');
    if (f.kind === 'textarea') {
      const block = el('label', 'field-block');
      block.append(el('span', '', f.label), input, currents[f.key]);
      form.append(block);
      continue;
    }
    const value = el('div', 'field-value');
    value.append(input, currents[f.key]);
    form.append(fieldRow(f.label, value));
  }
  if (inputs.classroom && inputs.crew) {
    inputs.classroom.addEventListener('change', () => fillSelect(inputs.crew, crewNamesFor(inputs.classroom.value), ''));
  }
  let selected = '';
  const show = (id, clear) => {
    const person = data.people.find(p => p.id === id);
    if (!person) {
      return;
    }
    selected = id;
    form.hidden = false;
    heading.textContent = person.fullName;
    address.textContent = person.email;
    for (const f of fields) {
      const override = person.override[f.key] || '';
      if (sources[f.kind]) {
        fillSelect(inputs[f.key], sources[f.kind](), override);
      } else {
        inputs[f.key].value = override;
      }
      currents[f.key].textContent = 'Veracross: ' + (person.veracross[f.key] || '— none —');
    }
    if (clear) {
      say(status, '');
    }
  };
  const save = actionButton('Save', 'button', async () => {
    const body = {};
    for (const f of fields) {
      body[f.key] = inputs[f.key].value;
    }
    save.disabled = true;
    say(status, 'Saving…');
    try {
      await act('person-records', selected, 'edit', body);
      say(status, 'Saved.');
      await refresh();
    } catch (err) {
      say(status, err.message, true);
    }
    save.disabled = false;
  });
  const actions = el('div', 'add-row');
  actions.append(save, status);
  form.append(actions);
  bar.append(mount, actionButton('Load', 'button button-secondary', () => show(picker.person && picker.person.id, true)));
  find.append(bar);
  painters.push(() => {
    if (selected) {
      show(selected, false);
    }
  });
  wrap.append(find, form);
  return wrap;
}

const names = [
  {key: 'fullName', label: 'Full name', max: 100},
  {key: 'legalName', label: 'Legal name', max: 100},
  {key: 'preferredName', label: 'Preferred name', max: 100},
];

const staffOverrides = () => overridesPanel({
  title: 'Find a person',
  hint: 'Edit the fields that decide which classroom or crew a staff member belongs to, plus their name and facts — Classroom, Crew, Department, Job Title, Grade Band, Full Name, Legal Name, Preferred Name, and Facts. Changes here save straight to the Overrides sheet, whether or not the field also has its own self-service editor elsewhere.',
  filter: p => p.isStaff,
  fields: [
    ...names,
    {key: 'classroom', label: 'Classroom', kind: 'classroom'},
    {key: 'crew', label: 'Crew', kind: 'crew'},
    {key: 'department', label: 'Department', kind: 'department'},
    {key: 'jobTitle', label: 'Job title', max: 100},
    {key: 'gradeBand', label: 'Grade band', kind: 'band'},
    {key: 'facts', label: 'Facts', kind: 'textarea', max: 4000},
  ],
});

const studentOverrides = () => overridesPanel({
  title: 'Find a student',
  hint: 'Edit a student’s Overrides-backed fields — Full Name, Legal Name, Preferred Name, Grade, Classroom, and Crew. Changes here save straight to the Overrides sheet, whether or not the field also has its own self-service editor elsewhere.',
  filter: p => p.isStudent,
  fields: [
    ...names,
    {key: 'grade', label: 'Grade', kind: 'grade'},
    {key: 'classroom', label: 'Classroom', kind: 'classroom'},
    {key: 'crew', label: 'Crew', kind: 'crew'},
  ],
});

const parentOverrides = () => overridesPanel({
  title: 'Find a parent',
  hint: 'Edit a parent’s Overrides-backed fields — Full Name, Legal Name, Preferred Name, Phone, Room Parent, and Address. Address is family-level: it’s written to this parent’s own row, and merges with whatever their co-parent’s row supplies.',
  filter: p => p.isParent,
  fields: [
    ...names,
    {key: 'phone', label: 'Phone', max: 40},
    {key: 'roomParent', label: 'Room parent for', kind: 'band'},
    {key: 'address', label: 'Address', max: 200},
  ],
});

function personForm(hint, person, label, submit, drop) {
  const box = el('div');
  const email = el('input');
  email.type = 'text';
  email.maxLength = 200;
  email.value = person.email;
  const fullName = el('input');
  fullName.type = 'text';
  fullName.maxLength = 100;
  fullName.value = person.fullName;
  const roles = el('div', 'field-row roles-row');
  const boxes = {};
  for (const [key, text] of [['isStudent', 'Is Student'], ['isParent', 'Is Parent'], ['isStaff', 'Is Staff']]) {
    const check = el('input');
    check.type = 'checkbox';
    check.checked = Boolean(person[key]);
    boxes[key] = check;
    const role = el('label');
    role.append(check, el('span', '', text));
    roles.append(role);
  }
  const status = el('span', 'save-status');
  const actions = el('div', 'modal-actions');
  const go = actionButton(label, 'button', async () => {
    go.disabled = true;
    say(status, 'Saving…');
    try {
      await submit({email: email.value.trim(), fullName: fullName.value.trim(), isStudent: boxes.isStudent.checked, isParent: boxes.isParent.checked, isStaff: boxes.isStaff.checked});
    } catch (err) {
      say(status, err.message, true);
    }
    go.disabled = false;
  });
  actions.append(go);
  if (drop) {
    actions.append(actionButton('Delete', 'danger-button', drop));
  }
  actions.append(status);
  box.append(el('div', 'hint', hint), fieldRow('Email', email), fieldRow('Full name', fullName), roles, actions);
  return box;
}

function openAddPerson() {
  let shut = null;
  const form = personForm('Create someone Veracross genuinely doesn’t have a record for yet - Overrides becomes the only source of their name and role. Pick at least one role; you can add the rest of their details (classroom, phone, and so on) afterward from the table or the other Overrides tabs.',
    {email: '', fullName: ''}, 'Add', async person => {
      await create('person-records', person);
      shut();
      await refresh();
    });
  shut = popup('Add a new person', form).shut;
}

function openEditPerson(p) {
  let shut = null;
  const form = personForm('Changing the email renames this person everywhere they’re keyed by it - their Overrides row, and any Tags or Photos rows they already have.',
    p, 'Save', async person => {
      await act('person-records', p.id, 'edit', person);
      shut();
      await refresh();
    }, async () => {
      if (!confirm(`Permanently delete ${p.fullName} (${p.email})? This also removes any tags or photos they have.`)) {
        return;
      }
      try {
        await remove('person-records', p.id);
      } catch (err) {
        alert(err.message);
        return;
      }
      shut();
      await refresh();
    });
  shut = popup('Edit person', form).shut;
}

function addedPanel() {
  const node = el('div', 'card');
  const head = el('div', 'card-header-row');
  head.append(el('h2', '', 'People not in Veracross'), actionButton('+ Add Person', 'button button-secondary button-small', openAddPerson));
  const holder = el('div');
  node.append(head, el('div', 'hint', 'Click Edit on a row to change it.'), holder);
  const columns = [
    {label: 'Full name', get: p => p.fullName},
    {label: 'Email', get: p => p.email},
    {label: 'Student', get: p => p.isStudent ? '✓' : ''},
    {label: 'Parent', get: p => p.isParent ? '✓' : ''},
    {label: 'Staff', get: p => p.isStaff ? '✓' : ''},
  ];
  painters.push(() => {
    const added = data.people.filter(p => p.added);
    if (!added.length) {
      holder.replaceChildren(el('div', 'empty', 'Nobody yet.'));
      return;
    }
    holder.replaceChildren(dataGrid({columns, rows: added, trailing: p => actionButton('Edit', 'button button-secondary button-small', () => openEditPerson(p))}).wrap);
  });
  return node;
}

function hiddenPanel() {
  const wrap = el('div');
  const hide = card('Hide a person', 'Hides an existing person from the whole directory - they stop appearing anywhere, including search, until unhidden.');
  const mount = el('div');
  const picker = createPersonPicker(mount, {people: () => data.people});
  const status = el('span', 'save-status');
  const go = actionButton('Hide', 'button', async () => {
    const person = picker.person;
    if (!person) {
      return;
    }
    go.disabled = true;
    say(status, 'Hiding…');
    try {
      await act('person-records', person.id, 'hide');
      picker.reset();
      say(status, '');
      await refresh();
    } catch (err) {
      say(status, err.message, true);
    }
    go.disabled = false;
  });
  const bar = el('div', 'add-row');
  bar.append(mount, go, status);
  hide.append(bar);
  const hidden = card('Currently hidden');
  const holder = el('div');
  hidden.append(holder);
  const unhideButton = record => {
    const unhide = actionButton('Unhide', 'button button-secondary button-small', async () => {
      unhide.disabled = true;
      try {
        await act('person-records', record.id, 'unhide');
      } catch (err) {
        alert(err.message);
        unhide.disabled = false;
        return;
      }
      await refresh();
    });
    return unhide;
  };
  painters.push(() => {
    if (!data.hidden.length) {
      holder.replaceChildren(el('div', 'empty', 'Nobody hidden.'));
      return;
    }
    holder.replaceChildren(dataGrid({columns: [{label: 'Email', get: record => record.email}], rows: data.hidden, trailing: unhideButton}).wrap);
  });
  wrap.append(hide, hidden);
  return wrap;
}

function sections() {
  const control = [];
  if (allowed('super-admins')) {
    control.push({key: 'super-admins', label: 'Super Admins', card: () => adminsCard({
      title: 'Super Admins',
      hint: 'Super admins can also use Spoof Mode, from the eye beside their avatar in any app’s toolbar, and manage this list. Regular admins never see this tab. Changes save immediately.',
      ...appAdmins('super'),
    })});
  }
  control.push({key: 'admins', label: 'Admins', card: () => adminsCard({hint: 'Whoever is on this list can reach this page. Changes save immediately.', ...appAdmins('who')})});
  return [
    {title: 'Display', tabs: [
      {key: 'images', label: 'Images', card: imagesPanel},
      {key: 'thresholds', label: 'Update Thresholds', card: thresholdsCard},
      {key: 'privacy-links', label: 'Privacy Links', card: privacyCard},
    ]},
    {title: 'Editing & Control', tabs: control},
    {title: 'Data Overrides', tabs: [
      {key: 'staff-assignments', label: 'Staff Overrides', card: staffOverrides},
      {key: 'student-overrides', label: 'Student Overrides', card: studentOverrides},
      {key: 'parent-overrides', label: 'Parent Overrides', card: parentOverrides},
      {key: 'added-overrides', label: 'Added Overrides', card: addedPanel},
      {key: 'hidden-overrides', label: 'Hidden Overrides', card: hiddenPanel},
    ]},
  ];
}

async function fillAdminPage(slot) {
  try {
    await fetchState();
  } catch (err) {
    slot.replaceWith(el('p', 'hint', `Failed to load admin state: ${err.message}`));
    return;
  }
  painters.length = 0;
  slot.replaceWith(buildAdminPage({appName: 'Helios Who?', allowed: true, email: state.model.user.email, sections: sections()}));
  for (const paint of painters) {
    paint();
  }
}

export function adminPage() {
  const user = state.model.user;
  if (!allowed('who.administer')) {
    return buildAdminPage({appName: 'Helios Who?', allowed: false, email: user.email, sections: []});
  }
  const slot = el('div');
  fillAdminPage(slot);
  return slot;
}
