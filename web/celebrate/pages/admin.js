import {state, isAdmin, me, money, whenLine, celebration} from '../state.js';
import {el, button, svg} from '../dom.js';
import {setTitle} from '../chrome.js';
import {openCelebration, openCategory, openSettings, send, reload, openTicket} from '../edit.js';
import {celebrationBand} from './parties.js';

function denied() {
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Admin access required'));
  return page;
}

// bannerCard is the band across the top of the parties page as it is now,
// with one button to change it: the current celebration's picture, title,
// theme, date and button.
function bannerCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Banner'));
  card.append(el('div', 'hint', 'The band across the top of the parties page: the current celebration\u2019s background picture, title and theme, date and place, and its button. Edit the current celebration to change it; Celebrations is where another year becomes current.'));
  const c = celebration(state.model.current);
  if (!c) {
    card.append(el('div', 'notice', 'No celebration is current yet - add one under Celebrations and mark it current.'));
    return card;
  }
  const preview = el('div', 'banner-preview');
  preview.append(celebrationBand(c));
  card.append(preview);
  const add = el('div', 'add-row');
  add.append(button('Edit Banner', 'edit', 'button', () => openCelebration(c)));
  card.append(add);
  return card;
}

function celebrationsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Celebrations'));
  card.append(el('div', 'hint', "Each year's Spring Celebration. Parties are filed under one; the current one leads the parties page."));
  for (const c of state.model.celebrations) {
    const row = el('div', 'admin-row');
    const body = el('div', 'grow');
    body.append(el('div', '', `${c.title}${c.current ? ' · current' : ''}`), el('div', 'sub', [c.code, c.subtitle, whenLine(c)].filter(Boolean).join(' · ')));
    row.append(body, button('Edit', 'edit', 'button button-secondary button-small', () => openCelebration(c)));
    card.append(row);
  }
  const add = el('div', 'add-row');
  add.append(button('Add a celebration', 'plus', 'button', () => openCelebration(null)));
  card.append(add);
  return card;
}

// categoriesCard lists the categories in their order, each renamable, with
// arrows to reorder and a way to add one.
function categoriesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Categories'));
  card.append(el('div', 'hint', 'The choices in the party filter, in this order.'));
  const list = el('div');
  const status = el('span', 'save-status');
  const paint = () => {
    list.replaceChildren();
    const cats = state.model.categories;
    cats.forEach((title, i) => {
      const row = el('div', 'admin-row');
      row.append(el('div', 'grow', title));
      const move = async (from, to) => {
        const order = [...cats];
        order.splice(to, 0, order.splice(from, 1)[0]);
        try {
          await send('POST', '/api/celebrate/categories/order', {titles: order});
          await reload();
          paint();
        } catch (err) {
          status.classList.add('error');
          status.textContent = err.message;
        }
      };
      const up = button('', 'up', 'edit-icon', () => move(i, i - 1));
      up.disabled = i === 0;
      const down = button('', 'down', 'edit-icon', () => move(i, i + 1));
      down.disabled = i === cats.length - 1;
      row.append(up, down, button('Rename', 'edit', 'button button-secondary button-small', () => openCategory(title, paint)));
      list.append(row);
    });
    if (!cats.length) {
      list.append(el('div', 'hint', 'No categories yet.'));
    }
  };
  paint();
  const add = el('div', 'add-row');
  add.append(button('Add a category', 'plus', 'button', () => openCategory('', paint)), status);
  card.append(list, add);
  return card;
}

function settingsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Settings'));
  card.append(el('div', 'hint', 'The intro under the parties heading, and the note on the ticket form.'));
  for (const [label, value] of [['Parties intro', state.model.settings.partiesIntro], ['Ticket note', state.model.settings.ticketNote]]) {
    const row = el('div', 'admin-row');
    const body = el('div', 'grow');
    body.append(el('div', '', label), el('div', 'sub', value || '—'));
    row.append(body);
    card.append(row);
  }
  const add = el('div', 'add-row');
  add.append(button('Edit Settings', 'edit', 'button', openSettings));
  card.append(add);
  return card;
}

// invoicesCard is every ticket of the chosen celebration by purchaser, with
// the totals the business office needs and a CSV of the same.
function invoicesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Invoicing'));
  card.append(el('div', 'hint', 'Every ticket, grouped by who is billed. Click a ticket to mark its invoice sent or paid. Nothing is charged here - the office invoices from this list.'));
  let code = state.model.current || (state.model.celebrations[0] || {}).code;
  const pick = el('select');
  for (const c of state.model.celebrations) {
    const o = new Option(c.title, c.code);
    o.selected = c.code === code;
    pick.append(o);
  }
  const csv = el('a', 'button button-secondary button-small');
  csv.append(svg('download'), el('span', '', 'Download CSV'));
  const bar = el('div', 'invoice-bar');
  bar.append(pick, csv);
  card.append(bar);
  const totals = el('div', 'invoice-totals');
  const table = el('div');
  card.append(totals, table);
  const paint = () => {
    csv.href = `/api/celebrate/invoices.csv?celebration=${encodeURIComponent(code)}`;
    table.replaceChildren();
    totals.replaceChildren();
    const byPurchaser = new Map();
    let sum = 0;
    let paid = 0;
    let sold = 0;
    for (const p of state.model.parties.filter(p => p.celebration === code)) {
      for (const a of p.attendees) {
        const key = a.purchaser || '?';
        if (!byPurchaser.has(key)) {
          byPurchaser.set(key, {name: a.purchaserName || key, email: key, rows: [], total: 0});
        }
        const group = byPurchaser.get(key);
        group.rows.push({p, a});
        group.total += a.price || 0;
        sum += a.price || 0;
        sold++;
        if (a.invoice === 'Paid') {
          paid += a.price || 0;
        }
      }
    }
    totals.append(el('div', 'invoice-total', `${sold} tickets · ${money(sum)} raised · ${money(paid)} paid`));
    const groups = [...byPurchaser.values()].sort((x, y) => x.name.localeCompare(y.name));
    if (!groups.length) {
      table.append(el('div', 'hint', 'No tickets yet.'));
    }
    for (const g of groups) {
      const head = el('div', 'invoice-head');
      head.append(el('span', 'invoice-name', g.name), el('span', 'invoice-email', g.email), el('span', 'invoice-sum', money(g.total)));
      table.append(head);
      for (const {p, a} of g.rows) {
        const row = el('button', 'invoice-row');
        row.type = 'button';
        row.append(el('span', 'invoice-party', p.title), el('span', 'invoice-who', a.name), el('span', 'invoice-price', money(a.price || 0)),
          el('span', 'invoice-status ' + (a.invoice ? 'is-' + a.invoice.toLowerCase() : ''), a.invoice || 'not yet'));
        row.addEventListener('click', () => openTicket(p, a));
        table.append(row);
      }
    }
  };
  pick.addEventListener('change', () => {
    code = pick.value;
    paint();
  });
  paint();
  return card;
}

// adminsCard mirrors the other apps' admin lists: every add or remove posts
// immediately, so nothing looks saved that isn't.
function adminsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Admins'));
  card.append(el('div', 'hint', 'Whoever is on this list can approve parties, edit any party, record invoicing, and reach this page. Hosts edit their own parties without being here.'));
  const notice = el('div');
  const rows = el('div');
  const status = el('span', 'save-status');
  let admins = [];
  const persist = async () => {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    const res = await fetch('/api/admin/admins', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({admins})});
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    status.textContent = 'Saved.';
  };
  const render = () => {
    rows.replaceChildren();
    if (!admins.length) {
      rows.append(el('div', 'hint', 'Nobody yet.'));
    }
    for (const email of admins) {
      const row = el('div', 'admin-row');
      row.append(el('div', 'grow', email));
      const remove = el('button', 'link-button danger', 'Remove');
      remove.type = 'button';
      remove.addEventListener('click', async () => {
        admins = admins.filter(e => e !== email);
        render();
        await persist();
      });
      row.append(remove);
      rows.append(row);
    }
  };
  const add = el('div', 'add-row');
  const input = el('input');
  input.type = 'email';
  input.placeholder = 'name@heliosschool.org';
  const addOne = async () => {
    const email = input.value.trim().toLowerCase();
    if (!email || admins.includes(email)) {
      return;
    }
    admins.push(email);
    input.value = '';
    render();
    await persist();
  };
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addOne();
    }
  });
  add.append(input, button('Add', null, 'button', addOne), status);
  card.append(notice, rows, add);
  fetch('/api/admin/state').then(async res => {
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = 'Failed to load the admin list.';
      return;
    }
    const data = await res.json();
    admins = data.admins;
    render();
  });
  return card;
}

const sections = [
  {title: 'Display', tabs: [
    {key: 'banner', label: 'Banner', card: bannerCard},
    {key: 'celebrations', label: 'Celebrations', card: celebrationsCard},
    {key: 'categories', label: 'Categories', card: categoriesCard},
    {key: 'settings', label: 'Settings', card: settingsCard},
  ]},
  {title: 'Money', tabs: [
    {key: 'invoices', label: 'Invoicing', card: invoicesCard},
  ]},
  {title: 'Editing & Control', tabs: [
    {key: 'admins', label: 'Admins', card: adminsCard},
  ]},
];

// The admin chrome every app shares (web/common/admin.css): the teal header
// with the mark, "Admin", the address and a close button; the rail of grouped
// tabs; one panel showing at a time.
export function adminPage() {
  setTitle('Admin Tools');
  if (!isAdmin()) {
    return denied();
  }
  const page = el('div', 'admin admin-strip');
  const header = el('header');
  const brand = el('a', 'brand-link');
  brand.href = '/';
  brand.setAttribute('data-link', '');
  const mark = el('img', 'admin-tile');
  mark.src = '/brand/icon-192.png';
  mark.alt = 'Helios Celebrate';
  brand.append(mark, el('span', '', 'Admin'));
  const right = el('span', 'right');
  right.append(el('span', 'email', me().email));
  const close = el('a', 'admin-close');
  close.href = '/';
  close.setAttribute('data-link', '');
  close.setAttribute('aria-label', 'Close admin tools');
  close.append(svg('close'));
  right.append(close);
  header.append(brand, right);

  const layout = el('div', 'layout');
  const rail = el('nav', 'sidebar');
  const container = el('div', 'container');
  const panels = {};
  const tabList = [];
  const show = key => {
    for (const tab of tabList) {
      tab.classList.toggle('active', tab.dataset.panel === key);
    }
    for (const [k, panel] of Object.entries(panels)) {
      panel.hidden = k !== key;
    }
    state.adminTab = key;
  };
  for (const section of sections) {
    const group = el('div', 'sidebar-section');
    group.append(el('div', 'sidebar-section-title', section.title));
    for (const item of section.tabs) {
      const tab = el('div', 'tab', item.label);
      tab.dataset.panel = item.key;
      tab.addEventListener('click', () => show(item.key));
      tabList.push(tab);
      group.append(tab);
      const panel = el('div', 'panel');
      panel.append(item.card());
      panels[item.key] = panel;
      container.append(panel);
    }
    rail.append(group);
  }
  show(state.adminTab && panels[state.adminTab] ? state.adminTab : sections[0].tabs[0].key);
  layout.append(rail, container);
  page.append(header, layout);
  return page;
}
