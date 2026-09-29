import {state, me, options, groupPath, person, personView, memberView, loadModel} from '../state.js';
import {pageHead} from '../dom.js';
import {el, svg, button, iconButton, copyText, toast} from '/elements.js';
import {whoLink} from '/appswitch.js';
import {setTitle} from '/shell.js';
import {api} from '/api.js';
import {query, act, create, remove} from '/data.js';
import {load, navigate} from '/router.js';
import {openLayer} from '/modal.js';
import {createPersonPicker} from '/picker.js';
import {field, text as textInput, textarea as textAreaInput, select as selectInput} from '/form.js';
import {tabStrip, tabParam, tabHref} from '/tabs.js';
import {rulesEditor} from '/rules.js';
import {visibilityWords} from './groups.js';
import {personCard} from '/people.js';
import {personRow} from '/personrow.js';
import {openPersonCard} from '/personcard.js';

const {ruleRow, newRule, ruleSaysSomething, personWords, ruleWords} = rulesEditor({
  options,
  personName: email => (person(email) || {}).fullName || '',
});

const visibilityNotes = {
  hidden: 'Only the email list\'s managers and the admins see it.',
  members: 'Anyone the rules or the additions place on the email list can see it, and take themselves off it or put themselves back; only managers can change it.',
  everyone: 'Anyone in Loop can see it, and only managers can change it.',
};

const postingWords = {everyone: 'Everyone', members: 'Members', managers: 'Managers'};

const postingNotes = {
  everyone: 'A new message from any address goes out to the email list.',
  members: 'A new message goes out only from the managers and the people the rules or the additions place on the email list; anyone else gets a note saying so.',
  managers: 'A new message goes out only from the email list\'s managers; anyone else gets a note saying so.',
};

const replyingNotes = {
  everyone: 'A reply to a message the email list sent goes out from any address.',
  members: 'A reply to a message the email list sent goes out only from the managers and the people the rules or the additions place on the email list; anyone else gets a note saying so.',
  managers: 'A reply to a message the email list sent goes out only from the email list\'s managers; anyone else gets a note saying so.',
};

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

function groupDraft(g, isNew) {
  return {
    id: isNew ? '' : g.id, name: g.name, aliases: [...(g.aliases || [])], title: g.title, description: g.description || '',
    prefix: isNew ? true : g.prefix,
    visibility: isNew ? 'hidden' : g.visibility,
    posting: isNew ? 'everyone' : g.posting,
    replying: isNew ? 'everyone' : g.replying,
    managers: g.managers.map(m => m.email),
    rules: g.rules.map(r => ({kind: r.kind, roles: [...r.roles], search: r.search, classrooms: [...r.classrooms], grades: [...r.grades], tags: [...r.tags], family: [...r.family], tagLabels: r.tagLabels})),
    additions: (g.additions || []).map(a => ({email: a.email, name: a.name})),
    excluded: (g.excluded || []).map(e => ({email: e.email, note: e.note || '', when: e.when || ''})),
  };
}

function editor(g, isNew, closeModal, startTab) {
  const draft = groupDraft(g, isNew);
  const firstRule = isNew && !draft.rules.length ? newRule('include') : null;
  if (firstRule) {
    draft.rules.push(firstRule);
  }
  const form = el('form', 'editor');
  form.addEventListener('submit', e => e.preventDefault());
  const ed = {
    g, isNew, closeModal, form, draft, original: JSON.parse(JSON.stringify(draft)),
    nameInput: null, nameTouched: false, addressNote: el('small'),
    opened: firstRule, ruleRows: el('div', 'rules'), ruleCounts: new Map(),
    preview: previewParts(g, isNew), previewTimer: null, previewing: false, previewAgain: false, lastPreview: null,
  };
  const overviewPanel = detailsPanel(ed);
  const rulesPanel = el('div');
  rulesPanel.append(rulesCard(ed), previewCard(ed));
  form.append(tabbed([
    {key: 'members', label: 'Members', panel: rulesPanel},
    {key: 'details', label: 'Details', panel: overviewPanel},
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
    ...prefixFields(draft, title),
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
      draft.name = ed.nameInput.value;
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
  ed.addressNote.textContent = draft.name ? `${draft.name}@${state.model.domain}` : `The address is <name>@${state.model.domain}, and cannot change once made.`;
}

function addressField(ed, alias) {
  const {draft} = ed;
  const name = textInput(draft.name, {maxLength: 40, placeholder: 'soccer-team'});
  name.disabled = !ed.isNew;
  name.addEventListener('input', () => {
    ed.nameTouched = true;
    draft.name = slug(name.value);
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
    if (alias === draft.name || draft.aliases.includes(alias)) {
      status.classList.add('error');
      status.textContent = 'Already this email list\'s.';
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
  add.append(box, button('Add', 'plus', 'button button-secondary', addAlias), status, el('small', '', `Another name for the same address, unused by every other email list:<name>@${state.model.domain}.`));
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
    row.append(el('span', 'alias-address', `${alias}@${state.model.domain}`), remove);
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

async function describeGroup(ed, desc, status, generate) {
  const {draft} = ed;
  generate.disabled = true;
  status.classList.remove('error');
  status.textContent = 'Writing…';
  try {
    const rules = draft.rules.filter(ruleSaysSomething);
    const {description} = await api('POST', '/api/loop/describe', {id: draft.id, title: draft.title, ruleWords: rules.map(r => (r.kind === 'exclude' ? 'Leaving out: ' : '') + ruleWords(r)), rules, additions: draft.additions, excluded: draft.excluded});
    desc.value = description;
    draft.description = description;
    status.textContent = '';
    desc.focus();
  } catch (err) {
    status.classList.add('error');
    status.textContent = err.message;
  } finally {
    generate.disabled = false;
  }
}

function prefixFields(draft, title) {
  const note = el('small');
  const updateNote = () => {
    note.textContent = draft.prefix ? `Every message goes out with “[${draft.title || 'Email list name'}]” at the front of its subject.` : 'Subjects go out as written.';
  };
  const prefix = el('input');
  prefix.type = 'checkbox';
  prefix.checked = draft.prefix;
  prefix.addEventListener('change', () => {
    draft.prefix = prefix.checked;
    updateNote();
  });
  title.addEventListener('input', updateNote);
  updateNote();
  const wrap = el('label', 'field check');
  wrap.append(prefix, el('span', '', 'Put the email list name in front of every subject'));
  return [wrap, note];
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
  card.append(el('h2', '', 'Managers'), el('div', 'hint', 'Managers can edit or delete the email list.'));
  const rows = el('div');
  const mount = el('div');
  const picker = createPersonPicker(mount, {people: () => state.people.filter(p => !draft.managers.includes(p.email))});
  const addManager = () => {
    const email = picker.value;
    if (!email || draft.managers.includes(email)) {
      return;
    }
    draft.managers.push(email);
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
  for (const email of draft.managers) {
    const known = person(email);
    const shown = known ? {email, name: known.fullName, photoUrl: known.heroPhotoUrl, words: known.words} : {email, name: email};
    const remove = el('button', 'link-button danger', 'Remove');
    remove.type = 'button';
    remove.disabled = draft.managers.length === 1;
    remove.addEventListener('click', () => {
      draft.managers = draft.managers.filter(m => m !== email);
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

function previewParts(g, isNew) {
  const current = isNew ? [] : g.members;
  const changes = el('div', 'change-band');
  changes.hidden = true;
  const search = el('input', 'member-search');
  search.type = 'search';
  search.placeholder = 'Search the members…';
  search.setAttribute('aria-label', 'Search the members');
  const empty = el('div', 'rule-empty', 'Nobody matches that.');
  empty.hidden = true;
  return {
    head: el('h2', '', 'Members'),
    headRow: el('div', 'card-head'),
    changes,
    list: el('div', 'compact-list'),
    status: el('div', 'save-status'),
    search,
    empty,
    summary: el('span', 'change-summary'),
    current,
    currentEmails: new Set(current.map(m => m.email)),
  };
}

function previewCard(ed) {
  const p = ed.preview;
  const person = personAdder(ed);
  const addition = additionAdder(ed);
  const headButtons = el('div', 'head-buttons');
  headButtons.append(button('Add Helios', 'plus', 'button button-small button-secondary', () => {
    addition.node.hidden = true;
    person.node.hidden = false;
    person.mount.querySelector('input').focus();
  }), button('Add Non-Helios', 'plus', 'button button-small button-secondary', () => {
    person.node.hidden = true;
    addition.node.hidden = false;
    addition.name.focus();
  }));
  p.headRow.append(p.head, headButtons);
  p.search.addEventListener('input', () => drawPreviewList(ed));
  const card = el('div', 'card preview');
  card.append(p.headRow, person.node, addition.node, p.status, p.changes, p.search, p.list, p.empty);
  return card;
}

function additionAdder(ed) {
  const {draft} = ed;
  const name = textInput('', {maxLength: 80, placeholder: 'Name'});
  const email = textInput('', {type: 'email', maxLength: 120, placeholder: 'name@example.org'});
  const status = el('span', 'save-status');
  const node = el('div', 'add-row addition-add');
  node.hidden = true;
  const addAddition = () => {
    const address = email.value.trim().toLowerCase();
    const who = name.value.trim();
    status.classList.remove('error');
    status.textContent = '';
    if (!address.includes('@')) {
      status.classList.add('error');
      status.textContent = 'An email address is needed.';
      return;
    }
    if (draft.additions.some(a => a.email === address)) {
      status.classList.add('error');
      status.textContent = 'Already added.';
      return;
    }
    draft.additions.push({email: address, name: who});
    name.value = '';
    email.value = '';
    node.hidden = true;
    rulesChanged(ed);
  };
  for (const input of [name, email]) {
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        addAddition();
      }
    });
  }
  node.append(name, email, button('Add', 'plus', 'button button-secondary', addAddition), iconButton('close', 'Never mind', '', () => {
    name.value = '';
    email.value = '';
    status.textContent = '';
    node.hidden = true;
  }), status, el('small', '', 'Someone the directory does not hold - a coach, a league office, a family friend - on the email list whatever the rules say.'));
  return {node, name};
}

function personAdder(ed) {
  const {draft} = ed;
  const mount = el('div');
  const picker = createPersonPicker(mount, {people: () => state.people});
  const status = el('span', 'save-status');
  const node = el('div', 'add-row addition-add');
  node.hidden = true;
  const addPerson = () => {
    const email = picker.value;
    status.classList.remove('error');
    status.textContent = '';
    if (!email) {
      status.classList.add('error');
      status.textContent = 'Pick someone from the list.';
      return;
    }
    if (!draft.rules.some(r => r.kind === 'include' && r.search === email && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length)) {
      draft.rules.push({...newRule('include'), search: email});
      renderRules(ed);
    }
    picker.reset();
    node.hidden = true;
    rulesChanged(ed);
  };
  mount.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addPerson();
    }
  });
  node.append(mount, button('Add', 'plus', 'button button-secondary', addPerson), iconButton('close', 'Never mind', '', () => {
    picker.reset();
    status.textContent = '';
    node.hidden = true;
  }), status, el('small', '', 'Someone from the directory, on the email list by a rule of their own.'));
  return {node, mount};
}

function pickMember(ed, row, m) {
  const {draft} = ed;
  for (const other of document.querySelectorAll('.member-menu')) {
    other.remove();
  }
  const menu = el('div', 'member-menu');
  const exclude = el('button', 'member-menu-item');
  exclude.type = 'button';
  exclude.append(svg('user-minus'), el('span', '', `Exclude ${m.name} from this email list`));
  exclude.addEventListener('click', e => {
    e.stopPropagation();
    menu.remove();
    if (!draft.rules.some(r => r.kind === 'exclude' && r.search === m.email && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length)) {
      draft.rules.push({...newRule('exclude'), search: m.email});
      renderRules(ed);
    }
    rulesChanged(ed);
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
}

function renderChanges(ed, members, rules) {
  const {isNew} = ed;
  const {changes, summary, current, currentEmails} = ed.preview;
  const memberEmails = new Set(members.map(m => m.email));
  const joining = isNew ? [] : members.filter(m => !currentEmails.has(m.email));
  const leaving = current.filter(m => !memberEmails.has(m.email));
  changes.replaceChildren();
  changes.hidden = isNew || (!joining.length && !leaving.length);
  if (!isNew) {
    summary.textContent = joining.length || leaving.length ? `${joining.length} ${joining.length === 1 ? 'joins' : 'join'} · ${leaving.length} ${leaving.length === 1 ? 'leaves' : 'leave'}` : 'Nobody joins or leaves';
    if (joining.length) {
      const line = el('div', 'change-line');
      line.append(el('span', 'chip joins', `${joining.length} ${joining.length === 1 ? 'joins' : 'join'}`), el('span', '', joining.map(m => m.name).join(', ')));
      changes.append(line);
    }
    if (leaving.length) {
      const line = el('div', 'change-line');
      line.append(el('span', 'chip leaves', `${leaving.length} ${leaving.length === 1 ? 'leaves' : 'leave'}`), el('span', '', leaving.map(m => m.name).join(', ')));
      changes.append(line);
    }
  }
  ed.lastPreview = {members, rules, leaving};
  drawPreviewList(ed);
}

function drawPreviewList(ed) {
  if (!ed.lastPreview) {
    return;
  }
  const {draft, isNew} = ed;
  const {search, list, empty, currentEmails} = ed.preview;
  const {members, rules, leaving} = ed.lastPreview;
  const q = search.value.trim().toLowerCase();
  const wanted = m => !q || m.name.toLowerCase().includes(q) || m.email.toLowerCase().includes(q) || (m.words || '').toLowerCase().includes(q);
  list.replaceChildren();
  let shown = 0;
  for (const m of [...members.filter(m => m.outside), ...members.filter(m => !m.outside)].filter(wanted)) {
    const chip = !isNew && !currentEmails.has(m.email) ? el('span', 'chip joins', 'Joins') : null;
    const row = compactRow(m, reasonWords(m, rules), chip, m.outside ? () => {
      draft.additions = draft.additions.filter(a => a.email !== m.email);
      rulesChanged(ed);
    } : null, m.outside ? null : (pickedRow, picked) => pickMember(ed, pickedRow, picked));
    list.append(row);
    shown++;
  }
  for (const m of leaving.filter(wanted)) {
    const row = compactRow(m, 'No rule picks them out any more.', el('span', 'chip leaves', 'Leaves'));
    row.classList.add('is-leaving');
    list.append(row);
    shown++;
  }
  search.hidden = !members.length && !leaving.length;
  empty.hidden = shown > 0 || (!members.length && !leaving.length);
}

async function refreshPreview(ed) {
  const {draft} = ed;
  const {status, head} = ed.preview;
  if (ed.previewing) {
    ed.previewAgain = true;
    return;
  }
  ed.previewing = true;
  status.classList.remove('error');
  status.textContent = 'Working out the members…';
  try {
    const rules = draft.rules.filter(ruleSaysSomething);
    const preview = await api('POST', '/api/loop/preview', {id: draft.id, rules, additions: draft.additions, excluded: draft.excluded});
    const members = preview.members.map(m => memberView(m, m.person ? state.dir.get(m.person) : null));
    const counts = preview.ruleCounts;
    head.textContent = `${members.length} ${members.length === 1 ? 'member' : 'members'}`;
    renderChanges(ed, members, rules);
    showRuleCounts(ed, rules, counts || []);
    status.textContent = rules.length ? '' : 'Add a rule to pick people out.';
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

function rulesCard(ed) {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Rules'));
  card.append(el('div', 'hint', 'Someone is on the email list if any include rule matches them and no exclude rule does; a rule matches only if every choice in it holds.'));
  renderRules(ed);
  const addRules = el('div', 'add-row');
  for (const kind of ['include', 'exclude']) {
    addRules.append(button(kind === 'include' ? 'Add include rule' : 'Add exclude rule', kind === 'include' ? 'plus' : 'minus', 'button button-secondary', () => {
      ed.opened = newRule(kind);
      ed.draft.rules.push(ed.opened);
      renderRules(ed);
    }));
  }
  card.append(ed.ruleRows, addRules);
  return card;
}

function countChip(ed, rule) {
  const chip = el('span', 'rule-count');
  const n = ed.ruleCounts.get(rule);
  chip.hidden = n === undefined;
  if (n !== undefined) {
    chip.textContent = rule.kind === 'include' ? `${n} match` : `${n} excluded`;
  }
  return chip;
}

function renderRules(ed) {
  const {draft, ruleRows} = ed;
  ruleRows.replaceChildren();
  if (!draft.rules.length) {
    ruleRows.append(el('div', 'rule-empty', 'No rules yet: the email list has nobody on it.'));
  }
  for (const kind of ['include', 'exclude']) {
    for (const rule of draft.rules.filter(r => r.kind === kind)) {
      ruleRows.append(ruleRow(rule, () => rulesChanged(ed), () => {
        draft.rules = draft.rules.filter(r => r !== rule);
        renderRules(ed);
        rulesChanged(ed);
      }, rule === ed.opened, r => countChip(ed, r)));
    }
  }
  ed.opened = null;
}

function showRuleCounts(ed, rules, counts) {
  ed.ruleCounts.clear();
  rules.forEach((rule, i) => ed.ruleCounts.set(rule, counts[i]));
  for (const row of ed.ruleRows.children) {
    if (row.refreshCount) {
      row.refreshCount();
    }
  }
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
  const del = el('button', 'danger-button', 'Delete email list');
  del.type = 'button';
  del.addEventListener('click', () => deleteGroup(ed, status));
  actions.append(button('Cancel', null, 'button button-secondary', () => {
    if (closeModal) {
      closeModal();
    } else {
      navigate(withTab(groupPath(g)));
    }
  }), ed.preview.summary, status, del);
  return actions;
}

const editable = ['aliases', 'title', 'description', 'prefix', 'visibility', 'posting', 'replying', 'rules', 'additions', 'excluded'];

function changed(ed) {
  const {draft, original} = ed;
  const now = {...draft, rules: draft.rules.filter(ruleSaysSomething)};
  const out = {};
  for (const key of editable) {
    if (JSON.stringify(now[key]) !== JSON.stringify(original[key])) {
      out[key] = now[key];
    }
  }
  return out;
}

async function saveGroup(ed, save, status) {
  const {g, draft, isNew, closeModal} = ed;
  status.classList.remove('error');
  status.textContent = 'Saving…';
  save.disabled = true;
  try {
    if (isNew) {
      await create('email-lists', {name: draft.name, aliases: draft.aliases, title: draft.title, description: draft.description, prefix: draft.prefix, visibility: draft.visibility, posting: draft.posting, replying: draft.replying, managers: draft.managers, rules: draft.rules.filter(ruleSaysSomething), additions: draft.additions, excluded: draft.excluded});
    } else {
      await act('email-lists', g.id, 'edit', changed(ed));
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

async function deleteGroup(ed, status) {
  const {g, closeModal} = ed;
  if (!confirm(`Delete ${g.address}? Mail sent to it will bounce.`)) {
    return;
  }
  try {
    await remove('email-lists', g.id);
    if (closeModal) {
      closeModal();
    }
    await load();
    toast('Email list deleted');
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
  const canEdit = g.can.edit;
  if (editing && canEdit) {
    page.append(pageHead(g.title));
    page.append(editor(g, false));
    return page;
  }
  const actions = [];
  if (canEdit) {
    actions.push(button('Edit', 'edit', 'button', () => editModal(g)));
  }
  if (g.can.unsubscribe || g.can.resubscribe) {
    const toggle = button(g.can.resubscribe ? 'Resubscribe' : 'Unsubscribe', null, 'button button-secondary', async () => {
      toggle.disabled = true;
      try {
        await act('email-lists', g.id, g.can.resubscribe ? 'resubscribe' : 'unsubscribe');
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
      await act('email-lists', g.id, g.archived ? 'unarchive' : 'archive');
      await load();
      toast(g.archived ? 'Back among your email lists' : 'Archived');
    } catch (err) {
      archive.disabled = false;
      toast(err.message);
    }
  });
  archive.title = g.archived
    ? 'Put the email list back in your normal view. It has been active all along.'
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
    overview.append(el('div', 'subject-note', 'You are on this email list\'s excluded list and get no mail from it. Resubscribe to get its mail again.'));
  }
  if (g.description) {
    overview.append(el('p', 'page-lead', g.description));
  }
  page.append(overview);

  const members = el('div');
  const grid = el('div', 'person-cards');
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
      rules.append(el('div', 'subject-note', `${g.excluded.length} ${g.excluded.length === 1 ? 'address is' : 'addresses are'} kept off the email list whatever the rules say.`));
    }
    members.append(rules);
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
  page.append(tabbed(tabs));
  return page;
}

function compactRow(m, why, chip, onRemove, onPick) {
  const line = el('div', 'person-row-line');
  if (m.outside) {
    line.append(el('span', 'chip outside', 'Non-Helios'), ' ');
  }
  line.append([m.outside ? '' : m.words, m.email].filter(Boolean).join(' · '));
  const tail = el('span', 'compact-tail');
  if (chip) {
    tail.append(chip);
  }
  if (onRemove) {
    tail.append(iconButton('close', `Remove ${m.name}`, 'compact-remove', onRemove));
  }
  const row = personRow(m, {
    className: 'compact-row' + (m.outside ? ' is-outside' : '') + (onPick ? ' is-pickable' : ''),
    lines: [line],
    after: [el('span', 'compact-why', why), tail],
    gradeColors: state.model.gradeColors,
  });
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
  }
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
      await act('email-lists', g.id, 'edit', {managers});
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
    const subscribed = g.members.some(x => x.email === m.email);
    const reach = el('div', 'manager-state' + (subscribed ? ' is-subscribed' : ''));
    reach.append(svg(subscribed ? 'check' : 'close'), el('span', '', subscribed ? 'Subscribed' : 'Not subscribed'));
    reach.title = subscribed ? 'The rules place them on the email list, so its mail reaches them.' : 'No rule places them on the email list, so its mail does not reach them.';
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
    const picker = createPersonPicker(mount, {people: () => state.people.filter(p => !g.managers.some(m => m.email === p.email))});
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
  return personCard(m, {
    onClick: () => openPersonCard(m),
    title: title || reasonWords(m, rules),
    line: m.outside ? 'Guest' : m.context,
    gradeColors: state.model.gradeColors,
  });
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
      const read = await query(`/api/email-lists/${encodeURIComponent(g.id)}?include=messages.copies.person,messages.sender`);
      const messages = read.follow(read.get(read.result), 'messages').map(m => messageView(read, m));
      status.textContent = messages.length ? '' : 'Nothing has been sent to the email list yet.';
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

function messageView(read, m) {
  const sender = read.follow(m, 'sender');
  const copies = read.follow(m, 'copies').map(c => {
    const p = read.follow(c, 'person');
    return {...c, email: p ? p.email : c.email, name: p ? p.fullName : c.email};
  });
  return {...m, from: sender ? personView(sender) : {email: m.fromEmail, name: m.fromName}, copies};
}

function messageRow(m) {
  const row = el('div', 'message-row');
  const known = person(m.from.email);
  const toggle = el('button', 'delivery-toggle');
  toggle.type = 'button';
  for (const [key, n] of [['delivered', m.delivered], ['failed', m.failed], ['pending', m.pending]]) {
    toggle.append(el('span', `delivery-count is-${key}${n ? '' : ' is-zero'}`, `${n} ${key}`));
  }
  toggle.append(svg('chevron-down'));
  row.append(personRow(known ? {name: known.fullName, email: known.email, photoUrl: known.heroPhotoUrl} : m.from, {
    name: m.subject || '(no subject)',
    open: Boolean(known),
    lines: [[m.from.name, stamp(m.received)].filter(Boolean).join(' · ')],
    after: [toggle],
  }));
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
  const known = person(c.email);
  const words = [stateWords[c.state], stamp(c.when)].filter(Boolean).join(' · ');
  const row = personRow({email: c.email, name: c.name || c.email, photoUrl: known ? known.heroPhotoUrl : ''}, {
    className: 'copy-row',
    open: Boolean(known),
    lines: [[words, c.email].filter(Boolean).join(' · ')],
  });
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
  row.querySelector('.person-row-words').append(list);
  return row;
}
