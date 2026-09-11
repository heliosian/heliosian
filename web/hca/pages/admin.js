import {state, isSystemAdmin} from '../state.js';
import {el, button} from '../dom.js';
import {setTitle} from '../chrome.js';
import {categoryList, openSettings, checkbox, send, peoplePicker} from '../edit.js';

function denied() {
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Admin access required'));
  return page;
}

function categoriesCard() {
  const card = el('div', 'admin-card');
  card.append(el('h2', '', 'Categories'));
  card.append(el('div', 'hint', 'The headings on the Opportunities page, in this order. Each event manages its own categories from its page.'));
  card.append(categoryList(null, null));
  return card;
}

function settingsCard() {
  const card = el('div', 'admin-card');
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
  const card = el('div', 'admin-card');
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
      await send('POST', '/api/events/notify', {kinds: kinds.map(k => k[0]).filter(k => boxes[k].checked)});
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

// spoofCard is Spoof Mode, as Helios Who? has it: pick anyone in the directory
// and the whole portal - every page, every button, every save - runs as them
// until Stop, on the banner or here. Only a real system admin gets the card;
// while viewing as someone without admin rights this page is not reachable,
// which is what the banner's Stop is for.
function spoofCard() {
  const card = el('div', 'admin-card');
  card.hidden = true;
  card.append(el('h2', '', 'Spoof Mode'));
  card.append(el('div', 'hint', 'See the portal exactly as someone else does - what they can open, sign up for and edit. Everything you do while viewing as them is done as them, so look, don\'t touch.'));
  const current = el('div', 'notice');
  current.hidden = true;
  const picker = peoplePicker();
  const row = el('div', 'spoof-row');
  const status = el('span', 'save-status');
  const go = button('View as', 'people', 'button', async () => {
    const email = picker.value();
    if (!email) {
      return;
    }
    try {
      await send('POST', '/api/admin/spoof', {email});
      location.href = '/';
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
    }
  });
  row.append(picker.wrap, go, status);
  card.append(current, row);
  fetch('/api/admin/state').then(async res => {
    if (!res.ok) {
      return;
    }
    const data = await res.json();
    card.hidden = !data.canSpoof;
    if (data.spoofing) {
      current.hidden = false;
      current.textContent = `Viewing as ${data.spoofing} - use Stop on the banner above to be yourself again.`;
    }
  });
  return card;
}

// adminsCard mirrors the other apps' admin lists: every add or remove posts
// immediately, so nothing looks saved that isn't.
function adminsCard() {
  const card = el('div', 'admin-card');
  card.append(el('h2', '', 'Admins'));
  card.append(el('div', 'hint', 'Whoever is on this list can approve suggestions, edit any activity, and reach this page. Co-chairs edit their own activities without being here.'));
  const notice = el('div');
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
  card.append(notice, rows, add);
  fetch('/api/admin/state').then(async res => {
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = 'Failed to load the admin list.';
      return;
    }
    const data = await res.json();
    admins = data.admins;
    if (!data.hasStore) {
      notice.append(el('div', 'notice', 'Running in sample mode: image uploads are disabled because there is no media bucket configured.'));
    }
    render();
  });
  return card;
}

export function adminPage() {
  setTitle('Admin Tools');
  // The page goes with being on the admin list, hat or no hat.
  if (!isSystemAdmin()) {
    return denied();
  }
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Admin Tools'), categoriesCard(), settingsCard(), notifyCard(), adminsCard(), spoofCard());
  return page;
}
