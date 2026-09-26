import {state, me, team, isAdmin, isSystemAdmin, settings, charity, longDate, dateCell, parseDate, year} from './state.js';
import {el, button, toast} from './dom.js';
import {appOrigin} from '/toolbar.js';

let modalState = null;

// Modals stack: one opened while another shows takes its place, and closing
// it brings the first back as it was, typed text and all.
const stack = [];

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
  const below = stack.pop();
  if (below) {
    form().replaceChildren(...below.nodes);
    modalState = below.state;
    return;
  }
  overlay().hidden = true;
  form().replaceChildren();
  modalState = null;
}

// A required modal - one that must be answered - ignores the overlay and Escape.
function dismissable() {
  return !(modalState && modalState.required);
}

export function initModal() {
  overlay().addEventListener('click', e => {
    if (e.target === overlay() && dismissable()) {
      closeModal();
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && dismissable()) {
      closeModal();
    }
  });
  form().addEventListener('submit', async e => {
    e.preventDefault();
    if (!modalState) {
      return;
    }
    // The state is taken now: a step that opens the next one replaces it
    // while its submit runs.
    const current = modalState;
    setStatus(current.working || 'Saving…');
    try {
      const saved = await current.submit();
      if (current.stay) {
        return;
      }
      const afterSave = current.afterSave;
      closeModal();
      await reload();
      if (afterSave) {
        afterSave(saved);
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

function textarea(value, rows) {
  const input = el('textarea');
  input.rows = rows || 4;
  input.value = value || '';
  return input;
}

function select(options, value) {
  const input = el('select');
  fillSelect(input, options, value);
  return input;
}

function fillSelect(input, options, value) {
  input.replaceChildren();
  for (const option of options) {
    const node = el('option', '', option.label === undefined ? option : option.label);
    node.value = option.value === undefined ? option : option.value;
    node.selected = node.value === value;
    input.append(node);
  }
}

function checkbox(label, checked) {
  const wrap = el('label', 'field field-toggle');
  const input = el('input');
  input.type = 'checkbox';
  input.checked = Boolean(checked);
  wrap.append(el('span', '', label), input);
  return {wrap, input};
}

// openModal shows one form. options.submit saves it (closing and reloading
// after, unless options.stay, for a step that opens the next one itself, with
// options.working as the status meanwhile); options.alternate is a second
// way on, beside the save button; options.replace takes an open modal's
// place rather than stacking on it, so closing goes where it would have.
function openModal(title, fields, options) {
  const f = form();
  if (modalState && !options.replace) {
    stack.push({nodes: [...f.children], state: modalState});
  }
  f.replaceChildren();
  const header = el('div', 'modal-header');
  header.append(el('h2', '', title));
  if (!options.required) {
    const close = el('button', 'modal-close', '×');
    close.type = 'button';
    close.setAttribute('aria-label', 'Close');
    close.addEventListener('click', closeModal);
    header.append(close);
  }
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
  actions.append(save);
  if (options.alternate) {
    const alt = el('button', 'button button-secondary', options.alternate.label);
    alt.type = 'button';
    alt.addEventListener('click', options.alternate.onClick);
    actions.append(alt);
  }
  if (!options.hideCancel) {
    actions.append(cancel);
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
  modalState = {submit: options.submit, afterSave: options.afterSave, stay: options.stay, working: options.working, required: options.required};
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

// openAssign offers the birthday team - the viewer first, then whoever else has
// someone assigned - with the current assignee kept on the list even when they
// have nobody else.
export function openAssign(sv) {
  const people = team();
  if (sv.assignedTo && !people.some(p => p.email === sv.assignedTo)) {
    people.push({email: sv.assignedTo, name: sv.assignedToName || sv.assignedTo});
  }
  const who = select(people.map(p => ({label: p.name, value: p.email})), sv.assignedTo || me().email);
  openModal(`Assign ${sv.name}`, [field('To', who)], {
    saveLabel: 'Assign',
    submit: () => send('POST', '/api/birthday/assign', {email: sv.email, assignedTo: who.value === me().email ? '' : who.value}),
    onDelete: sv.assignedTo ? () => send('DELETE', '/api/birthday/assign', {email: sv.email}) : null,
    deleteLabel: 'Unassign',
    confirmDelete: `Unassign ${sv.name}?`,
  });
}

export function markContacted(sv, contacted) {
  return act('POST', '/api/birthday/outreach', {email: sv.email, contacted}, contacted ? 'Marked as contacted' : 'Outreach reopened');
}

// markAllUsed marks every donation given as carried by the newsletter, one
// after another, then says how many.
export async function markAllUsed(list) {
  let n = 0;
  try {
    for (const sv of list) {
      await send('POST', '/api/birthday/used', {email: sv.email, used: true});
      n++;
    }
    await reload();
    toast(`Marked ${n} as used`);
  } catch (err) {
    await reload();
    toast(err.message);
  }
}

// toShare is who an issue would copy to the shared sheet: everyone it
// carries, No Newsletter too, not yet marked done.
export function toShare(date) {
  return state.model.staff.filter(sv => sv.newsletterDate === date && sv.level !== 'Skip' && !(sv.donation && sv.donation.usedOn));
}

// shareIssue runs the Thursday night export by hand: the issue's birthdays to
// the Staff Birthday List (Shared), each then marked done, the default
// charity recorded for anyone without one.
export async function shareIssue(date) {
  const list = toShare(date);
  const bare = list.filter(sv => !sv.donation).length;
  const words = `Copy ${list.length} ${list.length === 1 ? 'birthday' : 'birthdays'} for ${longDate(date)} to the Staff Birthday List (Shared) and mark ${list.length === 1 ? 'it' : 'them'} done?` +
    (bare ? ` ${bare} with no charity recorded ${bare === 1 ? 'gets' : 'get'} ${settings().defaultCharity}.` : '');
  if (!confirm(words)) {
    return;
  }
  try {
    const res = await fetch('/api/birthday/newsletter/share', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({date})});
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const {copied} = await res.json();
    await reload();
    toast(`Copied ${copied} to the shared sheet`);
  } catch (err) {
    await reload();
    toast(err.message);
  }
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
  const addLink = el('button', 'link-button', 'Not on the list? Add a charity.');
  addLink.type = 'button';
  // The charity form opens over this one; saving it brings this one back with
  // the new charity picked.
  addLink.addEventListener('click', () => openCharity(null, {afterSave: name => {
    fillSelect(pick, charityOptions(name), name);
    showBlurb();
  }}));
  add.append(addLink);
  // The charity's own sentence, above the note, so the person can see what
  // the newsletter already says and write the staff member's reason to it.
  const blurb = el('div', 'charity-blurb');
  const showBlurb = () => {
    const c = charity(pick.value);
    blurb.replaceChildren();
    blurb.hidden = !c || !c.about;
    if (blurb.hidden) {
      return;
    }
    const full = c.about.trim();
    const short = full.length > 120 ? full.slice(0, 120).replace(/\s+\S*$/, '') + '…' : full;
    const textNode = el('span', '', short);
    blurb.append(textNode);
    if (short !== full) {
      const more = el('button', 'link-button small', 'read more');
      more.type = 'button';
      more.addEventListener('click', () => {
        textNode.textContent = full;
        more.remove();
      });
      blurb.append(' ', more);
    }
  };
  pick.addEventListener('change', showBlurb);
  showBlurb();
  openModal(existing ? `Edit ${sv.name}'s donation` : `Record ${sv.name}'s donation`, [field('Charity', pick), add, blurb, field('Their note', note, 'Why they chose it, in their words, for the newsletter')], {
    submit: () => send('POST', '/api/birthday/donation', {email: sv.email, charity: pick.value, note: note.value}),
    onDelete: existing ? () => send('DELETE', '/api/birthday/donation', {email: sv.email}) : null,
    deleteLabel: 'Remove',
    confirmDelete: existing ? `Remove ${sv.name}'s donation this year?` : '',
  });
}

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long'});
const months = Array.from({length: 12}, (_, i) => ({label: monthFormat.format(new Date(2000, i, 1)), value: String(i + 1).padStart(2, '0')}));
const days = Array.from({length: 31}, (_, i) => String(i + 1).padStart(2, '0'));

export function openBirthday(sv) {
  const email = text(sv.email, {type: 'email', required: true, placeholder: 'name@heliosschool.org'});
  const [currentMonth, currentDay] = (sv.birthday || '').split('-');
  const month = select(months, currentMonth || '01');
  const day = select(days, currentDay || '01');
  const birthday = el('div', 'field-pair');
  birthday.append(month, day);
  const override = select([{label: 'The usual pick', value: ''}, ...state.model.newsletterDates.map(d => ({label: longDate(d), value: d}))], sv.override || '');
  const fields = [];
  if (!sv.birthday) {
    fields.push(field('Email', email));
  }
  fields.push(field('Birthday', birthday), field('Newsletter', override, 'Which issue carries the birthday, when the first one on or after it is wrong'));
  openModal(sv.birthday ? `Edit ${sv.name}'s birthday` : `Add ${sv.name ? `${sv.name}'s` : 'a'} birthday`, fields, {
    submit: () => send('POST', '/api/birthday/birthday', {email: sv.birthday ? sv.email : email.value, birthday: `${month.value}-${day.value}`, override: override.value}),
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

// describeCharity asks the server what Claude finds: where to donate and the
// newsletter's sentence.
async function describeCharity(name, donationLink) {
  const res = await fetch('/api/birthday/charity/describe', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({name, donationLink})});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

// openCharity is the charity form, whole: the name, then Generate with
// Claude, which finds where to donate and writes the sentence into the
// fields below for reading over, or the fields typed by hand.
export function openCharity(c, options) {
  openCharityForm(c, {}, options);
}

// openCharityForm is the full form, for an existing charity or a new one
// whose fields start as given.
function openCharityForm(c, start, options) {
  const admin = isAdmin();
  const name = text(c ? c.name : start.name || '', {required: true, maxLength: 120});
  const linkInput = text(c ? c.donationLink : start.donationLink || '', {type: 'url', required: true, placeholder: 'https://'});
  const about = textarea(c ? c.about : start.about || '', 4);
  const allowed = checkbox('Allowed', c ? c.allowed : true);
  const why = text(c ? c.whyNotAllowed : '', {placeholder: 'Not a non-profit'});
  const whyField = field('Why not', why);
  whyField.hidden = allowed.input.checked;
  allowed.input.addEventListener('change', () => {
    whyField.hidden = allowed.input.checked;
  });
  // Generate with Claude looks the charity up by its name and fills the link
  // and the sentence below, for reading over; from the link too, when one is
  // typed. A field that already says something different is not overwritten:
  // the two are shown side by side to choose between.
  const suggestRow = el('div', 'suggest-row');
  const compares = [];
  const offer = (input, found, label) => {
    const have = input.value.trim();
    if (!found || found === have) {
      return;
    }
    if (!have) {
      input.value = found;
      return;
    }
    const box = el('div', 'compare');
    box.append(el('div', 'compare-title', `${label}: yours, or Claude's?`));
    const pair = el('div', 'compare-pair');
    for (const [heading, value, pick] of [['Yours', have, have], ["Claude's", found, found]]) {
      const side = el('div', 'compare-side');
      side.append(el('div', 'compare-heading', heading), el('div', 'compare-text', value));
      const use = el('button', 'button button-secondary button-small', `Use ${heading === 'Yours' ? 'mine' : "Claude's"}`);
      use.type = 'button';
      use.addEventListener('click', () => {
        input.value = pick;
        box.remove();
      });
      side.append(use);
      pair.append(side);
    }
    box.append(pair);
    input.parentNode.after(box);
    compares.push(box);
  };
  const suggest = button('Generate with Claude', 'sparkle', 'button button-secondary button-small', async () => {
    if (!name.value.trim()) {
      suggestStatus.textContent = 'Give the name first.';
      return;
    }
    suggest.disabled = true;
    suggestStatus.textContent = 'Looking them up…';
    for (const box of compares.splice(0)) {
      box.remove();
    }
    try {
      const info = await describeCharity(name.value, linkInput.value);
      offer(linkInput, info.donationLink, 'Donation link');
      offer(about, info.sentence, 'About');
      suggestStatus.textContent = compares.length ? 'Choose below, then read it over.' : 'Read it over and change anything that is off.';
    } catch (err) {
      suggestStatus.textContent = err.message;
    } finally {
      suggest.disabled = false;
    }
  });
  const suggestStatus = el('span', 'suggest-status');
  suggestRow.append(suggest, suggestStatus);
  const fields = [field('Name', name), suggestRow, field('Donation link', linkInput), field('About', about, 'A sentence for the newsletter')];
  if (admin) {
    fields.push(allowed.wrap, whyField);
  }
  openModal(c ? 'Edit Charity' : 'Add Charity', fields, {
    replace: !c,
    submit: async () => {
      await send('POST', '/api/birthday/charity', {
        original: c ? c.name : '', name: name.value, donationLink: linkInput.value, about: about.value, ein: c ? c.ein : '',
        allowed: admin ? allowed.input.checked : true, whyNotAllowed: why.value,
      });
      if (c && c.name !== name.value.trim() && location.pathname.startsWith('/charities/')) {
        await goTo(`/charities/${encodeURIComponent(name.value.trim())}`);
      }
      return name.value.trim();
    },
    afterSave: options && options.afterSave,
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

export function openChangeNewsletterDate(original) {
  const date = text(original, {type: 'date', required: true});
  openModal('Change Newsletter Date', [field('Date', date, 'Anyone pinned to this issue moves with it')], {
    submit: () => send('PUT', '/api/birthday/newsletter-date', {original, date: date.value}),
  });
}

export function addNextWeek(after) {
  const next = parseDate(after);
  next.setDate(next.getDate() + 7);
  return act('POST', '/api/birthday/newsletter-date', {date: dateCell(next)}, `Added ${longDate(dateCell(next))}`);
}

// openCreateNewsletterDates lays out a weekly run of issues: a weekday, from
// a date to a final one, skipping any already listed.
export function openCreateNewsletterDates() {
  const dates = state.model.newsletterDates;
  const last = dates.length ? parseDate(dates[dates.length - 1]) : null;
  const start = last && dateCell(last) >= state.model.today ? new Date(last.getFullYear(), last.getMonth(), last.getDate() + 1) : parseDate(state.model.today);
  const weekday = select([['0', 'Sunday'], ['1', 'Monday'], ['2', 'Tuesday'], ['3', 'Wednesday'], ['4', 'Thursday'], ['5', 'Friday'], ['6', 'Saturday']].map(([value, label]) => ({value, label})), String(last ? last.getDay() : 5));
  const from = text(dateCell(start), {type: 'date', required: true});
  const to = text(year().end, {type: 'date', required: true});
  openModal('Create Newsletter Dates', [
    field('Every', weekday),
    field('Starting', from, 'The first issue is the first of that weekday on or after this'),
    field('Through', to, 'The final date; the birthday year ends ' + longDate(year().end)),
  ], {
    saveLabel: 'Create',
    submit: async () => {
      const res = await fetch('/api/birthday/newsletter-dates/create', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({weekday: Number(weekday.value), from: from.value, to: to.value})});
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const {added} = await res.json();
      toast(`Added ${added} ${added === 1 ? 'date' : 'dates'}`);
    },
  });
}

// clearFutureNewsletterDates removes every issue from today on, after a word.
export function clearFutureNewsletterDates(count) {
  if (!confirm(`Remove the ${count} newsletter ${count === 1 ? 'date' : 'dates'} from today on? The ones already out stay.`)) {
    return;
  }
  return act('POST', '/api/birthday/newsletter-dates/clear-future', {}, `Removed ${count} ${count === 1 ? 'date' : 'dates'}`);
}

export function removeNewsletterDate(date) {
  if (!confirm(`Remove the ${longDate(date)} newsletter?`)) {
    return;
  }
  return act('DELETE', '/api/birthday/newsletter-date', {date});
}

// Someone who is not on the birthday team is asked, whenever the app opens
// for them, whether they want to be. Yes puts them on it as a volunteer and
// opens the app to them on their Heliosian home; No sends them back to the
// Heliosian home. Admins run the app without being asked.
export function offerTeam() {
  const email = me().email;
  if (isSystemAdmin() || state.model.team.some(m => m.email === email)) {
    return;
  }
  const blurb = el('div', 'join-blurb');
  blurb.append(
    el('p', '', 'The birthday team celebrates every Helios staff member\'s birthday with a donation to a charity they choose, announced in the newsletter. Volunteers take a birthday each, reach out with a short email, and record the answer - a few minutes apiece.'),
    el('p', '', 'Would you like to be on the team? You can pick up birthdays from the Unassigned list whenever you have time.'),
  );
  openModal('Join the Birthday Team?', [blurb], {
    saveLabel: 'Join the Team',
    submit: () => send('POST', '/api/birthday/team/join', {}),
    afterSave: () => toast('Welcome to the team!'),
    alternate: {label: 'No thanks', onClick: () => {
      location.href = appOrigin('home');
    }},
    hideCancel: true,
    required: true,
  });
}

export function openSettings() {
  const s = settings();
  const defaultCharity = select(charityOptions(s.defaultCharity), s.defaultCharity);
  const yearStart = text(s.yearStart, {required: true, placeholder: '08-14'});
  const subject = text(s.emailSubject, {required: true});
  const body = textarea(s.emailBody, 10);
  const note = textarea(s.noNewsletterNote, 3);
  const cc = text(s.outreachCC || '', {type: 'email', placeholder: 'hca@heliosschool.org'});
  const lead = text(String(s.requestLeadDays), {type: 'number', required: true});
  lead.min = 0;
  lead.max = 60;
  const dueBy = text(String(s.dueByLeadDays), {type: 'number', required: true});
  dueBy.min = 0;
  dueBy.max = 60;
  openModal('Settings', [
    field('Default charity', defaultCharity, 'Where a donation goes when nobody answers'),
    field('Year start', yearStart, 'Month and day the birthday year turns over, like 08-14'),
    field('Ask-by lead', lead, 'Days before the newsletter that the request is due'),
    field('Birthday due by', dueBy, 'Days before the newsletter that the charity must be in; after that the birthday is late'),
    field('Email subject', subject),
    field('Email body', body, 'Placeholders: {first name}, {name}, {birthday} (September 26), {newsletter date}, {default charity}, {sender}, {last year} (a heading, the charity and their note), {no newsletter note}'),
    field('No-newsletter note', note, 'Put where {no newsletter note} sits in the body, or at the end, for anyone who asked to stay out of the newsletter'),
    field('CC on outreach', cc, 'Copied on the outreach email a reminder hands over; blank for nobody'),
  ], {
    submit: () => send('POST', '/api/birthday/settings', {
      defaultCharity: defaultCharity.value, yearStart: yearStart.value, emailSubject: subject.value, emailBody: body.value, noNewsletterNote: note.value, outreachCC: cc.value, requestLeadDays: Number(lead.value), dueByLeadDays: Number(dueBy.value),
    }),
  });
}

export function charityNamed(name) {
  return charity(name);
}
