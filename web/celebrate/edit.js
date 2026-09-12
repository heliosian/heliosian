import {state, me, isAdmin, household, billable, admits, audienceWords, ticketFor, money, currentCelebration, partyPath, party} from './state.js';
import {el, svg, toast, button, avatar} from './dom.js';
import {openCropTool} from '/crop.js';

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
  const type = res.headers.get('Content-Type') || '';
  return type.includes('json') ? res.json() : null;
}

async function goTo(path) {
  const {navigate} = await import('./app.js');
  navigate(path);
}

export async function uploadImage(file) {
  const body = new FormData();
  body.append('image', file);
  const res = await fetch('/api/celebrate/image', {method: 'POST', body});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return (await res.json()).name;
}

// openSheet is a second layer above the modal for something the form needs
// mid-edit - the image search - so the form underneath keeps every field
// typed into it.
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

export function closeModal() {
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
    if (!modalState || !modalState.submit) {
      return;
    }
    const {submit, afterSave} = modalState;
    setStatus('Saving…');
    try {
      const result = await submit();
      closeModal();
      await reload();
      if (afterSave) {
        afterSave(result);
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

function field(label, input, hint, required) {
  const wrap = el('label', 'field' + (required ? ' is-required' : ''));
  wrap.append(typeof label === 'string' ? el('span', '', label) : label, input);
  if (hint) {
    wrap.append(el('small', '', hint));
  }
  return wrap;
}

function text(value, options) {
  const input = el('input');
  input.type = (options && options.type) || 'text';
  input.value = value === undefined || value === null ? '' : String(value);
  if (options && options.placeholder) {
    input.placeholder = options.placeholder;
  }
  if (options && options.required) {
    input.required = true;
  }
  if (options && options.maxLength) {
    input.maxLength = options.maxLength;
  }
  if (options && options.min !== undefined) {
    input.min = options.min;
  }
  if (options && options.step !== undefined) {
    input.step = options.step;
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

// segmented is a two-or-more-way switch: one button per choice, the chosen one
// filled.
function segmented(options, initial, onChange) {
  const wrap = el('div', 'segmented');
  wrap.setAttribute('role', 'radiogroup');
  const s = {value: initial};
  const buttons = options.map(o => {
    const b = el('button', 'segment' + (o.value === initial ? ' is-on' : ''), o.label);
    b.type = 'button';
    b.setAttribute('role', 'radio');
    b.setAttribute('aria-checked', String(o.value === initial));
    b.addEventListener('click', () => {
      s.value = o.value;
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
  return {wrap, get value() { return s.value; }};
}

// checkbox is a labelled switch with an optional line of explanation.
export function checkbox(label, checked, hint) {
  const wrap = el('label', 'field field-toggle');
  const input = el('input');
  input.type = 'checkbox';
  input.checked = Boolean(checked);
  const words = el('span');
  words.append(el('span', '', label));
  if (hint) {
    words.append(el('small', '', hint));
  }
  const knob = el('span', 'switch');
  knob.append(input, el('span'));
  wrap.append(knob, words);
  return {wrap, input};
}

// whenPickers is a date beside an optional time. The sheet stores
// "YYYY-MM-DD" or "YYYY-MM-DD HH:MM"; a blank time is how an all-day thing is
// written, so the two stay separate controls.
function whenPickers(label, value) {
  const m = /^(\d{4}-\d{2}-\d{2})(?: (\d{2}:\d{2}))?$/.exec(value || '');
  const wrap = el('div', 'field-when-block');
  wrap.append(el('span', 'field-when-label', label));
  const pair = el('div', 'field-when-pair');
  const date = el('input');
  date.type = 'date';
  date.value = m ? m[1] : '';
  const time = el('input');
  time.type = 'time';
  time.value = m && m[2] ? m[2] : '';
  pair.append(date, time);
  wrap.append(pair);
  return {wrap, date, time, value: () => (date.value ? (time.value ? `${date.value} ${time.value}` : date.value) : '')};
}

// imagePicker uploads on selection, so the save that follows only records the
// name the server handed back; Find an image searches the libraries instead.
function imagePicker(current, currentUrl, options) {
  const wrap = el('div', 'field');
  wrap.append(el('span', '', (options && options.label) || 'Image'));
  if (options && options.hint) {
    wrap.append(el('small', 'field-lead', options.hint));
  }
  const row = el('div', 'image-drop');
  const preview = el('img');
  preview.alt = '';
  const placeholder = el('div', 'image-placeholder');
  placeholder.append(svg('image'), el('strong', '', 'Drag and drop an image here'), el('small', '', 'or click to choose a file'));
  const choose = el('label', 'button button-secondary button-small', 'Choose image');
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  choose.append(file);
  const remove = el('button', 'link-button', 'Remove');
  remove.type = 'button';
  const crop = el('button', 'link-button', 'Crop');
  crop.type = 'button';
  // A plain picker takes an upload whole - a finished poster is never
  // cropped or found in a library.
  const plain = Boolean(options && options.plain);
  let name = current || '';
  const show = url => {
    preview.hidden = !url;
    placeholder.hidden = Boolean(url);
    remove.hidden = !url;
    crop.hidden = !url || plain;
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
  const find = el('button', 'button button-secondary button-small image-find', 'Find an image');
  find.type = 'button';
  find.hidden = plain || !imageSearchOn();
  find.addEventListener('click', e => {
    e.stopPropagation();
    openImageSearch(options && options.query ? options.query() : '', async picked => {
      name = picked;
      show('/' + picked);
    });
  });
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
  buttons.append(choose, find);
  row.append(preview, placeholder, buttons, el('small', 'image-drop-note', 'JPG, PNG or GIF (max 8 MB)'), crop, remove);
  wrap.append(row);
  return {wrap, value: () => name};
}

export function imageSearchOn() {
  return Boolean(state.model && state.model.imageSearch);
}

function imageSources() {
  return (state.model && state.model.imageSources) || ['Wikimedia Commons'];
}

const sourceNotes = {
  'Unsplash': 'Free to use under the Unsplash License; the photographer is credited on each tile.',
  'Pexels': 'Free to use under the Pexels License; the photographer is credited on each tile.',
  'Pixabay': 'Photos, illustrations and vectors, free to use under the Pixabay Content License.',
  'Wikimedia Commons': 'Everything here is free to use; the licence is on each tile, and CC BY ones ask to be credited.',
  'Google Images': 'Pick a picture you have the right to use - a school photo, a poster, a public-domain image.',
};

// openImageSearch is the picture picker: a search box, a grid of results, and
// a click on one imports it through the server - which fetches and stores the
// picture like an upload - and hands the stored name to onPicked.
export function openImageSearch(initial, onPicked) {
  const wrap = el('div', 'image-search');
  const bar = el('div', 'image-search-bar');
  const input = el('input');
  input.type = 'search';
  input.value = initial || '';
  const go = button('Search', 'search', 'button', () => run());
  let source = imageSources()[0];
  const sources = imageSources();
  const picker = sources.length > 1 ? segmented(sources.map(s => ({label: s, value: s})), source, v => {
    source = v;
    input.placeholder = `Search ${source}…`;
    note.textContent = sourceNotes[source] || '';
    run();
  }) : null;
  bar.append(input, go);
  const status = el('div', 'image-search-status');
  const grid = el('div', 'image-search-grid');
  const note = el('div', 'hint', sourceNotes[source] || '');
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
      const res = await fetch(`/api/celebrate/images/search?q=${encodeURIComponent(q)}&source=${encodeURIComponent(source)}`);
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
            const imported = await fetch('/api/celebrate/images/import', {
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
  const shut = openSheet('Find an image', [wrap], {wide: true});
  input.focus();
  if (input.value) {
    run();
  }
}

// tabbedFields lays a long form out as tabs. Every field stays in the form -
// only the panels hide - so values and validation survive switching.
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
  const first = f.querySelector('input:not([type=hidden]):not([type=file]):not([type=checkbox]), textarea, select');
  if (first) {
    first.focus();
  }
}

// The directory, fetched once per page load, behind the people pickers.
let peopleCache = null;

async function people() {
  if (!peopleCache) {
    const res = await fetch('/api/celebrate/people');
    peopleCache = res.ok ? await res.json() : [];
  }
  return peopleCache;
}

// peoplePicker is the searchable directory list: type a few letters, see
// faces, names and what places each person, pick one. Typing a full address
// that matches nobody still works.
export function peoplePicker(placeholder) {
  const wrap = el('div', 'people-picker');
  const search = el('input');
  search.type = 'search';
  search.placeholder = placeholder || 'Search by name…';
  search.autocomplete = 'off';
  const results = el('div', 'people-results');
  results.hidden = true;
  const chosen = el('div', 'people-chosen');
  chosen.hidden = true;
  wrap.append(search, results, chosen);
  let value = '';
  let picked = null;
  const tile = person => {
    const row = el('button', 'people-row');
    row.type = 'button';
    row.append(avatar(person, 'people-face'));
    const words = el('div', 'people-text');
    words.append(el('div', 'people-name', person.name));
    if (person.title) {
      words.append(el('div', 'people-title', person.title));
    }
    row.append(words);
    return row;
  };
  const choose = person => {
    value = person.email;
    picked = person;
    chosen.replaceChildren();
    const row = tile(person);
    row.disabled = true;
    const clear = el('button', 'link-button', 'Change');
    clear.type = 'button';
    clear.addEventListener('click', () => {
      value = '';
      picked = null;
      chosen.hidden = true;
      search.hidden = false;
      search.value = '';
      search.focus();
    });
    chosen.append(row, clear);
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
      results.append(el('div', 'people-none', q.includes('@') ? `Nobody in the directory - “${q}” will be used as typed.` : 'Nobody matches.'));
    }
    results.hidden = false;
  };
  search.addEventListener('input', show);
  search.addEventListener('focus', show);
  return {
    wrap,
    value: () => value || (search.value.includes('@') ? search.value.trim().toLowerCase() : ''),
    person: () => picked,
    reset: () => {
      value = '';
      picked = null;
      chosen.hidden = true;
      search.hidden = false;
      search.value = '';
    },
  };
}

// personInfo is the directory's row for an address, or null for someone it
// does not know - a guest, or a family not yet imported.
export async function personInfo(email) {
  const all = await people();
  return all.find(p => p.email === email) || null;
}

// whoProfile is a person's page on Helios Who?, on this tier: celebrate.x
// pairs with who.x, and the page is named by the address's local part.
export function whoProfile(email) {
  const host = location.host.replace(/^celebrate\./, 'who.');
  return `${location.protocol}//${host}/people/${encodeURIComponent((email || '').split('@')[0])}`;
}

// openPerson is the little window that opens from a face - a host's, or
// someone who is coming - as HCA-Team opens one from a volunteer: the big
// face, the name with pronouns and what places them, round buttons to email,
// text, call or copy them, the contact rows and the household, and the way
// through to their Helios Who? page.
export async function openPerson(v) {
  const info = v.email ? await personInfo(v.email) : null;
  const done = el('button', 'button button-secondary', 'Done');
  done.type = 'button';
  done.addEventListener('click', closeModal);
  openModal('', [personHead(v, info), personContact(v, info), personFoot(v, info, done)], {actions: false, wide: 'person'});
}

function personHead(v, info) {
  const head = el('div', 'who-head');
  const face = avatar({name: (info && info.name) || v.name, email: v.email, photoUrl: (info && info.photoUrl) || v.photoUrl}, 'who-face');
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
  } else if (!v.email) {
    names.append(el('div', 'who-sub', 'Guest'));
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
  if (v.email) {
    action('mail', 'Email', `mailto:${v.email}`);
  }
  const phone = info && info.phone ? info.phone : '';
  const digits = phone.replace(/[^+\d]/g, '');
  if (digits) {
    action('chat', 'Message', `sms:${digits}`);
    action('phone', 'Call', `tel:${digits}`);
  }
  if (v.email) {
    action('copy', 'Copy info', null, () => {
      const lines = [(info && info.name) || v.name || '', v.email, phone].filter(Boolean);
      navigator.clipboard.writeText(lines.join('\n')).then(() => toast('Contact info copied'), () => toast('Could not copy'));
    });
  }
  head.append(face, names, actions);
  return head;
}

// personContact is the card's rows: how to reach them, then the household -
// each name a chip that opens that person's own window.
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
  if (v.email) {
    row('mail', 'Email', v.email);
  }
  if (info && info.phone) {
    row('phone', 'Phone', info.phone);
  }
  const chips = list => {
    const wrap = el('div', 'who-card-chips');
    for (const p of list) {
      const chip = el('button', 'who-card-chip', p.grade ? `${p.name} (${p.grade})` : p.name);
      chip.type = 'button';
      chip.addEventListener('click', () => openPerson({email: p.email, name: p.name, photoUrl: p.photoUrl}));
      wrap.append(chip);
    }
    return wrap;
  };
  const household = info && ((info.spouses && info.spouses.length) || (info.children && info.children.length) || (info.isStudent && info.parentEmails && info.parentEmails.length));
  if (household) {
    group = null;
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
  if (!card.children.length) {
    row('person', 'About', 'A guest, not in the directory.');
  }
  return card;
}

// personFoot is the band under the card: the way through to their Helios
// Who? page for someone the directory knows, and Done.
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

// personChip is one member of the household as a pick: face, name, a line.
function personChip(person, on, disabledWhy) {
  const chip = el('button', 'person-chip' + (on ? ' is-on' : ''));
  chip.type = 'button';
  chip.append(avatar(person, 'person-chip-face'));
  const words = el('span', 'person-chip-text');
  words.append(el('span', 'person-chip-name', person.name));
  if (person.grade || person.title) {
    words.append(el('span', 'person-chip-line', person.grade || person.title));
  }
  chip.append(words);
  if (disabledWhy) {
    chip.disabled = true;
    chip.title = disabledWhy;
  }
  return chip;
}

// openBuy is the ticket form: who to bill, who is coming - the household by
// face, guests by name - and a note. Whoever runs the party gets a directory
// picker instead, to add anyone and bill anyone.
export function openBuy(p) {
  const editor = p.canEdit;
  const fields = [];
  const chosen = new Set();
  let purchaser = '';
  const bills = billable();
  const guests = [];

  // Who should we bill?
  const billRow = el('div', 'chip-pick');
  const billField = field('Who should we bill?', billRow, editor ? 'Tickets are invoiced to this person.' : 'Tickets are invoiced to an adult in your family.', true);
  const billPicker = editor ? peoplePicker('Search for the person to bill…') : null;
  const paintBill = () => {
    billRow.replaceChildren();
    for (const b of bills) {
      const chip = personChip(b, purchaser === b.email);
      chip.addEventListener('click', () => {
        purchaser = b.email;
        paintBill();
      });
      billRow.append(chip);
    }
    if (billPicker) {
      billRow.append(billPicker.wrap);
    }
  };
  if (bills.length) {
    purchaser = bills[0].email;
  }
  paintBill();
  fields.push(billField);

  // Who's coming?
  const who = el('div', 'chip-pick');
  const whoField = field("Who's coming?", who, `This party is for ${audienceWords(p)}.`, true);
  const paintWho = () => {
    who.replaceChildren();
    for (const person of household()) {
      let why = '';
      const have = ticketFor(p, person.email);
      if (have) {
        why = have.status === 'Ticket' ? `${person.name} already has a ticket` : `${person.name} is already on the waitlist`;
      } else if (!admits(p, person)) {
        why = `This party is for ${audienceWords(p)}`;
      }
      const chip = personChip(person, chosen.has(person.email), why);
      chip.addEventListener('click', () => {
        if (chosen.has(person.email)) {
          chosen.delete(person.email);
        } else {
          chosen.add(person.email);
        }
        paintWho();
        paintTotal();
      });
      who.append(chip);
    }
  };
  paintWho();
  fields.push(whoField);

  // Whoever runs the party adds anyone in the directory.
  let anyone = null;
  const added = [];
  if (editor) {
    anyone = peoplePicker('Add someone from the directory…');
    const addedList = el('div', 'chip-pick');
    const addRow = el('div', 'guest-add');
    const addButton = button('Add', 'plus', 'button button-secondary button-small', () => {
      const email = anyone.value();
      if (!email || added.some(a => a.email === email) || chosen.has(email)) {
        return;
      }
      added.push({email, name: (anyone.person() || {}).name || email, photoUrl: (anyone.person() || {}).photoUrl});
      anyone.reset();
      paintAdded();
      paintTotal();
    });
    const paintAdded = () => {
      addedList.replaceChildren();
      for (const a of added) {
        const chip = personChip(a, true);
        chip.addEventListener('click', () => {
          added.splice(added.indexOf(a), 1);
          paintAdded();
          paintTotal();
        });
        addedList.append(chip);
      }
    };
    addRow.append(anyone.wrap, addButton);
    const wrap = el('div');
    wrap.append(addRow, addedList);
    fields.push(field('Anyone else', wrap, 'Hosts and admins can add anyone in the directory.'));
  }

  // Guests by name.
  const guestList = el('div', 'guest-list');
  const paintGuests = () => {
    guestList.replaceChildren();
    guests.forEach((g, i) => {
      const row = el('div', 'guest-row');
      const input = text(g.name, {placeholder: 'Full name, e.g. Zander Faulkner (sibling of Everly, age 8)', maxLength: 120});
      input.addEventListener('input', () => {
        g.name = input.value;
      });
      const remove = button('', 'close', 'edit-icon', () => {
        guests.splice(i, 1);
        paintGuests();
        paintTotal();
      });
      remove.title = 'Remove this guest';
      row.append(input, remove);
      guestList.append(row);
      if (i === guests.length - 1 && !g.name) {
        setTimeout(() => input.focus(), 0);
      }
    });
  };
  const addGuest = button('Add a guest', 'plus', 'button button-secondary button-small', () => {
    guests.push({name: ''});
    paintGuests();
    paintTotal();
  });
  const guestWrap = el('div');
  guestWrap.append(guestList, addGuest);
  fields.push(field('Guests', guestWrap, "Someone who isn't in the directory - a visiting cousin, a non-Helios sibling, a friend. Enter their full name."));

  const note = textarea('', 2);
  note.placeholder = 'Anything the hosts should know (optional)';
  fields.push(field('Note', note));

  // The tally: tickets × price, and whether they will be sold or waitlisted.
  const total = el('div', 'buy-total');
  const paintTotal = () => {
    const n = chosen.size + added.length + guests.filter(g => g.name.trim()).length;
    total.replaceChildren();
    if (!n) {
      total.append(el('span', 'buy-total-hint', 'Pick at least one person.'));
      return;
    }
    const line = el('span', 'buy-total-sum', `${n} ${n === 1 ? 'ticket' : 'tickets'} × ${money(p.price)} = ${money(n * p.price)}`);
    total.append(line);
    if (!editor && p.remaining >= 0 && n > p.remaining) {
      const over = n - p.remaining;
      total.append(el('span', 'buy-total-hint', p.remaining
        ? `Only ${p.remaining} left: ${over} of these will go on the waitlist.`
        : 'This party is full: these will go on the waitlist.'));
    }
  };
  paintTotal();
  fields.push(total);
  if (state.model.settings.ticketNote) {
    fields.push(el('p', 'buy-note', state.model.settings.ticketNote));
  }

  openModal(p.availability === 'waitlist' && !editor ? `Join the waitlist for ${p.title}` : `Tickets for ${p.title}`, fields, {
    saveLabel: p.availability === 'waitlist' && !editor ? 'Join Waitlist' : 'Get Tickets',
    submit: async () => {
      const bill = editor && billPicker && billPicker.value() ? billPicker.value() : purchaser;
      if (!bill) {
        throw new Error('Pick who to bill.');
      }
      const attendees = [...chosen].map(email => ({email}))
        .concat(added.map(a => ({email: a.email})))
        .concat(guests.filter(g => g.name.trim()).map(g => ({name: g.name.trim()})));
      if (!attendees.length) {
        throw new Error('Pick at least one person.');
      }
      return send('POST', '/api/celebrate/tickets', {partyId: p.id, purchaser: bill, note: note.value, attendees});
    },
    afterSave: result => {
      const parts = [];
      if (result.sold) {
        parts.push(`${result.sold} ${result.sold === 1 ? 'ticket' : 'tickets'} taken`);
      }
      if (result.waitlisted) {
        parts.push(`${result.waitlisted} on the waitlist`);
      }
      toast(parts.join(', ') || 'Done');
    },
  });
}

// removeTicket takes a ticket off a party (a host) or a name off the
// waitlist (the family too), after a word of confirmation.
export async function removeTicket(p, a) {
  const what = a.status === 'Ticket' ? `Remove ${a.name}'s ticket to ${p.title}?` : `Take ${a.name} off the waitlist for ${p.title}?`;
  if (!confirm(what)) {
    return;
  }
  try {
    await send('DELETE', '/api/celebrate/ticket', {ticketId: a.ticketId});
    await reload();
    toast(a.status === 'Ticket' ? 'Ticket removed' : 'Off the waitlist');
  } catch (err) {
    toast(err.message);
  }
}

// setTicketStatus moves a ticket on or off the waitlist - a host's offer.
export async function setTicketStatus(a, status) {
  try {
    await send('POST', '/api/celebrate/ticket', {ticketId: a.ticketId, status});
    await reload();
    toast(status === 'Ticket' ? `${a.name} now has a ticket` : `${a.name} is on the waitlist`);
  } catch (err) {
    toast(err.message);
  }
}

// openTicket is a host's window on someone who is coming, as HCA-Team opens
// a volunteer's for whoever runs the event: the person's head, then two tabs
// - Contact, the same card everyone else gets, and Ticket, who bought it, the
// note, the waitlist offer, and for an admin where the invoice stands.
export async function openTicket(p, a) {
  const info = a.email ? await personInfo(a.email) : null;
  const form = ticketForm(p, a);
  const tabs = tabbedFields([
    {label: 'Contact', icon: 'people', fields: [personContact(a, info), personFoot(a, info, null)]},
    {label: 'Ticket', icon: 'ticket', fields: form.fields},
  ]);
  openModal('', [personHead(a, info), tabs], {...form, wide: 'person'});
}

// ticketForm is the Ticket tab's fields and the modal options that save them.
function ticketForm(p, a) {
  const fields = [];
  const facts = el('div', 'ticket-facts');
  const fact = (label, value) => {
    if (!value) {
      return;
    }
    const row = el('div', 'ticket-fact');
    row.append(el('span', 'ticket-fact-label', label), el('span', '', value));
    facts.append(row);
  };
  fact('Party', p.title);
  fact('Billed to', a.purchaserName ? `${a.purchaserName} (${a.purchaser})` : a.purchaser);
  fact('Price', money(a.price || 0));
  fact('Taken', a.added);
  fact('Added by', a.addedBy);
  fields.push(facts);
  const note = textarea(a.note || '', 2);
  fields.push(field('Note', note));
  const status = select([{label: 'Ticket', value: 'Ticket'}, {label: 'Waitlist', value: 'Waitlist'}], a.status);
  fields.push(field('Status', status, 'Move a waitlisted person onto a ticket when a place opens up.'));
  let invoice = null;
  if (isAdmin()) {
    invoice = select([{label: 'Not yet', value: ''}, {label: 'Sent', value: 'Sent'}, {label: 'Paid', value: 'Paid'}], a.invoice || '');
    fields.push(field('Invoice', invoice));
  }
  return {
    fields,
    submit: () => {
      const body = {ticketId: a.ticketId, note: note.value, status: status.value};
      if (invoice) {
        body.invoice = invoice.value;
      }
      return send('POST', '/api/celebrate/ticket', body);
    },
    onDelete: () => send('DELETE', '/api/celebrate/ticket', {ticketId: a.ticketId}),
    deleteLabel: 'Remove',
    confirmDelete: `Remove ${a.name} from ${p.title}?`,
  };
}

// hostChips is the list of a party's hosts by address, each removable, with a
// picker to add another from the directory.
function hostChips(initial) {
  const hosts = initial.map(h => ({...h}));
  const wrap = el('div');
  const list = el('div', 'chip-pick');
  const picker = peoplePicker('Add a host from the directory…');
  const paint = () => {
    list.replaceChildren();
    for (const h of hosts) {
      const chip = personChip(h, true);
      chip.title = 'Remove';
      chip.addEventListener('click', () => {
        hosts.splice(hosts.indexOf(h), 1);
        paint();
      });
      list.append(chip);
    }
  };
  const add = button('Add', 'plus', 'button button-secondary button-small', () => {
    const email = picker.value();
    if (!email || hosts.some(h => h.email === email)) {
      return;
    }
    const person = picker.person() || {};
    hosts.push({email, name: person.name || email, photoUrl: person.photoUrl});
    picker.reset();
    paint();
  });
  const row = el('div', 'guest-add');
  row.append(picker.wrap, add);
  paint();
  list.classList.add('chip-pick-roomy');
  wrap.append(list, row);
  return {wrap, value: () => hosts.map(h => h.email)};
}

// openParty adds a party or edits one. A new one lands as Pending for an
// admin; a host's edits keep the status.
export function openParty(p) {
  const adding = !p;
  const user = me();
  const title = text(p ? p.title : '', {required: true, maxLength: 120, placeholder: 'Fondue & Fort Night'});
  const subtitle = text(p ? p.subtitle : '', {maxLength: 120, placeholder: 'Sweet & Savory Fondue, plus Build-Your-Own Fort'});
  const summary = textarea(p ? p.summary : '', 2);
  summary.placeholder = 'One or two sentences for the party card';
  const description = textarea(p ? p.description : '', 6);
  description.placeholder = 'Everything a guest should know about the party';
  const needToKnow = textarea(p ? p.needToKnow : '', 2);
  needToKnow.placeholder = 'Adults only; bring a swimsuit; drop-off is fine';
  const category = select([{label: 'No category', value: ''}, ...state.model.categories.map(c => ({label: c, value: c}))], p ? p.category : '');
  const audience = text(p ? p.audience : '', {maxLength: 60, placeholder: 'Adults, Families, Kids & Adults, Grades 3-6'});
  const image = imagePicker(p ? p.image : '', p ? p.imageUrl : '', {query: () => title.value, hint: 'The wide banner across the page and the card.'});
  const flyer = imagePicker(p ? p.flyer : '', p ? p.flyerUrl : '', {label: 'Flyer', plain: true, hint: 'The party\u2019s poster, shown whole beside the page. Optional.'});
  // The friendly address, with the whole address it makes shown under it.
  const pretty = text(p ? p.prettyId : '', {maxLength: 40, placeholder: 'fondue'});
  const prettyHint = el('small', '', '');
  const paintPretty = () => {
    const v = pretty.value.trim().toLowerCase();
    prettyHint.textContent = v ? `${location.origin}/p/${v}` : 'Lower-case letters, digits and hyphens; optional. Without one the party lives at /parties/{id}.';
  };
  pretty.addEventListener('input', paintPretty);
  paintPretty();
  const prettyField = field('Friendly address', pretty);
  prettyField.append(prettyHint);
  const basics = [
    field('Title', title, '', true), field('Subtitle', subtitle), field('Summary', summary, 'Shown on the party card.'),
    field('Description', description), field('Need to know', needToKnow, 'Shown in bold under the description.'),
    field('Category', category), field('Audience', audience, 'The words on the card: who the party is for.'), prettyField, image.wrap, flyer.wrap,
  ];

  const start = whenPickers('Starts', p ? p.start : '');
  const end = whenPickers('Ends', p ? p.end : '');
  const whenWrap = el('div', 'field-when');
  whenWrap.append(start.wrap, end.wrap);
  const location = text(p ? p.location : '', {maxLength: 120, placeholder: "The Parks' House in Los Altos"});
  const address = text(p ? p.address : '', {maxLength: 200, placeholder: '1420 Alder Court, Los Altos, CA 94024'});
  const when = [
    field('When', whenWrap),
    field('Where, in words', location, 'Shown to everyone: the neighborhood or the venue, not the street.'),
    field('Street address', address, 'Shown only to signed-in Helios members, with a map link.'),
  ];

  const price = text(p ? p.price : '', {type: 'number', min: 0, step: '0.01', required: true, placeholder: '65'});
  const unit = text(p ? p.unit : '', {maxLength: 40, placeholder: 'person, adult, child, family'});
  const capacity = text(p && p.capacity ? p.capacity : '', {type: 'number', min: 1, step: 1, placeholder: 'Leave blank for no limit'});
  const minimum = text(p && p.minimum ? p.minimum : '', {type: 'number', min: 1, step: 1, placeholder: 'Leave blank for none'});
  const ticketsOpen = checkbox('Tickets on sale', p ? p.ticketsOpen : true, 'Off, the party is listed but sells nothing.');
  const waitlist = checkbox('Take a waitlist when full', p ? p.waitlist : true, 'Off, a full party shows Sold Out.');
  const parents = checkbox('Parents', p ? p.parents : true, 'Parents can hold a ticket.');
  const students = checkbox('Students', p ? p.students : false, 'Students can hold a ticket.');
  const staff = checkbox('Staff', p ? p.staff : true, 'Staff can hold a ticket.');
  const dropOff = checkbox('Drop-off is okay', p ? p.dropOff : false, 'Kids can come without a parent.');
  const parentTicket = checkbox('A parent who stays needs a ticket', p ? p.parentTicket : false, '');
  const tickets = [
    field('Price', price, '', true), field('One ticket covers', unit, '"$65 per person": the word after "per".'),
    field('Tickets available', capacity), field('Minimum to hold the party', minimum, 'The party goes ahead only with at least this many tickets sold.'),
    ticketsOpen.wrap, waitlist.wrap, el('div', 'field-group-label', 'Who can come'), parents.wrap, students.wrap, staff.wrap, dropOff.wrap, parentTicket.wrap,
  ];

  const hostsText = text(p ? p.hosts : '', {maxLength: 120, placeholder: 'McDowell and Park/Gulliver Families'});
  const initialHosts = p ? p.hostPeople : [{email: user.email, name: user.name, photoUrl: user.photoUrl}];
  const hostEmails = hostChips(initialHosts);
  const hosts = [
    field('Hosts, as shown', hostsText, 'The names on the party page: "Hosted by …".'),
    field('Who runs it', hostEmails.wrap, 'These people can edit the party and see who is coming. Click a face to remove it.'),
  ];

  const panels = [
    {label: 'Basics', icon: 'party', fields: basics},
    {label: 'When & where', icon: 'calendar', fields: when},
    {label: 'Tickets', icon: 'ticket', fields: tickets},
    {label: 'Hosts', icon: 'people', fields: hosts},
  ];
  let status = null;
  let celebrationPick = null;
  if (isAdmin()) {
    status = select([{label: 'Open', value: 'Open'}, {label: 'Pending approval', value: 'Pending'}, {label: 'Hidden', value: 'Hidden'}], p ? p.status : 'Open');
    celebrationPick = select(state.model.celebrations.map(c => ({label: c.title, value: c.code})), p ? p.celebration : (currentCelebration() || {}).code);
    panels.push({label: 'Admin', icon: 'tools', fields: [
      field('Status', status, 'Open is listed for everyone; Pending waits for approval; Hidden is parked.'),
      field('Celebration', celebrationPick, 'Which year the party belongs to.'),
    ]});
  }
  const intro = adding && !isAdmin() ? [el('p', 'form-lead', 'Thank you for hosting! Fill this in and the celebration committee will review it and open it for tickets.')] : [];
  openModal(adding ? 'Host a Party' : `Edit ${p.title}`, [...intro, tabbedFields(panels)], {
    wide: true,
    saveLabel: adding ? (isAdmin() ? 'Add Party' : 'Submit for Approval') : 'Save',
    submit: () => send('POST', '/api/celebrate/party', {
      id: p ? p.id : '', celebration: celebrationPick ? celebrationPick.value : '',
      title: title.value, subtitle: subtitle.value, summary: summary.value, description: description.value, needToKnow: needToKnow.value,
      hosts: hostsText.value, hostEmails: hostEmails.value(), category: category.value, audience: audience.value, unit: unit.value,
      price: Number(price.value || 0), capacity: Number(capacity.value || 0), minimum: Number(minimum.value || 0),
      start: start.value(), end: end.value(), location: location.value, address: address.value, image: image.value(), flyer: flyer.value(), prettyId: pretty.value, status: status ? status.value : '',
      ticketsOpen: ticketsOpen.input.checked, waitlist: waitlist.input.checked, parents: parents.input.checked,
      students: students.input.checked, staff: staff.input.checked, dropOff: dropOff.input.checked, parentTicket: parentTicket.input.checked,
    }),
    afterSave: result => {
      if (result && result.id && party(result.id)) {
        // Changing the friendly address moves the page: the bar follows.
        goTo(partyPath(party(result.id)));
        if (adding && !isAdmin()) {
          toast('Submitted - an admin will review it');
        }
      }
    },
    onDelete: p && isAdmin() ? () => send('DELETE', '/api/celebrate/party', {id: p.id}) : null,
    confirmDelete: p ? `Delete ${p.title}? This cannot be undone.` : '',
    afterDelete: () => goTo('/'),
  });
}

// savePartyFields saves a party as it is with a few fields changed - the
// rail's flyer upload, say - so a change from the page needs no form.
export async function savePartyFields(p, changes) {
  const body = {
    id: p.id, celebration: p.celebration, title: p.title, subtitle: p.subtitle || '', summary: p.summary || '', description: p.description || '',
    needToKnow: p.needToKnow || '', hosts: p.hosts || '', hostEmails: p.hostEmails, category: p.category || '', audience: p.audience || '',
    unit: p.unit || '', price: p.price, capacity: p.capacity || 0, minimum: p.minimum || 0, start: p.start || '', end: p.end || '',
    location: p.location || '', address: p.address || '', image: p.image || '', flyer: p.flyer || '', prettyId: p.prettyId || '', status: isAdmin() ? p.status : '', ticketsOpen: p.ticketsOpen,
    waitlist: p.waitlist, parents: p.parents, students: p.students, staff: p.staff, dropOff: p.dropOff, parentTicket: p.parentTicket,
    ...changes,
  };
  try {
    await send('POST', '/api/celebrate/party', body);
    await reload();
  } catch (err) {
    toast(err.message);
  }
}

// setFlags posts the party page's row of switches, all together.
export async function setFlags(p, changes) {
  const body = {
    id: p.id, ticketsOpen: p.ticketsOpen, waitlist: p.waitlist, parents: p.parents, students: p.students, staff: p.staff,
    dropOff: p.dropOff, parentTicket: p.parentTicket, ...changes,
  };
  try {
    await send('POST', '/api/celebrate/party/flags', body);
    await reload();
  } catch (err) {
    toast(err.message);
    await reload();
  }
}

export async function setPartyStatus(p, status) {
  try {
    await send('POST', '/api/celebrate/party/status', {id: p.id, status});
    await reload();
    toast(status === 'Open' ? `${p.title} is open` : `${p.title} is ${status.toLowerCase()}`);
  } catch (err) {
    toast(err.message);
  }
}

// contactFor is how to reach one ticket holder: a student through their
// parents, a guest through whoever bought the ticket, anyone else at their
// own address. Each is a name and the addresses to write to.
async function contactFor(a) {
  if (!a.email) {
    return {via: a.purchaserName ? `${a.purchaserName} (bought the ticket)` : 'the purchaser', emails: a.purchaser ? [a.purchaser] : []};
  }
  const info = await personInfo(a.email);
  if (info && info.isStudent && info.parentEmails && info.parentEmails.length) {
    return {via: 'Parents', emails: info.parentEmails};
  }
  return {via: '', emails: [a.email]};
}

// openContacts is the hosts' attendee list: every ticket with how to reach
// the person - a child's parents, a guest's purchaser - and who is billed,
// with the whole table and the addresses alone each a click to copy.
export async function openContacts(p) {
  const all = [...p.attendees, ...p.waitlisted];
  const rows = await Promise.all(all.map(async a => ({a, contact: await contactFor(a)})));
  const table = el('table', 'contact-table');
  const head = el('tr');
  for (const h of ['Name', 'Contact', 'Billed to', 'Status', 'Note']) {
    head.append(el('th', '', h));
  }
  table.append(head);
  for (const {a, contact} of rows) {
    const row = el('tr');
    const who = el('td');
    who.append(el('div', 'contact-name', a.name));
    if (a.line) {
      who.append(el('div', 'contact-line', a.line));
    }
    const reach = el('td');
    if (contact.via) {
      reach.append(el('div', 'contact-line', contact.via));
    }
    for (const e of contact.emails) {
      const link = el('a', 'contact-email', e);
      link.href = `mailto:${e}`;
      reach.append(link);
    }
    if (!contact.emails.length) {
      reach.append(el('div', 'contact-line', '—'));
    }
    row.append(who, reach, el('td', '', a.purchaserName || a.purchaser || ''), el('td', '', a.status), el('td', '', a.note || ''));
    table.append(row);
  }
  const emails = [...new Set(rows.flatMap(r => r.contact.emails))];
  const copyAddresses = button(`Copy ${emails.length} ${emails.length === 1 ? 'address' : 'addresses'}`, 'mail', 'button button-secondary', () => {
    navigator.clipboard.writeText(emails.join(', ')).then(() => toast('Addresses copied'), () => toast('Could not copy'));
  });
  // The table as tab-separated lines, which pastes straight into a sheet.
  const copyTable = button('Copy table', 'copy', 'button button-secondary', () => {
    const lines = [['Name', 'Line', 'Contact', 'Billed to', 'Status', 'Note'].join('\t')];
    for (const {a, contact} of rows) {
      lines.push([a.name, a.line || '', contact.emails.join(' '), a.purchaser || '', a.status, a.note || ''].map(v => v.replace(/\s+/g, ' ')).join('\t'));
    }
    navigator.clipboard.writeText(lines.join('\n')).then(() => toast('Table copied'), () => toast('Could not copy'));
  });
  const actions = el('div', 'contact-actions');
  actions.append(copyAddresses, copyTable);
  const wrap = el('div', 'contact-wrap');
  wrap.append(table);
  openModal(`Who's coming to ${p.title}`, [actions, wrap], {wide: true});
}

// openCelebration adds or edits a year's celebration.
export function openCelebration(c) {
  const code = text(c ? c.code : '', {required: true, maxLength: 20, placeholder: 'SC-2027'});
  const title = text(c ? c.title : '', {required: true, maxLength: 120, placeholder: 'Helios Spring Celebration 2027'});
  const subtitle = text(c ? c.subtitle : '', {maxLength: 120, placeholder: 'The theme'});
  const start = whenPickers('Starts', c ? c.start : '');
  const end = whenPickers('Ends', c ? c.end : '');
  const whenWrap = el('div', 'field-when');
  whenWrap.append(start.wrap, end.wrap);
  const location = text(c ? c.location : '', {maxLength: 120});
  const address = text(c ? c.address : '', {maxLength: 200});
  const description = textarea(c ? c.description : '', 4);
  const buttonText = text(c ? c.buttonText : '', {maxLength: 40, placeholder: 'Learn More'});
  const buttonUrl = text(c ? c.buttonUrl : '', {type: 'url', maxLength: 500, placeholder: 'https://www.heliosschool.org/spring-celebration'});
  const current = checkbox('This is the current celebration', c ? c.current : true, 'The parties page leads with it: its banner across the top.');
  const image = imagePicker(c ? c.image : '', c ? c.imageUrl : '', {query: () => title.value, label: 'Banner image', hint: 'The background of the banner across the top of the parties page.'});
  openModal(c ? `Edit ${c.title}` : 'Add a celebration', [
    field('Code', code, 'Short and unique, like SC-2027. Parties are filed under it.', true), field('Title', title, 'The banner\u2019s small line: "Helios Spring Celebration 2026".', true),
    field('Subtitle', subtitle, 'The banner\u2019s big line: the theme.'),
    field('When', whenWrap), field('Where', location), field('Address', address), field('Description', description),
    el('div', 'field-group-label', 'Banner button'),
    field('Button text', buttonText, 'Leave both blank for no button.'), field('Button link', buttonUrl, 'Where it goes - the celebration\u2019s own site, say.'),
    current.wrap, image.wrap,
  ], {
    submit: () => send('POST', '/api/celebrate/celebration', {
      original: c ? c.code : '', code: code.value.trim(), title: title.value, subtitle: subtitle.value, start: start.value(), end: end.value(),
      location: location.value, address: address.value, description: description.value, image: image.value(), buttonText: buttonText.value, buttonUrl: buttonUrl.value, current: current.input.checked,
    }),
    onDelete: c ? () => send('DELETE', '/api/celebrate/celebration', {code: c.code}) : null,
    confirmDelete: c ? `Delete ${c.title}?` : '',
  });
}

export function openCategory(title, after) {
  const input = text(title || '', {required: true, maxLength: 120});
  openModal(title ? 'Rename category' : 'Add a category', [field('Title', input, '', true)], {
    submit: () => send('POST', '/api/celebrate/category', {original: title || '', title: input.value}),
    afterSave: after,
    onDelete: title ? () => send('DELETE', '/api/celebrate/category', {title}) : null,
    confirmDelete: title ? `Delete the category ${title}?` : '',
    afterDelete: after,
  });
}

export function openSettings() {
  const s = state.model.settings;
  const intro = textarea(s.partiesIntro, 3);
  const note = textarea(s.ticketNote, 4);
  openModal('Settings', [
    field('Parties intro', intro, 'The line under the parties page heading.'),
    field('Ticket note', note, 'Shown on the ticket form: how invoicing works, the refund policy.'),
  ], {
    submit: () => send('POST', '/api/celebrate/settings', {partiesIntro: intro.value, ticketNote: note.value}),
  });
}
