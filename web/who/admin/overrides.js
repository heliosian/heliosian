import {state} from './state.js';
import {createPersonPicker} from '/picker.js';
import {load} from './app.js';

export const overridesPanels = [];

// Replaces a <select>'s options with an empty "— none —" choice plus one per value,
// then selects current (falling back to empty if current isn't among values - e.g.
// after switching classrooms, the previous crew rarely belongs to the new one).
// current is always included as an option even when it's missing from values - e.g. a
// Crew override can exist with no matching Classroom override (deriveClassrooms only
// checks Crew against Classroom's crews when a Classroom is actually set), so the
// options crewNamesFor('') computes for an empty Classroom box wouldn't otherwise
// contain the real Crew override at all. Without this, fillSelect would silently fall
// back to "— none —", both hiding a real override from view and, if saved right after,
// erasing it.
function fillSelect(select, values, current) {
  select.replaceChildren();
  const empty = document.createElement('option');
  empty.value = '';
  empty.textContent = '— none —';
  select.append(empty);
  const options = current && !values.includes(current) ? [current, ...values] : values;
  for (const value of options) {
    const opt = document.createElement('option');
    opt.value = value;
    opt.textContent = value;
    select.append(opt);
  }
  select.value = current || '';
}

// With no classroom chosen, every crew is offered rather than none - a Crew override
// can legitimately exist with no Classroom override (validCrew in admin.go matches
// deriveClassrooms' own rule: it only checks crew-against-classroom when a classroom
// is actually given), so leaving Classroom blank shouldn't make every crew name look
// invalid.
function crewNamesFor(classroomName) {
  const crews = classroomName ? state.crews.filter(c => c.classroom === classroomName) : state.crews;
  return [...new Set(crews.map(c => c.name))];
}

function veracrossLabel(value) {
  return 'Veracross: ' + (value ? value : '— none —');
}

// Builds the shared "pick a person → edit some Overrides fields → save" behavior for
// one Overrides panel (Staff/Student/Parent). The HTML for each panel is hand-authored
// (element ids follow `${prefix}-${field.id}` and `${prefix}-${field.id}-current`) -
// only the field list, person filter, and save endpoint differ between panels.
//
// Each person from the server carries TWO views of every field (see admin.go's
// overridableFields): `override` is what's literally in the Overrides sheet right now
// (booleans travel as "TRUE"/"FALSE"/""), and `veracross` is what the Student/Staff
// Import actually supplies. A field's own box is pre-filled from `override` - so an
// untouched box never introduces a new override, and clearing one that DOES carry an
// override removes exactly that override - while the small label underneath shows
// `veracross`, purely for reference. field.key indexes both (they share overridableFields'
// JSON keys), and is also the request body key on save.
//
// field.kind is 'text' (also covers <textarea>, which shares the same .value API),
// 'checkbox', or one of the select kinds ('classroom', 'crew', 'department', 'grade',
// 'band') whose options come from `state`. 'crew' additionally depends on whichever
// field has kind 'classroom' in the same panel, refetching its options whenever that
// field's selection changes - mirroring deriveClassrooms' own Classroom/Crew coupling.
function bindOverridesPanel(spec) {
  let selectedEmail = null;
  const el = suffix => document.querySelector(`#${spec.prefix}-${suffix}`);
  const classroomField = spec.fields.find(f => f.kind === 'classroom');
  const selectSources = {
    classroom: () => state.classrooms.map(c => c.name),
    crew: () => crewNamesFor(classroomField ? el(classroomField.id).value : ''),
    department: () => state.departments,
    grade: () => state.grades.map(g => g.name),
    band: () => state.bands,
  };

  function loadPerson(email, resetStatus = true) {
    const person = state.people.find(p => p.email === email);
    if (!person) {
      return;
    }
    selectedEmail = email;
    el('fields-card').hidden = false;
    el('fields-name').textContent = person.name;
    el('fields-email').textContent = person.email;
    for (const f of spec.fields) {
      const overrideValue = person.override[f.key] || '';
      const veracrossValue = person.veracross[f.key] || '';
      const input = el(f.id);
      if (f.kind === 'checkbox') {
        input.checked = overrideValue === 'TRUE';
      } else if (selectSources[f.kind]) {
        fillSelect(input, selectSources[f.kind](), overrideValue);
      } else {
        input.value = overrideValue;
      }
      const currentEl = el(f.id + '-current');
      if (currentEl) {
        const label = f.kind === 'checkbox' ? {TRUE: 'Yes', FALSE: 'No'}[veracrossValue] || '' : veracrossValue;
        currentEl.textContent = veracrossLabel(label);
      }
    }
    if (resetStatus) {
      const status = el('fields-status');
      status.classList.remove('error', 'ok');
      status.textContent = '';
    }
  }

  const picker = createPersonPicker(el('person-select'));
  el('person-load-button').addEventListener('click', () => {
    if (picker.value) {
      loadPerson(picker.value);
    }
  });
  if (classroomField) {
    const crewField = spec.fields.find(f => f.kind === 'crew');
    el(classroomField.id).addEventListener('change', e => {
      if (crewField) {
        fillSelect(el(crewField.id), crewNamesFor(e.target.value), '');
      }
    });
  }
  el('save-button').addEventListener('click', async () => {
    if (!selectedEmail) {
      return;
    }
    const status = el('fields-status');
    const button = el('save-button');
    const body = {email: selectedEmail};
    for (const f of spec.fields) {
      const input = el(f.id);
      // Not trimmed here: a lone space is how an admin explicitly force-blanks a
      // field that has a Veracross default (see diffStringCell in admin.go) - an
      // empty box instead removes the override outright, falling back to Veracross.
      // Every server-side field handles its own whitespace normalization.
      body[f.key] = f.kind === 'checkbox' ? input.checked : input.value;
    }
    button.disabled = true;
    status.classList.remove('error', 'ok');
    status.textContent = 'Saving…';
    const res = await fetch(spec.endpoint, {
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
    status.classList.add('ok');
    status.textContent = 'Saved.';
    await load();
  });

  const panel = {
    refreshPeople() {
      picker.setPeople(state.people.filter(spec.filter));
    },
    // resetStatus is false here, so a post-save refresh redraws the form with the
    // now-persisted values without stomping on the "Saved."/error message the save
    // handler just set - only an explicit Load (a new person, or the picker) should
    // clear a stale status.
    refreshSelected() {
      if (selectedEmail) {
        loadPerson(selectedEmail, false);
      }
    },
    // load lets something other than this panel's own picker (e.g. a table row
    // elsewhere on the same tab) jump straight to editing a known person.
    load(email) {
      loadPerson(email);
    },
  };
  overridesPanels.push(panel);
  return panel;
}

export function initOverrides() {
  bindOverridesPanel({
    prefix: 'staff',
    endpoint: '/api/admin/person-fields',
    filter: p => p.isStaff,
    fields: [
      {key: 'fullName', id: 'full-name', kind: 'text'},
      {key: 'legalName', id: 'legal-name', kind: 'text'},
      {key: 'preferredName', id: 'preferred-name', kind: 'text'},
      {key: 'classroom', id: 'classroom', kind: 'classroom'},
      {key: 'crew', id: 'crew', kind: 'crew'},
      {key: 'department', id: 'department', kind: 'department'},
      {key: 'jobTitle', id: 'job-title', kind: 'text'},
      {key: 'gradeBand', id: 'grade-band', kind: 'band'},
      {key: 'facts', id: 'facts', kind: 'text'},
    ],
  });

  bindOverridesPanel({
    prefix: 'student',
    endpoint: '/api/admin/student-fields',
    filter: p => p.isStudent,
    fields: [
      {key: 'fullName', id: 'full-name', kind: 'text'},
      {key: 'legalName', id: 'legal-name', kind: 'text'},
      {key: 'preferredName', id: 'preferred-name', kind: 'text'},
      {key: 'grade', id: 'grade', kind: 'grade'},
      {key: 'classroom', id: 'classroom', kind: 'classroom'},
      {key: 'crew', id: 'crew', kind: 'crew'},
    ],
  });

  bindOverridesPanel({
    prefix: 'parent',
    endpoint: '/api/admin/parent-fields',
    filter: p => p.isParent,
    fields: [
      {key: 'fullName', id: 'full-name', kind: 'text'},
      {key: 'legalName', id: 'legal-name', kind: 'text'},
      {key: 'preferredName', id: 'preferred-name', kind: 'text'},
      {key: 'phone', id: 'phone', kind: 'text'},
      {key: 'roomParent', id: 'room-parent', kind: 'band'},
      {key: 'address', id: 'address', kind: 'text'},
    ],
  });
}
