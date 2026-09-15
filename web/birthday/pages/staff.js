import {me, isAdmin, settings, charity, longDate, mediumDate, emailLink, newsletterText} from '../state.js';
import {el, link, svg, thumb, button, iconButton, copyText} from '../dom.js';
import {setTitle} from '../chrome.js';
import {openAssign, markContacted, markUsed, useDefault, reuseLast, openDonation, openBirthday, openParticipation, openNote, removeNote} from '../edit.js';

function crumb(sv) {
  const nav = el('div', 'crumb');
  const back = link('/process', '');
  back.append(svg('back'));
  nav.append(back, link('/process', '', 'Process'), el('span', '', '/'), el('span', 'current', sv.name));
  return nav;
}

function header(sv) {
  const head = el('div', 'detail-head');
  head.append(thumb(sv, 'large'));
  const body = el('div', 'row-body');
  const label = el('div', 'label');
  if (sv.jobTitle) {
    label.append(el('span', '', sv.jobTitle));
  }
  if (sv.department) {
    label.append(el('span', '', sv.department));
  }
  if (sv.level) {
    label.append(el('span', 'need', sv.level === 'Skip' ? 'Opted out' : 'Not in the newsletter'));
  }
  if (!sv.inDirectory) {
    label.append(el('span', 'need', 'Not in the directory'));
  }
  body.append(label, el('h1', '', sv.name));
  const mail = el('a', 'row-text detail-mail');
  mail.href = `mailto:${sv.email}`;
  mail.append(svg('mail'), el('span', '', sv.email));
  body.append(mail);
  head.append(body);
  const actions = el('div', 'detail-actions');
  actions.append(iconButton('user', sv.assignedTo ? 'Change assignment' : 'Assign', 'filled', () => openAssign(sv)));
  actions.append(iconButton('edit', 'Edit birthday', '', () => openBirthday(sv)));
  head.append(actions);
  return head;
}

// fact is one of the three date tiles: an icon in a pale disc beside its label and value.
function fact(icon, label, value) {
  const wrap = el('div', 'fact');
  const disc = el('div', 'fact-icon');
  disc.append(svg(icon));
  const body = el('div');
  body.append(el('div', 'fact-label', label), el('div', 'fact-value', value));
  wrap.append(disc, body);
  return wrap;
}

function facts(sv) {
  const row = el('div', 'facts');
  row.append(fact('cake', 'Birthday', longDate(sv.birthdayThisYear)));
  row.append(fact('doc', 'Request By', sv.requestBy ? mediumDate(sv.requestBy) : 'No newsletter date'));
  row.append(fact('send', 'Newsletter Date', sv.newsletterDate ? longDate(sv.newsletterDate) + (sv.override ? ' (set by hand)' : '') : 'None this year'));
  return row;
}

// step is one row of the workflow: its number in a disc on the timeline - ticked in teal when done, teal
// when it is the step in hand, amber when it wants someone's attention, pale while it waits its turn -
// then the title, the text with an optional aside in lighter ink, and the actions.
function step(n, title, text, actions, status, aside) {
  const row = el('div', 'step step-' + status);
  const disc = el('div', 'step-num');
  if (status === 'done') {
    disc.append(svg('check'));
  } else {
    disc.textContent = String(n);
  }
  row.append(disc);
  const body = el('div', 'step-body');
  const line = el('div', 'step-text', text);
  if (aside) {
    line.append(el('span', 'step-aside', aside));
  }
  body.append(el('div', 'step-title', title), line);
  row.append(body);
  const wrap = el('div', 'step-actions');
  for (const a of actions) {
    wrap.append(a);
  }
  row.append(wrap);
  return row;
}

function outlined(label, icon, onClick) {
  return button(label, icon, 'button button-secondary', onClick);
}

function filled(label, icon, onClick) {
  return button(label, icon, 'button', onClick);
}

// stepStatus places a step against the stage: every step before the stage's own is done, the stage's is
// in hand, and the rest wait. Assignment stands apart: done once someone has it, wanting attention until.
function stepStatus(sv, n) {
  const current = {'Wait': 2, 'Awaiting Outreach': 3, 'Awaiting Response': 4, 'Awaiting Newsletter': 5, 'Complete': 6}[sv.stage] || 6;
  if (n === 1) {
    return sv.assignedTo ? 'done' : 'attention';
  }
  return n < current ? 'done' : n === current ? 'current' : 'pending';
}

// status is the pill in the workflow's header: where the person stands, in a word.
function status(sv) {
  const pill = el('div', 'workflow-status');
  const word = sv.stage === 'Complete' ? 'Complete' : sv.stage === 'Wait' ? 'Waiting' : 'In Progress';
  pill.append(el('span', 'status-dot ' + word.toLowerCase().replace(' ', '-')), el('span', '', word));
  return pill;
}

function steps(sv) {
  const card = el('div', 'workflow');
  const head = el('div', 'workflow-head');
  head.append(el('h2', '', 'Donation Workflow'), status(sv));
  card.append(head);
  const inner = el('div', 'workflow-steps');
  const assignActions = [sv.assignedTo ? outlined('Change', 'user', () => openAssign(sv)) : filled('Assign', 'user', () => openAssign(sv))];
  inner.append(step(1, 'Assignment', sv.assignedTo ? `Assigned to ${sv.assignedToName} (${sv.assignedTo})` : 'Unassigned', assignActions, stepStatus(sv, 1)));
  inner.append(step(2, 'Wait', sv.requestBy ? `Wait until ${mediumDate(sv.requestBy)} to request` : 'No newsletter falls in this birthday year yet',
    [outlined('Change', null, () => openBirthday(sv))], stepStatus(sv, 2), sv.requestBy ? `(for the ${longDate(sv.newsletterDate)} newsletter)` : ''));
  const mail = el('a', 'button button-secondary');
  mail.href = emailLink(sv);
  mail.append(svg('mail'), el('span', '', 'Compose Email'));
  inner.append(step(3, 'Outreach', sv.contactedOn ? `Contacted on ${mediumDate(sv.contactedOn)} by ${sv.contactedBy}` : `Contact on ${mediumDate(sv.requestBy)}`,
    [mail, outlined(sv.contactedOn ? 'Mark Not Done' : 'Mark Done', 'circlecheck', () => markContacted(sv, !sv.contactedOn))], stepStatus(sv, 3)));
  const donationActions = sv.donation
    ? [outlined('Edit', 'edit', () => openDonation(sv))]
    : [outlined('Add Donation', 'gift', () => openDonation(sv)), outlined('Use Default', 'vault', () => useDefault(sv))];
  inner.append(step(4, 'Record Response', sv.donation ? `Selected ${sv.donation.charity} on ${mediumDate(sv.donation.recordedOn)}` : 'No donation selected', donationActions, stepStatus(sv, 4)));
  if (sv.level === 'No Newsletter') {
    inner.append(step(5, 'Newsletter', 'Not in the newsletter, by request', [], stepStatus(sv, 5)));
  } else {
    const used = sv.donation && sv.donation.usedOn;
    inner.append(step(5, used ? 'Include in Newsletter' : 'Awaiting Newsletter',
      used ? `Used by ${sv.donation.usedBy} on ${mediumDate(sv.donation.usedOn)}` : `Intended for ${longDate(sv.newsletterDate) || 'a newsletter'}`,
      [outlined(used ? 'Mark Unused' : 'Mark Used', 'circlecheck', () => markUsed(sv, !used))], stepStatus(sv, 5)));
  }
  card.append(inner);
  return card;
}

function askBand(sv) {
  const band = el('div', 'ask-band');
  band.append(el('div', 'ask-icon', '🎁'));
  const body = el('div', 'row-body');
  body.append(el('div', 'row-title', 'Select Donation'));
  const last = sv.lastDonation ? sv.lastDonation.charity : '';
  body.append(el('div', 'row-text', `If no donation is specified, we will default to either last year's donation${last ? ` (${last})` : ''} or the default (${settings().defaultCharity}).`));
  band.append(body);
  const actions = el('div', 'row-actions');
  actions.append(filled('Add Donation', null, () => openDonation(sv)));
  const useDefaultButton = outlined('Use Default Donation', null, () => useDefault(sv));
  useDefaultButton.disabled = Boolean(sv.donation);
  actions.append(useDefaultButton);
  band.append(actions);
  return band;
}

// sprig is the little plant that stands in for a donation not yet chosen.
function sprig(className) {
  const node = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  node.setAttribute('viewBox', '0 0 64 64');
  node.setAttribute('class', className);
  node.innerHTML = '<path d="M32 60V26" stroke="currentColor" stroke-width="3" stroke-linecap="round" fill="none"/>'
    + '<path d="M32 30c-2-12 4-22 16-24 0 12-6 22-16 24z" fill="currentColor"/>'
    + '<path d="M32 42c2-10-4-18-14-19 0 10 6 18 14 19z" fill="currentColor" opacity="0.7"/>';
  return node;
}

function donationColumn(title, tint, sv, donation, actions) {
  const col = el('div', 'year-card year-' + tint);
  col.append(el('h3', '', title));
  if (!donation) {
    const empty = el('div', 'year-empty');
    empty.append(sprig('sprig'), el('div', 'year-empty-text', 'No donation selected yet'), outlined('Add Donation', 'plus', () => openDonation(sv)));
    col.append(empty, sprig('sprig-corner'));
    return col;
  }
  const c = charity(donation.charity);
  const card = el('div', 'donation-card');
  const text = el('div', 'donation-text');
  text.append(el('div', 'donation-name', donation.charity));
  if (donation.note) {
    text.append(el('div', 'donation-note', donation.note));
  }
  card.append(text);
  const icons = el('div', 'donation-icons');
  icons.append(iconButton('copy', 'Copy for the newsletter', 'boxed', () => copyText(newsletterText(sv, donation), 'Copied for the newsletter')));
  const open = iconButton('open', 'Open the donation page', 'boxed', () => window.open(c ? c.donationLink : '', '_blank', 'noopener'));
  open.disabled = !c;
  icons.append(open);
  card.append(icons);
  col.append(card);
  const wrap = el('div', 'year-actions');
  for (const a of actions) {
    wrap.append(a);
  }
  col.append(wrap);
  return col;
}

function donationBand(sv) {
  const cols = el('div', 'year-cards');
  cols.append(donationColumn('This Year', 'mint', sv, sv.donation, sv.donation ? [outlined('Edit Donation', 'edit', () => openDonation(sv))] : []));
  if (sv.lastDonation) {
    const reuse = outlined('Re-Use Donation', 'reuse', () => reuseLast(sv));
    reuse.disabled = Boolean(sv.donation && sv.donation.charity === sv.lastDonation.charity);
    cols.append(donationColumn('Last Year', 'sky', sv, sv.lastDonation, [reuse]));
  } else {
    const d = settings().defaultCharity;
    const useDefaultButton = outlined('Use Default Charity', 'reuse', () => useDefault(sv));
    useDefaultButton.disabled = Boolean(sv.donation && sv.donation.charity === d);
    const c = charity(d);
    cols.append(donationColumn('Default Donation', 'sky', sv, {charity: d, note: c ? c.about : ''}, [useDefaultButton]));
  }
  return cols;
}

function notes(sv) {
  const wrap = el('div', 'notes');
  for (const n of sv.notes) {
    const row = el('div', 'note');
    row.append(el('div', 'note-text', n.note), el('div', 'note-meta', `${n.addedBy} · ${mediumDate(n.added)}`));
    if (n.addedBy === me().email || isAdmin()) {
      row.append(iconButton('trash', 'Remove note', '', () => removeNote(n)));
    }
    wrap.append(row);
  }
  const actions = el('div', 'center-actions');
  actions.append(button('Add Note', 'plus', 'button button-secondary', () => openNote(sv)));
  actions.append(button(sv.level ? 'Edit Preference' : 'Set Preference', 'skipped', 'button button-secondary', () => openParticipation(sv)));
  wrap.append(actions);
  return wrap;
}

export function staffPage(sv) {
  setTitle(sv.name);
  const page = el('div', 'staff-page');
  page.append(crumb(sv), header(sv));
  if (!sv.birthday) {
    const band = el('div', 'ask-band');
    band.append(el('div', 'ask-icon', '🎂'));
    const body = el('div', 'row-body');
    body.append(el('div', 'row-title', 'No birthday on file'), el('div', 'row-text', sv.level === 'Skip' ? `Opted out${sv.levelNote ? `: ${sv.levelNote}` : ''}.` : 'Add their birthday to bring them into the process.'));
    band.append(body);
    const actions = el('div', 'row-actions');
    actions.append(filled('Add Birthday', 'plus', () => openBirthday(sv)));
    actions.append(outlined(sv.level ? 'Edit Preference' : 'Set Preference', 'skipped', () => openParticipation(sv)));
    band.append(actions);
    page.append(band);
    return page;
  }
  page.append(facts(sv));
  if (sv.level === 'Skip') {
    const band = el('div', 'ask-band');
    band.append(el('div', 'ask-icon', '🙅'));
    const body = el('div', 'row-body');
    body.append(el('div', 'row-title', 'Opted out'), el('div', 'row-text', sv.levelNote || 'Asked not to take part.'));
    band.append(body);
    const actions = el('div', 'row-actions');
    actions.append(outlined('Edit Preference', 'skipped', () => openParticipation(sv)));
    band.append(actions);
    page.append(band);
    return page;
  }
  page.append(steps(sv), askBand(sv), donationBand(sv), notes(sv));
  return page;
}
