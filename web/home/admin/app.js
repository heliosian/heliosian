
let admins = [];

const rows = document.querySelector('#admins-rows');
const status = document.querySelector('#admins-status');
const input = document.querySelector('#add-admin-email');

// Every add or remove posts immediately, so nothing looks saved that isn't.
async function persist() {
  status.classList.remove('error');
  status.textContent = 'Saving…';
  const res = await fetch('/api/admin/admins', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({admins}),
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
  if (!admins.length) {
    const empty = document.createElement('div');
    empty.className = 'hint';
    empty.textContent = 'Nobody yet.';
    rows.append(empty);
  }
  for (const email of admins) {
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
      admins = admins.filter(e => e !== email);
      render();
      await persist();
    });
    row.append(label, remove);
    rows.append(row);
  }
}

async function add() {
  const email = input.value.trim().toLowerCase();
  if (!email || admins.includes(email)) {
    return;
  }
  admins.push(email);
  input.value = '';
  render();
  await persist();
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
  admins = state.admins;
  const notice = document.querySelector('#store-notice');
  notice.replaceChildren();
  if (!state.hasStore) {
    const div = document.createElement('div');
    div.className = 'notice';
    div.textContent = 'Running in sample mode: image uploads are disabled because there is no media bucket configured.';
    notice.append(div);
  }
  render();
  loadCategories();
}

initTabs();
document.querySelector('#add-admin-button').addEventListener('click', add);
input.addEventListener('keydown', e => {
  if (e.key === 'Enter') {
    e.preventDefault();
    add();
  }
});
load();
