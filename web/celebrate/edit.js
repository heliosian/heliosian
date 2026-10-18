import {state, me, allows, household, billable, admits, inAudience, audienceWords, ticketFor, money, currentCelebration, partyPath, party, write, celebration, category, categoriesOf} from './state.js';
import {addressSuggest} from '/address.js';
import {createPersonPicker} from '/picker.js';
import {listed, known, emailOf, photoOf, wordsOf, gradeOf, isStudent, isParent, isStaff, relativesOf} from '/directory.js';
import {openPersonCard} from '/personcard.js';
import {api} from '/api.js';
import {keyBetween} from '/order.js';
import {el, svg, toast, button} from '/elements.js';
import {personRow} from '/personrow.js';
import {tabbedFields} from '/tabs.js';
import {imageTools} from '/images.js';
import {openModal, closeModal, popup} from '/modal.js';
import {load, navigate} from '/router.js';
import {field, text, textarea, select, checkbox, segmented, whenPickers} from '/form.js';
import {dataGrid} from '/datagrid.js';
import {familyDropdown} from '/rules.js';

export const {uploadImage, imageSearchOn, openImageSearch, imagePicker} = imageTools('/api/celebrate', {state});

function stamp() {
  const d = new Date();
  const two = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())} ${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`;
}

async function knownGuest(who, editor) {
  if (who.email) {
    const {person} = await api('POST', '/api/do/person-at', {address: who.email});
    if (person) {
      const listed = known(who.email);
      if (listed && !editor && !household().some(h => h.id === person)) {
        throw new Error(`${who.email} is ${listed.name_show}'s address in the directory: a ticket for someone outside your family is added by the party's hosts.`);
      }
      return {id: person, name: who.name};
    }
    return who;
  }
  const want = who.name.toLowerCase();
  const named = state.model.guests.find(g => g.name.toLowerCase() === want);
  return named ? {id: named.id, name: who.name} : who;
}

function guestEdits(as, guest) {
  const edits = [{insert: 'PERSON', as, row: guest.name ? {source: 'guest', name_long_override: guest.name} : {source: 'guest'}}];
  if (guest.email) {
    edits.push({insert: 'PERSON_EMAIL', row: {person: '@' + as, address: guest.email, primary: true, source: 'guest'}});
  }
  return edits;
}

function paidPlace(p, payer, holder, member) {
  const edits = [];
  if (p.price) {
    edits.push({insert: 'PAYMENT', row: {person: payer, for: holder, group: p.id, amount: p.row.price, added: stamp()}});
  }
  edits.push({insert: 'MEMBER', row: {group: p.id, person: holder, member: 'yes', added_by: me().id, added: stamp(), ...member}});
  return edits;
}

async function saved(edits, words) {
  try {
    await write(edits);
    await load();
    toast(words);
  } catch (err) {
    toast(err.message);
  }
}

function peoplePicker(options) {
  return createPersonPicker(el('div'), {people: listed, address: true, ...options});
}

export function openPerson(v) {
  return openPersonCard(v);
}

function personChip(person, on, disabledWhy) {
  const chip = personRow({name: person.name, email: person.email, photoUrl: person.photoUrl}, {
    button: true,
    className: 'person-chip' + (on ? ' is-on' : ''),
    lines: [person.grade || person.title],
  });
  if (disabledWhy) {
    chip.disabled = true;
    chip.title = disabledWhy;
  }
  return chip;
}

function buyEdits(p, payer, places, note, editor) {
  const viewer = me().id;
  const room = editor || !p.capacity ? places.length : p.remaining;
  const edits = [];
  let sold = 0;
  let waited = 0;
  places.forEach((who, i) => {
    let ref = who.id;
    if (!ref) {
      edits.push(...guestEdits('g' + i, who));
      ref = '@g' + i;
    }
    const brought = who.id && household().some(h => h.id === who.id) ? {} : {guest_of: viewer};
    if (i < room) {
      edits.push(...paidPlace(p, payer, ref, {...brought, note}));
      sold++;
      return;
    }
    edits.push({insert: 'MEMBER', row: {group: p.waitlistGroup, person: ref, member: 'yes', added_by: viewer, added: stamp(), ...brought, note}});
    waited++;
  });
  return {edits, sold, waited};
}

export function openBuy(p) {
  const editor = p.can.edit;
  if (p.availability === 'waitlist') {
    openWaitlist(p);
    return;
  }
  const waiting = false;
  const fields = [];
  const chosen = new Set();
  let purchaser = '';
  const bills = billable();
  const added = [];

  const billRow = el('div', 'chip-pick');
  const billField = field('Who should we bill?', billRow, 'Tickets are invoiced to an adult in your family.', true);
  const paintBill = () => {
    billRow.replaceChildren();
    for (const b of bills) {
      const chip = personChip(b, purchaser === b.email);
      chip.addEventListener('click', () => {
        purchaser = b.email;
        paintBill();
      });
      billRow.append(chip);
    }
  };
  if (bills.length) {
    purchaser = bills[0].email;
  }
  paintBill();
  if (!waiting) {
    fields.push(billField);
  }

  const who = el('div', 'chip-pick');
  const whoWrap = el('div');
  whoWrap.append(who);
  const whoField = field(waiting ? 'Who wants a ticket?' : "Who's coming?", whoWrap, `This party is for ${audienceWords(p)}.`, true);
  const paintWho = () => {
    who.replaceChildren();
    for (const person of household()) {
      let why = '';
      const have = ticketFor(p, person.email);
      if (have && have.status === 'Ticket') {
        why = `${person.name} already has a ticket`;
      } else if (have) {
        why = `${person.name} is already on the waitlist`;
      } else if (!admits(p, person)) {
        why = `This party is for ${audienceWords(p)}`;
      }
      const chip = personChip(person, chosen.has(person.email), why);
      chip.addEventListener('click', () => {
        if (chosen.has(person.email)) {
          chosen.delete(person.email);
        } else {
          chosen.add(person.email);
        }
        paintWho();
        paintTotal();
      });
      who.append(chip);
    }
    for (const a of added) {
      const chip = personChip({name: a.name, photoUrl: a.photoUrl, title: a.guest ? 'Guest' : a.title}, true);
      chip.title = 'Remove';
      chip.addEventListener('click', () => {
        added.splice(added.indexOf(a), 1);
        paintWho();
        paintTotal();
      });
      who.append(chip);
    }
  };
  paintWho();
  const addRow = el('div', 'who-add');
  addRow.append(button('Add someone', 'plus', 'button button-secondary button-small', () => openAddSomeone(p, editor, person => {
    if (person.email && (added.some(a => a.email === person.email) || household().some(h => h.email === person.email))) {
      toast(`${person.name} is already on the list`);
      return;
    }
    added.push(person);
    paintWho();
    paintTotal();
  })));
  whoWrap.append(addRow);
  fields.push(whoField);

  const note = textarea('', 2);
  note.placeholder = 'Anything the hosts should know (optional)';
  fields.push(field('Note', note));

  const total = el('div', 'buy-total');
  const paintTotal = () => {
    const n = chosen.size + added.length;
    total.replaceChildren();
    if (!n) {
      total.append(el('span', 'buy-total-hint', 'Pick at least one person.'));
      return;
    }
    const names = [...chosen].map(email => (household().find(h => h.email === email) || {}).name || email)
      .concat(added.map(a => a.name));
    total.append(el('span', 'buy-total-names', names.join(', ')));
    if (waiting) {
      const billed = bills.find(b => b.email === purchaser);
      total.append(el('span', 'buy-total-sum', `${n} on the waitlist`));
      total.append(el('span', 'buy-total-hint', `If a place opens up, tickets are ${money(p.price)} each${billed ? `, billed to ${billed.name}` : ''}.`));
      return;
    }
    total.append(el('span', 'buy-total-sum', `${n} ${n === 1 ? 'ticket' : 'tickets'} × ${money(p.price)} = ${money(n * p.price)}`));
    const room = p.remaining;
    if (!editor && p.capacity && n > room) {
      const over = n - room;
      total.append(el('span', 'buy-total-hint', room
        ? `Only ${room} left: ${over} of these will go on the waitlist.`
        : 'This party is full: these will go on the waitlist.'));
    }
  };
  paintTotal();
  fields.push(total);
  if (state.model.settings.ticketNote && !waiting) {
    fields.push(el('p', 'buy-note', state.model.settings.ticketNote));
  }

  let done = null;
  openModal(waiting ? `Join the waitlist for ${p.title}` : `Tickets for ${p.title}`, fields, {
    saveLabel: waiting ? 'Join Waitlist' : 'Get Tickets',
    submit: async () => {
      const payer = bills.find(b => b.email === purchaser);
      if (!payer) {
        throw new Error('Pick who to bill.');
      }
      const others = await Promise.all(added.map(a => (a.guest ? knownGuest({name: a.name, email: a.email || ''}, editor) : {id: a.id})));
      const places = [...chosen].map(email => household().find(h => h.email === email)).concat(others);
      if (!places.length) {
        throw new Error('Pick at least one person.');
      }
      done = buyEdits(p, payer.id, places, note.value.trim(), editor);
      return write(done.edits);
    },
    afterSave: () => {
      const parts = [];
      if (done.sold) {
        parts.push(`${done.sold} ${done.sold === 1 ? 'ticket' : 'tickets'} taken`);
      }
      if (done.waited) {
        parts.push(`${done.waited} on the waitlist`);
      }
      toast(parts.join(', ') || 'Done');
    },
  });
}

function waitlistEdits(p, have, want, note) {
  const viewer = me().id;
  const edits = [];
  let extras = have ? have.extra.length : 0;
  if (!have) {
    edits.push({insert: 'MEMBER', row: {group: p.waitlistGroup, person: viewer, member: 'yes', added_by: viewer, added: stamp(), note}});
  }
  for (const extra of have ? have.extra.slice(Math.max(0, want - 1)) : []) {
    edits.push({delete: extra.id});
    extras--;
  }
  for (let i = 0; extras + 1 < want; i++, extras++) {
    edits.push({insert: 'PERSON', as: 'w' + i, row: {source: 'guest'}});
    edits.push({insert: 'MEMBER', row: {group: p.waitlistGroup, person: '@w' + i, member: 'yes', added_by: viewer, added: stamp(), guest_of: have ? have.person : viewer}});
  }
  return edits;
}

export function openWaitlist(p) {
  const have = [...p.waitlisted].find(a => a.mine);
  const fields = [];
  fields.push(el('p', 'form-lead', `${p.title} is full. Say how many tickets your family would like and the hosts will give them as places open up - nothing is billed until then, and you\u2019ll get a note when it happens.`));
  const quantity = text(have ? have.quantity || 1 : 1, {type: 'number', min: 1, max: 20, step: 1, required: true});
  const qWrap = el('div', 'field-unit');
  qWrap.append(quantity, el('span', 'field-unit-label', `at ${money(p.price)} each, if a place opens up`));
  fields.push(field('How many tickets?', qWrap, `This party is for ${audienceWords(p)}.`, true));
  const note = textarea('', 2);
  if (!have) {
    note.placeholder = 'Anything the hosts should know - who it\u2019s for, dates that work (optional)';
    fields.push(field('Note', note));
  } else if (have.note) {
    fields.push(field('Note', el('p', 'buy-note', `\u201c${have.note}\u201d`)));
  }
  const asker = have ? have.addedBy || have.name : me().name;
  fields.push(el('p', 'buy-note', `If the hosts give you places, the tickets are billed to ${asker}.`));
  openModal(have ? `Your place on the waitlist for ${p.title}` : `Join the waitlist for ${p.title}`, fields, {
    saveLabel: have ? 'Update' : 'Join Waitlist',
    submit: () => {
      const edits = waitlistEdits(p, have, Math.max(1, Number(quantity.value) || 1), note.value.trim());
      return edits.length ? write(edits) : null;
    },
    afterSave: () => toast(have ? 'Waitlist request updated' : 'You\u2019re on the waitlist'),
    onDelete: have ? () => write([{delete: have.ticketId}, ...have.extra.map(x => ({delete: x.id}))]) : null,
    deleteLabel: 'Leave waitlist',
    confirmDelete: `Leave the waitlist for ${p.title}?`,
  });
}

function guestWords(p) {
  if (p.students && !p.adults) {
    return {lead: 'This party is for students, so a guest is a child who isn\u2019t in the directory - a cousin, a friend from another school.', label: 'Full name of the child', example: 'e.g., Percy Jackson'};
  }
  if (p.adults && !p.students) {
    return {lead: 'This party is for adults, so a guest is an adult who isn\u2019t in the directory - a partner, a friend, a visiting relative.', label: 'Full name of the adult', example: 'e.g., Sally Jackson'};
  }
  return {lead: 'A guest is anyone who isn\u2019t in the directory - a visiting cousin, a non-Helios sibling, a friend.', label: 'Full name', example: 'e.g., Percy Jackson'};
}

let guestListId = 0;

function guestFields(p, name, email) {
  const words = guestWords(p);
  const suggestions = el('datalist');
  suggestions.id = 'known-guests-' + ++guestListId;
  for (const g of state.model.guests) {
    const option = el('option');
    option.value = g.name;
    suggestions.append(option);
  }
  name.setAttribute('list', suggestions.id);
  const named = field(words.label, name, words.example, true);
  named.append(suggestions);
  return [
    el('p', 'form-lead', words.lead),
    named,
    field('Email address', email, 'Optional - so the hosts can reach them, e.g., percy.jackson@gmail.com'),
  ];
}

function openAddSomeone(p, editor, onAdd, opts = {}) {
  const wrap = el('div', 'add-someone');
  if (opts.lead) {
    wrap.append(el('p', 'form-lead', opts.lead));
  }
  const guestPanel = el('div');
  const name = text('', {placeholder: 'Percy Jackson', maxLength: 120, required: true});
  const email = text('', {type: 'email', placeholder: 'percy.jackson@gmail.com', maxLength: 200});
  guestPanel.append(...guestFields(p, name, email));
  const guest = () => {
    const n = name.value.trim();
    if (!n) {
      name.focus();
      return null;
    }
    const e = email.value.trim().toLowerCase();
    if (e && !e.includes('@')) {
      email.focus();
      return null;
    }
    return {guest: true, name: n, email: e};
  };
  let mode = editor ? 'directory' : 'guest';
  let chosen = null;
  let picker = null;
  const submit = () => {
    const who = mode === 'directory' ? chosen : guest();
    if (!who) {
      if (mode === 'directory') {
        picker.input.focus();
      }
      return;
    }
    onAdd(who);
    shut();
  };
  if (opts.guestExtra) {
    guestPanel.append(opts.guestExtra);
  }
  const addGuest = button('Add guest', 'plus', 'button', submit);
  if (!opts.hold) {
    guestPanel.append(addGuest);
  }
  for (const input of [name, email]) {
    input.addEventListener('keydown', ev => {
      if (ev.key === 'Enter') {
        ev.preventDefault();
        submit();
      }
    });
  }

  let directoryPanel = null;
  if (editor) {
    const kinds = [];
    if (p.adults) {
      kinds.push('an adult');
    }
    if (p.students) {
      kinds.push('a student');
    }
    const label = kinds.length === 2 ? 'Search the directory…' : `Search for ${kinds.join(' or ')}…`;
    picker = peoplePicker({placeholder: label, allow: person => inAudience(p, person), onPick: (email, person) => {
      chosen = {email, id: person ? person.id : '', name: person ? person.name_show : email, photoUrl: person ? photoOf(person) : '', title: person ? wordsOf(person) : ''};
      if (!opts.hold) {
        submit();
        return;
      }
      if (person) {
        picker.set(person);
      }
      opts.onChoose(person);
    }});
    picker.input.addEventListener('input', () => {
      chosen = null;
    });
    directoryPanel = el('div');
    directoryPanel.append(field('Who', picker.mount, `This party is for ${audienceWords(p)}.`));
  }
  if (directoryPanel) {
    const which = segmented([{label: 'From the directory', value: 'directory'}, {label: 'Guest', value: 'guest'}], 'directory', v => {
      mode = v;
      directoryPanel.hidden = v !== 'directory';
      guestPanel.hidden = v !== 'guest';
      if (opts.hold) {
        opts.onChoose(v === 'directory' && chosen ? picker.person : null);
      }
      (v === 'directory' ? picker.input : name).focus();
    });
    guestPanel.hidden = true;
    wrap.append(which.wrap);
  }
  if (opts.extra) {
    wrap.append(opts.extra);
  }
  if (directoryPanel) {
    wrap.append(directoryPanel, guestPanel);
  } else {
    wrap.append(guestPanel);
  }
  if (opts.foot) {
    wrap.append(opts.foot);
  }
  if (opts.hold) {
    const actions = el('div', 'add-someone-actions');
    if (opts.beside) {
      actions.append(opts.beside);
    }
    actions.append(button(opts.hold, 'ticket', 'button', submit));
    wrap.append(actions);
  }
  const {shut} = popup(opts.title || 'Add someone', wrap);
  const first = wrap.querySelector('input:not([hidden])');
  if (first && !opts.hold) {
    first.focus();
  }
}

export function openReassign(p, a) {
  const fields = [];
  fields.push(el('p', 'form-lead', `${a.name}'s ticket to ${p.title} goes to whoever you pick; ${a.name} comes off the list.`));
  let mode = 'directory';
  let picked = null;
  const kinds = [];
  if (p.adults) {
    kinds.push('an adult');
  }
  if (p.students) {
    kinds.push('a student');
  }
  const picker = peoplePicker({placeholder: kinds.length === 2 ? 'Search the directory…' : `Search for ${kinds.join(' or ')}…`, allow: person => inAudience(p, person)});
  const directoryPanel = field('Who', picker.mount, `This party is for ${audienceWords(p)}.`);
  const name = text('', {placeholder: 'Percy Jackson', maxLength: 120});
  const email = text('', {type: 'email', placeholder: 'percy.jackson@gmail.com', maxLength: 200});
  const guestPanel = el('div');
  guestPanel.append(...guestFields(p, name, email));
  guestPanel.hidden = true;
  const which = segmented([{label: 'From the directory', value: 'directory'}, {label: 'Guest', value: 'guest'}], mode, v => {
    mode = v;
    directoryPanel.hidden = v !== 'directory';
    guestPanel.hidden = v !== 'guest';
  });
  fields.push(which.wrap, directoryPanel, guestPanel);
  fields.push(el('p', 'buy-note', `The ticket stays billed to ${a.purchaserName || a.purchaser || 'the family that took it'}; if it was resold, that is settled between the two families.`));
  openModal(`Reassign ${a.name}'s ticket`, fields, {
    replace: true,
    saveLabel: 'Reassign',
    submit: async () => {
      if (mode === 'directory') {
        picked = picker.person;
        if (!picked) {
          throw new Error('Pick someone from the directory.');
        }
        return write([{set: a.ticketId, cells: {person: picked.id}}]);
      }
      if (!name.value.trim()) {
        throw new Error('Give the guest a name.');
      }
      const who = await knownGuest({name: name.value.trim(), email: email.value.trim().toLowerCase()}, p.can.edit);
      if (who.id) {
        return write([{set: a.ticketId, cells: {person: who.id}}]);
      }
      return write([...guestEdits('g', who), {set: a.ticketId, cells: {person: '@g'}}]);
    },
    afterSave: () => toast('Ticket reassigned'),
  });
}

function removeEdits(a) {
  return [{delete: a.ticketId}, ...(a.extra || []).map(x => ({delete: x.id}))];
}

export async function removeTicket(p, a) {
  const what = a.status === 'Ticket' ? `Remove ${a.name}'s ticket to ${p.title}?` : `Take ${a.name} off the waitlist for ${p.title}?`;
  if (!confirm(what)) {
    return;
  }
  await saved(removeEdits(a), a.status === 'Ticket' ? 'Ticket removed' : 'Off the waitlist');
}

export async function givePlace(p, a) {
  const n = a.quantity || 1;
  const payer = a.addedById || a.person;
  const places = n === 1 ? 'a ticket' : n + ' tickets';
  const billed = p.price ? `, billed to ${a.addedBy || a.name} at ${money(p.price)} each` : '';
  if (!confirm(`Give ${a.name} ${places} to ${p.title}${billed}? They come off the waitlist and are emailed their tickets.`)) {
    return;
  }
  const edits = removeEdits(a);
  edits.push(...paidPlace(p, payer, a.person, a.guestOf ? {guest_of: a.guestOf} : {}));
  for (const x of a.extra) {
    edits.push(...paidPlace(p, payer, x.person, {guest_of: a.person}));
  }
  await saved(edits, `${a.name} has ${places}`);
}

export function openAddTicket(p) {
  let paid = Boolean(p.price);
  const box = el('div', 'ticket-kind');
  const lead = el('p', 'form-lead');
  const guestOf = peoplePicker({placeholder: 'Search for an adult\u2026', allow: person => !isStudent(person)});
  const guestOfField = field('Guest of', guestOf.mount, 'Optional - who is bringing them. They get the note, and the ticket sits with their family to pass on. Blank means you.');
  const raise = p.capacity ? checkbox('Raise the capacity by one', true, `So this ticket takes none of the ${p.capacity} paid places.`) : null;
  const paidPanel = el('div');
  const invoice = peoplePicker({placeholder: 'Search for an adult\u2026', allow: person => !isStudent(person)});
  paidPanel.append(field('Invoice', invoice.mount, 'Who pays for it, and is emailed the ticket: the ticket holder, or a student\u2019s parent, unless you pick someone else. Blank, for a guest by name, bills you.'));
  const billTo = async person => {
    if (!person) {
      invoice.reset();
      return;
    }
    if (!isStudent(person)) {
      invoice.set(person);
      return;
    }
    const parent = relativesOf(person, 'parents')[0];
    if (!parent) {
      invoice.reset();
      return;
    }
    invoice.set(parent);
  };
  const show = () => {
    lead.textContent = paid
      ? `A ticket at the party\u2019s price, ${money(p.price)}, billed to whoever the Invoice picker names.`
      : 'A ticket at no charge - for a helper, a performer, a family you\u2019d like to treat. Nothing is billed.';
    guestOfField.hidden = paid;
    paidPanel.hidden = !paid;
    if (raise) {
      raise.wrap.hidden = paid;
    }
  };
  if (p.price) {
    const kind = segmented([{label: `Paid \u00b7 ${money(p.price)}`, value: 'paid'}, {label: 'Free', value: 'free'}], 'paid', v => {
      paid = v === 'paid';
      show();
    });
    box.append(kind.wrap);
  }
  box.append(lead, paidPanel);
  show();
  openAddSomeone(p, true, async picked => {
    let person = picked;
    try {
      person = picked.guest ? {...picked, ...await knownGuest(picked, true)} : picked;
    } catch (err) {
      toast(err.message);
      return;
    }
    const fresh = person.guest && !person.id;
    const edits = fresh ? guestEdits('g', person) : [];
    const holder = fresh ? '@g' : person.id;
    const host = person.guest && guestOf.person;
    const billed = invoice.person;
    if (paid) {
      edits.push(...paidPlace(p, billed ? billed.id : person.guest ? me().id : person.id, holder, person.guest ? {guest_of: me().id} : {}));
    } else {
      edits.push({insert: 'MEMBER', row: {group: p.id, person: holder, member: 'yes', added_by: me().id, added: stamp(), note: 'Free ticket from the hosts', ...(person.guest ? {guest_of: host ? host.id : me().id} : {})}});
      if (raise && raise.input.checked) {
        edits.push({set: p.id, cells: {capacity: String(p.capacity + 1)}});
      }
    }
    await saved(edits, paid ? (billed ? `${person.name} has a ticket, invoiced to ${billed.name_show}` : `${person.name} has a ticket`)
      : host ? `${person.name} has a free ticket as ${host.name_show}'s guest` : `${person.name} has a free ticket`);
  }, {title: 'Add a ticket', foot: box, beside: raise && raise.wrap, hold: 'Add Ticket', onChoose: billTo, guestExtra: guestOfField});
}

export function openTicket(p, a) {
  const form = ticketForm(p, a);
  return openPersonCard(a, [{label: 'Ticket', icon: svg('ticket'), fields: form.fields}], form);
}

function ticketForm(p, a) {
  const fields = [];
  const facts = el('div', 'ticket-facts');
  const fact = (label, value) => {
    if (!value) {
      return;
    }
    const row = el('div', 'ticket-fact');
    row.append(el('span', 'ticket-fact-label', label), el('span', '', value));
    facts.append(row);
  };
  fact('Party', p.title);
  fact('Billed to', a.purchaserName ? `${a.purchaserName} (${a.purchaser})` : a.purchaser);
  fact('Price', a.price ? money(a.price) : 'Free');
  fact('Taken', a.added);
  fact('Added by', a.addedBy);
  if (a.status === 'Waitlist') {
    fact('Tickets asked for', String(a.quantity || 1));
  }
  fact('Note', a.note);
  fields.push(facts);
  if (a.status === 'Waitlist') {
    const card = el('div', 'appoint-card');
    const words = el('div', 'setting-text');
    words.append(el('div', 'setting-label', 'Give the places'), el('div', 'setting-hint', 'They come off the waitlist and hold the tickets, billed to whoever asked, and are emailed them.'));
    const n = a.quantity || 1;
    card.append(words, button(`Give ${n === 1 ? 'a ticket' : n + ' tickets'}`, 'ticket', 'button button-small', () => {
      closeModal();
      givePlace(p, a);
    }));
    fields.push(card);
  }
  if (a.status === 'Ticket') {
    const card = el('div', 'appoint-card');
    const words = el('div', 'setting-text');
    words.append(el('div', 'setting-label', 'Reassign this ticket'), el('div', 'setting-hint', 'Give it to someone else - a sibling, or another family it was resold to.'));
    card.append(words, button('Reassign', 'people', 'button button-secondary button-small', () => openReassign(p, a)));
    fields.push(card);
  }
  return {
    fields,
    onDelete: () => write(removeEdits(a)),
    deleteLabel: 'Remove',
    confirmDelete: `Remove ${a.name} from ${p.title}?`,
  };
}

function hostChips(initial) {
  const hosts = initial.map(h => ({...h}));
  const wrap = el('div');
  const list = el('div', 'chip-pick');
  const picker = peoplePicker({placeholder: 'Add a host from the directory…'});
  const paint = () => {
    list.replaceChildren();
    for (const h of hosts) {
      const chip = personChip(h, true);
      chip.title = 'Remove';
      chip.addEventListener('click', () => {
        hosts.splice(hosts.indexOf(h), 1);
        paint();
      });
      list.append(chip);
    }
  };
  const add = button('Add', 'plus', 'button button-secondary button-small', () => {
    const email = picker.value;
    if (!email || hosts.some(h => h.email === email)) {
      return;
    }
    const person = picker.person;
    if (!person) {
      return;
    }
    hosts.push({id: person.id, email, name: person.name_show, photoUrl: photoOf(person)});
    picker.reset();
    paint();
  });
  const row = el('div', 'guest-add');
  row.append(picker.mount, add);
  paint();
  list.classList.add('chip-pick-roomy');
  wrap.append(list, row);
  return {wrap, value: () => hosts.map(h => h.id)};
}

function today() {
  return stamp().slice(0, 10);
}

function waitlistGroupEdits(ref, title, visibleTo, eligible) {
  const edits = [
    {insert: 'GROUP', as: 'wl', row: {kind: 'group', parent: ref, name: `${title} Waitlist`, status: 'open', listed: true, added_by: me().id, visible_to: visibleTo, members_visible_to: visibleTo}},
    {set: '@wl', cells: eligible ? {join: 'direct', eligible} : {join: 'direct'}},
    {set: ref, cells: {waitlist: '@wl'}},
  ];
  return edits;
}

function eligibleOf(t) {
  if (t.adults && !t.students) {
    return state.model.roleGroups.adults;
  }
  if (t.students && !t.adults) {
    return state.model.roleGroups.students;
  }
  return '';
}

function whenCells(f) {
  return {start: f.start, end: f.end, all_day: Boolean(f.start) && f.start.length === 10};
}

function ticketCells(t) {
  if (!t.adults && !t.students) {
    throw new Error('Let adults, students, or both hold a ticket.');
  }
  return {
    price: t.price ? String(t.price) : '', unit: t.unit.trim(), capacity: t.capacity ? String(t.capacity) : '', minimum: t.minimum ? String(t.minimum) : '',
    join: t.ticketsOpen ? 'direct' : 'none', eligible: eligibleOf(t), drop_off_allowed: t.students && t.dropOff, parent_ticket_required: t.adults && t.students && t.parentTicket,
  };
}

function changed(cells, row) {
  const out = {};
  for (const [k, v] of Object.entries(cells)) {
    const was = row[k] || '';
    const now = typeof v === 'boolean' ? (v ? 'Yes' : 'No') : v;
    if (now !== was && !(now === 'No' && was === '')) {
      out[k] = v;
    }
  }
  return out;
}

function hideEdits(p, hidden) {
  const shownTo = hidden ? '' : (celebration(p.celebration) || {row: {}}).row.visible_to || '';
  const edits = [{set: p.id, cells: {visible_to: shownTo}}];
  if (p.waitlistGroup) {
    edits.push({set: p.waitlistGroup, cells: {visible_to: shownTo, members_visible_to: shownTo}});
  }
  return edits;
}

function waitlistSwitch(p, on, eligible) {
  if (on && !p.waitlistGroup) {
    return waitlistGroupEdits(p.id, p.title, p.row.visible_to || '', eligible);
  }
  if (!on && p.waitlistGroup) {
    if (p.waitlisted.length) {
      throw new Error('People are waiting: give them places or take them off the waitlist before turning it off.');
    }
    return [{set: p.id, cells: {waitlist: ''}}, {delete: p.waitlistGroup}];
  }
  return [];
}

function hostEdits(p, ids) {
  const edits = [];
  for (const id of ids) {
    if (!p.hostRows.some(m => m.person === id)) {
      edits.push({insert: 'MEMBER', row: {group: p.row.managed_by, person: id, member: 'yes'}});
    }
  }
  for (const m of p.hostRows) {
    if (!ids.includes(m.person)) {
      edits.push({delete: m.id});
    }
  }
  return edits;
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

function parentOf(celebrationId, categoryId) {
  return categoryId || celebrationId;
}

async function hostParty(f, t, curates) {
  const c = celebration(f.celebration);
  const under = f.category ? category(f.category).row : c.row;
  const hidden = curates && f.status === 'Hidden';
  const cells = {
    kind: 'party', parent: under.id, name: f.title.trim(), subtitle: f.subtitle.trim(), description: f.description.trim(), slug: f.prettyId.trim().toLowerCase(),
    location: f.location.trim(), address_override: f.address.trim(), ...whenCells(f), ...ticketCells(t),
    status: curates && f.status !== 'Pending' ? 'open' : 'pending', listed: true, added_by: me().id, added: today(),
    visible_to: hidden ? '' : under.visible_to || '', members_visible_to: under.members_visible_to || '',
  };
  for (const k of Object.keys(cells)) {
    if (cells[k] === '' || cells[k] === false) {
      delete cells[k];
    }
  }
  const edits = [
    {insert: 'GROUP', as: 'p', row: cells},
    {insert: 'GROUP', as: 'm', row: {kind: 'group', name: `${cells.name} Managers`, status: 'open', added_by: me().id}},
    {set: '@m', cells: {managed_by: '@m'}},
    ...f.hostIds.map(id => ({insert: 'MEMBER', row: {group: '@m', person: id, member: 'yes'}})),
    {set: '@p', cells: {managed_by: '@m'}},
  ];
  if (t.waitlist) {
    edits.push(...waitlistGroupEdits('@p', cells.name, cells.visible_to || '', cells.eligible || ''));
  }
  const ids = await write(edits);
  return ids.result[0];
}

function partyEdits(p, f, curates) {
  const edits = [];
  const cells = changed({
    name: f.title.trim(), subtitle: f.subtitle.trim(), description: f.description.trim(), slug: f.prettyId.trim().toLowerCase(),
    location: f.location.trim(), address_override: f.address.trim(), ...whenCells(f),
  }, p.row);
  if (curates) {
    const parent = parentOf(f.celebration, f.category);
    if (parent !== p.row.parent) {
      cells.parent = parent;
    }
    if (f.status === 'Open' && p.status === 'Pending') {
      cells.status = 'open';
    }
    if (f.status === 'Pending' && p.status !== 'Pending') {
      cells.status = 'pending';
    }
  }
  if (Object.keys(cells).length) {
    edits.push({set: p.id, cells});
  }
  if (curates && (f.status === 'Hidden') !== (p.status === 'Hidden')) {
    edits.push(...hideEdits(p, f.status === 'Hidden'));
  }
  edits.push(...hostEdits(p, f.hostIds));
  return edits;
}

export function openParty(p) {
  const adding = !p;
  const user = me();
  const title = text(p ? p.title : '', {required: true, maxLength: 120, placeholder: 'Fondue & Fort Night'});
  const subtitle = text(p ? p.subtitle : '', {maxLength: 120, placeholder: 'Sweet & Savory Fondue, plus Build-Your-Own Fort'});
  const description = textarea(p ? p.row.description || '' : '', 10);
  description.placeholder = 'Everything a guest should know about the party';
  const pretty = text(p ? p.prettyId : '', {maxLength: 40, placeholder: 'fondue'});
  const prettyHint = el('small', '', '');
  const paintPretty = () => {
    const v = pretty.value.trim().toLowerCase();
    prettyHint.textContent = v ? `${location.origin}/g/${v}` : 'Lower-case letters, digits and hyphens; optional. Without one the party lives at /g/{id}.';
  };
  pretty.addEventListener('input', paintPretty);
  paintPretty();
  const prettyField = field('Friendly address', pretty);
  prettyField.append(prettyHint);
  const basics = [
    field('Title', title, '', true), field('Subtitle', subtitle),
    field('Description', description, 'Markdown works: **bold**, *italic*, - a list, [words](https://a.link). The first paragraph is the card’s summary; a line "> [!chip] Adults" puts a chip above the title, and "> [!note] 📣 Good to know" starts a tinted callout.'),
    prettyField,
  ];

  const start = whenPickers('Starts', p ? p.start : '', null, 'No start time');
  const end = whenPickers('Ends', p ? p.end : '', null, 'No end time');
  const whenWrap = el('div', 'field-when');
  whenWrap.append(start.wrap, end.wrap);
  // Named place, not location: window.location is what the address hint
  // under the friendly-address field reads.
  const place = text(p ? p.location : '', {maxLength: 120, placeholder: "The Parks' House in Los Altos"});
  const address = addressSuggest(text(p ? p.row.address_override || '' : '', {maxLength: 200, placeholder: '1420 Alder Court, Los Altos, CA 94024'}));
  const when = [
    field('When', whenWrap),
    field('Where, in words', place, 'Shown to everyone: the neighborhood or the venue, not the street.'),
    field('Street address', address, 'Shown only to signed-in Helios members, with a map link.'),
  ];

  const tickets = adding ? ticketFields(null) : null;

  const initialHosts = p ? p.hostPeople : [{id: user.id, email: user.email, name: user.name, photoUrl: user.photoUrl}];
  const hostIds = hostChips(initialHosts);
  const hosts = [
    field('Who runs it', hostIds.wrap, 'These people can edit the party and see who is coming, and the page names them as its hosts. Click a face to remove it.'),
  ];

  const panels = [
    {label: 'Basics', icon: svg('party'), fields: basics},
    {label: 'When & where', icon: svg('calendar'), fields: when},
    ...(tickets ? [{label: 'Tickets', icon: svg('ticket'), fields: tickets.fields}] : []),
    {label: 'Hosts', icon: svg('people'), fields: hosts},
  ];
  let status = null;
  let celebrationPick = null;
  let category = null;
  const curates = allows('celebrate.curate');
  if (curates) {
    status = select([{label: 'Open', value: 'Open'}, {label: 'Pending approval', value: 'Pending'}, {label: 'Hidden', value: 'Hidden'}], p ? p.status : 'Open');
    const shownCelebration = p ? p.celebration : (currentCelebration() || {}).id;
    celebrationPick = select(state.model.celebrations.map(c => ({label: c.title, value: c.id})), shownCelebration);
    category = select([], '');
    const paintCategories = () => {
      const was = category.value || (p ? p.category : '');
      category.replaceChildren(new Option('No category', ''), ...categoriesOf(celebrationPick.value).map(c => new Option(c.title, c.id)));
      category.value = categoriesOf(celebrationPick.value).some(c => c.id === was) ? was : '';
    };
    celebrationPick.addEventListener('change', paintCategories);
    paintCategories();
    panels.push({label: 'Admin', icon: svg('shield'), fields: [
      field('Status', status, 'Open is listed for everyone; Pending waits for approval; Hidden is parked.'),
      field('Celebration', celebrationPick, 'Which year the party belongs to.'),
      field('Category', category, 'For the filter on the parties page.'),
    ]});
  }
  const intro = adding && !curates ? [el('p', 'form-lead', 'Thank you for hosting! Fill this in and the celebration committee will review it and open it for tickets.')] : [];
  const fields = () => ({
    title: title.value, subtitle: subtitle.value, description: description.value, hostIds: hostIds.value(),
    start: start.value(), end: end.value(), location: place.value, address: address.value, prettyId: pretty.value,
    status: curates ? status.value : '', celebration: curates ? celebrationPick.value : state.celebration, category: curates ? category.value : '',
  });
  let made = '';
  openModal(adding ? 'Host a Party' : `Edit ${p.title}`, [...intro, tabbedFields(panels)], {
    wide: true,
    saveLabel: adding ? (curates ? 'Add Party' : 'Submit for Approval') : 'Save',
    submit: async () => {
      const f = fields();
      if (!f.hostIds.length) {
        throw new Error('A party needs at least one host.');
      }
      if (adding) {
        made = await hostParty(f, tickets.values(), curates);
        return made;
      }
      const edits = partyEdits(p, f, curates);
      return edits.length ? write(edits) : null;
    },
    afterSave: () => {
      const saved = party(adding ? made : p.id);
      if (saved) {
        navigate(partyPath(saved));
        if (adding && !curates) {
          toast('Submitted - an admin will review it');
        }
      }
    },
    onDelete: p && curates ? () => write(deleteEdits(p)) : null,
    confirmDelete: p ? `Delete ${p.title}? This cannot be undone.` : '',
    afterDelete: () => navigate('/'),
  });
}

function deleteEdits(p) {
  if (p.attendees.length || p.waitlisted.length) {
    throw new Error('Remove its tickets and waitlist first, or hide it instead.');
  }
  const edits = [];
  if (p.image) {
    edits.push({delete: p.image});
  }
  if (p.waitlistGroup) {
    edits.push({set: p.id, cells: {waitlist: ''}}, {delete: p.waitlistGroup});
  }
  for (const answers of [p.row.rsvp_yes, p.row.rsvp_no].filter(Boolean)) {
    edits.push({set: p.id, cells: {[answers === p.row.rsvp_yes ? 'rsvp_yes' : 'rsvp_no']: ''}}, {delete: answers});
  }
  edits.push({delete: p.id});
  return edits;
}

function ticketFields(p) {
  const price = text(p ? p.price : '', {type: 'number', min: 0, step: '0.01', required: true, placeholder: '65'});
  const unit = text(p ? p.unit : '', {maxLength: 40, placeholder: 'person, adult, child, family'});
  const capacity = text(p && p.capacity ? p.capacity : '', {type: 'number', min: 1, step: 1, placeholder: 'Leave blank for no limit'});
  const minimum = text(p && p.minimum ? p.minimum : '', {type: 'number', min: 1, step: 1, placeholder: 'Leave blank for none'});
  const ticketsOpen = checkbox('Tickets on sale', p ? p.ticketsOpen : true, 'When off, the party is listed but doesn’t sell tickets.');
  const waitlist = checkbox('Take a waitlist when full', p ? p.waitlist : true, 'When on, a full party shows Sold Out and allows a waitlist.');
  const adults = checkbox('Adults', p ? p.adults : true, 'Parents and staff can hold a ticket.');
  const students = checkbox('Students', p ? p.students : false, 'Students can hold a ticket.');
  const dropOff = checkbox('Drop-off is okay', p ? p.dropOff : false, 'Kids can come without a parent.');
  const parentTicket = checkbox('A parent who stays needs a ticket', p ? p.parentTicket : false, 'If a parent attends with their child, they need a ticket too.');

  const priceInput = el('div', 'tix-money');
  priceInput.append(el('span', 'tix-money-sign', '$'), price);
  const summary = el('div', 'tix-price-summary');
  const paintSummary = () => {
    const cost = Number(price.value || 0);
    summary.replaceChildren(
      el('div', 'tix-price-line', cost ? `${money(cost)} per ${unit.value.trim() || 'ticket'}` : 'Free'),
      el('div', 'tix-price-note', cost ? `Each ticket costs ${money(cost)}.` : 'Nothing is billed.'),
    );
  };
  price.addEventListener('input', paintSummary);
  unit.addEventListener('input', paintSummary);
  paintSummary();
  const priceBox = el('div', 'tix-price');
  priceBox.append(field('Price per ticket', priceInput, p && p.raised > 0 ? 'Tickets already bought keep the price they were bought at.' : '', true), summary);

  unit.setAttribute('list', 'tix-units');
  const units = el('datalist');
  units.id = 'tix-units';
  for (const word of ['Adult', 'Child', 'Person', 'Family']) {
    const option = el('option');
    option.value = word;
    units.append(option);
  }
  const unitBox = el('div', 'tix-icon-input');
  unitBox.append(svg('person'), unit, units);

  const iconLabel = (icon, words) => {
    const label = el('span', 'tix-label');
    label.append(svg(icon), el('span', '', words));
    return label;
  };
  const counts = el('div', 'tix-counts');
  counts.append(
    field(iconLabel('ticket', 'Tickets available'), capacity, 'Total number of tickets you can sell.'),
    field(iconLabel('people', 'Minimum to hold the party'), minimum, 'The party goes ahead only with at least this many tickets sold.'),
  );

  const saleRow = (icon, box) => {
    box.wrap.classList.add('tix-row');
    box.wrap.prepend(svg(icon));
    return box.wrap;
  };
  const sale = el('div', 'tix-sale');
  sale.append(saleRow('tag', ticketsOpen), saleRow('people', waitlist));

  const stay = el('div', 'tix-stay');
  stay.append(el('div', 'tix-stay-title', 'Drop-off or parents stay?'), dropOff.wrap, parentTicket.wrap);
  const paintStay = () => {
    stay.hidden = !students.input.checked;
    parentTicket.wrap.hidden = !adults.input.checked;
  };
  students.input.addEventListener('change', paintStay);
  adults.input.addEventListener('change', paintStay);
  paintStay();
  const who = el('div', 'tix-who');
  who.append(el('div', 'field-group-label', 'Who can come'), adults.wrap, students.wrap, stay);

  return {
    fields: [
      priceBox,
      field('One ticket covers', unitBox, 'e.g. Adult, Child, or a description like “Family (up to 4)”.'),
      counts, sale, who,
    ],
    values: () => ({
      unit: unit.value, price: Number(price.value || 0), capacity: Number(capacity.value || 0), minimum: Number(minimum.value || 0),
      ticketsOpen: ticketsOpen.input.checked, waitlist: waitlist.input.checked, adults: adults.input.checked,
      students: students.input.checked, dropOff: dropOff.input.checked, parentTicket: parentTicket.input.checked,
    }),
  };
}

export function openTickets(p) {
  const tickets = ticketFields(p);
  openModal(`Edit tickets for ${p.title}`, tickets.fields, {
    submit: () => {
      const t = tickets.values();
      const wanted = ticketCells(t);
      const cells = changed(wanted, p.row);
      const edits = [...(Object.keys(cells).length ? [{set: p.id, cells}] : []), ...waitlistSwitch(p, t.waitlist, wanted.eligible)];
      if (cells.eligible !== undefined && p.waitlistGroup) {
        edits.push({set: p.waitlistGroup, cells: {eligible: cells.eligible}});
      }
      return edits.length ? write(edits) : null;
    },
  });
}

export async function savePartyFields(p, changes) {
  try {
    if (changes.image !== undefined) {
      if (changes.image) {
        await setPicture(p.id, changes.image, p.image);
      } else if (p.image) {
        await write([{delete: p.image}]);
      }
    }
    if (changes.flyer !== undefined) {
      await write([{set: p.id, cells: {flyer: changes.flyer}}]);
    }
    await load();
  } catch (err) {
    toast(err.message);
  }
}

export async function setPartyStatus(p, status) {
  await saved(status === 'Open' ? [{set: p.id, cells: {status: 'open'}}] : hideEdits(p, true), status === 'Open' ? `${p.title} is open` : `${p.title} is hidden`);
}

export async function openContacts(p) {
  await listed();
  const signUps = [...p.attendees, ...p.waitlisted].map(a => ({a, relation: ''}));
  const relations = new Set();
  const firstName = a => (a.name || a.email).split(' ')[0];
  const follows = {Parents: ['parents', 'Parent'], Children: ['children', 'Child'], Siblings: ['siblings', 'Sibling']};
  const familyOf = r => {
    const info = known(r.a.email);
    if (!info) {
      return [];
    }
    const out = [];
    for (const [relation, [path, as]] of Object.entries(follows)) {
      if (!relations.has(relation)) {
        continue;
      }
      for (const q of relativesOf(info, path).filter(emailOf)) {
        out.push({a: {email: emailOf(q), name: q.name_show}, relation: `${as} of ${firstName(r.a)}`});
      }
    }
    return out;
  };
  const titleOf = r => {
    const info = known(r.a.email);
    if (!info) {
      return r.relation ? '' : 'Guest';
    }
    if (isStudent(info)) {
      return gradeOf(info) || 'Student';
    }
    return [isParent(info) ? 'Parent' : '', isStaff(info) ? 'Staff' : ''].filter(Boolean).join(', ');
  };
  const mailLink = e => {
    const link = el('a', 'contact-email', e);
    link.href = `mailto:${e}`;
    return link;
  };
  const statusOf = a => {
    if (a.status === 'Waitlist') {
      return `${a.status} (${a.quantity || 1})`;
    }
    if (a.status === 'Ticket' && !a.price) {
      return 'Free ticket';
    }
    return a.status || '';
  };
  const columns = [
    {label: 'Name', get: r => r.a.name || r.a.email || ''},
    {label: 'Title', get: titleOf},
    {label: 'Description', get: r => r.relation || r.a.line || ''},
    {label: 'Email', get: r => r.a.email || '', show: r => (r.a.email ? mailLink(r.a.email) : '')},
    {label: 'Status', get: r => statusOf(r.a)},
    ...(p.invited ? [{label: 'RSVP', get: r => ({yes: 'Yes', maybe: 'Maybe', no: 'No', none: 'No RSVP yet'})[r.a.rsvp] || ''}] : []),
    {label: 'Purchaser', get: r => r.a.purchaserName || r.a.purchaser || ''},
    {label: 'Purchaser Email', get: r => r.a.purchaser || '', show: r => (r.a.purchaser ? mailLink(r.a.purchaser) : '')},
    {label: 'Note', get: r => r.a.note || ''},
  ];
  const holder = el('div');
  const count = el('span', 'contact-count');
  const paint = () => {
    const seen = new Set(signUps.map(r => r.a.email).filter(Boolean));
    const rows = [...signUps];
    for (const r of signUps) {
      for (const k of familyOf(r)) {
        if (!seen.has(k.a.email)) {
          seen.add(k.a.email);
          rows.push(k);
        }
      }
    }
    holder.replaceChildren(rows.length ? dataGrid({columns, rows}).wrap : el('div', 'panel-empty', 'Nobody is coming yet.'));
    count.textContent = `${rows.length} ${rows.length === 1 ? 'person' : 'people'}`;
  };
  const tools = el('div', 'contact-tools');
  tools.append(count, familyDropdown(['Parents', 'Children', 'Siblings'], relations, paint));
  paint();
  openModal(`Who's coming to ${p.title}`, [tools, holder], {wide: 'table'});
}

export function openCelebration(c) {
  const code = text(c ? c.code : '', {required: true, maxLength: 20, placeholder: 'SC-2027'});
  const title = text(c ? c.title : '', {required: true, maxLength: 120, placeholder: 'Helios Spring Celebration 2027'});
  const subtitle = text(c ? c.subtitle : '', {maxLength: 120, placeholder: 'The theme'});
  const start = whenPickers('Starts', c ? c.start : '', null, 'No start time');
  const end = whenPickers('Ends', c ? c.end : '', null, 'No end time');
  const whenWrap = el('div', 'field-when');
  whenWrap.append(start.wrap, end.wrap);
  const place = text(c ? c.location : '', {maxLength: 120});
  const address = text(c ? c.row.address_override || '' : '', {maxLength: 200});
  const description = textarea(c ? c.description : '', 4);
  const hosting = checkbox('Takes parties', c ? c.adding === 'approval' : false, 'Anyone may host a party under it, which waits for an admin’s approval. Off, only admins add parties.');
  const current = checkbox('This is the current celebration', c ? c.current : true, 'Its parties are what the parties page lists first.');
  const banner = checkbox('Show its banner at the top', c ? c.id === state.model.banner : false, 'The band across the top of the parties page advertises it, whichever year\u2019s parties are listed. Only one celebration is the banner.');
  const image = imagePicker(c ? c.image : '', c ? c.imageUrl : '', {dropzone: true, query: () => title.value, label: 'Banner image', hint: 'The background of the banner across the top of the parties page.'});
  openModal(c ? `Edit ${c.title}` : 'Add a celebration', [
    field('Code', code, 'Short and unique, like SC-2027: the celebration’s address on the site.', true), field('Title', title, 'The banner\u2019s big line: "Helios Spring Celebration 2026".', true),
    field('Subtitle', subtitle, 'The banner\u2019s small line above it: the theme.'),
    field('When', whenWrap), field('Where', place), field('Address', address),
    field('Description', description, 'Markdown works. Each link in it, like [Learn More](https://celebrate.heliosschool.org), is a button on the banner; Add to Calendar is there while the celebration is still to come.'),
    current.wrap, banner.wrap, hosting.wrap, image.wrap,
  ], {
    submit: async () => {
      const cells = {
        slug: code.value.trim().toLowerCase(), name: title.value.trim(), subtitle: subtitle.value.trim(), ...whenCells({start: start.value(), end: end.value()}),
        location: place.value.trim(), address_override: address.value.trim(), description: description.value.trim(), adding: hosting.input.checked ? 'approval' : '',
      };
      let id = c ? c.id : '@c';
      const edits = [];
      if (c) {
        const diff = changed(cells, c.row);
        if (Object.keys(diff).length) {
          edits.push({set: c.id, cells: diff});
        }
      } else {
        const top = state.model.top;
        if (!top) {
          throw new Error('There is no Celebrate category to file a celebration under.');
        }
        edits.push({insert: 'GROUP', as: 'c', row: {...cells, kind: 'celebration', parent: top.id, status: 'open', added_by: me().id, visible_to: top.visible_to || '', members_visible_to: top.members_visible_to || ''}});
      }
      if (current.input.checked && state.model.current !== (c && c.id)) {
        edits.push(settingEdit('Current', id));
      }
      if (banner.input.checked && state.model.banner !== (c && c.id)) {
        edits.push(settingEdit('Banner', id));
      }
      if (edits.length) {
        const written = await write(edits);
        if (!c) {
          id = written.result[0];
        }
      }
      const picked = image.value();
      if (picked && picked !== (c ? c.image : '')) {
        await setPicture(id, picked, c ? c.image : '');
      } else if (!picked && c && c.image) {
        await write([{delete: c.image}]);
      }
    },
    onDelete: c ? () => write([...(c.image ? [{delete: c.image}] : []), {delete: c.id}]) : null,
    confirmDelete: c ? `Delete ${c.title}? Only a celebration with no parties or categories can be deleted.` : '',
  });
}

function settingEdit(key, value) {
  const row = state.model.settingRows.find(s => s.key === key);
  return row ? {set: row.id, cells: {value}} : {insert: 'SETTING', row: {app: 'celebrate', key, value}};
}

export async function saveSetting(key, value) {
  await write([settingEdit(key, value)]);
}

export function openCategory(c, celebrationId, after) {
  const input = text(c ? c.title : '', {required: true, maxLength: 120});
  const under = celebration(celebrationId);
  openModal(c ? 'Rename category' : `Add a category to ${under.title}`, [field('Title', input, '', true)], {
    submit: () => {
      if (c) {
        return write([{set: c.id, cells: {name: input.value.trim()}}]);
      }
      const last = categoriesOf(celebrationId).at(-1);
      return write([{insert: 'GROUP', row: {
        kind: 'category', parent: celebrationId, name: input.value.trim(), status: 'open', listed: true, added_by: me().id,
        order: keyBetween(last ? last.order : undefined, undefined), visible_to: under.row.visible_to || '', members_visible_to: under.row.members_visible_to || '',
      }}]);
    },
    afterSave: after,
    onDelete: c ? () => write([{delete: c.id}]) : null,
    confirmDelete: c ? `Delete the category ${c.title}? Only one with no parties filed under it can be deleted.` : '',
    afterDelete: after,
  });
}

export function openSettings() {
  const s = state.model.settings;
  const intro = textarea(s.partiesIntro, 3);
  const note = textarea(s.ticketNote, 4);
  openModal('Settings', [
    field('Parties intro', intro, 'The line under the parties page heading.'),
    field('Ticket note', note, 'Shown on the ticket form: how invoicing works, the refund policy.'),
  ], {
    submit: () => write([settingEdit('Parties Intro', intro.value.trim()), settingEdit('Ticket Note', note.value.trim())]),
  });
}
