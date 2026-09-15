import {state} from './state.js';

// A super admin's response carries fields (isSuperAdmin, superAdmins) that a
// regular admin's response never does, so this tab and panel only come into
// existence for a caller the server has already told is a super admin — there
// is nothing for a regular admin's page to hide, because it was never built.
// Spoof Mode, which used to sit beside it, is the toolbar's switch now, in
// every app (web/common/toolbar.js).
export function ensureSuperAdminUI() {
  if (!state.isSuperAdmin || document.querySelector('[data-panel="super-admins"]')) {
    return;
  }
  // Lands in the Editing & Control section, in front of Admins - the one
  // static tab already there.
  const editingControl = document.querySelector('#sidebar-editing-control');
  const adminsTab = editingControl.querySelector('[data-panel="admins"]');
  const superAdminsTab = document.createElement('div');
  superAdminsTab.className = 'tab';
  superAdminsTab.dataset.panel = 'super-admins';
  superAdminsTab.textContent = 'Super Admins';
  editingControl.insertBefore(superAdminsTab, adminsTab);

  const container = document.querySelector('.container');

  const superAdminsPanel = document.createElement('div');
  superAdminsPanel.className = 'panel';
  superAdminsPanel.id = 'panel-super-admins';
  superAdminsPanel.hidden = true;
  superAdminsPanel.innerHTML = `
    <div class="card">
      <h2>Super Admins</h2>
      <div class="hint">Super admins can also use Spoof Mode, from the eye beside their avatar in any app's toolbar, and manage this list. Regular admins never see this tab. Changes save immediately.</div>
      <div id="super-admins-rows"></div>
      <div class="add-admin-row">
        <div id="add-super-admin-select"></div>
        <button class="upload-button" id="add-super-admin-button" type="button">Add</button>
        <span class="save-status" id="super-admins-status"></span>
      </div>
    </div>`;
  container.append(superAdminsPanel);
}
