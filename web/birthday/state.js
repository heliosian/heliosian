import {batch, me as whoAmI} from '/data.js';

export const state = {model: null};

const byHandle = new Map();

function staffView(read, b) {
  const person = read.follow(b, 'person') || {};
  const assignee = read.follow(b, 'assignee');
  return {
    ...b,
    name: person.fullName || b.email,
    jobTitle: person.jobTitle || '',
    department: person.department || '',
    photoUrl: person.photoUrl || '',
    assignedTo: assignee ? assignee.email || '' : b.assignedTo || '',
    assignedToName: assignee ? assignee.fullName : b.assignedTo || '',
    donation: read.follow(b, 'donation'),
    lastDonation: read.follow(b, 'last-donation'),
    fallback: {charity: read.follow(b, 'fallback-charity').id, note: b.fallbackNote || ''},
    notes: read.follow(b, 'notes'),
  };
}

function teamMember(read, m) {
  const person = read.follow(m, 'person');
  return {...m, email: person ? person.email : m.email, name: person ? person.fullName : m.email};
}

const byName = (a, b) => a.name.localeCompare(b.name);

export async function loadModel() {
  const [read, viewer] = await Promise.all([batch({
    settings: '/api/birthday-settings?include=viewer',
    birthdays: '/api/birthdays?include=person,assignee,donation,last-donation,fallback-charity,notes',
    charities: '/api/charities',
    newsletterDates: '/api/newsletter-dates',
    team: '/api/birthday-team?include=person',
    departments: '/api/departments',
  }), whoAmI()]);
  const s = read.get(read.result.settings[0]);
  const person = read.follow(s, 'viewer') || {};
  const name = person.fullName || viewer.email;
  const everyone = read.result.birthdays.map(id => staffView(read, read.get(id)));
  const staff = everyone.filter(sv => !sv.missing && sv.level !== 'Skip');
  staff.sort((a, b) => (a.birthdayThisYear || '').localeCompare(b.birthdayThisYear || '') || byName(a, b));
  state.model = {
    user: {email: viewer.email, name, initial: name[0].toUpperCase(), photoUrl: person.photoUrl},
    allowances: viewer.allowances,
    settingsId: s.id,
    standing: s.me,
    settings: s.settings,
    year: s.year,
    today: read.now.slice(0, 10),
    staff,
    skipped: everyone.filter(sv => sv.level === 'Skip').sort(byName),
    missing: everyone.filter(sv => sv.missing && sv.level !== 'Skip').sort(byName),
    charities: read.result.charities.map(read.get),
    newsletterDates: read.result.newsletterDates.map(read.get),
    team: read.result.team.map(id => teamMember(read, read.get(id))),
    departments: read.result.departments.map(id => read.get(id).name),
  };
  byHandle.clear();
  for (const sv of everyone) {
    byHandle.set(sv.path, sv);
    byHandle.set(`/staff/${sv.email}`, sv);
  }
}

export function me() {
  return state.model.user;
}

export function isAdmin() {
  return state.model.allowances.includes('birthday.configure');
}

export function year() {
  return state.model.year;
}

export function settings() {
  return state.model.settings;
}

export function staff(handle) {
  return byHandle.get(`/staff/${decodeURIComponent(handle)}`) || null;
}

export function charity(id) {
  return state.model.charities.find(c => c.id === id) || null;
}

export function charityNamed(name) {
  return state.model.charities.find(c => c.name === name) || null;
}

export function charityName(id) {
  return charity(id).name;
}

export function issueDates() {
  return state.model.newsletterDates.map(n => n.date);
}

export function newsletterOn(date) {
  return state.model.newsletterDates.find(n => n.date === date) || null;
}

export const stages = ['Wait', 'Awaiting Outreach', 'Awaiting Response', 'Awaiting Newsletter', 'Complete'];

const stageNumbers = {'Wait': 2, 'Awaiting Outreach': 3, 'Awaiting Response': 4, 'Awaiting Newsletter': 5, 'Complete': 6};

export function stageLabel(stage) {
  return `${stageNumbers[stage]}. ${stage}`;
}

const stageNames = {'Wait': 'Scheduled', 'Awaiting Outreach': 'Ready to Contact', 'Awaiting Response': 'Waiting for Reply', 'Awaiting Newsletter': 'Ready for Newsletter', 'Complete': 'Complete'};

export function stageName(stage) {
  return stageNames[stage] || stage;
}

export function stageClass(stage) {
  return 'stage-' + stage.toLowerCase().replace(/[^a-z]+/g, '-');
}

export function parseDate(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s || '');
  return m ? new Date(+m[1], m[2] - 1, +m[3]) : null;
}

export function dateCell(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

const longFormat = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric', year: 'numeric'});
const mediumFormat = new Intl.DateTimeFormat('en-US', {month: 'long', day: 'numeric', year: 'numeric'});
const shortFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});
const tableFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric', year: 'numeric'});
const monthDayFormat = new Intl.DateTimeFormat('en-US', {month: 'long', day: 'numeric'});
const weekdayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short'});

export function longDate(s) {
  const d = parseDate(s);
  return d ? longFormat.format(d) : s || '';
}

export function mediumDate(s) {
  const d = parseDate(s);
  return d ? mediumFormat.format(d) : s || '';
}

export function monthDay(s) {
  const d = parseDate(s);
  return d ? monthDayFormat.format(d) : s || '';
}

export function tableDate(s) {
  const d = parseDate(s);
  return d ? tableFormat.format(d) : s || '';
}

export function weekday(s) {
  const d = parseDate(s);
  return d ? weekdayFormat.format(d) : '';
}

export function shortDate(s) {
  const d = parseDate(s);
  return d ? shortFormat.format(d) : s || '';
}

export function isUnassigned(sv) {
  return !sv.assigned && sv.stage !== 'Complete';
}

export function mine(sv) {
  return sv.me.mine;
}

export function urgency(sv) {
  return sv.urgency || {when: '', step: ''};
}

export function urgencyWords({when, step}) {
  const what = {info: 'Charity', outreach: 'Outreach', newsletter: 'Newsletter'}[step];
  return when === 'late' ? `${what} late` : when === 'today' ? `${what} due today` : '';
}

export function onComms() {
  return state.model.standing.comms;
}

export function commsOnly() {
  return state.model.standing.commsOnly;
}

export function team() {
  const seen = new Map([[me().email, me().name]]);
  for (const m of state.model.team) {
    if (m.role === 'Volunteer' && !seen.has(m.email)) {
      seen.set(m.email, m.name);
    }
  }
  for (const sv of [...state.model.staff, ...state.model.skipped, ...state.model.missing]) {
    if (sv.assignedTo && !seen.has(sv.assignedTo)) {
      seen.set(sv.assignedTo, sv.assignedToName || sv.assignedTo);
    }
  }
  const others = [...seen].slice(1).sort((a, b) => a[1].localeCompare(b[1]));
  return [{email: me().email, name: 'Me'}, ...others.map(([email, name]) => ({email, name}))];
}

export function inStage(list, stage) {
  return list.filter(sv => sv.stage === stage);
}

export function matches(sv, query) {
  if (!query) {
    return true;
  }
  return `${sv.name} ${sv.jobTitle || ''} ${sv.department || ''} ${sv.email}`.toLowerCase().includes(query);
}

export function firstName(sv) {
  return sv.name.split(' ')[0];
}

function lastYearLines(sv) {
  const d = sv.lastDonation;
  if (!d) {
    return '';
  }
  return ["*Last Year's Charity*", charityName(d.charity), d.note || ''].filter(Boolean).join('\n');
}

function tidy(text) {
  return text.replace(/[ \t]+$/gm, '').replace(/\n{3,}/g, '\n\n').trim();
}

export function fill(template, sv) {
  return template
    .replaceAll('{first name}', firstName(sv))
    .replaceAll('{name}', sv.name)
    .replaceAll('{newsletter date}', mediumDate(sv.newsletterDate))
    .replaceAll('{birthday}', monthDay(sv.birthdayThisYear))
    .replaceAll('{default charity}', charityName(settings().defaultCharity))
    .replaceAll('{sender}', me().name)
    .replaceAll('{last year}', lastYearLines(sv));
}

export function emailLink(sv) {
  const note = sv.level === 'No Newsletter' ? fill(settings().noNewsletterNote, sv) : '';
  let body = settings().emailBody;
  if (body.includes('{no newsletter note}')) {
    body = body.replaceAll('{no newsletter note}', note);
  } else if (note) {
    body += '\n\n' + note;
  }
  body = tidy(fill(body, sv));
  return `mailto:${sv.email}?subject=${encodeURIComponent(fill(settings().emailSubject, sv))}&body=${encodeURIComponent(body)}`;
}

export function newsletterText(sv, donation) {
  const parts = [`${sv.name}${sv.jobTitle ? ` (${sv.jobTitle})` : ''}: ${charityName(donation.charity)}`];
  if (donation.note) {
    parts.push(donation.note);
  }
  return parts.join('\n');
}

export function staffFor(newsletterDate) {
  return state.model.staff.filter(sv => sv.newsletterDate === newsletterDate && sv.level !== 'No Newsletter');
}

export function byDepartment(list) {
  const groups = [];
  for (const name of [...state.model.departments, '']) {
    const items = list.filter(sv => (sv.department || '') === name);
    if (items.length) {
      groups.push({name: name || 'Other', items});
    }
  }
  const known = new Set([...state.model.departments, '']);
  for (const sv of list) {
    if (!known.has(sv.department || '')) {
      known.add(sv.department);
      groups.push({name: sv.department, items: list.filter(x => x.department === sv.department)});
    }
  }
  return groups;
}

export function staffPath(sv) {
  return sv.path;
}

export function newsletterPath(date) {
  return `/newsletters/${date}`;
}

export function charityPath(c) {
  return c.path;
}
