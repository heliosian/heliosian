import {state, me, isSystemAdmin, settings} from '../state.js';
import {el, svg, button} from '../dom.js';
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
  for (const [label, value] of [['Default charity', s.defaultCharity], ['Year start', s.yearStart], ['Email subject', s.emailSubject], ['Email body', s.emailBody], ['No-newsletter note', s.noNewsletterNote]]) {
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

const sections = [
  {title: 'Display', tabs: [
    {key: 'settings', label: 'Settings', card: settingsCard},
  ]},
  {title: 'Editing & Control', tabs: [
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
  const mark = el('img');
  mark.src = '/brand/logo-lockup-light-horizontal.png';
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
