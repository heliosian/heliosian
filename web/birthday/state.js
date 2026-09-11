export const state = {model: null};

const byEmail = new Map();

export function applyModel(model) {
  state.model = model;
  byEmail.clear();
  for (const s of [...model.staff, ...model.skipped, ...model.missing]) {
    byEmail.set(s.email, s);
  }
}

export function me() {
  return state.model.user;
}

export function isAdmin() {
  return state.model.user.isAdmin;
}

export function year() {
  return state.model.year;
}

export function settings() {
  return state.model.settings;
}

export function staff(email) {
  return byEmail.get(email) || null;
}

export function charity(name) {
  return state.model.charities.find(c => c.name === name) || null;
}

export const stages = ['Wait', 'Awaiting Outreach', 'Awaiting Response', 'Awaiting Newsletter', 'Complete'];

const stageNumbers = {'Wait': 2, 'Awaiting Outreach': 3, 'Awaiting Response': 4, 'Awaiting Newsletter': 5, 'Complete': 6};

export function stageLabel(stage) {
  return `${stageNumbers[stage]}. ${stage}`;
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

export function longDate(s) {
  const d = parseDate(s);
  return d ? longFormat.format(d) : s || '';
}

export function mediumDate(s) {
  const d = parseDate(s);
  return d ? mediumFormat.format(d) : s || '';
}

export function shortDate(s) {
  const d = parseDate(s);
  return d ? shortFormat.format(d) : s || '';
}

export function isUnassigned(sv) {
  return !sv.assignedTo && sv.stage !== 'Complete';
}

export function mine(sv) {
  return sv.assignedTo === me().email;
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

export function fill(template, sv) {
  return template
    .replaceAll('{first name}', firstName(sv))
    .replaceAll('{name}', sv.name)
    .replaceAll('{newsletter date}', mediumDate(sv.newsletterDate))
    .replaceAll('{birthday}', mediumDate(sv.birthdayThisYear))
    .replaceAll('{default charity}', settings().defaultCharity);
}

export function emailLink(sv) {
  let body = fill(settings().emailBody, sv);
  if (sv.level === 'No Newsletter') {
    body += '\n\n' + fill(settings().noNewsletterNote, sv);
  }
  return `mailto:${sv.email}?subject=${encodeURIComponent(fill(settings().emailSubject, sv))}&body=${encodeURIComponent(body)}`;
}

export function newsletterText(sv, donation) {
  const parts = [`${sv.name}${sv.jobTitle ? ` (${sv.jobTitle})` : ''}: ${donation.charity}`];
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
  return `/staff/${encodeURIComponent(sv.email)}`;
}

export function charityPath(c) {
  return `/charities/${encodeURIComponent(c.name)}`;
}
