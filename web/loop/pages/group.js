import {state, groupPath, person, personView, loadModel, write, query, rowsOf, domain} from '../state.js';
import {rulesCard, ruleWords, ruleSaysSomething, newRule} from '../listrules.js';
import {pageHead} from '../dom.js';
import {visibilityWords} from './groups.js';
import {el, svg, button, iconButton, copyText, toast} from '/elements.js';
import {setTitle} from '/shell.js';
import {api} from '/api.js';
import {load, navigate} from '/router.js';
import {openLayer} from '/modal.js';
import {createPersonPicker} from '/picker.js';
import {field, text as textInput, textarea as textAreaInput, select as selectInput} from '/form.js';
import {tabStrip, tabParam, tabHref} from '/tabs.js';
import {personCard} from '/people.js';
import {personRow} from '/personrow.js';
import {openPersonCard} from '/personcard.js';
import {memberAdders, personAdder, outsideAdder, membersCard, reasonWords} from '/members.js';
import {keyBetween} from '/order.js';

const visibilityNotes = {
  hidden: 'Only the email list\'s managers and the super admins see it.',
  members: 'The people on the email list see it and can take themselves off it or put themselves back; only managers can change it.',
  everyone: 'Anyone in Loop can see it, and only managers can change it.',
};

const postingWords = {everyone: 'Everyone', members: 'Members', managers: 'Managers', none: 'Nobody'};

const postingNotes = {
  everyone: 'A new message from any address goes out to the email list.',
  members: 'A new message goes out only from the managers and the people on the email list; anyone else gets a note saying so.',
  managers: 'A new message goes out only from the email list\'s managers; anyone else gets a note saying so.',
  none: 'No new message goes out; whoever sends one gets a note saying so.',
};

const replyingNotes = {
  everyone: 'A reply to a message the email list sent goes out from any address.',
  members: 'A reply to a message the email list sent goes out only from the managers and the people on the email list; anyone else gets a note saying so.',
  managers: 'A reply to a message the email list sent goes out only from the email list\'s managers; anyone else gets a note saying so.',
  none: 'No reply goes out; whoever sends one gets a note saying so.',
};

const reserved = ['abuse', 'admin', 'administrator', 'hostmaster', 'noreply', 'no-reply', 'postmaster', 'root', 'unsubscribe', 'webmaster'];

function audienceField(label, notes, value, onChange) {
  const input = selectInput(Object.entries(postingWords).map(([option, words]) => ({value: option, label: words})), value);
  const note = el('small', '', notes[value]);
  input.addEventListener('change', () => {
    note.textContent = notes[input.value];
    onChange(input.value);
  });
  const wrap = field(label, input);
  wrap.append(note);
  return wrap;
}

function ruleOf(row) {
  return {id: row.id, exclude: row.exclude === 'Yes', target: row.target || '', replace_with: row.replace_with || '', within: row.within || '', person: row.person || '', search: row.search || ''};
}

function groupDraft(g, isNew) {
  return {
    id: isNew ? '' : g.id,
    slug: g.slug || '',
    title: g.title || '',
    description: g.description || '',
    aliases: [...(g.aliases || [])],
    visibility: isNew ? 'hidden' : g.visibility,
    posting: isNew ? 'members' : g.posting,
    replying: isNew ? 'members' : g.replying,
    managers: [...(g.managerIds || [])],
    rules: (g.rules || []).map(r => r.id ? ruleOf(r) : r),
    hand: (g.hand || []).map(m => ({person: m.person, member: m.member})),
    guests: [],
  };
}

function editor(g, isNew, closeModal, startTab) {
  const draft = groupDraft(g, isNew);
  const form = el('form', 'editor');
  form.addEventListener('submit', e => e.preventDefault());
  const ed = {
    g, isNew, closeModal, form, draft, original: JSON.parse(JSON.stringify(draft)),
    nameInput: null, nameTouched: false, addressNote: el('small'),
    ruleCounts: new Map(), summary: el('span', 'change-summary'),
    current: isNew ? [] : g.members, previewTimer: null, previewing: false, previewAgain: false, lastPreview: null,
  };
  const membersPanel = el('div');
  membersPanel.append(loopRulesCard(ed), previewCard(ed), keptOffCard(ed));
  form.append(tabbed([
    {key: 'members', label: 'Members', panel: membersPanel},
    {key: 'details', label: 'Details', panel: detailsPanel(ed)},
  ], !closeModal, startTab), editorActions(ed));
  refreshPreview(ed);
  return form;
}

function detailsPanel(ed) {
  const {draft, isNew} = ed;
  const words = el('div', 'card');
  words.append(el('h2', '', isNew ? 'A new email list' : 'The email list'));
  const title = groupTitleInput(ed);
  if (!ed.closeModal) {
    words.append(field('Email list name', title));
  }
  const alias = aliasField(ed);
  words.append(
    addressField(ed, alias),
    alias.field,
    descriptionField(ed),
    el('small', 'subject-note', 'Every message goes out with the email list\'s name in brackets at the front of its subject.'),
    visibilityField(draft),
    audienceField('Who can post', postingNotes, draft.posting, value => {
      draft.posting = value;
    }),
    audienceField('Who can reply', replyingNotes, draft.replying, value => {
      draft.replying = value;
    }),
  );
  const panel = el('div');
  panel.append(words);
  if (isNew) {
    panel.append(managersEditor(draft));
  }
  return panel;
}

function groupTitleInput(ed) {
  const {draft, isNew} = ed;
  const title = textInput(draft.title, {required: true, maxLength: 80, placeholder: 'Soccer Team Families'});
  title.addEventListener('input', () => {
    draft.title = title.value;
    if (isNew && !ed.nameTouched) {
      ed.nameInput.value = slug(title.value);
      draft.slug = ed.nameInput.value;
      updateAddress(ed);
    }
  });
  if (!ed.closeModal) {
    return title;
  }
  title.className = 'modal-title-input';
  title.setAttribute('aria-label', 'Email list name');
  const wanting = () => title.classList.toggle('is-wanted', isNew && !title.value.trim());
  title.addEventListener('input', wanting);
  wanting();
  ed.form.titleInput = title;
  return title;
}

function updateAddress(ed) {
  const {draft} = ed;
  ed.addressNote.textContent = draft.slug ? `${draft.slug}@${domain}` : `The address is <name>@${domain}, and cannot change once made.`;
}

function addressField(ed, alias) {
  const {draft} = ed;
  const name = textInput(draft.slug, {maxLength: 40, placeholder: 'soccer-team'});
  name.disabled = !ed.isNew;
  name.addEventListener('input', () => {
    ed.nameTouched = true;
    draft.slug = slug(name.value);
    updateAddress(ed);
  });
  ed.nameInput = name;
  const nameHead = el('span', 'field-head');
  nameHead.append(el('span', '', 'Address'), button('Add alias', 'plus', 'button button-small button-secondary', () => {
    alias.add.hidden = false;
    alias.input.focus();
  }));
  const nameField = el('div', 'field');
  nameField.append(nameHead, name, ed.addressNote);
  updateAddress(ed);
  return nameField;
}

function aliasField(ed) {
  const {draft} = ed;
  const rows = el('div', 'alias-rows');
  const input = textInput('', {maxLength: 40, placeholder: 'another-name'});
  const status = el('span', 'save-status');
  const add = el('div', 'add-row alias-add');
  add.hidden = true;
  const addAlias = () => {
    const alias = slug(input.value);
    status.classList.remove('error');
    status.textContent = '';
    if (!alias) {
      return;
    }
    if (alias === draft.slug || draft.aliases.includes(alias) || reserved.includes(alias)) {
      status.classList.add('error');
      status.textContent = 'That address cannot be added.';
      return;
    }
    draft.aliases.push(alias);
    input.value = '';
    add.hidden = true;
    renderAliases(draft, rows);
  };
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addAlias();
      return;
    }
    if (e.key === 'Escape') {
      input.value = '';
      add.hidden = true;
    }
  });
  const clear = iconButton('close', 'Clear', 'alias-clear', () => {
    input.value = '';
    status.textContent = '';
    add.hidden = true;
  });
  const box = el('div', 'alias-box');
  box.append(input, clear);
  add.append(box, button('Add', 'plus', 'button button-secondary', addAlias), status, el('small', '', `Another name for the same address, unused by every other email list: <name>@${domain}.`));
  const wrap = el('div', 'field alias-field');
  wrap.append(rows, add);
  renderAliases(draft, rows);
  return {field: wrap, add, input};
}

function renderAliases(draft, rows) {
  rows.replaceChildren();
  rows.hidden = !draft.aliases.length;
  for (const alias of draft.aliases) {
    const row = el('div', 'alias-row');
    const remove = el('button', 'link-button danger', 'Remove');
    remove.type = 'button';
    remove.addEventListener('click', () => {
      draft.aliases = draft.aliases.filter(x => x !== alias);
      renderAliases(draft, rows);
    });
    row.append(el('span', 'alias-address', `${alias}@${domain}`), remove);
    rows.append(row);
  }
}

function descriptionField(ed) {
  const {draft} = ed;
  const desc = textAreaInput(draft.description, 2);
  desc.maxLength = 300;
  desc.placeholder = 'What the email list is for, a line or two.';
  desc.setAttribute('aria-label', 'Description');
  desc.addEventListener('input', () => {
    draft.description = desc.value;
  });
  const status = el('small', 'save-status');
  const generate = button('Generate with AI', 'sparkle', 'button button-small button-secondary', () => describeGroup(ed, desc, status, generate));
  const head = el('span', 'field-head');
  head.append(el('span', '', 'Description'), generate);
  const wrap = el('div', 'field');
  wrap.append(head, desc, status);
  return wrap;
}

function draftBody(ed) {
  const rules = ed.draft.rules.filter(ruleSaysSomething);
  return {
    rules,
    body: {
      list: ed.draft.id,
      name: ed.draft.title,
      ruleWords: rules.map(ruleWords),
      rules: rules.map(r => ({exclude: r.exclude, target: r.target, replace_with: r.replace_with, within: r.within, person: r.person, search: r.search})),
      members: ed.draft.hand,
    },
  };
}

async function describeGroup(ed, desc, status, generate) {
  generate.disabled = true;
  status.classList.remove('error');
  status.textContent = 'Writing…';
  try {
    const {description} = await api('POST', '/api/do/describe-group', draftBody(ed).body);
    desc.value = description;
    ed.draft.description = description;
    status.textContent = '';
    desc.focus();
  } catch (err) {
    status.classList.add('error');
    status.textContent = err.message;
  } finally {
    generate.disabled = false;
  }
}

function visibilityField(draft) {
  const input = selectInput(Object.entries(visibilityWords).map(([value, label]) => ({value, label})), draft.visibility);
  const note = el('small');
  const updateNote = () => {
    note.textContent = visibilityNotes[draft.visibility];
  };
  input.addEventListener('change', () => {
    draft.visibility = input.value;
    updateNote();
  });
  updateNote();
  const wrap = field('Who sees the email list', input);
  wrap.append(note);
  return wrap;
}

function managersEditor(draft) {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Managers'), el('div', 'hint', 'Managers can edit or close the email list.'));
  const rows = el('div');
  const mount = el('div');
  const picker = createPersonPicker(mount, {people: () => state.people.filter(p => !draft.managers.includes(p.id))});
  const addManager = () => {
    const p = picker.value && person(picker.value);
    if (!p || draft.managers.includes(p.id)) {
      return;
    }
    draft.managers.push(p.id);
    picker.reset();
    renderManagerRows(draft, rows);
  };
  mount.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addManager();
    }
  });
  const addRow = el('div', 'add-row');
  addRow.append(mount, button('Add', null, 'button', addManager));
  card.append(rows, addRow);
  renderManagerRows(draft, rows);
  return card;
}

function renderManagerRows(draft, rows) {
  rows.replaceChildren();
  for (const id of draft.managers) {
    const shown = personView(id);
    const remove = el('button', 'link-button danger', 'Remove');
    remove.type = 'button';
    remove.disabled = id === state.model.viewer.id;
    remove.addEventListener('click', () => {
      draft.managers = draft.managers.filter(m => m !== id);
      renderManagerRows(draft, rows);
    });
    rows.append(personRow(shown, {
      className: 'manager-edit-row',
      open: true,
      lines: [[shown.words, shown.email].filter(Boolean).join(' · ')],
      after: [remove],
    }));
  }
}

function setHand(ed, id, member) {
  const {draft} = ed;
  draft.hand = draft.hand.filter(m => m.person !== id);
  if (member) {
    draft.hand.push({person: id, member});
  }
}

function previewCard(ed) {
  const {draft, isNew} = ed;
  const current = new Set(ed.current.map(m => m.person));
  const isOn = email => {
    const p = person(email);
    return Boolean(p && ed.lastPreview && ed.lastPreview.members.some(m => m.person === p.id)) || draft.guests.some(x => x.email === email);
  };
  const add = memberAdders(close => personAdder({
    isOn,
    gradeColors: state.model.gradeColors,
    onAdd: async people => {
      for (const {email} of people) {
        const p = person(email);
        if (p) {
          setHand(ed, p.id, 'yes');
        }
      }
      rulesChanged(ed);
      close();
    },
  }), close => outsideAdder({
    isOn,
    note: 'Someone the directory does not hold - a coach, a league office, a family friend - and their family, on the email list whatever the rules say.',
    onAdd: async people => {
      for (const {email, name} of people) {
        const p = email && person(email);
        if (p) {
          setHand(ed, p.id, 'yes');
          continue;
        }
        if (email && !draft.guests.some(x => x.email === email)) {
          draft.guests.push({email, name});
        }
      }
      rulesChanged(ed);
      close();
    },
  }));
  ed.members = membersCard({
    noun: ['member', 'members'],
    listWord: 'email list',
    adders: add,
    phrase: ruleWords,
    chip: m => (!isNew && m.person && !current.has(m.person) ? el('span', 'chip joins', 'Joins') : null),
    onExclude: m => {
      setHand(ed, m.person, 'excluded');
      rulesChanged(ed);
    },
    onRemove: m => {
      if (m.person) {
        setHand(ed, m.person, '');
      } else {
        draft.guests = draft.guests.filter(x => x.email !== m.email);
      }
      rulesChanged(ed);
    },
    gradeColors: state.model.gradeColors,
  });
  return ed.members.card;
}

function keptOffCard(ed) {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Kept off'), el('div', 'hint', 'People a manager keeps off the email list whatever the rules say.'));
  const rows = el('div');
  card.append(rows);
  ed.keptOff = () => {
    rows.replaceChildren();
    const off = ed.draft.hand.filter(m => m.member === 'excluded');
    card.hidden = !off.length;
    for (const m of off) {
      const shown = personView(m.person);
      const back = el('button', 'link-button', 'Put back');
      back.type = 'button';
      back.addEventListener('click', () => {
        setHand(ed, m.person, '');
        rulesChanged(ed);
      });
      rows.append(personRow(shown, {className: 'manager-edit-row', lines: [shown.email || shown.words], after: [back]}));
    }
  };
  ed.keptOff();
  return card;
}

function renderChanges(ed, members, rules) {
  const {isNew} = ed;
  const {changes} = ed.members;
  const current = new Set(ed.current.map(m => m.person));
  const now = new Set(members.map(m => m.person).filter(Boolean));
  const joining = isNew ? [] : members.filter(m => m.person && !current.has(m.person));
  const leaving = ed.current.filter(m => !now.has(m.person));
  changes.replaceChildren();
  changes.hidden = isNew || (!joining.length && !leaving.length);
  if (!isNew) {
    ed.summary.textContent = joining.length || leaving.length ? `${joining.length} ${joining.length === 1 ? 'joins' : 'join'} · ${leaving.length} ${leaving.length === 1 ? 'leaves' : 'leave'}` : 'Nobody joins or leaves';
    for (const [chip, word, people] of [['joins', joining.length === 1 ? 'joins' : 'join', joining], ['leaves', leaving.length === 1 ? 'leaves' : 'leave', leaving]]) {
      if (people.length) {
        const line = el('div', 'change-line');
        line.append(el('span', 'chip ' + chip, `${people.length} ${word}`), el('span', '', people.map(m => m.name).join(', ')));
        changes.append(line);
      }
    }
  }
  ed.lastPreview = {members, rules, leaving};
  const joins = m => !isNew && m.person && !current.has(m.person);
  const ordered = [...members.filter(m => m.outside), ...members.filter(m => !m.outside)];
  ed.members.show({members: [...ordered.filter(joins), ...ordered.filter(m => !joins(m))], rules, leaving});
}

async function refreshPreview(ed) {
  const {status} = ed.members;
  if (ed.previewing) {
    ed.previewAgain = true;
    return;
  }
  ed.previewing = true;
  status.classList.remove('error');
  status.textContent = 'Working out the members…';
  try {
    const {rules, body} = draftBody(ed);
    const preview = await api('POST', '/api/do/draft-members', body);
    const members = preview.members.map(m => {
      const shown = personView(m.person);
      return {...shown, name: shown.email ? shown.name : m.name || shown.name, reasons: m.reasons};
    });
    for (const x of ed.draft.guests) {
      members.push({email: x.email, name: x.name || x.email, words: 'Outside the directory', outside: true, reasons: [{added: true}]});
    }
    renderChanges(ed, members, rules);
    ed.ruleCounts.clear();
    rules.forEach((rule, i) => ed.ruleCounts.set(rule, preview.ruleCounts[i]));
    ed.rules.render();
    ed.keptOff();
    status.textContent = rules.length || members.length ? '' : 'Add a rule, or add people by hand.';
  } catch (err) {
    status.classList.add('error');
    status.textContent = err.message;
  }
  ed.previewing = false;
  if (ed.previewAgain) {
    ed.previewAgain = false;
    refreshPreview(ed);
  }
}

function rulesChanged(ed) {
  clearTimeout(ed.previewTimer);
  ed.previewTimer = setTimeout(() => refreshPreview(ed), 250);
}

function loopRulesCard(ed) {
  const made = rulesCard({
    list: () => ed.draft.rules,
    onAdd: rule => ed.draft.rules.push(rule),
    onChange: () => rulesChanged(ed),
    onRemove: rule => {
      ed.draft.rules = ed.draft.rules.filter(r => r !== rule);
      ed.rules.render();
      rulesChanged(ed);
    },
    count: r => ed.ruleCounts.get(r),
  });
  ed.rules = made;
  return made.card;
}

function editorActions(ed) {
  const {g, isNew, closeModal} = ed;
  const actions = el('div', 'editor-actions');
  const status = el('span', 'save-status');
  const save = button(isNew ? 'Make the email list' : 'Save', 'check', 'button', () => saveGroup(ed, save, status));
  actions.append(save);
  if (isNew) {
    actions.append(button('Cancel', null, 'button button-secondary', () => {
      if (closeModal) {
        closeModal();
      }
      navigate('/');
    }), status);
    return actions;
  }
  const close = el('button', 'danger-button', 'Close email list');
  close.type = 'button';
  close.addEventListener('click', () => closeGroup(ed, status));
  actions.append(button('Cancel', null, 'button button-secondary', () => {
    if (closeModal) {
      closeModal();
    } else {
      navigate(withTab(groupPath(g)));
    }
  }), ed.summary, status, close);
  return actions;
}

function visibleTo(visibility, id) {
  return {hidden: '', members: id, everyone: state.model.everyone}[visibility];
}

function ruleRow(group, r, order) {
  const row = {group, order};
  if (r.exclude) {
    row.exclude = true;
  }
  for (const column of ['target', 'replace_with', 'within', 'person', 'search']) {
    if (r[column]) {
      row[column] = r[column];
    }
  }
  return row;
}

function ruleEdits(ed, group) {
  const {draft, original, g} = ed;
  const now = draft.rules.filter(ruleSaysSomething);
  const strip = rules => JSON.stringify(rules.map(r => ({...r, id: undefined})));
  if (!ed.isNew && strip(now) === strip(original.rules.filter(ruleSaysSomething))) {
    return [];
  }
  const edits = ed.isNew ? [] : g.rules.map(r => ({delete: r.id}));
  let order = '';
  for (const r of now) {
    order = keyBetween(order, null);
    edits.push({insert: 'RULE', row: ruleRow(group, r, order)});
  }
  return edits;
}

function handEdits(ed, group) {
  const before = new Map((ed.isNew ? [] : ed.g.hand).map(m => [m.person, m]));
  const edits = [];
  for (const m of ed.draft.hand) {
    const was = before.get(m.person);
    before.delete(m.person);
    if (!was) {
      edits.push({insert: 'MEMBER', row: {group, person: m.person, member: m.member}});
      continue;
    }
    if (was.member !== m.member) {
      edits.push({set: was.id, cells: {member: m.member}});
    }
  }
  for (const was of before.values()) {
    edits.push({delete: was.id});
  }
  return edits;
}

function aliasEdits(ed, group) {
  const before = ed.isNew ? [] : ed.g.aliasRows;
  const edits = before.filter(a => !ed.draft.aliases.includes(a.alias)).map(a => ({delete: a.id}));
  for (const alias of ed.draft.aliases.filter(a => !before.some(b => b.alias === a))) {
    edits.push({insert: 'ALIAS', row: {alias, target: group}});
  }
  return edits;
}

async function addGuests(ed, group) {
  const edits = [];
  for (const x of ed.draft.guests) {
    const {result} = await api('POST', '/api/do/guest', {email: x.email, name: x.name});
    edits.push({insert: 'MEMBER', row: {group, person: result[0], member: 'yes'}});
  }
  if (edits.length) {
    await write(edits);
  }
}

async function makeGroup(ed) {
  const {draft} = ed;
  if (!draft.slug || reserved.includes(draft.slug)) {
    throw new Error('Give the email list an address.');
  }
  const viewer = state.model.viewer.id;
  const list = {kind: 'group', listed: true, slug: draft.slug, name: draft.title.trim(), status: 'open', members_visible_to: state.model.everyone, posting: draft.posting, replying: draft.replying, managed_by: '@managers', added_by: viewer};
  if (draft.description.trim()) {
    list.description = draft.description.trim();
  }
  if (draft.visibility === 'everyone') {
    list.visible_to = state.model.everyone;
  }
  const edits = [
    {insert: 'GROUP', as: 'managers', row: {kind: 'group', name: `${list.name} Managers`, status: 'open', added_by: viewer}},
    {set: '@managers', cells: {managed_by: '@managers'}},
    {insert: 'MEMBER', row: {group: '@managers', person: viewer, member: 'yes'}},
    ...draft.managers.filter(id => id !== viewer).map(id => ({insert: 'MEMBER', row: {group: '@managers', person: id, member: 'yes'}})),
    {insert: 'GROUP', as: 'list', row: list},
  ];
  if (draft.visibility === 'members') {
    edits.push({set: '@list', cells: {visible_to: '@list'}});
  }
  edits.push(...ruleEdits(ed, '@list'), ...handEdits(ed, '@list'), ...aliasEdits(ed, '@list'));
  const {result} = await write(edits);
  await addGuests(ed, result[edits.findIndex(e => e.as === 'list')]);
}

async function changeGroup(ed) {
  const {draft, original, g} = ed;
  const cells = {};
  for (const [key, column] of [['title', 'name'], ['description', 'description'], ['posting', 'posting'], ['replying', 'replying']]) {
    if (draft[key] !== original[key]) {
      cells[column] = draft[key].trim();
    }
  }
  if (draft.visibility !== original.visibility) {
    cells.visible_to = visibleTo(draft.visibility, g.id);
  }
  const edits = Object.keys(cells).length ? [{set: g.id, cells}] : [];
  edits.push(...ruleEdits(ed, g.id), ...handEdits(ed, g.id), ...aliasEdits(ed, g.id));
  if (edits.length) {
    await write(edits);
  }
  await addGuests(ed, g.id);
}

async function saveGroup(ed, save, status) {
  const {draft, isNew, closeModal} = ed;
  status.classList.remove('error');
  status.textContent = 'Saving…';
  save.disabled = true;
  try {
    if (isNew) {
      await makeGroup(ed);
    } else {
      await changeGroup(ed);
    }
    if (closeModal) {
      closeModal();
    }
    if (isNew) {
      await loadModel();
      toast('Email list made');
      navigate(groupPath(draft));
      return;
    }
    await load();
    toast('Saved');
    if (!closeModal) {
      navigate(withTab(groupPath(draft)));
    }
  } catch (err) {
    status.classList.add('error');
    status.textContent = err.message;
    save.disabled = false;
  }
}

async function closeGroup(ed, status) {
  const {g, closeModal} = ed;
  if (!confirm(`Close ${g.address}? Mail sent to it will go nowhere.`)) {
    return;
  }
  try {
    await write([{set: g.id, cells: {status: 'closed'}}]);
    if (closeModal) {
      closeModal();
    }
    await loadModel();
    toast('Email list closed');
    navigate('/');
  } catch (err) {
    status.classList.add('error');
    status.textContent = err.message;
  }
}

function editModal(g, startTab, isNew) {
  const overlay = el('div', 'modal-overlay');
  const box = el('div', 'modal modal-editor');
  box.setAttribute('role', 'dialog');
  box.setAttribute('aria-modal', 'true');
  box.setAttribute('aria-label', isNew ? 'New email list' : 'Edit ' + g.title);
  const onKey = e => {
    if (e.key === 'Escape') {
      close();
      if (isNew) {
        navigate('/');
      }
    }
  };
  const close = openLayer(() => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  });
  const form = editor(g, isNew, close, startTab);
  const header = el('div', 'modal-header');
  const heading = el('div', 'modal-heading');
  heading.append(form.titleInput, el('small', 'modal-heading-hint', isNew ? 'Type the email list\'s name' : 'Click to edit'));
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
  editModal({managerIds: [state.model.viewer.id], rules: [newRule()]}, null, true);
}

function unsubscribeButton(g) {
  const toggle = button(g.unsubscribed ? 'Resubscribe' : 'Unsubscribe', null, 'button button-secondary', async () => {
    toggle.disabled = true;
    try {
      await api('POST', '/api/do/unsubscribe', {group: g.id, subscribed: g.unsubscribed});
      await load();
      toast(g.unsubscribed ? 'Resubscribed' : 'Unsubscribed');
    } catch (err) {
      toggle.disabled = false;
      toast(err.message);
    }
  });
  return toggle;
}

export function groupPage(g) {
  setTitle(g.title);
  const page = el('div', 'group-page');
  const editing = new URLSearchParams(location.search).get('edit') === '1';
  const canEdit = g.run;
  if (editing && canEdit) {
    page.append(pageHead(g.title));
    page.append(editor(g, false));
    return page;
  }
  const actions = [];
  if (canEdit) {
    actions.push(button('Edit', 'edit', 'button', () => editModal(g)));
  }
  if (g.member || g.unsubscribed) {
    actions.push(unsubscribeButton(g));
  }
  page.append(pageHead(g.title, actions));
  page.append(overview(g));

  const members = el('div');
  const grid = el('div', 'person-cards');
  const search = el('input', 'member-search');
  search.type = 'search';
  search.placeholder = 'Search the members…';
  search.setAttribute('aria-label', 'Search the members');
  const empty = el('div', 'rule-empty');
  const rules = g.rules.map(ruleOf);
  const showMembers = () => {
    const q = search.value.trim().toLowerCase();
    grid.replaceChildren();
    const shown = g.members.filter(m => !q || m.name.toLowerCase().includes(q) || m.email.toLowerCase().includes(q) || (m.context || '').toLowerCase().includes(q));
    for (const m of shown) {
      grid.append(memberCard(canEdit ? m : {...m, reasons: []}, rules));
    }
    empty.textContent = g.members.length ? 'Nobody matches that.' : 'Nobody is on the email list yet.';
    empty.hidden = shown.length > 0;
  };
  search.addEventListener('input', showMembers);
  showMembers();
  if (g.members.length) {
    members.append(search);
  }
  members.append(grid, empty);
  if (canEdit) {
    members.append(rulesSummary(g, rules));
  }

  const managersPanel = el('div');
  managersPanel.append(managersCard(g, canEdit));
  const tabs = [
    {key: 'members', label: 'Members', icon: svg('check'), count: g.members.length, panel: members},
    {key: 'managers', label: 'Managers', icon: svg('star'), count: g.managers.length, panel: managersPanel},
  ];
  if (canEdit) {
    tabs.push(historyTab(g));
  }
  page.append(tabbed(tabs, true, null, tabs.length > 2 ? 1 : 2));
  return page;
}

function addressBand(g) {
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
      tip.append(el('div', '', `${alias}@${domain}`));
    }
    aliases.append(count, tip);
    address.append(aliases);
  }
  return address;
}

function overview(g) {
  const wrap = el('div', 'group-overview');
  if (g.address) {
    wrap.append(addressBand(g));
  }
  wrap.append(el('div', 'subject-note', `Every message goes out with “[${g.title}]” at the front of its subject.`));
  if (g.visibility === 'everyone') {
    wrap.append(el('div', 'subject-note', 'Visible to everyone in Loop; only its managers can change it.'));
  }
  if (g.visibility === 'members') {
    wrap.append(el('div', 'subject-note', 'Visible to the people on it; only its managers can change it.'));
  }
  const audienceNotes = {everyone: 'Anyone', members: 'Only its managers and the people on it', managers: 'Only its managers', none: 'Nobody'};
  if (g.posting !== 'everyone') {
    wrap.append(el('div', 'subject-note', `${audienceNotes[g.posting]} can post new messages.`));
  }
  if (g.replying !== g.posting) {
    wrap.append(el('div', 'subject-note', `${audienceNotes[g.replying]} can reply to its messages.`));
  }
  if (g.unsubscribed) {
    wrap.append(el('div', 'subject-note', 'You unsubscribed and get no mail from it. Resubscribe to get its mail again.'));
  }
  if (g.description) {
    wrap.append(el('p', 'page-lead', g.description));
  }
  return wrap;
}

function rulesSummary(g, rules) {
  const card = el('div', 'card');
  const head = el('div', 'card-head');
  head.append(el('h2', '', 'How the members are chosen'));
  head.append(iconButton('edit', 'Edit the rules', '', () => editModal(g, 'members')));
  card.append(head);
  const lines = el('div', 'rule-lines');
  for (const r of rules) {
    const line = el('div', 'rule-line');
    const sign = el('span', 'rule-sign rule-sign-' + (r.exclude ? 'exclude' : 'include'));
    sign.append(svg(r.exclude ? 'minus' : 'plus'));
    sign.title = r.exclude ? 'Left out' : 'Included';
    line.append(sign, el('span', 'rule-line-words', ruleWords(r)));
    lines.append(line);
  }
  card.append(lines);
  const added = g.hand.filter(m => m.member === 'yes').length;
  const off = g.hand.filter(m => m.member === 'excluded').length;
  if (added) {
    card.append(el('div', 'subject-note', `${added} ${added === 1 ? 'person is' : 'people are'} on the email list by hand.`));
  }
  if (off) {
    card.append(el('div', 'subject-note', `${off} ${off === 1 ? 'person is' : 'people are'} kept off the email list whatever the rules say.`));
  }
  return card;
}

let managersEditing = '';

function managersCard(g, canEdit) {
  const card = el('div', 'card managers-card');
  const head = el('div', 'card-head');
  head.append(el('h2', '', 'Managers'));
  const editing = canEdit && managersEditing === g.id && g.managedBy;
  if (canEdit && g.managedBy) {
    head.append(iconButton(editing ? 'check' : 'edit', editing ? 'Done' : 'Edit managers', '', () => {
      managersEditing = editing ? '' : g.id;
      card.replaceWith(managersCard(g, canEdit));
    }));
  }
  card.append(head);
  const status = el('div', 'save-status');
  const save = async edits => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    try {
      await write(edits);
      await load();
      toast('Managers saved');
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
    }
  };
  const row = el('div', 'manager-row');
  for (const m of g.managers) {
    const tile = el('button', 'manager-tile');
    tile.type = 'button';
    tile.title = m.name;
    tile.addEventListener('click', () => openPersonCard(m));
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
    const subscribed = g.members.some(x => x.person === m.person);
    const reach = el('div', 'manager-state' + (subscribed ? ' is-subscribed' : ''));
    reach.append(svg(subscribed ? 'check' : 'close'), el('span', '', subscribed ? 'Subscribed' : 'Not subscribed'));
    reach.title = subscribed ? 'They are on the email list, so its mail reaches them.' : 'They are not on the email list, so its mail does not reach them.';
    words.append(reach);
    tile.append(face, words);
    if (editing) {
      const rowOf = g.managerRows.find(x => x.person === m.person);
      const remove = iconButton('close', `Remove ${m.name}`, 'manager-remove', () => save([{delete: rowOf.id}]));
      remove.disabled = g.managers.length === 1;
      tile.append(remove);
    }
    row.append(tile);
  }
  card.append(row);
  if (editing) {
    const mount = el('div');
    const picker = createPersonPicker(mount, {people: () => state.people.filter(p => !g.managers.some(m => m.person === p.id))});
    const add = () => {
      const p = picker.value && person(picker.value);
      if (p) {
        save([{insert: 'MEMBER', row: {group: g.managedBy, person: p.id, member: 'yes'}}]);
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

function memberCard(m, rules) {
  return personCard(m, {
    onClick: () => openPersonCard(m),
    title: reasonWords(m, rules, ruleWords),
    line: m.outside ? 'Guest' : m.context,
    gradeColors: state.model.gradeColors,
  });
}

function tabbed(tabs, sync = true, initial, mobileVisible = 2) {
  const wrap = el('div', 'group-tabs');
  const fallback = tabs[0].key;
  const wanted = sync ? tabParam(fallback) : (initial || fallback);
  let strip = null;
  const show = key => {
    const next = tabStrip(tabs, key, mobileVisible, pick => {
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

const postWords = {received: 'Waiting to go out', dropped: 'Not sent', failed: 'Failed'};

const when = stamp => stamp ? new Date(stamp.replace(' ', 'T')).toLocaleString() : '';

function copyState(c) {
  if (c.delivered) {
    return {state: 'delivered', at: c.delivered};
  }
  if (c.failed) {
    return {state: 'failed', at: c.failed};
  }
  return {state: 'pending', at: c.sent};
}

function historyTab(g) {
  const panel = el('div', 'card');
  const status = el('div', 'save-status', 'Loading…');
  const list = el('div', 'message-list');
  panel.append(status, list);
  let loaded = false;
  const fetchHistory = async () => {
    loaded = true;
    const id = JSON.stringify(g.id);
    try {
      const answers = await query({
        posts: `(from MESSAGE (where (= group ${id}) (= kind "post") (= direction "in")) (order created desc))`,
        outs: `(from MESSAGE (where (= group ${id}) (= kind "post") (= direction "out")) (columns parent))`,
        copies: `(from RECIPIENT (where (in message (select MESSAGE.id (= group ${id}) (= kind "post") (= direction "out")))) (columns message person sent delivered failed detail))`,
      });
      const outOf = {};
      for (const o of rowsOf(answers.outs, 'MESSAGE')) {
        outOf[o.parent] = o.id;
      }
      const copiesOf = {};
      for (const c of rowsOf(answers.copies, 'RECIPIENT')) {
        (copiesOf[c.message] = copiesOf[c.message] || []).push({...c, ...copyState(c)});
      }
      const posts = rowsOf(answers.posts, 'MESSAGE');
      status.textContent = posts.length ? '' : 'Nothing has been sent to the email list yet.';
      for (const p of posts) {
        list.append(messageRow(p, copiesOf[outOf[p.id]] || []));
      }
    } catch (err) {
      loaded = false;
      status.classList.add('error');
      status.textContent = err.message;
    }
  };
  return {key: 'history', label: 'History', count: g.sent, panel, onShow: () => {
    if (!loaded) {
      fetchHistory();
    }
  }};
}

function messageRow(p, copies) {
  const row = el('div', 'message-row');
  const from = p.from_person ? personView(p.from_person) : {email: p.from_address, name: p.from_address};
  const toggle = el('button', 'delivery-toggle');
  toggle.type = 'button';
  if (p.state === 'sent') {
    for (const key of ['delivered', 'failed', 'pending']) {
      const n = copies.filter(c => c.state === key).length;
      toggle.append(el('span', `delivery-count is-${key}${n ? '' : ' is-zero'}`, `${n} ${key}`));
    }
  } else {
    toggle.append(el('span', 'delivery-count is-failed', postWords[p.state] || p.state));
  }
  toggle.append(svg('chevron-down'));
  row.append(personRow(from, {
    name: p.subject || '(no subject)',
    open: Boolean(from.email && person(from.email)),
    lines: [[from.name, when(p.created)].filter(Boolean).join(' · ')],
    after: [toggle],
  }));
  const details = el('div', 'delivery-list');
  details.hidden = true;
  if (p.detail) {
    details.append(el('div', 'rule-empty', p.detail));
  }
  for (const c of copies) {
    const shown = personView(c.person);
    details.append(personRow(shown, {
      className: 'copy-row',
      open: Boolean(shown.email && person(shown.email)),
      lines: [[stateWords[c.state], when(c.at), c.state === 'failed' ? c.detail : '', shown.email].filter(Boolean).join(' · ')],
    }));
  }
  if (!copies.length && !p.detail) {
    details.append(el('div', 'rule-empty', 'No copies recorded for this message.'));
  }
  toggle.addEventListener('click', () => {
    details.hidden = !details.hidden;
    toggle.classList.toggle('open', !details.hidden);
  });
  row.append(details);
  return row;
}
