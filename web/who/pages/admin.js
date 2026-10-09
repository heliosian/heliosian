import {model, q, rowsOf, write} from '../state.js';
import {el} from '/elements.js';
import {adminPage as buildAdminPage, adminsCard, appAdmins} from '/admin.js';
import {api} from '/api.js';
import {dataGrid} from '/datagrid.js';
import {photoOrInitials} from '../people.js';

let data = null;
const painters = [];

const gradeCodes = ['K', '1', '2', '3', '4', '5', '6', '7', '8'];

function byId(rows) {
  const out = {};
  for (const r of rows) {
    out[r.id] = r;
  }
  return out;
}

function all(answer, table) {
  return Object.values(answer.resources[table] || {});
}

async function fetchState() {
  const [people, emails, roles, groups, roomRows, photos, settings] = await Promise.all([
    q('(from PERSON (where (!= source "guest") (in id (select EFFECTIVE_MEMBER.person (in group (select GROUP.id (= slug "everyone")))))) (order name_sort asc))'),
    q('(from PERSON_EMAIL (where primary))'),
    q('(from EFFECTIVE_MEMBER (where (in group (select GROUP.id (in slug "students" "parents" "staff")))) (include group))'),
    q('(from GROUP (where (or (in kind "classroom" "grade" "band" "crew" "department") (and (= kind "group") (= parent.kind "band")))) (order order asc name asc))'),
    q('(from MEMBER (where (in group (select GROUP.id (= kind "group") (= parent.kind "band")))))'),
    q('(from PHOTO (where ready (or (= group.kind "classroom") (= group.kind "grade"))) (order order asc))'),
    q('(from SETTING (where (= app "platform")))'),
  ]);
  const emailOf = {};
  for (const e of all(emails, 'PERSON_EMAIL')) {
    emailOf[e.person] = e;
  }
  const slugs = {};
  for (const g of all(roles, 'GROUP')) {
    slugs[g.id] = g.slug;
  }
  const inRole = {students: new Set(), parents: new Set(), staff: new Set()};
  for (const e of all(roles, 'EFFECTIVE_MEMBER')) {
    inRole[slugs[e.group]].add(e.person);
  }
  const groupRows = rowsOf(groups, 'GROUP');
  const ofKind = kind => groupRows.filter(g => g.kind === kind);
  const tiles = {};
  for (const ph of rowsOf(photos, 'PHOTO')) {
    tiles[ph.group] = tiles[ph.group] || ph;
  }
  data = {
    people: rowsOf(people, 'PERSON').map(p => ({
      ...p,
      email: emailOf[p.id] ? emailOf[p.id].address : '',
      isStudent: inRole.students.has(p.id),
      isParent: inRole.parents.has(p.id),
      isStaff: inRole.staff.has(p.id),
    })),
    groups: byId(groupRows),
    classrooms: ofKind('classroom'),
    grades: ofKind('grade'),
    bands: ofKind('band'),
    crews: ofKind('crew'),
    departments: ofKind('department'),
    roomParents: groupRows.filter(g => g.kind === 'group' && g.parent),
    roomRows: all(roomRows, 'MEMBER'),
    tiles,
    settings: Object.fromEntries(all(settings, 'SETTING').map(s => [s.key, s])),
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

async function saving(status, work) {
  say(status, 'Saving…');
  try {
    await work();
    say(status, 'Saved.');
  } catch (err) {
    say(status, err.message, true);
    return false;
  }
  await refresh();
  return true;
}

function setting(key) {
  return data.settings[key];
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
  const input = swatch('Staff color', setting('Staff Color').value);
  input.addEventListener('change', () => saving(status, () => write([{set: setting('Staff Color').id, cells: {value: input.value}}])));
  row.append(input, el('div', 'name', 'Staff'), status);
  node.append(row);
  return node;
}

async function uploadTile(group, input, status) {
  if (!input.files.length) {
    return;
  }
  const form = new FormData();
  form.append('group', group.id);
  form.append('photo', input.files[0], input.files[0].name);
  say(status, 'Uploading…');
  await saving(status, () => api('POST', '/api/do/photo', form));
}

function imagesCard(title, hint, items) {
  const node = card(title, hint);
  const list = el('div');
  node.append(list);
  painters.push(() => {
    list.replaceChildren();
    for (const item of data[items]) {
      const row = el('div', 'image-row');
      const tile = data.tiles[item.id];
      if (tile) {
        const img = el('img');
        img.src = `/api/blob/${tile.id}/thumbnail`;
        img.alt = '';
        row.append(img);
      } else {
        row.append(el('div', 'placeholder'));
      }
      const status = el('span', 'status');
      const color = swatch(`Hover color for ${item.name}`, item.color || '#8a939b');
      color.addEventListener('change', () => saving(status, () => write([{set: item.id, cells: {color: color.value}}])));
      const replace = el('label', 'button button-secondary button-small', 'Replace');
      const file = el('input');
      file.type = 'file';
      file.accept = 'image/*';
      file.hidden = true;
      file.addEventListener('change', () => uploadTile(item, file, status));
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
    imagesCard('Classroom images & colors', 'Shown in the directory wherever a classroom is listed. A new image goes first and is the one shown.', 'classrooms'),
    imagesCard('Grade images & colors', 'Shown in Explore by Grade and on each grade’s page. A new image goes first and is the one shown.', 'grades'),
  );
  return wrap;
}

function fieldRow(label, input) {
  const row = el('label', 'field-row');
  row.append(el('span', '', label), input);
  return row;
}

function settingsCard(title, hint, fields) {
  const node = card(title, hint);
  const inputs = fields.map(([key, label, type]) => {
    const input = el('input');
    input.type = type;
    if (type === 'number') {
      input.step = '0.05';
      input.min = '0.05';
    }
    input.value = setting(key).value;
    return {key, input, row: fieldRow(label, input)};
  });
  const status = el('span', 'save-status');
  const save = actionButton('Save', 'button', async () => {
    save.disabled = true;
    await saving(status, () => write(inputs.map(({key, input}) => ({set: setting(key).id, cells: {value: input.value.trim()}}))));
    save.disabled = false;
  });
  const actions = el('div', 'add-row');
  actions.append(save, status);
  node.append(...inputs.map(i => i.row), actions);
  return node;
}

function thresholdsCard() {
  return settingsCard('Update thresholds', 'How many years old a photo or facts entry can get before the directory asks someone to refresh it.', [
    ['Photo Stale Years', 'Person photo', 'number'],
    ['Facts Stale Years', 'Student facts', 'number'],
    ['Family Photo Stale Years', 'Family photo', 'number'],
  ]);
}

function privacyCard() {
  return settingsCard('Privacy links', 'Where My Privacy sends people to fix a Veracross/Helios Who mismatch. Update these if either URL ever changes.', [
    ['Veracross Preferences URL', 'Veracross directory preferences', 'url'],
    ['Helios Who Opt-In URL', 'Helios Who opt-in form', 'url'],
  ]);
}

function personSearch(mount, people, onPick) {
  const box = el('div', 'share-search');
  const input = el('input');
  input.placeholder = 'Search by name or email…';
  const results = el('div', 'share-results');
  box.append(input, results);
  mount.append(box);
  const paint = () => {
    const text = input.value.trim().toLowerCase();
    results.replaceChildren();
    if (!text) {
      return;
    }
    for (const p of people().filter(x => x.name_show.toLowerCase().includes(text) || x.email.includes(text)).slice(0, 8)) {
      const row = el('button', 'share-result');
      row.type = 'button';
      row.append(photoOrInitials('', p.name_show, 'share-avatar'), el('span', '', p.name_show), el('span', 'hint', p.email));
      row.addEventListener('click', () => {
        input.value = p.name_show;
        results.replaceChildren();
        onPick(p);
      });
      results.append(row);
    }
  };
  input.addEventListener('input', paint);
  return {reset: () => {
    input.value = '';
    results.replaceChildren();
  }};
}

function groupSelect(groups, current) {
  const select = el('select');
  select.append(new Option('— none —', ''));
  for (const g of groups) {
    select.append(new Option(g.name, g.id));
  }
  select.value = current || '';
  return select;
}

function textInput(value, max) {
  const input = el('input');
  input.type = 'text';
  input.maxLength = max;
  input.value = value || '';
  return input;
}

function nameName(id) {
  return id && data.groups[id] ? data.groups[id].name : '';
}

const overrideFields = {
  name_long_override: p => ({label: 'Full name', input: textInput(p.name_long_override, 100), current: p.vc_name_long}),
  name_short_override: p => ({label: 'Preferred name', input: textInput(p.name_short_override, 100), current: p.vc_name_short}),
  name_sort_override: p => ({label: 'Sort name', input: textInput(p.name_sort_override, 100), current: p.vc_name_sort}),
  grade_override: p => {
    const select = el('select');
    select.append(new Option('— none —', ''));
    for (const code of gradeCodes) {
      select.append(new Option(code === 'K' ? 'Kindergarten' : `Grade ${code}`, code));
    }
    select.value = p.grade_override || '';
    return {label: 'Grade', input: select, current: p.vc_grade};
  },
  classroom_override: p => ({label: 'Classroom', input: groupSelect(data.classrooms, p.classroom_override), current: nameName(p.vc_classroom)}),
  crew_override: p => ({label: 'Crew', input: groupSelect(data.crews.map(c => ({id: c.id, name: `${c.name} (${nameName(c.parent)})`})), p.crew_override), current: nameName(p.vc_crew)}),
  department_override: p => ({label: 'Department', input: groupSelect(data.departments, p.department_override), current: nameName(p.vc_department)}),
  job_title_override: p => ({label: 'Job title', input: textInput(p.job_title_override, 100), current: p.vc_job_title}),
  phone_override: p => ({label: 'Phone override', input: textInput('', 40), current: p.phone ? `shown: ${p.phone}` : 'none shown', blankKeeps: true}),
  facts: p => ({label: 'Facts', input: Object.assign(el('textarea'), {value: p.facts || '', maxLength: 4000}), current: ''}),
};

function roomParentOf(p) {
  const row = data.roomRows.find(m => m.person === p.id);
  return row ? data.groups[row.group] : null;
}

function overridesPanel({title, hint, filter, fields, roomParent}) {
  const wrap = el('div');
  const find = card(title, hint);
  const mount = el('div');
  const form = el('div', 'card');
  form.hidden = true;
  const heading = el('h2');
  const address = el('div', 'hint');
  const body = el('div');
  const status = el('span', 'save-status');
  let selected = null;
  let inputs = {};
  let room = null;
  const show = (p, clear) => {
    selected = p;
    form.hidden = false;
    heading.textContent = p.name_show;
    address.textContent = p.email;
    body.replaceChildren();
    inputs = {};
    for (const key of fields) {
      const f = overrideFields[key](p);
      inputs[key] = f;
      const value = el('div', 'field-value');
      value.append(f.input, el('div', 'field-current', f.current ? 'Veracross: ' + f.current : ''));
      body.append(fieldRow(f.label, value));
    }
    if (roomParent) {
      const current = roomParentOf(p);
      room = groupSelect(data.roomParents.map(g => ({id: g.id, name: nameName(g.parent)})), current ? current.id : '');
      body.append(fieldRow('Room parent for', room));
    }
    if (clear) {
      say(status, '');
    }
  };
  const save = actionButton('Save', 'button', async () => {
    const p = selected;
    const cells = {};
    for (const [key, f] of Object.entries(inputs)) {
      const value = f.input.value.trim();
      if (f.blankKeeps && !value) {
        continue;
      }
      if (value !== (p[key] || '')) {
        cells[key] = value;
      }
    }
    const batch = Object.keys(cells).length ? [{set: p.id, cells}] : [];
    if (room) {
      const current = data.roomRows.find(m => m.person === p.id);
      if (current && current.group !== room.value) {
        batch.push({delete: current.id});
      }
      if (room.value && (!current || current.group !== room.value)) {
        batch.push({insert: 'MEMBER', row: {group: room.value, person: p.id, member: 'yes'}});
      }
    }
    if (!batch.length) {
      say(status, 'Nothing changed.');
      return;
    }
    save.disabled = true;
    await saving(status, () => write(batch));
    save.disabled = false;
  });
  const actions = el('div', 'add-row');
  actions.append(save, status);
  form.append(heading, address, body, actions);
  personSearch(mount, () => data.people.filter(filter), p => show(p, true));
  find.append(mount);
  painters.push(() => {
    if (selected) {
      const fresh = data.people.find(p => p.id === selected.id);
      if (fresh) {
        show(fresh, false);
      }
    }
  });
  wrap.append(find, form);
  return wrap;
}

const nameFields = ['name_long_override', 'name_short_override', 'name_sort_override'];

const staffOverrides = () => overridesPanel({
  title: 'Find a person',
  hint: 'Override a staff member’s names, classroom, crew, department, job title and facts. A blank override falls back to what Veracross says.',
  filter: p => p.isStaff,
  fields: [...nameFields, 'classroom_override', 'crew_override', 'department_override', 'job_title_override', 'facts'],
});

const studentOverrides = () => overridesPanel({
  title: 'Find a student',
  hint: 'Override a student’s names, grade, classroom and crew. A blank override falls back to what Veracross says.',
  filter: p => p.isStudent,
  fields: [...nameFields, 'grade_override', 'classroom_override', 'crew_override'],
});

const parentOverrides = () => overridesPanel({
  title: 'Find a parent',
  hint: 'Override a parent’s names and phone, and make them a band’s room parent. The phone override is never shown back here: leave it blank to keep it, and the field shows what the directory shows.',
  filter: p => p.isParent,
  fields: [...nameFields, 'phone_override'],
  roomParent: true,
});

function hiddenPanel() {
  const wrap = el('div');
  const hide = card('Hide a person', 'Hides an existing person from the whole directory and from signing in to any app, until unhidden.');
  const mount = el('div');
  const status = el('span', 'save-status');
  let chosen = null;
  const search = personSearch(mount, () => data.people.filter(p => p.hidden !== 'Yes'), p => {
    chosen = p;
  });
  const go = actionButton('Hide', 'button', async () => {
    if (!chosen) {
      return;
    }
    go.disabled = true;
    if (await saving(status, () => write([{set: chosen.id, cells: {hidden: true}}]))) {
      chosen = null;
      search.reset();
    }
    go.disabled = false;
  });
  const bar = el('div', 'add-row');
  bar.append(mount, go, status);
  hide.append(bar);
  const hidden = card('Currently hidden');
  const holder = el('div');
  hidden.append(holder);
  const unhideButton = p => {
    const unhide = actionButton('Unhide', 'button button-secondary button-small', async () => {
      unhide.disabled = true;
      try {
        await write([{set: p.id, cells: {hidden: false}}]);
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
    const hiddenPeople = data.people.filter(p => p.hidden === 'Yes');
    if (!hiddenPeople.length) {
      holder.replaceChildren(el('div', 'empty', 'Nobody hidden.'));
      return;
    }
    holder.replaceChildren(dataGrid({columns: [{label: 'Name', get: p => p.name_show}, {label: 'Email', get: p => p.email}], rows: hiddenPeople, trailing: unhideButton}).wrap);
  });
  wrap.append(hide, hidden);
  return wrap;
}

function sections() {
  const control = [];
  if (model.allowances.includes('super-admins')) {
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
      {key: 'hidden-overrides', label: 'Hidden People', card: hiddenPanel},
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
  slot.replaceWith(buildAdminPage({appName: 'Helios Who?', allowed: true, email: model.email, sections: sections()}));
  for (const paint of painters) {
    paint();
  }
}

export function adminPage() {
  if (!model.admin) {
    return buildAdminPage({appName: 'Helios Who?', allowed: false, email: model.email, sections: []});
  }
  const slot = el('div');
  fillAdminPage(slot);
  return slot;
}
