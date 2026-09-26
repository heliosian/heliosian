import {longDate, mediumDate, isUnassigned, staffPath, charityPath, stageClass, stageName, emailLink, urgency, urgencyWords} from './state.js';
import {el, link, svg, thumb, button, menu} from './dom.js';
import {assignToMe, markContacted, openDonation, useDefault, markUsed} from './edit.js';

export function staffRow(sv, options) {
  const opts = options || {};
  const row = link(staffPath(sv), 'row is-link');
  row.append(thumb(sv));
  const body = el('div', 'row-body');
  const label = el('div', 'label');
  if (opts.dateLabel && sv.birthdayThisYear) {
    label.append(el('span', '', longDate(sv.birthdayThisYear)));
  }
  if (sv.jobTitle) {
    label.append(el('span', '', sv.jobTitle));
  }
  if (opts.stage && sv.stage) {
    label.append(el('span', 'stage-chip ' + stageClass(sv.stage), stageName(sv.stage)));
  }
  // Late or due today, beside the stage, in the rail's red and amber.
  const due = urgency(sv);
  if (due.when) {
    label.append(el('span', 'urgency-chip is-' + due.when, urgencyWords(due)));
  }
  body.append(label, el('div', 'row-title', sv.name));
  const lines = [];
  if (opts.lines) {
    lines.push(...opts.lines);
  } else if (sv.birthdayThisYear) {
    lines.push(longDate(sv.birthdayThisYear));
  }
  for (const line of lines) {
    body.append(el('div', 'row-text', line));
  }
  row.append(body);
  const actions = el('div', 'row-actions');
  if (!opts.noActions) {
    if (isUnassigned(sv)) {
      actions.append(button('Assign to Me', 'bolt', 'button button-small', () => assignToMe(sv)));
    }
    if (sv.stage === 'Awaiting Outreach') {
      // The letter, addressed and written, in their mail app.
      const mail = el('a', 'button button-small');
      mail.href = emailLink(sv);
      mail.append(svg('mail'), el('span', '', 'Compose Email'));
      mail.addEventListener('click', e => e.stopPropagation());
      actions.append(mail, button('Mark Contacted', 'check', 'button button-secondary button-small', () => markContacted(sv, true)));
    }
    // Whatever the stage, the row offers its next step.
    if (sv.stage === 'Awaiting Response') {
      actions.append(button('Record Donation', 'gift', 'button button-small', () => openDonation(sv)));
      actions.append(button('Use Default', 'vault', 'button button-secondary button-small', () => useDefault(sv)));
    }
    if (sv.stage === 'Awaiting Newsletter') {
      actions.append(button('Mark Used', 'circlecheck', 'button button-secondary button-small', () => markUsed(sv, true)));
    }
  }
  if (opts.menu) {
    actions.append(menu(opts.menu));
  }
  const chevron = svg('chevron');
  chevron.classList.add('chevron');
  actions.append(chevron);
  row.append(actions);
  return row;
}

export function charityRow(c, items) {
  const row = link(charityPath(c), 'row is-link');
  const body = el('div', 'row-body');
  const label = el('div', 'label');
  if (!c.allowed) {
    label.append(el('span', 'need', c.whyNotAllowed || 'Not allowed'));
  }
  body.append(label, el('div', 'row-title', c.name));
  if (c.about) {
    body.append(el('div', 'row-text clamp', c.about));
  }
  row.append(body);
  const actions = el('div', 'row-actions');
  actions.append(menu(items));
  row.append(actions);
  return row;
}

export function emptyPanel(text) {
  const panel = el('div', 'panel');
  panel.append(el('div', 'panel-empty', text));
  return panel;
}

export function dateSummary(sv) {
  return `Request on ${mediumDate(sv.requestBy)} · Newsletter ${mediumDate(sv.newsletterDate)}`;
}
