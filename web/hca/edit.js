import {state, me, isAdmin, years, allYears, activityPath, activity, parentOf, canAdd, ADDING, descendants, rootOf, eventCategories, headingChoices, UNCATEGORIZED, isFamily} from './state.js';

function* allNodes() {
  for (const root of state.model.activities) {
    yield root;
    yield* descendants(root);
  }
}
import {el, svg, toast, button, thumb, whenEditor} from './dom.js';
import {openCropTool} from './crop.js';

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

// openSheet is a second layer above the modal for something the form needs
// mid-edit - the image search - so the form underneath keeps every field
// typed into it. It is its own overlay, made and removed per use; the main
// modal has no idea it was there.
function openSheet(title, nodes, options) {
  const layer = el('div', 'modal-overlay modal-sheet');
  const box = el('div', 'modal' + (options && options.wide ? ' modal-wide' : ''));
  const header = el('div', 'modal-header');
  header.append(el('h2', '', title));
  const close = el('button', 'modal-close', '×');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  const shut = () => {
    layer.remove();
    document.removeEventListener('keydown', onKey, true);
  };
  const onKey = e => {
    if (e.key === 'Escape') {
      e.stopImmediatePropagation();
      shut();
    }
  };
  close.addEventListener('click', shut);
  layer.addEventListener('click', e => {
    if (e.target === layer) {
      shut();
    }
  });
  document.addEventListener('keydown', onKey, true);
  header.append(close);
  box.append(header, ...nodes);
  layer.append(box);
  document.body.append(layer);
  return shut;
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
  wrap.append(typeof label === 'string' ? el('span', '', label) : label, input);
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

// checkbox is a labelled switch - the same control the page's toggle rows use -
// with an optional line of explanation under the label.
export function checkbox(label, checked, hint) {
  const wrap = el('label', 'field field-toggle');
  const input = el('input');
  input.type = 'checkbox';
  input.checked = Boolean(checked);
  const text = el('span');
  text.append(el('span', '', label));
  if (hint) {
    text.append(el('small', '', hint));
  }
  const knob = el('span', 'switch');
  knob.append(input, el('span'));
  wrap.append(knob, text);
  return {wrap, input};
}

// imagePicker uploads on selection, so the save that follows only records the
// name the server handed back. `dropzone` is the large form: a dashed area that
// takes a dropped file or a click, with the Choose button inside it.
function imagePicker(current, currentUrl, options) {
  const dropzone = Boolean(options && options.dropzone);
  const wrap = el('div', 'field');
  wrap.append(el('span', '', 'Image'));
  if (options && options.hint) {
    wrap.append(el('small', 'field-lead', options.hint));
  }
  const row = el('div', dropzone ? 'image-drop' : 'image-row');
  const preview = el('img');
  preview.alt = '';
  const placeholder = el('div', 'image-placeholder');
  if (dropzone) {
    placeholder.append(svg('image'), el('strong', '', 'Drag and drop an image here'), el('small', '', 'or click to choose a file'));
  } else {
    placeholder.append(svg('image'), el('strong', '', 'No image'), el('small', '', 'JPG, PNG or GIF'));
  }
  const choose = el('label', 'button button-secondary button-small', dropzone ? 'Choose image' : 'Choose');
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  choose.append(file);
  const remove = el('button', 'link-button', 'Remove');
  remove.type = 'button';
  const crop = el('button', 'link-button', 'Crop');
  crop.type = 'button';
  let name = current || '';
  const show = url => {
    preview.hidden = !url;
    placeholder.hidden = Boolean(url);
    remove.hidden = !url;
    crop.hidden = !url || Boolean(options && options.plain);
    if (url) {
      preview.src = url;
    }
  };
  show(currentUrl);
  const upload = async picked => {
    if (!picked) {
      return;
    }
    setStatus('Uploading image…');
    try {
      name = await uploadImage(picked);
      show('/' + name);
      setStatus('');
    } catch (err) {
      setStatus(err.message, true);
    }
    file.value = '';
  };
  file.addEventListener('change', () => upload(file.files[0]));
  remove.addEventListener('click', () => {
    name = '';
    show('');
  });
  crop.addEventListener('click', () => openCropTool(preview.src, false, async blob => {
    await upload(new File([blob], 'crop.jpg', {type: 'image/jpeg'}));
    return true;
  }));
  const plain = Boolean(options && options.plain);
  const google = el('button', 'button button-secondary button-small image-find', 'Find an image');
  google.type = 'button';
  google.hidden = plain || !imageSearchOn();
  google.addEventListener('click', e => {
    e.stopPropagation();
    openImageSearch(options && options.query ? options.query() : '', async picked => {
      name = picked;
      show('/' + picked);
    });
  });
  if (dropzone) {
    // The zone itself takes a click (anywhere but the buttons) and a drop.
    row.addEventListener('click', e => {
      if (!e.target.closest('label, button')) {
        file.click();
      }
    });
    row.addEventListener('dragover', e => {
      e.preventDefault();
      row.classList.add('is-dragover');
    });
    row.addEventListener('dragleave', () => row.classList.remove('is-dragover'));
    row.addEventListener('drop', e => {
      e.preventDefault();
      row.classList.remove('is-dragover');
      upload(e.dataTransfer.files[0]);
    });
    const buttons = el('div', 'image-drop-buttons');
    buttons.append(choose, google);
    row.append(preview, placeholder, buttons, el('small', 'image-drop-note', 'JPG, PNG or GIF (max 8 MB)'), crop, remove);
  } else {
    row.append(preview, placeholder, choose, google, crop, remove);
  }
  wrap.append(row);
  return {wrap, value: () => name};
}

// settingCard is a bordered panel for one setting: its name and a sentence,
// then whatever controls it takes, one under another.
function settingCard(label, hint, ...controls) {
  const card = el('div', 'setting-card');
  card.append(el('div', 'setting-label', label), el('div', 'setting-hint', hint));
  for (const c of controls) {
    card.append(c);
  }
  return card;
}

// settingRow is one line of a settings-style form: what the setting is and a
// sentence about it on the left, the control on the right, a hairline under.
function settingRow(label, hint, control) {
  const row = el('div', 'setting-row');
  const text = el('div', 'setting-text');
  text.append(el('div', 'setting-label', label));
  if (hint) {
    text.append(el('div', 'setting-hint', hint));
  }
  const side = el('div', 'setting-control');
  side.append(control);
  row.append(text, side);
  return row;
}

// tabbedFields lays a long form out as tabs. Every field stays in the form -
// only the panels hide - so values and validation survive switching. A field
// the browser refuses on submit brings its own tab forward, since a hidden
// invalid control would otherwise block the save without a word.
function tabbedFields(panels) {
  const wrap = el('div', 'form-tabs');
  const bar = el('div', 'tabs light');
  const bodies = [];
  let active = 0;
  const show = i => {
    active = i;
    bar.querySelectorAll('button').forEach((b, j) => b.classList.toggle('is-active', j === i));
    bodies.forEach((body, j) => {
      body.hidden = j !== i;
    });
  };
  panels.forEach((panel, i) => {
    const tab = el('button', i === 0 ? 'is-active' : '');
    tab.type = 'button';
    if (panel.icon) {
      tab.append(svg(panel.icon));
    }
    tab.append(el('span', '', panel.label));
    tab.addEventListener('click', () => show(i));
    bar.append(tab);
    const body = el('div', 'form-tab-body');
    body.hidden = i !== 0;
    body.append(...panel.fields);
    body.addEventListener('invalid', () => {
      if (active !== i) {
        show(i);
      }
    }, true);
    bodies.push(body);
  });
  wrap.append(bar, ...bodies);
  return wrap;
}

function openModal(title, fields, options) {
  const f = form();
  f.replaceChildren();
  f.classList.toggle('modal-wide', options.wide === true);
  f.classList.toggle('modal-person', options.wide === 'person');
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
  if (options.actions === false) {
    // A window whose body carries its own way out - the person card's Done.
    actions.hidden = true;
  } else if (options.submit) {
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

// whoProfile is a person's page on Helios Who?, on this tier: hca.x.heliosian.com
// pairs with who.x.heliosian.com, and the page is named by the address's local
// part, as Who's own links are.
export function whoProfile(email) {
  const host = location.host.replace(/^hca\./, 'who.');
  return `${location.protocol}//${host}/people/${encodeURIComponent((email || '').split('@')[0])}`;
}

// openPerson opens from a person's chip: a header with their face, name and
// how to reach them, then the contact card. For whoever runs the event a
// volunteer's chip adds a Sign up tab beside Contact, with their sign-up and
// the co-chair appointment, since a chair clicking a face wants either.
export async function openPerson(v, node) {
  const info = await personInfo(v.email);
  const head = personHead(v, info);
  const contact = personContact(v, info);
  // Whoever runs the thing reaches the sign-up from the face, and so does
  // the person's own household.
  if (!node || !(node.canEdit || isFamily(v.email)) || !v.position) {
    const done = el('button', 'button button-secondary', 'Done');
    done.type = 'button';
    done.addEventListener('click', closeModal);
    openModal('', [head, contact, personFoot(v, info, done)], {actions: false, wide: 'person'});
    return;
  }
  const form = signUpForm(node, v);
  const tabs = tabbedFields([
    {label: 'Contact', icon: 'people', fields: [contact, personFoot(v, info)]},
    {label: 'Sign up', icon: 'edit', fields: form.fields},
  ]);
  openModal('', [head, tabs], {...form, wide: 'person'});
}

// personHead is the top of a person's window: the big face, the name with
// pronouns and what places them (grade and classroom, job and department, or
// Parent), and captioned round buttons to email, text, call or copy them.
function personHead(v, info) {
  const head = el('div', 'who-head');
  const face = el('div', 'avatar who-face');
  const photo = (info && info.photoUrl) || v.photoUrl;
  if (photo) {
    const img = el('img');
    img.src = photo;
    img.alt = '';
    face.append(img);
  } else {
    face.textContent = (v.name || v.email).slice(0, 1).toUpperCase();
  }
  const names = el('div', 'who-names');
  names.append(el('div', 'who-name', (info && info.name) || v.name || v.email));
  if (info && info.pronouns) {
    names.append(el('div', 'who-sub', info.pronouns));
  }
  if (info) {
    const place = info.isStudent
      ? [info.grade, info.classroom].filter(Boolean).join(' · ')
      : [info.jobTitle, info.department].filter(Boolean).join(' · ') || info.title;
    if (place) {
      names.append(el('div', 'who-sub', place));
    }
  }
  const actions = el('div', 'who-actions');
  const action = (icon, label, href, onClick) => {
    const a = el(href ? 'a' : 'button', 'who-action');
    if (href) {
      a.href = href;
    } else {
      a.type = 'button';
      a.addEventListener('click', onClick);
    }
    a.title = label;
    const round = el('span', 'icon-round');
    round.append(svg(icon));
    a.append(round, el('span', 'who-action-label', label));
    actions.append(a);
  };
  action('mail', 'Email', `mailto:${v.email}`);
  const phone = info && info.phone ? info.phone : '';
  const digits = phone.replace(/[^+\d]/g, '');
  if (digits) {
    action('chat', 'Message', `sms:${digits}`);
    action('phone', 'Call', `tel:${digits}`);
  }
  action('copy', 'Copy info', null, () => {
    const lines = [(info && info.name) || v.name || '', v.email, phone].filter(Boolean);
    navigator.clipboard.writeText(lines.join('\n')).then(() => toast('Contact info copied'), () => toast('Could not copy'));
  });
  head.append(face, names, actions);
  return head;
}

// personContact is the card's rows: how to reach them, then the household -
// each name a chip that opens that person's own window. Someone the directory
// does not know gets just their address.
function personContact(v, info) {
  const card = el('div', 'who-rows');
  let group = null;
  const row = (icon, label, value) => {
    if (!group) {
      group = el('div', 'who-group');
      card.append(group);
    }
    const r = el('div', 'who-row');
    r.append(svg(icon), el('span', 'who-label', label), typeof value === 'string' ? el('span', 'who-value', value) : value);
    group.append(r);
  };
  const divide = () => {
    group = null;
  };
  row('mail', 'Email', v.email);
  if (info && info.phone) {
    row('phone', 'Phone', info.phone);
  }
  const chips = list => {
    const wrap = el('div', 'who-card-chips');
    for (const p of list) {
      const chip = el('button', 'who-card-chip', p.grade ? `${p.name} (${p.grade})` : p.name);
      chip.type = 'button';
      chip.addEventListener('click', () => openPerson({email: p.email, name: p.name}));
      wrap.append(chip);
    }
    return wrap;
  };
  const household = info && ((info.spouses && info.spouses.length) || (info.children && info.children.length) || (info.isStudent && info.parentEmails && info.parentEmails.length));
  if (household) {
    divide();
  }
  if (info && info.spouses && info.spouses.length) {
    row('people', info.spouses.length === 1 ? 'Partner' : 'Partners', chips(info.spouses));
  }
  if (info && info.children && info.children.length) {
    row('person', info.children.length === 1 ? 'Child' : 'Children', chips(info.children));
  }
  if (info && info.isStudent && info.parentEmails && info.parentEmails.length) {
    const parents = el('div');
    for (const e of info.parentEmails) {
      const a = el('a', 'who-card-link', e);
      a.href = `mailto:${e}`;
      parents.append(a);
    }
    row('people', 'Parents', parents);
  }
  return card;
}

// personFoot is the band under the card: the way through to their Helios Who?
// page for someone the directory knows, and whatever button closes the window.
function personFoot(v, info, done) {
  const foot = el('div', 'who-foot');
  if (info) {
    const profile = el('a', 'button', '');
    profile.append(svg('open'), el('span', '', 'Open Helios Who? Profile'));
    profile.href = whoProfile(v.email);
    profile.target = '_blank';
    profile.rel = 'noopener';
    foot.append(profile);
  }
  if (done) {
    foot.append(done);
  }
  return foot.children.length ? foot : el('div');
}

export function peoplePicker() {
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
  const form = signUpForm(node, existing);
  openModal(existing ? `Edit sign-up for ${node.title}` : `Sign up for ${node.title}`, form.fields, form);
}

// signUpForm is the sign-up editor's fields and the modal options that save
// them, so the same form opens on its own and as a tab of a person's window.
function signUpForm(node, existing) {
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
  // Nobody picks Co-Chair for themselves: it is an appointment by whoever runs
  // the event - an admin or one of its co-chairs - so only they see it in the
  // list. Anyone else chooses between the two kinds of volunteer, and a sitting
  // co-chair editing their own note keeps the position.
  const isChair = Boolean(existing && existing.position === 'Co-Chair');
  const positions = [{label: 'Volunteer', value: 'Volunteer'}, {label: 'Volunteer, and open to co-chairing', value: 'Open to Co-Chair'}];
  if (editor) {
    positions.push({label: 'Co-Chair', value: 'Co-Chair'});
  }
  const position = select(positions, existing && (editor || !isChair) ? existing.position : 'Volunteer');
  const note = textarea(existing ? existing.note : '', 3);
  const fields = [];
  if (!existing) {
    fields.push(field('Who', who.wrap), emailField);
  }
  if (isChair && !editor) {
    fields.push(field('Availability', el('div', 'field-static', 'Co-Chair')));
  } else {
    fields.push(field('Availability', position));
  }
  note.placeholder = 'Anything the organizers should know';
  const noteLabel = el('span', '', 'Note ');
  noteLabel.append(el('small', '', '(optional)'));
  fields.push(field(noteLabel, note));
  // Whoever runs the event appoints a co-chair - or lets one step down - with
  // one button; the change is saved at once, since it is a decision rather
  // than part of the form.
  if (existing && editor) {
    const card = el('div', 'appoint-card');
    const icon = el('div', 'appoint-icon');
    icon.append(svg('people'));
    const text = el('div', 'setting-text');
    text.append(el('div', 'setting-label', isChair ? 'Co-chair' : 'Make co-chair'),
      el('div', 'setting-hint', isChair ? `${existing.name} runs this with you.` : 'Give this person co-chair permissions for this role.'));
    const appoint = button(isChair ? 'Remove as co-chair' : 'Make co-chair', isChair ? 'close' : 'plus',
      'button button-secondary button-small', async () => {
        try {
          await send('POST', '/api/events/volunteer', {
            id: node.id, email: existing.email, position: isChair ? 'Volunteer' : 'Co-Chair', note: note.value,
          });
          closeModal();
          await reload();
          toast(isChair ? `${existing.name} is no longer a co-chair` : `${existing.name} is now a co-chair`);
        } catch (err) {
          toast(err.message);
        }
      });
    card.append(icon, text, appoint);
    fields.push(card);
  }
  return {
    fields,
    saveLabel: existing ? 'Save' : 'Sign Up',
    submit: () => send('POST', '/api/events/volunteer', {
      id: node.id,
      email: existing ? existing.email : (who.value === 'other' ? picker.value() : ''),
      position: isChair && !editor ? 'Co-Chair' : position.value, note: note.value,
    }),
    onDelete: existing ? () => send('DELETE', '/api/events/volunteer', {id: node.id, email: existing.email}) : null,
    deleteLabel: 'Remove',
    confirmDelete: existing ? `Remove ${existing.name} from ${node.title}?` : '',
  };
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

// highlightInputs is the callout's three fields - the headline, the body and
// an emoji for it, with a row of usual ones to tap or any typed in - shared by
// the activity editor's Basics tab and the page's inline pencil. Its value is
// null when there is nothing to show, which is how a highlight comes off.
export function highlightInputs(current) {
  const wrap = el('div', 'highlight-inputs');
  const headline = text(current ? current.headline : '', {maxLength: 80, placeholder: 'Performances'});
  const body = textarea(current ? current.body : '', 4);
  body.placeholder = 'A few lines people should not miss. **Double stars** make words bold.';
  const icon = text(current ? current.icon || '' : '', {maxLength: 8, placeholder: '📣'});
  icon.classList.add('highlight-icon-input');
  const picks = el('div', 'emoji-picks');
  // The usual suspects for a school callout - announce, warn, celebrate, feed,
  // perform, build, thank - any emoji still goes in the box.
  const usual = [
    '📣', '📢', '⭐', '✨', '⚠️', '❗', '💡', '✅', '📌', '📅', '⏰', '🗓️',
    '🎉', '🎊', '🎈', '🎂', '🎁', '❤️', '🙏', '👏', '🤝', '🙋', '👋', '👨‍👩‍👧',
    '🎭', '🎶', '🎤', '🎨', '📸', '🎬', '📚', '✏️', '🏫', '🎓', '🔬', '🧩',
    '🍕', '🍪', '☕', '🧁', '🥗', '🍎', '🌮', '🍜', '🍿', '🧃',
    '🏃', '⚽', '🚴', '🥾', '🌳', '🌸', '🌍', '☀️', '🌧️', '🔥',
    '🛠️', '🧹', '📦', '🚗', '🅿️', '🎟️', '💰', '🛍️', '🧺', '🪑',
  ];
  const paintPicks = () => {
    picks.querySelectorAll('.emoji-pick').forEach(b => b.classList.toggle('is-active', b.textContent === icon.value));
  };
  for (const e of usual) {
    const b = el('button', 'emoji-pick', e);
    b.type = 'button';
    b.addEventListener('click', () => {
      icon.value = e;
      paintPicks();
    });
    picks.append(b);
  }
  icon.addEventListener('input', paintPicks);
  paintPicks();
  const iconRow = el('div', 'emoji-row');
  iconRow.append(icon, picks);
  wrap.append(field('Headline', headline), field('Body', body), field('Emoji', iconRow));
  // fieldEditor focuses whatever it is handed.
  wrap.focus = () => headline.focus();
  return {
    wrap,
    clear: () => {
      headline.value = '';
      body.value = '';
      icon.value = '';
      paintPicks();
    },
    value: () => {
      if (!headline.value.trim() && !body.value.trim()) {
        return null;
      }
      return {headline: headline.value.trim(), body: body.value.trim(), icon: icon.value.trim()};
    },
  };
}

// highlightFields is the Basics tab's callout editor. It starts as one button,
// Add Highlight Section, and opens into the fields with a way to take the
// section off again.
function highlightFields(current) {
  const wrap = el('div', 'highlight-fields');
  const inputs = highlightInputs(current);
  const add = button('Add Highlight Section', 'plus', 'button button-secondary button-small', () => {
    section.hidden = false;
    add.hidden = true;
    inputs.wrap.focus();
  });
  const section = el('div', 'setting-card highlight-editor');
  section.hidden = !current;
  add.hidden = Boolean(current);
  const remove = button('Remove highlight', 'trash', 'button button-secondary button-small', () => {
    inputs.clear();
    section.hidden = true;
    add.hidden = false;
  });
  section.append(el('div', 'setting-label', 'Highlight section'),
    el('div', 'setting-hint', 'A callout under the description for the one thing people must read.'),
    inputs.wrap, remove);
  wrap.append(add, section);
  return {
    wrap,
    value: () => (section.hidden ? null : inputs.value()),
  };
}

// whenFields is the When tab: the shared when editor (dom.js) in a form field,
// offered the parent's timing to follow when the thing sits under one.
function whenFields(act, parent) {
  const own = act ? act.own : {start: '', end: '', timing: ''};
  const when = whenEditor(own.start, own.end, own.timing, parent, 'rows');
  when.wrap.classList.add('field');
  return when;
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
  // Where it can move to is one level: a thing under an event may move to any
  // of that event's siblings, and an event may move under any other event of
  // the year. Never itself or anything under it - the server refuses the loop.
  const yearOf = act ? act.year : (opts.parent ? opts.parent.year : year.value);
  const roots = state.model.activities.filter(a => a.year === yearOf);
  const currentParent = under ? activity(under) : null;
  const grandparent = currentParent ? parentOf(currentParent) : null;
  const level = currentParent ? (grandparent ? grandparent.children : roots) : roots;
  const mine = new Set(act ? [act.id, ...descendants(act).map(d => d.id)] : []);
  const parents = [{label: 'Nothing - this stands on its own', value: ''}];
  for (const n of level) {
    if (!mine.has(n.id)) {
      parents.push({label: n.title, value: n.id});
    }
  }
  const parentSelect = select(parents, under);
  // An event's Under select stays out of the way behind a small link until it
  // is wanted; a thing already under something shows it outright.
  const moveLink = el('button', 'link-button move-under', 'Move under an event…');
  moveLink.type = 'button';
  moveLink.addEventListener('click', () => {
    moveLink.hidden = true;
    underField.hidden = false;
    parentSelect.focus();
  });
  const underField = field('Parent Event', parentSelect, 'What this is part of, if anything');
  // The category select follows where the row sits: a root picks one of the
  // page's headings, a child picks one of its root event's own, or none. Someone
  // proposing rather than running the event only sees categories that allow it.
  const editor = admin || (root ? root.canEdit : false);
  // Uncategorized is only for editors: it is where things land without a
  // heading, not a heading people propose into.
  const headings = headingChoices(c => editor || canAdd(c)).filter(c => editor || c.value !== UNCATEGORIZED);
  const category = select(headings,
    act ? act.category : (opts.category || (headings[0] ? headings[0].value : '')));
  const own = root ? eventCategories(root).filter(c => editor || canAdd(c)) : [];
  const eventCategory = select([{label: 'None', value: ''}, ...own.map(c => ({label: c.title, value: c.id}))],
    act ? act.category : (opts.category || ''));
  const status = select(['Pending', 'Open', 'Done', 'Hidden'], act ? act.status : 'Open');
  // A coloured dot beside a policy select: green for yes, yellow for approval,
  // red for no, grey for "same as the parent". `blank` says what a blank means.
  const policyDot = (sel, blank) => {
    const wrap = el('div', 'status-select policy-select');
    const paint = () => {
      wrap.dataset.policy = sel.value || blank;
    };
    sel.addEventListener('change', paint);
    paint();
    wrap.append(sel);
    return wrap;
  };
  // A coloured dot beside the status, the way the mockup reads it at a glance.
  const dotted = sel => {
    const wrap = el('div', 'status-select');
    wrap.dataset.status = sel.value;
    sel.addEventListener('change', () => {
      wrap.dataset.status = sel.value;
    });
    wrap.append(sel);
    return wrap;
  };
  const description = textarea(act ? act.description : '', 6);
  const when = whenFields(act, act ? parentOf(act) : opts.parent || null);
  const timing = text(act ? act.own.timing : '', {placeholder: 'All Year, Late February, A few times per year'});
  const spots = text(act && act.spots ? String(act.spots) : '', {type: 'number'});
  spots.min = '1';
  spots.max = '99';
  // Unlimited by default; switching that off shows a small box for the number,
  // with its unit beside it.
  const unlimited = checkbox('Unlimited spots', !(act && act.spots), 'Allow unlimited volunteers to sign up.');
  const spotsRow = el('div', 'field-unit');
  spotsRow.append(spots, el('span', 'field-unit-label', 'people'));
  const spotsField = settingRow('Volunteer spots', 'How many volunteers can sign up for this activity?', el('div', 'setting-stack'));
  spotsField.querySelector('.setting-stack').append(spotsRow, unlimited.wrap);
  unlimited.wrap.classList.add('is-compact');
  const paintSpots = () => {
    spotsRow.hidden = unlimited.input.checked;
    if (!unlimited.input.checked && !spots.value) {
      spots.value = act && act.spots ? String(act.spots) : '10';
    }
  };
  unlimited.input.addEventListener('change', () => {
    paintSpots();
    if (!unlimited.input.checked) {
      spots.focus();
    }
  });
  paintSpots();
  // A new thing starts wanting a co-leader - most do, and the sheet's own
  // default for a blank cell says the same.
  const coLeader = checkbox('Co-leader needed', act ? act.coLeaderNeeded : true,
    'This lets people offer to be a co-leader; you still confirm them as co-leaders.');
  // A new thing under a parent starts with the parent's privacy - a private
  // event's committees are usually private too - and can be switched after.
  const hidden = checkbox('Keep volunteers secret',
    act ? act.volunteersHidden : Boolean(opts.parent && opts.parent.volunteersHidden),
    'E.g., hide room parent applications, which are secret.');
  const direct = checkbox(under ? 'Allow volunteers for this itself' : 'Allow volunteers for the event itself', act ? act.directSignUp : true,
    under ? 'Unchecking this will allow volunteers for subcommittees, but not this itself.' : 'Unchecking this will allow volunteers for subcommittees, but not the event itself.');
  const coChair = checkbox("I'd be open to co-chairing this", false);
  // What people who do not run this may add under it - and the default for
  // the event's own categories. Blank takes the parent's; an event's blank is No.
  const inheritLabel = under ? 'Same as the parent' : 'No, unless a category says otherwise';
  // An event's blank already means No, so it needs no No of its own; a
  // committee under a permissive parent does.
  const allowAdding = select([
    {label: inheritLabel, value: ''},
    {label: 'Yes - people can add, and it goes live', value: ADDING.yes},
    {label: 'Approval needed - people can add, an admin approves', value: ADDING.approval},
    ...(under ? [{label: 'No - only organizers add here', value: ADDING.no}] : []),
  ], act ? act.allowAddingOwn || '' : '');
  const allowAddingRow = settingRow('Allow adding subactivities',
    'Can users add subactivities? Note that this is a default and can be overwritten by the settings of a category.',
    policyDot(allowAdding, under ? 'inherit' : ADDING.no));
  // The search opens on the title, which is usually what the picture is of.
  const image = imagePicker(act ? act.image : '', act ? act.imageUrl : '', {query: () => title.value.trim()});
  const flyer = imagePicker(act ? act.flyer : '', act ? act.flyerUrl : '', {plain: true});
  const pretty = text(act ? act.prettyId || '' : '', {placeholder: 'applause', maxLength: 40});
  // The address as it will read, kept current as the field is typed in, with a
  // way to copy it - what the form is for, in the end.
  const addressBase = () => `${location.origin}${under ? activityPath(root) + '/' : '/v/'}`;
  const addressLine = el('div', 'address-line');
  const addressText = el('code', 'address-text');
  const addressCopy = button('Copy', 'copy', 'button button-secondary button-small', () => {
    navigator.clipboard.writeText(addressText.textContent).then(() => toast('Address copied'), () => toast('Could not copy'));
  });
  const paintAddress = () => {
    const slug = pretty.value.trim().toLowerCase();
    addressText.textContent = addressBase() + (slug || (act ? act.id : '…'));
    addressCopy.disabled = !act && !slug;
  };
  pretty.addEventListener('input', paintAddress);
  paintAddress();
  addressLine.append(addressText, addressCopy);
  const fields = [field('Title', title)];
  // Only an event has a year of its own; everything under it lives in the event's.
  const yearField = (admin || !act) && !under ? settingRow('School year', 'The year this event belongs to.', year) : null;
  if (suggesting) {
    fields.push(field('Year', year));
  }
  if (under) {
    fields.push(underField);
  }
  if (under) {
    if (own.length) {
      fields.push(field('Category', eventCategory, `One of ${root.title}'s own categories`));
    }
  } else {
    fields.push(field('Category', category));
  }
  if (!under && !suggesting && parents.length > 1) {
    // The link takes the slot beside Category, where Under would have been.
    underField.hidden = true;
    const slot = el('div', 'field field-link-slot');
    slot.append(moveLink);
    fields.push(slot, underField);
  }
  let statusSelect = null;
  if (admin && act) {
    statusSelect = status;
  } else if (act && act.status !== 'Pending') {
    statusSelect = select(['Open', 'Done'], act.status === 'Done' ? 'Done' : 'Open');
  }
  // Status lives on the Sign-ups tab as a settings row; the short Suggest form
  // never shows it (a suggestion is pending until approved).
  const statusRow = statusSelect ? settingRow('Status', 'Control whether this activity is open for sign-ups.', dotted(statusSelect)) : null;
  fields.push(field('Description', description));
  const highlight = highlightFields(act ? act.highlight : null);
  let body;
  if (!suggesting) {
    // The full form is long, so it is a wide modal with the fields sorted into
    // tabs: what it is, when and where, who may sign up, how it looks and
    // where it lives. The short selects pair up across the width; the title
    // and description keep the whole line. A suggestion stays one column.
    // Basics as settings rows, like the other tabs: the title, what it is
    // part of and where it is listed, then the description across the width.
    const basics = [settingRow('Title', 'What this is called.', title)];
    if (under) {
      basics.push(settingRow('Parent Event', 'What this is part of.', parentSelect));
      if (own.length) {
        basics.push(settingRow('Category', `One of ${root.title}'s own categories.`, eventCategory));
      }
    } else {
      const where = el('div', 'setting-stack');
      where.append(category);
      if (parents.length > 1) {
        where.append(moveLink, underField);
        underField.classList.add('is-compact');
      }
      basics.push(settingRow('Category', 'Where this is listed on the Opportunities page.', where));
    }
    const about = settingRow('Description', 'What people should know before they sign up.', description);
    about.classList.add('is-stacked');
    basics.push(about, highlight.wrap);
    body = [tabbedFields([
      {label: 'Basics', icon: 'doc', fields: basics},
      {label: 'When', icon: 'calendar', fields: [...(yearField ? [yearField] : []), when.wrap]},
      {label: 'Sign-ups', icon: 'people', fields: [...(statusRow ? [statusRow] : []), spotsField, allowAddingRow, coLeader.wrap, hidden.wrap, direct.wrap]},
      {label: 'Image & Address', icon: 'image', fields: [
        settingCard('Top Banner Image', 'The wide picture across the top of the page and on the card (optional).', image.wrap.querySelector('.image-row')),
        settingCard('Flyer', 'The event\'s poster, shown beside the details and used for the social share image when there is one (optional).', flyer.wrap.querySelector('.image-row')),
        settingCard('Friendly address', under
          ? 'A short address for this activity, under its event. Letters, digits and hyphens; unique among the things beside it.'
          : 'A short address for this event. Letters, digits and hyphens; one address per event, across every year.',
        pretty, addressLine),
      ]},
    ])];
  } else {
    fields.push(field('Timing', timing, 'When would this happen?'), image.wrap, coChair.wrap);
    body = fields;
  }
  openModal(act ? 'Edit Activity' : (suggesting ? 'Suggest an Idea' : (opts.parent ? `Add under ${opts.parent.title}` : 'Add Activity')), body, {
    saveLabel: suggesting ? 'Suggest' : (act ? 'Save changes' : 'Add'),
    deleteLabel: 'Delete activity',
    wide: !suggesting,
    submit: async () => {
      const scheduled = suggesting ? {start: '', end: '', timing: timing.value} : when.value();
      if (!suggesting && when.validate(scheduled)) {
        throw new Error(when.validate(scheduled));
      }
      const body = {
        id: act ? act.id : '',
        year: year.value, title: title.value, parent: parentSelect.value,
        category: parentSelect.value ? eventCategory.value : category.value,
        status: statusSelect ? statusSelect.value : '',
        description: description.value, image: image.value(), flyer: flyer.value(), timing: scheduled.timing,
        highlight: highlight.value(),
        // Location is no longer asked for or shown; a value already in the sheet is kept.
        start: scheduled.start, end: scheduled.end, location: act ? act.location || '' : '', spots: unlimited.input.checked ? 0 : Number(spots.value) || 0,
        coLeaderNeeded: coLeader.input.checked, volunteersHidden: hidden.input.checked, directSignUp: direct.input.checked,
        coChair: coChair.input.checked, prettyId: pretty.value.trim().toLowerCase(), allowAdding: allowAdding.value,
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
    // A deleted thing under an event sends you back up to that event; a
    // deleted event, to the front page. The path is taken now, while the
    // parent is still in the model as this thing's parent.
    afterDelete: () => goTo(currentParent ? activityPath(currentParent) : '/'),
  });
}

export function openLink(node, item) {
  const title = text(item ? item.title : '', {required: true, maxLength: 120});
  const url = text(item ? item.url : '', {type: 'url', required: true, placeholder: 'https://'});
  const description = textarea(item ? item.description || '' : '', 2);
  const image = imagePicker(item ? item.image : '', item ? item.imageUrl : '');
  openModal(item ? 'Edit Resource' : 'Add Resource', [
    field('Title', title), field('URL', url),
    field('Description', description, 'A line about what people will find there'),
    image.wrap,
  ], {
    submit: () => send('POST', '/api/events/link', {
      id: node.id, original: item ? item.title : '',
      title: title.value, url: url.value, description: description.value, image: image.value(),
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
    if (e.key === 'Enter' && e.target.tagName !== 'TEXTAREA') {
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

// imageSearchOn says the server can search Google Images (it has a key).
export function imageSearchOn() {
  return Boolean(state.model && state.model.imageSearch);
}

// openImageSearch is the Google Images picker: a search box, a grid of
// results, and a click on one imports it through the server - which fetches
// and stores the picture like an upload - and hands the stored name to
// `onPicked`. SafeSearch is on server-side.
// imageSources is where the server can look, first first; imageSource the
// one the picker leads with.
export function imageSources() {
  return (state.model && state.model.imageSources) || ['Wikimedia Commons'];
}

export function imageSource() {
  return imageSources()[0];
}

const notes = {
  'Unsplash': 'Free to use under the Unsplash License; the photographer is credited on each tile.',
  'Pexels': 'Free to use under the Pexels License; the photographer is credited on each tile.',
  'Pixabay': 'Photos, illustrations and vectors, free to use under the Pixabay Content License.',
  'Wikimedia Commons': 'Everything here is free to use; the licence is on each tile, and CC BY ones ask to be credited.',
  'Google Images': 'Pick a picture you have the right to use - a school photo, a poster, a flag, a public-domain image.',
};

export function openImageSearch(initial, onPicked) {
  const wrap = el('div', 'image-search');
  const bar = el('div', 'image-search-bar');
  const input = el('input');
  input.type = 'search';
  input.value = initial || '';
  const go = button('Search', 'search', 'button', () => run());
  // With more than one place to look, a segmented switch picks between them.
  let source = imageSource();
  const sources = imageSources();
  const picker = sources.length > 1 ? segmented(sources.map(s => ({label: s, value: s})), source, v => {
    source = v;
    input.placeholder = `Search ${source}…`;
    note.textContent = notes[source] || '';
    run();
  }) : null;
  bar.append(input, go);
  const status = el('div', 'image-search-status');
  const grid = el('div', 'image-search-grid');
  const note = el('div', 'hint', notes[source] || '');
  input.placeholder = `Search ${source}…`;
  if (picker) {
    wrap.append(picker.wrap);
  }
  wrap.append(bar, status, grid, note);
  let busy = false;
  const run = async () => {
    const q = input.value.trim();
    if (!q || busy) {
      return;
    }
    busy = true;
    status.textContent = 'Searching…';
    grid.replaceChildren();
    try {
      const res = await fetch(`/api/events/images/search?q=${encodeURIComponent(q)}&source=${encodeURIComponent(source)}`);
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const hits = await res.json();
      status.textContent = hits.length ? '' : 'Nothing found.';
      for (const hit of hits) {
        const tile = el('button', 'image-search-hit');
        tile.type = 'button';
        const img = el('img');
        img.src = hit.thumb;
        img.alt = hit.title;
        img.loading = 'lazy';
        img.addEventListener('load', () => img.classList.add('is-loaded'));
        tile.append(img, el('span', 'image-search-source', hit.credit || (hit.license ? `${hit.license} · ${hit.source}` : hit.source)));
        tile.title = `${hit.title} - ${hit.width}×${hit.height}${hit.license ? ` - ${hit.license}` : ''}`;
        tile.addEventListener('click', async () => {
          if (busy) {
            return;
          }
          busy = true;
          status.textContent = 'Importing…';
          tile.classList.add('is-picked');
          try {
            const imported = await fetch('/api/events/images/import', {
              method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({url: hit.url, download: hit.download || ''}),
            });
            if (!imported.ok) {
              throw new Error(await imported.text());
            }
            const {name} = await imported.json();
            shut();
            await onPicked(name);
          } catch (err) {
            status.textContent = err.message;
            tile.classList.remove('is-picked');
          }
          busy = false;
        });
        grid.append(tile);
      }
    } catch (err) {
      status.textContent = err.message;
    }
    busy = false;
  };
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      run();
    }
  });
  // Its own layer, so a Find an image from inside the activity editor comes
  // back to the form as it was - not to an empty one.
  const shut = openSheet('Find an image', [wrap], {wide: true});
  input.focus();
  if (input.value) {
    run();
  }
}

// openCategory edits one category, or adds one to `eventId`'s event - or to the
// page's headings when eventId is empty.
export function openCategory(category, eventId, after) {
  const title = text(category ? category.title : '', {required: true, maxLength: 120});
  const description = textarea(category ? category.description : '', 3);
  description.placeholder = 'Add a brief description (optional).';
  const image = imagePicker(category ? category.image : '', category ? category.imageUrl : '',
    {dropzone: true, hint: 'Add an image to represent this category (optional).'});
  // What people may add here: live, after approval, or not at all - and for an
  // event's own category, the event's own setting unless said otherwise.
  const policies = [
    ...(eventId ? [{label: 'Same as the event', value: ''}] : []),
    {label: 'Yes - people can add, and it goes live', value: ADDING.yes},
    {label: 'Approval needed - people can add, an admin approves', value: ADDING.approval},
    {label: 'No - only organizers add here', value: ADDING.no},
  ];
  const adding = select(policies, category ? category.allowAddingOwn || '' : (eventId ? '' : ADDING.no));
  const addingWrap = el('div', 'status-select policy-select');
  const paintAdding = () => {
    addingWrap.dataset.policy = adding.value || 'inherit';
  };
  adding.addEventListener('change', paintAdding);
  paintAdding();
  addingWrap.append(adding);
  const titleField = field('Title', title, 'A short, clear name for this category.');
  titleField.classList.add('is-required');
  const addingField = field('Allow adding', addingWrap, 'Control whether people can add activities to this category.');
  addingField.classList.add('is-required');
  const fields = [titleField, field('Description', description), image.wrap, addingField];
  // Only a page heading can be kept off the main page; it still sits in the
  // rail with its count, and clicking it there shows its events.
  const onMain = eventId ? null : checkbox('Show on the main page (it stays in the toolbar either way)', category ? category.showOnMain : true);
  if (onMain) {
    fields.push(onMain.wrap);
  }
  openModal(category ? 'Edit Category' : 'Add Category', fields, {
    saveLabel: category ? 'Save changes' : 'Add',
    submit: () => send('POST', '/api/events/category', {
      id: category ? category.id : '', eventId: eventId || '',
      title: title.value, description: description.value, image: image.value(), allowAdding: adding.value,
      showOnMain: onMain ? onMain.input.checked : true,
    }),
    afterSave: after,
    onDelete: category ? () => send('DELETE', '/api/events/category', {id: category.id}) : null,
    confirmDelete: category ? `Delete the category “${category.title}”?` : '',
    afterDelete: after,
  });
}

// addingWords is a category's policy in a phrase for the manager's list.
function addingWords(category) {
  const own = category.allowAddingOwn || '';
  const word = {[ADDING.yes]: 'People can add here', [ADDING.approval]: 'People can suggest here', [ADDING.no]: 'Only organizers add here'}[category.allowAdding] || '';
  return own ? word : (category.eventId ? `${word} (same as the event)` : word);
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
    body.append(el('div', 'sub', [category.description, addingWords(category),
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
    // The sheet's own dates, not the ones inherited for display (state.js).
    description: act.description || '', image: act.image || '', flyer: act.flyer || '', timing: act.own.timing,
    start: act.own.start, end: act.own.end, location: act.location || '', spots: act.spots || 0,
    coLeaderNeeded: act.coLeaderNeeded, volunteersHidden: act.volunteersHidden, directSignUp: act.directSignUp,
    prettyId: act.prettyId || '', allowAdding: act.allowAddingOwn || '', highlight: act.highlight || null,
    ...changes,
  };
  try {
    await saveActivity(body);
  } catch (err) {
    toast(err.message);
    await reload();
  }
}

