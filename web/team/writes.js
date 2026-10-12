import {state, me, activity, category, descendants, yearGroup, addingValue, write, formatWhen} from './state.js';
import {known} from '/directory.js';
import {keyBetween} from '/order.js';
import {api} from '/api.js';

function now() {
  return formatWhen(new Date(), true);
}

function joinOf(direct, complete) {
  if (complete) {
    return 'none';
  }
  return direct ? 'others' : 'below';
}

function yearEdits(year) {
  const existing = yearGroup(year);
  if (existing) {
    return {id: existing.id, edits: []};
  }
  const current = state.model.yearGroups.find(g => g.name === state.model.years.current);
  return {
    id: '@year',
    edits: [{insert: 'GROUP', as: 'year', row: {
      kind: 'activity', name: year, parent: current.parent, status: 'open', visible_to: state.model.everyone,
      members_visible_to: state.model.everyone, adding: 'managers', notify: current.notify || '', added_by: me().id,
    }}],
  };
}

function placeCell(fields, node, edits) {
  const parent = fields.parent !== undefined ? fields.parent : (node ? node.parent : '');
  const cat = fields.category !== undefined ? fields.category : (node ? node.category : '');
  if (cat && category(cat) && (category(cat).eventId === parent || (!parent && !category(cat).eventId))) {
    return cat;
  }
  if (parent) {
    return parent;
  }
  const year = yearEdits(fields.year || (node ? node.year : state.model.years.current));
  edits.push(...year.edits);
  return year.id;
}

function visibility(open) {
  return open ? state.model.everyone : '';
}

function treeOf(node) {
  const out = [node, ...descendants(node)];
  for (const n of [...out]) {
    out.push(...(n.categories || []));
  }
  return out;
}

export function activityCells(fields, node, edits) {
  const cells = {};
  const copy = {title: 'name', description: 'description', start: 'start', end: 'end', timing: 'timing', location: 'location', prettyId: 'slug', flyer: 'flyer'};
  for (const [from, to] of Object.entries(copy)) {
    if (fields[from] !== undefined) {
      cells[to] = fields[from];
    }
  }
  if (fields.spots !== undefined) {
    cells.capacity = fields.spots ? String(fields.spots) : '';
  }
  if (fields.priority !== undefined) {
    cells.priority = Boolean(fields.priority);
  }
  if (fields.allowAdding !== undefined) {
    cells.adding = addingValue(fields.allowAdding);
  }
  if (fields.directSignUp !== undefined || fields.volunteersComplete !== undefined) {
    const direct = fields.directSignUp !== undefined ? fields.directSignUp === 'Yes' : node && node.directSignUp;
    const complete = fields.volunteersComplete !== undefined ? fields.volunteersComplete : node && node.volunteersComplete;
    cells.join = joinOf(direct, complete);
  }
  if (fields.volunteersHidden !== undefined) {
    cells.members_visible_to = visibility(fields.volunteersHidden !== 'Yes');
  }
  if (fields.status === 'Done') {
    cells.status = 'done';
  } else if (fields.status === 'Open' && node && node.status !== 'Hidden') {
    cells.status = 'open';
  }
  if (fields.parent !== undefined || fields.category !== undefined || fields.year !== undefined) {
    cells.parent = placeCell(fields, node, edits);
  }
  return cells;
}

function hideEdits(node, open) {
  const edits = [];
  for (const n of treeOf(node)) {
    edits.push({set: n.id, cells: {visible_to: visibility(open)}});
    if (n.row.chair_offers) {
      edits.push({set: n.row.chair_offers, cells: {visible_to: visibility(open)}});
    }
  }
  return edits;
}

export async function saveActivity(fields) {
  const edits = [];
  if (fields.id) {
    const node = activity(fields.id);
    const cells = activityCells(fields, node, edits);
    if (Object.keys(cells).length) {
      edits.push({set: node.id, cells});
    }
    if (fields.status === 'Hidden' || (fields.status === 'Open' && node.status === 'Hidden')) {
      edits.push(...hideEdits(node, fields.status === 'Open'));
    }
    if (edits.length) {
      await write(edits);
    }
    if (fields.image !== undefined && fields.image !== node.image) {
      if (fields.image) {
        await setPicture(node.id, fields.image, node.image);
      } else if (node.image) {
        await removePicture(node.image);
      }
    }
    return node.id;
  }
  const parent = fields.parent ? activity(fields.parent) : null;
  const policy = (fields.category && category(fields.category)) || parent;
  const runs = parent ? parent.canEdit : state.model.allowances.includes('team.curate');
  const live = runs || state.model.allowances.includes('team.curate') || Boolean(policy && policy.allowAdding === 'Yes');
  const cells = activityCells({...fields, directSignUp: 'Yes', volunteersHidden: 'No', allowAdding: fields.allowAdding || 'No'}, null, edits);
  edits.push({insert: 'GROUP', as: 'made', row: {
    ...cells, kind: 'activity', status: live ? 'open' : 'pending',
    visible_to: state.model.everyone, members_visible_to: state.model.everyone, added_by: me().id, added: now().slice(0, 10),
  }});
  const made = edits.length - 1;
  if (fields.signUp && fields.signUp !== 'none') {
    edits.push({insert: 'MEMBER', row: {group: '@made', person: me().id, member: 'yes', added_by: me().id, added: now()}});
  }
  const ids = await write(edits);
  if (fields.image) {
    await setPicture(ids.result[made], fields.image, '');
  }
  return ids.result[made];
}

export function personFor(email) {
  const p = email ? known(email) : null;
  return p ? p.id : me().id;
}

function chairEdits(node, person) {
  if (node.row.managed_by) {
    return node.managerRows.some(m => m.person === person) ? [] : [{insert: 'MEMBER', row: {group: node.row.managed_by, person, member: 'yes'}}];
  }
  return [
    {insert: 'GROUP', as: 'managers', row: {kind: 'group', name: `${node.title} Managers`, status: 'open', added_by: me().id}},
    {set: node.id, cells: {managed_by: '@managers'}},
    {set: '@managers', cells: {managed_by: '@managers'}},
    {insert: 'MEMBER', row: {group: '@managers', person, member: 'yes'}},
  ];
}

function unchairEdits(node, person) {
  return node.managerRows.filter(m => m.person === person).map(m => ({delete: m.id}));
}

export async function signUp(node, {email, position, note, from}) {
  const person = personFor(email);
  const edits = [];
  const old = from ? activity(from) : null;
  if (old) {
    const row = old.volunteers.find(v => v.person === person);
    if (row) {
      edits.push({delete: row.id});
    }
  }
  const existing = node.volunteers.find(v => v.person === person);
  const lead = position === 'Co-Chair';
  const offering = position === 'Open to Co-Chair';
  if (existing) {
    edits.push({set: existing.id, cells: {note, lead}});
  } else {
    edits.push({insert: 'MEMBER', row: {group: node.id, person, member: 'yes', note, lead, added_by: me().id, added: now()}});
  }
  if (lead && node.canEdit) {
    edits.push(...chairEdits(node, person));
  } else if (existing && existing.position === 'Co-Chair' && node.canEdit) {
    edits.push(...unchairEdits(node, person));
  }
  const offer = existing ? existing.offer : '';
  if (offering && !offer && node.offersOpen) {
    edits.push({insert: 'MEMBER', row: {group: node.row.chair_offers, person, member: 'yes', added_by: me().id, added: now()}});
  } else if (!offering && offer) {
    edits.push({delete: offer});
  }
  await write(edits);
}

export async function setChair(node, volunteer, on) {
  await write([
    {set: volunteer.id, cells: {lead: on}},
    ...(on ? chairEdits(node, volunteer.person) : unchairEdits(node, volunteer.person)),
    ...(on && volunteer.offer ? [{delete: volunteer.offer}] : []),
  ]);
}

export async function removeSignUp(node, volunteer) {
  await write([
    {delete: volunteer.id},
    ...(volunteer.position === 'Co-Chair' && node.canEdit ? unchairEdits(node, volunteer.person) : []),
    ...(volunteer.offer ? [{delete: volunteer.offer}] : []),
  ]);
}

function closeOffersEdits(node) {
  if (!node.row.chair_offers) {
    return [];
  }
  return [...node.offerRows.map(m => ({delete: m.id})), {set: node.id, cells: {chair_offers: ''}}, {delete: node.row.chair_offers}];
}

export async function setOffers(node, open) {
  if (!open) {
    await write(closeOffersEdits(node));
    return;
  }
  if (node.row.chair_offers) {
    return;
  }
  await write([
    {insert: 'GROUP', as: 'offers', row: {
      kind: 'group', parent: node.id, name: `${node.title} Co-Chair Offers`, status: 'open', join: 'direct', eligible: node.id,
      visible_to: node.row.visible_to || '', posting: 'none', replying: 'none', added_by: me().id,
    }},
    {set: node.id, cells: {chair_offers: '@offers'}},
  ]);
}

export async function deleteActivity(node) {
  await write([...closeOffersEdits(node), ...(node.image ? [{delete: node.image}] : []), {delete: node.id}]);
}

export async function reorder(list, id, before) {
  const rest = list.filter(n => n.id !== id);
  const at = before ? rest.findIndex(n => n.id === before) : rest.length;
  const key = keyBetween(rest[at - 1] && rest[at - 1].row.order, rest[at] && rest[at].row.order);
  await write([{set: id, cells: {order: key}}]);
}

export async function approve(node, yes) {
  await write(yes ? [{set: node.id, cells: {status: 'open'}}] : hideEdits(node, false));
}

export async function saveHeading(heading, fields, owner) {
  const cells = {};
  if (fields.title !== undefined) {
    cells.name = fields.title;
  }
  if (fields.description !== undefined) {
    cells.description = fields.description;
  }
  if (fields.allowAdding !== undefined) {
    cells.adding = addingValue(fields.allowAdding);
  }
  if (fields.showOnMain !== undefined) {
    cells.listed = Boolean(fields.showOnMain);
  }
  if (fields.hidden !== undefined) {
    cells.visible_to = visibility(!fields.hidden);
  }
  if (heading) {
    const edits = [{set: heading.id, cells}];
    if (fields.hidden !== undefined) {
      for (const root of state.model.activities) {
        for (const n of [root, ...descendants(root)]) {
          if (n.category === heading.id) {
            edits.push(...hideEdits(n, !fields.hidden));
          }
        }
      }
    }
    await write(edits);
    return heading.id;
  }
  const ids = await write([{insert: 'GROUP', row: {
    kind: 'category', parent: owner, status: 'open', visible_to: state.model.everyone, members_visible_to: state.model.everyone,
    listed: true, added_by: me().id, ...cells,
  }}]);
  return ids.result[0];
}

export function yearOwner(year) {
  const g = yearGroup(year);
  if (!g) {
    throw new Error(`${year} has no school year yet; copy an event into it first`);
  }
  return g.id;
}

export async function deleteHeading(heading) {
  await write([{delete: heading.id}]);
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

export async function removePicture(photo) {
  await write([{delete: photo}]);
}

export async function setReach(node, on) {
  const rules = state.model.rules.filter(r => r.group === node.id);
  if (!on) {
    await write(rules.map(r => ({delete: r.id})));
    return;
  }
  if (rules.length) {
    return;
  }
  await write([
    {insert: 'RULE', row: {group: node.id, order: 'i', target: node.id, under: true}},
    {insert: 'RULE', row: {group: node.id, order: 'j', target: node.id, under: true, replace_with: 'parents'}},
  ]);
}

export async function saveSetting(key, value) {
  const row = state.model.settingRows.find(s => s.key === key);
  await write([row ? {set: row.id, cells: {value}} : {insert: 'SETTING', row: {app: 'team', key, value}}]);
}

export async function copyToYear(node, year) {
  const edits = [];
  const target = yearEdits(year);
  edits.push(...target.edits);
  const headings = new Map();
  const yearHeading = node.category && category(node.category);
  let under = target.id;
  if (yearHeading) {
    const same = state.model.categories.find(c => c.year === year && c.title === yearHeading.title);
    if (same) {
      under = same.id;
    } else {
      edits.push({insert: 'GROUP', as: 'heading', row: {kind: 'category', parent: target.id, name: yearHeading.title, description: yearHeading.description,
        status: 'open', listed: yearHeading.showOnMain, visible_to: state.model.everyone, members_visible_to: state.model.everyone, order: yearHeading.order || ''}});
      under = '@heading';
    }
  }
  let n = 0;
  const copy = (src, parentRef) => {
    const as = 'a' + n++;
    edits.push({insert: 'GROUP', as, row: {
      kind: 'activity', parent: parentRef, name: src.title, description: src.description, timing: src.own.timing, location: src.location,
      capacity: src.spots ? String(src.spots) : '', join: src.row.join, adding: src.row.adding, status: 'open',
      visible_to: src.row.visible_to, members_visible_to: src.row.members_visible_to, order: src.row.order || '', added_by: me().id,
    }});
    for (const h of src.categories || []) {
      const ref = 'h' + n++;
      headings.set(h.id, ref);
      edits.push({insert: 'GROUP', as: ref, row: {kind: 'category', parent: '@' + as, name: h.title, description: h.description, adding: h.row.adding,
        status: 'open', visible_to: h.row.visible_to, members_visible_to: h.row.members_visible_to, order: h.order || ''}});
    }
    for (const c of src.children) {
      copy(c, c.category && headings.has(c.category) ? '@' + headings.get(c.category) : '@' + as);
    }
  };
  copy(node, under);
  await write(edits);
}
