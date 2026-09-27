import {state, tags, tagManagers, shared, lists, byEmail} from './state.js';
import {loadLastTag, saveLastTag, loadTagUsage, recordTagUsage} from './storage.js';
import {el, svg, firstName} from './dom.js';
import {appOrigin} from '/toolbar.js';
import {photoOrInitials, personPhotoUrl} from './people.js';
import {clampFilterPanel} from './filters.js';

export function tagNames() {
  return Object.keys(tags).sort((a, b) => a.localeCompare(b));
}

export function sharedKey(owner, name) {
  return `shared:${owner}:${name}`;
}

export function isSharedKey(key) {
  return Object.hasOwn(shared, key);
}

export function sharedKeys() {
  return Object.keys(shared).sort((a, b) => shared[a].name.localeCompare(shared[b].name) || shared[a].owner.localeCompare(shared[b].owner));
}

export function sharedOf(key) {
  return shared[key];
}

export function managersOf(name) {
  return tagManagers[name] || [];
}

export function tagLabel(key) {
  if (shared[key]) {
    return shared[key].name;
  }
  return lists[key] ? lists[key].name : key;
}

export function tagHref(key) {
  if (shared[key]) {
    return '/people?shared=' + encodeURIComponent(`${shared[key].owner}:${shared[key].name}`);
  }
  if (lists[key]) {
    return '/people?list=' + encodeURIComponent(key);
  }
  return '/people?tag=' + encodeURIComponent(key);
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
  group: {app: 'loop', name: 'Helios Loop', path: '/groups/', thing: 'group'},
};

export function listSource(key) {
  const source = lists[key] && listSources[lists[key].kind];
  if (!source) {
    return null;
  }
  const id = key.slice(key.indexOf(':') + 1);
  return {...source, href: appOrigin(source.app) + source.path + encodeURIComponent(id)};
}

export function listApp(key) {
  const kind = lists[key] && lists[key].kind;
  return kind === 'room' ? 'who' : listSources[kind] ? listSources[kind].app : 'who';
}

export function members(key) {
  if (shared[key]) {
    return shared[key].people;
  }
  return lists[key] ? lists[key].people : tags[key] || [];
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
    ...tagNames(),
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

export async function deleteTag(name) {
  const form = new FormData();
  form.append('tag', name);
  const res = await fetch('/api/directory/tag-delete', {method: 'POST', body: form});
  if (!res.ok) {
    alert(await res.text());
    return false;
  }
  delete tags[name];
  chromeChanged();
  pageChanged();
  return true;
}

export async function renameTag(from, to) {
  const form = new FormData();
  form.append('tag', from);
  form.append('name', to);
  const res = await fetch('/api/directory/tag-rename', {method: 'POST', body: form});
  if (!res.ok) {
    alert(await res.text());
    return false;
  }
  tags[to] = tags[from];
  delete tags[from];
  if (tagManagers[from]) {
    tagManagers[to] = tagManagers[from];
    delete tagManagers[from];
  }
  chromeChanged();
  pageChanged();
  return true;
}

export async function copyTag(key, to) {
  const form = new FormData();
  if (shared[key]) {
    form.append('tag', shared[key].name);
    form.append('owner', shared[key].owner);
  } else {
    form.append('tag', key);
  }
  form.append('name', to);
  const res = await fetch('/api/directory/tag-copy', {method: 'POST', body: form});
  if (!res.ok) {
    alert(await res.text());
    return false;
  }
  tags[to] = [...members(key)];
  chromeChanged();
  pageChanged();
  return true;
}

function tagNamesByRecency() {
  const usage = loadTagUsage();
  return tagNames().sort((a, b) => (usage[b] || 0) - (usage[a] || 0));
}

export function tagsOf(email) {
  return [...tagNames().filter(name => tags[name].includes(email)), ...sharedKeys().filter(key => shared[key].people.includes(email))];
}

function isTagged(email) {
  return tagsOf(email).length > 0;
}

async function setTag(email, key, on) {
  const form = new FormData();
  form.append('person', email);
  form.append('on', on ? '1' : '0');
  if (shared[key]) {
    const t = shared[key];
    t.people = on ? (t.people.includes(email) ? t.people : [...t.people, email].sort()) : t.people.filter(e => e !== email);
    form.append('tag', t.name);
    form.append('owner', t.owner);
  } else {
    const people = tags[key] || [];
    if (on) {
      tags[key] = people.includes(email) ? people : [...people, email];
    } else {
      tags[key] = people.filter(e => e !== email);
      if (!tags[key].length) {
        delete tags[key];
      }
    }
    form.append('tag', key);
  }
  if (on) {
    saveLastTag(key);
    recordTagUsage(key);
  }
  chromeChanged();
  pageChanged();
  const res = await fetch('/api/directory/tag', {method: 'POST', body: form});
  if (!res.ok) {
    alert(await res.text());
  }
}

export async function shareTag(name, manager, on) {
  const form = new FormData();
  form.append('tag', name);
  form.append('manager', manager);
  form.append('on', on ? '1' : '0');
  const res = await fetch('/api/directory/tag-share', {method: 'POST', body: form});
  if (!res.ok) {
    alert(await res.text());
    return false;
  }
  const current = (tagManagers[name] || []).filter(e => e !== manager);
  if (on) {
    current.push(manager);
    current.sort();
  }
  if (current.length) {
    tagManagers[name] = current;
  } else {
    delete tagManagers[name];
  }
  return true;
}

export function manageControl(key, onManagersChange) {
  const isShared = !!shared[key];
  const name = isShared ? shared[key].name : key;
  const wrap = el('div', 'filter-wrap');
  const button = el('button', 'filter-button facet-button tag-manage');
  button.type = 'button';
  button.append(svg('gear'), el('span', '', 'Manage'), svg('chevron'));

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
    item('pencil', 'Edit name', 'Give this tag a new name', async () => {
      open(null);
      const to = (prompt('New name for the tag', name) || '').trim().slice(0, 40);
      if (!to || to === name) {
        return;
      }
      if (await renameTag(name, to)) {
        location.href = '/people?tag=' + encodeURIComponent(to);
      }
    });
  }
  item('copy', 'Duplicate', 'Make a new tag of your own with the same people', async () => {
    open(null);
    const suggested = isShared && !tags[name] ? name : `${name} copy`;
    const to = (prompt('Name for the copy', suggested) || '').trim().slice(0, 40);
    if (!to || (!isShared && to === name)) {
      return;
    }
    if (await copyTag(key, to)) {
      location.href = '/people?tag=' + encodeURIComponent(to);
    }
  });
  let shareItem = null;
  if (isShared) {
    const t = shared[key];
    item('x', 'Leave', `Stop managing this tag - it stays ${t.ownerName}'s`, async () => {
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
    const count = members(name).length;
    const who = count === 1 ? 'the one person' : `all ${count} people`;
    if (!confirm(`Delete the tag "${name}"? It comes off ${who} in it. This can't be undone.`)) {
      return;
    }
    if (await deleteTag(name)) {
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
    const n = managersOf(name).length;
    shareItem.querySelector('span').textContent = n ? `Share (${n})` : 'Share';
  };
  const paintManagers = () => {
    managers.replaceChildren();
    for (const email of managersOf(name)) {
      const p = byEmail[email];
      if (!p) {
        continue;
      }
      const row = el('div', 'share-manager');
      row.append(photoOrInitials(personPhotoUrl(p), p.fullName, 'share-avatar'), el('span', '', p.fullName));
      const remove = el('button', 'share-remove');
      remove.type = 'button';
      remove.title = `Stop ${firstName(p.fullName)} managing this tag`;
      remove.append(svg('x'));
      remove.addEventListener('click', async () => {
        if (await shareTag(name, email, false)) {
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
    const me = state.model.user.email;
    const matches = state.model.people
      .filter(p => p.email !== me && !managersOf(name).includes(p.email))
      .filter(p => p.fullName.toLowerCase().includes(q) || p.email.toLowerCase().includes(q))
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
        if (await shareTag(name, p.email, true)) {
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
  const t = shared[key];
  const form = new FormData();
  form.append('owner', t.owner);
  form.append('tag', t.name);
  const res = await fetch('/api/directory/tag-leave', {method: 'POST', body: form});
  if (!res.ok) {
    alert(await res.text());
    return false;
  }
  delete shared[key];
  chromeChanged();
  pageChanged();
  return true;
}

function tagMenu(email, onChange) {
  const menu = el('div', 'card-menu tag-menu');
  menu.hidden = true;
  menu.focusTag = name => {
    menu.querySelector(`.tag-option input[data-tag-name="${CSS.escape(name)}"]`)?.focus();
  };
  const render = () => {
    const typed = menu.querySelector('.tag-new input')?.value || '';
    menu.replaceChildren();
    const option = (key, label, note) => {
      const row = el('label', 'tag-option');
      const box = el('input');
      box.type = 'checkbox';
      box.dataset.tagName = key;
      box.checked = members(key).includes(email);
      box.addEventListener('change', async () => {
        await setTag(email, key, box.checked);
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
    for (const name of tagNamesByRecency()) {
      option(name, name, '');
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
      await setTag(email, name, true);
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
  const existing = [...tagNames(), ...sharedKeys()];
  if (!existing.length) {
    return 'My List';
  }
  const last = loadLastTag();
  return existing.includes(last) ? last : existing[0];
}

export function tagControl(email, wrapClass, buttonClass, onChange) {
  const wrap = el('div', wrapClass);
  const button = el('button', buttonClass + (isTagged(email) ? ' active' : ''));
  button.title = 'Tags';
  button.append(svg('tag'));
  const menu = tagMenu(email, () => {
    button.classList.toggle('active', isTagged(email));
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
    if (isTagged(email)) {
      menu.refreshTags();
      menu.querySelector('.tag-option input')?.focus();
      return;
    }
    const tag = mostRecentTag();
    await setTag(email, tag, true);
    menu.refreshTags();
    button.classList.toggle('active', isTagged(email));
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
