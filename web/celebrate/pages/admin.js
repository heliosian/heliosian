import {state, anyAllowance, me, money, whenLine, celebration, categoriesOf, parties, party} from '../state.js';
import {write} from '/rail.js';
import {el, button} from '/elements.js';
import {openCelebration, openCategory, openSettings, saveSetting} from '../edit.js';
import {load} from '/router.js';
import {keyBetween} from '/order.js';
import {celebrationBand} from './parties.js';
import {adminPage as buildAdminPage, adminsCard, groupMembers} from '/admin.js';
import {openModal} from '/modal.js';
import {field, text, checkbox} from '/form.js';
import {createPersonPicker} from '/picker.js';
import {listed, isStudent} from '/directory.js';

function celebrationPicker(shown, onPick) {
  const pick = el('select');
  for (const c of state.model.celebrations) {
    const o = new Option(c.title, c.id);
    o.selected = c.id === shown;
    pick.append(o);
  }
  pick.addEventListener('change', () => onPick(pick.value));
  return pick;
}

function bannerCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Banner'));
  card.append(el('div', 'hint', 'The band across the top of the parties page: a celebration’s background picture, title and theme, date and place, and the links in its description as buttons. Which celebration it advertises is separate from which one’s parties are listed, so next year’s gala can sit over this season’s parties.'));
  const c = celebration(state.model.banner);
  if (!c) {
    card.append(el('div', 'notice', 'No celebration to show yet - add one under Celebrations.'));
    return card;
  }
  const pick = celebrationPicker(c.id, async id => {
    try {
      await saveSetting('Banner', id);
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
  card.append(el('div', 'hint', "Each year's Spring Celebration. Parties are filed under one; the current one's parties lead the parties page, and the banner one is advertised across its top. One that takes parties lets anyone host a party under it, waiting for approval."));
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
    if (c.adding === 'approval') {
      name.append(el('span', 'admin-tag is-on', 'Takes parties'));
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
  card.append(el('div', 'hint', 'The choices in the party filter, in this order. Each celebration keeps its own.'));
  let shown = state.celebration;
  const list = el('div');
  const status = el('span', 'save-status');
  const paint = () => {
    list.replaceChildren();
    const cats = categoriesOf(shown);
    cats.forEach((cat, i) => {
      const row = el('div', 'admin-row');
      row.append(el('div', 'grow', cat.title));
      const move = async to => {
        const rest = cats.filter(c => c.id !== cat.id);
        const order = keyBetween(rest[to - 1] ? rest[to - 1].order : undefined, rest[to] ? rest[to].order : undefined);
        try {
          await write([{set: cat.id, cells: {order}}]);
          await load();
        } catch (err) {
          status.classList.add('error');
          status.textContent = err.message;
        }
      };
      const up = button('', 'up', 'edit-icon', () => move(i - 1));
      up.disabled = i === 0;
      const down = button('', 'down', 'edit-icon', () => move(i + 1));
      down.disabled = i === cats.length - 1;
      row.append(up, down, button('Rename', 'edit', 'button button-secondary button-small', () => openCategory(cat, shown, paint)));
      list.append(row);
    });
    if (!cats.length) {
      list.append(el('div', 'hint', 'No categories yet.'));
    }
  };
  const pick = celebrationPicker(shown, id => {
    shown = id;
    paint();
  });
  paint();
  const add = el('div', 'add-row');
  add.append(button('Add a category', 'plus', 'button', () => openCategory(null, shown, paint)), status);
  card.append(pick, list, add);
  return card;
}

function settingsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Settings'));
  card.append(el('div', 'hint', 'The intro under the parties heading and the note on the ticket form. Whether anyone can host a party is each celebration’s, under Celebrations.'));
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

const statusWords = {'': 'Owed', requested: 'Requested', paid: 'Paid', cancelled: 'Cancelled'};

function openPayer(x) {
  const mount = el('div');
  const picker = createPersonPicker(mount, {people: async () => (await listed()).filter(p => !isStudent(p))});
  openModal(`Who pays for ${x.forName || x.name || 'this'}?`, [field('Payer', mount, `Now ${x.payerName || x.payerEmail}.`, true)], {
    submit: () => {
      if (!picker.person) {
        throw new Error('Pick an adult from the directory.');
      }
      return write([{set: x.id, cells: {person: picker.person.id}}]);
    },
  });
}

function openInvoice(payer, owed, label) {
  const id = text('', {placeholder: 'in_…', required: true});
  const picks = owed.map(x => ({x, box: checkbox(`${label(x)}${x.forName ? ` · ${x.forName}` : ''} · ${money(Number(x.amount) || 0)}`, true)}));
  openModal(`Invoice ${payer}`, [field('Stripe invoice ID', id, 'Marks the rows it bills Requested.', true), ...picks.map(p => p.box.wrap)], {
    saveLabel: 'Record invoice',
    submit: () => {
      const invoice = id.value.trim();
      if (!/^in_[A-Za-z0-9]+$/.test(invoice)) {
        throw new Error('A Stripe invoice ID starts with in_.');
      }
      const billed = picks.filter(p => p.box.input.checked);
      if (!billed.length) {
        throw new Error('Pick the rows this invoice bills.');
      }
      return write(billed.map(p => ({set: p.x.id, cells: {stripe_invoice: invoice, status: 'requested'}})));
    },
  });
}

function paymentsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Payments'));
  card.append(el('div', 'hint', 'One row for every place billed, and for anything sold outside the app. The rows sharing a Stripe invoice are that invoice. Once you send one, record its ID on the payer’s owed rows it bills, which marks them Requested. Set where each stands, or who pays; nothing is charged here.'));
  let shown = state.model.current || (state.model.celebrations[0] || {}).id;
  const partyPick = el('select');
  const status = el('select');
  for (const [label, value] of [['Every status', 'all'], ['Owed', ''], ['Requested', 'requested'], ['Paid', 'paid'], ['Cancelled', 'cancelled']]) {
    status.append(new Option(label, value));
  }
  const find = el('input', 'invoice-search');
  find.type = 'search';
  find.placeholder = 'Search payer, ticket holder, invoice…';
  const pick = celebrationPicker(shown, id => {
    shown = id;
    paintParties();
    paint();
  });
  const bar = el('div', 'invoice-bar');
  bar.append(pick, partyPick, status, find);
  const totals = el('div', 'invoice-totals');
  const table = el('div');
  card.append(bar, totals, table);
  const rows = () => {
    const groups = new Set([shown, ...parties(shown).map(p => p.id)]);
    return state.model.payments.filter(x => groups.has(x.group));
  };
  const paintParties = () => {
    partyPick.replaceChildren(new Option('Every party', ''));
    for (const p of parties(shown).filter(p => rows().some(x => x.group === p.id)).sort((a, b) => a.title.localeCompare(b.title))) {
      partyPick.append(new Option(p.title, p.id));
    }
  };
  const label = x => (x.group === shown ? x.name : (party(x.group) || {title: ''}).title);
  const paint = () => {
    table.replaceChildren();
    totals.replaceChildren();
    const q = find.value.trim().toLowerCase();
    const list = rows().filter(x => (!partyPick.value || x.group === partyPick.value)
      && (status.value === 'all' || (x.status || '') === status.value)
      && (!q || [x.payerName, x.payerEmail, x.forName, x.stripe_invoice, label(x)].some(v => (v || '').toLowerCase().includes(q))));
    const sum = which => list.filter(x => (x.status || '') === which).reduce((n, x) => n + (Number(x.amount) || 0), 0);
    totals.append(el('div', 'invoice-total', `${list.length} ${list.length === 1 ? 'payment' : 'payments'} · ${money(sum('paid'))} paid · ${money(sum('requested'))} requested · ${money(sum(''))} owed · ${money(sum('cancelled'))} cancelled`));
    const byPayer = new Map();
    for (const x of list) {
      if (!byPayer.has(x.person)) {
        byPayer.set(x.person, []);
      }
      byPayer.get(x.person).push(x);
    }
    if (!byPayer.size) {
      table.append(el('div', 'hint', rows().length ? 'Nothing matches.' : 'No payments yet.'));
    }
    const payers = [...byPayer.entries()].sort((a, b) => (a[1][0].payerName || '').localeCompare(b[1][0].payerName || ''));
    for (const [, items] of payers) {
      const first = items[0];
      const head = el('div', 'invoice-head');
      const owed = items.filter(x => x.status !== 'cancelled').reduce((n, x) => n + (Number(x.amount) || 0), 0);
      head.append(el('span', 'invoice-name', first.payerName || first.payerEmail), el('span', 'invoice-email', `${first.payerEmail} · ${items.length} ${items.length === 1 ? 'row' : 'rows'}`));
      const unbilled = items.filter(x => !x.status);
      if (unbilled.length) {
        head.append(button('Record invoice', 'plus', 'button button-secondary button-small', () => openInvoice(first.payerName || first.payerEmail, unbilled, label)));
      }
      head.append(el('span', 'invoice-sum', money(owed)));
      table.append(head);
      for (const x of items.sort((a, b) => (a.added || '').localeCompare(b.added || ''))) {
        const row = el('div', 'invoice-row invoice-ledger-row');
        const set = el('select', 'invoice-status');
        for (const [value, words] of Object.entries(statusWords)) {
          const o = new Option(words, value);
          o.selected = (x.status || '') === value;
          set.append(o);
        }
        set.addEventListener('change', async () => {
          try {
            await write([{set: x.id, cells: {status: set.value}}]);
            await load();
          } catch (err) {
            alert(err.message);
          }
        });
        row.append(el('span', 'invoice-date', (x.added || '').slice(0, 10)), el('span', 'invoice-party', label(x)), el('span', 'invoice-who', x.forName || ''),
          el('span', 'invoice-price', money(Number(x.amount) || 0)), el('span', 'invoice-number', x.stripe_invoice || 'not yet invoiced'), set,
          button('', 'people', 'edit-icon', () => openPayer(x)));
        table.append(row);
      }
    }
  };
  partyPick.addEventListener('change', paint);
  status.addEventListener('change', paint);
  find.addEventListener('input', paint);
  paintParties();
  paint();
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
    {key: 'payments', label: 'Payments', card: paymentsCard},
  ]},
  {title: 'Editing & Control', tabs: [
    {key: 'admins', label: 'Admins', card: () => adminsCard({
      hint: 'Whoever is on this list can approve parties, edit any party, keep the payments, and reach this page. Hosts edit their own parties without being here.',
      ...groupMembers(['in', {n: 'id'}, ['select', 'APP.admins', ['=', {n: 'key'}, 'celebrate']]]),
    })},
  ]},
];

export function adminPage() {
  return buildAdminPage({appName: 'Helios Celebrate', allowed: anyAllowance(), email: me().email, sections});
}
