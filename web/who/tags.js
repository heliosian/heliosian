import {state, model, tags, lists, byId, tagKey, viewerId, write, tagPeople, tagManagers, groupPath} from './state.js';
import {loadLastTag, saveLastTag, loadTagUsage, recordTagUsage} from './storage.js';
import {firstName} from './dom.js';
import {el, svg} from '/elements.js';
import {appOrigin} from '/appswitch.js';
import {photoOrInitials, personPhotoUrl} from './people.js';
import {clampFilterPanel} from '/rules.js';
import {navigate, render as renderPage} from '/router.js';

export function tagKeys() {
  return Object.keys(tags).sort((a, b) => tags[a].name.localeCompare(tags[b].name));
}

export function tagOf(key) {
  return tags[key];
}

export function sharedTag(key) {
  return Boolean(tags[key]) && tagManagers(tags[key]).length > 1;
}

export function otherManagers(key) {
  return tags[key] ? tagManagers(tags[key]).filter(id => id !== viewerId()) : [];
}

export function tagLabel(key) {
  if (tags[key]) {
    return tags[key].name;
  }
  return lists[key] ? lists[key].name : key;
}

export function tagHref(key) {
  return groupPath(key);
}

const listIcons = {party: 'party', activity: 'activity', event: 'calendar', room: 'classrooms', group: 'people', admins: 'gear'};

export function listKeys() {
  return Object.keys(lists).sort((a, b) => lists[a].name.localeCompare(lists[b].name));
}

export function listOf(key) {
  return lists[key];
}

export function listSections() {
  const keys = listKeys();
  const backstage = key => lists[key].run && !lists[key].member && lists[key].kind !== 'event';
  const shown = keys.filter(key => !backstage(key));
  return {
    running: shown.filter(key => lists[key].run && !lists[key].start),
    upcoming: shown.filter(key => lists[key].start).sort((a, b) => lists[a].start.localeCompare(lists[b].start)),
    joined: shown.filter(key => !lists[key].run && !lists[key].start),
    managing: keys.filter(backstage),
  };
}

export function listIcon(key) {
  return listIcons[lists[key].kind];
}

const listSources = {
  party: {app: 'celebrate', name: 'Celebrate', path: '/p/', thing: 'party'},
  activity: {app: 'team', name: 'HCA-Team', path: '/v/', thing: 'activity'},
  event: {app: 'when', name: 'Helios When', path: '/e/', thing: 'event'},
  group: {app: 'loop', name: 'Helios Loop', path: '/groups/', thing: 'email list'},
};

export function listSource(key) {
  const source = lists[key] && listSources[lists[key].kind];
  if (!source) {
    return null;
  }
  return {...source, href: appOrigin(source.app) + source.path + encodeURIComponent(lists[key].slug)};
}

export function listApp(key) {
  const kind = lists[key] && lists[key].kind;
  return listSources[kind] ? listSources[kind].app : 'who';
}

export function members(key) {
  if (tags[key]) {
    return tagPeople(tags[key]);
  }
  return lists[key] ? lists[key].people : [];
}

export function selectedGuests() {
  if (state.filterTags.size !== 1 || state.filterGrades.size || state.filterClassrooms.size || state.filterRoles.size) {
    return [];
  }
  const list = lists[[...state.filterTags][0]];
  if (!list) {
    return [];
  }
  return list.guests.filter(g => g.name_show.toLowerCase().includes(state.q));
}

export function tagFacetOptions() {
  return [
    ...tagKeys().map(key => ({value: key, label: tags[key].name, icon: sharedTag(key) ? 'families' : undefined})),
    ...listKeys().map(key => ({value: key, label: lists[key].name, icon: listIcon(key)})),
  ];
}

let pageChanged = () => {};
let chromeChanged = () => {};

export function onTagsChange(fn) {
  pageChanged = fn;
}

export function onTagsChangeChrome(fn) {
  chromeChanged = fn;
}

function changed() {
  chromeChanged();
  pageChanged();
}

function closing(t) {
  const out = [{set: t.id, cells: {status: 'closed'}}];
  if (t.ownManagers) {
    out.push({set: t.managersGroup, cells: {status: 'closed'}});
  }
  return out;
}

export async function deleteTag(key) {
  try {
    await write(closing(tags[key]));
  } catch (err) {
    alert(err.message);
    return false;
  }
  delete tags[key];
  changed();
  return true;
}

export async function renameTag(key, to) {
  try {
    await write([{set: tags[key].id, cells: {name: to}}]);
  } catch (err) {
    alert(err.message);
    return false;
  }
  tags[key].name = to;
  changed();
  return true;
}

async function makeTag(name, people) {
  const me = viewerId();
  const batch = [
    {insert: 'GROUP', as: 'managers', row: {kind: 'group', name: `${name} Managers`, status: 'open', listed: true, added_by: me}},
    {set: '@managers', cells: {managed_by: '@managers'}},
    {insert: 'MEMBER', row: {group: '@managers', person: me, member: 'yes'}},
    {insert: 'GROUP', as: 'tag', row: {kind: 'group', name, status: 'open', listed: true, managed_by: '@managers', added_by: me}},
    ...people.map(person => ({insert: 'MEMBER', row: {group: '@tag', person, member: 'yes'}})),
  ];
  const {result} = await write(batch);
  const memberRows = {};
  people.forEach((person, i) => {
    memberRows[person] = result[5 + i];
  });
  const key = tagKey(result[3]);
  tags[key] = {id: result[3], name, memberRows, managerRows: {[me]: result[2]}, managersGroup: result[0], ownManagers: true};
  return key;
}

export async function copyTag(key, to) {
  let made;
  try {
    made = await makeTag(to, [...members(key)]);
  } catch (err) {
    alert(err.message);
    return null;
  }
  changed();
  return made;
}

function tagKeysByRecency() {
  const usage = loadTagUsage();
  return tagKeys().sort((a, b) => (usage[b] || 0) - (usage[a] || 0));
}

export function tagsOf(id) {
  return tagKeys().filter(key => tags[key].memberRows[id]);
}

function isTagged(id) {
  return tagsOf(id).length > 0;
}

async function setTag(id, key, on) {
  const t = tags[key];
  if (on) {
    saveLastTag(key);
    recordTagUsage(key);
  }
  try {
    if (on && !t.memberRows[id]) {
      const {result} = await write([{insert: 'MEMBER', row: {group: t.id, person: id, member: 'yes'}}]);
      t.memberRows[id] = result[0];
    } else if (!on && t.memberRows[id]) {
      const last = tagPeople(t).length === 1;
      await write([{delete: t.memberRows[id]}, ...(last ? closing(t) : [])]);
      delete t.memberRows[id];
      if (last) {
        delete tags[key];
      }
    }
  } catch (err) {
    alert(err.message);
  }
  changed();
}

async function addToNewTag(id, name) {
  let key;
  try {
    key = await makeTag(name, [id]);
  } catch (err) {
    alert(err.message);
    return null;
  }
  saveLastTag(key);
  recordTagUsage(key);
  changed();
  return key;
}

export async function shareTag(key, manager, on) {
  const t = tags[key];
  try {
    if (on) {
      const {result} = await write([{insert: 'MEMBER', row: {group: t.managersGroup, person: manager, member: 'yes'}}]);
      t.managerRows[manager] = result[0];
    } else {
      await write([{delete: t.managerRows[manager]}]);
      delete t.managerRows[manager];
    }
  } catch (err) {
    alert(err.message);
    return false;
  }
  return true;
}

export async function leaveTag(key) {
  const t = tags[key];
  try {
    await write([{delete: t.managerRows[viewerId()]}]);
  } catch (err) {
    alert(err.message);
    return false;
  }
  delete tags[key];
  changed();
  return true;
}

export function manageControl(key, onManagersChange) {
  const name = tags[key].name;
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button facet-button tag-manage');
  button.type = 'button';
  button.append(svg('gear'), el('span', '', 'Manage'), svg('chevron-down'));

  const menu = el('div', 'filter-panel manage-menu');
  menu.hidden = true;
  const share = el('div', 'filter-panel share-panel');
  share.hidden = true;
  const open = panel => {
    menu.hidden = panel !== menu;
    share.hidden = panel !== share;
    button.classList.toggle('open', !!panel);
    if (panel) {
      clampFilterPanel(wrap, panel);
    }
  };
  button.addEventListener('click', () => {
    open(menu.hidden && share.hidden ? menu : null);
  });

  const item = (icon, label, title, onClick) => {
    const row = el('button', 'manage-item');
    row.type = 'button';
    row.title = title;
    row.append(svg(icon), el('span', '', label));
    row.addEventListener('click', onClick);
    menu.append(row);
    return row;
  };
  item('edit', 'Edit name', 'Give this tag a new name', async () => {
    open(null);
    const to = (prompt('New name for the tag', name) || '').trim().slice(0, 40);
    if (!to || to === name) {
      return;
    }
    if (await renameTag(key, to)) {
      renderPage();
    }
  });
  item('copy', 'Duplicate', 'Make a new tag with the same people', async () => {
    open(null);
    const to = (prompt('Name for the copy', `${name} copy`) || '').trim().slice(0, 40);
    if (!to) {
      return;
    }
    const made = await copyTag(key, to);
    if (made) {
      navigate(tagHref(made));
    }
  });
  const shareItem = item('families', 'Share', 'Let others manage this tag with you', () => {
    open(share);
    input.focus();
  });
  if (sharedTag(key)) {
    item('close', 'Leave', 'Stop managing this tag - the others keep it', async () => {
      open(null);
      if (!confirm(`Leave "${name}"? You'll no longer see or manage it unless someone shares it with you again.`)) {
        return;
      }
      if (await leaveTag(key)) {
        navigate('/people');
      }
    }).classList.add('manage-item-danger');
  }
  item('trash', 'Delete tag', 'Delete this tag - nobody is removed from the directory, just untagged', async () => {
    open(null);
    const count = members(key).length;
    const who = count === 1 ? 'the one person' : `all ${count} people`;
    if (!confirm(`Delete the tag "${name}"? It comes off ${who} in it, for everyone who manages it.`)) {
      return;
    }
    if (await deleteTag(key)) {
      navigate('/people');
    }
  }).classList.add('manage-item-danger');

  const head = el('div', 'family-head');
  head.append(svg('families'));
  const text = el('div');
  text.append(el('div', 'family-title', 'Share this tag'),
    el('div', 'family-desc', 'Anyone you add can tag and untag people, rename, share and delete it, just as you can.'));
  head.append(text);
  share.append(head);

  const managers = el('div', 'share-managers');
  const searchBox = el('div', 'share-search');
  const input = el('input');
  input.placeholder = 'Add someone by name';
  input.maxLength = 60;
  const results = el('div', 'share-results');
  searchBox.append(input, results);
  share.append(managers, searchBox);

  const updateLabel = () => {
    const n = otherManagers(key).length;
    shareItem.querySelector('span').textContent = n ? `Share (${n})` : 'Share';
  };
  const paintManagers = () => {
    managers.replaceChildren();
    for (const id of otherManagers(key)) {
      const p = byId[id];
      if (!p) {
        continue;
      }
      const row = el('div', 'share-manager');
      row.append(photoOrInitials(personPhotoUrl(p), p.name_show, 'share-avatar'), el('span', '', p.name_show));
      const remove = el('button', 'share-remove');
      remove.type = 'button';
      remove.title = `Stop ${firstName(p.name_show)} managing this tag`;
      remove.append(svg('close'));
      remove.addEventListener('click', async () => {
        if (await shareTag(key, id, false)) {
          paintManagers();
          updateLabel();
          onManagersChange();
        }
      });
      row.append(remove);
      managers.append(row);
    }
    managers.hidden = !managers.children.length;
  };
  const paintResults = () => {
    const q = input.value.trim().toLowerCase();
    results.replaceChildren();
    if (!q) {
      return;
    }
    const managing = tagManagers(tags[key]);
    const matches = model.people
      .filter(p => !managing.includes(p.id))
      .filter(p => p.name_show.toLowerCase().includes(q))
      .slice(0, 6);
    if (!matches.length) {
      results.append(el('div', 'share-empty', 'No one by that name.'));
      return;
    }
    for (const p of matches) {
      const row = el('button', 'share-result');
      row.type = 'button';
      row.append(photoOrInitials(personPhotoUrl(p), p.name_show, 'share-avatar'), el('span', '', p.name_show), svg('plus'));
      row.addEventListener('click', async () => {
        if (await shareTag(key, p.id, true)) {
          input.value = '';
          paintResults();
          paintManagers();
          updateLabel();
          onManagersChange();
        }
      });
      results.append(row);
    }
  };
  input.addEventListener('input', paintResults);

  paintManagers();
  updateLabel();
  wrap.append(button, menu, share);
  return wrap;
}

function tagMenu(id, onChange) {
  const menu = el('div', 'card-menu tag-menu');
  menu.hidden = true;
  menu.focusTag = key => {
    menu.querySelector(`.tag-option input[data-tag-key="${CSS.escape(key)}"]`)?.focus();
  };
  const render = () => {
    const typed = menu.querySelector('.tag-new input')?.value || '';
    menu.replaceChildren();
    for (const key of tagKeysByRecency()) {
      const row = el('label', 'tag-option');
      const box = el('input');
      box.type = 'checkbox';
      box.dataset.tagKey = key;
      box.checked = Boolean(tags[key].memberRows[id]);
      box.addEventListener('change', async () => {
        await setTag(id, key, box.checked);
        render();
        onChange();
        menu.focusTag(key);
      });
      const text = el('span', '', tags[key].name);
      if (sharedTag(key)) {
        text.append(el('small', 'tag-option-note', 'shared'));
      }
      row.append(text, box);
      menu.append(row);
    }
    const form = el('form', 'tag-new');
    const input = el('input');
    input.placeholder = 'New tag';
    input.maxLength = 40;
    input.value = typed;
    form.append(input);
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const name = input.value.trim();
      if (!name) {
        return;
      }
      input.value = '';
      await addToNewTag(id, name);
      render();
      onChange();
    });
    menu.append(form);
  };
  render();
  // The menu sits inside the card's <a>; preventDefault on a checkbox would cancel
  // its own toggle, so only other targets are kept from following the link.
  menu.addEventListener('click', e => {
    e.stopPropagation();
    if (!e.target.closest('.tag-option')) {
      e.preventDefault();
    }
  });
  menu.addEventListener('keydown', e => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') {
      return;
    }
    const boxes = [...menu.querySelectorAll('.tag-option input[type="checkbox"]')];
    const current = boxes.indexOf(document.activeElement);
    if (current === -1) {
      return;
    }
    e.preventDefault();
    const delta = e.key === 'ArrowDown' ? 1 : -1;
    boxes[(current + delta + boxes.length) % boxes.length].focus();
  });
  menu.refreshTags = render;
  return menu;
}

function mostRecentTag() {
  const existing = tagKeys();
  const last = loadLastTag();
  return existing.includes(last) ? last : existing[0];
}

export function tagControl(id, wrapClass, buttonClass, onChange) {
  const wrap = el('div', wrapClass);
  const button = el('button', buttonClass + (isTagged(id) ? ' active' : ''));
  button.title = 'Tags';
  button.append(svg('tag'));
  const menu = tagMenu(id, () => {
    button.classList.toggle('active', isTagged(id));
    onChange();
  });
  button.addEventListener('click', async e => {
    e.preventDefault();
    e.stopPropagation();
    const opening = menu.hidden;
    menu.hidden = !menu.hidden;
    if (!opening) {
      return;
    }
    if (isTagged(id)) {
      menu.refreshTags();
      menu.querySelector('.tag-option input')?.focus();
      return;
    }
    let tag = mostRecentTag();
    if (tag) {
      await setTag(id, tag, true);
    } else {
      tag = await addToNewTag(id, 'My List');
    }
    menu.refreshTags();
    button.classList.toggle('active', isTagged(id));
    onChange();
    menu.focusTag(tag);
  });
  if (window.matchMedia('(hover: hover) and (pointer: fine)').matches) {
    let closeTimer = null;
    wrap.addEventListener('mouseenter', () => {
      clearTimeout(closeTimer);
      if (menu.hidden) {
        menu.refreshTags();
      }
      menu.hidden = false;
    });
    wrap.addEventListener('mouseleave', () => {
      closeTimer = setTimeout(() => {
        menu.hidden = true;
      }, 150);
    });
  }
  wrap.append(button, menu);
  return wrap;
}
