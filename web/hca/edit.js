import {state, me, isAdmin, years, allYears, allRoles, activityPath, rolePath} from './state.js';
import {el, toast} from './dom.js';

let modalState = null;

const overlay = () => document.querySelector('#modal-overlay');
const form = () => document.querySelector('#modal');

export async function reload() {
  const {load} = await import('./app.js');
  await load();
}

export async function send(method, url, body) {
  const res = await fetch(url, {method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  if (!res.ok) {
    throw new Error(await res.text());
  }
}

async function uploadImage(file) {
  const body = new FormData();
  body.append('image', file);
  const res = await fetch('/api/events/image', {method: 'POST', body});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return (await res.json()).name;
}

function closeModal() {
  overlay().hidden = true;
  form().replaceChildren();
  modalState = null;
}

export function initModal() {
  overlay().addEventListener('click', e => {
    if (e.target === overlay()) {
      closeModal();
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeModal();
    }
  });
  form().addEventListener('submit', async e => {
    e.preventDefault();
    if (!modalState) {
      return;
    }
    setStatus('Saving…');
    try {
      await modalState.submit();
      closeModal();
      await reload();
    } catch (err) {
      setStatus(err.message, true);
    }
  });
}

function setStatus(message, error) {
  const status = form().querySelector('.save-status');
  if (!status) {
    return;
  }
  status.textContent = message;
  status.classList.toggle('error', Boolean(error));
}

function field(label, input, hint) {
  const wrap = el('label', 'field');
  wrap.append(el('span', '', label), input);
  if (hint) {
    wrap.append(el('small', '', hint));
  }
  return wrap;
}

function text(value, options) {
  const input = el('input');
  input.type = (options && options.type) || 'text';
  input.value = value || '';
  if (options && options.placeholder) {
    input.placeholder = options.placeholder;
  }
  if (options && options.required) {
    input.required = true;
  }
  if (options && options.maxLength) {
    input.maxLength = options.maxLength;
  }
  return input;
}

function textarea(value, rows) {
  const input = el('textarea');
  input.rows = rows || 4;
  input.value = value || '';
  return input;
}

function select(options, value) {
  const input = el('select');
  for (const option of options) {
    const node = el('option', '', option.label === undefined ? option : option.label);
    node.value = option.value === undefined ? option : option.value;
    node.selected = node.value === value;
    input.append(node);
  }
  return input;
}

function checkbox(label, checked) {
  const wrap = el('label', 'field field-toggle');
  const input = el('input');
  input.type = 'checkbox';
  input.checked = Boolean(checked);
  wrap.append(el('span', '', label), input);
  return {wrap, input};
}

// imagePicker uploads on selection, so the save that follows only records the
// name the server handed back.
function imagePicker(current, currentUrl) {
  const wrap = el('div', 'field');
  wrap.append(el('span', '', 'Image'));
  const row = el('div', 'image-row');
  const preview = el('img');
  preview.alt = '';
  const placeholder = el('div', 'image-placeholder', 'No image');
  const choose = el('label', 'button button-secondary button-small', 'Choose');
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  choose.append(file);
  const remove = el('button', 'link-button', 'Remove');
  remove.type = 'button';
  let name = current || '';
  const show = url => {
    preview.hidden = !url;
    placeholder.hidden = Boolean(url);
    remove.hidden = !url;
    if (url) {
      preview.src = url;
    }
  };
  show(currentUrl);
  file.addEventListener('change', async () => {
    if (!file.files.length) {
      return;
    }
    setStatus('Uploading image…');
    try {
      name = await uploadImage(file.files[0]);
      show('/' + name);
      setStatus('');
    } catch (err) {
      setStatus(err.message, true);
    }
    file.value = '';
  });
  remove.addEventListener('click', () => {
    name = '';
    show('');
  });
  row.append(preview, placeholder, choose, remove);
  wrap.append(row);
  return {wrap, value: () => name};
}

function openModal(title, fields, options) {
  const f = form();
  f.replaceChildren();
  const header = el('div', 'modal-header');
  header.append(el('h2', '', title));
  const close = el('button', 'modal-close', '×');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  close.addEventListener('click', closeModal);
  header.append(close);
  f.append(header);
  for (const node of fields) {
    f.append(node);
  }
  const actions = el('div', 'modal-actions');
  const save = el('button', 'button', options.saveLabel || 'Save');
  save.type = 'submit';
  const cancel = el('button', 'button button-secondary', 'Cancel');
  cancel.type = 'button';
  cancel.addEventListener('click', closeModal);
  actions.append(save, cancel);
  if (options.onDelete) {
    const del = el('button', 'danger-button', options.deleteLabel || 'Delete');
    del.type = 'button';
    del.addEventListener('click', async () => {
      if (!confirm(options.confirmDelete)) {
        return;
      }
      setStatus('Deleting…');
      try {
        await options.onDelete();
        closeModal();
        await reload();
        if (options.afterDelete) {
          options.afterDelete();
        }
      } catch (err) {
        setStatus(err.message, true);
      }
    });
    actions.append(del);
  }
  actions.append(el('span', 'save-status'));
  f.append(actions);
  modalState = {submit: options.submit};
  overlay().hidden = false;
  const first = f.querySelector('input:not([type=hidden]):not([type=file]), textarea, select');
  if (first) {
    first.focus();
  }
}

async function goTo(path) {
  const {navigate} = await import('./app.js');
  navigate(path);
}

// openSignUp signs the viewer, or someone they name, up for an activity or one
// of its roles; with an existing sign-up it edits that one.
export function openSignUp(act, role, existing) {
  const node = role || act;
  const editor = act.canEdit;
  const who = select([{label: 'Me', value: ''}, {label: 'Someone else', value: 'other'}], existing && existing.email !== me().email ? 'other' : '');
  const email = text(existing && existing.email !== me().email ? existing.email : '', {type: 'email', placeholder: 'name@heliosschool.org'});
  const emailField = field('Their email', email);
  emailField.hidden = who.value !== 'other';
  who.addEventListener('change', () => {
    emailField.hidden = who.value !== 'other';
  });
  const positions = [{label: 'Volunteer', value: 'Volunteer'}, {label: 'Volunteer, and open to co-chairing', value: 'Open to Co-Chair'}];
  if (editor) {
    positions.push({label: 'Co-Chair', value: 'Co-Chair'});
  }
  const position = select(positions, existing ? existing.position : 'Volunteer');
  const note = textarea(existing ? existing.note : '', 3);
  const fields = [];
  if (!existing) {
    fields.push(field('Who', who), emailField);
  }
  fields.push(field('As', position), field('Note', note, 'Anything the organizers should know'));
  openModal(existing ? `Edit sign-up for ${node.title}` : `Sign up for ${node.title}`, fields, {
    saveLabel: existing ? 'Save' : 'Sign Up',
    submit: () => send('POST', '/api/events/volunteer', {
      year: act.year, activity: act.title, role: role ? role.title : '',
      email: existing ? existing.email : (who.value === 'other' ? email.value : ''),
      position: position.value, note: note.value,
    }),
    onDelete: existing ? () => send('DELETE', '/api/events/volunteer', {
      year: act.year, activity: act.title, role: role ? role.title : '', email: existing.email,
    }) : null,
    deleteLabel: 'Remove',
    confirmDelete: existing ? `Remove ${existing.name} from ${node.title}?` : '',
  });
}

export async function removeVolunteer(act, role, volunteer) {
  const node = role || act;
  if (!confirm(`Remove ${volunteer.name} from ${node.title}?`)) {
    return;
  }
  try {
    await send('DELETE', '/api/events/volunteer', {year: act.year, activity: act.title, role: role ? role.title : '', email: volunteer.email});
    await reload();
  } catch (err) {
    toast(err.message);
  }
}

function whenHint() {
  return 'Like 2026-09-24 16:00, or 2026-09-24 for a whole day';
}

// openActivity edits an activity, or with none adds one: an admin's addition
// opens right away, anyone else's is a suggestion that waits for approval.
export function openActivity(act, options) {
  const opts = options || {};
  const admin = isAdmin();
  const suggesting = !act && !admin;
  const yearOptions = allYears().map(y => ({label: y, value: y}));
  const known = new Set(yearOptions.map(y => y.value));
  for (const y of [years().current, years().next]) {
    if (!known.has(y)) {
      yearOptions.unshift({label: y, value: y});
    }
  }
  const year = select(yearOptions, act ? act.year : years().current);
  const title = text(act ? act.title : '', {required: true, maxLength: 120});
  const category = select(state.model.categories.map(c => c.title), act ? act.category : (opts.category || state.model.categories[0].title));
  const status = select(['Pending', 'Open', 'Done', 'Hidden'], act ? act.status : 'Open');
  const description = textarea(act ? act.description : '', 6);
  const timing = text(act ? act.timing : '', {placeholder: 'All Year, Late February, A few times per year'});
  const start = text(act ? act.start : '', {placeholder: '2026-09-24 16:00'});
  const end = text(act ? act.end : '', {placeholder: '2026-09-24 18:00'});
  const location = text(act ? act.location : '');
  const spots = text(act && act.spots ? String(act.spots) : '', {type: 'number', placeholder: 'Unlimited'});
  const coLeader = checkbox('Co-leader needed', act ? act.coLeaderNeeded : false);
  const hidden = checkbox('Hide the volunteer list from everyone but co-chairs', act ? act.volunteersHidden : false);
  const direct = checkbox('People can sign up for the activity itself, not just its roles', act ? act.directSignUp : true);
  const coChair = checkbox("I'd be open to co-chairing this", false);
  const image = imagePicker(act ? act.image : '', act ? act.imageUrl : '');
  const fields = [field('Title', title)];
  if (admin || !act) {
    fields.push(field('Year', year));
  }
  fields.push(field('Category', category));
  if (admin && act) {
    fields.push(field('Status', status));
  } else if (act && act.status !== 'Pending') {
    fields.push(field('Status', select(['Open', 'Done'], act.status === 'Done' ? 'Done' : 'Open')));
  }
  fields.push(field('Description', description));
  if (!suggesting) {
    const grid = el('div', 'field-grid');
    grid.append(field('Start', start, whenHint()), field('End', end));
    fields.push(field('Timing', timing, 'Shown when there is no date'), grid, field('Location', location),
      field('Spots', spots, 'How many volunteers can sign up for the activity itself'), image.wrap, coLeader.wrap, hidden.wrap, direct.wrap);
  } else {
    fields.push(field('Timing', timing, 'When would this happen?'), image.wrap, coChair.wrap);
  }
  const statusField = fields.find(f => f.firstChild && f.firstChild.textContent === 'Status');
  openModal(act ? 'Edit Activity' : (suggesting ? 'Suggest an Idea' : 'Add Activity'), fields, {
    saveLabel: suggesting ? 'Suggest' : 'Save',
    submit: async () => {
      const body = {
        original: act ? {year: act.year, title: act.title} : {year: '', title: ''},
        year: year.value, title: title.value, category: category.value,
        status: statusField ? statusField.querySelector('select').value : '',
        description: description.value, image: image.value(), timing: timing.value,
        start: start.value, end: end.value, location: location.value, spots: Number(spots.value) || 0,
        coLeaderNeeded: coLeader.input.checked, volunteersHidden: hidden.input.checked, directSignUp: direct.input.checked,
        coChair: coChair.input.checked,
      };
      await send('POST', '/api/events/activity', body);
      if (!act || act.year !== body.year || act.title !== body.title) {
        await goTo(activityPath({year: body.year, title: body.title}));
      }
    },
    onDelete: act && admin ? () => send('DELETE', '/api/events/activity', {year: act.year, title: act.title}) : null,
    confirmDelete: act ? `Delete “${act.title}” (${act.year})? Its roles and links go with it.` : '',
    afterDelete: () => goTo('/'),
  });
}

// openRole edits a role, or adds one under an activity or another role. An
// editor's addition opens right away; anyone else's is a suggestion.
export function openRole(act, role, parent) {
  const editor = act.canEdit;
  const suggesting = !role && !editor;
  const title = text(role ? role.title : '', {required: true, maxLength: 120});
  const parents = [{label: 'The activity itself', value: ''}];
  for (const r of allRoles(act)) {
    if (!role || r.title !== role.title) {
      parents.push({label: r.title, value: r.title});
    }
  }
  const parentSelect = select(parents, role ? role.parent : (parent ? parent.title : ''));
  const group = text(role ? role.group : (parent ? '' : ''), {placeholder: 'Event Support, Booths, Shifts…'});
  const status = select(['Pending', 'Open', 'Done', 'Hidden'], role ? role.status : 'Open');
  const description = textarea(role ? role.description : '', 4);
  const start = text(role ? role.start : '', {placeholder: '2026-09-24 12:30'});
  const end = text(role ? role.end : '', {placeholder: '2026-09-24 16:00'});
  const spots = text(role && role.spots ? String(role.spots) : '', {type: 'number', placeholder: 'Unlimited'});
  const coLeader = checkbox('Co-leader needed', role ? role.coLeaderNeeded : false);
  const hidden = checkbox('Hide the volunteer list from everyone but co-chairs', role ? role.volunteersHidden : false);
  const coChair = checkbox("I'd be open to co-chairing this", false);
  const image = imagePicker(role ? role.image : '', role ? role.imageUrl : '');
  const fields = [field('Title', title), field('Under', parentSelect), field('Description', description)];
  if (!suggesting) {
    const grid = el('div', 'field-grid');
    grid.append(field('Start', start, whenHint()), field('End', end));
    fields.push(field('Group', group, 'Roles with the same group show under one heading'), field('Status', status), grid,
      field('Spots', spots), image.wrap, coLeader.wrap, hidden.wrap);
  } else {
    fields.push(image.wrap, coChair.wrap);
  }
  openModal(role ? 'Edit Role' : (suggesting ? 'Suggest a Role' : 'Add Role'), fields, {
    saveLabel: suggesting ? 'Suggest' : 'Save',
    submit: async () => {
      const body = {
        year: act.year, activity: act.title, original: role ? role.title : '',
        title: title.value, parent: parentSelect.value, group: group.value, status: suggesting ? '' : status.value,
        description: description.value, image: image.value(), start: start.value, end: end.value,
        spots: Number(spots.value) || 0, coLeaderNeeded: coLeader.input.checked, volunteersHidden: hidden.input.checked,
        coChair: coChair.input.checked,
      };
      await send('POST', '/api/events/role', body);
      if (role && role.title !== body.title) {
        await goTo(rolePath(act, {title: body.title}));
      }
    },
    onDelete: role && editor ? () => send('DELETE', '/api/events/role', {year: act.year, activity: act.title, title: role.title}) : null,
    confirmDelete: role ? `Delete the role “${role.title}”?` : '',
    afterDelete: () => goTo(activityPath(act)),
  });
}

export function openLink(act, role, item) {
  const title = text(item ? item.title : '', {required: true, maxLength: 120});
  const url = text(item ? item.url : '', {type: 'url', required: true, placeholder: 'https://'});
  const image = imagePicker(item ? item.image : '', item ? item.imageUrl : '');
  openModal(item ? 'Edit Link' : 'Add Link', [field('Title', title), field('URL', url), image.wrap], {
    submit: () => send('POST', '/api/events/link', {
      year: act.year, activity: act.title, role: role ? role.title : '', original: item ? item.title : '',
      title: title.value, url: url.value, image: image.value(),
    }),
    onDelete: item ? () => send('DELETE', '/api/events/link', {year: act.year, activity: act.title, role: role ? role.title : '', title: item.title}) : null,
    confirmDelete: item ? `Remove the link “${item.title}”?` : '',
  });
}

export function openCategory(category) {
  const title = text(category ? category.title : '', {required: true, maxLength: 120});
  const description = textarea(category ? category.description : '', 3);
  openModal(category ? 'Edit Category' : 'Add Category', [field('Title', title), field('Description', description)], {
    submit: () => send('POST', '/api/events/category', {original: category ? category.title : '', title: title.value, description: description.value}),
    onDelete: category ? () => send('DELETE', '/api/events/category', {title: category.title}) : null,
    confirmDelete: category ? `Delete the category “${category.title}”?` : '',
  });
}

export function openSettings() {
  const settings = state.model.settings;
  const expense = text(settings.expenseFormUrl, {type: 'url', required: true});
  const intro = textarea(settings.intro, 4);
  openModal('Settings', [field('Expense form URL', expense), field('Intro', intro, 'Shown under the Sign Up heading')], {
    submit: () => send('POST', '/api/events/settings', {expenseFormUrl: expense.value, intro: intro.value}),
  });
}

export async function copyToNextYear(act) {
  if (!confirm(`Copy “${act.title}” and its roles into ${years().next}?`)) {
    return;
  }
  try {
    await send('POST', '/api/events/copy', {year: act.year, title: act.title});
    await reload();
    toast(`Copied to ${years().next}`);
  } catch (err) {
    toast(err.message);
  }
}

// setControls posts one of the editor panel's switches by resubmitting the
// whole record with that field changed.
export async function saveActivityFields(act, changes) {
  const body = {
    original: {year: act.year, title: act.title},
    year: act.year, title: act.title, category: act.category, status: act.status,
    description: act.description || '', image: act.image || '', timing: act.timing || '',
    start: act.start || '', end: act.end || '', location: act.location || '', spots: act.spots || 0,
    coLeaderNeeded: act.coLeaderNeeded, volunteersHidden: act.volunteersHidden, directSignUp: act.directSignUp,
    ...changes,
  };
  try {
    await send('POST', '/api/events/activity', body);
    await reload();
  } catch (err) {
    toast(err.message);
    await reload();
  }
}

export async function saveRoleFields(act, role, changes) {
  const body = {
    year: act.year, activity: act.title, original: role.title,
    title: role.title, parent: role.parent || '', group: role.group || '', status: role.status,
    description: role.description || '', image: role.image || '', start: role.start || '', end: role.end || '',
    spots: role.spots || 0, coLeaderNeeded: role.coLeaderNeeded, volunteersHidden: role.volunteersHidden,
    ...changes,
  };
  try {
    await send('POST', '/api/events/role', body);
    await reload();
  } catch (err) {
    toast(err.message);
    await reload();
  }
}
