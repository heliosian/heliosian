import {state, byEmail} from './state.js';
import {el, svg} from './dom.js';
import {familiesOf} from './families.js';
import {load} from './app.js';

export async function submitField(key, field, value, status) {
  status.classList.remove('error');
  status.textContent = 'Saving…';
  const form = new FormData();
  form.append('key', key);
  form.append('field', field);
  form.append('value', value);
  const res = await fetch('/api/directory/edit', {method: 'POST', body: form});
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return false;
  }
  await load();
  return true;
}

export function editPencil(title) {
  const pencil = el('button', 'edit-icon inline');
  pencil.title = title;
  pencil.append(svg('pencil'));
  return pencil;
}

// fieldEditor is the shared text-field editor behind every simple Overrides
// field (preferred name, phone, address, pronouns...): an input, a Save/Cancel
// pair, and (when opts.allowHide and there's a current value) a Hide button
// that clears it. opts.presets, if given ([{label, value}]), adds a row of
// quick-pick buttons above the input - each just fills it in rather than
// submitting immediately, so picking one still goes through the same explicit
// Save as typing a custom value, and both are always available side by side.
export function fieldEditor(anchor, pencil, opts) {
  const box = el('div', 'field-editor');
  const input = el('input');
  input.type = 'text';
  input.value = opts.current || '';
  if (opts.presets) {
    const presets = el('div', 'field-presets');
    for (const preset of opts.presets) {
      const btn = el('button', 'field-preset', preset.label);
      btn.type = 'button';
      btn.addEventListener('click', () => {
        input.value = preset.value;
        input.focus();
      });
      presets.append(btn);
    }
    box.append(presets);
  }
  box.append(input);
  const note = el('div', 'field-note', "This doesn't affect the values shown in Veracross.");
  const buttons = el('div', 'about-buttons');
  const status = el('div', 'media-status about-status');
  const save = el('button', 'media-button primary', 'Save');
  const cancel = el('button', 'media-button', 'Cancel');
  buttons.append(save, cancel);
  if (opts.allowHide && opts.current) {
    const hide = el('button', 'media-button', 'Hide');
    buttons.append(hide);
    hide.addEventListener('click', () => opts.submit('', status));
  }
  box.append(note, buttons, status);
  cancel.addEventListener('click', () => {
    box.remove();
    anchor.hidden = false;
    pencil.hidden = false;
  });
  save.addEventListener('click', () => opts.submit(input.value.trim(), status));
  anchor.hidden = true;
  pencil.hidden = true;
  anchor.after(box);
  input.focus();
}

export async function submitMedia(target, key, kind, file, name, status) {
  status.classList.remove('error');
  status.textContent = 'Uploading…';
  const form = new FormData();
  form.append('target', target);
  form.append('key', key);
  form.append('kind', kind);
  form.append('file', file, name);
  const res = await fetch('/api/directory/upload', {method: 'POST', body: form});
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return;
  }
  await load();
}

// submitPhotoOrder posts a person's complete photo order to the reorder-photos
// endpoint - reordering (drag), deleting (order with one name missing), and
// setting a photo primary (order with that name moved to the front) are all just
// this same request, so drag-reorder, delete, and the photo menu's "Set as
// primary" all funnel through it instead of three separate copies of this fetch.
// onError, if given, runs only on failure - drag-reorder uses it to snap the tiles
// back to where they were; delete and "Set as primary" have no DOM order to revert.
export async function submitPhotoOrder(key, order, status, onError) {
  status.classList.remove('error');
  const res = await fetch('/api/directory/reorder-photos', {
    method: 'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body: new URLSearchParams({key, order: order.join(',')}),
  });
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    if (onError) {
      onError();
    }
    return false;
  }
  await load();
  return true;
}

// submitCrop posts a cropped image as the crop for one of a person's photos
// (target 'person', name identifies which) or for a family's single photo
// (target 'family', name unused).
export async function submitCrop(target, key, name, blob, status) {
  status.classList.remove('error');
  status.textContent = 'Saving crop…';
  const form = new FormData();
  form.append('target', target);
  form.append('key', key);
  form.append('name', name);
  form.append('file', blob, 'crop.jpg');
  const res = await fetch('/api/directory/crop-photo', {method: 'POST', body: form});
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return false;
  }
  await load();
  return true;
}

export function canEditPerson(email) {
  const meEmail = document.body.dataset.userEmail;
  if (email === meEmail || state.model.superEdit) {
    return true;
  }
  const me = byEmail[meEmail];
  return familiesOf(me).some(family =>
    (family.kidEmails || []).includes(email) ||
    (!me.isStudent && (family.adultEmails || []).includes(email)));
}

export function uploadIcon(iconName, title, accept, target, key, kind, status) {
  const wrap = el('label', 'edit-icon');
  wrap.title = title;
  wrap.append(svg(iconName));
  const input = el('input');
  input.type = 'file';
  input.accept = accept;
  input.hidden = true;
  input.addEventListener('change', () => {
    if (input.files.length) {
      submitMedia(target, key, kind, input.files[0], input.files[0].name, status);
    }
  });
  wrap.append(input);
  return wrap;
}

function recordIcon(target, key, status, preview) {
  const button = el('button', 'edit-icon');
  button.title = 'Record pronunciation';
  button.append(svg('mic'));
  let recorder = null;
  button.addEventListener('click', async () => {
    if (recorder) {
      recorder.stop();
      return;
    }
    let stream;
    try {
      stream = await navigator.mediaDevices.getUserMedia({audio: true});
    } catch (err) {
      status.classList.add('error');
      status.textContent = 'microphone unavailable: ' + err.message;
      return;
    }
    status.classList.remove('error');
    status.textContent = 'Recording… tap the microphone again to stop';
    const chunks = [];
    recorder = new MediaRecorder(stream);
    recorder.addEventListener('dataavailable', e => chunks.push(e.data));
    recorder.addEventListener('stop', () => {
      for (const track of stream.getTracks()) {
        track.stop();
      }
      const blob = new Blob(chunks, {type: recorder.mimeType || 'audio/webm'});
      recorder = null;
      button.classList.remove('recording');
      status.textContent = '';
      preview.replaceChildren();
      const audio = el('audio');
      audio.controls = true;
      audio.src = URL.createObjectURL(blob);
      const save = el('button', 'media-button primary', 'Save');
      save.addEventListener('click', () => submitMedia(target, key, 'pronunciation', blob, 'recording', status));
      const discard = el('button', 'media-button', 'Discard');
      discard.addEventListener('click', () => preview.replaceChildren());
      preview.append(audio, save, discard);
    });
    recorder.start();
    button.classList.add('recording');
  });
  return button;
}

function deletePronunciationIcon(target, key, status) {
  const button = el('button', 'edit-icon');
  button.type = 'button';
  button.title = 'Delete pronunciation';
  button.append(svg('trash'));
  button.addEventListener('click', () => submitField(key, target === 'family' ? 'family-pronunciation' : 'pronunciation', '', status));
  return button;
}

export function pronounceEditor(target, key, hasPronunciation) {
  const box = el('div', 'pronounce-edit');
  const actions = el('div', 'pronounce-actions');
  const status = el('div', 'media-status');
  const preview = el('div', 'record-preview');
  actions.append(recordIcon(target, key, status, preview));
  actions.append(uploadIcon('upload', 'Upload an audio file', 'audio/*', target, key, 'pronunciation', status));
  if (hasPronunciation) {
    actions.append(deletePronunciationIcon(target, key, status));
  }
  box.append(actions, status, preview);
  return box;
}
