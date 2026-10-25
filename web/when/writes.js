import {write} from '/rail.js';
import {postPhoto} from '/images.js';
import {keyBetween} from '/order.js';
import {state, me, wordsFor, classroomNames, tagIds} from './state.js';

export async function saveFeed(f, {name, emoji, classrooms, tags}) {
  const search = JSON.stringify(wordsFor(classrooms.length ? classrooms : classroomNames(), tags.length ? tags : tagIds()));
  if (f && f.id) {
    await write([{set: f.id, cells: {name, emoji, search}}]);
    return f;
  }
  const feeds = state.model.feeds;
  const last = feeds.length ? feeds[feeds.length - 1].order : '';
  const [id] = await write([{insert: 'SAVED_SEARCH', row: {person: me().id, name, emoji, order: keyBetween(last || undefined, undefined), default: false, search}}]);
  return {id};
}

export async function makeDefault(f) {
  const home = state.model.user.home;
  const batch = home.id && home.id !== f.id ? [{set: home.id, cells: {default: false}}] : [];
  batch.push({set: f.id, cells: {default: true}});
  await write(batch);
}

export async function orderFeeds(ids) {
  const batch = [];
  let last;
  for (const id of ids) {
    last = keyBetween(last, undefined);
    batch.push({set: id, cells: {order: last}});
  }
  await write(batch);
}

export async function forgetView() {
  const home = state.model.user.home;
  const rooms = state.model.user.classrooms.length ? state.model.user.classrooms : classroomNames();
  const tags = state.model.tags.filter(t => t.default).map(t => t.id);
  await write([{set: home.id, cells: {search: JSON.stringify(wordsFor(rooms, tags))}}]);
}

function moreInformation(source) {
  if (!source) {
    return '';
  }
  return /^https?:\/\//i.test(source) ? `More information: [${source}](${source})` : `More information: ${source}`;
}

function audienceEdits(event, name, classrooms, audience) {
  const everyone = classrooms.length === classroomNames().length;
  const edits = [];
  let group = audience;
  if (!group) {
    edits.push({insert: 'GROUP', as: 'audience', row: {kind: 'group', parent: event, name: name + ' Audience', status: 'open', added_by: me().id, listed: false, visible_to: state.model.everyone, members_visible_to: state.model.everyone, posting: 'none', replying: 'none'}});
    edits.push({set: event, cells: {audience: '@audience'}});
    group = '@audience';
  }
  let order;
  if (everyone) {
    order = keyBetween(undefined, undefined);
    edits.push({insert: 'RULE', row: {group, order, target: state.model.everyone}});
    return edits;
  }
  for (const c of state.model.classrooms.filter(c => classrooms.includes(c.name))) {
    order = keyBetween(order, undefined);
    edits.push({insert: 'RULE', row: {group, order, target: c.id, replace_with: 'household'}});
  }
  return edits;
}

function eventCells(body) {
  return {
    name: body.title, start: body.start, end: body.end || body.start, all_day: !body.start.includes(' '),
    location: body.location, description: [body.description, moreInformation(body.source)].filter(Boolean).join('\n\n'),
  };
}

export async function addEvent(body) {
  const admin = state.model.admin;
  const live = admin || body.sharing !== 'Public';
  const row = {
    ...eventCells(body), kind: 'event', parent: body.category || state.model.misc, status: live ? 'open' : 'pending', added_by: me().id,
    listed: body.sharing === 'Public', visible_to: body.sharing === 'Invite Only' ? '' : state.model.everyone,
    members_visible_to: body.sharing === 'Invite Only' ? '' : state.model.everyone,
  };
  if (body.address) {
    row.slug = body.address;
  }
  const batch = [
    {insert: 'GROUP', as: 'event', row},
    {insert: 'GROUP', as: 'hosts', row: {kind: 'group', name: body.title + ' Managers', status: 'open', added_by: me().id}},
    {set: '@hosts', cells: {managed_by: '@hosts'}},
    {insert: 'MEMBER', row: {group: '@hosts', person: me().id, member: 'yes'}},
    {set: '@event', cells: {managed_by: '@hosts'}},
  ];
  if (body.sharing === 'Invite Only') {
    batch.push({set: '@event', cells: {visible_to: '@event'}});
  }
  if (body.sharing === 'Public') {
    batch.push(...audienceEdits('@event', body.title, body.classrooms, ''));
  }
  const [id] = await write(batch);
  return {id};
}

export async function editEvent(e, body) {
  const cells = eventCells(body);
  if (body.category) {
    cells.parent = body.category;
  }
  if (body.address !== undefined) {
    cells.slug = body.address;
  }
  if (body.sharing !== e.sharing) {
    const open = body.sharing !== 'Invite Only';
    Object.assign(cells, {listed: body.sharing === 'Public', visible_to: open ? state.model.everyone : e.id, members_visible_to: open ? state.model.everyone : ''});
    if (body.sharing === 'Public' && !state.model.admin) {
      cells.status = 'pending';
    } else if (e.pending) {
      cells.status = 'open';
    }
  }
  const batch = [{set: e.id, cells}];
  if (body.classrooms) {
    batch.push(...e.audienceRules.map(id => ({delete: id})), ...audienceEdits(e.id, body.title, body.classrooms, e.groups.audience));
  }
  await write(batch);
}

export async function setStatus(e, status) {
  await write([{set: e.id, cells: {status}}]);
}

export async function moveEvent(e, start, end) {
  await write([{set: e.id, cells: {start, end: end || start, all_day: !start.includes(' ')}}]);
}

export async function setPicture(e, name) {
  if (name) {
    await postPhoto({group: e.id}, name);
  }
  if (e.photo) {
    await write([{delete: e.photo}]);
  }
}

export async function saveCategories(tags) {
  const batch = [];
  let order;
  for (const t of tags) {
    order = keyBetween(order, undefined);
    const cells = {name: t.name, description: t.description || '', default: Boolean(t.default), order};
    const old = state.model.tags.find(x => x.id === t.id);
    if (old) {
      batch.push({set: t.id, cells});
      continue;
    }
    batch.push({insert: 'GROUP', row: {...cells, kind: 'category', slug: t.slug, status: 'open', listed: true, visible_to: state.model.everyone, members_visible_to: state.model.everyone, adding: 'approval'}});
  }
  for (const old of state.model.tags.filter(x => x.slug && !['going', 'waitlisted'].includes(x.id) && !tags.some(t => t.id === old.id))) {
    batch.push({delete: old.id});
  }
  await write(batch);
}
