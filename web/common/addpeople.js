import {el, button, longToast} from '/elements.js';
import {popup} from '/modal.js';
import {personAdder, outsideAdder} from '/members.js';
import {importAdder} from '/contactimport.js';
import {ruleChoices, newRule} from '/listrules.js';
import {addPeople, addRules, previewRules} from '/guestlist.js';
import {host} from '/guesthost.js';

export const outsideNote = 'Someone outside Helios - a coach, a grandparent, a friend - and their family. They get the email and the calendar invite with a page of their own to answer from, no sign-in needed, that shows the event and nothing of who else is coming. Everyone in a family answers for the others.';

export function inviteWords(host, people) {
  const n = people.length;
  if (host) {
    return n === 1 ? `${people[0].name} added to the list.` : `${n} people added to the list.`;
  }
  return n === 1 ? `${people[0].name} invited - the invitation is on its way.` : `${n} people invited - the invitations are on their way.`;
}

export function guestAdders(e, view, done) {
  const onList = new Set([...(view.list || []), ...view.mine].filter(r => r.invited && r.email).map(r => r.email));
  const isOn = email => onList.has(email);
  const onAdd = close => async people => {
    await addPeople(e, view, people);
    close();
    done(inviteWords(view.host, people));
  };
  return {
    person: close => personAdder({isOn, onAdd: onAdd(close), gradeColors: host.gradeColors()}),
    outside: close => outsideAdder({isOn, onAdd: onAdd(close), note: outsideNote}),
    import: close => importAdder({isOn, onAdd: onAdd(close), note: importNote}),
  };
}

const importNote = 'Many people from outside Helios at once - paste names and email addresses, or choose a contacts file. Each is added on their own and gets an invitation and a page of their own to answer from.';

export function openPicker(e, view, refresh, {tab = 'person'} = {}) {
  const box = el('div', 'picker');
  const tabs = el('div', 'tabs');
  const panel = el('div', 'picker-panel');
  const kinds = view.host ? [['person', 'Add Person'], ['group', 'Add Group'], ['outside', 'Add Non-Helios'], ['import', 'Import a list']] : [['person', 'Add Person'], ['outside', 'Add Non-Helios']];
  let active = kinds.some(([key]) => key === tab) ? tab : 'person';
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
  const adders = guestAdders(e, view, done);
  const paintPanel = () => {
    panel.replaceChildren();
    const made = active === 'group' ? groupPanel(e, view, done) : adders[active](() => {});
    panel.append(made);
    if (made.focus) {
      setTimeout(() => made.focus(), 0);
    }
  };
  box.append(tabs, panel);
  paintPanel();
  shut = popup('Add to the guest list', box, {wide: true}).shut;
}

const sections = [['classroom', 'Classrooms'], ['grade', 'Grades'], ['gradeband', 'Gradebands'], ['crew', 'Crews'], ['department', 'Departments'], ['activity', 'Activities'], ['party', 'Parties'], ['event', 'Events'], ['group', 'Lists']];

const school = new Set(['classroom', 'grade', 'gradeband']);

const whoChoices = [
  {key: 'parents', school: 'Students and parents', other: 'With their parents', rules: id => [{...newRule(), target: id}, {...newRule(), target: id, replace_with: 'parents'}]},
  {key: 'household', school: 'Whole families', other: 'Whole families', rules: id => [{...newRule(), target: id, replace_with: 'household'}]},
  {key: 'people', school: 'Just the students', other: 'Just them', rules: id => [{...newRule(), target: id}]},
];

function count(n, one, many) {
  return `${n} ${n === 1 ? one : many}`;
}

function gradeOrder(g) {
  const n = g.slug.replace('grade-', '');
  return n === 'k' ? 0 : Number(n) + 1 || 99;
}

function searchable(groups) {
  // A recurring event's dates are imported calendar entries with no guests of their own, so the series stands for them.
  const byId = new Map(groups.map(g => [g.id, g]));
  return groups.filter(g => !(g.kind === 'event' && byId.get(g.parent)?.kind === 'event'));
}

function groupNote(g) {
  if (g.kind === 'event' || g.kind === 'party') {
    return g.start ? host.dayLabel(g.start.slice(0, 10)) : 'repeats';
  }
  return '';
}

function groupPanel(e, view, done) {
  const wrap = el('div', 'picker-group');
  const picked = new Map();
  let who = 'parents';
  let groups = null;
  const search = el('input', 'picker-search');
  search.type = 'search';
  search.placeholder = 'Search classes, grades, activities, lists…';
  const results = el('div', 'picker-results');
  const pickedRow = el('div', 'filter-chips picker-picked');
  const roomChips = el('div', 'filter-chips picker-rooms');
  const gradeChips = el('div', 'filter-chips picker-grades');
  const whoRow = el('div', 'picker-who');
  const preview = el('div', 'audience-preview picker-preview');
  const status = el('span', 'save-status');
  const toggle = g => {
    if (picked.has(g.id)) {
      picked.delete(g.id);
    } else {
      picked.set(g.id, g);
    }
    paint();
    askPreview();
  };
  const chip = (g, color) => {
    const b = el('button', 'filter-chip' + (picked.has(g.id) ? ' is-on' : ''), g.name);
    b.type = 'button';
    if (color) {
      b.style.setProperty('--room', color);
      b.classList.add('has-color');
    }
    b.addEventListener('click', () => toggle(g));
    return b;
  };
  const wanted = () => [...picked.keys()].flatMap(whoChoices.find(c => c.key === who).rules);
  const rules = () => wanted().filter(r => !view.rules.some(x => !x.exclude && !x.within && x.target === r.target && x.replace_with === r.replace_with));
  const reached = (want, answer) => {
    const byRules = answer.members.map(m => m.reasons.filter(r => r.rule !== undefined).map(r => want[r.rule])).filter(rs => rs.length);
    const students = byRules.filter(rs => rs.some(r => !r.replace_with)).length;
    return {people: byRules.length, students, parents: byRules.length - students};
  };
  const allSchool = () => [...picked.values()].every(g => school.has(g.kind));
  let asked = 0;
  const askPreview = async () => {
    const mark = ++asked;
    const want = wanted();
    if (!want.length) {
      preview.textContent = '';
      return;
    }
    try {
      const answer = await previewRules(e, view, want);
      if (mark !== asked) {
        return;
      }
      const names = [...picked.values()].map(g => g.name).join(', ');
      const n = reached(want, answer);
      const already = rules().length ? '' : ' Already on the guest list.';
      if (who === 'parents' && allSchool()) {
        preview.textContent = `${names}: ${count(n.students, 'student', 'students')} and ${count(n.parents, 'parent', 'parents')}.${already}`;
        return;
      }
      preview.textContent = `${names}: ${count(n.people, 'person', 'people')}.${already}`;
    } catch (err) {
      preview.textContent = err.message;
    }
  };
  const paintResults = () => {
    results.replaceChildren();
    const words = search.value.toLowerCase().split(/\s+/).filter(Boolean);
    if (!words.length) {
      return;
    }
    if (!groups) {
      results.append(el('div', 'save-status', 'Loading the groups…'));
      return;
    }
    const found = groups.filter(g => words.every(w => g.name.toLowerCase().includes(w)));
    if (!found.length) {
      results.append(el('div', 'picker-result-none', 'Nothing by that name.'));
      return;
    }
    const seen = new Map();
    for (const g of found) {
      seen.set(g.name, (seen.get(g.name) || 0) + 1);
    }
    for (const [kind, title] of sections) {
      const hits = found.filter(g => g.kind === kind);
      if (!hits.length) {
        continue;
      }
      results.append(el('div', 'picker-result-head', title));
      for (const g of hits) {
        const row = el('button', 'picker-result' + (picked.has(g.id) ? ' is-on' : ''));
        row.type = 'button';
        row.append(el('span', '', g.name), el('small', '', picked.has(g.id) ? 'Picked' : groupNote(g) || (seen.get(g.name) > 1 ? g.slug : '')));
        row.addEventListener('click', () => toggle(g));
        results.append(row);
      }
    }
  };
  const paint = () => {
    roomChips.replaceChildren(...host.gradebands().flatMap(b => b.classrooms).map(c => chip({id: c.id, name: c.name, kind: 'classroom'}, host.classroomColors()[c.name])));
    const grades = groups ? groups.filter(g => g.kind === 'grade').sort((a, b) => gradeOrder(a) - gradeOrder(b)) : [];
    gradeChips.replaceChildren(...grades.map(g => chip(g, g.color)));
    const others = [...picked.values()].filter(g => g.kind !== 'classroom' && g.kind !== 'grade');
    pickedRow.replaceChildren(...others.map(g => {
      const b = chip(g);
      b.append(el('span', 'picker-unpick', '×'));
      return b;
    }));
    pickedRow.hidden = !others.length;
    whoRow.replaceChildren();
    for (const c of whoChoices) {
      const b = el('button', 'picker-who-choice' + (c.key === who ? ' is-on' : ''), allSchool() ? c.school : c.other);
      b.type = 'button';
      b.addEventListener('click', () => {
        who = c.key;
        paint();
        askPreview();
      });
      whoRow.append(b);
    }
    paintResults();
  };
  search.addEventListener('input', paintResults);
  ruleChoices().then(c => {
    groups = searchable(c.groups).filter(g => g.id !== e.id);
    paint();
  }, err => {
    status.textContent = err.message;
    status.classList.add('error');
  });
  paint();
  wrap.append(search, results, pickedRow, el('div', 'picker-label', 'Classrooms'), roomChips, el('div', 'picker-label', 'Grades'), gradeChips, el('div', 'picker-label', 'Who'), whoRow, preview);
  wrap.append(el('small', 'hint', view.autoInvite ? 'Whoever joins these later goes on the list and is sent the invitation as they come.' : 'Whoever joins these later goes on the list, and waits in Pending for you to send.'));
  const foot = el('div', 'modal-actions');
  const add = button('Add', 'plus', 'button', async () => {
    const want = rules();
    if (!want.length) {
      status.textContent = picked.size ? 'Already on the guest list.' : 'Pick a classroom, a grade or a group.';
      status.classList.add('error');
      return;
    }
    add.disabled = true;
    try {
      const all = wanted();
      const answer = await previewRules(e, view, all);
      await addRules(e, view, want);
      const names = [...picked.values()].map(g => g.name).join(', ').replace(/, ([^,]*)$/, ' and $1');
      done(`${names} added: ${count(reached(all, answer).people, 'person', 'people')} on the list now.`);
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
