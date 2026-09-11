import {longDate, mediumDate, isUnassigned, staffPath, charityPath, staffFor, stageClass} from './state.js';
import {el, link, svg, thumb, button, menu} from './dom.js';
import {assignToMe, markContacted} from './edit.js';

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
    label.append(el('span', 'stage-chip ' + stageClass(sv.stage), sv.stage));
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
      actions.append(button('Mark: Contacted', 'check', 'button button-secondary button-small', () => markContacted(sv, true)));
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
  label.append(el('span', '', c.ein ? `EIN: ${c.ein}` : 'EIN: unknown'));
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

export function newsletterRow(date, items) {
  const row = el('div', 'row');
  const body = el('div', 'row-body');
  const people = staffFor(date);
  body.append(el('div', 'label', `${people.length} staff`), el('div', 'row-title', longDate(date)));
  if (people.length) {
    body.append(el('div', 'row-text', people.map(sv => sv.name.split(' ')[0]).join(', ')));
  }
  row.append(body);
  const actions = el('div', 'row-actions');
  for (const item of items) {
    actions.append(item);
  }
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
