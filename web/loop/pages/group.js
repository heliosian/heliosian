import {state, me, isAdmin, options, groupPath} from '../state.js';
import {el, svg, link, button, iconButton, copyText, toast, personRow, pageHead, thumb, whoLink} from '../dom.js';
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

// ruleRow is one rule in the editor. Read, it is the sentence it makes -
// "Parents in Grade 4" - with a pencil and the way to drop it; opened, its
// facets as controls with Done to close it, which a new rule starts as. A
// rule another manager wrote is read and never opened: its tags are theirs.
function ruleRow(rule, onChange, onRemove, open, countChip) {
  const row = el('div', 'rule');
  const mine = rule.owner === me().email;
  // Read: a tinted band with the kind's label and mark, the sentence, and
  // how many people the rule touches once the preview has said - asked
  // afresh each time the band is drawn, and again when the preview answers.
  let count = null;
  row.refreshCount = () => {
    if (count && countChip) {
      const next = countChip(rule);
      count.replaceWith(next);
      count = next;
    }
  };
  const showWords = () => {
    row.classList.remove('is-open');
    row.classList.add('is-line', 'is-' + rule.kind);
    row.replaceChildren();
    const words = el('div', 'rule-controls');
    const line = el('div', 'rule-line');
    line.append(el('span', 'rule-kind', rule.kind === 'include' ? 'Include' : 'Exclude'));
    line.append(svg(rule.kind === 'include' ? 'groups' : 'user-minus'));
    line.append(el('span', 'rule-line-words', ruleWords(rule)));
    if (countChip) {
      count = countChip(rule);
      line.append(count);
    }
    words.append(line);
    if (!mine) {
      row.classList.add('is-theirs');
      const owner = state.model.people.find(p => p.email === rule.owner);
      words.append(el('div', 'rule-note', `Written by ${owner ? owner.name : rule.owner}, reading their tags; it can be removed but not changed.`));
    }
    row.append(words);
    if (mine) {
      row.append(iconButton('edit', 'Change this rule', 'rule-remove', showControls));
    }
    row.append(iconButton('trash', 'Remove this rule', 'rule-remove', onRemove));
  };
  const showControls = () => {
    row.classList.add('is-open', 'is-' + rule.kind);
    row.classList.remove('is-line');
    count = null;
    row.replaceChildren();
    const controls = ruleControls(rule, onChange);
    row.append(controls);
    // Done closes the rule to its sentence; a rule that still says nothing
    // is dropped instead of kept empty.
    const done = button('Done', 'check', 'button button-small', () => {
      if (!ruleSaysSomething(rule)) {
        onRemove();
        return;
      }
      showWords();
    });
    controls.querySelector('.rule-said').after(done);
    row.append(iconButton('trash', 'Remove this rule', 'rule-remove', onRemove));
    const first = controls.querySelector('.chip-toggle');
    if (first) {
      first.focus();
    }
  };
  if (open && mine) {
    showControls();
  } else {
    showWords();
  }
  return row;
}

// ruleControls is a rule's facets as controls, with the rule read out under
// them as it stands, so a choice reads back as the sentence it makes.
function ruleControls(rule, onChange) {
  const controls = el('div', 'rule-controls');
  const said = el('div', 'rule-said');
  const sayIt = () => {
    const words = ruleSaysSomething(rule) ? ruleWords(rule) : '';
    said.textContent = words ? (rule.kind === 'exclude' ? 'Leaves out: ' : 'Includes: ') + words : 'Pick a role, some words, a classroom, a grade or a tag.';
    said.classList.toggle('is-empty', !words);
  };
  const changed = () => {
    sayIt();
    onChange();
  };
  const roles = el('div', 'chip-row');
  for (const role of options().roles) {
    roles.append(chipToggle(role + 's', rule.roles.includes(role), on => {
      rule.roles = on ? [...rule.roles, role] : rule.roles.filter(r => r !== role);
      changed();
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
    timer = setTimeout(changed, 300);
  });
  controls.append(search);
  const classrooms = new Set(rule.classrooms);
  controls.append(facetDropdown('Classroom', null, options().classrooms, classrooms, () => {
    rule.classrooms = [...classrooms];
    changed();
  }));
  const grades = new Set(rule.grades);
  controls.append(facetDropdown('Grade', null, options().grades, grades, () => {
    rule.grades = [...grades];
    changed();
  }));
  const tags = new Set(rule.tags);
  const tagValues = [
    ...options().tags,
    ...options().shared.map(s => ({value: s.key, label: `${s.name} (${s.ownerName}'s)`, icon: 'tag'})),
    ...options().lists.map(l => ({value: l.key, label: l.name, icon: listIcons[l.kind]})),
  ];
  controls.append(facetDropdown('Tags', 'tag', tagValues, tags, () => {
    rule.tags = [...tags];
    rule.tagLabels = rule.tags.map(t => (tagValues.find(v => (typeof v === 'string' ? v : v.value) === t) || {label: t}).label || t);
    changed();
  }));
  const family = new Set(rule.family);
  controls.append(facetDropdown('Add family', 'families', options().relations.map(r => ({value: r, label: 'Their ' + r.toLowerCase()})), family, () => {
    rule.family = [...family];
    changed();
  }));
  controls.append(said);
  sayIt();
  return controls;
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
// editor is the group form. Opened in the modal from a group's page
// (Edit), `closeModal` is how it leaves - after saving, deleting or
// cancelling - and its tabs keep to themselves rather than the address.
// startTab is the tab the modal opens on - Rules from the rules card's
// pencil, the first otherwise.
function editor(g, isNew, closeModal, startTab) {
  const draft = {
    name: g.name, aliases: [...(g.aliases || [])], title: g.title, description: g.description || '',
    prefix: isNew ? true : g.prefix,
    visibility: isNew ? 'hidden' : g.visibility,
    managers: g.managers.map(m => m.email),
    rules: g.rules.map(r => ({kind: r.kind, roles: [...r.roles], search: r.search, classrooms: [...r.classrooms], grades: [...r.grades], tags: [...r.tags], family: [...r.family], owner: r.owner, tagLabels: r.tagLabels})),
    additions: (g.additions || []).map(a => ({email: a.email, name: a.name})),
    excluded: (g.excluded || []).map(e => ({email: e.email, note: e.note || '', when: e.when || ''})),
  };
  // A blank new group starts with one include rule, open to be filled in.
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
  // In the modal the title is the window's own heading, typed in place;
  // on the page it is the form's first field.
  if (closeModal) {
    title.className = 'modal-title-input';
    title.setAttribute('aria-label', 'Group name');
    // A new group's heading wears a dotted ring until a name is typed, so
    // the eye lands on the one thing the window needs first.
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
  // The aliases - other local parts that reach the group, each unique
  // across every group's name and alias - sit under the address: a small
  // Add alias at the label's right opens a box for one, and each alias is
  // a row with its Remove.
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
  // The box's clear: empties it and puts it away, as Escape does.
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
  // Generate with AI writes the description from the draft: its name, its
  // rules in words and a tally of who they pick out, asked of the server,
  // which asks Claude; the words land in the box to change or keep.
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
  const overviewPanel = el('div');
  overviewPanel.append(words);

  // A new group's managers are picked here; an existing group's are
  // changed in the Managers box on its page.
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
  preview.append(previewHeadRow, previewStatus, previewChanges, previewList);
  const summary = el('span', 'change-summary');
  const current = isNew ? [] : g.members;
  const currentEmails = new Set(current.map(m => m.email));

  // pickMember is a click on a directory member in the list: a small menu
  // by the row offering to exclude them - which adds an exclude rule naming
  // their address, read out as their name - or to open them in Who?.
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

  // renderChanges says what a save does to the membership: the people the
  // draft adds and the people it drops, by name, and marks each in the
  // list - a joiner with a chip, a leaver greyed at the end.
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
    // Everyone the draft picks out, a compact row each - the face, the
    // name, the address, the word that places them and why they are on -
    // a joiner with a Joins chip, a leaver greyed at the end with Leaves.
    // The Non-Helios people first, where their rows stand out, then the
    // directory's.
    previewList.replaceChildren();
    for (const m of [...members.filter(m => m.outside), ...members.filter(m => !m.outside)]) {
      const chip = !isNew && !currentEmails.has(m.email) ? el('span', 'chip joins', 'Joins') : null;
      const row = compactRow(m, reasonWords(m, rules), chip, m.outside ? () => {
        draft.additions = draft.additions.filter(a => a.email !== m.email);
        rulesChanged();
      } : null, m.outside ? null : pickMember);
      previewList.append(row);
    }
    for (const m of leaving) {
      const row = compactRow(m, 'No rule picks them out any more.', el('span', 'chip leaves', 'Leaves'));
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

  // The include and exclude rules each in a section of their own, under
  // the green plus or the red minus the page reads them out with.
  // The rules in one card, each behind its sign - the includes then the
  // excludes - with a button to add either kind under them.
  const rulesCard = el('div', 'card');
  rulesCard.append(el('h2', '', 'Rules'));
  rulesCard.append(el('div', 'hint', 'Someone is on the group if any include rule matches them and no exclude rule does; a rule matches only if every choice in it holds.'));
  const rows = el('div', 'rules');
  let opened = firstRule;
  // ruleCounts is what the last preview said each rule touches, by the
  // rule; the chips on the rows read from it and refill as it changes.
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

  // The additions: people the directory does not hold, each a name and an
  // address typed into the box Add Non-Helios opens at the members' head,
  // on the group whatever the rules say until removed from the list.
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

  // Add Helios puts one person from the directory on the group: an include
  // rule naming their address, which reads out as their name.
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

  // Members is the rules, the additions and, at the foot, who they come to.
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
      const body = {original: isNew ? '' : draft.name, name: draft.name, aliases: draft.aliases, title: draft.title, description: draft.description, prefix: draft.prefix, visibility: draft.visibility, managers: draft.managers, rules: draft.rules.filter(ruleSaysSomething), additions: draft.additions, excluded: draft.excluded};
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
    const danger = el('div', 'editor-danger');
    danger.append(del);
    overviewPanel.append(danger);
  } else {
    actions.append(button('Cancel', null, 'button button-secondary', () => {
      if (closeModal) {
        closeModal();
      }
      navigate('/');
    }));
  }
  actions.append(status);
  form.append(actions);
  refreshPreview();
  return form;
}

// editModal opens the editor over the group's page, in a window of the
// page's own: a box with the title and a close, the editor inside with
// its Save bar stuck to the box's foot. Only the close, Cancel, Escape
// and a save or delete shut it, so a stray click cannot lose an edit.
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

function ruleSaysSomething(r) {
  return r.roles.length || r.search || r.classrooms.length || r.grades.length || r.tags.length;
}

function slug(text) {
  return text.toLowerCase().replace(/[^a-z0-9.]+/g, '-').replace(/\.{2,}/g, '.').replace(/^[-.]+|[-.]+$/g, '').slice(0, 40);
}

// newGroupModal opens the editor over the page for a group that does not
// exist yet - the same window as Edit - blank, or filled in from a
// suggestion (`?from=<Magic Tag key>`): everyone on the Magic Tag,
// whatever their role, and the parents of any student on it.
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
  // A rule that is one address and nothing else names them.
  if (r.search && r.search.includes('@') && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length && state.model.people.some(p => p.email === r.search)) {
    return 'named in a rule';
  }
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
  // A rule that is one address and nothing else - as excluding someone
  // from the member list makes - reads as the person.
  if (r.search && r.search.includes('@') && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length) {
    const person = state.model.people.find(p => p.email === r.search);
    if (person) {
      return `${person.name} (${r.search})`;
    }
  }
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
  // Edit opens the editor in a window over the page, so the page stays
  // where it is and reloads when the editor saves.
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
  // Archive is the viewer's own tidy: the group goes under Archived in the
  // rail and its Magic Tag off Who?'s lists for them, and nothing about
  // the group itself changes.
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

  // What the group is, above the tabs: the address, the notes that apply,
  // the description.
  const overview = el('div', 'group-overview');
  const address = el('div', 'address-band');
  const mail = el('a', 'address-mail');
  mail.href = 'mailto:' + g.address;
  mail.append(svg('mail'), el('span', '', g.address));
  address.append(mail, iconButton('copy', 'Copy the address', '', () => copyText(g.address, 'Address copied')));
  // The aliases sit behind a quiet count at the band's right; resting on
  // it, or focusing it, shows the addresses.
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

  // Members: the people as Who? shows them, a card each, narrowed by the
  // box above; then who manages the group, how its members are chosen,
  // and the same people in Who? itself.
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
      grid.append(memberCard(m, g.rules));
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

  members.append(managersCard(g, canEdit));

  const rules = el('div', 'card');
  const rulesHead = el('div', 'card-head');
  rulesHead.append(el('h2', '', 'How the members are chosen'));
  if (canEdit) {
    rulesHead.append(iconButton('edit', 'Edit the rules', '', () => editModal(g, 'members')));
  }
  rules.append(rulesHead);
  // Each rule a line with its sign: a green plus for who is included, a
  // red minus for who is taken out, the includes first.
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

  if (canEdit) {
    const elsewhere = el('div', 'elsewhere');
    const who = el('a', 'elsewhere-link');
    who.href = appOrigin('who') + '/people?list=' + encodeURIComponent('group:' + g.name);
    const mark = el('img');
    mark.src = '/brand/apps/who.png';
    mark.alt = '';
    who.append(mark, el('span', '', 'See these people in Helios Who?'));
    elsewhere.append(who);
    members.append(elsewhere);
  }

  const tabs = [
    {key: 'members', label: 'Members', count: g.members.length, panel: members},
  ];
  if (canEdit) {
    tabs.push(historyTab(g));
  }
  page.append(tabbed(tabs));
  page.append(link('/', 'back-link', '← All groups'));
  return page;
}

// compactRow is one person in the editor's member list: a small face, the
// name, the address, the word that places them, why they are on the group,
// and a chip when saving changes them.
// Someone from outside the directory is marked plainly - a tinted row and
// a Non-Helios chip - with a remove when the list is the editor's.
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

// managersEditing is the group whose Managers box is open for changes,
// kept across the reload each change brings.
let managersEditing = '';

// managersCard is the managers as the members are drawn, smaller. Its
// pencil, for a manager or an admin, opens it for changes: a remove on
// each tile but the last, and a picker to add one, each change saved at
// once - the managers are the group's, not the editor's, so they are
// changed here rather than in the editor.
function managersCard(g, canEdit) {
  const card = el('div', 'card');
  const head = el('div', 'card-head');
  head.append(el('h2', '', 'Managers'));
  const editing = canEdit && managersEditing === g.name;
  if (canEdit) {
    head.append(iconButton(editing ? 'close' : 'edit', editing ? 'Done' : 'Change the managers', editing ? 'is-on' : '', () => {
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
      await send('POST', '/api/loop/group', {original: g.name, name: g.name, aliases: g.aliases, title: g.title, description: g.description, prefix: g.prefix, visibility: g.visibility, managers, rules: g.rules, additions: g.additions, excluded: g.excluded});
      await load();
      toast('Managers saved');
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
    }
  };
  const grid = el('div', 'attendee-grid attendee-grid-small');
  for (const m of g.managers) {
    const tile = memberCard(m, g.rules, 'Manages the group');
    if (editing) {
      const wrap = el('div', 'attendee-wrap');
      const remove = iconButton('close', `Remove ${m.name}`, 'attendee-remove', () => save(g.managers.map(x => x.email).filter(e => e !== m.email)));
      remove.disabled = g.managers.length === 1;
      wrap.append(tile, remove);
      grid.append(wrap);
    } else {
      grid.append(tile);
    }
  }
  card.append(grid);
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

// memberCard is one member as Celebrate's Who's Coming grid draws a face:
// a square photo, or the first letter of the name, the name under it and a
// line placing them - a student's grade, a staff member's job, a parent's
// children, Guest for someone from outside the directory. The tile opens
// their page in Who?, and its tooltip says why they are on the group.
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
  // A student's grade rides on the corner of their face, short - "5",
  // "K" - in Who?'s colour for the grade, darkened as HCA-Team darkens it
  // so the white figure reads even on a yellow.
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

// tabbed is a strip over its panels. With sync the chosen tab is kept in
// the address's tab parameter; without, as in the editor's modal, it is
// the strip's own, so the page's tab under it is left alone.
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
