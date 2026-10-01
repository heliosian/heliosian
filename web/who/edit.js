import {el, svg} from '/elements.js';
import {load} from '/router.js';
import {api} from '/api.js';
import {act} from '/data.js';

const types = {person: 'people', family: 'families'};
const photoActions = {person: 'add-photo', family: 'photo'};

function failed(status, err) {
  status.classList.add('error');
  status.textContent = err.message;
}

export async function submitEdit(target, id, body, status) {
  status.classList.remove('error');
  status.textContent = 'Saving…';
  try {
    await act(types[target], id, 'edit', body);
  } catch (err) {
    failed(status, err);
    return false;
  }
  await load();
  return true;
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

export async function storeMedia(kind, file, name) {
  const form = new FormData();
  form.append('kind', kind);
  form.append('file', file, name);
  return (await api('POST', '/api/directory/media', form)).name;
}

export async function submitMedia(target, id, kind, file, name, status) {
  status.classList.remove('error');
  status.textContent = 'Uploading…';
  try {
    const stored = await storeMedia(kind, file, name);
    await act(types[target], id, kind === 'pronunciation' ? 'pronunciation' : photoActions[target], {name: stored});
  } catch (err) {
    failed(status, err);
    return;
  }
  await load();
}

export async function submitPhotoOrder(id, order, status, onError) {
  status.classList.remove('error');
  try {
    await act('people', id, 'order-photos', {names: order});
  } catch (err) {
    failed(status, err);
    if (onError) {
      onError();
    }
    return false;
  }
  await load();
  return true;
}

export async function submitCrop(target, id, name, blob, status) {
  status.classList.remove('error');
  status.textContent = 'Saving crop…';
  try {
    const crop = await storeMedia('photo', blob, 'crop.jpg');
    await act(types[target], id, 'crop-photo', target === 'family' ? {crop} : {name, crop});
  } catch (err) {
    failed(status, err);
    return false;
  }
  await load();
  return true;
}

export function uploadIcon(iconName, title, accept, target, id, kind, status) {
  const wrap = el('label', 'edit-icon');
  wrap.title = title;
  wrap.append(svg(iconName));
  const input = el('input');
  input.type = 'file';
  input.accept = accept;
  input.hidden = true;
  input.addEventListener('change', () => {
    if (input.files.length) {
      submitMedia(target, id, kind, input.files[0], input.files[0].name, status);
    }
  });
  wrap.append(input);
  return wrap;
}

function recordIcon(target, id, status, preview) {
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
      save.addEventListener('click', () => submitMedia(target, id, 'pronunciation', blob, 'recording', status));
      const discard = el('button', 'media-button', 'Discard');
      discard.addEventListener('click', () => preview.replaceChildren());
      preview.append(audio, save, discard);
    });
    recorder.start();
    button.classList.add('recording');
  });
  return button;
}

function deletePronunciationIcon(target, id, status) {
  const button = el('button', 'edit-icon');
  button.type = 'button';
  button.title = 'Delete pronunciation';
  button.append(svg('trash'));
  button.addEventListener('click', async () => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    try {
      await act(types[target], id, 'pronunciation', {name: ''});
    } catch (err) {
      failed(status, err);
      return;
    }
    await load();
  });
  return button;
}

export function pronounceEditor(target, id, hasPronunciation) {
  const box = el('div', 'pronounce-edit');
  const actions = el('div', 'pronounce-actions');
  const status = el('div', 'media-status');
  const preview = el('div', 'record-preview');
  actions.append(recordIcon(target, id, status, preview));
  actions.append(uploadIcon('upload', 'Upload an audio file', 'audio/*', target, id, 'pronunciation', status));
  if (hasPronunciation) {
    actions.append(deletePronunciationIcon(target, id, status));
  }
  box.append(actions, status, preview);
  return box;
}
