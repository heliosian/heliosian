// superEdit is an admin's hat, as HCA-Team has it: off, they see and can do
// what any team member can; on, every admin control comes back. It is
// remembered per browser. The server keeps enforcing by the admin list
// either way; the switch is about what the page shows and offers.
export const state = {model: null, superEdit: readSuperEdit()};

function readSuperEdit() {
  try {
    return localStorage.getItem('birthday.superEdit') === '1';
  } catch (err) {
    return false;
  }
}

export function setSuperEdit(on) {
  state.superEdit = on;
  try {
    localStorage.setItem('birthday.superEdit', on ? '1' : '0');
  } catch (err) {
    // A browser that refuses storage just forgets the choice on reload.
  }
}

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

// isSystemAdmin says the person is on the admin list, hat or no hat.
export function isSystemAdmin() {
  return state.model.user.isAdmin;
}

// isAdmin is what the page offers: the list and the hat together.
export function isAdmin() {
  return isSystemAdmin() && state.superEdit;
}

export function year() {
  return state.model.year;
}

export function settings() {
  return state.model.settings;
}

// staff finds a person by the address in their page's path - the part before
// the @ for a school address, the whole address for anyone else.
export function staff(handle) {
  if (handle.includes('@')) {
    return byEmail.get(handle) || null;
  }
  for (const [email, sv] of byEmail) {
    if (email.split('@')[0] === handle && email.endsWith(schoolDomain)) {
      return sv;
    }
  }
  return null;
}

const schoolDomain = '@heliosschool.org';

export function charity(name) {
  return state.model.charities.find(c => c.name === name) || null;
}

export const stages = ['Wait', 'Awaiting Outreach', 'Awaiting Response', 'Awaiting Newsletter', 'Complete'];

const stageNumbers = {'Wait': 2, 'Awaiting Outreach': 3, 'Awaiting Response': 4, 'Awaiting Newsletter': 5, 'Complete': 6};

export function stageLabel(stage) {
  return `${stageNumbers[stage]}. ${stage}`;
}

// stageName is the stage as the pages word it for the team: what to do next
// rather than what is being awaited.
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

// monthDay is a date without its year, September 26, as the birthday letter says it.
export function monthDay(s) {
  const d = parseDate(s);
  return d ? monthDayFormat.format(d) : s || '';
}

// tableDate is a date as a table column shows it, Oct 5, 2026, and weekday its
// day of the week, Mon.
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
  return !sv.assignedTo && sv.stage !== 'Complete';
}

export function mine(sv) {
  return sv.assignedTo === me().email;
}

// urgency is where a birthday stands against its days, reckoned as the
// toolbar's late badge reckons it (internal/birthday/late.go), and a day
// early too: {when: 'late' | 'today' | '', step}. The steps are the
// birthday's information - the charity, due by dueBy - and before that the
// outreach, due by requestBy, both the assignee's; and once the charity is
// in, the newsletter, the comms team's. A late step outranks one due today.
export function urgency(sv) {
  if (sv.stage === 'Complete') {
    return {when: '', step: ''};
  }
  const steps = [
    ['info', !sv.donation && sv.dueBy],
    ['outreach', sv.stage === 'Awaiting Outreach' && sv.requestBy],
    ['newsletter', sv.stage === 'Awaiting Newsletter' && sv.newsletterDate],
  ];
  const today = state.model.today;
  for (const when of ['late', 'today']) {
    for (const [step, day] of steps) {
      if (day && (when === 'late' ? day < today : day === today)) {
        return {when, step};
      }
    }
  }
  return {when: '', step: ''};
}

// urgencyWords is a birthday's urgency as a row's chip says it.
export function urgencyWords({when, step}) {
  const what = {info: 'Charity', outreach: 'Outreach', newsletter: 'Newsletter'}[step];
  return when === 'late' ? `${what} late` : when === 'today' ? `${what} due today` : '';
}

// commsOnly says the viewer is on the comms team and nothing else - not a
// volunteer, not an admin - so the app shows them the newsletters alone.
export function onComms() {
  const email = me().email;
  return state.model.team.some(m => m.email === email && m.role === 'Comms Team');
}

export function commsOnly() {
  const email = me().email;
  const roles = state.model.team.filter(m => m.email === email).map(m => m.role);
  return roles.includes('Comms Team') && !roles.includes('Volunteer') && !isSystemAdmin();
}

// team is who a birthday can be assigned to: the volunteers on the Team tab
// and everyone who already has a staff member assigned to them, by name, with
// the viewer first whether or not they are either.
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

// lastYearLines is what {last year} stands for: a heading, the charity and
// the staff member's note from last year, and nothing when there is no last
// year. The heading wears asterisks, the plain-text mark for bold, since a
// mailto draft carries no formatting.
function lastYearLines(sv) {
  const d = sv.lastDonation;
  if (!d) {
    return '';
  }
  return ["*Last Year's Charity*", d.charity, d.note || ''].filter(Boolean).join('\n');
}

// tidy drops the blank lines and trailing spaces an empty placeholder leaves behind.
function tidy(text) {
  return text.replace(/[ \t]+$/gm, '').replace(/\n{3,}/g, '\n\n').trim();
}

export function fill(template, sv) {
  return template
    .replaceAll('{first name}', firstName(sv))
    .replaceAll('{name}', sv.name)
    .replaceAll('{newsletter date}', mediumDate(sv.newsletterDate))
    .replaceAll('{birthday}', monthDay(sv.birthdayThisYear))
    .replaceAll('{default charity}', settings().defaultCharity)
    .replaceAll('{sender}', me().name)
    .replaceAll('{last year}', lastYearLines(sv));
}

// emailLink is the draft: the body with the person filled in, and the no-newsletter note for anyone who
// asked to stay out of the newsletter, placed where {no newsletter note} sits or at the end when the body
// does not say.
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
  const email = sv.email;
  return `/staff/${encodeURIComponent(email.endsWith(schoolDomain) ? email.slice(0, -schoolDomain.length) : email)}`;
}

export function newsletterPath(date) {
  return `/newsletters/${date}`;
}

export function charityPath(c) {
  return `/charities/${encodeURIComponent(c.name)}`;
}
