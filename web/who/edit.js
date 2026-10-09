import {el, svg} from '/elements.js';
import {load} from '/router.js';
import {api} from '/api.js';
import {write, today} from './state.js';
import {keyBetween} from '/order.js';

function failed(status, err) {
  status.classList.add('error');
  status.textContent = err.message;
}

export async function saveCells(id, cells, status) {
  status.classList.remove('error');
  status.textContent = 'Saving…';
  try {
    await write([{set: id, cells}]);
  } catch (err) {
    failed(status, err);
    return false;
  }
  await load();
  return true;
}

export function namesFor(p, preferred) {
  const last = (p.name_sort || '').split(',')[0].trim();
  if (!preferred) {
    return {name_short_override: '', name_long_override: '', name_sort_override: ''};
  }
  return {name_short_override: preferred, name_long_override: last ? `${preferred} ${last}` : preferred, name_sort_override: last ? `${last}, ${preferred}` : preferred};
}

export function editPencil(title) {
  const pencil = el('button', 'edit-icon inline');
  pencil.title = title;
  pencil.append(svg('edit'));
  return pencil;
}

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

export async function uploadPhoto(target, file, status) {
  status.classList.remove('error');
  status.textContent = 'Uploading…';
  const form = new FormData();
  for (const [k, v] of Object.entries(target)) {
    form.append(k, v);
  }
  form.append('photo', file, file.name || 'photo');
  try {
    await api('POST', '/api/do/photo', form);
    if (target.person) {
      await write([{set: target.person, cells: {photo_updated: today()}}]);
    }
  } catch (err) {
    failed(status, err);
    return;
  }
  await load();
}

export async function movePhoto(photos, from, to, status) {
  const rest = photos.filter((_, i) => i !== from);
  const key = keyBetween(rest[to - 1] && rest[to - 1].order, rest[to] && rest[to].order);
  status.classList.remove('error');
  try {
    await write([{set: photos[from].id, cells: {order: key}}]);
  } catch (err) {
    failed(status, err);
    return false;
  }
  await load();
  return true;
}

export async function removePhoto(photo, status) {
  status.classList.remove('error');
  status.textContent = 'Removing…';
  try {
    await write([{delete: photo.id}]);
  } catch (err) {
    failed(status, err);
    return false;
  }
  await load();
  return true;
}

export async function cropPhoto(photo, box, status) {
  status.classList.remove('error');
  status.textContent = 'Saving crop…';
  try {
    await write([{set: photo.id, cells: {crop_left: box.left, crop_top: box.top, crop_width: box.width, crop_height: box.height}}]);
  } catch (err) {
    failed(status, err);
    return false;
  }
  await load();
  return true;
}

async function record(target, blob, name, status) {
  status.classList.remove('error');
  status.textContent = 'Uploading…';
  const form = new FormData();
  for (const [k, v] of Object.entries(target)) {
    form.append(k, v);
  }
  form.append('recording', blob, name);
  try {
    await api('POST', '/api/do/pronunciation', form);
  } catch (err) {
    failed(status, err);
    return;
  }
  await load();
}

export function uploadIcon(iconName, title, accept, onFile) {
  const wrap = el('label', 'edit-icon');
  wrap.title = title;
  wrap.append(svg(iconName));
  const input = el('input');
  input.type = 'file';
  input.accept = accept;
  input.hidden = true;
  input.addEventListener('change', () => {
    if (input.files.length) {
      onFile(input.files[0]);
    }
  });
  wrap.append(input);
  return wrap;
}

function recordIcon(target, status, preview) {
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
      save.addEventListener('click', () => record(target, blob, 'recording', status));
      const discard = el('button', 'media-button', 'Discard');
      discard.addEventListener('click', () => preview.replaceChildren());
      preview.append(audio, save, discard);
    });
    recorder.start();
    button.classList.add('recording');
  });
  return button;
}

function deletePronunciationIcon(id, status) {
  const button = el('button', 'edit-icon');
  button.type = 'button';
  button.title = 'Delete pronunciation';
  button.append(svg('trash'));
  button.addEventListener('click', () => saveCells(id, {pronunciation: ''}, status));
  return button;
}

export function pronounceEditor(target, id, hasPronunciation) {
  const box = el('div', 'pronounce-edit');
  const actions = el('div', 'pronounce-actions');
  const status = el('div', 'media-status');
  const preview = el('div', 'record-preview');
  actions.append(recordIcon(target, status, preview));
  actions.append(uploadIcon('upload', 'Upload an audio file', 'audio/*', file => record(target, file, file.name, status)));
  if (hasPronunciation) {
    actions.append(deletePronunciationIcon(id, status));
  }
  box.append(actions, status, preview);
  return box;
}
