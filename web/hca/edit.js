import {state, me, isAdmin, years, allYears, activityPath, activity, descendants, rootOf, eventCategories, headingChoices, UNCATEGORIZED} from './state.js';

function* allNodes() {
  for (const root of state.model.activities) {
    yield root;
    yield* descendants(root);
  }
}
import {el, svg, toast, button, thumb} from './dom.js';

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
    const text = await res.text();
    const err = new Error(text);
    // A conflict comes back as JSON with the details of what stands in the
    // way, so the caller can offer a way round it.
    if (res.status === 409 && (res.headers.get('Content-Type') || '').includes('json')) {
      try {
        Object.assign(err, {conflict: JSON.parse(text)});
        err.message = err.conflict.error || text;
      } catch {
        // Not JSON after all; the text is the message.
      }
    }
    throw err;
  }
}

// prettyHost is where a Pretty ID lives: this site, /v/... - shown beside the
// field so people see the whole address they are choosing.
export function prettyHost() {
  return `${location.host}/v/`;
}

// saveActivity posts one activity, and when its Pretty ID belongs to a prior
// year's activity, asks whether to rename that one out of the way and then
// posts again with the agreement.
export async function saveActivity(body) {
  const before = body.id && activity(body.id) ? activityPath(activity(body.id)) : null;
  try {
    await send('POST', '/api/events/activity', body);
  } catch (err) {
    const c = err.conflict;
    if (!c || !c.prior) {
      throw err;
    }
    if (!confirm(`“${body.prettyId}” is the address of “${c.title}” from ${c.year}. Rename that one to “${c.renamed}” and use “${body.prettyId}” here?`)) {
      throw new Error('Pick another address, or agree to rename the old one.');
    }
    await send('POST', '/api/events/activity', {...body, takeOver: true});
  }
  await reload();
  // Changing the friendly address while on its page moves the page: the old
  // address no longer names anything, so the bar follows to the new one.
  const now = body.id ? activity(body.id) : null;
  if (before && now && location.pathname === before && activityPath(now) !== before) {
    history.replaceState(null, '', activityPath(now));
    const {render} = await import('./app.js');
    render();
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
    const {submit, afterSave} = modalState;
    if (!submit) {
      return;
    }
    setStatus('Saving…');
    try {
      await submit();
      closeModal();
      await reload();
      if (afterSave) {
        afterSave();
      }
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

// segmented is a two-or-more-way switch: one button per choice, the chosen one
// filled. For a choice this small a dropdown hides the other option; this shows
// both. `value` reads the current choice.
function segmented(options, initial, onChange) {
  const wrap = el('div', 'segmented');
  wrap.setAttribute('role', 'radiogroup');
  const state = {value: initial};
  const buttons = options.map(o => {
    const b = el('button', 'segment' + (o.value === initial ? ' is-on' : ''), o.label);
    b.type = 'button';
    b.setAttribute('role', 'radio');
    b.setAttribute('aria-checked', String(o.value === initial));
    b.addEventListener('click', () => {
      state.value = o.value;
      for (const other of buttons) {
        const on = other === b;
        other.classList.toggle('is-on', on);
        other.setAttribute('aria-checked', String(on));
      }
      onChange(o.value);
    });
    wrap.append(b);
    return b;
  });
  return {wrap, get value() { return state.value; }};
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
  f.classList.toggle('modal-wide', Boolean(options.wide));
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
  if (options.submit) {
    const save = el('button', 'button', options.saveLabel || 'Save');
    save.type = 'submit';
    const cancel = el('button', 'button button-secondary', 'Cancel');
    cancel.type = 'button';
    cancel.addEventListener('click', closeModal);
    actions.append(save, cancel);
  } else {
    // A modal that only shows things - the category manager - has nothing to
    // save, so it gets a single way out.
    const done = el('button', 'button', 'Done');
    done.type = 'button';
    done.addEventListener('click', closeModal);
    actions.append(done);
  }
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
  modalState = {submit: options.submit, afterSave: options.afterSave};
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
// of the things under it; with an existing sign-up it edits that one.
// peoplePicker is the searchable directory list behind "Someone else": type a
// few letters, see faces, names and what places each person (a grade, a job,
// "Parent"), pick one. The list is fetched once per page load. Typing a full
// address that matches nobody still works - guests and new families are real.
let peopleCache = null;

async function people() {
  if (!peopleCache) {
    const res = await fetch('/api/events/people');
    peopleCache = res.ok ? await res.json() : [];
  }
  return peopleCache;
}

// personInfo is the directory's row for an address, or null for someone it does
// not know - a guest, or a family that has not been imported yet.
export async function personInfo(email) {
  const all = await people();
  return all.find(p => p.email === email) || null;
}

function peoplePicker() {
  const wrap = el('div', 'people-picker');
  const search = el('input');
  search.type = 'search';
  search.placeholder = 'Search by name…';
  search.autocomplete = 'off';
  const results = el('div', 'people-results');
  results.hidden = true;
  const chosen = el('div', 'people-chosen');
  chosen.hidden = true;
  wrap.append(search, results, chosen);
  let value = '';

  const tile = person => {
    const row = el('button', 'people-row');
    row.type = 'button';
    const face = el('div', 'avatar people-face');
    if (person.photoUrl) {
      const img = el('img');
      img.src = person.photoUrl;
      img.alt = '';
      img.loading = 'lazy';
      face.append(img);
    } else {
      face.textContent = (person.name || person.email).slice(0, 1).toUpperCase();
    }
    const text = el('div', 'people-text');
    text.append(el('div', 'people-name', person.name));
    if (person.title) {
      text.append(el('div', 'people-title', person.title));
    }
    row.append(face, text);
    return row;
  };

  const choose = person => {
    value = person.email;
    chosen.replaceChildren();
    const picked = tile(person);
    picked.disabled = true;
    const clear = el('button', 'link-button', 'Change');
    clear.type = 'button';
    clear.addEventListener('click', () => {
      value = '';
      chosen.hidden = true;
      search.hidden = false;
      search.value = '';
      search.focus();
    });
    chosen.append(picked, clear);
    chosen.hidden = false;
    results.hidden = true;
    search.hidden = true;
  };

  const show = async () => {
    const q = search.value.trim().toLowerCase();
    results.replaceChildren();
    if (!q) {
      results.hidden = true;
      return;
    }
    const all = await people();
    const hits = all.filter(p => p.name.toLowerCase().includes(q) || p.email.toLowerCase().includes(q)).slice(0, 8);
    for (const person of hits) {
      const row = tile(person);
      row.addEventListener('click', () => choose(person));
      results.append(row);
    }
    if (!hits.length) {
      results.append(el('div', 'people-none', q.includes('@') ? `Nobody in the directory - “${q}” will be signed up by email.` : 'Nobody matches.'));
    }
    results.hidden = false;
  };
  search.addEventListener('input', show);
  search.addEventListener('focus', show);

  return {
    wrap,
    // A typed address that matched nobody is still a value; anything else typed
    // and not picked is not.
    value: () => value || (search.value.includes('@') ? search.value.trim() : ''),
  };
}

export function openSignUp(node, existing) {
  const editor = node.canEdit;
  const picker = peoplePicker();
  const emailField = field('Who is it?', picker.wrap, 'Search the directory, or type an address for someone not in it');
  const who = segmented([{label: 'Me', value: ''}, {label: 'Someone else', value: 'other'}],
    existing && existing.email !== me().email ? 'other' : '', value => {
      emailField.hidden = value !== 'other';
      if (value === 'other') {
        picker.wrap.querySelector('input').focus();
      }
    });
  emailField.hidden = who.value !== 'other';
  const positions = [{label: 'Volunteer', value: 'Volunteer'}, {label: 'Volunteer, and open to co-chairing', value: 'Open to Co-Chair'}];
  if (editor) {
    positions.push({label: 'Co-Chair', value: 'Co-Chair'});
  }
  const position = select(positions, existing ? existing.position : 'Volunteer');
  const note = textarea(existing ? existing.note : '', 3);
  const fields = [];
  if (!existing) {
    fields.push(field('Who', who.wrap), emailField);
  }
  fields.push(field('As', position), field('Note', note, 'Anything the organizers should know'));
  openModal(existing ? `Edit sign-up for ${node.title}` : `Sign up for ${node.title}`, fields, {
    saveLabel: existing ? 'Save' : 'Sign Up',
    submit: () => send('POST', '/api/events/volunteer', {
      id: node.id,
      email: existing ? existing.email : (who.value === 'other' ? picker.value() : ''),
      position: position.value, note: note.value,
    }),
    onDelete: existing ? () => send('DELETE', '/api/events/volunteer', {id: node.id, email: existing.email}) : null,
    deleteLabel: 'Remove',
    confirmDelete: existing ? `Remove ${existing.name} from ${node.title}?` : '',
  });
}

export async function removeVolunteer(node, volunteer) {
  if (!confirm(`Remove ${volunteer.name} from ${node.title}?`)) {
    return;
  }
  try {
    await send('DELETE', '/api/events/volunteer', {id: node.id, email: volunteer.email});
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
  // Everything is an activity; what it sits under is the only difference between
  // a headline event and a shift on its sign-up sheet.
  const under = act ? act.parent : (opts.parent ? opts.parent.id : '');
  const root = act ? rootOf(act) : (opts.parent ? rootOf(opts.parent) : null);
  const parents = [{label: 'Nothing - this stands on its own', value: ''}];
  if (root) {
    // Anything in the tree can be the parent except the row itself and what
    // already sits under it - the server refuses the loop, so don't offer it.
    const below = new Set(act ? descendants(act).map(d => d.id) : []);
    for (const node of [root, ...descendants(root)]) {
      if ((!act || node.id !== act.id) && !below.has(node.id)) {
        parents.push({label: node.title, value: node.id});
      }
    }
  }
  const parentSelect = select(parents, under);
  // The category select follows where the row sits: a root picks one of the
  // page's headings, a child picks one of its root event's own, or none. Someone
  // proposing rather than running the event only sees categories that allow it.
  const editor = admin || (root ? root.canEdit : false);
  // Uncategorized is only for editors: it is where things land without a
  // heading, not a heading people propose into.
  const headings = headingChoices(c => editor || c.allowAdding).filter(c => editor || c.value !== UNCATEGORIZED);
  const category = select(headings,
    act ? act.category : (opts.category || (headings[0] ? headings[0].value : '')));
  const own = root ? eventCategories(root).filter(c => editor || c.allowAdding) : [];
  const eventCategory = select([{label: 'None', value: ''}, ...own.map(c => ({label: c.title, value: c.id}))],
    act ? act.category : (opts.category || ''));
  const status = select(['Pending', 'Open', 'Done', 'Hidden'], act ? act.status : 'Open');
  const description = textarea(act ? act.description : '', 6);
  const timing = text(act ? act.timing : '', {placeholder: 'All Year, Late February, A few times per year'});
  const start = text(act ? act.start : '', {placeholder: '2026-09-24 16:00'});
  const end = text(act ? act.end : '', {placeholder: '2026-09-24 18:00'});
  const location = text(act ? act.location : '');
  const spots = text(act && act.spots ? String(act.spots) : '', {type: 'number', placeholder: 'Unlimited'});
  const coLeader = checkbox('Co-leader needed', act ? act.coLeaderNeeded : false);
  // A new thing under a parent starts with the parent's privacy - a private
  // event's committees are usually private too - and can be switched after.
  const hidden = checkbox('Hide the volunteer list from everyone but co-chairs',
    act ? act.volunteersHidden : Boolean(opts.parent && opts.parent.volunteersHidden));
  const direct = checkbox('People can sign up for this itself, not just the things under it', act ? act.directSignUp : true);
  const coChair = checkbox("I'd be open to co-chairing this", false);
  const image = imagePicker(act ? act.image : '', act ? act.imageUrl : '');
  const pretty = text(act ? act.prettyId || '' : '', {placeholder: 'applause', maxLength: 40});
  const fields = [field('Title', title)];
  if (admin || !act) {
    fields.push(field('Year', year));
  }
  if (root) {
    fields.push(field('Under', parentSelect, 'What this sits inside, if anything'));
  }
  if (under) {
    if (own.length) {
      fields.push(field('Category', eventCategory, `One of ${root.title}'s own categories`));
    }
  } else {
    fields.push(field('Category', category));
  }
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
      field('Spots', spots, 'How many volunteers can sign up for the activity itself'), image.wrap, coLeader.wrap, hidden.wrap, direct.wrap,
      field('Friendly address', pretty, under
        ? `Under ${root.title}'s address: ${prettyHost()}${(root.prettyId || root.id)}/… - letters, digits and hyphens; unique among the things beside it`
        : `${prettyHost()}… - letters, digits and hyphens; one address per event, across every year`));
  } else {
    fields.push(field('Timing', timing, 'When would this happen?'), image.wrap, coChair.wrap);
  }
  const statusField = fields.find(f => f.firstChild && f.firstChild.textContent === 'Status');
  openModal(act ? 'Edit' : (suggesting ? 'Suggest an Idea' : (opts.parent ? `Add under ${opts.parent.title}` : 'Add Activity')), fields, {
    saveLabel: suggesting ? 'Suggest' : 'Save',
    submit: async () => {
      const body = {
        id: act ? act.id : '',
        year: year.value, title: title.value, parent: parentSelect.value,
        category: parentSelect.value ? eventCategory.value : category.value,
        status: statusField ? statusField.querySelector('select').value : '',
        description: description.value, image: image.value(), timing: timing.value,
        start: start.value, end: end.value, location: location.value, spots: Number(spots.value) || 0,
        coLeaderNeeded: coLeader.input.checked, volunteersHidden: hidden.input.checked, directSignUp: direct.input.checked,
        coChair: coChair.input.checked, prettyId: pretty.value.trim().toLowerCase(),
      };
      await saveActivity(body);
      if (!act) {
        // The server minted the id, so find the new row by the one thing we
        // know about it - the reload has already run, so the model is current.
        await reload();
        const made = [...allNodes()].find(n => n.year === body.year && n.title === body.title && n.parent === body.parent);
        if (made) {
          await goTo(activityPath(made));
        }
      }
    },
    onDelete: act && admin ? () => send('DELETE', '/api/events/activity', {id: act.id}) : null,
    confirmDelete: act ? `Delete “${act.title}” (${act.year})? Its links go with it.` : '',
    afterDelete: () => goTo('/'),
  });
}

export function openLink(node, item) {
  const title = text(item ? item.title : '', {required: true, maxLength: 120});
  const url = text(item ? item.url : '', {type: 'url', required: true, placeholder: 'https://'});
  const image = imagePicker(item ? item.image : '', item ? item.imageUrl : '');
  openModal(item ? 'Edit Link' : 'Add Link', [field('Title', title), field('URL', url), image.wrap], {
    submit: () => send('POST', '/api/events/link', {
      id: node.id, original: item ? item.title : '',
      title: title.value, url: url.value, image: image.value(),
    }),
    onDelete: item ? () => send('DELETE', '/api/events/link', {id: node.id, title: item.title}) : null,
    confirmDelete: item ? `Remove the link “${item.title}”?` : '',
  });
}

// openVolunteerGrid is the organizers' roster for an event: every sign-up in the
// tree, with where it is (category > committee > ... , or "(itself)"), the
// position, the address, and for a student the parents' addresses - the people
// an organizer actually needs to reach. Copy emails copies every address in it.
export function openVolunteerGrid(root, nodes, pathOf) {
  const rows = [];
  for (const node of nodes) {
    for (const v of node.volunteers) {
      rows.push({node, v, parents: []});
    }
  }
  // The columns as text, for copying: each header has a glyph that copies its
  // column one value per line, and the toolbar copies the whole table
  // tab-separated with its headings, which pastes into a spreadsheet as cells.
  const columns = [
    {label: 'Volunteer', get: r => r.v.name || r.v.email},
    {label: 'Where', get: r => pathOf(r.node)},
    {label: 'As', get: r => r.v.position},
    {label: 'Email', get: r => r.v.email},
    {label: "Parents' email", get: r => r.parents.join(', ')},
  ];
  const copied = (btn, icon, label) => {
    btn.classList.add('copied');
    btn.replaceChildren(svg('check'));
    setTimeout(() => {
      btn.classList.remove('copied');
      btn.replaceChildren(svg(icon));
      if (label) {
        btn.append(el('span', '', label));
      }
    }, 1200);
  };
  const copyGlyph = (title, text) => {
    const btn = el('button', 'copy-glyph');
    btn.type = 'button';
    btn.title = title;
    btn.append(svg('copy'));
    btn.addEventListener('click', () => {
      navigator.clipboard.writeText(text()).then(() => copied(btn, 'copy'), () => toast('Could not copy'));
    });
    return btn;
  };
  const table = el('table', 'roster');
  const head = el('tr');
  for (const c of columns) {
    const th = el('th');
    th.append(el('span', '', c.label), copyGlyph(`Copy the ${c.label} column`, () => rows.map(c.get).filter(Boolean).join('\n')));
    head.append(th);
  }
  const thead = el('thead');
  thead.append(head);
  table.append(thead);
  const body = el('tbody');
  const addresses = new Set();
  const parentCells = [];
  for (const row of rows) {
    const {node, v} = row;
    const tr = el('tr');
    const who = el('td', 'roster-who');
    const face = el('div', 'avatar people-face');
    if (v.photoUrl) {
      const img = el('img');
      img.src = v.photoUrl;
      img.alt = '';
      face.append(img);
    } else {
      face.textContent = (v.name || v.email).slice(0, 1).toUpperCase();
    }
    who.append(face, el('span', '', v.name));
    tr.append(who);
    tr.append(el('td', 'roster-where', pathOf(node)));
    tr.append(el('td', '', v.position));
    const mail = el('td', 'roster-mail');
    mail.append(mailto(v.email));
    tr.append(mail);
    addresses.add(v.email);
    const parents = el('td', 'roster-mail');
    parentCells.push({cell: parents, row});
    tr.append(parents);
    body.append(tr);
  }
  table.append(body);
  // The directory lookup is the slow part, and only the parents column needs
  // it, so the roster shows at once and that column fills in behind it.
  people().then(all => {
    const byEmail = new Map(all.map(p => [p.email, p]));
    for (const {cell, row} of parentCells) {
      const info = byEmail.get(row.v.email);
      if (!info || !info.isStudent) {
        continue;
      }
      row.parents = info.parentEmails || [];
      for (const e of row.parents) {
        cell.append(mailto(e));
        addresses.add(e);
      }
      if (!row.parents.length) {
        cell.append(el('span', 'roster-none', 'none listed'));
      }
    }
  });
  const scroll = el('div', 'roster-scroll');
  scroll.append(rows.length ? table : el('div', 'panel-empty', 'Nobody has signed up yet.'));
  const tools = el('div', 'roster-tools');
  tools.append(el('span', 'roster-count', `${rows.length} sign-up${rows.length === 1 ? '' : 's'}`));
  const buttons = el('div', 'roster-buttons');
  const copyTable = button('Copy table', 'copy', 'button button-secondary button-small', () => {
    const text = [columns.map(c => c.label).join('\t'), ...rows.map(r => columns.map(c => c.get(r)).join('\t'))].join('\n');
    navigator.clipboard.writeText(text).then(() => copied(copyTable, 'copy', 'Copy table'), () => toast('Could not copy'));
  });
  copyTable.title = 'Copy every column, ready to paste into a spreadsheet';
  const copyEmails = button('Copy emails', 'copy', 'button button-secondary button-small', () => {
    navigator.clipboard.writeText([...addresses].join(', ')).then(() => copied(copyEmails, 'copy', 'Copy emails'), () => toast('Could not copy'));
  });
  copyEmails.title = "Every volunteer's address, and their parents' for students, comma-separated";
  buttons.append(copyTable, copyEmails);
  tools.append(buttons);
  openModal(`${root.title}: Volunteers`, [tools, scroll], {wide: true});
}

function mailto(email) {
  const a = el('a', 'roster-link', email);
  a.href = `mailto:${email}`;
  return a;
}

// editPencil and fieldEditor are the inline editing pattern Helios Who? uses:
// a small pencil beside a value, and on click the value and its pencil step
// aside for an input with Save/Cancel right there. Nothing else on the page
// moves, and the reader never leaves the page to change one thing.
export function editPencil(label) {
  const pencil = el('button', 'edit-icon');
  pencil.type = 'button';
  pencil.title = label;
  pencil.setAttribute('aria-label', label);
  pencil.append(svg('edit'));
  return pencil;
}

// opts: {input, value(), submit(value), hint}. A successful submit reloads the
// model and repaints the page, which takes the editor with it - so there is no
// close-on-success path to get wrong. Failures surface as a toast, the way every
// other write in this app reports itself.
export function fieldEditor(anchor, pencil, opts) {
  const box = el('div', 'field-editor');
  box.append(opts.input);
  if (opts.hint) {
    box.append(el('small', 'field-note', opts.hint));
  }
  const actions = el('div', 'field-editor-actions');
  const save = el('button', 'button button-small', 'Save');
  save.type = 'button';
  const cancel = el('button', 'button button-secondary button-small', 'Cancel');
  cancel.type = 'button';
  const status = el('span', 'field-status');
  actions.append(save, cancel, status);
  box.append(actions);
  const close = () => {
    box.remove();
    anchor.hidden = false;
    pencil.hidden = false;
  };
  cancel.addEventListener('click', close);
  save.addEventListener('click', async () => {
    const value = opts.value();
    // A validate() that returns a message stops the save and says why, right
    // beside the buttons - nothing is sent and the editor stays open.
    const problem = opts.validate ? opts.validate(value) : '';
    if (problem) {
      status.classList.add('error');
      status.textContent = problem;
      return;
    }
    status.classList.remove('error');
    save.disabled = true;
    status.textContent = 'Saving…';
    await opts.submit(value);
    save.disabled = false;
    status.textContent = '';
  });
  box.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      e.stopPropagation();
      close();
    }
    if (e.key === 'Enter' && opts.input.tagName !== 'TEXTAREA') {
      e.preventDefault();
      save.click();
    }
  });
  anchor.hidden = true;
  pencil.hidden = true;
  anchor.after(box);
  opts.input.focus();
}

// editable hangs a pencil off a rendered value and wires it to one field of the
// activity. The caller says how to render the input and what to send.
export function editable(anchor, label, make, submit) {
  const pencil = editPencil(label);
  pencil.addEventListener('click', () => {
    const built = make();
    fieldEditor(anchor, pencil, {
      input: built.input,
      hint: built.hint,
      value: built.value,
      validate: built.validate,
      submit,
    });
  });
  return pencil;
}

export function textInput(value, options) {
  return text(value, options);
}

export function textAreaInput(value, rows) {
  return textarea(value, rows);
}

export function selectInput(options, value) {
  return select(options, value);
}

// uploadAndSave uploads the chosen file and records it in one step: the upload
// returns a name, and that name is the only field saved, so the hero updates as
// soon as it lands with no separate Save to remember. `save` is whichever of
// saveActivityFields, which is every node's save now.
export async function uploadAndSave(save, file) {
  try {
    const name = await uploadImage(file);
    await save({image: name});
  } catch (err) {
    toast(err.message);
  }
}

// openCategory edits one category, or adds one to `eventId`'s event - or to the
// page's headings when eventId is empty.
export function openCategory(category, eventId, after) {
  const title = text(category ? category.title : '', {required: true, maxLength: 120});
  const description = textarea(category ? category.description : '', 3);
  const image = imagePicker(category ? category.image : '', category ? category.imageUrl : '');
  const adding = checkbox('People can add new things to this category', category ? category.allowAdding : true);
  const fields = [field('Title', title), field('Description', description), image.wrap, adding.wrap];
  // Only a page heading can be kept off the main page; it still sits in the
  // rail with its count, and clicking it there shows its events.
  const onMain = eventId ? null : checkbox('Show on the main page (it stays in the toolbar either way)', category ? category.showOnMain : true);
  if (onMain) {
    fields.push(onMain.wrap);
  }
  openModal(category ? 'Edit Category' : 'Add Category', fields, {
    submit: () => send('POST', '/api/events/category', {
      id: category ? category.id : '', eventId: eventId || '',
      title: title.value, description: description.value, image: image.value(), allowAdding: adding.input.checked,
      showOnMain: onMain ? onMain.input.checked : true,
    }),
    afterSave: after,
    onDelete: category ? () => send('DELETE', '/api/events/category', {id: category.id}) : null,
    confirmDelete: category ? `Delete the category “${category.title}”?` : '',
    afterDelete: after,
  });
}

// categoryList is the one list of categories with reorder, edit and add. The
// Admin Tools card and the manager an editor opens from an event both show it,
// so they cannot drift. `after` runs once the model has reloaded from a change -
// the manager passes itself, so it comes back showing the new state; the admin
// page passes nothing, since it re-renders on its own.
export function categoryList(root, after) {
  const wrap = el('div', 'category-list');
  const eventId = root ? root.id : '';
  const list = root ? eventCategories(root) : state.model.categories.filter(c => !c.builtIn);
  const move = async (from, to) => {
    const ids = list.map(c => c.id);
    const [moved] = ids.splice(from, 1);
    ids.splice(to, 0, moved);
    try {
      await send('POST', '/api/events/categories/order', {eventId, ids});
      await reload();
      if (after) {
        after();
      }
    } catch (err) {
      toast(err.message);
    }
  };
  list.forEach((category, i) => {
    const row = el('div', 'admin-row');
    if (category.imageUrl) {
      row.append(thumb(category.imageUrl, category.title, 'small'));
    }
    const body = el('div', 'grow');
    body.append(el('div', '', category.title));
    body.append(el('div', 'sub', [category.description, category.allowAdding ? 'People can add here' : 'Only organizers add here',
      !root && !category.showOnMain ? 'Toolbar only' : ''].filter(Boolean).join(' · ')));
    const up = button('', 'up', 'icon-button', () => move(i, i - 1));
    up.setAttribute('aria-label', `Move ${category.title} up`);
    up.disabled = i === 0;
    const down = button('', 'down', 'icon-button', () => move(i, i + 1));
    down.setAttribute('aria-label', `Move ${category.title} down`);
    down.disabled = i === list.length - 1;
    row.append(body, up, down, button('Edit', 'edit', 'button button-secondary button-small', () => openCategory(category, eventId, after)));
    wrap.append(row);
  });
  if (!list.length) {
    wrap.append(el('div', 'panel-empty', root ? `${root.title} has no categories of its own yet.` : 'No categories yet.'));
  }
  if (!root) {
    wrap.append(el('div', 'hint', 'Uncategorized is built in: it collects events whose category is blank or names nothing here.'));
  }
  const add = el('div', 'add-row');
  add.append(button('Add Category', 'plus', 'button', () => openCategory(null, eventId, after)));
  wrap.append(add);
  return wrap;
}

// openCategoryManager is an event's own categories in a modal, for whoever runs
// it: the headings its committees, booths and shifts are grouped under.
export function openCategoryManager(root) {
  const again = () => openCategoryManager(root);
  openModal(`${root.title}: Categories`, [
    el('div', 'hint', 'The things under this event are grouped by these, in this order. Each one says whether people may add to it.'),
    categoryList(root, again),
  ], {});
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
  if (!confirm(`Copy “${act.title}” and everything under it into ${years().next}?`)) {
    return;
  }
  try {
    await send('POST', '/api/events/copy', {id: act.id});
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
    id: act.id,
    year: act.year, title: act.title, parent: act.parent || '',
    category: act.category || '', status: act.status,
    description: act.description || '', image: act.image || '', timing: act.timing || '',
    start: act.start || '', end: act.end || '', location: act.location || '', spots: act.spots || 0,
    coLeaderNeeded: act.coLeaderNeeded, volunteersHidden: act.volunteersHidden, directSignUp: act.directSignUp,
    prettyId: act.prettyId || '',
    ...changes,
  };
  try {
    await saveActivity(body);
  } catch (err) {
    toast(err.message);
    await reload();
  }
}

