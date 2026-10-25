import {queries, rowsOf} from '/rail.js';

export const birthdayTeam = 'bday';

export function birthdaySchoolYear(today, start) {
  const y = Number(today.slice(0, 4));
  return today.slice(5) < start ? y - 1 : y;
}

export function stageOf(sv, record, today) {
  const donated = Boolean(record && record.charity);
  if ((record && record.published) || (donated && sv.level === 'No Newsletter')) {
    return 'Complete';
  }
  if (donated) {
    return 'Awaiting Newsletter';
  }
  if (record && record.contacted) {
    return 'Awaiting Response';
  }
  if (sv.requestBy && today >= sv.requestBy) {
    return 'Awaiting Outreach';
  }
  return 'Wait';
}

export function urgencyOf(sv, today) {
  if (sv.stage === 'Complete') {
    return null;
  }
  const steps = [
    {step: 'info', day: sv.donation ? '' : sv.dueBy},
    {step: 'outreach', day: sv.stage === 'Awaiting Outreach' ? sv.requestBy : ''},
    {step: 'newsletter', day: sv.stage === 'Awaiting Newsletter' ? sv.newsletterDate : ''},
  ];
  for (const when of ['late', 'today']) {
    for (const s of steps) {
      if (s.day && ((when === 'late' && s.day < today) || (when === 'today' && s.day === today))) {
        return {when, step: s.step};
      }
    }
  }
  return null;
}

export function nextStep(sv) {
  switch (sv.stage) {
    case 'Wait':
    case 'Awaiting Outreach':
      return {step: 'outreach', day: sv.requestBy};
    case 'Awaiting Response':
      return {step: 'info', day: sv.dueBy};
    case 'Awaiting Newsletter':
      return {step: 'newsletter', day: sv.newsletterDate};
  }
  return null;
}

export function lateFor(sv, today, {viewer, admin, comms}) {
  const mine = admin || sv.assignedToId === viewer;
  if (sv.stage === 'Complete') {
    return null;
  }
  if (!sv.donation && sv.dueBy && sv.dueBy < today) {
    return mine ? {step: 'info', due: sv.dueBy} : null;
  }
  if (sv.stage === 'Awaiting Outreach' && sv.requestBy && sv.requestBy < today) {
    return mine ? {step: 'outreach', due: sv.requestBy} : null;
  }
  if (sv.stage === 'Awaiting Newsletter' && sv.newsletterDate && sv.newsletterDate < today) {
    return admin || comms ? {step: 'newsletter', due: sv.newsletterDate} : null;
  }
  return null;
}

export const levels = {skip: 'Skip', no_newsletter: 'No Newsletter'};

export function staffPathOf(p) {
  return `/staff/${encodeURIComponent(p.slug || p.id)}`;
}

function day(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

export async function birthdayWork() {
  const a = await queries({
    viewer: '(from PERSON (where (= id @viewer)) (columns name_show))',
    admin: '(from APP (where (= key "birthday") (admin_of "birthday")) (columns key))',
    settings: '(from SETTING (where (= app "birthday") (in key "Year Start" "Default Charity")))',
    birthdays: '(from BIRTHDAY (include person newsletter))',
    years: '(from BIRTHDAY_YEAR (include assigned_to))',
    charities: '(from CHARITY (columns name allowed))',
    issues: '(from GROUP (where (in parent (select GROUP.parent (in id (select BIRTHDAY.newsletter)))) (>= start today)) (order start asc) (limit 2) (columns start))',
    comms: `(from MEMBER (where (= person @viewer) (= member "yes") (in group (select GROUP.leads (= slug "${birthdayTeam}")))) (columns group))`,
    photos: '(from PHOTO (where ready (in person (select BIRTHDAY.person))) (order order asc) (columns person))',
  });
  const viewer = a.viewer.result[0] || '';
  const admin = a.admin.result.length > 0;
  const comms = a.comms.result.length > 0;
  const birthdays = rowsOf(a.birthdays, 'BIRTHDAY');
  if (!birthdays.length) {
    return {viewer, admin, comms, items: [], issues: []};
  }
  const settings = {};
  for (const s of rowsOf(a.settings, 'SETTING')) {
    settings[s.key] = s.value;
  }
  const today = day(new Date());
  const current = birthdaySchoolYear(today, settings['Year Start']);
  const people = a.birthdays.resources.PERSON;
  const issueOf = id => (a.birthdays.resources.GROUP[id].start || '').slice(0, 10);
  const charities = a.charities.resources.CHARITY || {};
  const records = new Map();
  for (const y of rowsOf(a.years, 'BIRTHDAY_YEAR')) {
    records.set(`${y.person}|${y.year}`, y);
  }
  const assignees = a.years.resources.PERSON || {};
  const photo = {};
  for (const ph of rowsOf(a.photos, 'PHOTO')) {
    photo[ph.person] = photo[ph.person] || `/api/blob/${ph.id}/thumbnail`;
  }
  const items = [];
  for (const b of birthdays) {
    const p = people[b.person];
    if (Number(b.year) !== current || p.birthday_participation === 'skip') {
      continue;
    }
    const record = records.get(`${b.person}|${b.year}`) || null;
    const last = records.get(`${b.person}|${current - 1}`) || null;
    const lastCharity = last && last.charity && charities[last.charity];
    const fallback = lastCharity && lastCharity.allowed === 'Yes' ? last.charity : settings['Default Charity'];
    const sv = {
      person: b.person, name: p.name_show, photo: photo[b.person] || '', path: staffPathOf(p), level: levels[p.birthday_participation] || '',
      birthdayThisYear: b.day, newsletterDate: issueOf(b.newsletter), requestBy: b.ask_by, dueBy: b.due_by,
      assignedToId: record ? record.assigned_to || '' : '',
      assignee: record && record.assigned_to && assignees[record.assigned_to] ? assignees[record.assigned_to].name_show : '',
      donation: Boolean(record && record.charity),
      charity: (charities[record && record.charity ? record.charity : fallback] || {}).name || '',
    };
    sv.stage = stageOf(sv, record, today);
    sv.urgency = urgencyOf(sv, today);
    sv.next = nextStep(sv);
    sv.late = lateFor(sv, today, {viewer, admin, comms});
    items.push(sv);
  }
  items.sort((x, y) => x.name.localeCompare(y.name));
  return {viewer, admin, comms, items, issues: rowsOf(a.issues, 'GROUP').map(g => g.start.slice(0, 10))};
}
