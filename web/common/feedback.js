import {api} from '/api.js';
import {el, toast} from '/elements.js';

const recentErrors = [];

export function noteError(text) {
  recentErrors.push(String(text).slice(0, 300));
  if (recentErrors.length > 5) {
    recentErrors.shift();
  }
}

window.addEventListener('error', e => {
  const where = e.filename ? ` (${e.filename.split('/').pop()}:${e.lineno})` : '';
  noteError(e.message + where);
});

window.addEventListener('unhandledrejection', e => {
  const reason = e.reason;
  noteError(reason && reason.stack ? reason.stack.split('\n').slice(0, 2).join(' ') : String(reason));
});

const prompts = {
  bug: {summary: 'What went wrong?', details: 'What you did, what you expected, and what happened instead.'},
  idea: {summary: 'What would you like?', details: 'Where it would help, and what it would do.'},
};

let feedback;

function buildFeedback() {
  const overlay = el('div', 'feedback-overlay');
  overlay.hidden = true;
  const form = el('form', 'feedback-modal');
  const header = el('div', 'feedback-header');
  const close = el('button', 'feedback-close', '×');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  header.append(el('h2', '', 'Report a problem or idea'), close);
  const summary = document.createElement('input');
  summary.type = 'text';
  summary.maxLength = 120;
  summary.required = true;
  const details = document.createElement('textarea');
  details.rows = 5;
  details.maxLength = 4000;
  const prompt = kind => {
    summary.placeholder = prompts[kind].summary;
    details.placeholder = prompts[kind].details;
  };
  const kinds = el('div', 'feedback-kinds');
  for (const [kind, label] of [['bug', 'Something’s wrong'], ['idea', 'I’d like…']]) {
    const pill = el('label', 'feedback-kind');
    const radio = document.createElement('input');
    radio.type = 'radio';
    radio.name = 'kind';
    radio.value = kind;
    radio.checked = kind === 'bug';
    radio.addEventListener('change', () => prompt(kind));
    pill.append(radio, el('span', '', label));
    kinds.append(pill);
  }
  prompt('bug');
  const summaryField = el('label', 'feedback-field');
  summaryField.append(el('span', '', 'In a line'), summary);
  const detailsField = el('label', 'feedback-field');
  detailsField.append(el('span', '', 'Details'), details);
  let shot = null;
  const picker = document.createElement('input');
  picker.type = 'file';
  picker.accept = 'image/png,image/jpeg,image/gif,image/webp';
  picker.hidden = true;
  const add = el('button', 'feedback-shot-add', 'Add a screenshot');
  add.type = 'button';
  const preview = el('div', 'feedback-shot-preview');
  preview.hidden = true;
  const thumb = document.createElement('img');
  thumb.alt = 'Your screenshot';
  const remove = el('button', 'feedback-shot-remove', 'Remove');
  remove.type = 'button';
  preview.append(thumb, remove);
  const shotField = el('div', 'feedback-field feedback-shot');
  shotField.append(el('span', '', 'Screenshot'), add, el('small', '', 'or paste one'), preview, picker);
  const clearShot = () => {
    if (thumb.src) {
      URL.revokeObjectURL(thumb.src);
      thumb.removeAttribute('src');
    }
    shot = null;
    picker.value = '';
    preview.hidden = true;
    add.hidden = false;
  };
  const takeShot = file => {
    if (!/^image\/(png|jpeg|gif|webp)$/.test(file.type)) {
      status.textContent = 'A screenshot has to be a picture: PNG, JPEG, GIF or WebP.';
      return;
    }
    if (file.size > 10 << 20) {
      status.textContent = 'That picture is over 10 MB; try a smaller one.';
      return;
    }
    clearShot();
    shot = file;
    thumb.src = URL.createObjectURL(file);
    preview.hidden = false;
    add.hidden = true;
    status.textContent = '';
  };
  add.addEventListener('click', () => picker.click());
  remove.addEventListener('click', clearShot);
  picker.addEventListener('change', () => {
    if (picker.files[0]) {
      takeShot(picker.files[0]);
    }
  });
  form.addEventListener('paste', e => {
    const file = [...e.clipboardData.files].find(f => f.type.startsWith('image/'));
    if (file) {
      e.preventDefault();
      takeShot(file);
    }
  });
  const note = el('p', 'feedback-note', 'Goes to the people who build Heliosian, along with this page’s address, your email, your browser details, and any screenshot you add.');
  const actions = el('div', 'feedback-actions');
  const send = el('button', 'feedback-send', 'Send');
  send.type = 'submit';
  const cancel = el('button', 'feedback-cancel', 'Cancel');
  cancel.type = 'button';
  const status = el('span', 'feedback-status');
  actions.append(send, cancel, status);
  form.append(header, kinds, summaryField, detailsField, shotField, note, actions);
  overlay.append(form);
  document.body.append(overlay);
  const hide = () => {
    overlay.hidden = true;
  };
  close.addEventListener('click', hide);
  cancel.addEventListener('click', hide);
  overlay.addEventListener('click', e => {
    if (e.target === overlay) {
      hide();
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      hide();
    }
  });
  form.addEventListener('submit', async e => {
    e.preventDefault();
    status.textContent = 'Sending…';
    send.disabled = true;
    const body = new FormData();
    body.append('report', JSON.stringify({
      kind: form.elements.kind.value,
      summary: summary.value,
      details: details.value,
      url: location.href,
      page: document.title,
      viewport: `${window.innerWidth}×${window.innerHeight}`,
      screen: `${screen.width}×${screen.height} @${window.devicePixelRatio}x`,
      language: navigator.language,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      errors: recentErrors,
    }));
    if (shot) {
      body.append('screenshot', shot);
    }
    try {
      await api('POST', '/api/feedback', body);
      hide();
      form.reset();
      clearShot();
      prompt('bug');
      toast('Thanks, we got it.');
    } catch (err) {
      status.textContent = err.message;
    } finally {
      send.disabled = false;
    }
  });
  return {overlay, summary, status};
}

export function openFeedback() {
  if (!feedback) {
    feedback = buildFeedback();
  }
  feedback.status.textContent = '';
  feedback.overlay.hidden = false;
  feedback.summary.focus();
}
