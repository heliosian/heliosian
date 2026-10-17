import {state, me, isAdmin, settings, charityName} from '../state.js';
import {el, button} from '/elements.js';
import {openSettings} from '../edit.js';
import {adminPage as buildAdminPage, adminsCard, groupMembers} from '/admin.js';

function settingsCard() {
  const s = settings();
  const card = el('div', 'card');
  card.append(el('h2', '', 'Settings'));
  card.append(el('div', 'hint', 'The default charity, when the birthday year turns over, and the outreach email.'));
  for (const [label, value] of [['Default charity', charityName(s.defaultCharity)], ['Year start', s.yearStart], ['Ask-by lead', `${s.requestLeadDays} days before the newsletter`], ['Birthday due by', `${s.dueByLeadDays} days before the newsletter`], ['Email subject', s.emailSubject], ['Email body', s.emailBody], ['No-newsletter note', s.noNewsletterNote], ['CC on outreach', s.outreachCC || '—']]) {
    const row = el('div', 'admin-row');
    const body = el('div', 'grow');
    body.append(el('div', '', label), el('div', 'sub pre', value));
    row.append(body);
    card.append(row);
  }
  const add = el('div', 'add-row');
  add.append(button('Edit Settings', 'edit', 'button', openSettings));
  card.append(add);
  return card;
}

const byId = id => ['=', {n: 'id'}, id];

const sections = [
  {title: 'Display', tabs: [
    {key: 'settings', label: 'Settings', card: settingsCard},
  ]},
  {title: 'Editing & Control', tabs: [
    {key: 'volunteers', label: 'Volunteers', card: () => adminsCard({title: 'Volunteers', hint: 'Offered as choices when a birthday is assigned.', ...groupMembers(byId(state.model.activity.id))})},
    {key: 'comms', label: 'Comms Team', card: () => adminsCard({title: 'Comms Team', hint: 'Carries the donations into the newsletter.', ...groupMembers(byId(state.model.activity.leads))})},
    {key: 'admins', label: 'Admins', card: () => adminsCard({hint: 'Whoever is on this list can change the settings, the newsletter dates, and which charities are allowed, and reach this page.', ...groupMembers(['in', {n: 'id'}, ['select', 'APP.admins', ['=', {n: 'key'}, 'birthday']]])})},
  ]},
];

export function adminPage() {
  return buildAdminPage({appName: 'Helios Staff Birthdays', allowed: isAdmin(), email: me().email, sections});
}
