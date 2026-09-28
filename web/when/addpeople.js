import {el, svg, button, toast, longToast} from '/elements.js';
import {popup} from '/modal.js';
import {text as textInput} from '/form.js';
import {api} from '/api.js';
import {directory, contactLine} from '/directory.js';
import {chipToggle, familyDropdown} from '/rules.js';
import {firstName, face, fetchPickerData, ruleOptions, setRuleOptions, rules} from './inviteparts.js';

export async function openPicker(e, view, refresh) {
  let data = null;
  let dir = null;
  try {
    const [found, people, options] = await Promise.all([fetchPickerData(e), directory(), view.host ? api('GET', '/api/when/invites/options') : ruleOptions]);
    data = found;
    dir = people;
    setRuleOptions(options);
  } catch (err) {
    toast(err.message);
    return;
  }
  const onList = new Set(data.onList || []);
  const box = el('div', 'picker');
  const tabs = el('div', 'tabs');
  const panel = el('div', 'picker-panel');
  const kinds = view.host ? [['person', 'Add Person'], ['group', 'Add Group'], ['outside', 'Add Non-Helios']] : [['person', 'Add Person'], ['outside', 'Add Non-Helios']];
  let active = 'person';
  for (const [key, label] of kinds) {
    const b = el('button', 'tab-button' + (key === active ? ' is-active' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      active = key;
      for (const t of tabs.children) {
        t.classList.toggle('is-active', t === b);
      }
      paintPanel();
    });
    tabs.append(b);
  }
  let shut = null;
  const done = async words => {
    longToast(words);
    shut();
    refresh();
  };
  const paintPanel = () => {
    panel.replaceChildren();
    switch (active) {
      case 'person':
        panel.append(personPanel(e, dir, onList, done));
        break;
      case 'group':
        panel.append(groupPanel(e, done));
        break;
      case 'outside':
        panel.append(outsidePanel(e, onList, done));
        break;
    }
  };
  box.append(tabs, panel);
  paintPanel();
  shut = popup('Add to the guest list', box, {wide: true}).shut;
}

const roleTests = {student: p => p.isStudent, parent: p => p.isParent, staff: p => p.isStaff};

function personPanel(e, dir, onList, done) {
  const pick = {
    dir,
    people: dir.data.map(dir.get),
    onList,
    picked: new Map(),
    family: new Set(),
    roles: new Set(),
    search: el('input', 'picker-search'),
    list: el('div', 'picker-results'),
    add: el('button', 'button'),
  };
  pick.search.type = 'search';
  pick.search.placeholder = 'Search by name or email';
  pick.search.addEventListener('input', () => paintPersonList(pick));
  pick.add.type = 'button';
  const status = el('span', 'save-status');
  pick.add.addEventListener('click', () => addPicked(e, pick, status, done));
  const foot = el('div', 'modal-actions');
  foot.append(pick.add, status);
  const wrap = el('div');
  wrap.append(pick.search, personFilters(pick), pick.list, foot);
  paintPersonList(pick);
  paintPickButton(pick);
  setTimeout(() => pick.search.focus(), 0);
  return wrap;
}

function personFilters(pick) {
  const bar = el('div', 'picker-bar');
  const chips = el('div', 'chip-row picker-roles');
  for (const [role, label] of [['student', 'Students'], ['parent', 'Parents'], ['staff', 'Staff']]) {
    chips.append(chipToggle(label, false, on => {
      if (on) {
        pick.roles.add(role);
      } else {
        pick.roles.delete(role);
      }
      paintPersonList(pick);
    }, role));
  }
  bar.append(chips, familyDropdown(['Parents', 'Children', 'Siblings'], pick.family, () => {
    paintPersonList(pick);
    paintPickButton(pick);
  }));
  return bar;
}

function relativesOf(pick, p) {
  const out = [];
  for (const relation of ['Parents', 'Children', 'Siblings']) {
    if (!pick.family.has(relation)) {
      continue;
    }
    for (const r of pick.dir.follow(p, relation.toLowerCase())) {
      if (r && r.email && !pick.onList.has(r.email) && !out.includes(r)) {
        out.push(r);
      }
    }
  }
  return out;
}

function everyonePicked(pick) {
  const out = new Map();
  for (const p of pick.picked.values()) {
    out.set(p.email, {email: p.email, name: p.fullName, via: 'search'});
    for (const r of relativesOf(pick, p)) {
      if (!out.has(r.email)) {
        out.set(r.email, {email: r.email, name: r.fullName, via: 'family'});
      }
    }
  }
  return out;
}

function paintPickButton(pick) {
  const n = everyonePicked(pick).size;
  pick.add.replaceChildren(svg('plus'), el('span', '', n ? `Add ${n} ${n === 1 ? 'person' : 'people'}` : 'Add to the list'));
  pick.add.disabled = !n;
}

function paintPersonList(pick) {
  const {list} = pick;
  list.replaceChildren();
  const q = pick.search.value.trim().toLowerCase();
  const tests = [...pick.roles].map(role => roleTests[role]);
  const found = pick.people.filter(p => (!q || p.fullName.toLowerCase().includes(q) || p.email.toLowerCase().includes(q)) && (!tests.length || tests.some(t => t(p))));
  if (!found.length) {
    list.append(el('div', 'picker-note', 'Nobody by that name. Someone outside Helios goes on Add Non-Helios.'));
  }
  for (const p of found.slice(0, 200)) {
    list.append(personChoice(pick, p));
  }
  if (found.length > 200) {
    list.append(el('div', 'picker-note', `${found.length - 200} more - type a name to narrow it.`));
  }
}

function personChoice(pick, p) {
  const on = pick.onList.has(p.email);
  const row = el('button', 'picker-person' + (on ? ' is-on' : pick.picked.has(p.email) ? ' is-picked' : ''));
  row.type = 'button';
  row.disabled = on;
  const mark = el('span', 'picker-check');
  mark.append(svg('check'));
  row.append(mark, face({name: p.fullName, email: p.email, photoUrl: p.heroPhotoUrl && p.heroPhotoUrl + '?thumb=1', grade: p.grade}));
  const who = el('div', 'invite-who');
  who.append(el('div', 'invite-name', p.fullName || p.email));
  const along = relativesOf(pick, p).map(r => firstName({name: r.fullName, email: r.email}));
  const line = [on ? 'On the list' : contactLine(pick.dir, p), along.length ? 'with ' + along.join(', ') : ''].filter(Boolean).join(' · ');
  if (line) {
    const words = el('div', 'invite-line', line);
    if (along.length) {
      words.classList.add('has-family');
    }
    who.append(words);
  }
  row.append(who);
  row.addEventListener('click', () => {
    if (pick.picked.has(p.email)) {
      pick.picked.delete(p.email);
    } else {
      pick.picked.set(p.email, p);
    }
    row.classList.toggle('is-picked', pick.picked.has(p.email));
    paintPickButton(pick);
  });
  return row;
}

async function addPicked(e, pick, status, done) {
  pick.add.disabled = true;
  try {
    const made = await api('POST', '/api/when/invites/people', {id: e.id, people: [...everyonePicked(pick).values()]});
    done(made.sent ? `${made.added} invited - the invitation is on its way.` : `${made.added} added to the list.`);
  } catch (err) {
    status.textContent = err.message;
    status.classList.add('error');
    pick.add.disabled = false;
  }
}

function groupPanel(e, done) {
  const wrap = el('div', 'picker-group');
  const rule = rules.newRule('include');
  const holder = el('div', 'rule is-include picker-rule');
  const preview = el('div', 'audience-preview picker-preview');
  let timer = null;
  const askPreview = () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      if (!rules.ruleSaysSomething(rule)) {
        preview.textContent = '';
        return;
      }
      try {
        const answer = await api('POST', '/api/when/invites/preview', {id: e.id, rule});
        preview.textContent = answer.count ? `Picks out ${answer.count} ${answer.count === 1 ? 'person' : 'people'} now: ${answer.names.join(', ')}${answer.count > answer.names.length ? '…' : ''}` : 'Picks out nobody yet.';
      } catch (err) {
        preview.textContent = err.message;
      }
    }, 300);
  };
  holder.append(rules.ruleControls(rule, askPreview));
  wrap.append(holder, preview);
  const auto = el('label', 'picker-auto');
  const box = el('input');
  box.type = 'checkbox';
  box.checked = true;
  auto.append(box, el('span', '', 'Auto-invite new members'), el('small', '', 'Whoever comes to match this later - a family joining the classroom, a ticket sold - goes on the list either way. With this on they are sent their invitation too, once you have sent this group its invitations yourself; off, they wait in Pending for you.'));
  wrap.append(auto);
  const foot = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const add = button('Add group', 'plus', 'button', async () => {
    if (!rules.ruleSaysSomething(rule)) {
      status.textContent = 'Pick a role, some words, a classroom, a grade or a tag.';
      status.classList.add('error');
      return;
    }
    add.disabled = true;
    try {
      const made = await api('POST', '/api/when/invites/group', {id: e.id, rule, auto: box.checked});
      done(`Group added, with ${made.added} ${made.added === 1 ? 'person' : 'people'} on the list now.`);
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      add.disabled = false;
    }
  });
  foot.append(add, status);
  wrap.append(foot);
  return wrap;
}

const emailForm = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

function outsidePanel(e, onList, done) {
  const wrap = el('div');
  wrap.append(el('div', 'picker-note', 'Someone outside Helios - a coach, a grandparent, a friend - and their family. They get the email and the calendar invite with a page of their own to answer from, no sign-in needed, that shows the event and nothing of who else is coming.'));
  const families = el('div', 'outside-families');
  families.append(outsideFamilyCard());
  wrap.append(families, button('Add another family', 'plus', 'link-button', () => {
    const card = outsideFamilyCard();
    families.append(card);
    card.focus();
  }));
  const status = el('span', 'save-status');
  const foot = el('div', 'modal-actions');
  const add = button('Add to the list', 'plus', 'button', () => addOutside(e, onList, families, status, add, done));
  foot.append(add, status);
  wrap.append(foot);
  setTimeout(() => families.firstChild.focus(), 0);
  return wrap;
}

function outsideFamilyCard() {
  const card = el('div', 'outside-family');
  const members = [];
  const name = textInput('', {placeholder: 'Name'});
  const email = textInput('', {type: 'email', placeholder: 'Email address'});
  const head = el('div', 'picker-email');
  head.append(name, email);
  card.append(head, outsideMembers(members));
  card.members = members;
  card.head = () => ({name: name.value.trim(), email: email.value.trim().toLowerCase()});
  card.focus = () => name.focus();
  return card;
}

function outsideMembers(members) {
  const family = el('div', 'outside-members');
  family.append(el('div', 'outside-members-title', 'Family members (optional)'), el('div', 'picker-note', 'A spouse, a partner, children - whoever is coming with them. Anyone with an address gets an invitation of their own.'));
  const list = el('div', 'outside-member-list');
  const adder = outsideMemberAdder(members, list);
  family.append(list, button('Add family member', 'plus', 'button button-secondary button-small', () => {
    adder.node.hidden = false;
    adder.name.focus();
  }), adder.node);
  return family;
}

function paintOutsideMembers(list, members) {
  list.replaceChildren();
  for (const m of members) {
    const row = el('div', 'outside-member');
    const initials = el('span', 'avatar outside-member-face', m.name.split(/\s+/).map(w => w[0] || '').join('').slice(0, 2).toUpperCase());
    row.append(initials, el('span', 'outside-member-name', m.name), el('span', 'outside-member-email', m.email || 'No address - answered for by the family'));
    const remove = el('button', 'guests-action is-remove');
    remove.type = 'button';
    remove.title = 'Remove';
    remove.append(svg('trash'));
    remove.addEventListener('click', () => {
      members.splice(members.indexOf(m), 1);
      paintOutsideMembers(list, members);
    });
    row.append(remove);
    list.append(row);
  }
}

function outsideMemberAdder(members, list) {
  const node = el('div', 'picker-email outside-adder');
  node.hidden = true;
  const name = textInput('', {placeholder: 'Family member’s name'});
  const email = textInput('', {type: 'email', placeholder: 'Their email (optional)'});
  const put = button('Add', 'plus', 'button button-small', () => {
    const n = name.value.trim();
    const a = email.value.trim().toLowerCase();
    if (!n) {
      name.focus();
      return;
    }
    if (a && !emailForm.test(a)) {
      email.focus();
      return;
    }
    members.push({name: n, email: a});
    name.value = '';
    email.value = '';
    paintOutsideMembers(list, members);
    name.focus();
  });
  node.append(name, email, put);
  for (const input of [name, email]) {
    input.addEventListener('keydown', ev => {
      if (ev.key === 'Enter') {
        ev.preventDefault();
        put.click();
      }
    });
  }
  return {node, name};
}

async function addOutside(e, onList, families, status, add, done) {
  const people = [];
  for (const card of families.children) {
    const head = card.head();
    if (!head.name && !head.email && !card.members.length) {
      continue;
    }
    if (!head.name || !emailForm.test(head.email)) {
      status.textContent = 'Each family needs a name and an email address at its head.';
      status.classList.add('error');
      card.focus();
      return;
    }
    if (onList.has(head.email)) {
      status.textContent = `${head.name} is on the list already.`;
      status.classList.add('error');
      return;
    }
    people.push({email: head.email, name: head.name, via: 'outside', household: head.email});
    for (const m of card.members) {
      people.push({email: m.email, name: m.name, via: 'outside', household: head.email});
    }
  }
  if (!people.length) {
    status.textContent = 'A name and an email address, please.';
    status.classList.add('error');
    return;
  }
  add.disabled = true;
  try {
    const made = await api('POST', '/api/when/invites/people', {id: e.id, people});
    done(made.sent ? (made.added === 1 ? `${people[0].name} invited - the invitation is on its way.` : `${made.added} people invited - the invitations are on their way.`) : made.added === 1 ? `${people[0].name} added to the list.` : `${made.added} people added to the list.`);
  } catch (err) {
    status.textContent = err.message;
    status.classList.add('error');
    add.disabled = false;
  }
}
