import {api} from '/api.js';
import {keyBetween, movedKey} from '/order.js';
import {ruleRow, ruleSaysSomething} from '/listrules.js';
import {state} from './state.js';

export function write(batch) {
  if (!batch.length) {
    return Promise.resolve({result: []});
  }
  return api('POST', '/api/q', {batch});
}

function things() {
  const out = [];
  for (const w of state.model.widgets) {
    out.push(w);
  }
  for (const a of state.model.apps) {
    out.push(a);
  }
  for (const c of state.model.categories) {
    for (const l of c.links) {
      out.push(l);
    }
  }
  return out;
}

function dedicated(group) {
  if (!group || group === state.model.everyone) {
    return false;
  }
  const g = state.model.audiences[group];
  if (!g || g.kind !== 'group' || g.slug) {
    return false;
  }
  return things().filter(t => t.visibleTo === group).length === 1;
}

function deletable(group) {
  const g = state.model.audiences[group];
  return dedicated(group) && g.listed !== 'Yes';
}

function ruleInserts(group, rules) {
  let order = '';
  return rules.filter(ruleSaysSomething).map(r => {
    order = keyBetween(order, null);
    return {insert: 'RULE', row: ruleRow(group, r, order)};
  });
}

function clearAudience(group) {
  return [
    ...(state.model.audienceRules[group] || []).map(r => ({delete: r.id})),
    ...(state.model.audienceMembers[group] || []).map(m => ({delete: m.id})),
  ];
}

export function audienceEdits(thing, name, draft) {
  const old = thing.visibleTo;
  const rules = (draft.rules || []).filter(ruleSaysSomething);
  const people = draft.people || [];
  if (draft.hidden) {
    if (!old) {
      return [];
    }
    const edits = dedicated(old) ? clearAudience(old) : [];
    edits.push({set: thing.id, cells: {visible_to: ''}});
    if (deletable(old)) {
      edits.push({delete: old});
    }
    return edits;
  }
  if (!rules.length && !people.length) {
    if (old === state.model.everyone) {
      return [];
    }
    const edits = dedicated(old) ? clearAudience(old) : [];
    edits.push({set: thing.id, cells: {visible_to: state.model.everyone}});
    if (deletable(old)) {
      edits.push({delete: old});
    }
    return edits;
  }
  if (dedicated(old)) {
    const before = state.model.audienceMembers[old] || [];
    return [
      ...(state.model.audienceRules[old] || []).map(r => ({delete: r.id})),
      ...ruleInserts(old, rules),
      ...before.filter(m => !people.includes(m.person)).map(m => ({delete: m.id})),
      ...people.filter(p => !before.some(m => m.person === p)).map(p => ({insert: 'MEMBER', row: {group: old, person: p, member: 'yes', added_by: state.model.user.id}})),
    ];
  }
  return [
    {insert: 'GROUP', as: 'audience', row: {kind: 'group', name: `${name} Viewers`, status: 'open', listed: false, posting: 'none', replying: 'none', added_by: state.model.user.id}},
    {set: thing.id, cells: {visible_to: '@audience'}},
    ...ruleInserts('@audience', rules),
    ...people.map(p => ({insert: 'MEMBER', row: {group: '@audience', person: p, member: 'yes', added_by: state.model.user.id}})),
  ];
}

export async function setPicture(groupId, name, old) {
  const blob = await (await fetch('/' + name)).blob();
  const form = new FormData();
  form.append('group', groupId);
  form.append('photo', blob, name.split('/').pop());
  await api('POST', '/api/do/photo', form);
  if (old) {
    await write([{delete: old}]);
  }
}

function lastOrder(list) {
  const orders = list.map(x => x.order).filter(Boolean).sort();
  return keyBetween(orders[orders.length - 1] || '', null);
}

export async function saveLink(link, fields) {
  const category = state.model.categories.find(c => c.id === fields.category);
  const draft = {hidden: !fields.visible, rules: fields.rules};
  if (!link) {
    const row = {
      kind: 'group', parent: category.group, name: fields.title, description: fields.description, url: fields.url, status: 'open', listed: false,
      posting: 'none', replying: 'none', order: lastOrder(category.links), visible_to: state.model.everyone, added_by: state.model.user.id,
    };
    const made = await write([{insert: 'GROUP', as: 'link', row}]);
    const id = made.result[0];
    await write(audienceEdits({id, visibleTo: state.model.everyone}, fields.title, draft));
    if (fields.image) {
      await setPicture(id, fields.image, '');
    }
    return;
  }
  const cells = {};
  for (const [column, value] of [['name', fields.title], ['description', fields.description], ['url', fields.url]]) {
    if ((link[column === 'name' ? 'title' : column] || '') !== value) {
      cells[column] = value;
    }
  }
  if (link.category !== fields.category) {
    cells.parent = category.group;
    cells.order = lastOrder(category.links);
  }
  const edits = Object.keys(cells).length ? [{set: link.id, cells}] : [];
  await write([...edits, ...audienceEdits(link, fields.title, draft)]);
  if (fields.image !== undefined) {
    if (fields.image) {
      await setPicture(link.id, fields.image, link.photo);
    } else if (link.photo) {
      await write([{delete: link.photo}]);
    }
  }
}

export async function deleteLink(link) {
  const edits = [];
  if (dedicated(link.visibleTo)) {
    edits.push(...clearAudience(link.visibleTo));
  }
  if (link.photo) {
    edits.push({delete: link.photo});
  }
  edits.push({delete: link.id});
  if (deletable(link.visibleTo)) {
    edits.push({delete: link.visibleTo});
  }
  await write(edits);
}

const styles = {cards: 'grid', 'apps-grid': 'grid', tiles: 'list', apps: 'list'};

export async function saveCategory(category, fields) {
  const style = styles[fields.style];
  const picture = fields.emoji.startsWith('image:') ? fields.emoji.slice('image:'.length) : '';
  const icon = fields.emoji.startsWith('image:') ? '' : fields.emoji;
  if (!category) {
    const widgets = state.model.widgets;
    const made = await write([
      {insert: 'GROUP', as: 'section', row: {kind: 'group', name: fields.title, status: 'open', listed: false, posting: 'none', replying: 'none', visible_to: state.model.everyone, added_by: state.model.user.id}},
      {insert: 'WIDGET', as: 'widget', row: {key: 'links', group: '@section', name: fields.title, icon, style, descriptions: fields.descriptions, sidebar: fields.sidebar, visible_to: state.model.everyone, order: lastOrder(widgets)}},
    ]);
    await write(audienceEdits({id: made.result[1], visibleTo: state.model.everyone}, fields.title, {rules: fields.rules}));
    if (picture) {
      await setPicture(made.result[0], picture, '');
    }
    return;
  }
  const widget = state.model.widgets.find(w => w.id === category.id);
  const cells = {};
  if (fields.title !== category.title) {
    cells.name = fields.title;
  }
  if (icon !== category.icon) {
    cells.icon = icon;
  }
  if (style !== styles[category.style]) {
    cells.style = style;
  }
  if (fields.descriptions !== category.descriptions) {
    cells.descriptions = fields.descriptions;
  }
  if (fields.sidebar !== widget.sidebar) {
    cells.sidebar = fields.sidebar;
  }
  const edits = Object.keys(cells).length ? [{set: category.id, cells}] : [];
  if (cells.name && category.group) {
    edits.push({set: category.group, cells: {name: fields.title}});
  }
  await write([...edits, ...audienceEdits(widget, fields.title, {rules: fields.rules})]);
  if (picture) {
    await setPicture(category.group, picture, category.photo);
  } else if (!fields.emoji.startsWith('image:') && category.photo) {
    await write([{delete: category.photo}]);
  }
}

export async function deleteCategory(category) {
  const widget = state.model.widgets.find(w => w.id === category.id);
  const edits = dedicated(widget.visibleTo) ? clearAudience(widget.visibleTo) : [];
  if (category.photo) {
    edits.push({delete: category.photo});
  }
  edits.push({delete: category.id}, {delete: category.group});
  if (deletable(widget.visibleTo)) {
    edits.push({delete: widget.visibleTo});
  }
  await write(edits);
}

export async function saveApp(app, fields) {
  const cells = {};
  if (fields.name !== app.name) {
    cells.name = fields.name;
  }
  if (fields.tagline !== app.tagline) {
    cells.subtitle = fields.tagline;
  }
  const edits = Object.keys(cells).length ? [{set: app.id, cells}] : [];
  const draft = fields.everyone ? {rules: [], people: []} : {rules: fields.rules, people: fields.people};
  if (!fields.everyone && !draft.rules.filter(ruleSaysSomething).length && !draft.people.length) {
    throw new Error('Pick out who sees it: add a rule or a person, or open it to everyone.');
  }
  await write([...edits, ...audienceEdits(app, fields.name, draft)]);
}

export async function saveWidget(widget, name, fields) {
  const edits = fields.sidebar !== widget.sidebar ? [{set: widget.id, cells: {sidebar: fields.sidebar}}] : [];
  await write([...edits, ...audienceEdits(widget, name, {rules: fields.rules})]);
}

export function move(list, from, to) {
  return write([{set: list[from].id, cells: {order: movedKey(list.map(item => item.order), from, to)}}]);
}

export function saveLayout(rows) {
  const value = rows.join('; ');
  if (state.model.layoutSetting) {
    return write([{set: state.model.layoutSetting, cells: {value}}]);
  }
  return write([{insert: 'SETTING', row: {app: 'home', key: 'layout', value}}]);
}

export function markToDo(todo, next) {
  if (!next) {
    return write(todo.mark ? [{delete: todo.mark.id}] : []);
  }
  const now = new Date();
  const pad = n => String(n).padStart(2, '0');
  const changed = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;
  if (todo.mark) {
    return write([{set: todo.mark.id, cells: {state: next, changed}}]);
  }
  return write([{insert: 'TODO_MARK', row: {person: state.model.user.id, todo: todo.id, state: next, changed}}]);
}
