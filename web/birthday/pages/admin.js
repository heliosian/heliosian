import {me, isSystemAdmin, settings} from '../state.js';
import {el, button} from '../dom.js';
import {createPersonPicker} from '/picker.js';
import {openSettings} from '../edit.js';
import {adminPage as buildAdminPage, adminsCard} from '/admin.js';

function settingsCard() {
  const s = settings();
  const card = el('div', 'card');
  card.append(el('h2', '', 'Settings'));
  card.append(el('div', 'hint', 'The default charity, when the birthday year turns over, and the outreach email.'));
  for (const [label, value] of [['Default charity', s.defaultCharity], ['Year start', s.yearStart], ['Ask-by lead', `${s.requestLeadDays} days before the newsletter`], ['Birthday due by', `${s.dueByLeadDays} days before the newsletter`], ['Email subject', s.emailSubject], ['Email body', s.emailBody], ['No-newsletter note', s.noNewsletterNote], ['CC on outreach', s.outreachCC || '—']]) {
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

function teamCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Team'));
  card.append(el('div', 'hint', 'Volunteers are offered when a birthday is assigned. The comms team carries the donations into the newsletter. Pick someone from the directory, or type an address the directory does not have.'));
  const status = el('span', 'save-status');
  let team = [];
  let people = [];
  const groups = el('div');
  const roleGroup = (role, blurb) => {
    const group = el('div', 'team-group');
    group.append(el('h3', '', role), el('div', 'hint', blurb));
    const rows = el('div');
    const add = el('div', 'add-row');
    const mount = el('div');
    const picker = createPersonPicker(mount, {address: true, people: () => people.filter(p => !team.some(m => m.role === role && m.email === p.email))});
    const render = () => {
      rows.replaceChildren();
      const members = team.filter(m => m.role === role);
      if (!members.length) {
        rows.append(el('div', 'hint', 'Nobody yet.'));
      }
      for (const m of members) {
        const row = el('div', 'admin-row');
        const body = el('div', 'grow');
        body.append(el('div', '', m.name), el('div', 'sub', m.email));
        row.append(body);
        const remove = el('button', 'link-button danger', 'Remove');
        remove.type = 'button';
        remove.addEventListener('click', () => change('DELETE', m.email, role));
        row.append(remove);
        rows.append(row);
      }
    };
    const addOne = () => {
      const email = picker.value;
      if (!email) {
        if (picker.input.value.trim()) {
          status.classList.add('error');
          status.textContent = 'Pick someone from the list, or type a full email address.';
        }
        return;
      }
      picker.reset();
      change('POST', email, role);
    };
    mount.addEventListener('keydown', e => {
      if (e.key === 'Enter' && !mount.querySelector('.person-picker-option.active')) {
        e.preventDefault();
        addOne();
      }
    });
    add.append(mount, button('Add', null, 'button', addOne));
    group.append(rows, add);
    groups.append(group);
    return render;
  };
  const renders = [];
  const renderAll = () => renders.forEach(r => r());
  const change = async (method, email, role) => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    const res = await fetch('/api/admin/team', {method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify({email, role})});
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    await load();
    status.textContent = 'Saved.';
  };
  const load = async () => {
    const res = await fetch('/api/admin/state');
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = 'Failed to load the team.';
      return;
    }
    const data = await res.json();
    team = data.team;
    people = data.people;
    if (!renders.length) {
      for (const [role, blurb] of [['Volunteer', 'Offered as choices when a birthday is assigned.'], ['Comms Team', 'Carries the donations into the newsletter.']]) {
        renders.push(roleGroup(role, blurb));
      }
    }
    renderAll();
  };
  card.append(groups, status);
  load();
  return card;
}

function invitesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Calendar Invites'));
  card.append(el('div', 'hint', 'Everyone holding a birthday gets its invite when it is assigned, and again when its day to ask by moves. Resend them all so every calendar shows the dates as they stand now - each replaces its earlier one rather than adding to it.'));
  const row = el('div', 'add-row');
  const status = el('span', 'save-status');
  const send = button('Resend All Invites', 'send', 'button', async () => {
    if (!confirm('Send everyone holding a birthday its invite again?')) {
      return;
    }
    send.disabled = true;
    status.classList.remove('error');
    status.textContent = 'Sending…';
    const res = await fetch('/api/admin/resend-invites', {method: 'POST'});
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
    } else {
      const {sent} = await res.json();
      status.textContent = `Sent ${sent} ${sent === 1 ? 'invite' : 'invites'}.`;
    }
    send.disabled = false;
  });
  row.append(send, status);
  card.append(row);
  return card;
}

const sections = [
  {title: 'Display', tabs: [
    {key: 'settings', label: 'Settings', card: settingsCard},
  ]},
  {title: 'Editing & Control', tabs: [
    {key: 'team', label: 'Team', card: teamCard},
    {key: 'invites', label: 'Calendar Invites', card: invitesCard},
    {key: 'admins', label: 'Admins', card: () => adminsCard({hint: 'Whoever is on this list can change the settings, the newsletter dates, and which charities are allowed, and reach this page. Everyone signed in can work the process.', people: directory})},
  ]},
];

async function directory() {
  const res = await fetch('/api/admin/state');
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return (await res.json()).people;
}

export function adminPage() {
  return buildAdminPage({appName: 'Helios Staff Birthdays', allowed: isSystemAdmin(), email: me().email, sections});
}
