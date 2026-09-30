import {el, svg, button, iconButton} from '/elements.js';
import {text as textInput} from '/form.js';
import {directory, contactLine} from '/directory.js';
import {chipToggle, familyDropdown} from '/rules.js';
import {personRow} from '/personrow.js';
import {popup} from '/modal.js';

const throughWords = {Parents: 'Parent', Children: 'Child', Siblings: 'Sibling'};

export function reasonWords(member, rules, phrase) {
  return (member.reasons || []).map(reason => {
    if (reason.guestOf) {
      return `Guest of ${reason.guestOf}`;
    }
    if (reason.invitedBy) {
      return `Invited by ${reason.invitedBy}`;
    }
    if (reason.added) {
      return 'Added by hand';
    }
    const rule = rules[reason.rule];
    if (!rule) {
      return '';
    }
    const words = phrase(rule);
    if (reason.through) {
      return `${throughWords[reason.through]} of ${(reason.viaName || '').split(' ')[0]}, ${words}`;
    }
    return words.charAt(0).toUpperCase() + words.slice(1);
  }).filter(Boolean).join(' · ');
}

export const pageSize = 10;

export function fillPager(pager, page, total, onPage) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  pager.replaceChildren();
  pager.hidden = pages === 1;
  if (pages === 1) {
    return;
  }
  const back = button('Previous', 'chevron-left', 'button button-small button-secondary', () => onPage(page - 1));
  back.disabled = page === 0;
  const next = button('Next', 'chevron-right', 'button button-small button-secondary', () => onPage(page + 1));
  next.disabled = page === pages - 1;
  pager.append(back, el('span', 'member-pager-words', `${page * pageSize + 1}–${Math.min((page + 1) * pageSize, total)} of ${total}`), next);
}

export function membersCard({noun, listWord, adders, phrase, chip, onExclude, onRemove, gradeColors}) {
  const head = el('h2', '', `0 ${noun[1]}`);
  const headRow = el('div', 'card-head');
  headRow.append(head, adders.buttons);
  const status = el('div', 'save-status');
  const changes = el('div', 'change-band');
  changes.hidden = true;
  const search = el('input', 'member-search');
  search.type = 'search';
  search.placeholder = `Search the ${noun[1]}…`;
  search.setAttribute('aria-label', `Search the ${noun[1]}`);
  const list = el('div', 'compact-list');
  const empty = el('div', 'rule-empty', 'Nobody matches that.');
  empty.hidden = true;
  const pager = el('div', 'member-pager');
  let shown = {members: [], leaving: [], rules: []};
  let page = 0;
  const draw = () => {
    const {members, leaving, rules} = shown;
    const q = search.value.trim().toLowerCase();
    const wanted = m => !q || (m.name || '').toLowerCase().includes(q) || (m.email || '').toLowerCase().includes(q) || (m.words || '').toLowerCase().includes(q);
    const rows = [
      ...members.filter(wanted).map(m => ({m, leaving: false})),
      ...leaving.filter(wanted).map(m => ({m, leaving: true})),
    ];
    const pages = Math.max(1, Math.ceil(rows.length / pageSize));
    page = Math.min(page, pages - 1);
    list.replaceChildren();
    for (const {m, leaving: going} of rows.slice(page * pageSize, (page + 1) * pageSize)) {
      if (going) {
        const row = compactRow(m, {why: 'No rule picks them out any more.', chip: el('span', 'chip leaves', 'Leaves'), gradeColors});
        row.classList.add('is-leaving');
        list.append(row);
        continue;
      }
      list.append(compactRow(m, {
        why: reasonWords(m, rules, phrase),
        chip: chip(m),
        onRemove: () => (m.outside ? onRemove(m) : onExclude(m)),
        removeWords: m.outside ? `Remove ${m.name}` : `Exclude ${m.name} from this ${listWord}`,
        gradeColors,
      }));
    }
    fillPager(pager, page, rows.length, to => {
      page = to;
      draw();
    });
    search.hidden = !members.length && !leaving.length;
    empty.hidden = rows.length > 0 || (!members.length && !leaving.length);
  };
  search.addEventListener('input', () => {
    page = 0;
    draw();
  });
  const card = el('div', 'card preview');
  card.append(headRow, status, changes, search, list, empty, pager);
  return {
    card,
    status,
    changes,
    show(next) {
      shown = {leaving: [], ...next};
      head.textContent = `${shown.members.length} ${shown.members.length === 1 ? noun[0] : noun[1]}`;
      draw();
    },
  };
}

export function compactRow(m, {why, chip, onRemove, removeWords, gradeColors}) {
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
    tail.append(iconButton('close', removeWords, 'compact-remove', onRemove));
  }
  return personRow(m, {
    className: 'compact-row' + (m.outside ? ' is-outside' : ''),
    lines: [line],
    after: [el('span', 'compact-why', why), tail],
    gradeColors,
  });
}

export function memberAdders(person, outside) {
  const open = (make, title) => {
    let shut = null;
    const adder = make(() => shut());
    shut = popup(title, adder, {wide: true}).shut;
    setTimeout(() => adder.focus(), 0);
  };
  const buttons = el('div', 'head-buttons');
  buttons.append(
    button('Add Helios', 'plus', 'button button-small button-secondary', () => open(person, 'Add from the directory')),
    button('Add Non-Helios', 'plus', 'button button-small button-secondary', () => open(outside, 'Add someone outside Helios')),
  );
  return {buttons};
}

const roleTests = {student: p => p.isStudent, parent: p => p.isParent, staff: p => p.isStaff};

const relations = ['Parents', 'Children', 'Siblings'];

const firstName = p => (p.fullName || p.email || '').split(' ')[0];

export function personAdder({isOn, onAdd, gradeColors}) {
  const wrap = el('div', 'person-adder');
  const pick = {
    dir: null,
    people: [],
    picked: new Map(),
    family: new Set(),
    roles: new Set(),
    search: el('input', 'picker-search'),
    list: el('div', 'picker-results'),
    add: el('button', 'button'),
  };
  pick.search.type = 'search';
  pick.search.placeholder = 'Search by name or email';
  pick.add.type = 'button';
  const status = el('span', 'save-status');
  const relativesOf = p => {
    const out = [];
    for (const relation of relations) {
      if (!pick.family.has(relation)) {
        continue;
      }
      for (const r of pick.dir.follow(p, relation.toLowerCase())) {
        if (r && r.email && !isOn(r.email) && !out.includes(r)) {
          out.push(r);
        }
      }
    }
    return out;
  };
  const everyone = () => {
    const out = new Map();
    for (const p of pick.picked.values()) {
      out.set(p.email, {email: p.email, name: p.fullName, via: 'search'});
      for (const r of relativesOf(p)) {
        if (!out.has(r.email)) {
          out.set(r.email, {email: r.email, name: r.fullName, via: 'family'});
        }
      }
    }
    return out;
  };
  const paintButton = () => {
    const n = everyone().size;
    pick.add.replaceChildren(svg('plus'), el('span', '', n ? `Add ${n} ${n === 1 ? 'person' : 'people'}` : 'Add to the list'));
    pick.add.disabled = !n;
  };
  const choice = p => {
    const on = isOn(p.email);
    const mark = el('span', 'picker-check');
    mark.append(svg('check'));
    const along = relativesOf(p).map(firstName);
    const line = [on ? 'On the list' : contactLine(pick.dir, p), along.length ? 'with ' + along.join(', ') : ''].filter(Boolean).join(' · ');
    const row = personRow({name: p.fullName || p.email, email: p.email, photoUrl: p.heroPhotoUrl && p.heroPhotoUrl + '?thumb=1', grade: p.grade}, {
      button: true,
      className: 'picker-person' + (on ? ' is-on' : pick.picked.has(p.email) ? ' is-picked' : ''),
      before: [mark],
      lines: [line ? el('div', 'person-row-line' + (along.length ? ' has-family' : ''), line) : null],
      gradeColors,
    });
    row.disabled = on;
    row.addEventListener('click', () => {
      if (pick.picked.has(p.email)) {
        pick.picked.delete(p.email);
      } else {
        pick.picked.set(p.email, p);
      }
      row.classList.toggle('is-picked', pick.picked.has(p.email));
      paintButton();
    });
    return row;
  };
  const paintList = () => {
    pick.list.replaceChildren();
    if (!pick.dir) {
      pick.list.append(el('div', 'picker-note', 'Loading the directory…'));
      return;
    }
    const q = pick.search.value.trim().toLowerCase();
    const tests = [...pick.roles].map(role => roleTests[role]);
    const found = pick.people.filter(p => (!q || p.fullName.toLowerCase().includes(q) || p.email.toLowerCase().includes(q)) && (!tests.length || tests.some(t => t(p))));
    if (!found.length) {
      pick.list.append(el('div', 'picker-note', 'Nobody by that name. Someone outside Helios goes on Add Non-Helios.'));
    }
    for (const p of found.slice(0, 200)) {
      pick.list.append(choice(p));
    }
    if (found.length > 200) {
      pick.list.append(el('div', 'picker-note', `${found.length - 200} more - type a name to narrow it.`));
    }
  };
  pick.search.addEventListener('input', paintList);
  const bar = el('div', 'picker-bar');
  const chips = el('div', 'chip-row picker-roles');
  for (const [role, label] of [['student', 'Students'], ['parent', 'Parents'], ['staff', 'Staff']]) {
    chips.append(chipToggle(label, false, on => {
      if (on) {
        pick.roles.add(role);
      } else {
        pick.roles.delete(role);
      }
      paintList();
    }, role));
  }
  bar.append(chips, familyDropdown(relations, pick.family, () => {
    paintList();
    paintButton();
  }));
  pick.add.addEventListener('click', async () => {
    pick.add.disabled = true;
    status.textContent = '';
    status.classList.remove('error');
    try {
      await onAdd([...everyone().values()]);
      pick.picked.clear();
      paintList();
      paintButton();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      pick.add.disabled = false;
    }
  });
  const foot = el('div', 'modal-actions');
  foot.append(pick.add, status);
  wrap.append(pick.search, bar, pick.list, foot);
  wrap.focus = () => pick.search.focus();
  paintList();
  paintButton();
  directory().then(dir => {
    pick.dir = dir;
    pick.people = dir.result.map(dir.get);
    paintList();
  }).catch(err => {
    status.textContent = 'Couldn’t load the directory: ' + err.message;
    status.classList.add('error');
  });
  return wrap;
}

const emailForm = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

export function outsideAdder({isOn, onAdd, note}) {
  const wrap = el('div', 'outside-adder-wrap');
  wrap.append(el('div', 'picker-note', note));
  const families = el('div', 'outside-families');
  families.append(outsideFamilyCard());
  wrap.append(families, button('Add another family', 'plus', 'link-button outside-another', () => {
    const card = outsideFamilyCard();
    families.append(card);
    card.focus();
  }));
  const status = el('span', 'save-status');
  const foot = el('div', 'modal-actions');
  const add = button('Add to the list', 'plus', 'button', async () => {
    status.classList.remove('error');
    status.textContent = '';
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
      if (isOn(head.email)) {
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
      await onAdd(people);
      families.replaceChildren(outsideFamilyCard());
      add.disabled = false;
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      add.disabled = false;
    }
  });
  foot.append(add, status);
  wrap.append(foot);
  wrap.focus = () => families.firstChild.focus();
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
  family.append(el('div', 'outside-members-title', 'Family members (optional)'), el('div', 'picker-note', 'A spouse, a partner, children. Anyone with an email address gets their own.'));
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
    const remove = iconButton('trash', 'Remove', 'compact-remove', () => {
      members.splice(members.indexOf(m), 1);
      paintOutsideMembers(list, members);
    });
    list.append(personRow({name: m.name, email: m.email}, {
      className: 'outside-member',
      lines: [m.email || 'No email address'],
      after: [remove],
    }));
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
