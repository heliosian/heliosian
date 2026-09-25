import {state, isAdmin, year, staffFor, charity, longDate, mediumDate, monthDay, dateCell, parseDate, newsletterPath} from '../state.js';
import {el, link, svg, thumb, button, pageHead, menu, copyRich} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {staffRow, emptyPanel} from '../cards.js';
import {openNewsletterDate, openChangeNewsletterDate, addNextWeek, removeNewsletterDate, clearFutureNewsletterDates, openCreateNewsletterDates, markUsed, markAllUsed, toShare, shareIssue} from '../edit.js';

let query = '';

// Past issues are hidden until asked for, and the asking is remembered on
// this browser.
const pastKey = 'birthday.showPastIssues';

function showPast() {
  try {
    return localStorage.getItem(pastKey) === '1';
  } catch {
    return false;
  }
}

function setShowPast(on) {
  try {
    localStorage.setItem(pastKey, on ? '1' : '0');
  } catch {
    // Then it is forgotten on the next visit.
  }
}

// issueMenu is an admin's actions on an issue, moving or removing the date;
// nobody else gets a menu.
function issueMenu(date) {
  if (!isAdmin()) {
    return null;
  }
  return menu([
    {icon: 'edit', label: 'Change date', onClick: () => openChangeNewsletterDate(date)},
    {icon: 'trash', label: 'Remove', danger: true, onClick: () => removeNewsletterDate(date)},
  ]);
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
  const all = thisYear().filter(d => !query || longDate(d).toLowerCase().includes(query));
  const next = nextIssue();
  const dates = (showPast() ? all : all.filter(d => d >= state.model.today)).filter(d => d !== next);
  const root = el('div');
  if (!dates.length) {
    root.append(emptyPanel(query ? 'Nothing matches.' : all.length ? 'Nothing more this year.' : 'No newsletter dates this year yet.'));
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
    if (issueMenu(date)) {
      actions.push(issueMenu(date));
    }
    panel.append(issueRow(date, actions));
  });
  return root;
}

// entry is what Copy Info hands the newsletter writer about one staff
// member: the name and title, the birthday, the photo and its address, the charity,
// where to give, and what it does - and their own words when they left some.
// It comes as words and as a formatted twin for a paste into mail or a doc.
function entry(sv) {
  const c = charity(sv.donation.charity);
  const photo = sv.photoUrl ? new URL(sv.photoUrl, location.origin).href : '';
  const esc = t => String(t).replace(/[&<>"]/g, ch => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'}[ch]));
  const linked = v => /^https?:\/\//.test(v) ? `<a href="${esc(v)}">${esc(v)}</a>` : esc(v);
  // Who they are, then under a DONATION heading the charity, where to give,
  // what it does, and their own words when they left some.
  const who = [`Birthday: ${monthDay(sv.birthdayThisYear)}`, `Photo: ${photo || 'None on file'}`];
  const donation = [sv.donation.charity];
  if (c && c.donationLink) {
    donation.push(c.donationLink);
  }
  if (c && c.about) {
    donation.push(c.about);
  }
  if (sv.donation.note) {
    donation.push(`In their words: ${sv.donation.note}`);
  }
  const text = [sv.name, sv.jobTitle || '', ...who, '', 'DONATION', ...donation].filter((line, i) => line || i === who.length + 2).join('\n');
  let html = `<p><b>${esc(sv.name)}</b>${sv.jobTitle ? '<br>' + esc(sv.jobTitle) : ''}</p>`;
  if (photo) {
    html += `<p><img src="${esc(photo)}" alt="${esc(sv.name)}" width="160"></p>`;
  }
  html += '<p>' + who.map(linked).join('<br>') + '</p>';
  html += '<p><b>DONATION</b><br>' + donation.map(linked).join('<br>') + '</p>';
  return {text, html};
}

// nextCard is the next issue on its own, ahead of the list, so it is never
// lost among the rest.
function nextCard() {
  const next = nextIssue();
  const wrap = el('div', 'next-issue');
  if (!next) {
    return wrap;
  }
  wrap.append(el('div', 'section-title', 'Next issue'));
  const panel = el('div', 'panel issues next-panel');
  const actions = [];
  // Copy to Shared Sheet does by hand what Thursday night does: the issue's
  // birthdays to the Staff Birthday List (Shared), each marked done.
  const waiting = toShare(next).length;
  const share = button('Copy to Shared Sheet', 'send', 'button button-small', () => shareIssue(next));
  share.disabled = !waiting;
  share.title = waiting ? 'Thursday night at 11:59 does this on its own' : 'Everyone in this issue is copied and done';
  actions.push(share);
  const dates = thisYear();
  if (isAdmin() && next === dates[dates.length - 1]) {
    actions.push(button('Add Next Week', 'bolt', 'button button-secondary button-small', () => addNextWeek(next)));
  }
  if (issueMenu(next)) {
    actions.push(issueMenu(next));
  }
  panel.append(issueRow(next, actions));
  wrap.append(panel);
  return wrap;
}

// pastSwitch shows or hides the issues already out, just over the list.
function pastSwitch(onChange) {
  const past = thisYear().filter(d => d < state.model.today).length;
  const row = el('label', 'switch-row');
  if (!past) {
    row.hidden = true;
    return row;
  }
  const input = el('input');
  input.type = 'checkbox';
  input.checked = showPast();
  input.addEventListener('change', () => {
    setShowPast(input.checked);
    onChange();
  });
  const track = el('span', 'switch');
  track.append(el('span', 'switch-knob'));
  row.append(input, track, el('span', 'switch-label', `Show the ${past} ${past === 1 ? 'issue' : 'issues'} already out`));
  return row;
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
  body.append(title, el('div', 'summary-note', `${longDate(year().start)} to ${longDate(year().end)}. Each issue announces the birthdays between it and the next, so the word goes out before the day, never on it.`));
  band.append(icon, body);
  return band;
}

export function newslettersPage() {
  setTitle('Newsletters');
  const page = el('div', 'list-page');
  const actions = [];
  if (isAdmin()) {
    const future = state.model.newsletterDates.filter(d => d >= state.model.today).length;
    if (future) {
      actions.push(button('Clear Future Dates', 'trash', 'button button-secondary', () => clearFutureNewsletterDates(future)));
    }
    actions.push(button('Create Newsletters', 'calendar', 'button button-secondary', openCreateNewsletterDates));
    actions.push(button('Add', 'plus', 'button', openNewsletterDate));
  }
  query = '';
  setSearch('Search newsletter dates…', q => {
    query = q;
    page.querySelector('.list').replaceChildren(list());
  });
  const wrap = el('div', 'list');
  page.append(pageHead('Newsletter Dates', actions), overview(), nextCard(), pastSwitch(() => wrap.replaceChildren(list())));
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
  requestBy.setDate(requestBy.getDate() - state.model.settings.requestLeadDays);
  // Copy All is every recorded donation in the issue, ready for the newsletter.
  const recorded = people.filter(sv => sv.donation);
  const copyAll = button('Copy All', 'copy', 'button', () => {
    const entries = recorded.map(entry);
    copyRich(entries.map(e => e.text).join('\n\n'), entries.map(e => e.html).join('<hr>'), `Copied ${recorded.length} ${recorded.length === 1 ? 'entry' : 'entries'}`);
  });
  copyAll.disabled = !recorded.length;
  copyAll.title = recorded.length ? '' : 'No donations recorded yet';
  // Mark All Used says the issue carried every donation recorded and not yet marked.
  const unused = recorded.filter(sv => !sv.donation.usedOn);
  const markAll = button('Mark All Used', 'circlecheck', 'button button-secondary', () => {
    if (confirm(`Mark ${unused.length} ${unused.length === 1 ? 'donation' : 'donations'} as used in this issue?`)) {
      markAllUsed(unused);
    }
  });
  markAll.disabled = !unused.length;
  markAll.title = unused.length ? '' : recorded.length ? 'Every recorded donation is marked used' : 'No donations recorded yet';
  page.append(pageHead(longDate(date), [markAll, copyAll, issueMenu(date)].filter(Boolean)));
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
    const row = staffRow(sv, {stage: true, noActions: true, lines});
    // Copy Info is this one's entry - name, title, charity and note.
    const copy = button('Copy Info', 'copy', 'button button-secondary button-small', () => {
      const e = entry(sv);
      copyRich(e.text, e.html, `Copied ${sv.name}`);
    });
    copy.disabled = !sv.donation;
    copy.title = sv.donation ? '' : 'No donation recorded yet';
    const actions = row.querySelector('.row-actions');
    actions.prepend(copy);
    if (sv.donation) {
      const used = Boolean(sv.donation.usedOn);
      actions.prepend(button(used ? 'Mark Unused' : 'Mark Used', 'circlecheck', used ? 'button button-secondary button-small' : 'button button-small', () => markUsed(sv, !used)));
    }
    panel.append(row);
  }
  page.append(panel);
  return page;
}
