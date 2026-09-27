import {state, isSystemAdmin, me} from '../state.js';
import {el, button} from '../dom.js';
import {categoryList, openSettings, openRedirect, send, people} from '../edit.js';
import {checkbox} from '/form.js';
import {adminPage as buildAdminPage, adminsCard} from '/admin.js';

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

const sections = [
  {title: 'Display', tabs: [
    {key: 'categories', label: 'Categories', card: categoriesCard},
    {key: 'settings', label: 'Settings', card: settingsCard},
  ]},
  {title: 'Editing & Control', tabs: [
    {key: 'notify', label: 'Email Notifications', card: notifyCard},
    {key: 'admins', label: 'Admins', card: () => adminsCard({hint: 'Whoever is on this list can approve suggestions, edit any activity, and reach this page. Co-chairs edit their own activities without being here.', people})},
  ]},
  {title: 'Addresses', tabs: [
    {key: 'redirects', label: 'Redirects', card: redirectsCard},
  ]},
];

export function adminPage() {
  return buildAdminPage({appName: 'HCA-Team', allowed: isSystemAdmin(), email: me().email, sections});
}
