import {state, tags, shared, lists, byId, tagKey} from './state.js';
import {loadLastTag, saveLastTag, loadTagUsage, recordTagUsage} from './storage.js';
import {firstName} from './dom.js';
import {el, svg} from '/elements.js';
import {appOrigin} from '/appswitch.js';
import {act, create, remove} from '/data.js';
import {photoOrInitials, personPhotoUrl} from './people.js';
import {clampFilterPanel} from '/rules.js';

export function tagKeys() {
  return Object.keys(tags).sort((a, b) => tags[a].name.localeCompare(tags[b].name));
}

export function sharedKeys() {
  return Object.keys(shared).sort((a, b) => shared[a].name.localeCompare(shared[b].name) || shared[a].ownerName.localeCompare(shared[b].ownerName));
}

export function sharedOf(key) {
  return shared[key];
}

function tagOf(key) {
  return tags[key] || shared[key];
}

export function managersOf(key) {
  return tagOf(key) ? tagOf(key).managers : [];
}

export function tagLabel(key) {
  if (tagOf(key)) {
    return tagOf(key).name;
  }
  return lists[key] ? lists[key].name : key;
}

export function tagHref(key) {
  if (tagOf(key)) {
    return '/people?tag=' + encodeURIComponent(tagOf(key).id);
  }
  return '/people?list=' + encodeURIComponent(key);
}

const listIcons = {party: 'party', activity: 'activity', room: 'classrooms', group: 'people'};

export function listKeys() {
  return Object.keys(lists).filter(key => !lists[key].archived).sort((a, b) => lists[a].name.localeCompare(lists[b].name));
}

export function listLabel(key) {
  return tagLabel(key);
}

export function listIcon(key) {
  return listIcons[lists[key].kind];
}

const listSources = {
  party: {app: 'celebrate', name: 'Celebrate', path: '/parties/', thing: 'party'},
  activity: {app: 'team', name: 'HCA-Team', path: '/activities/', thing: 'activity'},
  group: {app: 'loop', name: 'Helios Loop', path: '/groups/', thing: 'email list'},
};

export function listSource(key) {
  const source = lists[key] && listSources[lists[key].kind];
  if (!source) {
    return null;
  }
  const page = lists[key].kind === 'group' ? lists[key].slug : key.slice(key.indexOf(':') + 1);
  return {...source, href: appOrigin(source.app) + source.path + encodeURIComponent(page)};
}

export function listApp(key) {
  const kind = lists[key] && lists[key].kind;
  return kind === 'room' ? 'who' : listSources[kind] ? listSources[kind].app : 'who';
}

export function members(key) {
  if (tagOf(key)) {
    return tagOf(key).people;
  }
  return lists[key] ? lists[key].people : [];
}

export function selectedGuests() {
  if (state.filterTags.size !== 1 || state.filterGrades.size || state.filterClassrooms.size || state.filterRoles.size) {
    return [];
  }
  const list = lists[[...state.filterTags][0]];
  if (!list || (list.kind !== 'party' && list.kind !== 'group')) {
    return [];
  }
  return list.guests.filter(g => g.name.toLowerCase().includes(state.q) || (g.email || '').toLowerCase().includes(state.q));
}

export function tagFacetOptions() {
  return [
    ...tagKeys().map(key => ({value: key, label: tags[key].name})),
    ...sharedKeys().map(key => ({value: key, label: shared[key].name, icon: 'families'})),
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

export async function deleteTag(key) {
  try {
    await remove('tags', tags[key].id);
  } catch (err) {
    alert(err.message);
    return false;
  }
  delete tags[key];
  chromeChanged();
  pageChanged();
  return true;
}

export async function renameTag(key, to) {
  try {
    await act('tags', tags[key].id, 'rename', {name: to});
  } catch (err) {
    alert(err.message);
    return false;
  }
  tags[key].name = to;
  chromeChanged();
  pageChanged();
  return true;
}

function ownTag(id, name, people) {
  const me = state.model.user;
  return {id, owner: me.id, ownerName: me.name, name, people, managers: [], me: {mine: true}};
}

export async function copyTag(key, to) {
  let saved;
  try {
    saved = await act('tags', tagOf(key).id, 'copy', {name: to});
  } catch (err) {
    alert(err.message);
    return null;
  }
  tags[tagKey(saved.id)] = ownTag(saved.id, to, [...members(key)]);
  chromeChanged();
  pageChanged();
  return saved.id;
}

function tagKeysByRecency() {
  const usage = loadTagUsage();
  return tagKeys().sort((a, b) => (usage[b] || 0) - (usage[a] || 0));
}

export function tagsOf(id) {
  return [...tagKeys(), ...sharedKeys()].filter(key => tagOf(key).people.includes(id));
}

function isTagged(id) {
  return tagsOf(id).length > 0;
}

async function setTag(id, key, on) {
  const t = tagOf(key);
  t.people = on ? (t.people.includes(id) ? t.people : [...t.people, id]) : t.people.filter(e => e !== id);
  if (!t.people.length) {
    delete tags[key];
    delete shared[key];
  }
  if (on) {
    saveLastTag(key);
    recordTagUsage(key);
  }
  chromeChanged();
  pageChanged();
  try {
    await act('tags', t.id, on ? 'add' : 'remove', {person: id});
  } catch (err) {
    alert(err.message);
  }
}

async function addToNewTag(id, name) {
  let saved;
  try {
    saved = await create('tags', {name, person: id});
  } catch (err) {
    alert(err.message);
    return null;
  }
  const key = tagKey(saved.id);
  if (tags[key]) {
    tags[key].people = tags[key].people.includes(id) ? tags[key].people : [...tags[key].people, id];
  } else {
    tags[key] = ownTag(saved.id, name, [id]);
  }
  saveLastTag(key);
  recordTagUsage(key);
  chromeChanged();
  pageChanged();
  return key;
}

export async function shareTag(key, manager, on) {
  try {
    await act('tags', tags[key].id, on ? 'share' : 'unshare', {person: manager});
  } catch (err) {
    alert(err.message);
    return false;
  }
  const current = tags[key].managers.filter(e => e !== manager);
  if (on) {
    current.push(manager);
  }
  tags[key].managers = current;
  return true;
}

export function manageControl(key, onManagersChange) {
  const isShared = !!shared[key];
  const name = tagOf(key).name;
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
  if (!isShared) {
    item('edit', 'Edit name', 'Give this tag a new name', async () => {
      open(null);
      const to = (prompt('New name for the tag', name) || '').trim().slice(0, 40);
      if (!to || to === name) {
        return;
      }
      if (await renameTag(key, to)) {
        location.href = tagHref(key);
      }
    });
  }
  item('copy', 'Duplicate', 'Make a new tag of your own with the same people', async () => {
    open(null);
    const suggested = isShared && !tagKeys().some(k => tags[k].name === name) ? name : `${name} copy`;
    const to = (prompt('Name for the copy', suggested) || '').trim().slice(0, 40);
    if (!to || (!isShared && to === name)) {
      return;
    }
    const made = await copyTag(key, to);
    if (made) {
      location.href = tagHref(tagKey(made));
    }
  });
  let shareItem = null;
  if (isShared) {
    const t = shared[key];
    item('close', 'Leave', `Stop managing this tag - it stays ${t.ownerName}'s`, async () => {
      open(null);
      if (!confirm(`Leave "${name}"? You'll no longer see or manage it unless ${t.ownerName} shares it again.`)) {
        return;
      }
      if (await leaveTag(key)) {
        location.href = '/people';
      }
    }).classList.add('manage-item-danger');
    wrap.append(button, menu);
    return wrap;
  }
  shareItem = item('families', 'Share', 'Let others manage this tag with you', () => {
    open(share);
    input.focus();
  });
  item('trash', 'Delete tag', 'Delete this tag - nobody is removed from the directory, just untagged', async () => {
    open(null);
    const count = members(key).length;
    const who = count === 1 ? 'the one person' : `all ${count} people`;
    if (!confirm(`Delete the tag "${name}"? It comes off ${who} in it. This can't be undone.`)) {
      return;
    }
    if (await deleteTag(key)) {
      location.href = '/people';
    }
  }).classList.add('manage-item-danger');

  const head = el('div', 'family-head');
  head.append(svg('families'));
  const text = el('div');
  text.append(el('div', 'family-title', 'Share this tag'),
    el('div', 'family-desc', 'Anyone you add can tag and untag people on it, just as you can. It stays yours to delete.'));
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
    const n = managersOf(key).length;
    shareItem.querySelector('span').textContent = n ? `Share (${n})` : 'Share';
  };
  const paintManagers = () => {
    managers.replaceChildren();
    for (const id of managersOf(key)) {
      const p = byId[id];
      if (!p) {
        continue;
      }
      const row = el('div', 'share-manager');
      row.append(photoOrInitials(personPhotoUrl(p), p.fullName, 'share-avatar'), el('span', '', p.fullName));
      const remove = el('button', 'share-remove');
      remove.type = 'button';
      remove.title = `Stop ${firstName(p.fullName)} managing this tag`;
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
    const me = state.model.user.id;
    const matches = state.model.people
      .filter(p => p.id !== me && !managersOf(key).includes(p.id))
      .filter(p => p.fullName.toLowerCase().includes(q) || (p.email || '').toLowerCase().includes(q))
      .slice(0, 6);
    if (!matches.length) {
      results.append(el('div', 'share-empty', 'No one by that name.'));
      return;
    }
    for (const p of matches) {
      const row = el('button', 'share-result');
      row.type = 'button';
      row.append(photoOrInitials(personPhotoUrl(p), p.fullName, 'share-avatar'), el('span', '', p.fullName), svg('plus'));
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

export async function leaveTag(key) {
  try {
    await act('tags', shared[key].id, 'leave');
  } catch (err) {
    alert(err.message);
    return false;
  }
  delete shared[key];
  chromeChanged();
  pageChanged();
  return true;
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
    const option = (key, label, note) => {
      const row = el('label', 'tag-option');
      const box = el('input');
      box.type = 'checkbox';
      box.dataset.tagKey = key;
      box.checked = members(key).includes(id);
      box.addEventListener('change', async () => {
        await setTag(id, key, box.checked);
        render();
        onChange();
        menu.focusTag(key);
      });
      const text = el('span', '', label);
      if (note) {
        text.append(el('small', 'tag-option-note', note));
      }
      row.append(text, box);
      menu.append(row);
    };
    for (const key of tagKeysByRecency()) {
      option(key, tags[key].name, '');
    }
    for (const key of sharedKeys()) {
      option(key, shared[key].name, `${firstName(shared[key].ownerName)}'s`);
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
  const existing = [...tagKeys(), ...sharedKeys()];
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
