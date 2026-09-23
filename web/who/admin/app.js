import {state, applyState} from './state.js';
import {ensureSuperAdminUI} from './superadmins.js';
import {buildAdminListEditor} from './admins.js';
import {renderImages} from './images.js';
import {overridesPanels, initOverrides} from './overrides.js';
import {renderAddedPeopleTable, renderHiddenPeopleTable, hidePersonPicker, initPeople} from './people.js';
import {initSettings} from './settings.js';
import {initSidebar} from './sidebar.js';

// The settings, colors, and super admin list live in the platform config, not the
// directory: they come from /api/config, and the super admin list only from its own
// endpoint, which answers anyone but a super admin with a 403.
export async function load() {
  const [res, configRes] = await Promise.all([fetch('/api/admin/state'), fetch('/api/config')]);
  if (!res.ok || !configRes.ok) {
    document.body.innerHTML = '<p style="padding:28px;font-family:sans-serif">' +
      (res.status === 403 ? 'Admin access required.' : 'Failed to load admin state.') + '</p>';
    return;
  }
  const next = await res.json();
  next.config = await configRes.json();
  if (next.isSuperAdmin) {
    const superRes = await fetch('/api/config/super-admins');
    if (!superRes.ok) {
      document.body.innerHTML = '<p style="padding:28px;font-family:sans-serif">Failed to load admin state.</p>';
      return;
    }
    next.superAdmins = (await superRes.json()).superAdmins;
  }
  applyState(next);
  render();
}

function render() {
  document.querySelector('#me').textContent = state.email;

  document.querySelector('#staff-color').value = state.config.staffColor;

  renderImages('#classrooms', state.classrooms, 'classroom', state.config.classroomColors);
  renderImages('#grades', state.grades, 'grade', state.config.gradeColors);

  document.querySelector('#years-photo').value = state.config.staleYears.photo;
  document.querySelector('#years-facts').value = state.config.staleYears.facts;
  document.querySelector('#years-family-photo').value = state.config.staleYears.familyPhoto;

  document.querySelector('#privacy-veracross-url').value = state.config.privacyLinks.veracrossPreferences;
  document.querySelector('#privacy-optin-url').value = state.config.privacyLinks.heliosWhoOptIn;

  buildAdminListEditor(
    '#admins-rows', '#add-admin-select', '#admins-status', '/api/admin/admins', 'admins', state.admins);

  for (const panel of overridesPanels) {
    panel.refreshPeople();
    panel.refreshSelected();
  }
  renderAddedPeopleTable();
  hidePersonPicker.setPeople(state.people);
  renderHiddenPeopleTable();

  ensureSuperAdminUI();
  if (state.isSuperAdmin) {
    buildAdminListEditor(
      '#super-admins-rows', '#add-super-admin-select', '#super-admins-status', '/api/config/super-admins', 'superAdmins', state.superAdmins);
  }
}

initOverrides();
initPeople();
initSettings();
initSidebar();
load();
