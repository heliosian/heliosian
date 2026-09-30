import {el, svg, button, toast, longToast} from '/elements.js';
import {popup, closeModal} from '/modal.js';
import {field, text as textInput} from '/form.js';
import {load} from '/router.js';
import {openPersonCard} from '/personcard.js';
import {act} from '/data.js';
import {createPersonPicker} from '/picker.js';
import {answerWords, firstName, answerButtons, ticketWords, ticketDetail, answeredWords, pickerPeople} from './inviteparts.js';
import {state, me} from './state.js';
import {personRow} from '/personrow.js';

function guestForm(e, of, onDone) {
  const form = el('form', 'admin-form guest-form');
  const tabs = el('div', 'tabs');
  const panels = {};
  let active = 'helios';
  for (const [key, label] of [['helios', 'From Helios'], ['outside', 'Someone else']]) {
    const b = el('button', 'tab-button' + (key === active ? ' is-active' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      active = key;
      for (const t of tabs.children) {
        t.classList.toggle('is-active', t === b);
      }
      for (const [k, panel] of Object.entries(panels)) {
        panel.hidden = k !== key;
      }
    });
    tabs.append(b);
  }
  form.append(tabs);
  const helios = el('div');
  const mount = el('div', 'cohost-picker');
  const picker = createPersonPicker(mount, {people: pickerPeople(() => true)});
  helios.append(field('Who', mount));
  panels.helios = helios;
  const outside = el('div');
  outside.hidden = true;
  const name = textInput('', {maxLength: 200, placeholder: 'Full name'});
  const email = textInput('', {type: 'email', placeholder: 'Optional'});
  outside.append(field('Name', name), field('Email', email));
  panels.outside = outside;
  form.append(helios, outside);
  const choices = el('div', 'guest-form-choices');
  const yes = el('input');
  yes.type = 'checkbox';
  yes.checked = true;
  const yesLabel = el('label', 'message-to-choice');
  yesLabel.append(yes, el('span', '', 'RSVP them as Yes'));
  const invite = el('input');
  invite.type = 'checkbox';
  invite.checked = true;
  const inviteLabel = el('label', 'message-to-choice');
  inviteLabel.append(invite, el('span', '', 'Send them their own invitation'));
  choices.append(yesLabel, inviteLabel);
  form.append(choices);
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('plus'), el('span', '', 'Add guest'));
  actions.append(submit, status);
  form.append(actions);
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    const body = {of, answer: yes.checked ? 'yes' : '', invite: invite.checked};
    if (active === 'helios') {
      if (!picker.value) {
        status.textContent = 'Pick someone from the directory first.';
        status.classList.add('error');
        return;
      }
      body.email = picker.value;
      body.name = picker.person.fullName;
    } else {
      if (!name.value.trim()) {
        status.textContent = 'A name, please.';
        status.classList.add('error');
        return;
      }
      if (invite.checked && !email.value.trim()) {
        status.textContent = 'An email address is needed to send them an invitation - or untick that.';
        status.classList.add('error');
        return;
      }
      body.name = name.value.trim();
      body.email = email.value.trim();
    }
    submit.disabled = true;
    try {
      await act('events', e.id, 'bring-guest', body);
      toast(`${body.name} added`);
      onDone();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      submit.disabled = false;
    }
  });
  return form;
}

export function openGuestForm(e, of, onDone) {
  let shut = null;
  const form = guestForm(e, of, () => {
    shut();
    onDone();
  });
  shut = popup('Add a guest', form).shut;
  form.querySelector('.person-picker input').focus();
}

export function openGuestCard(e, p, view, refresh) {
  const person = {...p, line: p.guestOf ? `Guest of ${p.guestOfName}` : p.line};
  if (!view.host) {
    return openPersonCard(person);
  }
  const fields = [];
  if (view.party && p.invited) {
    const standing = el('div', 'guest-card-ticket is-' + (p.ticket || 'none'));
    standing.append(svg(p.ticket === 'ticket' ? 'ticket' : p.ticket === 'free' ? 'gift' : p.ticket === 'waitlist' ? 'clock' : 'close'));
    const words = el('div');
    words.append(el('strong', '', ticketWords(p.ticket)), el('div', 'invite-line', ticketDetail(p.ticket)));
    standing.append(words);
    fields.push(standing);
  }
  const ask = el('div', 'guest-card-ask');
  ask.append(el('div', 'rsvps-head', 'Their answer'));
  ask.append(answerButtons(p, e, () => {
    toast(p.answer ? `${firstName(p)}: ${answerWords[p.answer]}` : `${firstName(p)}’s answer cleared`);
    refresh();
  }, {small: false}));
  if (p.answer) {
    ask.append(el('div', 'guests-by', answeredWords(p)));
  }
  fields.push(ask);
  const links = el('div', 'guest-card-links');
  if (p.invited && p.email) {
    links.append(button(p.sent ? 'Resend invitation' : 'Send invitation', 'calendar', 'link-button', async () => {
      try {
        await act('events', e.id, 'send', {emails: [p.key]});
        longToast('The invitation is on its way');
        closeModal();
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
  }
  if (p.invited || p.key === me().email) {
    links.append(button('Take off the list', 'trash', 'link-button danger', async () => {
      if (!confirm(`Take ${p.name || p.email} off the list? Their answer goes with them${p.via && p.via.startsWith('group:') ? ', and the group will not add them back' : ''}.`)) {
        return;
      }
      try {
        await act('events', e.id, 'uninvite', {email: p.key});
        closeModal();
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
  }
  if (links.childElementCount) {
    fields.push(links);
  }
  return openPersonCard(person, [{label: 'Answer', icon: svg('calendar'), fields}]);
}

export function warningChip(e, view, r, refresh) {
  const chip = el('span', 'guests-chip is-warning');
  chip.title = r.warningWords;
  chip.append(svg('info'), el('span', '', r.warning === 'bounced' ? 'Bounced' : 'Not in the directory'));
  const edit = el('button', 'guests-chip-edit');
  edit.type = 'button';
  edit.textContent = 'Edit email';
  edit.addEventListener('click', ev => {
    ev.stopPropagation();
    openEmailEdit(e, view, r, refresh);
  });
  chip.append(edit);
  return chip;
}

function openEmailEdit(e, view, r, refresh) {
  const form = el('form', 'admin-form');
  form.append(el('p', 'hint', r.warningWords + ' Give a different address, or close this and send to it anyway.'));
  const wrap = el('div', 'field');
  const input = el('input', 'email-edit-input');
  input.type = 'email';
  input.required = true;
  input.value = r.email;
  wrap.append(el('span', '', 'Email address'), input);
  form.append(wrap);
  let everywhere = null;
  if (view.moveEverywhere) {
    everywhere = el('input');
    everywhere.type = 'checkbox';
    everywhere.checked = true;
    const label = el('label', 'message-to-choice');
    label.append(everywhere, el('span', '', 'Change it on every Celebrate party - tickets too - and resend invitations already sent'));
    form.append(label);
  }
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('check'), el('span', '', 'Save'));
  actions.append(submit, status);
  form.append(actions);
  let shut = null;
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    submit.disabled = true;
    try {
      const all = Boolean(everywhere && everywhere.checked);
      await act('events', e.id, 'change-email', {email: r.key, to: input.value.trim(), everywhere: all});
      toast(all ? 'Address changed on every party' : 'Address changed');
      shut();
      refresh();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      submit.disabled = false;
    }
  });
  shut = popup(`${r.name || r.email}’s email`, form).shut;
  setTimeout(() => input.select(), 0);
}

export function openPending(e, view, refresh) {
  const box = el('div');
  const unsent = view.list.filter(r => r.invited && !r.sent && r.email);
  box.append(el('p', 'hint', 'Not sent the invitation yet. Each gets an email with the calendar invite; a student’s goes to them and their parents.'));
  const list = el('div', 'picker-results');
  let shut = null;
  const sendTo = async (emails, words) => {
    try {
      await act('events', e.id, 'send', {emails});
      longToast(emails.length === 1 ? 'One invite is on its way' : `${emails.length} invites are on their way`);
      shut();
      refresh();
    } catch (err) {
      toast(err.message);
    }
  };
  for (const r of unsent) {
    let marks = null;
    if (r.warning) {
      marks = el('div', 'guests-marks');
      marks.append(warningChip(e, view, r, () => {
        shut();
        refresh();
      }));
    }
    const tools = el('div', 'pending-tools');
    tools.append(button('Send now', 'calendar', 'button button-small', () => sendTo([r.key])));
    tools.append(button('Skip sending', null, 'link-button pending-skip', async () => {
      const name = r.name || r.email;
      if (!confirm(`${name} will be moved to No reply yet without being sent the invitation. Skip sending to ${name}?`)) {
        return;
      }
      try {
        await act('events', e.id, 'skip', {emails: [r.key]});
        toast(`${name} moved to No reply yet - no email sent`);
        shut();
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
    list.append(personRow(r, {
      className: 'picker-person pending-row',
      lines: [[r.guestOf ? `Guest of ${r.guestOfName}` : r.line, r.email].filter(Boolean).join(' · '), marks],
      after: [tools],
      gradeColors: state.model.gradeColors,
    }));
  }
  box.append(list);
  const actions = el('div', 'modal-actions');
  actions.append(button(`Send all ${unsent.length}`, 'calendar', 'button', () => sendTo(unsent.map(r => r.key))));
  box.append(actions);
  shut = popup(`Pending · ${unsent.length} not sent yet`, box).shut;
}

export async function sendInvites(e, to, count, words, refresh) {
  if (!confirm(words)) {
    return;
  }
  try {
    await act('events', e.id, 'send', {to});
    longToast(count === 1 ? 'One invite is on its way' : `${count} invites are on their way`);
    refresh();
  } catch (err) {
    toast(err.message);
  }
}

export async function deleteInvitation(e, view) {
  const words = view.party
    ? 'Delete the invitation? The guest list and every answer go; the party itself stays on Helios Celebrate.'
    : view.linked
      ? 'Delete the invitation? The guest list and every answer go; the event itself stays on HCA-Team.'
      : 'Delete this event? It comes off the calendar with its guest list, as if it had never been made.';
  if (!confirm(words)) {
    return;
  }
  const own = e.source === 'sheet';
  try {
    await act('events', e.id, 'delete-invitation');
    toast(own ? 'Event deleted' : 'Invitation deleted');
    if (own) {
      location.href = '/';
    } else {
      await load();
    }
  } catch (err) {
    toast(err.message);
  }
}

export function openCancel(e, view, refresh) {
  const box = el('div', 'cancel-box');
  const sent = view.list.filter(r => r.invited && r.sent && r.email).length;
  box.append(el('p', 'hint', `${sent} ${sent === 1 ? 'person has' : 'people have'} been sent the invitation. Cancelling takes the event off everyone’s calendar and lists; its page stays, saying it was cancelled.`));
  const choices = el('div', 'cancel-choices');
  const noteWrap = el('div', 'cancel-note');
  noteWrap.hidden = true;
  const note = el('textarea');
  note.rows = 3;
  note.placeholder = 'A word on why, if you like - it goes in the email.';
  const status = el('span', 'save-status');
  let shut = null;
  const cancel = async notify => {
    try {
      await act('events', e.id, 'cancel', {notify, note: note.value});
      longToast(notify ? `Cancelled - ${sent} ${sent === 1 ? 'person' : 'people'} told` : 'Cancelled');
      shut();
      refresh();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
    }
  };
  const notify = button('Cancel event & notify guests', 'mail', 'button', () => {
    if (noteWrap.hidden) {
      noteWrap.hidden = false;
      note.focus();
      return;
    }
    cancel(true);
  });
  choices.append(notify);
  noteWrap.append(el('label', '', 'Note to the guests'), note, button('Send the cancellation', 'mail', 'button button-small', () => cancel(true)));
  choices.append(noteWrap);
  choices.append(button('Cancel event & don’t notify', 'close', 'button button-secondary', () => cancel(false)));
  choices.append(button('Don’t do anything', null, 'button button-secondary', () => shut()));
  box.append(choices, status);
  shut = popup('Cancel this event?', box).shut;
}
