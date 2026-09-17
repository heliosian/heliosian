import {state, me, isAdmin, options, groupPath} from '../state.js';
import {el, svg, link, button, iconButton, copyText, toast, personRow, pageHead, thumb} from '../dom.js';
import {setTitle} from '../chrome.js';
import {load, navigate} from '../app.js';
import {createPersonPicker} from '/picker.js';
import {tabStrip, tabParam, tabHref} from '/tabs.js';
import {appOrigin} from '/toolbar.js';
import {visibilityWords} from './groups.js';

const visibilityNotes = {
  hidden: 'Only the group\'s managers and the admins see it.',
  members: 'Anyone the rules or the additions place on the group can see it, and take themselves off it or put themselves back; only managers can change it.',
  everyone: 'Anyone in Loop can see it, and only managers can change it.',
};

function chipToggle(label, on, onChange) {
  const b = el('button', 'chip-toggle' + (on ? ' active' : ''), label);
  b.type = 'button';
  b.addEventListener('click', () => {
    b.classList.toggle('active');
    onChange(b.classList.contains('active'));
  });
  return b;
}

// facetDropdown is a button opening a checklist, as Who?'s Grade and
// Classroom dropdowns are: values are strings or {value, label, icon}.
function facetDropdown(label, icon, values, chosen, onChange) {
  const wrap = el('div', 'facet-wrap');
  const b = el('button', 'facet-button');
  b.type = 'button';
  const labelSpan = el('span', '', label);
  if (icon) {
    b.append(svg(icon));
  }
  b.append(labelSpan, svg('chevron'));
  const panel = el('div', 'facet-panel');
  panel.hidden = true;
  const updateLabel = () => {
    labelSpan.textContent = chosen.size ? `${label} (${chosen.size})` : label;
  };
  b.addEventListener('click', () => {
    const opening = panel.hidden;
    for (const other of document.querySelectorAll('.facet-panel')) {
      other.hidden = true;
    }
    for (const open of document.querySelectorAll('.facet-button.open')) {
      open.classList.remove('open');
    }
    panel.hidden = !opening;
    b.classList.toggle('open', opening);
  });
  if (!values.length) {
    panel.append(el('div', 'facet-empty', 'Nothing to choose yet.'));
  }
  for (const v of values) {
    const {value, label: text, icon: mark} = typeof v === 'string' ? {value: v, label: v} : v;
    const row = el('label', 'facet-option');
    const box = el('input');
    box.type = 'checkbox';
    box.checked = chosen.has(value);
    box.addEventListener('change', () => {
      if (box.checked) {
        chosen.add(value);
      } else {
        chosen.delete(value);
      }
      updateLabel();
      onChange();
    });
    if (mark) {
      row.append(svg(mark));
    }
    row.append(el('span', '', text), box);
    panel.append(row);
  }
  const foot = el('div', 'facet-foot');
  foot.append(button('Done', null, 'button button-small', () => {
    panel.hidden = true;
    b.classList.remove('open');
  }));
  panel.append(foot);
  updateLabel();
  wrap.append(b, panel);
  return wrap;
}

const listIcons = {party: 'party', activity: 'activity', room: 'classrooms'};

// ruleRow is one rule in the editor: its facets, each a control, and the
// way to drop it. A rule another manager wrote is shown, not edited: its
// tags are theirs to read.
function ruleRow(rule, onChange, onRemove) {
  const row = el('div', 'rule');
  const mine = rule.owner === me().email;
  const controls = el('div', 'rule-controls');
  if (!mine) {
    row.classList.add('is-theirs');
    const words = [];
    if (rule.roles.length) {
      words.push(rule.roles.join(', '));
    }
    if (rule.search) {
      words.push(`“${rule.search}”`);
    }
    if (rule.classrooms.length) {
      words.push(rule.classrooms.join(', '));
    }
    if (rule.grades.length) {
      words.push(rule.grades.join(', '));
    }
    if (rule.tags.length) {
      words.push('tags: ' + rule.tagLabels.join(', '));
    }
    if (rule.family.length) {
      words.push('plus their ' + rule.family.map(f => f.toLowerCase()).join(', '));
    }
    const owner = state.model.people.find(p => p.email === rule.owner);
    controls.append(el('div', 'rule-words', words.join(' · ')));
    controls.append(el('div', 'rule-note', `Written by ${owner ? owner.name : rule.owner}, reading their tags; it can be removed but not changed.`));
  } else {
    const roles = el('div', 'chip-row');
    for (const role of options().roles) {
      roles.append(chipToggle(role + 's', rule.roles.includes(role), on => {
        rule.roles = on ? [...rule.roles, role] : rule.roles.filter(r => r !== role);
        onChange();
      }));
    }
    controls.append(roles);
    const search = el('input', 'rule-search');
    search.type = 'search';
    search.placeholder = 'Words in a name or address';
    search.maxLength = 80;
    search.value = rule.search;
    let timer;
    search.addEventListener('input', () => {
      rule.search = search.value.trim();
      clearTimeout(timer);
      timer = setTimeout(onChange, 300);
    });
    controls.append(search);
    const classrooms = new Set(rule.classrooms);
    controls.append(facetDropdown('Classroom', null, options().classrooms, classrooms, () => {
      rule.classrooms = [...classrooms];
      onChange();
    }));
    const grades = new Set(rule.grades);
    controls.append(facetDropdown('Grade', null, options().grades, grades, () => {
      rule.grades = [...grades];
      onChange();
    }));
    const tags = new Set(rule.tags);
    const tagValues = [
      ...options().tags,
      ...options().shared.map(s => ({value: s.key, label: `${s.name} (${s.ownerName}'s)`, icon: 'tag'})),
      ...options().lists.map(l => ({value: l.key, label: l.name, icon: listIcons[l.kind]})),
    ];
    controls.append(facetDropdown('Tags', 'tag', tagValues, tags, () => {
      rule.tags = [...tags];
      onChange();
    }));
    const family = new Set(rule.family);
    controls.append(facetDropdown('Add family', 'families', options().relations.map(r => ({value: r, label: 'Their ' + r.toLowerCase()})), family, () => {
      rule.family = [...family];
      onChange();
    }));
  }
  row.append(controls);
  row.append(iconButton('trash', 'Remove this rule', 'rule-remove', onRemove));
  return row;
}

function newRule(kind) {
  return {kind, roles: [], search: '', classrooms: [], grades: [], tags: [], family: [], owner: me().email};
}

async function send(method, url, body) {
  const res = await fetch(url, {method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.status === 204 ? null : res.json();
}

// editor is the group form, for a new group and an existing one alike: the
// words, the managers, the include and exclude rules, the people added by
// hand from outside the directory, and a preview of who all that picks
// out, asked of the server as it changes.
function editor(g, isNew) {
  const draft = {
    name: g.name, aliases: [...(g.aliases || [])], title: g.title, description: g.description || '',
    prefix: isNew ? true : g.prefix,
    visibility: isNew ? 'hidden' : g.visibility,
    managers: g.managers.map(m => m.email),
    rules: g.rules.map(r => ({kind: r.kind, roles: [...r.roles], search: r.search, classrooms: [...r.classrooms], grades: [...r.grades], tags: [...r.tags], family: [...r.family], owner: r.owner, tagLabels: r.tagLabels})),
    additions: (g.additions || []).map(a => ({email: a.email, name: a.name})),
    excluded: (g.excluded || []).map(e => ({email: e.email, note: e.note || '', when: e.when || ''})),
  };
  if (isNew) {
    draft.rules.push(newRule('include'));
  }
  const form = el('form', 'editor');
  form.addEventListener('submit', e => e.preventDefault());

  const words = el('div', 'card');
  words.append(el('h2', '', isNew ? 'A new group' : 'The group'));
  const titleField = el('label', 'field');
  const title = el('input');
  title.type = 'text';
  title.required = true;
  title.maxLength = 80;
  title.value = draft.title;
  title.placeholder = 'Soccer Team Families';
  title.addEventListener('input', () => {
    draft.title = title.value;
    if (isNew && !nameTouched) {
      name.value = slug(title.value);
      draft.name = name.value;
      updateAddress();
    }
  });
  titleField.append(el('span', '', 'Title'), title);
  words.append(titleField);
  const nameField = el('label', 'field');
  const name = el('input');
  name.type = 'text';
  name.maxLength = 40;
  name.value = draft.name;
  name.placeholder = 'soccer-team';
  name.disabled = !isNew;
  let nameTouched = false;
  const addressNote = el('small');
  const updateAddress = () => {
    addressNote.textContent = draft.name ? `${draft.name}@${state.model.domain}` : `The address is <name>@${state.model.domain}, and cannot change once made.`;
  };
  name.addEventListener('input', () => {
    nameTouched = true;
    draft.name = slug(name.value);
    updateAddress();
  });
  nameField.append(el('span', '', 'Address'), name, addressNote);
  updateAddress();
  words.append(nameField);
  // The aliases: other local parts that reach the group, each unique
  // across every group's name and alias.
  const aliasField = el('div', 'field');
  aliasField.append(el('span', '', 'Also answers as'));
  const aliasRows = el('div', 'alias-rows');
  const renderAliases = () => {
    aliasRows.replaceChildren();
    if (!draft.aliases.length) {
      aliasRows.append(el('div', 'rule-empty', 'No other addresses.'));
    }
    for (const alias of draft.aliases) {
      const row = el('div', 'alias-row');
      const remove = el('button', 'link-button danger', 'Remove');
      remove.type = 'button';
      remove.addEventListener('click', () => {
        draft.aliases = draft.aliases.filter(x => x !== alias);
        renderAliases();
      });
      row.append(el('span', 'alias-address', `${alias}@${state.model.domain}`), remove);
      aliasRows.append(row);
    }
  };
  const aliasInput = el('input');
  aliasInput.type = 'text';
  aliasInput.maxLength = 40;
  aliasInput.placeholder = 'another-name';
  const aliasStatus = el('span', 'save-status');
  const addAlias = () => {
    const alias = slug(aliasInput.value);
    aliasStatus.classList.remove('error');
    aliasStatus.textContent = '';
    if (!alias) {
      return;
    }
    if (alias === draft.name || draft.aliases.includes(alias)) {
      aliasStatus.classList.add('error');
      aliasStatus.textContent = 'Already this group\'s.';
      return;
    }
    draft.aliases.push(alias);
    aliasInput.value = '';
    renderAliases();
  };
  aliasInput.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addAlias();
    }
  });
  const aliasAdd = el('div', 'add-row');
  aliasAdd.append(aliasInput, button('Add', 'plus', 'button button-secondary', addAlias), aliasStatus);
  aliasField.append(aliasRows, aliasAdd, el('small', '', 'Each must be unused by every other group.'));
  renderAliases();
  words.append(aliasField);
  const descField = el('label', 'field');
  const desc = el('textarea');
  desc.rows = 2;
  desc.maxLength = 300;
  desc.value = draft.description;
  desc.placeholder = 'What the group is for, a line or two.';
  desc.addEventListener('input', () => {
    draft.description = desc.value;
  });
  descField.append(el('span', '', 'Description'), desc);
  words.append(descField);
  const prefixField = el('label', 'field check');
  const prefix = el('input');
  prefix.type = 'checkbox';
  prefix.checked = draft.prefix;
  prefix.addEventListener('change', () => {
    draft.prefix = prefix.checked;
    updatePrefixNote();
  });
  const prefixNote = el('small');
  const updatePrefixNote = () => {
    prefixNote.textContent = draft.prefix ? `Every message goes out with “[${draft.title || 'Title'}]” at the front of its subject.` : 'Subjects go out as written.';
  };
  title.addEventListener('input', updatePrefixNote);
  updatePrefixNote();
  prefixField.append(prefix, el('span', '', 'Put the title in front of every subject'));
  words.append(prefixField, prefixNote);
  const visibilityField = el('label', 'field');
  const visibility = el('select');
  for (const [value, label] of Object.entries(visibilityWords)) {
    const option = el('option', '', label);
    option.value = value;
    option.selected = value === draft.visibility;
    visibility.append(option);
  }
  const visibilityNote = el('small');
  const updateVisibilityNote = () => {
    visibilityNote.textContent = visibilityNotes[draft.visibility];
  };
  visibility.addEventListener('change', () => {
    draft.visibility = visibility.value;
    updateVisibilityNote();
  });
  updateVisibilityNote();
  visibilityField.append(el('span', '', 'Who sees the group'), visibility, visibilityNote);
  words.append(visibilityField);
  const overviewPanel = el('div');
  overviewPanel.append(words);

  const managers = el('div', 'card');
  managers.append(el('h2', '', 'Managers'));
  managers.append(el('div', 'hint', 'Managers can edit or delete the group.'));
  const managerRows = el('div');
  const mount = el('div');
  const picker = createPersonPicker(mount);
  const renderManagers = () => {
    managerRows.replaceChildren();
    for (const email of draft.managers) {
      const person = state.model.people.find(p => p.email === email) || {email, name: email};
      const remove = el('button', 'link-button danger', 'Remove');
      remove.type = 'button';
      remove.disabled = draft.managers.length === 1;
      remove.addEventListener('click', () => {
        draft.managers = draft.managers.filter(m => m !== email);
        renderManagers();
      });
      managerRows.append(personRow(person, remove));
    }
    picker.setPeople(state.model.people.filter(p => !draft.managers.includes(p.email)));
  };
  const addManager = () => {
    const email = picker.value;
    if (!email || draft.managers.includes(email)) {
      return;
    }
    draft.managers.push(email);
    picker.reset();
    renderManagers();
  };
  mount.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addManager();
    }
  });
  const addRow = el('div', 'add-row');
  addRow.append(mount, button('Add', null, 'button', addManager));
  managers.append(managerRows, addRow);
  renderManagers();
  overviewPanel.append(managers);

  const preview = el('div', 'card preview');
  const previewHead = el('h2', '', 'Members');
  const previewChanges = el('div', 'change-band');
  previewChanges.hidden = true;
  const previewList = el('div', 'member-list');
  const previewStatus = el('div', 'save-status');
  preview.append(previewHead, previewStatus, previewChanges, previewList);
  const summary = el('span', 'change-summary');
  const current = isNew ? [] : g.members;
  const currentEmails = new Set(current.map(m => m.email));

  // renderChanges says what a save does to the membership: the people the
  // draft adds and the people it drops, by name, and marks each in the
  // list - a joiner with a chip, a leaver greyed at the end.
  const renderChanges = (members, rules) => {
    const memberEmails = new Set(members.map(m => m.email));
    const joining = isNew ? [] : members.filter(m => !currentEmails.has(m.email));
    const leaving = current.filter(m => !memberEmails.has(m.email));
    previewChanges.replaceChildren();
    previewChanges.hidden = isNew;
    if (!isNew) {
      summary.textContent = joining.length || leaving.length ? `${joining.length} ${joining.length === 1 ? 'joins' : 'join'} · ${leaving.length} ${leaving.length === 1 ? 'leaves' : 'leave'}` : 'Nobody joins or leaves';
      if (!joining.length && !leaving.length) {
        previewChanges.append(el('span', 'change-none', 'Saving changes nobody: the members stay as they are.'));
      }
      if (joining.length) {
        const line = el('div', 'change-line');
        line.append(el('span', 'chip joins', `${joining.length} ${joining.length === 1 ? 'joins' : 'join'}`), el('span', '', joining.map(m => m.name).join(', ')));
        previewChanges.append(line);
      }
      if (leaving.length) {
        const line = el('div', 'change-line');
        line.append(el('span', 'chip leaves', `${leaving.length} ${leaving.length === 1 ? 'leaves' : 'leave'}`), el('span', '', leaving.map(m => m.name).join(', ')));
        previewChanges.append(line);
      }
    }
    previewList.replaceChildren();
    for (const m of members) {
      const mark = !isNew && !currentEmails.has(m.email) ? el('span', 'chip joins', 'Joins') : null;
      previewList.append(personRow(m, mark, reasonWords(m, rules)));
    }
    for (const m of leaving) {
      const row = personRow(m, el('span', 'chip leaves', 'Leaves'), 'No rule picks them out any more.');
      row.classList.add('is-leaving');
      previewList.append(row);
    }
  };

  let previewTimer;
  let previewing = false;
  let previewAgain = false;
  const refreshPreview = async () => {
    if (previewing) {
      previewAgain = true;
      return;
    }
    previewing = true;
    previewStatus.classList.remove('error');
    previewStatus.textContent = 'Working out the members…';
    try {
      const rules = draft.rules.filter(ruleSaysSomething);
      const {members} = await send('POST', '/api/loop/preview', {name: isNew ? '' : draft.name, rules, additions: draft.additions, excluded: draft.excluded});
      previewHead.textContent = `${members.length} ${members.length === 1 ? 'member' : 'members'}`;
      renderChanges(members, rules);
      previewStatus.textContent = rules.length ? '' : 'Add a rule to pick people out.';
    } catch (err) {
      previewStatus.classList.add('error');
      previewStatus.textContent = err.message;
    }
    previewing = false;
    if (previewAgain) {
      previewAgain = false;
      refreshPreview();
    }
  };
  const rulesChanged = () => {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(refreshPreview, 250);
  };

  const ruleSection = (kind, heading, blurb) => {
    const card = el('div', 'card');
    card.append(el('h2', '', heading));
    if (blurb) {
      card.append(el('div', 'hint', blurb));
    }
    const rows = el('div', 'rules');
    const render = () => {
      rows.replaceChildren();
      const own = draft.rules.filter(r => r.kind === kind);
      if (!own.length) {
        rows.append(el('div', 'rule-empty', kind === 'include' ? 'No include rules yet: the group has nobody in it.' : 'No exclude rules.'));
      }
      for (const rule of own) {
        rows.append(ruleRow(rule, rulesChanged, () => {
          draft.rules = draft.rules.filter(r => r !== rule);
          render();
          rulesChanged();
        }));
      }
    };
    render();
    const add = el('div', 'add-row');
    add.append(button(kind === 'include' ? 'Add include rule' : 'Add exclude rule', 'plus', 'button button-secondary', () => {
      draft.rules.push(newRule(kind));
      render();
    }));
    card.append(rows, add);
    return card;
  };
  const rulesPanel = el('div');
  rulesPanel.append(ruleSection('include', 'Include', 'Someone matches a rule only if every choice in it holds.'));
  rulesPanel.append(ruleSection('exclude', 'Exclude', null));

  // The additions: people the directory does not hold, each a name and an
  // address typed in, on the group whatever the rules say until removed.
  const outside = el('div', 'card');
  outside.append(el('h2', '', 'Outside the directory'));
  outside.append(el('div', 'hint', 'For people who aren\'t in the directory.'));
  const additionRows = el('div');
  const renderAdditions = () => {
    additionRows.replaceChildren();
    if (!draft.additions.length) {
      additionRows.append(el('div', 'rule-empty', 'Nobody added by hand.'));
    }
    for (const a of draft.additions) {
      const remove = el('button', 'link-button danger', 'Remove');
      remove.type = 'button';
      remove.addEventListener('click', () => {
        draft.additions = draft.additions.filter(x => x !== a);
        renderAdditions();
        rulesChanged();
      });
      additionRows.append(personRow({email: a.email, name: a.name || a.email, outside: true}, remove));
    }
  };
  const additionName = el('input');
  additionName.type = 'text';
  additionName.maxLength = 80;
  additionName.placeholder = 'Name';
  const additionEmail = el('input');
  additionEmail.type = 'email';
  additionEmail.maxLength = 120;
  additionEmail.placeholder = 'name@example.org';
  const additionStatus = el('span', 'save-status');
  const addAddition = () => {
    const email = additionEmail.value.trim().toLowerCase();
    const name = additionName.value.trim();
    additionStatus.classList.remove('error');
    additionStatus.textContent = '';
    if (!email.includes('@')) {
      additionStatus.classList.add('error');
      additionStatus.textContent = 'An email address is needed.';
      return;
    }
    if (draft.additions.some(a => a.email === email)) {
      additionStatus.classList.add('error');
      additionStatus.textContent = 'Already added.';
      return;
    }
    draft.additions.push({email, name});
    additionName.value = '';
    additionEmail.value = '';
    renderAdditions();
    rulesChanged();
    additionName.focus();
  };
  for (const input of [additionName, additionEmail]) {
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        addAddition();
      }
    });
  }
  const additionAdd = el('div', 'add-row');
  additionAdd.append(additionName, additionEmail, button('Add', 'plus', 'button button-secondary', addAddition), additionStatus);
  outside.append(additionRows, additionAdd);
  renderAdditions();
  rulesPanel.append(outside);

  // The excluded: addresses kept off the group whatever the rules and the
  // additions say - people who unsubscribed, and anyone a manager lists
  // here with a note saying why.
  const excluded = el('div', 'card');
  const excludedRows = el('div');
  const renderExcluded = () => {
    excludedRows.replaceChildren();
    if (!draft.excluded.length) {
      excludedRows.append(el('div', 'rule-empty', 'Nobody is excluded.'));
    }
    for (const e of draft.excluded) {
      const remove = el('button', 'link-button danger', 'Remove');
      remove.type = 'button';
      remove.addEventListener('click', () => {
        draft.excluded = draft.excluded.filter(x => x !== e);
        renderExcluded();
        rulesChanged();
      });
      excludedRows.append(personRow(excludedPerson(e), remove));
    }
  };
  const excludedEmail = el('input');
  excludedEmail.type = 'email';
  excludedEmail.maxLength = 120;
  excludedEmail.placeholder = 'name@example.org';
  const excludedNote = el('input');
  excludedNote.type = 'text';
  excludedNote.maxLength = 120;
  excludedNote.placeholder = 'Why';
  const excludedStatus = el('span', 'save-status');
  const addExcluded = () => {
    const email = excludedEmail.value.trim().toLowerCase();
    const note = excludedNote.value.trim();
    excludedStatus.classList.remove('error');
    excludedStatus.textContent = '';
    if (!email.includes('@')) {
      excludedStatus.classList.add('error');
      excludedStatus.textContent = 'An email address is needed.';
      return;
    }
    if (draft.excluded.some(e => e.email === email)) {
      excludedStatus.classList.add('error');
      excludedStatus.textContent = 'Already excluded.';
      return;
    }
    draft.excluded.push({email, note, when: new Date().toISOString()});
    excludedEmail.value = '';
    excludedNote.value = '';
    renderExcluded();
    rulesChanged();
    excludedEmail.focus();
  };
  for (const input of [excludedEmail, excludedNote]) {
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        addExcluded();
      }
    });
  }
  const excludedAdd = el('div', 'add-row');
  excludedAdd.append(excludedEmail, excludedNote, button('Add', 'plus', 'button button-secondary', addExcluded), excludedStatus);
  excluded.append(excludedRows, excludedAdd);
  renderExcluded();
  const tabs = [
    {key: 'overview', label: 'Overview', panel: overviewPanel},
    {key: 'rules', label: 'Rules', panel: rulesPanel},
    {key: 'members', label: 'Members', panel: preview},
    {key: 'excluded', label: 'Excluded', panel: excluded},
  ];
  if (!isNew) {
    tabs.push(historyTab(g));
  }
  form.append(tabbed(tabs));

  const actions = el('div', 'editor-actions');
  const status = el('span', 'save-status');
  const save = button(isNew ? 'Make the group' : 'Save', 'check', 'button', async () => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    save.disabled = true;
    try {
      const body = {original: isNew ? '' : draft.name, name: draft.name, aliases: draft.aliases, title: draft.title, description: draft.description, prefix: draft.prefix, visibility: draft.visibility, managers: draft.managers, rules: draft.rules.filter(ruleSaysSomething), additions: draft.additions, excluded: draft.excluded};
      const saved = await send('POST', '/api/loop/group', body);
      await load();
      toast(isNew ? 'Group made' : 'Saved');
      navigate(withTab(groupPath(saved)));
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
      save.disabled = false;
    }
  });
  actions.append(save);
  if (!isNew) {
    actions.append(button('Cancel', null, 'button button-secondary', () => navigate(withTab(groupPath(g)))), summary);
    const del = el('button', 'danger-button', 'Delete group');
    del.type = 'button';
    del.addEventListener('click', async () => {
      if (!confirm(`Delete ${g.address}? Mail sent to it will bounce.`)) {
        return;
      }
      try {
        await send('DELETE', '/api/loop/group', {name: g.name});
        await load();
        toast('Group deleted');
        navigate('/');
      } catch (err) {
        status.classList.add('error');
        status.textContent = err.message;
      }
    });
    const danger = el('div', 'editor-danger');
    danger.append(del);
    overviewPanel.append(danger);
  } else {
    actions.append(button('Cancel', null, 'button button-secondary', () => navigate('/')));
  }
  actions.append(status);
  form.append(actions);
  refreshPreview();
  return form;
}

function ruleSaysSomething(r) {
  return r.roles.length || r.search || r.classrooms.length || r.grades.length || r.tags.length;
}

// excludedPerson is someone on the excluded list as a row shows them: by
// name when the directory holds them, with the note and when they went on.
function excludedPerson(e) {
  const person = state.model.people.find(p => p.email === e.email);
  const words = [e.note, e.when ? new Date(e.when).toLocaleDateString() : ''].filter(Boolean).join(' · ');
  return {email: e.email, name: person ? person.name : e.email, photoUrl: person ? person.photoUrl : '', words, outside: !person};
}

function slug(text) {
  return text.toLowerCase().replace(/[^a-z0-9.]+/g, '-').replace(/\.{2,}/g, '.').replace(/^[-.]+|[-.]+$/g, '').slice(0, 40);
}

export function newGroupPage() {
  setTitle('New Group');
  const page = el('div', 'group-page');
  page.append(pageHead('New Group'));
  page.append(editor({name: '', title: '', description: '', managers: [{email: me().email, name: me().name}], rules: [], additions: []}, true));
  return page;
}

// reasonWords says why a member is on the group: each include rule that
// reached them, said of one person - "Tagged in Tech Team", "Student in
// Hummingbirds" - and, when Add family brought them in, whose relative they
// are: "Parent of Mia, student in Hummingbirds"; or that a manager added
// them by hand.
const throughWords = {Parents: 'Parent', Children: 'Child', Siblings: 'Sibling'};

function reasonWords(member, rules) {
  return (member.reasons || []).map(reason => {
    if (reason.added) {
      return 'Added by hand';
    }
    const rule = rules[reason.rule];
    if (!rule) {
      return '';
    }
    const phrase = personWords(rule);
    if (reason.through) {
      return `${throughWords[reason.through]} of ${(reason.viaName || '').split(' ')[0]}, ${phrase}`;
    }
    return phrase.charAt(0).toUpperCase() + phrase.slice(1);
  }).filter(Boolean).join(' · ');
}

const singular = {Student: 'student', Parent: 'parent', Staff: 'staff member'};

// personWords is a rule said of one person who matches it, lowercase, for
// a member's reason: "parent in Grade 5 or Grade 6", "tagged in Tech Team".
function personWords(r) {
  const parts = [];
  if (r.roles.length) {
    parts.push(r.roles.map(x => singular[x]).join(' or '));
  }
  if (r.search) {
    parts.push(`with “${r.search}” in their name or address`);
  }
  if (r.grades.length) {
    parts.push('in ' + r.grades.join(' or '));
  }
  if (r.classrooms.length) {
    parts.push('in ' + r.classrooms.join(' or '));
  }
  if (r.tags.length) {
    parts.push('tagged in ' + (r.tagLabels || r.tags).join(' or '));
  }
  return parts.join(' ');
}

// ruleWords is a rule as the group's page reads it out.
function ruleWords(r) {
  const parts = [];
  if (r.roles.length) {
    parts.push(r.roles.map(x => x + 's').join(' or '));
  } else {
    parts.push('Anyone');
  }
  if (r.search) {
    parts.push(`with “${r.search}” in their name or address`);
  }
  if (r.grades.length) {
    parts.push('in ' + r.grades.join(' or '));
  }
  if (r.classrooms.length) {
    parts.push('in ' + r.classrooms.join(' or '));
  }
  if (r.tags.length) {
    parts.push('tagged in ' + (r.tagLabels || r.tags).join(' or '));
  }
  let words = parts.join(' ');
  if (r.family.length) {
    words += ', plus their ' + r.family.map(f => f.toLowerCase()).join(' and ');
  }
  return words;
}

export function groupPage(g) {
  setTitle(g.title);
  const page = el('div', 'group-page');
  const editing = new URLSearchParams(location.search).get('edit') === '1';
  const canEdit = g.mine || isAdmin();
  if (editing && canEdit) {
    page.append(pageHead(g.title));
    page.append(editor(g, false));
    return page;
  }
  const actions = [];
  if (canEdit) {
    actions.push(button('Edit', 'edit', 'button', () => navigate(withTab(groupPath(g) + '?edit=1'))));
  }
  if (g.member) {
    const toggle = button(g.unsubscribed ? 'Resubscribe' : 'Unsubscribe', null, 'button button-secondary', async () => {
      toggle.disabled = true;
      try {
        await send('POST', '/api/loop/subscription', {name: g.name, subscribed: g.unsubscribed});
        await load();
        toast(g.unsubscribed ? 'Resubscribed' : 'Unsubscribed');
      } catch (err) {
        toggle.disabled = false;
        toast(err.message);
      }
    });
    actions.push(toggle);
  }
  page.append(pageHead(g.title, actions));
  const overview = el('div');
  const address = el('div', 'address-band');
  const mail = el('a', 'address-mail');
  mail.href = 'mailto:' + g.address;
  mail.append(svg('mail'), el('span', '', g.address));
  address.append(mail, iconButton('copy', 'Copy the address', '', () => copyText(g.address, 'Address copied')));
  overview.append(address);
  if (g.aliases.length) {
    const also = el('div', 'subject-note', 'Also reached as');
    const list = el('ul', 'rule-list');
    for (const alias of [...g.aliases].sort()) {
      list.append(el('li', '', `${alias}@${state.model.domain}`));
    }
    also.append(list);
    overview.append(also);
  }
  overview.append(el('div', 'subject-note', g.prefix ? `Every message goes out with “[${g.title}]” at the front of its subject.` : 'Subjects go out as written.'));
  if (g.visibility === 'everyone') {
    overview.append(el('div', 'subject-note', 'Visible to everyone in Loop; only its managers can change it.'));
  }
  if (g.visibility === 'members') {
    overview.append(el('div', 'subject-note', 'Visible to the people on it; only its managers can change it.'));
  }
  if (g.member && g.unsubscribed) {
    overview.append(el('div', 'subject-note', 'You are on this group\'s excluded list and get no mail from it. Resubscribe to get its mail again.'));
  }
  if (g.description) {
    overview.append(el('p', 'page-lead', g.description));
  }
  // The same people elsewhere: the group's Magic Tag in Who?, where the
  // members can be filtered, mapped and mailed one by one.
  if (canEdit) {
    const elsewhere = el('div', 'elsewhere');
    const who = el('a', 'elsewhere-link');
    who.href = appOrigin('who') + '/people?list=' + encodeURIComponent('group:' + g.name);
    const mark = el('img');
    mark.src = '/brand/apps/who.png';
    mark.alt = '';
    who.append(mark, el('span', '', 'See these people in Helios Who?'));
    elsewhere.append(who);
    overview.append(elsewhere);
  }

  const rules = el('div', 'card');
  for (const kind of ['include', 'exclude']) {
    const own = g.rules.filter(r => r.kind === kind);
    if (!own.length) {
      continue;
    }
    rules.append(el('h3', '', kind === 'include' ? 'Include' : 'Exclude'));
    const list = el('ul', 'rule-list');
    for (const r of own) {
      list.append(el('li', '', ruleWords(r)));
    }
    rules.append(list);
  }

  const managers = el('div', 'card');
  managers.append(el('h2', '', 'Managers'));
  for (const m of g.managers) {
    managers.append(personRow(m));
  }
  overview.append(managers);

  const members = el('div', 'card');
  const list = el('div', 'member-list');
  for (const m of g.members) {
    list.append(personRow(m, null, reasonWords(m, g.rules)));
  }
  if (!g.members.length) {
    list.append(el('div', 'rule-empty', 'Nobody matches the rules yet.'));
  }
  members.append(list);

  const excluded = el('div', 'card');
  for (const e of g.excluded) {
    excluded.append(personRow(excludedPerson(e)));
  }
  if (!g.excluded.length) {
    excluded.append(el('div', 'rule-empty', 'Nobody is excluded.'));
  }

  const tabs = [
    {key: 'overview', label: 'Overview', panel: overview},
    {key: 'members', label: 'Members', count: g.members.length, panel: members},
    {key: 'rules', label: 'Rules', count: g.rules.length, panel: rules},
    {key: 'excluded', label: 'Excluded', count: g.excluded.length, panel: excluded},
  ];
  if (canEdit) {
    tabs.push(historyTab(g));
  }
  page.append(tabbed(tabs));
  page.append(link('/', 'back-link', '← All groups'));
  return page;
}

function tabbed(tabs) {
  const wrap = el('div', 'group-tabs');
  const fallback = tabs[0].key;
  const wanted = tabParam(fallback);
  let strip = null;
  const show = key => {
    const next = tabStrip(tabs, key, 2, pick => {
      history.replaceState(null, '', tabHref(pick));
      show(pick);
    });
    if (strip) {
      strip.replaceWith(next);
    }
    strip = next;
    for (const t of tabs) {
      t.panel.hidden = t.key !== key;
      if (t.key === key && t.onShow) {
        t.onShow();
      }
    }
  };
  show(tabs.some(t => t.key === wanted) ? wanted : fallback);
  wrap.append(strip, ...tabs.map(t => t.panel));
  return wrap;
}

function withTab(path) {
  const tab = new URLSearchParams(location.search).get('tab');
  if (!tab) {
    return path;
  }
  return path + (path.includes('?') ? '&' : '?') + 'tab=' + encodeURIComponent(tab);
}

const troubleWords = {bounced: 'bounced', delivery_delayed: 'delayed', complained: 'marked it as spam'};

function historyTab(g) {
  const panel = el('div', 'card');
  const status = el('div', 'save-status', 'Loading…');
  const list = el('div', 'message-list');
  panel.append(status, list);
  let loaded = false;
  const load = async () => {
    loaded = true;
    try {
      const res = await fetch('/api/loop/messages?name=' + encodeURIComponent(g.name));
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const {messages} = await res.json();
      status.textContent = messages.length ? '' : 'Nothing has been sent to the group yet.';
      for (const m of messages) {
        list.append(messageRow(m));
      }
    } catch (err) {
      loaded = false;
      status.classList.add('error');
      status.textContent = err.message;
    }
  };
  return {key: 'history', label: 'History', count: g.sent, panel, onShow: () => {
    if (!loaded) {
      load();
    }
  }};
}

function messageRow(m) {
  const row = el('div', 'message-row');
  const head = el('div', 'message-head');
  const person = state.model.people.find(p => p.email === m.from.email);
  head.append(thumb(person || m.from, 'small'));
  const body = el('div', 'person-body');
  body.append(el('div', 'message-subject', m.subject || '(no subject)'));
  const when = m.received ? new Date(m.received).toLocaleString() : '';
  body.append(el('div', 'person-words', [m.from.name, when, `${m.recipients} ${m.recipients === 1 ? 'copy' : 'copies'} sent`].filter(Boolean).join(' · ')));
  head.append(body);
  row.append(head);
  if (!m.trouble.length) {
    return row;
  }
  const counts = {};
  for (const t of m.trouble) {
    counts[t.event] = (counts[t.event] || 0) + 1;
  }
  const toggle = el('button', 'trouble-toggle');
  toggle.type = 'button';
  toggle.append(el('span', '', Object.entries(counts).map(([event, n]) => `${n} ${troubleWords[event] || event}`).join(' · ')), svg('chevron'));
  const details = el('div', 'trouble-list');
  details.hidden = true;
  for (const t of m.trouble) {
    const known = state.model.people.find(p => p.email === t.email);
    const words = [troubleWords[t.event] || t.event, t.when ? new Date(t.when).toLocaleString() : ''].filter(Boolean).join(' · ');
    details.append(personRow({email: t.email, name: t.name || t.email, photoUrl: known ? known.photoUrl : '', words, outside: !known}, el('span'), t.detail));
  }
  toggle.addEventListener('click', () => {
    details.hidden = !details.hidden;
    toggle.classList.toggle('open', !details.hidden);
  });
  head.append(toggle);
  row.append(details);
  return row;
}
