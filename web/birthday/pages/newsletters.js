import {state, isAdmin, year, staffFor, newsletterText, longDate, mediumDate, dateCell, parseDate, newsletterPath} from '../state.js';
import {el, link, svg, thumb, button, pageHead, menu, copyText} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {staffRow, emptyPanel} from '../cards.js';
import {openNewsletterDate, openChangeNewsletterDate, addNextWeek, removeNewsletterDate} from '../edit.js';

let query = '';

function copyIssue(date) {
  const lines = staffFor(date).filter(sv => sv.donation).map(sv => newsletterText(sv, sv.donation));
  if (!lines.length) {
    return copyText('', 'No donations recorded for this issue yet');
  }
  return copyText(lines.join('\n\n'), 'Copied the issue');
}

// issueMenu is the row's and the issue's own actions: copy for anyone, and for
// an admin, moving or removing the date.
function issueMenu(date) {
  const items = [{icon: 'copy', label: 'Copy this issue', onClick: () => copyIssue(date)}];
  if (isAdmin()) {
    items.push({icon: 'edit', label: 'Change date', onClick: () => openChangeNewsletterDate(date)});
    items.push({icon: 'trash', label: 'Remove', danger: true, onClick: () => removeNewsletterDate(date)});
  }
  return menu(items);
}

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'long', year: 'numeric'});
const badgeMonth = new Intl.DateTimeFormat('en-US', {month: 'short'});
const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric'});

function thisYear() {
  return state.model.newsletterDates.filter(d => d >= year().start && d <= year().end);
}

// nextIssue is the first issue on or after today, the one the team is working towards.
function nextIssue() {
  return thisYear().find(d => d >= state.model.today) || '';
}

// issueRow is one issue: its day on a badge, the date, who is announced in it
// as small faces, and its count, ringed when it is the next one.
function issueRow(date, actions) {
  const row = link(newsletterPath(date), 'issue' + (date < state.model.today ? ' is-past' : '') + (date === nextIssue() ? ' is-next' : ''));
  const day = parseDate(date);
  const badge = el('div', 'issue-badge');
  badge.append(el('div', 'issue-badge-month', badgeMonth.format(day)), el('div', 'issue-badge-day', String(day.getDate())));
  row.append(badge);
  const body = el('div', 'issue-body');
  const title = el('div', 'issue-title', dayFormat.format(day));
  if (date === nextIssue()) {
    title.append(el('span', 'issue-next', 'Next issue'));
  }
  body.append(title);
  const people = staffFor(date);
  const faces = el('div', 'issue-people');
  if (!people.length) {
    faces.append(el('span', 'issue-none', 'No birthdays in this issue'));
  }
  for (const sv of people) {
    const face = el('span', 'issue-person');
    face.append(thumb(sv, 'tiny'), el('span', '', sv.name.split(' ')[0]));
    faces.append(face);
  }
  body.append(faces);
  row.append(body);
  const side = el('div', 'issue-side');
  side.append(el('span', 'issue-count' + (people.length ? '' : ' is-empty'), `${people.length} ${people.length === 1 ? 'staff' : 'staff'}`));
  for (const a of actions) {
    side.append(a);
  }
  const chevron = svg('chevron');
  chevron.classList.add('chevron');
  side.append(chevron);
  row.append(side);
  return row;
}

function list() {
  const dates = thisYear().filter(d => !query || longDate(d).toLowerCase().includes(query));
  const root = el('div');
  if (!dates.length) {
    root.append(emptyPanel(query ? 'Nothing matches.' : 'No newsletter dates this year yet.'));
    return root;
  }
  let month = '';
  let panel = null;
  dates.forEach((date, i) => {
    const m = monthFormat.format(parseDate(date));
    if (m !== month) {
      month = m;
      root.append(el('div', 'section-title', m));
      panel = el('div', 'panel issues');
      root.append(panel);
    }
    const actions = [];
    if (isAdmin() && i === dates.length - 1) {
      actions.push(button('Add Next Week', 'bolt', 'button button-secondary button-small', () => addNextWeek(date)));
    }
    actions.push(issueMenu(date));
    panel.append(issueRow(date, actions));
  });
  return root;
}

// overview is the band under the title: the year, the count of issues, and
// the next issue with who is in it.
function overview() {
  const band = el('div', 'stage-summary tint-teal');
  const icon = el('div', 'summary-icon');
  icon.append(svg('newsletter'));
  const body = el('div', 'summary-body');
  const title = el('div', 'summary-title');
  const issues = thisYear();
  title.append(el('strong', '', `${year().current} birthday year`), el('span', '', ` · ${issues.length} ${issues.length === 1 ? 'issue' : 'issues'}`));
  body.append(title, el('div', 'summary-note', `${longDate(year().start)} to ${longDate(year().end)}. A birthday lands in the first newsletter on or after it, or the last one of the year for a summer birthday.`));
  band.append(icon, body);
  const next = nextIssue();
  if (next) {
    const people = staffFor(next);
    const side = el('div', 'summary-next');
    const cal = el('div', 'summary-next-icon');
    cal.append(svg('send'));
    const text = el('div');
    text.append(el('div', 'summary-next-label', 'Next issue'), el('div', 'summary-next-date', dayFormat.format(parseDate(next))),
      el('div', 'summary-next-who', people.length ? people.map(sv => sv.name.split(' ')[0]).join(', ') : 'No birthdays'));
    side.append(cal, text);
    band.append(side);
  }
  return band;
}

export function newslettersPage() {
  setTitle('Newsletters');
  const page = el('div', 'list-page');
  const actions = [];
  if (isAdmin()) {
    actions.push(button('Add', 'plus', 'button', openNewsletterDate));
  }
  query = '';
  setSearch('Search newsletter dates…', q => {
    query = q;
    page.querySelector('.list').replaceChildren(list());
  });
  page.append(pageHead('Newsletter Dates', actions), overview());
  const wrap = el('div', 'list');
  wrap.append(list());
  page.append(wrap);
  return page;
}

// newsletterPage is one issue: who is announced in it, each with their stage
// and what has been recorded, so the team can see what the issue still needs.
export function newsletterPage(date) {
  setTitle(longDate(date));
  const page = el('div', 'list-page');
  const nav = el('div', 'crumb');
  const back = link('/newsletters', '');
  back.append(svg('back'));
  nav.append(back, link('/newsletters', '', 'Newsletters'), el('span', '', '/'), el('span', 'current', longDate(date)));
  page.append(nav);
  const people = staffFor(date);
  const requestBy = parseDate(date);
  requestBy.setDate(requestBy.getDate() - 10);
  page.append(pageHead(longDate(date), [button('Copy this issue', 'copy', 'button button-secondary', () => copyIssue(date)), issueMenu(date)]));
  page.append(el('div', 'section-note', `${people.length} staff ${people.length === 1 ? 'birthday is' : 'birthdays are'} announced in this issue. Requests go out by ${mediumDate(dateCell(requestBy))}.`));
  if (!people.length) {
    page.append(emptyPanel('No birthdays land in this issue.'));
    return page;
  }
  const panel = el('div', 'panel');
  for (const sv of people) {
    const lines = [`Birthday ${longDate(sv.birthdayThisYear)}`];
    if (sv.donation) {
      lines.push(`${sv.donation.usedOn ? 'Used' : 'Chose'} ${sv.donation.charity}`);
    } else if (sv.contactedOn) {
      lines.push(`Asked on ${mediumDate(sv.contactedOn)}, awaiting their answer`);
    } else {
      lines.push(sv.assignedTo ? `${sv.assignedToName} to ask by ${mediumDate(sv.requestBy)}` : 'Unassigned');
    }
    panel.append(staffRow(sv, {stage: true, lines}));
  }
  page.append(panel);
  return page;
}
