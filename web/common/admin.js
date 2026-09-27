import {el} from '/toolbar.js';
import {api} from '/api.js';
import {createPersonPicker} from '/picker.js';

let remembered = '';

function closeIcon() {
  const ns = 'http://www.w3.org/2000/svg';
  const icon = document.createElementNS(ns, 'svg');
  icon.setAttribute('viewBox', '0 0 24 24');
  const path = document.createElementNS(ns, 'path');
  path.setAttribute('d', 'M18 6 6 18M6 6l12 12');
  icon.append(path);
  return icon;
}

function header(appName, email) {
  const bar = el('header');
  const brand = el('a', 'brand-link');
  brand.href = '/';
  brand.setAttribute('data-link', '');
  const tile = el('img', 'admin-tile');
  tile.src = '/brand/icon-192.png';
  tile.alt = appName;
  brand.append(tile, el('span', '', 'Admin'));
  const right = el('span', 'right');
  const close = el('a', 'admin-close');
  close.href = '/';
  close.setAttribute('data-link', '');
  close.setAttribute('aria-label', 'Close admin tools');
  close.append(closeIcon());
  right.append(el('span', 'email', email), close);
  bar.append(brand, right);
  return bar;
}

function remember(key) {
  remembered = key;
  const params = new URLSearchParams(location.search);
  if (params.get('tab') === key) {
    return;
  }
  params.set('tab', key);
  history.replaceState(history.state, '', location.pathname + '?' + params);
}

export function adminPage({appName, allowed, email, sections}) {
  document.title = `Admin Tools · ${appName}`;
  if (!allowed) {
    const page = el('div', 'list-page admin-denied');
    page.append(el('h1', 'page-title', 'Admin access required'));
    return page;
  }
  const page = el('div', 'admin');
  const layout = el('div', 'admin-layout');
  const rail = el('nav', 'admin-rail');
  const body = el('div', 'admin-body');
  const tabs = [];
  const panels = [];
  const show = key => {
    for (const tab of tabs) {
      tab.classList.toggle('active', tab.dataset.panel === key);
    }
    for (const panel of panels) {
      panel.hidden = panel.dataset.panel !== key;
    }
    remember(key);
  };
  for (const section of sections.filter(s => s.tabs.length)) {
    const group = el('div', 'admin-rail-section');
    group.append(el('div', 'admin-rail-title', section.title));
    for (const item of section.tabs) {
      const tab = el('div', 'tab');
      tab.dataset.panel = item.key;
      tab.append(el('span', '', item.label));
      if (item.count) {
        tab.append(el('span', 'tab-count', String(item.count)));
      }
      tab.addEventListener('click', () => show(item.key));
      tabs.push(tab);
      group.append(tab);
      const panel = el('div', 'panel');
      panel.dataset.panel = item.key;
      panel.append(item.card());
      panels.push(panel);
      body.append(panel);
    }
    rail.append(group);
  }
  const keys = tabs.map(t => t.dataset.panel);
  const asked = new URLSearchParams(location.search).get('tab');
  show([asked, remembered].find(k => keys.includes(k)) || keys[0]);
  layout.append(rail, body);
  page.append(header(appName, email), layout);
  return page;
}

export function adminsCard({hint, people, title = 'Admins', read = '/api/admin/state', write = '/api/admin/admins', key = 'admins'}) {
  const card = el('div', 'card');
  card.append(el('h2', '', title), el('div', 'hint', hint));
  const rows = el('div');
  const status = el('span', 'save-status');
  let list = [];
  let names = new Map();
  const say = (text, error) => {
    status.textContent = text;
    status.classList.toggle('error', error);
  };
  const persist = async () => {
    say('Saving…', false);
    try {
      await api('POST', write, {[key]: list});
    } catch (err) {
      say(err.message, true);
      return;
    }
    say('Saved.', false);
  };
  const render = () => {
    rows.replaceChildren();
    if (!list.length) {
      rows.append(el('div', 'hint', 'Nobody yet.'));
    }
    for (const email of list) {
      const row = el('div', 'admin-row');
      const who = el('div', 'grow');
      who.append(el('div', '', names.get(email) || email));
      if (names.has(email)) {
        who.append(el('div', 'sub', email));
      }
      const remove = el('button', 'link-button danger', 'Remove');
      remove.type = 'button';
      remove.addEventListener('click', () => {
        list = list.filter(e => e !== email);
        render();
        persist();
      });
      row.append(who, remove);
      rows.append(row);
    }
  };
  const mount = el('div');
  const picker = createPersonPicker(mount, {address: true, people: async () => (await people()).filter(p => !list.includes(p.email))});
  const addOne = () => {
    const email = picker.value;
    if (!email) {
      if (picker.input.value.trim()) {
        say('Pick someone from the list, or type a full email address.', true);
      }
      return;
    }
    picker.reset();
    if (list.includes(email)) {
      return;
    }
    list.push(email);
    render();
    persist();
  };
  mount.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !mount.querySelector('.person-picker-option.active')) {
      e.preventDefault();
      addOne();
    }
  });
  const add = el('button', 'button', 'Add');
  add.type = 'button';
  add.addEventListener('click', addOne);
  const bar = el('div', 'add-row');
  bar.append(mount, add, status);
  card.append(rows, bar);
  Promise.all([api('GET', read), people()]).then(([data, everyone]) => {
    list = data[key];
    names = new Map(everyone.map(p => [p.email, p.name]));
    render();
  }).catch(err => say(`Failed to load the list: ${err.message}`, true));
  return card;
}
