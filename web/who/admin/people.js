import {state} from './state.js';
import {createPersonPicker} from '/picker.js';
import {load} from './app.js';

let editingEmail = null;

export function renderAddedPeopleTable() {
  const tbody = document.querySelector('#added-people-rows');
  const added = state.people.filter(p => p.isAdded);
  tbody.replaceChildren();
  if (!added.length) {
    const row = document.createElement('tr');
    const cell = document.createElement('td');
    cell.colSpan = 6;
    cell.className = 'data-table-empty';
    cell.textContent = 'Nobody yet.';
    row.append(cell);
    tbody.append(row);
    return;
  }
  const mark = yes => yes ? '✓' : '';
  for (const p of added) {
    const row = document.createElement('tr');
    for (const text of [p.name, p.email, mark(p.isStudent), mark(p.isParent), mark(p.isStaff)]) {
      const cell = document.createElement('td');
      cell.textContent = text;
      row.append(cell);
    }
    const actionCell = document.createElement('td');
    const editButton = document.createElement('button');
    editButton.className = 'upload-button';
    editButton.type = 'button';
    editButton.textContent = 'Edit';
    editButton.addEventListener('click', () => openEditPersonModal(p));
    actionCell.append(editButton);
    row.append(actionCell);
    tbody.append(row);
  }
}

function openEditPersonModal(p) {
  editingEmail = p.email;
  document.querySelector('#edit-person-email').value = p.email;
  document.querySelector('#edit-person-full-name').value = p.name;
  document.querySelector('#edit-person-is-student').checked = p.isStudent;
  document.querySelector('#edit-person-is-parent').checked = p.isParent;
  document.querySelector('#edit-person-is-staff').checked = p.isStaff;
  const status = document.querySelector('#edit-person-status');
  status.classList.remove('error', 'ok');
  status.textContent = '';
  document.querySelector('#edit-person-modal').hidden = false;
}

function closeEditPersonModal() {
  document.querySelector('#edit-person-modal').hidden = true;
  editingEmail = null;
}

async function deleteAddedPerson(p) {
  if (!confirm(`Permanently delete ${p.name} (${p.email})? This also removes any tags or photos they have.`)) {
    return;
  }
  const res = await fetch('/api/admin/delete-person', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({email: p.email}),
  });
  if (!res.ok) {
    alert(await res.text());
    return;
  }
  await load();
  closeEditPersonModal();
}

// Never needs filtering: someone already hidden is, by definition, already gone from
// state.people (removeOptedOut in load.go drops them from the model entirely), so
// they can't show up here to be hidden a second time.
export let hidePersonPicker = null;

export function renderHiddenPeopleTable() {
  const tbody = document.querySelector('#hidden-people-rows');
  tbody.replaceChildren();
  if (!state.hiddenEmails.length) {
    const row = document.createElement('tr');
    const cell = document.createElement('td');
    cell.colSpan = 2;
    cell.className = 'data-table-empty';
    cell.textContent = 'Nobody hidden.';
    row.append(cell);
    tbody.append(row);
    return;
  }
  for (const hiddenEmail of state.hiddenEmails) {
    const row = document.createElement('tr');
    const emailCell = document.createElement('td');
    emailCell.textContent = hiddenEmail;
    row.append(emailCell);
    const actionCell = document.createElement('td');
    const unhideButton = document.createElement('button');
    unhideButton.className = 'upload-button';
    unhideButton.type = 'button';
    unhideButton.textContent = 'Unhide';
    unhideButton.addEventListener('click', async () => {
      unhideButton.disabled = true;
      const res = await fetch('/api/admin/unhide-person', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({email: hiddenEmail}),
      });
      unhideButton.disabled = false;
      if (!res.ok) {
        alert(await res.text());
        return;
      }
      await load();
    });
    actionCell.append(unhideButton);
    row.append(actionCell);
    tbody.append(row);
  }
}

export function initPeople() {
  document.querySelector('#delete-person-button').addEventListener('click', () => {
    if (!editingEmail) {
      return;
    }
    const person = state.people.find(p => p.email === editingEmail);
    if (person) {
      deleteAddedPerson(person);
    }
  });

  document.querySelector('#close-edit-person-button').addEventListener('click', closeEditPersonModal);
  document.querySelector('#edit-person-modal').addEventListener('click', e => {
    if (e.target.id === 'edit-person-modal') {
      closeEditPersonModal();
    }
  });

  document.querySelector('#edit-person-button').addEventListener('click', async () => {
    if (!editingEmail) {
      return;
    }
    const status = document.querySelector('#edit-person-status');
    const button = document.querySelector('#edit-person-button');
    const body = {
      email: editingEmail,
      newEmail: document.querySelector('#edit-person-email').value.trim(),
      fullName: document.querySelector('#edit-person-full-name').value.trim(),
      isStudent: document.querySelector('#edit-person-is-student').checked,
      isParent: document.querySelector('#edit-person-is-parent').checked,
      isStaff: document.querySelector('#edit-person-is-staff').checked,
    };
    button.disabled = true;
    status.classList.remove('error', 'ok');
    status.textContent = 'Saving…';
    const res = await fetch('/api/admin/added-fields', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body),
    });
    button.disabled = false;
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    await load();
    closeEditPersonModal();
  });

  document.querySelector('#open-add-person-button').addEventListener('click', () => {
    document.querySelector('#add-person-modal').hidden = false;
  });
  document.querySelector('#close-add-person-button').addEventListener('click', () => {
    document.querySelector('#add-person-modal').hidden = true;
  });
  document.querySelector('#add-person-modal').addEventListener('click', e => {
    if (e.target.id === 'add-person-modal') {
      document.querySelector('#add-person-modal').hidden = true;
    }
  });

  hidePersonPicker = createPersonPicker(document.querySelector('#hide-person-select'));

  document.querySelector('#hide-person-button').addEventListener('click', async () => {
    const status = document.querySelector('#hide-person-status');
    const button = document.querySelector('#hide-person-button');
    const email = hidePersonPicker.value;
    if (!email) {
      return;
    }
    button.disabled = true;
    status.classList.remove('error', 'ok');
    status.textContent = 'Hiding…';
    const res = await fetch('/api/admin/hide-person', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({email}),
    });
    button.disabled = false;
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    hidePersonPicker.reset();
    status.textContent = '';
    await load();
  });

  document.querySelector('#add-person-button').addEventListener('click', async () => {
    const status = document.querySelector('#add-person-status');
    const button = document.querySelector('#add-person-button');
    const body = {
      email: document.querySelector('#add-person-email').value.trim(),
      fullName: document.querySelector('#add-person-full-name').value.trim(),
      isStudent: document.querySelector('#add-person-is-student').checked,
      isParent: document.querySelector('#add-person-is-parent').checked,
      isStaff: document.querySelector('#add-person-is-staff').checked,
    };
    button.disabled = true;
    status.classList.remove('error', 'ok');
    status.textContent = 'Adding…';
    const res = await fetch('/api/admin/add-person', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body),
    });
    button.disabled = false;
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    document.querySelector('#add-person-email').value = '';
    document.querySelector('#add-person-full-name').value = '';
    document.querySelector('#add-person-is-student').checked = false;
    document.querySelector('#add-person-is-parent').checked = false;
    document.querySelector('#add-person-is-staff').checked = false;
    status.textContent = '';
    document.querySelector('#add-person-modal').hidden = true;
    await load();
  });
}
