import {el, button, iconButton} from '/elements.js';
import {select as selectInput, text as textInput} from '/form.js';
import {queries} from '/rail.js';

const whoWords = {'': 'the people', parents: 'their parents', children: 'their children', household: 'their households'};

const kindWords = {group: 'group', classroom: 'classroom', grade: 'grade', gradeband: 'gradeband', crew: 'crew', department: 'department', activity: 'activity', party: 'party', event: 'event'};

const roleSlugs = ['students', 'parents', 'staff', 'adults'];

let choices = null;

let names = {group: () => null, person: () => ''};

export function nameRulesWith(lookup) {
  names = lookup;
}

function rowsOf(answer, table) {
  return answer.result.map(id => answer.resources[table][id]);
}

export async function ruleChoices() {
  if (choices) {
    return choices;
  }
  const answers = await queries({
    groups: `(from GROUP @g (where (sees_members @g) (!= status "closed") (in kind ${Object.keys(kindWords).map(k => JSON.stringify(k)).join(' ')})) (order name asc) (columns name kind slug))`,
    roles: `(from GROUP (where (= kind "group") (in slug ${roleSlugs.map(s => JSON.stringify(s)).join(' ')})) (columns name slug))`,
  });
  const groups = rowsOf(answers.groups, 'GROUP');
  const roles = rowsOf(answers.roles, 'GROUP');
  choices = {groups, roles, byId: {}};
  for (const g of [...groups, ...roles]) {
    choices.byId[g.id] = g;
  }
  return choices;
}

function groupName(id) {
  const g = (choices && choices.byId[id]) || names.group(id);
  return g ? g.name : 'a group you cannot see';
}

export function ruleWords(r) {
  let words = '';
  if (r.target) {
    words = `${groupName(r.target)} — ${whoWords[r.replace_with || '']}`;
  } else if (r.person) {
    words = names.person(r.person);
  } else if (r.search) {
    words = `whoever's name or address holds “${r.search}”`;
  }
  if (r.within) {
    words += `, only ${groupName(r.within).toLowerCase()}`;
  }
  return (r.exclude ? 'Leaving out ' : '') + words;
}

export function ruleSaysSomething(r) {
  return Boolean(r.target || r.person || r.search);
}

export function newRule() {
  return {exclude: false, target: '', replace_with: '', within: '', person: '', search: ''};
}

export function ruleOf(row) {
  return {id: row.id, exclude: row.exclude === 'Yes', target: row.target || '', replace_with: row.replace_with || '', within: row.within || '', person: row.person || '', search: row.search || ''};
}

export function ruleRow(group, r, order) {
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

function label(g) {
  return `${g.name} (${kindWords[g.kind] || g.kind})`;
}

function targetInput(rule, onChange) {
  const input = textInput(rule.target ? label(choices.byId[rule.target] || {name: groupName(rule.target), kind: ''}) : '', {placeholder: 'Pick a group, class, activity…'});
  const listId = 'rule-groups';
  if (!document.getElementById(listId)) {
    const list = el('datalist');
    list.id = listId;
    for (const g of choices.groups) {
      const option = el('option');
      option.value = label(g);
      list.append(option);
    }
    document.body.append(list);
  }
  input.setAttribute('list', listId);
  input.addEventListener('change', () => {
    const g = choices.groups.find(x => label(x) === input.value);
    rule.target = g ? g.id : '';
    input.classList.toggle('is-wanted', !g);
    onChange();
  });
  return input;
}

function ruleLine(rule, {onChange, onRemove, count}) {
  const row = el('div', 'rule-edit-row');
  if (!rule.target && (rule.person || rule.search)) {
    row.append(el('span', 'rule-line-words', ruleWords(rule)), iconButton('close', 'Remove the rule', 'compact-remove', onRemove));
    return row;
  }
  const kind = selectInput([{value: 'include', label: 'Include'}, {value: 'exclude', label: 'Leave out'}], rule.exclude ? 'exclude' : 'include');
  kind.addEventListener('change', () => {
    rule.exclude = kind.value === 'exclude';
    onChange();
  });
  const who = selectInput(Object.entries(whoWords).map(([value, words]) => ({value, label: words})), rule.replace_with || '');
  who.addEventListener('change', () => {
    rule.replace_with = who.value;
    onChange();
  });
  const only = selectInput([{value: '', label: 'anyone'}, ...choices.roles.map(r => ({value: r.id, label: 'only ' + r.name.toLowerCase()}))], rule.within || '');
  only.addEventListener('change', () => {
    rule.within = only.value;
    onChange();
  });
  const chip = el('span', 'chip rule-count');
  const n = count(rule);
  chip.textContent = n === undefined ? '' : `${n} ${rule.exclude ? 'left out' : 'match'}`;
  chip.hidden = n === undefined;
  row.append(kind, targetInput(rule, onChange), who, only);
  if (onRemove) {
    row.append(iconButton('close', 'Remove the rule', 'compact-remove', onRemove));
  }
  const line = el('div', 'rule-edit');
  line.append(row, chip);
  return line;
}

export function ruleEditor(rule, onChange) {
  const holder = el('div', 'rule-edit-rows');
  const paint = () => {
    holder.replaceChildren(ruleLine(rule, {onChange, onRemove: null, count: () => undefined}));
  };
  if (choices) {
    paint();
  } else {
    holder.append(el('div', 'save-status', 'Loading the groups…'));
    ruleChoices().then(paint, err => {
      holder.replaceChildren(el('div', 'save-status error', err.message));
    });
  }
  return holder;
}

export function rulesCard({list, onChange, onRemove, onAdd, count, empty = 'No rules yet: only the people added by hand are on the email list.', hint = 'Each rule takes in a group\'s members, or their parents, children or households, and can keep to one role. A Leave out rule keeps whoever it picks off the list.'}) {
  const card = el('div', 'card');
  const head = el('div', 'card-head');
  head.append(el('h2', '', 'Rules'));
  const status = el('div', 'save-status');
  const rows = el('div', 'rule-edit-rows');
  const render = () => {
    rows.replaceChildren();
    if (!choices) {
      status.textContent = 'Loading the groups…';
      ruleChoices().then(() => {
        status.textContent = '';
        render();
      }, err => {
        status.classList.add('error');
        status.textContent = err.message;
      });
      return;
    }
    for (const rule of list()) {
      rows.append(ruleLine(rule, {onChange, onRemove: () => onRemove(rule), count}));
    }
    if (!list().length) {
      rows.append(el('div', 'rule-empty', empty));
    }
  };
  const add = button('Add rule', 'plus', 'button button-small button-secondary', () => {
    onAdd(newRule());
    render();
  });
  head.append(add);
  card.append(head, el('div', 'hint', hint), status, rows);
  render();
  return {card, render};
}
