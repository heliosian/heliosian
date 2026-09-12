import {state} from './state.js';
import {createPersonPicker} from '/picker.js';

let spoofPicker = null;

// The escape hatch for viewing the admin tools as someone else: spoofingAs is always
// keyed on the real admin regardless of the simulated tier, so this shows (and offers
// a way out) even on a regular admin's simulated page, which has no Spoof Mode tab of
// its own to stop it from.
export function renderTopLevelSpoofBanner() {
  let banner = document.querySelector('.spoof-banner');
  if (!state.spoofingAs) {
    if (banner) {
      banner.remove();
    }
    return;
  }
  if (banner) {
    banner.querySelector('.spoof-banner-name').textContent = state.spoofingAs;
    return;
  }
  banner = document.createElement('div');
  banner.className = 'spoof-banner';
  const label = document.createElement('span');
  label.textContent = 'Viewing admin tools as ';
  banner.append(label);
  const name = document.createElement('span');
  name.className = 'spoof-banner-name';
  name.textContent = state.spoofingAs;
  banner.append(name);
  const link = document.createElement('a');
  link.href = '#';
  link.textContent = 'Stop';
  link.addEventListener('click', async e => {
    e.preventDefault();
    await fetch('/api/admin/spoof', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({email: ''}),
    });
    location.reload();
  });
  banner.append(link);
  document.body.append(banner);
}

// A super admin's response carries fields (isSuperAdmin, superAdmins, spoofingAs) that
// a regular admin's response never does, so this tab and panel only come into
// existence for a caller the server has already told is a super admin — there is
// nothing for a regular admin's page to hide, because it was never built.
export function ensureSuperAdminUI() {
  if (!state.isSuperAdmin || document.querySelector('[data-panel="spoof"]')) {
    return;
  }
  // Both land in the Editing & Control section, in requested order (Super Admins,
  // Admins, Spoof Mode) - Admins is the one static tab already there, so Super Admins
  // goes in front of it and Spoof Mode after.
  const editingControl = document.querySelector('#sidebar-editing-control');
  const adminsTab = editingControl.querySelector('[data-panel="admins"]');
  const superAdminsTab = document.createElement('div');
  superAdminsTab.className = 'tab';
  superAdminsTab.dataset.panel = 'super-admins';
  superAdminsTab.textContent = 'Super Admins';
  editingControl.insertBefore(superAdminsTab, adminsTab);

  const spoofTab = document.createElement('div');
  spoofTab.className = 'tab';
  spoofTab.dataset.panel = 'spoof';
  spoofTab.textContent = 'Spoof Mode';
  editingControl.append(spoofTab);

  const container = document.querySelector('.container');

  const superAdminsPanel = document.createElement('div');
  superAdminsPanel.className = 'panel';
  superAdminsPanel.id = 'panel-super-admins';
  superAdminsPanel.hidden = true;
  superAdminsPanel.innerHTML = `
    <div class="card">
      <h2>Super Admins</h2>
      <div class="hint">Super admins can also use Spoof Mode and manage this list. Regular admins never see this tab. Changes save immediately.</div>
      <div id="super-admins-rows"></div>
      <div class="add-admin-row">
        <div id="add-super-admin-select"></div>
        <button class="upload-button" id="add-super-admin-button" type="button">Add</button>
        <span class="save-status" id="super-admins-status"></span>
      </div>
    </div>`;
  container.append(superAdminsPanel);

  const spoofPanel = document.createElement('div');
  spoofPanel.className = 'panel';
  spoofPanel.id = 'panel-spoof';
  spoofPanel.hidden = true;
  spoofPanel.innerHTML = `
    <div class="card">
      <h2>Spoof Mode</h2>
      <div class="hint">View and use the directory and admin tools exactly as someone else would, including their edit permissions and admin tier.</div>
      <div id="spoof-active" hidden>
        <div class="admin-row">
          <div class="name" id="spoof-active-name"></div>
          <button class="remove-button" id="stop-spoof-button" type="button">Stop viewing as them</button>
        </div>
      </div>
      <div id="spoof-picker" class="add-admin-row">
        <div id="spoof-select"></div>
        <button class="upload-button" id="start-spoof-button" type="button">View As</button>
      </div>
      <span class="save-status" id="spoof-status"></span>
    </div>`;
  container.append(spoofPanel);

  spoofPicker = createPersonPicker(document.querySelector('#spoof-select'));
  document.querySelector('#start-spoof-button').addEventListener('click', () => setSpoof(spoofPicker.value));
  document.querySelector('#stop-spoof-button').addEventListener('click', () => setSpoof(''));
}

export function renderSpoofPanel() {
  const active = document.querySelector('#spoof-active');
  const picker = document.querySelector('#spoof-picker');
  if (state.spoofingAs) {
    active.hidden = false;
    picker.hidden = true;
    document.querySelector('#spoof-active-name').textContent = 'Viewing as ' + state.spoofingAs;
  } else {
    active.hidden = true;
    picker.hidden = false;
    spoofPicker.setPeople(state.people);
  }
}

async function setSpoof(email) {
  const status = document.querySelector('#spoof-status');
  status.classList.remove('error', 'ok');
  status.textContent = email ? 'Switching…' : 'Stopping…';
  const res = await fetch('/api/admin/spoof', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({email}),
  });
  if (!res.ok) {
    status.classList.add('error');
    status.textContent = await res.text();
    return;
  }
  // A full reload, not load(): ensureSuperAdminUI() only ever adds the Super Admins
  // and Spoof Mode tabs, it never tears them down, so switching to a regular admin's
  // view without reloading left them stranded in the DOM from the pre-spoof render —
  // exactly the tier leak a regular admin must never see.
  //
  // Starting a spoof sends the admin to /people instead of reloading in place: the
  // spoofed person is very often not an admin, so reloading this page hits the admin
  // check as them and errors out.
  if (email) {
    location.href = '/people';
  } else {
    location.reload();
  }
}
