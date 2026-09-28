import {state, isAdmin, me, money, whenLine, celebration} from '../state.js';
import {el, button, svg} from '/elements.js';
import {openCelebration, openCategory, openSettings, openMoveAddress} from '../edit.js';
import {load} from '/router.js';
import {api} from '/api.js';
import {celebrationBand} from './parties.js';
import {adminPage as buildAdminPage, adminsCard} from '/admin.js';

function bannerCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Banner'));
  card.append(el('div', 'hint', 'The band across the top of the parties page: a celebration\u2019s background picture, title and theme, date and place, and its button. Which celebration it advertises is separate from which one\u2019s parties are listed, so next year\u2019s gala can sit over this season\u2019s parties.'));
  const c = celebration(state.model.banner);
  if (!c) {
    card.append(el('div', 'notice', 'No celebration to show yet - add one under Celebrations.'));
    return card;
  }
  const pick = el('select');
  for (const each of state.model.celebrations) {
    const o = new Option(each.title, each.id);
    o.selected = each.id === c.id;
    pick.append(o);
  }
  pick.addEventListener('change', async () => {
    const chosen = celebration(pick.value);
    try {
      await api('POST', '/api/celebrate/celebration', {
        id: chosen.id, code: chosen.code, title: chosen.title, subtitle: chosen.subtitle || '', start: chosen.start || '', end: chosen.end || '',
        location: chosen.location || '', address: chosen.address || '', description: chosen.description || '', image: chosen.image || '',
        buttonText: chosen.buttonText || '', buttonUrl: chosen.buttonUrl || '', current: chosen.current, banner: true,
      });
      await load();
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
    const name = el('div', 'admin-row-name');
    name.append(el('span', '', c.title));
    if (c.current) {
      name.append(el('span', 'admin-tag is-on', 'Parties listed'));
    }
    if (c.id === state.model.banner) {
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

function categoriesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Categories'));
  card.append(el('div', 'hint', 'The choices in the party filter, in this order.'));
  const list = el('div');
  const status = el('span', 'save-status');
  const paint = () => {
    list.replaceChildren();
    const cats = state.model.categories;
    cats.forEach((cat, i) => {
      const row = el('div', 'admin-row');
      row.append(el('div', 'grow', cat.title));
      const move = async (from, to) => {
        const order = cats.map(c => c.id);
        order.splice(to, 0, order.splice(from, 1)[0]);
        try {
          await api('POST', '/api/celebrate/categories/order', {ids: order});
          await load();
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
      row.append(up, down, button('Rename', 'edit', 'button button-secondary button-small', () => openCategory(cat, paint)));
      list.append(row);
    });
    if (!cats.length) {
      list.append(el('div', 'hint', 'No categories yet.'));
    }
  };
  paint();
  const add = el('div', 'add-row');
  add.append(button('Add a category', 'plus', 'button', () => openCategory(null, paint)), status);
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

function invoicesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Invoicing'));
  card.append(el('div', 'hint', 'The INVOICING tab of the sheet, as accounting keeps it: a row for every ticket sold, with the invoice number and who it went to once the office fills them in. Nothing is charged here.'));
  let shownId = state.model.current || (state.model.celebrations[0] || {}).id;
  const pick = el('select');
  for (const c of state.model.celebrations) {
    const o = new Option(c.title, c.id);
    o.selected = c.id === shownId;
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
  const ledger = () => (state.model.invoicing || []).filter(l => l.celebration === shownId);
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
    const titles = new Map(ledger().map(l => [l.partyId, l.party]));
    for (const [id, title] of [...titles].sort((x, y) => x[1].localeCompare(y[1]))) {
      party.append(new Option(title, id));
    }
  };
  const paint = () => {
    csv.href = `/api/celebrate/invoices.csv?celebration=${encodeURIComponent(shownId)}`;
    table.replaceChildren();
    totals.replaceChildren();
    const q = find.value.trim().toLowerCase();
    const rows = ledger().filter(l => {
      if (party.value && l.partyId !== party.value) {
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
    shownId = pick.value;
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

function addressesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Addresses'));
  card.append(el('div', 'hint', 'School addresses on a ticket, a waitlist request, a bill or a host that the directory does not have - most often an alum whose school account closed after graduation. Change one and it moves on every party, tickets and guest lists alike, and invitations already sent to it go again.'));
  const past = el('input');
  past.type = 'checkbox';
  const pastLabel = el('label', 'email-fix-past');
  pastLabel.append(past, el('span', '', 'Show addresses only on past parties'));
  const list = el('div', 'email-fix-list');
  const movedHead = el('h3', 'email-fix-moved-head', 'Already changed');
  const moved = el('div', 'email-fix-list');
  card.append(pastLabel, list, movedHead, moved);
  let data = null;
  const paint = () => {
    list.replaceChildren();
    moved.replaceChildren();
    if (!data) {
      list.append(el('div', 'hint', 'Loading…'));
      return;
    }
    const rows = data.problems.filter(pr => past.checked || pr.upcoming);
    const hidden = data.problems.length - rows.length;
    if (!rows.length) {
      list.append(el('div', 'hint', hidden ? `None on a party still to come; ${hidden} only on past parties.` : 'Every school address on the parties is in the directory.'));
    }
    for (const pr of rows) {
      const row = el('div', 'email-fix-row');
      const who = el('div', 'email-fix-who');
      who.append(el('div', 'email-fix-name', pr.name), el('div', 'email-fix-email', pr.email));
      const uses = el('div', 'email-fix-uses');
      for (const u of pr.uses) {
        const chip = el('a', 'email-fix-use' + (u.past ? ' is-past' : ''));
        chip.href = u.path;
        chip.setAttribute('data-link', '');
        chip.textContent = u.role === 'Ticket' ? u.party : `${u.party} · ${u.role === 'Billed' ? 'billed' : u.role === 'Host' ? 'host' : 'waitlist'}`;
        uses.append(chip);
      }
      who.append(uses);
      row.append(who, button('Change address', 'mail', 'button button-secondary button-small', () => openMoveAddress(pr, load)));
      list.append(row);
    }
    if (hidden && rows.length) {
      list.append(el('div', 'hint', `${hidden} more only on past parties.`));
    }
    movedHead.hidden = !data.moved.length;
    for (const m of data.moved) {
      const row = el('div', 'email-fix-row');
      const who = el('div', 'email-fix-who');
      who.append(el('div', 'email-fix-name', m.name || m.old), el('div', 'email-fix-email', `${m.old} \u2192 ${m.new}${m.changed ? ' \u00b7 ' + m.changed : ''}`));
      row.append(who);
      moved.append(row);
    }
  };
  const load = async () => {
    try {
      data = await api('GET', '/api/celebrate/addresses');
      paint();
    } catch (err) {
      list.replaceChildren(el('div', 'hint error', err.message));
    }
  };
  past.addEventListener('change', paint);
  paint();
  load();
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
    {key: 'addresses', label: 'Addresses', card: addressesCard},
    {key: 'admins', label: 'Admins', card: () => adminsCard({hint: 'Whoever is on this list can approve parties, edit any party, record invoicing, and reach this page. Hosts edit their own parties without being here.'})},
  ]},
];

export function adminPage() {
  return buildAdminPage({appName: 'Helios Celebrate', allowed: isAdmin(), email: me().email, sections});
}
