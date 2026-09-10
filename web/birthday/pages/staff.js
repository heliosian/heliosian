import {me, settings, charity, longDate, mediumDate, emailLink, newsletterText, isUnassigned} from '../state.js';
import {el, link, svg, thumb, button, iconButton, copyText} from '../dom.js';
import {setTitle} from '../chrome.js';
import {assignToMe, openAssign, markContacted, markUsed, useDefault, reuseLast, openDonation, openBirthday, openParticipation, openNote, removeNote} from '../edit.js';

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
  const mail = el('a', 'row-text', sv.email);
  mail.href = `mailto:${sv.email}`;
  body.append(mail);
  head.append(body);
  const actions = el('div', 'detail-actions');
  actions.append(iconButton('user', sv.assignedTo ? 'Change assignment' : 'Assign', 'filled', () => openAssign(sv)));
  actions.append(iconButton('edit', 'Edit birthday', '', () => openBirthday(sv)));
  head.append(actions);
  return head;
}

function fact(label, value) {
  const wrap = el('div', 'fact');
  wrap.append(el('div', 'fact-label', label), el('div', 'fact-value', value));
  return wrap;
}

function facts(sv) {
  const row = el('div', 'facts');
  row.append(fact('Birthday', longDate(sv.birthdayThisYear)));
  row.append(fact('Request By', sv.requestBy ? mediumDate(sv.requestBy) : 'No newsletter date'));
  row.append(fact('Newsletter Date', sv.newsletterDate ? longDate(sv.newsletterDate) + (sv.override ? ' (set by hand)' : '') : 'None this year'));
  return row;
}

function step(title, text, actions) {
  const row = el('div', 'step');
  const body = el('div', 'step-body');
  body.append(el('div', 'step-title', title), el('div', 'step-text', text));
  row.append(body);
  const wrap = el('div', 'step-actions');
  for (const a of actions) {
    wrap.append(a);
  }
  row.append(wrap);
  return row;
}

function dark(label, onClick) {
  return button(label, null, 'button button-dark button-small', onClick);
}

function steps(sv) {
  const band = el('div', 'steps-band');
  const inner = el('div', 'band-inner');
  const assignActions = [];
  if (isUnassigned(sv) || !sv.assignedTo) {
    assignActions.push(dark('Assign to Me', () => assignToMe(sv)));
  } else {
    assignActions.push(dark('Unassign', () => openAssign(sv)));
  }
  assignActions.push(dark('Change', () => openAssign(sv)));
  inner.append(step('Step 1: Assignment', sv.assignedTo ? `Assigned to ${sv.assignedToName} (${sv.assignedTo})` : 'Unassigned', assignActions));
  inner.append(step('Step 2: Wait', sv.requestBy ? `Wait until ${mediumDate(sv.requestBy)} to request (for the ${longDate(sv.newsletterDate)} newsletter)` : 'No newsletter falls in this birthday year yet', [dark('Change', () => openBirthday(sv))]));
  const mail = el('a', 'button button-dark button-small');
  mail.href = emailLink(sv);
  mail.append(svg('mail'), el('span', '', 'Email'));
  inner.append(step('Step 3: Outreach', sv.contactedOn ? `Contacted on ${mediumDate(sv.contactedOn)} by ${sv.contactedBy}` : `Contact on ${mediumDate(sv.requestBy)}`,
    [mail, dark(sv.contactedOn ? 'Mark Not Done' : 'Mark Done', () => markContacted(sv, !sv.contactedOn))]));
  const donationActions = sv.donation
    ? [dark('Edit', () => openDonation(sv))]
    : [dark('Add Donation', () => openDonation(sv)), dark('Use Default', () => useDefault(sv))];
  inner.append(step('Step 4: Record Response', sv.donation ? `Selected ${sv.donation.charity} on ${mediumDate(sv.donation.recordedOn)}` : 'No donation selected', donationActions));
  if (sv.level === 'No Newsletter') {
    inner.append(step('Step 5: Newsletter', 'Not in the newsletter, by request', []));
  } else {
    const used = sv.donation && sv.donation.usedOn;
    inner.append(step(used ? 'Step 5: Include in Newsletter' : 'Step 5: Awaiting Newsletter',
      used ? `Used by ${sv.donation.usedBy} on ${mediumDate(sv.donation.usedOn)}` : `Intended for ${longDate(sv.newsletterDate) || 'a newsletter'}`,
      [dark(used ? 'Mark Unused' : 'Mark Used', () => markUsed(sv, !used))]));
  }
  band.append(inner);
  return band;
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
  actions.append(button('Add Donation', null, 'button button-secondary button-small', () => openDonation(sv)));
  const useDefaultButton = button('Use Default Donation', null, 'button button-secondary button-small', () => useDefault(sv));
  useDefaultButton.disabled = Boolean(sv.donation);
  actions.append(useDefaultButton);
  band.append(actions);
  return band;
}

function donationColumn(title, sv, donation, actions) {
  const col = el('div', 'donation-col');
  col.append(el('h3', '', title));
  if (!donation) {
    col.append(button('Add Donation', 'plus', 'button button-dark', () => openDonation(sv)));
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
  icons.append(iconButton('copy', 'Copy for the newsletter', 'round', () => copyText(newsletterText(sv, donation), 'Copied for the newsletter')));
  const open = iconButton('open', 'Open the donation page', 'round', () => window.open(c ? c.donationLink : '', '_blank', 'noopener'));
  open.disabled = !c;
  icons.append(open);
  card.append(icons);
  col.append(card);
  for (const a of actions) {
    col.append(a);
  }
  return col;
}

function donationBand(sv) {
  const band = el('div', 'donation-band');
  const cols = el('div', 'donation-cols');
  cols.append(donationColumn('This Year', sv, sv.donation, sv.donation ? [button('Edit Donation', 'edit', 'button button-dark', () => openDonation(sv))] : []));
  if (sv.lastDonation) {
    const reuse = button('Re-Use Donation', 'reuse', 'button button-dark', () => reuseLast(sv));
    reuse.disabled = Boolean(sv.donation && sv.donation.charity === sv.lastDonation.charity);
    cols.append(donationColumn('Last Year', sv, sv.lastDonation, [reuse]));
  } else {
    const d = settings().defaultCharity;
    const useDefaultButton = button('Use Default Charity', 'reuse', 'button button-dark', () => useDefault(sv));
    useDefaultButton.disabled = Boolean(sv.donation && sv.donation.charity === d);
    const c = charity(d);
    cols.append(donationColumn('Default Donation', sv, {charity: d, note: c ? c.about : ''}, [useDefaultButton]));
  }
  band.append(cols);
  return band;
}

function notes(sv) {
  const wrap = el('div', 'notes');
  for (const n of sv.notes) {
    const row = el('div', 'note');
    row.append(el('div', 'note-text', n.note), el('div', 'note-meta', `${n.addedBy} · ${mediumDate(n.added)}`));
    if (n.addedBy === me().email || me().isAdmin) {
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
  const page = el('div');
  page.append(crumb(sv), header(sv));
  if (!sv.birthday) {
    const band = el('div', 'ask-band');
    band.append(el('div', 'ask-icon', '🎂'));
    const body = el('div', 'row-body');
    body.append(el('div', 'row-title', 'No birthday on file'), el('div', 'row-text', sv.level === 'Skip' ? `Opted out${sv.levelNote ? `: ${sv.levelNote}` : ''}.` : 'Add their birthday to bring them into the process.'));
    band.append(body);
    const actions = el('div', 'row-actions');
    actions.append(button('Add Birthday', 'plus', 'button button-small', () => openBirthday(sv)));
    actions.append(button(sv.level ? 'Edit Preference' : 'Set Preference', 'skipped', 'button button-secondary button-small', () => openParticipation(sv)));
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
    actions.append(button('Edit Preference', 'skipped', 'button button-secondary button-small', () => openParticipation(sv)));
    band.append(actions);
    page.append(band);
    return page;
  }
  page.append(steps(sv), askBand(sv), donationBand(sv), notes(sv));
  return page;
}
