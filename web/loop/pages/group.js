import {state, me, isAdmin, options, groupPath} from '../state.js';
import {el, svg, link, button, iconButton, copyText, toast, personRow, pageHead, thumb, whoLink} from '../dom.js';
import {setTitle} from '../chrome.js';
import {load, navigate} from '../app.js';
import {createPersonPicker} from '/picker.js';
import {tabStrip, tabParam, tabHref} from '/tabs.js';
import {rulesEditor} from '/rules.js';
import {visibilityWords} from './groups.js';

const {ruleRow, newRule, ruleSaysSomething, personWords, ruleWords} = rulesEditor({
  el, svg, options,
  personName: email => (state.model.people.find(p => p.email === email) || {}).name || '',
});

const visibilityNotes = {
  hidden: 'Only the group\'s managers and the admins see it.',
  members: 'Anyone the rules or the additions place on the group can see it, and take themselves off it or put themselves back; only managers can change it.',
  everyone: 'Anyone in Loop can see it, and only managers can change it.',
};

const postingWords = {everyone: 'Everyone', members: 'Members', managers: 'Managers'};

const postingNotes = {
  everyone: 'A new message from any address goes out to the group.',
  members: 'A new message goes out only from the managers and the people the rules or the additions place on the group; anyone else gets a note saying so.',
  managers: 'A new message goes out only from the group\'s managers; anyone else gets a note saying so.',
};

const replyingNotes = {
  everyone: 'A reply to a message the group sent goes out from any address.',
  members: 'A reply to a message the group sent goes out only from the managers and the people the rules or the additions place on the group; anyone else gets a note saying so.',
  managers: 'A reply to a message the group sent goes out only from the group\'s managers; anyone else gets a note saying so.',
};

function audienceField(label, notes, value, onChange) {
  const field = el('label', 'field');
  const select = el('select');
  for (const [option, words] of Object.entries(postingWords)) {
    const o = el('option', '', words);
    o.value = option;
    o.selected = option === value;
    select.append(o);
  }
  const note = el('small', '', notes[value]);
  select.addEventListener('change', () => {
    note.textContent = notes[select.value];
    onChange(select.value);
  });
  field.append(el('span', '', label), select, note);
  return field;
}

async function send(method, url, body) {
  const res = await fetch(url, {method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.status === 204 ? null : res.json();
}

function editor(g, isNew, closeModal, startTab) {
  const draft = {
    name: g.name, aliases: [...(g.aliases || [])], title: g.title, description: g.description || '',
    prefix: isNew ? true : g.prefix,
    visibility: isNew ? 'hidden' : g.visibility,
    posting: isNew ? 'everyone' : g.posting,
    replying: isNew ? 'everyone' : g.replying,
    managers: g.managers.map(m => m.email),
    rules: g.rules.map(r => ({kind: r.kind, roles: [...r.roles], search: r.search, classrooms: [...r.classrooms], grades: [...r.grades], tags: [...r.tags], family: [...r.family], tagLabels: r.tagLabels})),
    additions: (g.additions || []).map(a => ({email: a.email, name: a.name})),
    excluded: (g.excluded || []).map(e => ({email: e.email, note: e.note || '', when: e.when || ''})),
  };
  const firstRule = isNew && !draft.rules.length ? newRule('include') : null;
  if (firstRule) {
    draft.rules.push(firstRule);
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
  titleField.append(el('span', '', 'Group name'), title);
  if (closeModal) {
    title.className = 'modal-title-input';
    title.setAttribute('aria-label', 'Group name');
    const wanting = () => title.classList.toggle('is-wanted', isNew && !title.value.trim());
    title.addEventListener('input', wanting);
    wanting();
    form.titleInput = title;
  } else {
    words.append(titleField);
  }
  const nameField = el('div', 'field');
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
  const nameHead = el('span', 'field-head');
  const addAliasButton = button('Add alias', 'plus', 'button button-small button-secondary', () => {
    aliasAdd.hidden = false;
    aliasInput.focus();
  });
  nameHead.append(el('span', '', 'Address'), addAliasButton);
  nameField.append(nameHead, name, addressNote);
  updateAddress();
  words.append(nameField);
  const aliasField = el('div', 'field alias-field');
  const aliasRows = el('div', 'alias-rows');
  const renderAliases = () => {
    aliasRows.replaceChildren();
    aliasRows.hidden = !draft.aliases.length;
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
    aliasAdd.hidden = true;
    renderAliases();
  };
  aliasInput.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addAlias();
    }
  });
  aliasInput.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      aliasInput.value = '';
      aliasAdd.hidden = true;
    }
  });
  const aliasAdd = el('div', 'add-row alias-add');
  aliasAdd.hidden = true;
  const aliasClear = iconButton('close', 'Clear', 'alias-clear', () => {
    aliasInput.value = '';
    aliasStatus.textContent = '';
    aliasAdd.hidden = true;
  });
  const aliasBox = el('div', 'alias-box');
  aliasBox.append(aliasInput, aliasClear);
  aliasAdd.append(aliasBox, button('Add', 'plus', 'button button-secondary', addAlias), aliasStatus, el('small', '', `Another name for the same address, unused by every other group: <name>@${state.model.domain}.`));
  aliasField.append(aliasRows, aliasAdd);
  renderAliases();
  words.append(aliasField);
  const descField = el('div', 'field');
  const desc = el('textarea');
  desc.rows = 2;
  desc.maxLength = 300;
  desc.value = draft.description;
  desc.placeholder = 'What the group is for, a line or two.';
  desc.setAttribute('aria-label', 'Description');
  desc.addEventListener('input', () => {
    draft.description = desc.value;
  });
  const descHead = el('span', 'field-head');
  const descStatus = el('small', 'save-status');
  const generate = button('Generate with AI', 'sparkle', 'button button-small button-secondary', async () => {
    generate.disabled = true;
    descStatus.classList.remove('error');
    descStatus.textContent = 'Writing…';
    try {
      const rules = draft.rules.filter(ruleSaysSomething);
      const {description} = await send('POST', '/api/loop/describe', {name: isNew ? '' : draft.name, title: draft.title, ruleWords: rules.map(r => (r.kind === 'exclude' ? 'Leaving out: ' : '') + ruleWords(r)), rules, additions: draft.additions, excluded: draft.excluded});
      desc.value = description;
      draft.description = description;
      descStatus.textContent = '';
      desc.focus();
    } catch (err) {
      descStatus.classList.add('error');
      descStatus.textContent = err.message;
    } finally {
      generate.disabled = false;
    }
  });
  descHead.append(el('span', '', 'Description'), generate);
  descField.append(descHead, desc, descStatus);
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
    prefixNote.textContent = draft.prefix ? `Every message goes out with “[${draft.title || 'Group name'}]” at the front of its subject.` : 'Subjects go out as written.';
  };
  title.addEventListener('input', updatePrefixNote);
  updatePrefixNote();
  prefixField.append(prefix, el('span', '', 'Put the group name in front of every subject'));
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
  words.append(
    audienceField('Who can post', postingNotes, draft.posting, value => { draft.posting = value; }),
    audienceField('Who can reply', replyingNotes, draft.replying, value => { draft.replying = value; }),
  );
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
  if (isNew) {
    overviewPanel.append(managers);
  }

  const preview = el('div', 'card preview');
  const previewHead = el('h2', '', 'Members');
  const previewHeadRow = el('div', 'card-head');
  previewHeadRow.append(previewHead);
  const previewChanges = el('div', 'change-band');
  previewChanges.hidden = true;
  const previewList = el('div', 'compact-list');
  const previewStatus = el('div', 'save-status');
  const previewSearch = el('input', 'member-search');
  previewSearch.type = 'search';
  previewSearch.placeholder = 'Search the members…';
  previewSearch.setAttribute('aria-label', 'Search the members');
  const previewEmpty = el('div', 'rule-empty', 'Nobody matches that.');
  previewEmpty.hidden = true;
  preview.append(previewHeadRow, previewStatus, previewChanges, previewSearch, previewList, previewEmpty);
  const summary = el('span', 'change-summary');
  const current = isNew ? [] : g.members;
  const currentEmails = new Set(current.map(m => m.email));

  const pickMember = (row, m) => {
    for (const other of document.querySelectorAll('.member-menu')) {
      other.remove();
    }
    const menu = el('div', 'member-menu');
    const exclude = el('button', 'member-menu-item');
    exclude.type = 'button';
    exclude.append(svg('user-minus'), el('span', '', `Exclude ${m.name} from this group`));
    exclude.addEventListener('click', e => {
      e.stopPropagation();
      menu.remove();
      if (!draft.rules.some(r => r.kind === 'exclude' && r.search === m.email && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length)) {
        draft.rules.push({...newRule('exclude'), search: m.email});
        renderRules();
      }
      rulesChanged();
    });
    const who = el('a', 'member-menu-item');
    who.href = whoLink(m.email);
    who.target = '_blank';
    who.rel = 'noopener';
    who.append(svg('user'), el('span', '', 'Open in Helios Who?'));
    menu.append(exclude, who);
    row.append(menu);
    const close = e => {
      if (!menu.contains(e.target)) {
        menu.remove();
        document.removeEventListener('click', close, true);
      }
    };
    setTimeout(() => document.addEventListener('click', close, true));
  };

  const renderChanges = (members, rules) => {
    const memberEmails = new Set(members.map(m => m.email));
    const joining = isNew ? [] : members.filter(m => !currentEmails.has(m.email));
    const leaving = current.filter(m => !memberEmails.has(m.email));
    previewChanges.replaceChildren();
    previewChanges.hidden = isNew || (!joining.length && !leaving.length);
    if (!isNew) {
      summary.textContent = joining.length || leaving.length ? `${joining.length} ${joining.length === 1 ? 'joins' : 'join'} · ${leaving.length} ${leaving.length === 1 ? 'leaves' : 'leave'}` : 'Nobody joins or leaves';
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
    lastPreview = {members, rules, leaving};
    drawPreviewList();
  };
  let lastPreview = null;
  const drawPreviewList = () => {
    if (!lastPreview) {
      return;
    }
    const {members, rules, leaving} = lastPreview;
    const q = previewSearch.value.trim().toLowerCase();
    const wanted = m => !q || m.name.toLowerCase().includes(q) || m.email.toLowerCase().includes(q) || (m.words || '').toLowerCase().includes(q);
    previewList.replaceChildren();
    let shown = 0;
    for (const m of [...members.filter(m => m.outside), ...members.filter(m => !m.outside)].filter(wanted)) {
      const chip = !isNew && !currentEmails.has(m.email) ? el('span', 'chip joins', 'Joins') : null;
      const row = compactRow(m, reasonWords(m, rules), chip, m.outside ? () => {
        draft.additions = draft.additions.filter(a => a.email !== m.email);
        rulesChanged();
      } : null, m.outside ? null : pickMember);
      previewList.append(row);
      shown++;
    }
    for (const m of leaving.filter(wanted)) {
      const row = compactRow(m, 'No rule picks them out any more.', el('span', 'chip leaves', 'Leaves'));
      row.classList.add('is-leaving');
      previewList.append(row);
      shown++;
    }
    previewSearch.hidden = !members.length && !leaving.length;
    previewEmpty.hidden = shown > 0 || (!members.length && !leaving.length);
  };
  previewSearch.addEventListener('input', drawPreviewList);

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
      const {members, ruleCounts: counts} = await send('POST', '/api/loop/preview', {name: isNew ? '' : draft.name, rules, additions: draft.additions, excluded: draft.excluded});
      previewHead.textContent = `${members.length} ${members.length === 1 ? 'member' : 'members'}`;
      renderChanges(members, rules);
      showRuleCounts(rules, counts || []);
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

  const rulesCard = el('div', 'card');
  rulesCard.append(el('h2', '', 'Rules'));
  rulesCard.append(el('div', 'hint', 'Someone is on the group if any include rule matches them and no exclude rule does; a rule matches only if every choice in it holds.'));
  const rows = el('div', 'rules');
  let opened = firstRule;
  const ruleCounts = new Map();
  const countChip = rule => {
    const chip = el('span', 'rule-count');
    const n = ruleCounts.get(rule);
    chip.hidden = n === undefined;
    if (n !== undefined) {
      chip.textContent = rule.kind === 'include' ? `${n} match` : `${n} excluded`;
    }
    return chip;
  };
  const renderRules = () => {
    rows.replaceChildren();
    if (!draft.rules.length) {
      rows.append(el('div', 'rule-empty', 'No rules yet: the group has nobody in it.'));
    }
    for (const kind of ['include', 'exclude']) {
      for (const rule of draft.rules.filter(r => r.kind === kind)) {
        rows.append(ruleRow(rule, rulesChanged, () => {
          draft.rules = draft.rules.filter(r => r !== rule);
          renderRules();
          rulesChanged();
        }, rule === opened, countChip));
      }
    }
    opened = null;
  };
  const showRuleCounts = (rules, counts) => {
    ruleCounts.clear();
    rules.forEach((rule, i) => ruleCounts.set(rule, counts[i]));
    for (const row of rows.children) {
      if (row.refreshCount) {
        row.refreshCount();
      }
    }
  };
  renderRules();
  const addRules = el('div', 'add-row');
  for (const kind of ['include', 'exclude']) {
    addRules.append(button(kind === 'include' ? 'Add include rule' : 'Add exclude rule', kind === 'include' ? 'plus' : 'minus', 'button button-secondary', () => {
      opened = newRule(kind);
      draft.rules.push(opened);
      renderRules();
    }));
  }
  rulesCard.append(rows, addRules);
  const rulesPanel = el('div');
  rulesPanel.append(rulesCard);

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
    additionAdd.hidden = true;
    rulesChanged();
  };
  for (const input of [additionName, additionEmail]) {
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        addAddition();
      }
    });
  }
  const additionAdd = el('div', 'add-row addition-add');
  additionAdd.hidden = true;
  additionAdd.append(additionName, additionEmail, button('Add', 'plus', 'button button-secondary', addAddition), iconButton('close', 'Never mind', '', () => {
    additionName.value = '';
    additionEmail.value = '';
    additionStatus.textContent = '';
    additionAdd.hidden = true;
  }), additionStatus, el('small', '', 'Someone the directory does not hold - a coach, a league office, a family friend - on the group whatever the rules say.'));
  previewHeadRow.after(additionAdd);

  const personMount = el('div');
  const personPicker = createPersonPicker(personMount);
  personPicker.setPeople(state.model.people);
  const personStatus = el('span', 'save-status');
  const personAdd = el('div', 'add-row addition-add');
  personAdd.hidden = true;
  const addPerson = () => {
    const email = personPicker.value;
    personStatus.classList.remove('error');
    personStatus.textContent = '';
    if (!email) {
      personStatus.classList.add('error');
      personStatus.textContent = 'Pick someone from the list.';
      return;
    }
    if (!draft.rules.some(r => r.kind === 'include' && r.search === email && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length)) {
      draft.rules.push({...newRule('include'), search: email});
      renderRules();
    }
    personPicker.reset();
    personAdd.hidden = true;
    rulesChanged();
  };
  personMount.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addPerson();
    }
  });
  personAdd.append(personMount, button('Add', 'plus', 'button button-secondary', addPerson), iconButton('close', 'Never mind', '', () => {
    personPicker.reset();
    personStatus.textContent = '';
    personAdd.hidden = true;
  }), personStatus, el('small', '', 'Someone from the directory, on the group by a rule of their own.'));
  previewHeadRow.after(personAdd);
  const headButtons = el('div', 'head-buttons');
  headButtons.append(button('Add Helios', 'plus', 'button button-small button-secondary', () => {
    additionAdd.hidden = true;
    personAdd.hidden = false;
    personMount.querySelector('input').focus();
  }), button('Add Non-Helios', 'plus', 'button button-small button-secondary', () => {
    personAdd.hidden = true;
    additionAdd.hidden = false;
    additionName.focus();
  }));
  previewHeadRow.append(headButtons);

  rulesPanel.append(preview);
  const tabs = [
    {key: 'members', label: 'Members', panel: rulesPanel},
    {key: 'details', label: 'Details', panel: overviewPanel},
  ];
  form.append(tabbed(tabs, !closeModal, startTab));

  const actions = el('div', 'editor-actions');
  const status = el('span', 'save-status');
  const save = button(isNew ? 'Make the group' : 'Save', 'check', 'button', async () => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    save.disabled = true;
    try {
      const body = {original: isNew ? '' : draft.name, name: draft.name, aliases: draft.aliases, title: draft.title, description: draft.description, prefix: draft.prefix, visibility: draft.visibility, posting: draft.posting, replying: draft.replying, managers: draft.managers, rules: draft.rules.filter(ruleSaysSomething), additions: draft.additions, excluded: draft.excluded};
      const saved = await send('POST', '/api/loop/group', body);
      if (closeModal) {
        closeModal();
      }
      await load();
      toast(isNew ? 'Group made' : 'Saved');
      if (!closeModal || isNew) {
        navigate(withTab(groupPath(saved)));
      }
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
      save.disabled = false;
    }
  });
  actions.append(save);
  let deleteButton = null;
  if (!isNew) {
    actions.append(button('Cancel', null, 'button button-secondary', () => {
      if (closeModal) {
        closeModal();
      } else {
        navigate(withTab(groupPath(g)));
      }
    }), summary);
    const del = el('button', 'danger-button', 'Delete group');
    del.type = 'button';
    del.addEventListener('click', async () => {
      if (!confirm(`Delete ${g.address}? Mail sent to it will bounce.`)) {
        return;
      }
      try {
        await send('DELETE', '/api/loop/group', {name: g.name});
        if (closeModal) {
          closeModal();
        }
        await load();
        toast('Group deleted');
        navigate('/');
      } catch (err) {
        status.classList.add('error');
        status.textContent = err.message;
      }
    });
    deleteButton = del;
  } else {
    actions.append(button('Cancel', null, 'button button-secondary', () => {
      if (closeModal) {
        closeModal();
      }
      navigate('/');
    }));
  }
  actions.append(status);
  if (deleteButton) {
    actions.append(deleteButton);
  }
  form.append(actions);
  refreshPreview();
  return form;
}

function editModal(g, startTab, isNew) {
  const overlay = el('div', 'modal-overlay');
  const box = el('div', 'modal modal-editor');
  box.setAttribute('role', 'dialog');
  box.setAttribute('aria-modal', 'true');
  box.setAttribute('aria-label', isNew ? 'New group' : 'Edit ' + g.title);
  const onKey = e => {
    if (e.key === 'Escape') {
      close();
      if (isNew) {
        navigate('/');
      }
    }
  };
  const close = () => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  };
  const form = editor(g, isNew, close, startTab);
  const header = el('div', 'modal-header');
  const heading = el('div', 'modal-heading');
  heading.append(form.titleInput, el('small', 'modal-heading-hint', isNew ? 'Type the group\'s name' : 'Click to edit'));
  header.append(heading);
  const x = el('button', 'modal-close', '×');
  x.type = 'button';
  x.setAttribute('aria-label', 'Close');
  x.addEventListener('click', () => {
    close();
    if (isNew) {
      navigate('/');
    }
  });
  header.append(x);
  box.append(header, form);
  overlay.append(box);
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
  box.scrollTo(0, 0);
}

function slug(text) {
  return text.toLowerCase().replace(/[^a-z0-9.]+/g, '-').replace(/\.{2,}/g, '.').replace(/^[-.]+|[-.]+$/g, '').slice(0, 40);
}

export function newGroupModal() {
  const from = new URLSearchParams(location.search).get('from');
  const suggestion = state.model.suggestions.find(s => s.key === from);
  if (suggestion) {
    const rule = {...newRule('include'), tags: [suggestion.key], tagLabels: [suggestion.name], family: ['Parents']};
    editModal({name: slug(suggestion.name), title: suggestion.name, description: '', managers: suggestion.managers, rules: [rule], additions: []}, null, true);
    return;
  }
  editModal({name: '', title: '', description: '', managers: [{email: me().email, name: me().name}], rules: [], additions: []}, null, true);
}

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
    actions.push(button('Edit', 'edit', 'button', () => editModal(g)));
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
  const archive = button(g.archived ? 'Unarchive' : 'Archive', 'archive', 'button button-secondary', async () => {
    archive.disabled = true;
    try {
      await send('POST', '/api/loop/archive', {name: g.name, archived: !g.archived});
      await load();
      toast(g.archived ? 'Back among your groups' : 'Archived');
    } catch (err) {
      archive.disabled = false;
      toast(err.message);
    }
  });
  archive.title = g.archived
    ? 'Put the group back in your normal view. It has been active all along.'
    : 'Archiving removes from your normal view, but the list stays active.';
  actions.push(archive);
  page.append(pageHead(g.title, actions));

  const overview = el('div', 'group-overview');
  const address = el('div', 'address-band');
  const mail = el('a', 'address-mail');
  mail.href = 'mailto:' + g.address;
  mail.append(svg('mail'), el('span', '', g.address));
  address.append(mail, iconButton('copy', 'Copy the address', '', () => copyText(g.address, 'Address copied')));
  if (g.aliases.length) {
    const aliases = el('div', 'address-aliases');
    const count = el('button', 'address-aliases-count', `${g.aliases.length} ${g.aliases.length === 1 ? 'alias' : 'aliases'}`);
    count.type = 'button';
    count.setAttribute('aria-label', 'Also reached as');
    const tip = el('div', 'address-aliases-tip');
    tip.append(el('div', 'address-aliases-head', 'Also reached as'));
    for (const alias of [...g.aliases].sort()) {
      tip.append(el('div', '', `${alias}@${state.model.domain}`));
    }
    aliases.append(count, tip);
    address.append(aliases);
  }
  overview.append(address);
  overview.append(el('div', 'subject-note', g.prefix ? `Every message goes out with “[${g.title}]” at the front of its subject.` : 'Subjects go out as written.'));
  if (g.visibility === 'everyone') {
    overview.append(el('div', 'subject-note', 'Visible to everyone in Loop; only its managers can change it.'));
  }
  if (g.visibility === 'members') {
    overview.append(el('div', 'subject-note', 'Visible to the people on it; only its managers can change it.'));
  }
  const audienceNotes = {everyone: 'Anyone', members: 'Only its managers and the people on it', managers: 'Only its managers'};
  if (g.posting !== 'everyone') {
    overview.append(el('div', 'subject-note', `${audienceNotes[g.posting]} can post new messages.`));
  }
  if (g.replying !== g.posting) {
    overview.append(el('div', 'subject-note', `${audienceNotes[g.replying]} can reply to its messages.`));
  }
  if (g.archived) {
    overview.append(el('div', 'subject-note', 'Archived for you: it sits under Archived in the rail and its Magic Tag is off your lists in Helios Who?, and it works as it always did.'));
  }
  if (g.member && g.unsubscribed) {
    overview.append(el('div', 'subject-note', 'You are on this group\'s excluded list and get no mail from it. Resubscribe to get its mail again.'));
  }
  if (g.description) {
    overview.append(el('p', 'page-lead', g.description));
  }
  page.append(overview);

  const members = el('div');
  const grid = el('div', 'attendee-grid');
  const search = el('input', 'member-search');
  search.type = 'search';
  search.placeholder = 'Search the members…';
  search.setAttribute('aria-label', 'Search the members');
  const empty = el('div', 'rule-empty');
  const showMembers = () => {
    const q = search.value.trim().toLowerCase();
    grid.replaceChildren();
    const shown = g.members.filter(m => !q || m.name.toLowerCase().includes(q) || m.email.toLowerCase().includes(q) || (m.context || '').toLowerCase().includes(q));
    for (const m of shown) {
      // Why each is on it is the group's managers' - and an admin's with
      // the hat on - to read.
      grid.append(memberCard(canEdit ? m : {...m, reasons: []}, g.rules));
    }
    empty.textContent = g.members.length ? 'Nobody matches that.' : 'Nobody matches the rules yet.';
    empty.hidden = shown.length > 0;
  };
  search.addEventListener('input', showMembers);
  showMembers();
  if (g.members.length) {
    members.append(search);
  }
  members.append(grid, empty);

  if (canEdit) {
    const rules = el('div', 'card');
    const rulesHead = el('div', 'card-head');
    rulesHead.append(el('h2', '', 'How the members are chosen'));
    rulesHead.append(iconButton('edit', 'Edit the rules', '', () => editModal(g, 'members')));
    rules.append(rulesHead);
    const lines = el('div', 'rule-lines');
    for (const kind of ['include', 'exclude']) {
      for (const r of g.rules.filter(x => x.kind === kind)) {
        const line = el('div', 'rule-line');
        const sign = el('span', 'rule-sign rule-sign-' + kind);
        sign.append(svg(kind === 'include' ? 'plus' : 'minus'));
        sign.title = kind === 'include' ? 'Included' : 'Excluded';
        line.append(sign, el('span', 'rule-line-words', ruleWords(r)));
        lines.append(line);
      }
    }
    rules.append(lines);
    if (g.excluded.length) {
      rules.append(el('div', 'subject-note', `${g.excluded.length} ${g.excluded.length === 1 ? 'address is' : 'addresses are'} kept off the group whatever the rules say.`));
    }
    members.append(rules);
  }

  const managersPanel = el('div');
  managersPanel.append(managersCard(g, canEdit));
  const tabs = [
    {key: 'members', label: 'Members', count: g.members.length, panel: members},
    {key: 'managers', label: 'Managers', count: g.managers.length, panel: managersPanel},
  ];
  if (canEdit) {
    tabs.push(historyTab(g));
  }
  page.append(tabbed(tabs));
  return page;
}

function compactRow(m, why, chip, onRemove, onPick) {
  const row = el(m.outside || onPick ? 'div' : 'a', 'compact-row' + (m.outside ? ' is-outside' : '') + (onPick ? ' is-pickable' : ''));
  if (onPick) {
    row.tabIndex = 0;
    row.setAttribute('role', 'button');
    row.title = `${m.name}: exclude, or open in Helios Who?`;
    row.addEventListener('click', e => onPick(row, m, e));
    row.addEventListener('keydown', e => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        onPick(row, m, e);
      }
    });
  } else if (!m.outside) {
    row.href = whoLink(m.email);
    row.title = `${m.name} in Helios Who?`;
  }
  row.append(thumb(m, 'tiny'));
  row.append(el('span', 'compact-name', m.name));
  row.append(el('span', 'compact-email', m.email));
  const words = el('span', 'compact-words');
  if (m.outside) {
    words.append(el('span', 'chip outside', 'Non-Helios'));
  } else {
    words.textContent = m.words || '';
  }
  row.append(words);
  row.append(el('span', 'compact-why', why));
  const tail = el('span', 'compact-tail');
  if (chip) {
    tail.append(chip);
  }
  if (onRemove) {
    tail.append(iconButton('close', `Remove ${m.name}`, 'compact-remove', onRemove));
  }
  row.append(tail);
  return row;
}

let managersEditing = '';

function managersCard(g, canEdit) {
  const card = el('div', 'card managers-card');
  const head = el('div', 'card-head');
  head.append(el('h2', '', 'Managers'));
  const editing = canEdit && managersEditing === g.name;
  if (canEdit) {
    head.append(iconButton(editing ? 'check' : 'edit', editing ? 'Done' : 'Edit managers', '', () => {
      managersEditing = editing ? '' : g.name;
      card.replaceWith(managersCard(g, canEdit));
    }));
  }
  card.append(head);
  const status = el('div', 'save-status');
  const save = async managers => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    try {
      await send('POST', '/api/loop/group', {original: g.name, name: g.name, aliases: g.aliases, title: g.title, description: g.description, prefix: g.prefix, visibility: g.visibility, posting: g.posting, replying: g.replying, managers, rules: g.rules, additions: g.additions, excluded: g.excluded});
      await load();
      toast('Managers saved');
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
    }
  };
  const row = el('div', 'manager-row');
  for (const m of g.managers) {
    const tile = el('a', 'manager-tile');
    tile.href = whoLink(m.email);
    tile.title = `${m.name} in Helios Who?`;
    const face = el('div', 'avatar manager-face');
    if (m.photoUrl) {
      const img = el('img');
      img.src = m.photoUrl;
      img.alt = '';
      img.loading = 'lazy';
      face.append(img);
    } else {
      face.textContent = (m.name || m.email || '?').slice(0, 1).toUpperCase();
    }
    const words = el('div', 'manager-words');
    words.append(el('div', 'manager-name', m.name));
    const subscribed = g.members.some(x => x.email === m.email);
    const reach = el('div', 'manager-state' + (subscribed ? ' is-subscribed' : ''));
    reach.append(svg(subscribed ? 'check' : 'close'), el('span', '', subscribed ? 'Subscribed' : 'Not subscribed'));
    reach.title = subscribed ? 'The rules place them on the group, so its mail reaches them.' : 'No rule places them on the group, so its mail does not reach them.';
    words.append(reach);
    tile.append(face, words);
    if (editing) {
      const remove = iconButton('close', `Remove ${m.name}`, 'manager-remove', () => {
        save(g.managers.map(x => x.email).filter(email => email !== m.email));
      });
      remove.disabled = g.managers.length === 1;
      tile.append(remove);
    }
    row.append(tile);
  }
  card.append(row);
  if (editing) {
    const mount = el('div');
    const picker = createPersonPicker(mount);
    picker.setPeople(state.model.people.filter(p => !g.managers.some(m => m.email === p.email)));
    const add = () => {
      if (picker.value) {
        save([...g.managers.map(m => m.email), picker.value]);
      }
    };
    mount.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        add();
      }
    });
    const addRow = el('div', 'add-row');
    addRow.append(mount, button('Add', 'plus', 'button', add));
    card.append(addRow, status);
  }
  return card;
}

function memberCard(m, rules, title) {
  const tile = el(m.outside ? 'div' : 'a', 'attendee');
  if (!m.outside) {
    tile.href = whoLink(m.email);
  }
  tile.title = title || reasonWords(m, rules);
  const face = el('div', 'avatar attendee-face');
  if (m.photoUrl) {
    const img = el('img');
    img.src = m.photoUrl;
    img.alt = '';
    img.loading = 'lazy';
    face.append(img);
  } else {
    face.textContent = (m.name || m.email || '?').slice(0, 1).toUpperCase();
  }
  if (m.grade) {
    const grade = el('span', 'grade-badge', /^kindergarten$/i.test(m.grade) ? 'K' : m.grade.replace(/^grade\s*/i, ''));
    grade.title = m.grade;
    const color = (state.model.gradeColors || {})[m.grade];
    if (color) {
      grade.style.background = `color-mix(in srgb, ${color} 65%, black)`;
    }
    face.append(grade);
  }
  tile.append(face, el('div', 'attendee-name', m.name));
  const line = m.outside ? 'Guest' : m.context;
  if (line) {
    tile.append(el('div', 'attendee-line', line));
  }
  return tile;
}

function tabbed(tabs, sync = true, initial) {
  const wrap = el('div', 'group-tabs');
  const fallback = tabs[0].key;
  const wanted = sync ? tabParam(fallback) : (initial || fallback);
  let strip = null;
  const show = key => {
    const next = tabStrip(tabs, key, 2, pick => {
      if (sync) {
        history.replaceState(null, '', tabHref(pick));
      }
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

const stateWords = {delivered: 'Delivered', failed: 'Failed', pending: 'Pending'};

const attemptWords = {delivered: 'Delivered', bounced: 'Bounced', delivery_delayed: 'Delayed', complained: 'Marked as spam'};

const stamp = when => when ? new Date(when).toLocaleString() : '';

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
  body.append(el('div', 'person-words', [m.from.name, stamp(m.received)].filter(Boolean).join(' · ')));
  head.append(body);
  const toggle = el('button', 'delivery-toggle');
  toggle.type = 'button';
  for (const [key, n] of [['delivered', m.delivered], ['failed', m.failed], ['pending', m.pending]]) {
    toggle.append(el('span', `delivery-count is-${key}${n ? '' : ' is-zero'}`, `${n} ${key}`));
  }
  toggle.append(svg('chevron'));
  head.append(toggle);
  row.append(head);
  const details = el('div', 'delivery-list');
  details.hidden = true;
  for (const c of m.copies) {
    details.append(copyRow(c));
  }
  if (!m.copies.length) {
    details.append(el('div', 'rule-empty', 'No delivery records for this message.'));
  }
  toggle.addEventListener('click', () => {
    details.hidden = !details.hidden;
    toggle.classList.toggle('open', !details.hidden);
  });
  row.append(details);
  return row;
}

function copyRow(c) {
  const known = state.model.people.find(p => p.email === c.email);
  const words = [stateWords[c.state], stamp(c.when)].filter(Boolean).join(' · ');
  const row = personRow({email: c.email, name: c.name || c.email, photoUrl: known ? known.photoUrl : '', words, outside: !known}, el('span'));
  const attempts = c.attempts.filter(a => a.event !== 'sent');
  if (!attempts.some(a => a.event !== 'delivered')) {
    return row;
  }
  const list = el('div', 'attempt-list');
  for (const a of attempts) {
    const line = el('div', 'attempt');
    line.append(el('span', 'attempt-event', attemptWords[a.event] || a.event), el('span', 'attempt-when', stamp(a.when)));
    if (a.detail) {
      line.append(el('span', 'attempt-detail', a.detail));
    }
    list.append(line);
  }
  row.querySelector('.person-body').append(list);
  return row;
}
