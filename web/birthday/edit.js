import {state, me, isAdmin, settings, charity, longDate, dateCell, parseDate, year} from './state.js';
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

export async function act(method, url, body, message) {
  try {
    await send(method, url, body);
    await reload();
    if (message) {
      toast(message);
    }
  } catch (err) {
    toast(err.message);
  }
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
  const first = f.querySelector('input:not([type=hidden]), textarea, select');
  if (first) {
    first.focus();
  }
}

async function goTo(path) {
  const {navigate} = await import('./app.js');
  navigate(path);
}

export function assignToMe(sv) {
  return act('POST', '/api/birthday/assign', {email: sv.email}, `${sv.name} is yours`);
}

export function unassign(sv) {
  return act('DELETE', '/api/birthday/assign', {email: sv.email});
}

export function openAssign(sv) {
  const who = select([{label: 'Me', value: ''}, {label: 'Someone else', value: 'other'}], sv.assignedTo && sv.assignedTo !== me().email ? 'other' : '');
  const email = text(sv.assignedTo && sv.assignedTo !== me().email ? sv.assignedTo : '', {type: 'email', placeholder: 'name@heliosschool.org'});
  const emailField = field('Their email', email);
  emailField.hidden = who.value !== 'other';
  who.addEventListener('change', () => {
    emailField.hidden = who.value !== 'other';
  });
  openModal(`Assign ${sv.name}`, [field('To', who), emailField], {
    saveLabel: 'Assign',
    submit: () => send('POST', '/api/birthday/assign', {email: sv.email, assignedTo: who.value === 'other' ? email.value : ''}),
    onDelete: sv.assignedTo ? () => send('DELETE', '/api/birthday/assign', {email: sv.email}) : null,
    deleteLabel: 'Unassign',
    confirmDelete: `Unassign ${sv.name}?`,
  });
}

export function markContacted(sv, contacted) {
  return act('POST', '/api/birthday/outreach', {email: sv.email, contacted}, contacted ? 'Marked as contacted' : 'Outreach reopened');
}

export function markUsed(sv, used) {
  return act('POST', '/api/birthday/used', {email: sv.email, used}, used ? 'Marked as used in the newsletter' : 'Marked as not yet used');
}

export function useDefault(sv) {
  return act('POST', '/api/birthday/donation', {email: sv.email, charity: settings().defaultCharity, note: ''}, `Recorded ${settings().defaultCharity}`);
}

export function reuseLast(sv) {
  return act('POST', '/api/birthday/donation', {email: sv.email, charity: sv.lastDonation.charity, note: sv.lastDonation.note || ''}, `Recorded ${sv.lastDonation.charity} again`);
}

function charityOptions(current) {
  const options = state.model.charities.filter(c => c.allowed || c.name === current).map(c => ({label: c.allowed ? c.name : `${c.name} (not allowed)`, value: c.name}));
  options.sort((a, b) => a.value.localeCompare(b.value));
  return options;
}

export function openDonation(sv) {
  const existing = sv.donation;
  const pick = select(charityOptions(existing ? existing.charity : ''), existing ? existing.charity : settings().defaultCharity);
  const note = textarea(existing ? existing.note : '', 5);
  const add = el('div', 'field-note');
  const addLink = el('a', 'link-button', 'Not on the list? Add a charity first.');
  addLink.href = '/charities';
  addLink.setAttribute('data-link', '');
  addLink.addEventListener('click', closeModal);
  add.append(addLink);
  openModal(existing ? `Edit ${sv.name}'s donation` : `Record ${sv.name}'s donation`, [field('Charity', pick), add, field('Their note', note, 'Why they chose it, in their words, for the newsletter')], {
    submit: () => send('POST', '/api/birthday/donation', {email: sv.email, charity: pick.value, note: note.value}),
    onDelete: existing ? () => send('DELETE', '/api/birthday/donation', {email: sv.email}) : null,
    deleteLabel: 'Remove',
    confirmDelete: existing ? `Remove ${sv.name}'s donation this year?` : '',
  });
}

export function openBirthday(sv) {
  const email = text(sv.email, {type: 'email', required: true, placeholder: 'name@heliosschool.org'});
  const birthday = text(sv.birthday || '', {type: 'date', required: true});
  const override = select([{label: 'The usual pick', value: ''}, ...state.model.newsletterDates.map(d => ({label: longDate(d), value: d}))], sv.override || '');
  const fields = [];
  if (!sv.birthday) {
    fields.push(field('Email', email));
  }
  fields.push(field('Birthday', birthday, 'The year matters only when it is known'), field('Newsletter', override, 'Which issue carries the birthday, when the first one on or after it is wrong'));
  openModal(sv.birthday ? `Edit ${sv.name}'s birthday` : `Add ${sv.name ? `${sv.name}'s` : 'a'} birthday`, fields, {
    submit: () => send('POST', '/api/birthday/birthday', {email: sv.birthday ? sv.email : email.value, birthday: birthday.value, override: override.value}),
    onDelete: sv.birthday && isAdmin() ? () => send('DELETE', '/api/birthday/birthday', {email: sv.email}) : null,
    deleteLabel: 'Remove birthday',
    confirmDelete: `Remove ${sv.name}'s birthday? Their assignments, outreach, donations, and notes must already be gone.`,
    afterDelete: () => goTo('/skipped'),
  });
}

export function openParticipation(sv) {
  const email = text(sv.email || '', {type: 'email', required: true, placeholder: 'name@heliosschool.org'});
  const level = select([
    {label: 'Skip: does not want to take part at all', value: 'Skip'},
    {label: 'No newsletter: ask and donate, but leave them out of the newsletter', value: 'No Newsletter'},
  ], sv.level || 'Skip');
  const note = textarea(sv.levelNote || '', 3);
  const fields = [];
  if (!sv.email) {
    fields.push(field('Email', email));
  }
  fields.push(field('Preference', level), field('Note', note, 'Who asked, when, anything the team should know'));
  openModal(sv.level ? `Edit ${sv.name}'s preference` : `Set ${sv.name ? `${sv.name}'s` : 'a'} preference`, fields, {
    submit: () => send('POST', '/api/birthday/participation', {email: sv.email || email.value, level: level.value, note: note.value}),
    onDelete: sv.level ? () => send('DELETE', '/api/birthday/participation', {email: sv.email}) : null,
    deleteLabel: 'Clear preference',
    confirmDelete: `Clear ${sv.name}'s preference and treat them like everyone else?`,
  });
}

export function openNote(sv) {
  const note = textarea('', 4);
  openModal(`Note on ${sv.name}`, [field('Note', note)], {
    saveLabel: 'Add Note',
    submit: () => send('POST', '/api/birthday/note', {email: sv.email, note: note.value}),
  });
}

export function removeNote(note) {
  if (!confirm('Remove this note?')) {
    return;
  }
  return act('DELETE', '/api/birthday/note', note);
}

export function openCharity(c) {
  const admin = isAdmin();
  const name = text(c ? c.name : '', {required: true, maxLength: 120});
  const linkInput = text(c ? c.donationLink : '', {type: 'url', required: true, placeholder: 'https://'});
  const about = textarea(c ? c.about : '', 4);
  const ein = text(c ? c.ein : '', {placeholder: '12-3456789'});
  const allowed = checkbox('Allowed', c ? c.allowed : true);
  const why = text(c ? c.whyNotAllowed : '', {placeholder: 'Not a non-profit'});
  const whyField = field('Why not', why);
  whyField.hidden = allowed.input.checked;
  allowed.input.addEventListener('change', () => {
    whyField.hidden = allowed.input.checked;
  });
  const fields = [field('Name', name), field('Donation link', linkInput), field('About', about, 'A sentence for the newsletter'), field('EIN', ein)];
  if (admin) {
    fields.push(allowed.wrap, whyField);
  }
  openModal(c ? 'Edit Charity' : 'Add Charity', fields, {
    submit: async () => {
      await send('POST', '/api/birthday/charity', {
        original: c ? c.name : '', name: name.value, donationLink: linkInput.value, about: about.value, ein: ein.value,
        allowed: admin ? allowed.input.checked : true, whyNotAllowed: why.value,
      });
      if (c && c.name !== name.value.trim() && location.pathname.startsWith('/charities/')) {
        await goTo(`/charities/${encodeURIComponent(name.value.trim())}`);
      }
    },
    onDelete: c && admin ? () => send('DELETE', '/api/birthday/charity', {name: c.name}) : null,
    confirmDelete: c ? `Delete “${c.name}”? Charities with donations can only be marked not allowed.` : '',
    afterDelete: () => goTo('/charities'),
  });
}

export function openNewsletterDate() {
  const dates = state.model.newsletterDates;
  const last = dates.length ? parseDate(dates[dates.length - 1]) : parseDate(year().start);
  last.setDate(last.getDate() + 7);
  const date = text(dateCell(last), {type: 'date', required: true});
  openModal('Add Newsletter Date', [field('Date', date)], {
    saveLabel: 'Add',
    submit: () => send('POST', '/api/birthday/newsletter-date', {date: date.value}),
  });
}

export function addNextWeek(after) {
  const next = parseDate(after);
  next.setDate(next.getDate() + 7);
  return act('POST', '/api/birthday/newsletter-date', {date: dateCell(next)}, `Added ${longDate(dateCell(next))}`);
}

export function removeNewsletterDate(date) {
  if (!confirm(`Remove the ${longDate(date)} newsletter?`)) {
    return;
  }
  return act('DELETE', '/api/birthday/newsletter-date', {date});
}

export function openSettings() {
  const s = settings();
  const defaultCharity = select(charityOptions(s.defaultCharity), s.defaultCharity);
  const yearStart = text(s.yearStart, {required: true, placeholder: '08-14'});
  const subject = text(s.emailSubject, {required: true});
  const body = textarea(s.emailBody, 10);
  const note = textarea(s.noNewsletterNote, 3);
  openModal('Settings', [
    field('Default charity', defaultCharity, 'Where a donation goes when nobody answers'),
    field('Year start', yearStart, 'Month and day the birthday year turns over, like 08-14'),
    field('Email subject', subject),
    field('Email body', body, 'Placeholders: {first name}, {name}, {newsletter date}, {birthday}, {default charity}'),
    field('No-newsletter note', note, 'Added to the email for anyone who asked to stay out of the newsletter'),
  ], {
    submit: () => send('POST', '/api/birthday/settings', {
      defaultCharity: defaultCharity.value, yearStart: yearStart.value, emailSubject: subject.value, emailBody: body.value, noNewsletterNote: note.value,
    }),
  });
}

export function charityNamed(name) {
  return charity(name);
}
