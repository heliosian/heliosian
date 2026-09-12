import {state} from './state.js';
import {createPersonPicker} from '/picker.js';

// Shared by the Admins and Super Admins panels: a working copy of the list, a picker
// to add from, and a POST that fires immediately on every add or remove — no separate
// save step to forget, since that's exactly what caused an add to silently not count
// (the person looked added in the list, but the server never heard about it until an
// explicit Save that hadn't been clicked).
export function buildAdminListEditor(rowsSelector, selectSelector, statusSelector, url, bodyKey, initial) {
  let pending = [...initial];
  const rowsEl = document.querySelector(rowsSelector);
  const selectEl = document.querySelector(selectSelector);
  const statusEl = document.querySelector(statusSelector);

  async function persist() {
    statusEl.classList.remove('error', 'ok');
    statusEl.textContent = 'Saving…';
    const res = await fetch(url, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({[bodyKey]: pending}),
    });
    if (!res.ok) {
      statusEl.classList.add('error');
      statusEl.textContent = await res.text();
      return;
    }
    statusEl.classList.remove('error');
    statusEl.classList.add('ok');
    statusEl.textContent = 'Saved.';
  }

  function renderRows() {
    rowsEl.replaceChildren();
    if (!pending.length) {
      const empty = document.createElement('div');
      empty.className = 'hint';
      empty.textContent = 'Nobody yet.';
      rowsEl.append(empty);
    }
    for (const email of pending) {
      const person = state.people.find(p => p.email === email);
      const row = document.createElement('div');
      row.className = 'admin-row';
      const name = document.createElement('div');
      name.className = 'name';
      name.textContent = person ? person.name : email;
      const emailEl = document.createElement('div');
      emailEl.className = 'email';
      emailEl.textContent = email;
      const remove = document.createElement('button');
      remove.className = 'remove-button';
      remove.type = 'button';
      remove.textContent = 'Remove';
      remove.addEventListener('click', async () => {
        pending = pending.filter(e => e !== email);
        renderRows();
        renderSelect();
        await persist();
      });
      row.append(name, emailEl, remove);
      rowsEl.append(row);
    }
  }

  const picker = createPersonPicker(selectEl);

  function renderSelect() {
    picker.setPeople(state.people.filter(p => !pending.includes(p.email)));
  }

  renderRows();
  renderSelect();

  const addButton = selectEl.parentElement.querySelector('.upload-button');
  addButton.onclick = async () => {
    const email = picker.value;
    if (email && !pending.includes(email)) {
      pending.push(email);
      picker.reset();
      renderRows();
      renderSelect();
      await persist();
    }
  };
}
