import {state, isSystemAdmin, me} from '../state.js';
import {el, button, svg} from '../dom.js';
import {setTitle} from '../chrome.js';
import {categoryList, openSettings, openRedirect, checkbox, send} from '../edit.js';

function denied() {
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Admin access required'));
  return page;
}

function categoriesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Categories'));
  card.append(el('div', 'hint', 'The headings on the Opportunities page, in this order. Each event manages its own categories from its page.'));
  card.append(categoryList(null, null));
  return card;
}

function settingsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Settings'));
  card.append(el('div', 'hint', 'The expense form the Sign Up page links to, and the intro under its heading.'));
  const expense = el('div', 'admin-row');
  const expenseBody = el('div', 'grow');
  expenseBody.append(el('div', '', 'Expense form'), el('div', 'sub', state.model.settings.expenseFormUrl));
  expense.append(expenseBody);
  const intro = el('div', 'admin-row');
  const introBody = el('div', 'grow');
  introBody.append(el('div', '', 'Intro'), el('div', 'sub', state.model.settings.intro));
  intro.append(introBody);
  const add = el('div', 'add-row');
  add.append(button('Edit Settings', 'edit', 'button', openSettings));
  card.append(expense, intro, add);
  return card;
}

// notifyCard is the signed-in admin's own email notices: four switches, each
// saved the moment it is flipped. Every admin has their own set.
function notifyCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Email notifications'));
  card.append(el('div', 'hint', 'Which of these you want an email about. These are yours alone; every admin picks their own.'));
  const kinds = [
    ['events', 'A new event is added', 'Someone adds an event to the Opportunities page, or suggests one.'],
    ['activities', 'A new activity is added under an event', 'A committee, booth or shift is added or suggested under any event.'],
    ['signups', 'Someone signs up', 'A new volunteer on any event or thing under one.'],
    ['offers', 'Someone offers to co-chair', 'A volunteer marks themselves open to co-chairing.'],
  ];
  const stack = el('div', 'setting-stack notify-stack');
  const status = el('span', 'save-status');
  const boxes = {};
  const persist = async () => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    try {
      await send('POST', '/api/team/notify', {kinds: kinds.map(k => k[0]).filter(k => boxes[k].checked)});
      status.textContent = 'Saved.';
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
    }
  };
  for (const [kind, label, hint] of kinds) {
    const box = checkbox(label, false, hint);
    box.input.disabled = true;
    box.input.addEventListener('change', persist);
    boxes[kind] = box.input;
    stack.append(box.wrap);
  }
  const notice = el('div');
  card.append(notice, stack, status);
  fetch('/api/admin/state').then(async res => {
    if (!res.ok) {
      return;
    }
    const data = await res.json();
    for (const kind of Object.keys(boxes)) {
      boxes[kind].checked = (data.notify || []).includes(kind);
      boxes[kind].disabled = false;
    }
    if (!data.mail) {
      notice.append(el('div', 'notice', 'Email is not set up on this server yet, so nothing is sent; the choices are kept for when it is.'));
    }
  });
  return card;
}

// adminsCard mirrors the other apps' admin lists: every add or remove posts
// immediately, so nothing looks saved that isn't.
function adminsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Admins'));
  card.append(el('div', 'hint', 'Whoever is on this list can approve suggestions, edit any activity, and reach this page. Co-chairs edit their own activities without being here.'));
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
    const data = await res.json();
    admins = data.admins;
    render();
  });
  return card;
}

// redirectsCard is the Redirects tab: every old address the portal sends on,
// the ones renames wrote as well as the ones added here, each with where it
// goes. A link from the old volunteer site is the usual reason to add one.
function redirectsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Redirects'));
  card.append(el('div', 'hint', 'Old addresses on this site and where they now lead, followed before sign-in. Renaming an event adds one on its own; add one here for a link from the old volunteer site, or any address that should land somewhere else.'));
  const rows = el('div');
  const redirects = (state.model.redirects || []).slice().sort((a, b) => (b.date || '').localeCompare(a.date || '') || a.old.localeCompare(b.old));
  if (!redirects.length) {
    rows.append(el('div', 'hint', 'None yet.'));
  }
  for (const item of redirects) {
    const row = el('div', 'admin-row redirect-row');
    const body = el('div', 'grow');
    const line = el('div', 'redirect-line');
    line.append(el('code', 'redirect-old', item.old), el('span', 'redirect-arrow', '→'), el('code', 'redirect-new', item.new));
    const sub = [];
    if (item.type === 'Activity' || item.type === 'pretty') {
      sub.push('From a rename');
    } else if (item.type && item.type !== 'Admin') {
      sub.push(item.type);
    }
    if (item.date) {
      sub.push(item.date);
    }
    body.append(line);
    if (sub.length) {
      body.append(el('div', 'sub', sub.join(' · ')));
    }
    row.append(body, button('Edit', 'edit', 'button button-secondary button-small', () => openRedirect(item)));
    rows.append(row);
  }
  const add = el('div', 'add-row');
  add.append(button('Add Redirect', 'plus', 'button', () => openRedirect(null)));
  card.append(rows, add);
  return card;
}

// The admin chrome every app shares (web/common/admin.css): the teal header
// with the mark, "Admin", the address and a close button; the rail of grouped
// tabs; one panel showing at a time.
const sections = [
  {title: 'Display', tabs: [
    {key: 'categories', label: 'Categories', card: categoriesCard},
    {key: 'settings', label: 'Settings', card: settingsCard},
  ]},
  {title: 'Editing & Control', tabs: [
    {key: 'notify', label: 'Email Notifications', card: notifyCard},
    {key: 'admins', label: 'Admins', card: adminsCard},
  ]},
  {title: 'Addresses', tabs: [
    {key: 'redirects', label: 'Redirects', card: redirectsCard},
  ]},
];

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
  mark.alt = 'HCA-Team';
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
  const tabs = [];
  const show = key => {
    for (const tab of tabs) {
      tab.classList.toggle('active', tab.dataset.panel === key);
    }
    for (const [k, panel] of Object.entries(panels)) {
      panel.hidden = k !== key;
    }
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
      tabs.push(tab);
      group.append(tab);
      const panel = el('div', 'panel');
      panel.append(item.card());
      panels[item.key] = panel;
      container.append(panel);
    }
    rail.append(group);
  }
  show(sections[0].tabs[0].key);
  layout.append(rail, container);
  page.append(header, layout);
  return page;
}
