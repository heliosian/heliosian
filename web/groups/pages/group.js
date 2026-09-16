import {state, me, isAdmin, options, groupPath} from '../state.js';
import {el, svg, link, button, iconButton, copyText, toast, personRow, pageHead} from '../dom.js';
import {setTitle} from '../chrome.js';
import {load, navigate} from '../app.js';
import {createPersonPicker} from '/picker.js';

// statusLine says where a group stands with Google: synced and when, not
// yet, or what went wrong.
export function statusLine(status) {
  const line = el('div', 'status-line');
  if (status.error) {
    line.classList.add('is-error');
    line.append(svg('warn'), el('span', '', 'Google sync failed: ' + status.error));
  } else if (status.synced) {
    line.append(svg('check'), el('span', '', 'Synced with Google ' + new Date(status.synced).toLocaleString()));
  } else {
    line.append(svg('sync'), el('span', '', 'Not synced with Google yet'));
  }
  return line;
}

// pendingSync is the group a save just changed, with the standing it had
// before, so its page can watch for the sync that follows.
let pendingSync = null;

function sameStanding(a, b) {
  return (a.synced || '') === (b.synced || '') && (a.error || '') === (b.error || '');
}

// watchSync asks after the group's standing every couple of seconds, for a
// minute, until it moves on from `before`, and redraws the line in place.
// It stops on its own once the line has left the page.
function watchSync(g, line, before) {
  let tries = 0;
  const tick = async () => {
    if (!line.isConnected || tries++ >= 30) {
      return;
    }
    try {
      const res = await fetch(`/api/groups/status?name=${encodeURIComponent(g.name)}`);
      if (!res.ok) {
        return;
      }
      const status = await res.json();
      if (!sameStanding(status, before)) {
        g.status = status;
        line.replaceWith(statusLine(status));
        return;
      }
    } catch {
      return;
    }
    setTimeout(tick, 2000);
  };
  setTimeout(tick, 2000);
}

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
// words, the managers, the include and exclude rules, and a preview of who
// the rules pick out, asked of the server as the rules change.
function editor(g, isNew) {
  const draft = {
    name: g.name, title: g.title, description: g.description || '',
    managers: g.managers.map(m => m.email),
    rules: g.rules.map(r => ({kind: r.kind, roles: [...r.roles], search: r.search, classrooms: [...r.classrooms], grades: [...r.grades], tags: [...r.tags], family: [...r.family], owner: r.owner, tagLabels: r.tagLabels})),
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
  form.append(words);

  const managers = el('div', 'card');
  managers.append(el('h2', '', 'Managers'));
  managers.append(el('div', 'hint', 'Whoever is listed can change the rules and the managers, or delete the group. You are one on a group you make.'));
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
  form.append(managers);

  const preview = el('div', 'card preview');
  const previewHead = el('h2', '', 'Members');
  const previewNote = el('div', 'hint', 'Who the rules pick out right now. The list follows the directory as it changes.');
  const previewList = el('div', 'member-list');
  const previewStatus = el('div', 'save-status');
  preview.append(previewHead, previewNote, previewStatus, previewList);

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
      const {members} = await send('POST', '/api/groups/preview', {name: isNew ? '' : draft.name, rules});
      previewHead.textContent = `${members.length} ${members.length === 1 ? 'member' : 'members'}`;
      previewList.replaceChildren();
      for (const m of members) {
        previewList.append(personRow(m));
      }
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
    card.append(el('h2', '', heading), el('div', 'hint', blurb));
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
  form.append(ruleSection('include', 'Include', 'Everyone matching any of these rules is in the group. Within a rule every choice must hold: a parent of a Grade 3 student, say, is Parents and Grade 3 together. Add family widens a rule by the relatives of the people it matches.'));
  form.append(ruleSection('exclude', 'Exclude', 'Anyone matching any of these rules is left out, whatever the include rules say.'));
  form.append(preview);

  const actions = el('div', 'editor-actions');
  const status = el('span', 'save-status');
  const save = button(isNew ? 'Make the group' : 'Save', 'check', 'button', async () => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    save.disabled = true;
    try {
      const body = {original: isNew ? '' : draft.name, name: draft.name, title: draft.title, description: draft.description, managers: draft.managers, rules: draft.rules.filter(ruleSaysSomething)};
      const saved = await send('POST', '/api/groups/group', body);
      pendingSync = {name: saved.name, before: g.status || {}};
      await load();
      toast(isNew ? 'Group made' : 'Saved');
      navigate(groupPath(saved));
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
      save.disabled = false;
    }
  });
  actions.append(save);
  if (!isNew) {
    actions.append(button('Cancel', null, 'button button-secondary', () => navigate(groupPath(g))));
    const del = el('button', 'danger-button', 'Delete group');
    del.type = 'button';
    del.addEventListener('click', async () => {
      if (!confirm(`Delete ${g.address}? Its Google group goes with it.`)) {
        return;
      }
      try {
        await send('DELETE', '/api/groups/group', {name: g.name});
        await load();
        toast('Group deleted');
        navigate('/');
      } catch (err) {
        status.classList.add('error');
        status.textContent = err.message;
      }
    });
    actions.append(del);
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

function slug(text) {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40);
}

export function newGroupPage() {
  setTitle('New Group');
  const page = el('div', 'group-page');
  page.append(pageHead('New Group'));
  page.append(editor({name: '', title: '', description: '', managers: [{email: me().email, name: me().name}], rules: []}, true));
  return page;
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
    parts.push('tagged ' + r.tagLabels.join(' or '));
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
    actions.push(button('Edit', 'edit', 'button', () => navigate(groupPath(g) + '?edit=1')));
  }
  page.append(pageHead(g.title, actions));
  const address = el('div', 'address-band');
  const mail = el('a', 'address-mail');
  mail.href = 'mailto:' + g.address;
  mail.append(svg('mail'), el('span', '', g.address));
  address.append(mail, iconButton('copy', 'Copy the address', '', () => copyText(g.address, 'Address copied')));
  page.append(address);
  if (g.description) {
    page.append(el('p', 'page-lead', g.description));
  }
  const line = statusLine(g.status);
  page.append(line);
  // The sync follows a change within seconds; a page arriving after one,
  // or showing a failure a retry may clear, watches for it.
  if (pendingSync && pendingSync.name === g.name) {
    const before = pendingSync.before;
    pendingSync = null;
    if (sameStanding(g.status, before)) {
      watchSync(g, line, before);
    }
  } else if (g.status.error || !g.status.synced) {
    watchSync(g, line, g.status);
  }

  const rules = el('div', 'card');
  rules.append(el('h2', '', 'Rules'));
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
  page.append(rules);

  const managers = el('div', 'card');
  managers.append(el('h2', '', 'Managers'));
  for (const m of g.managers) {
    managers.append(personRow(m));
  }
  page.append(managers);

  const members = el('div', 'card');
  members.append(el('h2', '', `${g.members.length} ${g.members.length === 1 ? 'member' : 'members'}`));
  members.append(el('div', 'hint', 'Who the rules pick out right now. Google is kept in step as the directory changes.'));
  const list = el('div', 'member-list');
  for (const m of g.members) {
    list.append(personRow(m));
  }
  if (!g.members.length) {
    list.append(el('div', 'rule-empty', 'Nobody matches the rules yet.'));
  }
  members.append(list);
  page.append(members);
  page.append(link('/', 'back-link', '← All groups'));
  return page;
}
