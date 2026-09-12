import {createPersonPicker} from '/picker.js';


// A list of addresses that saves itself: every add or remove posts at once, so
// nothing looks saved that isn't. The Admins panel is one. body wraps the list
// as the route wants it.
function listEditor({rows, status, input, button, url, body, empty, initial}) {
  let list = [...initial];

  async function persist() {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    const res = await fetch(url, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body(list)),
    });
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    status.textContent = 'Saved.';
  }

  function render() {
    rows.replaceChildren();
    if (!list.length) {
      const hint = document.createElement('div');
      hint.className = 'hint';
      hint.textContent = empty;
      rows.append(hint);
    }
    for (const email of list) {
      const row = document.createElement('div');
      row.className = 'admin-row';
      const label = document.createElement('div');
      label.className = 'email';
      label.textContent = email;
      const remove = document.createElement('button');
      remove.className = 'link-button';
      remove.type = 'button';
      remove.textContent = 'Remove';
      remove.addEventListener('click', async () => {
        list = list.filter(e => e !== email);
        render();
        await persist();
      });
      row.append(label, remove);
      rows.append(row);
    }
  }

  async function add() {
    const email = input.value.trim().toLowerCase();
    if (!email || list.includes(email)) {
      return;
    }
    list.push(email);
    input.value = '';
    render();
    await persist();
  }

  button.addEventListener('click', add);
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      add();
    }
  });
  render();
}

// One card per community app: the switch between everyone and its list, and
// the list itself while that is what the switch says. The switch and every add
// or remove save at once, together, the way the Admins list does.
function renderVisibility(apps, people) {
  const cards = document.querySelector('#visibility-cards');
  cards.replaceChildren();
  for (const app of apps) {
    cards.append(visibilityCard(app, people));
  }
}

function visibilityCard(app, people) {
  let visibility = app.visibility;
  let emails = [...app.emails];
  const byEmail = new Map(people.map(p => [p.email, p.name]));

  const card = document.createElement('div');
  card.className = 'card';
  const title = document.createElement('h2');
  title.textContent = app.name;

  const toggle = document.createElement('div');
  toggle.className = 'visibility-switch';
  toggle.setAttribute('role', 'radiogroup');
  toggle.setAttribute('aria-label', `Who sees ${app.name}`);
  for (const [value, text] of [['everyone', 'Everyone at heliosschool.org'], ['list', 'Only these people']]) {
    const label = document.createElement('label');
    const radio = document.createElement('input');
    radio.type = 'radio';
    radio.name = `visibility-${app.key}`;
    radio.value = value;
    radio.checked = visibility === value;
    radio.addEventListener('change', async () => {
      visibility = value;
      render();
      await persist();
    });
    label.append(radio, Object.assign(document.createElement('span'), {textContent: text}));
    toggle.append(label);
  }

  const note = document.createElement('div');
  note.className = 'visibility-note';
  const listWrap = document.createElement('div');
  listWrap.className = 'visibility-list';
  const rows = document.createElement('div');
  const addRow = document.createElement('div');
  addRow.className = 'add-admin-row';
  const mount = document.createElement('div');
  const button = document.createElement('button');
  button.className = 'button';
  button.type = 'button';
  button.textContent = 'Add';
  const status = document.createElement('span');
  status.className = 'save-status';
  addRow.append(mount, button, status);
  listWrap.append(rows, addRow);
  card.append(title, toggle, note, listWrap);

  const picker = createPersonPicker(mount);

  async function persist() {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    const res = await fetch('/api/admin/visibility', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({app: app.key, visibility, emails}),
    });
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    status.textContent = 'Saved.';
  }

  function render() {
    const listed = emails.length;
    if (visibility === 'everyone') {
      note.textContent = listed
        ? `Everyone sees this app. ${listed} ${listed === 1 ? 'person is' : 'people are'} on its list, kept for if you switch to only them.`
        : 'Everyone sees this app.';
    } else {
      note.textContent = listed
        ? `Only the ${listed === 1 ? 'person' : listed + ' people'} listed ${listed === 1 ? 'sees' : 'see'} this app.`
        : 'Nobody sees this app until someone is added.';
    }
    listWrap.hidden = visibility !== 'list';
    rows.replaceChildren();
    for (const email of emails) {
      const row = document.createElement('div');
      row.className = 'admin-row';
      const name = document.createElement('div');
      name.className = 'name';
      name.textContent = byEmail.get(email) || email;
      const label = document.createElement('div');
      label.className = 'email';
      label.textContent = byEmail.has(email) ? email : '';
      const remove = document.createElement('button');
      remove.className = 'link-button';
      remove.type = 'button';
      remove.textContent = 'Remove';
      remove.addEventListener('click', async () => {
        emails = emails.filter(e => e !== email);
        render();
        await persist();
      });
      row.append(name, label, remove);
      rows.append(row);
    }
    picker.setPeople(people.filter(p => !emails.includes(p.email)));
  }

  button.addEventListener('click', async () => {
    const email = picker.value;
    if (!email || emails.includes(email)) {
      return;
    }
    emails.push(email);
    picker.reset();
    render();
    await persist();
  });

  render();
  return card;
}

// The tabs on the left show one panel at a time, as Who?'s admin does.
function initTabs() {
  for (const tab of document.querySelectorAll('.tab')) {
    tab.addEventListener('click', () => {
      for (const other of document.querySelectorAll('.tab')) {
        other.classList.toggle('active', other === tab);
      }
      for (const panel of document.querySelectorAll('.panel')) {
        panel.hidden = panel.id !== 'panel-' + tab.dataset.panel;
      }
    });
  }
}

// The categories as the front page has them, read from its own model.
async function loadCategories() {
  const list = document.querySelector('#category-rows');
  const res = await fetch('/api/apps/model');
  if (!res.ok) {
    list.textContent = 'Could not load the categories.';
    return;
  }
  const model = await res.json();
  list.replaceChildren();
  for (const category of model.categories) {
    const row = document.createElement('div');
    row.className = 'category-row';
    const emoji = document.createElement('div');
    emoji.className = 'emoji' + (category.emoji ? '' : ' is-blank');
    emoji.textContent = category.emoji || category.title.slice(0, 1).toUpperCase();
    const body = document.createElement('div');
    const title = document.createElement('div');
    title.className = 'title';
    title.textContent = category.title;
    const meta = document.createElement('div');
    meta.className = 'meta';
    if (category.style === 'events') {
      meta.textContent = `Upcoming events from HCA-Team · ${(model.upcoming || []).length} ahead`;
    } else {
      const style = category.style === 'cards' ? 'Feature cards' : 'Compact tiles';
      meta.textContent = `${style} · ${category.links.length} link${category.links.length === 1 ? '' : 's'}`;
    }
    body.append(title, meta);
    row.append(emoji, body);
    list.append(row);
  }
}

async function load() {
  const res = await fetch('/api/admin/state');
  if (!res.ok) {
    document.body.innerHTML = '<p style="padding:28px;font-family:sans-serif">' +
      (res.status === 403 ? 'Admin access required.' : 'Failed to load admin state.') + '</p>';
    return;
  }
  const state = await res.json();
  document.querySelector('#me').textContent = state.email;
  const notice = document.querySelector('#store-notice');
  notice.replaceChildren();
  if (!state.hasStore) {
    const div = document.createElement('div');
    div.className = 'notice';
    div.textContent = 'Running in sample mode: image uploads are disabled because there is no media bucket configured.';
    notice.append(div);
  }
  listEditor({
    rows: document.querySelector('#admins-rows'),
    status: document.querySelector('#admins-status'),
    input: document.querySelector('#add-admin-email'),
    button: document.querySelector('#add-admin-button'),
    url: '/api/admin/admins',
    body: admins => ({admins}),
    empty: 'Nobody yet.',
    initial: state.admins,
  });
  renderVisibility(state.apps, state.people);
  loadCategories();
}

initTabs();
load();
