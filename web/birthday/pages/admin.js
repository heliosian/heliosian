import {state, me, isSystemAdmin, settings} from '../state.js';
import {el, svg, button} from '../dom.js';
import {createPersonPicker} from '/picker.js';
import {setTitle} from '../chrome.js';
import {openSettings} from '../edit.js';

function denied() {
  const page = el('div', 'list-page');
  page.append(el('h1', 'page-title', 'Admin access required'));
  return page;
}

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

function adminsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Admins'));
  card.append(el('div', 'hint', 'Whoever is on this list can change the settings, the newsletter dates, and which charities are allowed, and reach this page. Everyone signed in can work the process.'));
  const rows = el('div');
  const status = el('span', 'save-status');
  let admins = [];
  const persist = async () => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    const res = await fetch('/api/admin/admins', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({admins})});
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    status.textContent = 'Saved.';
  };
  const render = () => {
    rows.replaceChildren();
    if (!admins.length) {
      rows.append(el('div', 'hint', 'Nobody yet.'));
    }
    for (const email of admins) {
      const row = el('div', 'admin-row');
      row.append(el('div', 'grow', email));
      const remove = el('button', 'link-button danger', 'Remove');
      remove.type = 'button';
      remove.addEventListener('click', async () => {
        admins = admins.filter(e => e !== email);
        render();
        await persist();
      });
      row.append(remove);
      rows.append(row);
    }
  };
  const add = el('div', 'add-row');
  const input = el('input');
  input.type = 'email';
  input.placeholder = 'name@heliosschool.org';
  const addOne = async () => {
    const email = input.value.trim().toLowerCase();
    if (!email || admins.includes(email)) {
      return;
    }
    admins.push(email);
    input.value = '';
    render();
    await persist();
  };
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addOne();
    }
  });
  add.append(input, button('Add', null, 'button', addOne), status);
  card.append(rows, add);
  fetch('/api/admin/state').then(async res => {
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = 'Failed to load the admin list.';
      return;
    }
    admins = (await res.json()).admins;
    render();
  });
  return card;
}

// teamCard is the birthday team by role: who volunteers to work the birthdays
// and who carries them into the newsletter. Each role takes someone from the
// directory or an address typed in, and loses them with Remove.
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

// invitesCard resends every assignee's calendar invite, for after the dates'
// rules change under them.
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
    {key: 'admins', label: 'Admins', card: adminsCard},
  ]},
];

// The admin chrome every app shares (web/common/admin.css): the teal header
// with the mark, "Admin", the address and a close button; the rail of grouped
// tabs; one panel showing at a time.
export function adminPage() {
  setTitle('Admin Tools');
  // The page goes with being on the admin list, hat or no hat.
  if (!isSystemAdmin()) {
    return denied();
  }
  const page = el('div', 'admin admin-strip');
  const header = el('header');
  const brand = el('a', 'brand-link');
  brand.href = '/';
  brand.setAttribute('data-link', '');
  const mark = el('img', 'admin-tile');
  mark.src = '/brand/icon-192.png';
  mark.alt = 'Helios Staff Birthdays';
  brand.append(mark, el('span', '', 'Admin'));
  const right = el('span', 'right');
  right.append(el('span', 'email', me().email));
  const close = el('a', 'admin-close');
  close.href = '/';
  close.setAttribute('data-link', '');
  close.setAttribute('aria-label', 'Close admin tools');
  close.append(svg('close'));
  right.append(close);
  header.append(brand, right);

  const layout = el('div', 'layout');
  const rail = el('nav', 'sidebar');
  const container = el('div', 'container');
  const panels = {};
  const tabList = [];
  const show = key => {
    for (const tab of tabList) {
      tab.classList.toggle('active', tab.dataset.panel === key);
    }
    for (const [k, panel] of Object.entries(panels)) {
      panel.hidden = k !== key;
    }
    state.adminTab = key;
  };
  for (const section of sections) {
    const group = el('div', 'sidebar-section');
    group.append(el('div', 'sidebar-section-title', section.title));
    for (const item of section.tabs) {
      // Appearance - what colours the app - is the platform's super admins'
      // alone; a regular admin's page is built without it.
      if (item.superOnly && !me().isSuperAdmin) {
        continue;
      }
      const tab = el('div', 'tab', item.label);
      tab.dataset.panel = item.key;
      tab.addEventListener('click', () => show(item.key));
      tabList.push(tab);
      group.append(tab);
      const panel = el('div', 'panel');
      panel.append(item.card());
      panels[item.key] = panel;
      container.append(panel);
    }
    rail.append(group);
  }
  show(state.adminTab && panels[state.adminTab] ? state.adminTab : sections[0].tabs[0].key);
  layout.append(rail, container);
  page.append(header, layout);
  return page;
}
