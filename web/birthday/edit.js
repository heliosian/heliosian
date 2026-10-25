import {state, me, team, isAdmin, settings, charity, charityNamed, charityName, issueDates, newsletterOn, longDate, dateCell, parseDate, year, nowMoment, levelValues} from './state.js';
import {el, button, toast} from '/elements.js';
import {appOrigin} from '/appswitch.js';
import {api} from '/api.js';
import {openModal} from '/modal.js';
import {load, navigate} from '/router.js';
import {field, text, textarea, select, fillSelect, checkbox} from '/form.js';
import {known} from '/directory.js';
import {queries, write} from '/rail.js';

export async function run(work, message) {
  try {
    await work();
    await load();
    if (message) {
      toast(message);
    }
  } catch (err) {
    toast(err.message);
  }
}

function yearEdit(sv, cells) {
  if (sv.record) {
    return {set: sv.record, cells};
  }
  return {insert: 'BIRTHDAY_YEAR', row: {person: sv.id, year: year().number, ...cells}};
}

function assignTo(sv, person) {
  return write([yearEdit(sv, {assigned_to: person, assigned: nowMoment()})]);
}

function unassignWrite(sv) {
  return write([{set: sv.record, cells: {assigned_to: '', assigned: ''}}]);
}

export function assignToMe(sv) {
  return run(() => assignTo(sv, me().id), `${sv.name} is yours`);
}

export function openAssign(sv) {
  const people = team();
  if (sv.assignedToId && !people.some(p => p.id === sv.assignedToId)) {
    people.push({id: sv.assignedToId, name: sv.assignedToName});
  }
  const who = select(people.map(p => ({label: p.name, value: p.id})), sv.assignedToId || me().id);
  openModal(`Assign ${sv.name}`, [field('To', who)], {
    saveLabel: 'Assign',
    submit: () => assignTo(sv, who.value),
    onDelete: sv.assigned ? () => unassignWrite(sv) : null,
    deleteLabel: 'Unassign',
    confirmDelete: `Unassign ${sv.name}?`,
  });
}

export function markContacted(sv, contacted) {
  const cells = contacted ? {contacted: nowMoment(), contacted_by: me().id} : {contacted: '', contacted_by: ''};
  return run(() => write([yearEdit(sv, cells)]), contacted ? 'Marked as contacted' : 'Outreach reopened');
}

function usedCells(used) {
  return used ? {published: nowMoment(), published_by: me().id} : {published: '', published_by: ''};
}

export function markAllUsed(list) {
  return run(() => write(list.map(sv => ({set: sv.record, cells: usedCells(true)}))), `Marked ${list.length} as used`);
}

export function markUsed(sv, used) {
  return run(() => write([{set: sv.record, cells: usedCells(used)}]), used ? 'Marked as used in the newsletter' : 'Marked as not yet used');
}

function donate(sv, chosen, note) {
  return write([yearEdit(sv, {charity: chosen, note, recorded: nowMoment(), recorded_by: me().id})]);
}

export function useDefault(sv) {
  return run(() => donate(sv, sv.fallback.charity, sv.fallback.note), `Recorded ${charityName(sv.fallback.charity)}`);
}

export function reuseLast(sv) {
  return run(() => donate(sv, sv.lastDonation.charity, sv.lastDonation.note), `Recorded ${charityName(sv.lastDonation.charity)} again`);
}

function charityOptions(current) {
  const options = state.model.charities.filter(c => c.allowed || c.id === current).map(c => ({label: c.allowed ? c.name : `${c.name} (not allowed)`, value: c.id}));
  options.sort((a, b) => a.label.localeCompare(b.label));
  return options;
}

export function openDonation(sv) {
  const existing = sv.donation;
  const pick = select(charityOptions(existing ? existing.charity : ''), existing ? existing.charity : sv.fallback.charity);
  const note = textarea(existing ? existing.note : '', 5);
  const add = el('div', 'field-note');
  const addLink = el('button', 'link-button', 'Not on the list? Add a charity.');
  addLink.type = 'button';
  addLink.addEventListener('click', () => openCharity(null, {afterSave: name => {
    const added = charityNamed(name);
    fillSelect(pick, charityOptions(added.id), added.id);
    showBlurb();
  }}));
  add.append(addLink);
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
    submit: () => donate(sv, pick.value, note.value),
    onDelete: existing && !existing.usedOn ? () => write([{set: sv.record, cells: {charity: '', note: '', recorded: '', recorded_by: ''}}]) : null,
    deleteLabel: 'Remove',
    confirmDelete: existing ? `Remove ${sv.name}'s donation this year?` : '',
  });
}

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long'});
const months = Array.from({length: 12}, (_, i) => ({label: monthFormat.format(new Date(2000, i, 1)), value: String(i + 1).padStart(2, '0')}));
const days = Array.from({length: 31}, (_, i) => String(i + 1).padStart(2, '0'));

function personOf(sv, email) {
  if (sv.id) {
    return sv.id;
  }
  const p = known(email.value.trim().toLowerCase());
  if (!p) {
    throw new Error(`${email.value} is not in the directory`);
  }
  return p.id;
}

export function openBirthday(sv) {
  const email = text(sv.email || '', {type: 'email', required: true, placeholder: 'name@heliosschool.org'});
  const [currentMonth, currentDay] = (sv.birthday || '').split('-');
  const month = select(months, currentMonth || '01');
  const day = select(days, currentDay || '01');
  const birthday = el('div', 'field-pair');
  birthday.append(month, day);
  const fields = [];
  if (!sv.id) {
    fields.push(field('Email', email));
  }
  fields.push(field('Birthday', birthday));
  openModal(sv.birthday ? `Edit ${sv.name}'s birthday` : `Add ${sv.name ? `${sv.name}'s` : 'a'} birthday`, fields, {
    submit: () => {
      const born = (sv.birthdayCell || '2000').slice(0, 4);
      return write([{set: personOf(sv, email), cells: {birthday: `${born}-${month.value}-${day.value}`}}]);
    },
    onDelete: sv.birthday ? () => write([{set: sv.id, cells: {birthday: ''}}]) : null,
    deleteLabel: 'Remove birthday',
    confirmDelete: `Remove ${sv.name}'s birthday?`,
    afterDelete: () => navigate('/skipped'),
  });
}

export function openParticipation(sv) {
  const email = text(sv.email || '', {type: 'email', required: true, placeholder: 'name@heliosschool.org'});
  const level = select([
    {label: 'Skip: does not want to take part at all', value: 'Skip'},
    {label: 'No newsletter: ask and donate, but leave them out of the newsletter', value: 'No Newsletter'},
  ], sv.level || 'Skip');
  const fields = [];
  if (!sv.id) {
    fields.push(field('Email', email));
  }
  fields.push(field('Preference', level));
  openModal(sv.level ? `Edit ${sv.name}'s preference` : `Set ${sv.name ? `${sv.name}'s` : 'a'} preference`, fields, {
    submit: () => write([{set: personOf(sv, email), cells: {birthday_participation: levelValues[level.value]}}]),
    onDelete: sv.level ? () => write([{set: sv.id, cells: {birthday_participation: ''}}]) : null,
    deleteLabel: 'Clear preference',
    confirmDelete: `Clear ${sv.name}'s preference and treat them like everyone else?`,
  });
}

function describeCharity(name, url) {
  return api('POST', '/api/do/describe-charity', {name, url});
}

export function openCharity(c, options) {
  const admin = isAdmin();
  const name = text(c ? c.name : '', {required: true, maxLength: 120});
  const linkInput = text(c ? c.donationLink : '', {type: 'url', required: true, placeholder: 'https://'});
  const about = textarea(c ? c.about : '', 4);
  const allowed = checkbox('Allowed', c ? c.allowed : true);
  const why = text(c ? c.whyNotAllowed : '', {placeholder: 'Not a non-profit'});
  const whyField = field('Why not', why);
  whyField.hidden = allowed.input.checked;
  allowed.input.addEventListener('change', () => {
    whyField.hidden = allowed.input.checked;
  });
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
    for (const [heading, value] of [['Yours', have], ["Claude's", found]]) {
      const side = el('div', 'compare-side');
      side.append(el('div', 'compare-heading', heading), el('div', 'compare-text', value));
      const use = el('button', 'button button-secondary button-small', `Use ${heading === 'Yours' ? 'mine' : "Claude's"}`);
      use.type = 'button';
      use.addEventListener('click', () => {
        input.value = value;
        box.remove();
      });
      side.append(use);
      pair.append(side);
    }
    box.append(pair);
    input.parentNode.after(box);
    compares.push(box);
  };
  const suggestStatus = el('span', 'suggest-status');
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
      offer(linkInput, info.url, 'Donation link');
      offer(about, info.description, 'About');
      suggestStatus.textContent = compares.length ? 'Choose below, then read it over.' : 'Read it over and change anything that is off.';
    } catch (err) {
      suggestStatus.textContent = err.message;
    } finally {
      suggest.disabled = false;
    }
  });
  suggestRow.append(suggest, suggestStatus);
  const fields = [field('Name', name), suggestRow, field('Donation link', linkInput), field('About', about, 'A sentence for the newsletter')];
  if (admin) {
    fields.push(allowed.wrap, whyField);
  }
  openModal(c ? 'Edit Charity' : 'Add Charity', fields, {
    replace: !c,
    submit: async () => {
      const trimmed = name.value.trim();
      const other = charityNamed(trimmed);
      if (other && (!c || other.id !== c.id)) {
        throw new Error(`“${trimmed}” is already on the list`);
      }
      const cells = {name: trimmed, url: linkInput.value.trim(), description: about.value.trim()};
      if (admin) {
        cells.allowed = allowed.input.checked;
        cells.not_allowed_reason = allowed.input.checked ? '' : why.value.trim();
      }
      if (!c) {
        await write([{insert: 'CHARITY', row: {allowed: true, ...cells, added: nowMoment()}}]);
      } else {
        await write([{set: c.id, cells}]);
      }
      if (c && c.name !== trimmed && location.pathname.startsWith('/charities/')) {
        navigate(`/charities/${encodeURIComponent(trimmed)}`);
      }
      return trimmed;
    },
    afterSave: options && options.afterSave,
    onDelete: c && admin && !c.named && c.id !== settings().defaultCharity ? () => write([{delete: c.id}]) : null,
    confirmDelete: c ? `Delete “${c.name}”?` : '',
    afterDelete: () => navigate('/charities'),
  });
}

async function issueRows(dates) {
  const answer = await queries({series: `(from GROUP (where (= id "${settings().newsletter}")) (columns name visible_to posting replying))`});
  const series = answer.series.resources.GROUP[settings().newsletter];
  return dates.map(start => ({insert: 'GROUP', row: {
    kind: 'group', parent: series.id, name: series.name, status: 'open', listed: false, visible_to: series.visible_to,
    posting: series.posting, replying: series.replying, added_by: me().id, start, all_day: true,
  }}));
}

async function addIssues(dates) {
  await write(await issueRows(dates));
}

export function openNewsletterDate() {
  const dates = issueDates();
  const last = dates.length ? parseDate(dates[dates.length - 1]) : parseDate(year().start);
  last.setDate(last.getDate() + 7);
  const date = text(dateCell(last), {type: 'date', required: true});
  openModal('Add Newsletter Date', [field('Date', date)], {
    saveLabel: 'Add',
    submit: () => {
      if (newsletterOn(date.value)) {
        throw new Error(`${longDate(date.value)} is already on the list.`);
      }
      return addIssues([date.value]);
    },
  });
}

export function openChangeNewsletterDate(original) {
  const {id} = newsletterOn(original);
  const date = text(original, {type: 'date', required: true});
  openModal('Change Newsletter Date', [field('Date', date, 'Everyone announced in this issue moves with it')], {
    submit: () => write([{set: id, cells: {start: date.value}}]),
  });
}

export function addNextWeek(after) {
  const next = parseDate(after);
  next.setDate(next.getDate() + 7);
  return run(() => addIssues([dateCell(next)]), `Added ${longDate(dateCell(next))}`);
}

function weekly(weekday, from, to) {
  const out = [];
  const day = parseDate(from);
  day.setDate(day.getDate() + (weekday - day.getDay() + 7) % 7);
  for (const end = parseDate(to); day <= end; day.setDate(day.getDate() + 7)) {
    out.push(dateCell(day));
  }
  return out;
}

export function openCreateNewsletterDates() {
  const dates = issueDates();
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
      if (to.value < from.value) {
        throw new Error('The final date is before the first.');
      }
      const added = weekly(Number(weekday.value), from.value, to.value).filter(d => !newsletterOn(d));
      if (!added.length) {
        throw new Error('Every one of those dates is already on the list.');
      }
      await addIssues(added);
      toast(`Added ${added.length} ${added.length === 1 ? 'date' : 'dates'}`);
    },
  });
}

export function clearFutureNewsletterDates(future) {
  if (!confirm(`Remove the ${future.length} newsletter ${future.length === 1 ? 'date' : 'dates'} from today on? The ones already out stay.`)) {
    return;
  }
  return run(() => write(future.map(n => ({delete: n.id}))), `Removed ${future.length} ${future.length === 1 ? 'date' : 'dates'}`);
}

export function removeNewsletterDate(date) {
  if (!confirm(`Remove the ${longDate(date)} newsletter?`)) {
    return;
  }
  return run(() => write([{delete: newsletterOn(date).id}]));
}

export function offerTeam() {
  if (state.model.standing.team) {
    return;
  }
  const blurb = el('div', 'join-blurb');
  blurb.append(
    el('p', '', 'The birthday team celebrates every Helios staff member\'s birthday with a donation to a charity they choose, announced in the newsletter. Volunteers take a birthday each, reach out with a short email, and record the answer - a few minutes apiece.'),
    el('p', '', 'Would you like to be on the team? You can pick up birthdays from the Unassigned list whenever you have time.'),
  );
  openModal('Join the Birthday Team?', [blurb], {
    saveLabel: 'Join the Team',
    submit: () => write([{insert: 'MEMBER', row: {group: state.model.activity.id, person: me().id, member: 'yes', added_by: me().id}}]),
    afterSave: () => toast('Welcome to the team!'),
    alternate: {label: 'No thanks', onClick: () => {
      location.href = appOrigin('home');
    }},
    hideCancel: true,
    required: true,
  });
}

const settingKeys = {
  defaultCharity: 'Default Charity', yearStart: 'Year Start', emailSubject: 'Email Subject', emailBody: 'Email Body', noNewsletterNote: 'No Newsletter Note',
  outreachCC: 'Outreach CC', requestLeadDays: 'Request Lead Days', dueByLeadDays: 'Due By Lead Days',
};

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
    submit: () => {
      if (!/^\d{2}-\d{2}$/.test(yearStart.value.trim())) {
        throw new Error('Give the year start as a month and day, like 08-14.');
      }
      const values = {
        defaultCharity: defaultCharity.value, yearStart: yearStart.value.trim(), emailSubject: subject.value, emailBody: body.value, noNewsletterNote: note.value,
        outreachCC: cc.value.trim(), requestLeadDays: String(Number(lead.value)), dueByLeadDays: String(Number(dueBy.value)),
      };
      const edits = [];
      for (const [field, key] of Object.entries(settingKeys)) {
        const value = values[field];
        if (String(s[field]) === value) {
          continue;
        }
        const id = state.model.settingIds[key];
        edits.push(id ? {set: id, cells: {value}} : {insert: 'SETTING', row: {app: 'birthday', key, value}});
      }
      return edits.length ? write(edits) : Promise.resolve();
    },
  });
}
