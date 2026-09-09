import {state, applyState} from './state.js';
import {renderTopLevelSpoofBanner, ensureSuperAdminUI, renderSpoofPanel} from './spoof.js';
import {buildAdminListEditor} from './admins.js';
import {renderImages} from './images.js';
import {overridesPanels, initOverrides} from './overrides.js';
import {renderAddedPeopleTable, renderHiddenPeopleTable, hidePersonPicker, initPeople} from './people.js';
import {initSettings} from './settings.js';
import {initSidebar} from './sidebar.js';

export async function load() {
  const res = await fetch('/api/admin/state');
  if (!res.ok) {
    document.body.innerHTML = '<p style="padding:28px;font-family:sans-serif">' +
      (res.status === 403 ? 'Admin access required.' : 'Failed to load admin state.') + '</p>';
    return;
  }
  applyState(await res.json());
  render();
}

function render() {
  document.querySelector('#me').textContent = state.email;
  renderTopLevelSpoofBanner();

  const notice = document.querySelector('#store-notice');
  notice.innerHTML = '';
  if (!state.hasStore) {
    const div = document.createElement('div');
    div.className = 'notice';
    div.textContent = 'Running in sample mode: image uploads are disabled because there is no media bucket configured.';
    notice.append(div);
  }

  document.querySelector('#staff-color').value = state.staffColor || '#1f4d53';

  renderImages('#classrooms', state.classrooms, 'classroom');
  renderImages('#grades', state.grades, 'grade');

  document.querySelector('#years-photo').value = state.staleYears.photo;
  document.querySelector('#years-facts').value = state.staleYears.facts;
  document.querySelector('#years-family-photo').value = state.staleYears.familyPhoto;

  document.querySelector('#privacy-veracross-url').value = state.privacyLinks.veracrossPreferences;
  document.querySelector('#privacy-optin-url').value = state.privacyLinks.heliosWhoOptIn;

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
      '#super-admins-rows', '#add-super-admin-select', '#super-admins-status', '/api/admin/super-admins', 'superAdmins', state.superAdmins);
    renderSpoofPanel();
  }
}

initOverrides();
initPeople();
initSettings();
initSidebar();
load();
