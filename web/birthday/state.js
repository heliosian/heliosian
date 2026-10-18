import {api} from '/api.js';
import {queries} from '/rail.js';
import {directory, emailOf, photoOf} from '/directory.js';
import {birthdayTeam, birthdaySchoolYear, stageOf, urgencyOf, levels, staffPathOf} from '/birthdays.js';

export const state = {model: null};

const byHandle = new Map();

export function write(batch) {
  return api('POST', '/api/q', {batch});
}

function rowsOf(answer, table) {
  return answer.result.map(id => answer.resources[table][id]);
}

export const levelValues = {'Skip': 'skip', 'No Newsletter': 'no_newsletter'};

const two = n => String(n).padStart(2, '0');

export function nowMoment() {
  const d = new Date();
  return `${dateCell(d)} ${two(d.getHours())}:${two(d.getMinutes())}`;
}

function dayOf(moment) {
  return (moment || '').slice(0, 10);
}

export async function loadModel() {
  const [a, dir] = await Promise.all([queries({
    viewer: '(from PERSON (where (= id @viewer)))',
    admin: '(from APP (where (= key "birthday") (admin_of "birthday")) (columns key))',
    team: '(from APP (where (= key "birthday") (birthday_team)) (columns key))',
    settings: '(from SETTING (where (= app "birthday")))',
    people: '(from PERSON (where (!= source "guest") (or (not (blank birthday)) (in id (members (slug "staff"))))) (columns name_show slug job_title department birthday birthday_participation who_href))',
    birthdays: '(from BIRTHDAY)',
    years: '(from BIRTHDAY_YEAR)',
    charities: '(from CHARITY (order name asc))',
    issues: '(from GROUP (where (in parent (select GROUP.parent (in id (select BIRTHDAY.newsletter))))) (order start asc) (columns start))',
    activity: `(from GROUP (where (= slug "${birthdayTeam}")) (columns leads))`,
    members: `(from MEMBER (where (= member "yes") (or (in group (select GROUP.id (= slug "${birthdayTeam}"))) (in group (select GROUP.leads (= slug "${birthdayTeam}"))))) (columns group person))`,
  }), directory()]);
  const viewer = rowsOf(a.viewer, 'PERSON')[0];
  const settings = {};
  const settingIds = {};
  for (const s of rowsOf(a.settings, 'SETTING')) {
    settings[s.key] = s.value;
    settingIds[s.key] = s.id;
  }
  const today = dateCell(new Date());
  const start = settings['Year Start'];
  const current = birthdaySchoolYear(today, start);
  const end = parseDate(`${current + 1}-${start}`);
  end.setDate(end.getDate() - 1);
  const yearInfo = {number: current, current: `${current} - ${current + 1}`, start: `${current}-${start}`, end: dateCell(end)};
  const charities = rowsOf(a.charities, 'CHARITY').map(c => ({...c, allowed: c.allowed === 'Yes', about: c.description || '', donationLink: c.url || '', whyNotAllowed: c.not_allowed_reason || '', addedOn: dayOf(c.added), path: `/charities/${encodeURIComponent(c.name)}`}));
  const charityById = new Map(charities.map(c => [c.id, c]));
  const issues = rowsOf(a.issues, 'GROUP').map(g => ({id: g.id, date: dayOf(g.start)}));
  const issueDay = new Map(issues.map(n => [n.id, n.date]));
  const records = new Map();
  const named = new Set();
  for (const y of rowsOf(a.years, 'BIRTHDAY_YEAR')) {
    records.set(`${y.person}|${y.year}`, y);
    if (y.charity) {
      named.add(y.charity);
    }
  }
  for (const c of charities) {
    c.named = named.has(c.id);
  }
  const births = new Map();
  for (const b of rowsOf(a.birthdays, 'BIRTHDAY')) {
    if (Number(b.year) === current) {
      births.set(b.person, b);
    }
  }
  const nameOf = id => (dir.byId[id] && dir.byId[id].name_show) || '';
  const groupName = id => (id && dir.groups[id] ? dir.groups[id].name : '');
  const donationOf = r => r && r.charity ? {
    record: r.id, charity: r.charity, note: r.note || '', recordedOn: dayOf(r.recorded), recordedBy: nameOf(r.recorded_by),
    usedOn: dayOf(r.published), usedBy: r.published ? nameOf(r.published_by) || 'the weekly copy' : '',
  } : null;
  const defaultCharity = settings['Default Charity'] || '';
  const everyone = [];
  for (const p of rowsOf(a.people, 'PERSON')) {
    const b = births.get(p.id);
    const record = records.get(`${p.id}|${current}`) || null;
    const last = records.get(`${p.id}|${current - 1}`) || null;
    const lastAllowed = last && last.charity && charityById.get(last.charity) && charityById.get(last.charity).allowed;
    const sv = {
      id: p.id,
      record: record ? record.id : '',
      email: emailOf(p),
      name: p.name_show,
      profile: p.who_href,
      jobTitle: p.job_title || '',
      department: groupName(p.department),
      photoUrl: photoOf(p),
      birthday: p.birthday ? p.birthday.slice(5, 10) : '',
      birthdayCell: p.birthday || '',
      missing: !p.birthday,
      level: levels[p.birthday_participation] || '',
      birthdayThisYear: b ? b.day : '',
      newsletterDate: b ? issueDay.get(b.newsletter) || '' : '',
      requestBy: b ? b.ask_by : '',
      dueBy: b ? b.due_by : '',
      assigned: Boolean(record && record.assigned_to),
      assignedTo: record && record.assigned_to ? emailOf({id: record.assigned_to}) : '',
      assignedToId: record ? record.assigned_to || '' : '',
      assignedToName: record && record.assigned_to ? nameOf(record.assigned_to) : '',
      contactedOn: record ? dayOf(record.contacted) : '',
      contactedBy: record && record.contacted_by ? nameOf(record.contacted_by) : '',
      donation: donationOf(record),
      lastDonation: donationOf(last),
      fallback: lastAllowed ? {charity: last.charity, note: last.note || ''} : {charity: defaultCharity, note: ''},
      path: staffPathOf(p),
      me: {mine: Boolean(record && viewer && record.assigned_to === viewer.id)},
    };
    sv.stage = stageOf(sv, record, today);
    sv.urgency = urgencyOf(sv, today);
    everyone.push(sv);
  }
  const staff = everyone.filter(sv => !sv.missing && sv.level !== 'Skip');
  staff.sort((x, y) => (x.birthdayThisYear || '').localeCompare(y.birthdayThisYear || '') || byName(x, y));
  const activity = rowsOf(a.activity, 'GROUP')[0];
  const memberRows = rowsOf(a.members, 'MEMBER');
  const volunteers = memberRows.filter(m => m.group === activity.id).map(m => m.person);
  const comms = memberRows.filter(m => m.group === activity.leads).map(m => m.person);
  const admin = a.admin.result.length > 0;
  const name = viewer ? viewer.name_show : '';
  const viewerId = viewer ? viewer.id : '';
  const holding = staff.some(sv => sv.assignedToId === viewerId);
  state.model = {
    user: {id: viewerId, email: viewer ? emailOf(viewer) : '', name, initial: (name[0] || '?').toUpperCase(), photoUrl: viewer ? photoOf(viewer) : ''},
    admin,
    standing: {team: a.team.result.length > 0, comms: comms.includes(viewerId), commsOnly: comms.includes(viewerId) && !admin && !holding},
    settings: {
      defaultCharity, yearStart: start, newsletter: settings['Newsletter'] || '',
      emailSubject: settings['Email Subject'] || '', emailBody: settings['Email Body'] || '', noNewsletterNote: settings['No Newsletter Note'] || '',
      outreachCC: settings['Outreach CC'] || '', requestLeadDays: Number(settings['Request Lead Days']), dueByLeadDays: Number(settings['Due By Lead Days']),
    },
    settingIds,
    year: yearInfo,
    today,
    staff,
    skipped: everyone.filter(sv => sv.level === 'Skip').sort(byName),
    missing: everyone.filter(sv => sv.missing && sv.level !== 'Skip').sort(byName),
    charities,
    newsletterDates: issues,
    activity,
    volunteers,
    departments: Object.values(dir.groups).filter(g => g.kind === 'department').map(g => g.name).sort(),
    nameOf,
  };
  byHandle.clear();
  for (const sv of everyone) {
    for (const handle of [sv.path, `/staff/${sv.id}`, `/staff/${encodeURIComponent(sv.email)}`]) {
      byHandle.set(handle, sv);
    }
  }
}

const byName = (x, y) => x.name.localeCompare(y.name);

export function me() {
  return state.model.user;
}

export function isAdmin() {
  return state.model.admin;
}

export function year() {
  return state.model.year;
}

export function settings() {
  return state.model.settings;
}

export function staff(handle) {
  return byHandle.get(`/staff/${encodeURIComponent(decodeURIComponent(handle))}`) || null;
}

export function charity(id) {
  return state.model.charities.find(c => c.id === id) || null;
}

export function charityNamed(name) {
  return state.model.charities.find(c => c.name === name) || null;
}

export function charityName(id) {
  const c = charity(id);
  return c ? c.name : '';
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
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())}`;
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
  const seen = new Map([[me().id, me().name]]);
  for (const id of state.model.volunteers) {
    if (!seen.has(id)) {
      seen.set(id, state.model.nameOf(id));
    }
  }
  for (const sv of [...state.model.staff, ...state.model.skipped, ...state.model.missing]) {
    if (sv.assignedToId && !seen.has(sv.assignedToId)) {
      seen.set(sv.assignedToId, sv.assignedToName);
    }
  }
  const others = [...seen].slice(1).sort((x, y) => x[1].localeCompare(y[1]));
  return [{id: me().id, name: 'Me'}, ...others.map(([id, name]) => ({id, name}))];
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
