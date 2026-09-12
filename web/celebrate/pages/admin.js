import {state, isSystemAdmin, me, money, whenLine, celebration} from '../state.js';
import {el, button, svg} from '../dom.js';
import {setTitle} from '../chrome.js';
import {openCelebration, openCategory, openSettings, send, reload} from '../edit.js';
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
  card.append(el('div', 'hint', 'The band across the top of the parties page: a celebration\u2019s background picture, title and theme, date and place, and its button. Which celebration it advertises is separate from which one\u2019s parties are listed, so next year\u2019s gala can sit over this season\u2019s parties.'));
  const c = celebration(state.model.banner);
  if (!c) {
    card.append(el('div', 'notice', 'No celebration to show yet - add one under Celebrations.'));
    return card;
  }
  // Which celebration the band shows, switched right here.
  const pick = el('select');
  for (const each of state.model.celebrations) {
    const o = new Option(each.title, each.code);
    o.selected = each.code === c.code;
    pick.append(o);
  }
  pick.addEventListener('change', async () => {
    const chosen = celebration(pick.value);
    try {
      await send('POST', '/api/celebrate/celebration', {
        original: chosen.code, code: chosen.code, title: chosen.title, subtitle: chosen.subtitle || '', start: chosen.start || '', end: chosen.end || '',
        location: chosen.location || '', address: chosen.address || '', description: chosen.description || '', image: chosen.image || '',
        buttonText: chosen.buttonText || '', buttonUrl: chosen.buttonUrl || '', current: chosen.current, banner: true,
      });
      await reload();
    } catch (err) {
      alert(err.message);
    }
  });
  const row = el('label', 'field');
  row.append(el('span', '', 'Show the banner for'), pick);
  card.append(row);
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
  card.append(el('div', 'hint', "Each year's Spring Celebration. Parties are filed under one; the current one's parties lead the parties page, and the banner one is advertised across its top."));
  for (const c of state.model.celebrations) {
    const row = el('div', 'admin-row');
    const body = el('div', 'grow');
    // Each row says what the site does with it: whose parties are listed,
    // and whose banner is shown - two flags, so they can sit on different
    // years.
    const name = el('div', 'admin-row-name');
    name.append(el('span', '', c.title));
    if (c.current) {
      name.append(el('span', 'admin-tag is-on', 'Parties listed'));
    }
    if (c.code === state.model.banner) {
      name.append(el('span', 'admin-tag is-on', 'Banner shown'));
    }
    body.append(name, el('div', 'sub', [c.code, c.subtitle, whenLine(c)].filter(Boolean).join(' · ')));
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
  card.append(el('div', 'hint', 'The intro under the parties heading, the note on the ticket form, and whether anyone can post a party.'));
  for (const [label, value] of [['Parties intro', state.model.settings.partiesIntro], ['Ticket note', state.model.settings.ticketNote], ['Hosting', state.model.settings.hostingOpen ? 'Open - anyone can post a party' : 'Closed - only admins can post a party']]) {
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

// invoicesCard is the INVOICING ledger - the sheet's own accounting tab -
// for the chosen celebration, grouped by who is billed, with filters to
// find a family, a party, or what is still to be invoiced. The app writes
// a row when a ticket sells; accounting fills in Invoice and Invoice To in
// the sheet, and this is where it shows.
function invoicesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Invoicing'));
  card.append(el('div', 'hint', 'The INVOICING tab of the sheet, as accounting keeps it: a row for every ticket sold, with the invoice number and who it went to once the office fills them in. Nothing is charged here.'));
  let code = state.model.current || (state.model.celebrations[0] || {}).code;
  const pick = el('select');
  for (const c of state.model.celebrations) {
    const o = new Option(c.title, c.code);
    o.selected = c.code === code;
    pick.append(o);
  }
  const party = el('select');
  const status = el('select');
  for (const [label, value] of [['All rows', 'all'], ['Not yet invoiced', 'open'], ['Invoiced', 'done']]) {
    status.append(new Option(label, value));
  }
  const find = el('input', 'invoice-search');
  find.type = 'search';
  find.placeholder = 'Search purchaser, guest, invoice…';
  const csv = el('a', 'button button-secondary button-small');
  csv.append(svg('download'), el('span', '', 'Download CSV'));
  const bar = el('div', 'invoice-bar');
  bar.append(pick, party, status, find, csv);
  card.append(bar);
  const totals = el('div', 'invoice-totals');
  const table = el('div');
  card.append(totals, table);
  const ledger = () => (state.model.invoicing || []).filter(l => l.code === code);
  // The ledger holds addresses; the parties' tickets know the names.
  const names = new Map();
  for (const p of state.model.parties) {
    for (const a of [...p.attendees, ...p.waitlisted]) {
      if (a.purchaser && a.purchaserName) {
        names.set(a.purchaser, a.purchaserName);
      }
    }
  }
  const paintParties = () => {
    party.replaceChildren(new Option('All parties', ''));
    for (const title of [...new Set(ledger().map(l => l.party))].sort((x, y) => x.localeCompare(y))) {
      party.append(new Option(title, title));
    }
  };
  const paint = () => {
    csv.href = `/api/celebrate/invoices.csv?celebration=${encodeURIComponent(code)}`;
    table.replaceChildren();
    totals.replaceChildren();
    const q = find.value.trim().toLowerCase();
    const rows = ledger().filter(l => {
      if (party.value && l.party !== party.value) {
        return false;
      }
      if (status.value === 'open' && l.invoice) {
        return false;
      }
      if (status.value === 'done' && !l.invoice) {
        return false;
      }
      return !q || [l.purchaser, names.get(l.purchaser), l.guest, l.invoice, l.invoiceTo, l.party].some(v => (v || '').toLowerCase().includes(q));
    });
    const byPurchaser = new Map();
    let sum = 0;
    let invoiced = 0;
    let tickets = 0;
    for (const l of rows) {
      if (!byPurchaser.has(l.purchaser)) {
        byPurchaser.set(l.purchaser, {email: l.purchaser, rows: [], total: 0});
      }
      const g = byPurchaser.get(l.purchaser);
      const amount = (l.cost || 0) * (l.quantity || 1);
      g.rows.push(l);
      g.total += amount;
      sum += amount;
      tickets += l.quantity || 1;
      if (l.invoice) {
        invoiced += amount;
      }
    }
    totals.append(el('div', 'invoice-total', `${tickets} tickets · ${money(sum)} · ${money(invoiced)} invoiced · ${money(sum - invoiced)} still to invoice`));
    const groups = [...byPurchaser.values()].sort((x, y) => (names.get(x.email) || x.email).localeCompare(names.get(y.email) || y.email));
    if (!groups.length) {
      table.append(el('div', 'hint', rows.length || ledger().length ? 'Nothing matches.' : 'Nothing in the ledger yet.'));
    }
    for (const g of groups) {
      const head = el('div', 'invoice-head');
      head.append(el('span', 'invoice-name', names.get(g.email) || g.email), el('span', 'invoice-email', `${g.email} · ${g.rows.length} ${g.rows.length === 1 ? 'row' : 'rows'}`), el('span', 'invoice-sum', money(g.total)));
      table.append(head);
      for (const l of g.rows) {
        const row = el('div', 'invoice-row invoice-ledger-row');
        row.append(el('span', 'invoice-date', l.date), el('span', 'invoice-party', l.party), el('span', 'invoice-who', l.guest),
          el('span', 'invoice-price', money((l.cost || 0) * (l.quantity || 1))),
          el('span', 'invoice-status ' + (l.invoice ? 'is-paid invoice-number' : ''), l.invoice ? `${l.invoice}${l.invoiceTo ? ' · ' + l.invoiceTo : ''}` : 'not yet'));
        table.append(row);
      }
    }
  };
  pick.addEventListener('change', () => {
    code = pick.value;
    paintParties();
    paint();
  });
  party.addEventListener('change', paint);
  status.addEventListener('change', paint);
  find.addEventListener('input', paint);
  paintParties();
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
  // The page goes with being on the admin list, hat or no hat.
  if (!isSystemAdmin()) {
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
