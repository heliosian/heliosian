import {state, me, isAdmin, household, billable, admits, audienceWords, ticketFor, money, currentCelebration, partyPath, party} from './state.js';
import {addressSuggest} from '/address.js';
import {createPersonPicker} from '/picker.js';
import {directory, listed} from '/directory.js';
import {whoLink} from '/appswitch.js';
import {api} from '/api.js';
import {el, svg, toast, button, avatar} from '/elements.js';
import {tabbedFields} from '/tabs.js';
import {imageTools} from '/images.js';
import {openModal, closeModal, popup} from '/modal.js';
import {load, navigate} from '/router.js';
import {field, text, textarea, select, checkbox, segmented, whenPickers} from '/form.js';
import {dataGrid} from '/datagrid.js';
import {familyDropdown} from '/rules.js';

export const {uploadImage, imageSearchOn, openImageSearch, imagePicker} = imageTools('/api/celebrate', {state});

function peoplePicker(options) {
  return createPersonPicker(el('div'), {people: listed, address: true, ...options});
}

async function personInfo(email) {
  const dir = await directory();
  return {dir, info: dir.result.map(dir.get).find(p => p.email === email) || null};
}

export async function openPerson(v) {
  const {dir, info} = await personInfo(v.email);
  const done = el('button', 'button button-secondary', 'Done');
  done.type = 'button';
  done.addEventListener('click', closeModal);
  openModal('', [personHead(v, info), personContact(dir, v, info), personFoot(v, info, done)], {actions: false, wide: 'person'});
}

function personHead(v, info) {
  const head = el('div', 'who-head');
  const face = avatar({name: (info && info.fullName) || v.name, email: v.email, photoUrl: (info && info.heroPhotoUrl) || v.photoUrl}, 'who-face');
  const names = el('div', 'who-names');
  names.append(el('div', 'who-name', (info && info.fullName) || v.name || v.email));
  if (info && info.pronouns) {
    names.append(el('div', 'who-sub', info.pronouns));
  }
  if (info) {
    const place = info.isStudent
      ? [info.grade, info.classroom].filter(Boolean).join(' · ')
      : [info.jobTitle, info.department].filter(Boolean).join(' · ') || info.words;
    if (place) {
      names.append(el('div', 'who-sub', place));
    }
  } else if (!v.email) {
    names.append(el('div', 'who-sub', 'Guest'));
  }
  const actions = el('div', 'who-actions');
  const action = (icon, label, href, onClick) => {
    const a = el(href ? 'a' : 'button', 'who-action');
    if (href) {
      a.href = href;
    } else {
      a.type = 'button';
      a.addEventListener('click', onClick);
    }
    a.title = label;
    const round = el('span', 'icon-round');
    round.append(svg(icon));
    a.append(round, el('span', 'who-action-label', label));
    actions.append(a);
  };
  if (v.email) {
    action('mail', 'Email', `mailto:${v.email}`);
  }
  const phone = info && info.phone ? info.phone : '';
  const digits = phone.replace(/[^+\d]/g, '');
  if (digits) {
    action('chat', 'Message', `sms:${digits}`);
    action('phone', 'Call', `tel:${digits}`);
  }
  if (v.email) {
    action('copy', 'Copy info', null, () => {
      const lines = [(info && info.fullName) || v.name || '', v.email, phone].filter(Boolean);
      navigator.clipboard.writeText(lines.join('\n')).then(() => toast('Contact info copied'), () => toast('Could not copy'));
    });
  }
  head.append(face, names, actions);
  return head;
}

function personContact(dir, v, info) {
  const card = el('div', 'who-rows');
  let group = null;
  const row = (icon, label, value) => {
    if (!group) {
      group = el('div', 'who-group');
      card.append(group);
    }
    const r = el('div', 'who-row');
    r.append(svg(icon), el('span', 'who-label', label), typeof value === 'string' ? el('span', 'who-value', value) : value);
    group.append(r);
  };
  if (v.email) {
    row('mail', 'Email', v.email);
  }
  if (info && info.phone) {
    row('phone', 'Phone', info.phone);
  }
  const chips = list => {
    const wrap = el('div', 'who-card-chips');
    for (const p of list) {
      const chip = el('button', 'who-card-chip', p.grade ? `${p.fullName} (${p.grade})` : p.fullName);
      chip.type = 'button';
      chip.addEventListener('click', () => openPerson({email: p.email, name: p.fullName, photoUrl: p.heroPhotoUrl}));
      wrap.append(chip);
    }
    return wrap;
  };
  const partners = info ? dir.follow(info, 'partners').filter(Boolean) : [];
  const children = info ? dir.follow(info, 'children').filter(Boolean) : [];
  const household = info && (partners.length || children.length || (info.isStudent && info.parentContactEmails && info.parentContactEmails.length));
  if (household) {
    group = null;
  }
  if (partners.length) {
    row('people', partners.length === 1 ? 'Partner' : 'Partners', chips(partners));
  }
  if (children.length) {
    row('person', children.length === 1 ? 'Child' : 'Children', chips(children));
  }
  if (info && info.isStudent && info.parentContactEmails && info.parentContactEmails.length) {
    const parents = el('div');
    for (const e of info.parentContactEmails) {
      const a = el('a', 'who-card-link', e);
      a.href = `mailto:${e}`;
      parents.append(a);
    }
    row('people', 'Parents', parents);
  }
  if (!card.children.length) {
    row('person', 'About', 'A guest, not in the directory.');
  }
  return card;
}

function personFoot(v, info, done) {
  const foot = el('div', 'who-foot');
  if (info) {
    const profile = el('a', 'button', '');
    profile.append(svg('open'), el('span', '', 'Open Helios Who? Profile'));
    profile.href = whoLink(v.email);
    profile.target = '_blank';
    profile.rel = 'noopener';
    foot.append(profile);
  }
  if (done) {
    foot.append(done);
  }
  return foot.children.length ? foot : el('div');
}

function personChip(person, on, disabledWhy) {
  const chip = el('button', 'person-chip' + (on ? ' is-on' : ''));
  chip.type = 'button';
  chip.append(avatar(person, 'person-chip-face'));
  const words = el('span', 'person-chip-text');
  words.append(el('span', 'person-chip-name', person.name));
  if (person.grade || person.title) {
    words.append(el('span', 'person-chip-line', person.grade || person.title));
  }
  chip.append(words);
  if (disabledWhy) {
    chip.disabled = true;
    chip.title = disabledWhy;
  }
  return chip;
}

export function openBuy(p) {
  const editor = p.canEdit;
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
      if (have) {
        why = have.status === 'Ticket' ? `${person.name} already has a ticket` : `${person.name} is already on the waitlist`;
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
    if (!editor && p.remaining >= 0 && n > p.remaining) {
      const over = n - p.remaining;
      total.append(el('span', 'buy-total-hint', p.remaining
        ? `Only ${p.remaining} left: ${over} of these will go on the waitlist.`
        : 'This party is full: these will go on the waitlist.'));
    }
  };
  paintTotal();
  fields.push(total);
  if (state.model.settings.ticketNote && !waiting) {
    fields.push(el('p', 'buy-note', state.model.settings.ticketNote));
  }

  openModal(waiting ? `Join the waitlist for ${p.title}` : `Tickets for ${p.title}`, fields, {
    saveLabel: waiting ? 'Join Waitlist' : 'Get Tickets',
    submit: async () => {
      if (!purchaser) {
        throw new Error('Pick who to bill.');
      }
      const attendees = [...chosen].map(email => ({email}))
        .concat(added.map(a => (a.guest ? {name: a.name, email: a.email || ''} : {email: a.email})));
      if (!attendees.length) {
        throw new Error('Pick at least one person.');
      }
      return api('POST', '/api/celebrate/tickets', {partyId: p.id, purchaser, note: note.value, attendees});
    },
    afterSave: result => {
      const parts = [];
      if (result.sold) {
        parts.push(`${result.sold} ${result.sold === 1 ? 'ticket' : 'tickets'} taken`);
      }
      if (result.waitlisted) {
        parts.push(`${result.waitlisted} on the waitlist`);
      }
      toast(parts.join(', ') || 'Done');
    },
  });
}

export function openWaitlist(p) {
  const bills = billable();
  const purchaser = bills.length ? bills[0].email : '';
  const have = [...p.waitlisted].find(a => a.mine);
  const fields = [];
  fields.push(el('p', 'form-lead', `${p.title} is full. Say how many tickets your family would like and the hosts will offer them as places open up - nothing is billed until then, and you\u2019ll get a note when it happens.`));
  const quantity = text(have ? have.quantity || 1 : 1, {type: 'number', min: 1, max: 20, step: 1, required: true});
  const qWrap = el('div', 'field-unit');
  qWrap.append(quantity, el('span', 'field-unit-label', `at ${money(p.price)} each, if a place opens up`));
  fields.push(field('How many tickets?', qWrap, `This party is for ${audienceWords(p)}.`, true));
  const note = textarea(have ? have.note || '' : '', 2);
  note.placeholder = 'Anything the hosts should know - who it\u2019s for, dates that work (optional)';
  fields.push(field('Note', note));
  const billed = bills.find(b => b.email === purchaser);
  if (billed) {
    fields.push(el('p', 'buy-note', `If the hosts offer you places, the tickets are billed to ${billed.name}.`));
  }
  openModal(have ? `Your place on the waitlist for ${p.title}` : `Join the waitlist for ${p.title}`, fields, {
    saveLabel: have ? 'Update' : 'Join Waitlist',
    submit: () => api('POST', '/api/celebrate/waitlist', {partyId: p.id, purchaser, quantity: Number(quantity.value) || 1, note: note.value}),
    afterSave: () => toast(have ? 'Waitlist request updated' : 'You\u2019re on the waitlist'),
    onDelete: have ? () => api('DELETE', '/api/celebrate/ticket', {ticketId: have.ticketId}) : null,
    deleteLabel: 'Leave waitlist',
    confirmDelete: `Leave the waitlist for ${p.title}?`,
  });
}

const calloutEmoji = ['📣', '⚠️', 'ℹ️', '⭐', '🎉', '🎈', '🎁', '🍫', '🍕', '🍷', '🍸', '🧁', '🎂', '🎶', '🎮', '🏊', '🌧️', '☀️', '👟', '🧥', '🚗', '🅿️', '🐶', '🧒', '👨‍👩‍👧', '🔥', '💡', '❤️', '✅', '🕒'];

function emojiPicker(value) {
  const input = text(value || '', {maxLength: 16, placeholder: '📣'});
  input.className = 'note-emoji-input';
  input.setAttribute('aria-label', 'Emoji');
  const wrap = el('div', 'emoji-pick');
  const caret = el('button', 'emoji-pick-caret');
  caret.type = 'button';
  caret.setAttribute('aria-label', 'Choose an emoji');
  caret.append(svg('chevron-down'));
  const menu = el('div', 'emoji-pick-menu');
  menu.hidden = true;
  for (const e of calloutEmoji) {
    const b = el('button', 'emoji-pick-item', e);
    b.type = 'button';
    b.addEventListener('click', () => {
      input.value = e;
      menu.hidden = true;
      input.dispatchEvent(new Event('input', {bubbles: true}));
    });
    menu.append(b);
  }
  caret.addEventListener('click', e => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
    if (!menu.hidden) {
      const box = wrap.getBoundingClientRect();
      menu.classList.toggle('is-up', window.innerHeight - box.bottom < 280);
      document.addEventListener('click', () => {
        menu.hidden = true;
      }, {once: true});
    }
  });
  menu.addEventListener('click', e => e.stopPropagation());
  wrap.append(input, caret, menu);
  return {wrap, input};
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

function guestFields(p, name, email) {
  const words = guestWords(p);
  return [
    el('p', 'form-lead', words.lead),
    field(words.label, name, words.example, true),
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
  const addGuest = button('Add guest', 'plus', 'button', () => {
    const n = name.value.trim();
    if (!n) {
      name.focus();
      return;
    }
    const e = email.value.trim().toLowerCase();
    if (e && !e.includes('@')) {
      email.focus();
      return;
    }
    onAdd({guest: true, name: n, email: e});
    shut();
  });
  guestPanel.append(addGuest);
  name.addEventListener('keydown', ev => {
    if (ev.key === 'Enter') {
      ev.preventDefault();
      addGuest.click();
    }
  });
  email.addEventListener('keydown', ev => {
    if (ev.key === 'Enter') {
      ev.preventDefault();
      addGuest.click();
    }
  });

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
    const picker = peoplePicker({placeholder: label, allow: person => admits(p, person), onPick: person => {
      onAdd({email: person.email, name: person.fullName || person.email, photoUrl: person.heroPhotoUrl, title: person.words});
      shut();
    }});
    directoryPanel = el('div');
    directoryPanel.append(field('Who', picker.mount, `This party is for ${audienceWords(p)}.`));
  }
  if (directoryPanel) {
    const which = segmented([{label: 'From the directory', value: 'directory'}, {label: 'Guest', value: 'guest'}], 'directory', v => {
      directoryPanel.hidden = v !== 'directory';
      guestPanel.hidden = v !== 'guest';
      (v === 'directory' ? directoryPanel.querySelector('input') : name).focus();
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
  const {shut} = popup(opts.title || 'Add someone', wrap);
  const first = wrap.querySelector('input:not([hidden])');
  if (first) {
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
  const picker = peoplePicker({placeholder: kinds.length === 2 ? 'Search the directory…' : `Search for ${kinds.join(' or ')}…`, allow: person => admits(p, person)});
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
      const body = {ticketId: a.ticketId};
      if (mode === 'directory') {
        picked = picker.value;
        if (!picked) {
          throw new Error('Pick someone from the directory.');
        }
        body.email = picked;
      } else {
        if (!name.value.trim()) {
          throw new Error('Give the guest a name.');
        }
        body.name = name.value.trim();
        body.email = email.value.trim().toLowerCase();
      }
      return api('POST', '/api/celebrate/ticket/reassign', body);
    },
    afterSave: () => toast('Ticket reassigned'),
  });
}

export function openMoveAddress(a, onDone) {
  const to = text('', {type: 'email', placeholder: 'percy.jackson@gmail.com', maxLength: 200});
  const name = text(a.name || '', {placeholder: 'Percy Jackson', maxLength: 120});
  const fields = [
    el('p', 'form-lead', `${a.email} moves to the new address on every party - tickets, waitlist requests and guest lists alike - and anyone already sent an invitation there is sent it again.`),
    field('New email address', to, null, true),
    field('Name', name, 'The directory no longer holds them, so this is what the parties call them.'),
  ];
  openModal(`Change ${a.name}'s address`, fields, {
    replace: true,
    saveLabel: 'Change address',
    submit: () => {
      if (!to.value.trim()) {
        throw new Error('Give the new address.');
      }
      return api('POST', '/api/celebrate/address', {old: a.email, to: to.value.trim().toLowerCase(), name: name.value.trim()});
    },
    afterSave: () => {
      toast('Address changed on every party');
      if (onDone) {
        onDone();
      }
    },
  });
}

export async function removeTicket(p, a) {
  const what = a.status === 'Ticket' ? `Remove ${a.name}'s ticket to ${p.title}?` : `Take ${a.name} off the waitlist for ${p.title}?`;
  if (!confirm(what)) {
    return;
  }
  try {
    await api('DELETE', '/api/celebrate/ticket', {ticketId: a.ticketId});
    await load();
    toast(a.status === 'Ticket' ? 'Ticket removed' : 'Off the waitlist');
  } catch (err) {
    toast(err.message);
  }
}

export async function offerTickets(p, a, quantity) {
  const n = quantity || a.quantity || 1;
  if (!confirm(`Offer ${a.name} ${n === 1 ? 'a ticket' : n + ' tickets'} to ${p.title}? They\u2019ll be billed and told by email.`)) {
    return;
  }
  try {
    await api('POST', '/api/celebrate/waitlist/offer', {ticketId: a.ticketId, quantity: n});
    await load();
    toast(`${a.name} now has ${n === 1 ? 'a ticket' : n + ' tickets'}`);
  } catch (err) {
    toast(err.message);
  }
}

export function openFreeTicket(p) {
  const guestOf = peoplePicker({placeholder: 'Search for an adult\u2026', allow: person => !person.isStudent});
  const extra = el('div');
  extra.append(field('Guest of', guestOf.mount, 'Optional - who is bringing them. They get the note, and the ticket sits with their family to pass on. Blank means you.'));
  const raise = p.capacity ? checkbox('Raise the capacity by one', true, `So this ticket takes none of the ${p.capacity} paid places.`) : null;
  if (raise) {
    extra.append(raise.wrap);
  }
  openAddSomeone(p, true, async person => {
    try {
      await api('POST', '/api/celebrate/tickets', {
        partyId: p.id, free: true, purchaser: guestOf.value, raiseCapacity: Boolean(raise && raise.input.checked), note: 'Free ticket from the hosts',
        attendees: [{email: person.email || '', name: person.guest ? person.name : ''}],
      });
      await load();
      const host = guestOf.person;
      toast(host ? `${person.name} has a free ticket as ${host.fullName}'s guest` : `${person.name} has a free ticket`);
    } catch (err) {
      toast(err.message);
    }
  }, {title: 'Add a free ticket', lead: 'A ticket at no charge - for a helper, a performer, a family you\u2019d like to treat. Nothing is billed.', extra});
}

export async function openTicket(p, a) {
  const {dir, info} = await personInfo(a.email);
  const form = ticketForm(p, a);
  const tabs = tabbedFields([
    {label: 'Contact', icon: svg('people'), fields: [personContact(dir, a, info), personFoot(a, info, null)]},
    {label: 'Ticket', icon: svg('ticket'), fields: form.fields},
  ]);
  openModal('', [personHead(a, info), tabs], {...form, wide: 'person'});
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
  fields.push(facts);
  const note = textarea(a.note || '', 2);
  fields.push(field('Note', note));
  let quantity = null;
  if (a.status === 'Waitlist') {
    quantity = text(a.quantity || 1, {type: 'number', min: 1, step: 1});
    fields.push(field('Tickets asked for', quantity));
    const card = el('div', 'appoint-card');
    const words = el('div', 'setting-text');
    words.append(el('div', 'setting-label', 'Offer the tickets'), el('div', 'setting-hint', 'They get that many tickets at the price they were asked for, billed to their family, and a note saying so.'));
    const n = a.quantity || 1;
    card.append(words, button(`Offer ${n === 1 ? 'a ticket' : n + ' tickets'}`, 'ticket', 'button button-small', () => {
      closeModal();
      offerTickets(p, a, Number(quantity.value) || n);
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
  if (isAdmin() && a.email && a.kind === 'Guest') {
    const card = el('div', 'appoint-card');
    const words = el('div', 'setting-text');
    words.append(el('div', 'setting-label', 'Change their address'), el('div', 'setting-hint', 'An alum whose school address closed, or any address that no longer reaches them - on every party at once.'));
    card.append(words, button('Change address', 'mail', 'button button-secondary button-small', () => openMoveAddress(a)));
    fields.push(card);
  }
  return {
    fields,
    submit: () => {
      const body = {ticketId: a.ticketId, note: note.value};
      if (quantity) {
        body.quantity = Number(quantity.value) || 1;
      }
      return api('POST', '/api/celebrate/ticket', body);
    },
    onDelete: () => api('DELETE', '/api/celebrate/ticket', {ticketId: a.ticketId}),
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
    const person = picker.person || {};
    hosts.push({email, name: person.fullName || email, photoUrl: person.heroPhotoUrl});
    picker.reset();
    paint();
  });
  const row = el('div', 'guest-add');
  row.append(picker.mount, add);
  paint();
  list.classList.add('chip-pick-roomy');
  wrap.append(list, row);
  return {wrap, value: () => hosts.map(h => h.email)};
}

export function openParty(p) {
  const adding = !p;
  const user = me();
  const title = text(p ? p.title : '', {required: true, maxLength: 120, placeholder: 'Fondue & Fort Night'});
  const subtitle = text(p ? p.subtitle : '', {maxLength: 120, placeholder: 'Sweet & Savory Fondue, plus Build-Your-Own Fort'});
  const summary = textarea(p ? p.summary : '', 2);
  summary.placeholder = 'One or two sentences for the party card';
  const description = textarea(p ? p.description : '', 6);
  description.placeholder = 'Everything a guest should know about the party';
  const needToKnow = textarea(p ? p.needToKnow : '', 2);
  needToKnow.placeholder = 'Adults only; bring a swimsuit; drop-off is fine';
  const emojiPick = emojiPicker(p ? p.noteEmoji : '');
  const noteEmoji = emojiPick.input;
  const noteTitle = text(p ? p.noteTitle : '', {maxLength: 60, placeholder: 'Good to know'});
  noteTitle.setAttribute('aria-label', 'Title');
  const noteHead = el('div', 'note-head-inputs');
  noteHead.append(emojiPick.wrap, noteTitle);
  const calloutBox = el('div', 'callout-editor');
  const calloutHead = el('div', 'callout-editor-head');
  const removeCallout = el('button', 'link-button', 'Remove callout');
  removeCallout.type = 'button';
  calloutHead.append(el('span', 'callout-editor-title', 'Callout'), removeCallout);
  calloutBox.append(
    calloutHead,
    el('p', 'form-lead', 'A tinted note under the description for the one thing every guest should know.'),
    field('Emoji and title', noteHead, 'Blank means the megaphone and "Good to know".'),
    field('Words', needToKnow, '', true),
  );
  const createCallout = button('Create Callout', 'plus', 'button button-secondary button-small', () => {
    calloutBox.hidden = false;
    createCallout.hidden = true;
    needToKnow.focus();
  });
  removeCallout.addEventListener('click', () => {
    needToKnow.value = '';
    noteEmoji.value = '';
    noteTitle.value = '';
    calloutBox.hidden = true;
    createCallout.hidden = false;
  });
  calloutBox.hidden = !(p && p.needToKnow);
  createCallout.hidden = !calloutBox.hidden;
  const callout = el('div', 'callout-field');
  callout.append(createCallout, calloutBox);
  const audience = text(p ? p.audience : '', {maxLength: 60, placeholder: 'Adults, Families, Kids & Adults, Grades 3-6'});
  const pretty = text(p ? p.prettyId : '', {maxLength: 40, placeholder: 'fondue'});
  const prettyHint = el('small', '', '');
  const paintPretty = () => {
    const v = pretty.value.trim().toLowerCase();
    prettyHint.textContent = v ? `${location.origin}/p/${v}` : 'Lower-case letters, digits and hyphens; optional. Without one the party lives at /parties/{id}.';
  };
  pretty.addEventListener('input', paintPretty);
  paintPretty();
  const prettyField = field('Friendly address', pretty);
  prettyField.append(prettyHint);
  const basics = [
    field('Title', title, '', true), field('Subtitle', subtitle), field('Summary', summary, 'Shown on the party card.'),
    field('Description', description), callout, prettyField,
  ];

  const start = whenPickers('Starts', p ? p.start : '', null, 'No start time');
  const end = whenPickers('Ends', p ? p.end : '', null, 'No end time');
  const whenWrap = el('div', 'field-when');
  whenWrap.append(start.wrap, end.wrap);
  // Named place, not location: window.location is what the address hint
  // under the friendly-address field reads.
  const place = text(p ? p.location : '', {maxLength: 120, placeholder: "The Parks' House in Los Altos"});
  const address = addressSuggest(text(p ? p.address : '', {maxLength: 200, placeholder: '1420 Alder Court, Los Altos, CA 94024'}));
  const when = [
    field('When', whenWrap),
    field('Where, in words', place, 'Shown to everyone: the neighborhood or the venue, not the street.'),
    field('Street address', address, 'Shown only to signed-in Helios members, with a map link.'),
  ];

  const price = text(p ? p.price : '', {type: 'number', min: 0, step: '0.01', required: true, placeholder: '65'});
  const unit = text(p ? p.unit : '', {maxLength: 40, placeholder: 'person, adult, child, family'});
  const capacity = text(p && p.capacity ? p.capacity : '', {type: 'number', min: 1, step: 1, placeholder: 'Leave blank for no limit'});
  const minimum = text(p && p.minimum ? p.minimum : '', {type: 'number', min: 1, step: 1, placeholder: 'Leave blank for none'});
  const ticketsOpen = checkbox('Tickets on sale', p ? p.ticketsOpen : true, 'Off, the party is listed but sells nothing.');
  const waitlist = checkbox('Take a waitlist when full', p ? p.waitlist : true, 'Off, a full party shows Sold Out.');
  const adults = checkbox('Adults', p ? p.adults : true, 'Parents and staff can hold a ticket.');
  const students = checkbox('Students', p ? p.students : false, 'Students can hold a ticket.');
  const dropOff = checkbox('Drop-off is okay', p ? p.dropOff : false, 'Kids can come without a parent.');
  const parentTicket = checkbox('A parent who stays needs a ticket', p ? p.parentTicket : false, '');
  const tickets = [
    field('Price', price, '', true), field('One ticket covers', unit, '"$65 per person": the word after "per".'),
    field('Tickets available', capacity), field('Minimum to hold the party', minimum, 'The party goes ahead only with at least this many tickets sold.'),
    ticketsOpen.wrap, waitlist.wrap, el('div', 'field-group-label', 'Who can come'), adults.wrap, students.wrap, dropOff.wrap, parentTicket.wrap,
    field('Audience', audience, 'The words on the card: who the party is for.'),
  ];

  const hostsText = text(p ? p.hosts : '', {maxLength: 120, placeholder: 'McDowell and Park/Gulliver Families'});
  const initialHosts = p ? p.hostPeople : [{email: user.email, name: user.name, photoUrl: user.photoUrl}];
  const hostEmails = hostChips(initialHosts);
  const hosts = [
    field('Hosts, as shown', hostsText, 'The names on the party page: "Hosted by …".'),
    field('Who runs it', hostEmails.wrap, 'These people can edit the party and see who is coming. Click a face to remove it.'),
  ];

  const panels = [
    {label: 'Basics', icon: svg('party'), fields: basics},
    {label: 'When & where', icon: svg('calendar'), fields: when},
    {label: 'Tickets', icon: svg('ticket'), fields: tickets},
    {label: 'Hosts', icon: svg('people'), fields: hosts},
  ];
  let status = null;
  let celebrationPick = null;
  let category = null;
  if (isAdmin()) {
    status = select([{label: 'Open', value: 'Open'}, {label: 'Pending approval', value: 'Pending'}, {label: 'Hidden', value: 'Hidden'}], p ? p.status : 'Open');
    celebrationPick = select(state.model.celebrations.map(c => ({label: c.title, value: c.id})), p ? p.celebration : (currentCelebration() || {}).id);
    category = select([{label: 'No category', value: ''}, ...state.model.categories.map(c => ({label: c.title, value: c.id}))], p ? p.category : '');
    panels.push({label: 'Admin', icon: svg('shield'), fields: [
      field('Status', status, 'Open is listed for everyone; Pending waits for approval; Hidden is parked.'),
      field('Celebration', celebrationPick, 'Which year the party belongs to.'),
      field('Category', category, 'For the filter on the parties page.'),
    ]});
  }
  const intro = adding && !isAdmin() ? [el('p', 'form-lead', 'Thank you for hosting! Fill this in and the celebration committee will review it and open it for tickets.')] : [];
  openModal(adding ? 'Host a Party' : `Edit ${p.title}`, [...intro, tabbedFields(panels)], {
    wide: true,
    saveLabel: adding ? (isAdmin() ? 'Add Party' : 'Submit for Approval') : 'Save',
    submit: () => api('POST', '/api/celebrate/party', {
      id: p ? p.id : '', celebration: celebrationPick ? celebrationPick.value : '',
      title: title.value, subtitle: subtitle.value, summary: summary.value, description: description.value, needToKnow: needToKnow.value,
      noteEmoji: noteEmoji.value.trim(), noteTitle: noteTitle.value.trim(),
      hosts: hostsText.value, hostEmails: hostEmails.value(), category: category ? category.value : (p ? p.category || '' : ''), audience: audience.value, unit: unit.value,
      price: Number(price.value || 0), capacity: Number(capacity.value || 0), minimum: Number(minimum.value || 0),
      start: start.value(), end: end.value(), location: place.value, address: address.value, image: p ? p.image || '' : '', flyer: p ? p.flyer || '' : '', prettyId: pretty.value, status: status ? status.value : '',
      ticketsOpen: ticketsOpen.input.checked, waitlist: waitlist.input.checked, adults: adults.input.checked,
      students: students.input.checked, dropOff: dropOff.input.checked, parentTicket: parentTicket.input.checked,
    }),
    afterSave: result => {
      if (result && result.id && party(result.id)) {
        navigate(partyPath(party(result.id)));
        if (adding && !isAdmin()) {
          toast('Submitted - an admin will review it');
        }
      }
    },
    onDelete: p && isAdmin() ? () => api('DELETE', '/api/celebrate/party', {id: p.id}) : null,
    confirmDelete: p ? `Delete ${p.title}? This cannot be undone.` : '',
    afterDelete: () => navigate('/'),
  });
}

export async function savePartyFields(p, changes) {
  const body = {
    id: p.id, celebration: p.celebration, title: p.title, subtitle: p.subtitle || '', summary: p.summary || '', description: p.description || '',
    needToKnow: p.needToKnow || '', noteEmoji: p.noteEmoji || '', noteTitle: p.noteTitle || '', hosts: p.hosts || '', hostEmails: p.hostEmails, category: p.category || '', audience: p.audience || '',
    unit: p.unit || '', price: p.price, capacity: p.capacity || 0, minimum: p.minimum || 0, start: p.start || '', end: p.end || '',
    location: p.location || '', address: p.address || '', image: p.image || '', flyer: p.flyer || '', prettyId: p.prettyId || '', status: isAdmin() ? p.status : '', ticketsOpen: p.ticketsOpen,
    waitlist: p.waitlist, adults: p.adults, students: p.students, dropOff: p.dropOff, parentTicket: p.parentTicket,
    ...changes,
  };
  try {
    await api('POST', '/api/celebrate/party', body);
    await load();
  } catch (err) {
    toast(err.message);
  }
}

export async function setFlags(p, changes) {
  const body = {
    id: p.id, ticketsOpen: p.ticketsOpen, waitlist: p.waitlist, adults: p.adults, students: p.students,
    dropOff: p.dropOff, parentTicket: p.parentTicket, ...changes,
  };
  try {
    await api('POST', '/api/celebrate/party/flags', body);
    await load();
  } catch (err) {
    toast(err.message);
    await load();
  }
}

export async function setPartyStatus(p, status) {
  try {
    await api('POST', '/api/celebrate/party/status', {id: p.id, status});
    await load();
    toast(status === 'Open' ? `${p.title} is open` : `${p.title} is ${status.toLowerCase()}`);
  } catch (err) {
    toast(err.message);
  }
}

export async function openContacts(p) {
  const dir = await directory();
  const byEmail = new Map(dir.result.map(dir.get).map(q => [q.email, q]));
  const signUps = [...p.attendees, ...p.waitlisted].map(a => ({a, relation: ''}));
  const relations = new Set();
  const firstName = a => (a.name || a.email).split(' ')[0];
  const follows = {Parents: ['parents', 'Parent'], Children: ['children', 'Child'], Siblings: ['siblings', 'Sibling']};
  const relativesOf = r => {
    const info = byEmail.get(r.a.email);
    if (!info) {
      return [];
    }
    const out = [];
    for (const [relation, [path, as]] of Object.entries(follows)) {
      if (!relations.has(relation)) {
        continue;
      }
      for (const q of dir.follow(info, path).filter(q => q && q.email)) {
        out.push({a: {email: q.email, name: q.fullName}, relation: `${as} of ${firstName(r.a)}`});
      }
    }
    return out;
  };
  const titleOf = r => {
    const info = byEmail.get(r.a.email);
    if (!info) {
      return r.relation ? '' : 'Guest';
    }
    if (info.isStudent) {
      return info.grade || 'Student';
    }
    return [info.isParent ? 'Parent' : '', info.isStaff ? 'Staff' : ''].filter(Boolean).join(', ');
  };
  const mailLink = e => {
    const link = el('a', 'contact-email', e);
    link.href = `mailto:${e}`;
    return link;
  };
  const statusOf = a => (a.status === 'Waitlist' ? `Waitlist (${a.quantity || 1})` : a.status || '');
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
      for (const k of relativesOf(r)) {
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
  const address = text(c ? c.address : '', {maxLength: 200});
  const description = textarea(c ? c.description : '', 4);
  const isCalendar = Boolean(c && c.buttonUrl === 'calendar');
  const buttonText = text(c ? c.buttonText : '', {maxLength: 40, placeholder: 'Learn More'});
  const buttonUrl = text(c && !isCalendar ? c.buttonUrl : '', {type: 'url', maxLength: 500, placeholder: 'https://www.heliosschool.org/spring-celebration'});
  const urlField = field('Button link', buttonUrl, 'Where it goes - the celebration\u2019s own site, say.');
  urlField.hidden = isCalendar;
  let buttonKind = isCalendar ? 'calendar' : 'link';
  const kind = segmented([{label: 'Opens a link', value: 'link'}, {label: 'Save the Date', value: 'calendar'}], buttonKind, v => {
    buttonKind = v;
    urlField.hidden = v === 'calendar';
    if (v === 'calendar' && !buttonText.value.trim()) {
      buttonText.value = 'Save the Date';
    }
  });
  const kindField = field('Button', kind.wrap, 'Save the Date adds the celebration - its date, place and theme - to the reader\u2019s calendar.');
  const current = checkbox('This is the current celebration', c ? c.current : true, 'Its parties are what the parties page lists first.');
  const banner = checkbox('Show its banner at the top', c ? c.banner : false, 'The band across the top of the parties page advertises it, whichever year\u2019s parties are listed. Only one celebration is the banner.');
  const image = imagePicker(c ? c.image : '', c ? c.imageUrl : '', {dropzone: true, query: () => title.value, label: 'Banner image', hint: 'The background of the banner across the top of the parties page.'});
  openModal(c ? `Edit ${c.title}` : 'Add a celebration', [
    field('Code', code, 'Short and unique, like SC-2027: the celebration’s address on the site.', true), field('Title', title, 'The banner\u2019s big line: "Helios Spring Celebration 2026".', true),
    field('Subtitle', subtitle, 'The banner\u2019s small line above it: the theme.'),
    field('When', whenWrap), field('Where', place), field('Address', address), field('Description', description),
    el('div', 'field-group-label', 'Banner button'),
    kindField, field('Button text', buttonText, 'Leave it blank for no button.'), urlField,
    current.wrap, banner.wrap, image.wrap,
  ], {
    submit: () => api('POST', '/api/celebrate/celebration', {
      id: c ? c.id : '', code: code.value.trim(), title: title.value, subtitle: subtitle.value, start: start.value(), end: end.value(),
      location: place.value, address: address.value, description: description.value, image: image.value(), buttonText: buttonText.value, buttonUrl: buttonKind === 'calendar' ? 'calendar' : buttonUrl.value, current: current.input.checked, banner: banner.input.checked,
    }),
    onDelete: c ? () => api('DELETE', '/api/celebrate/celebration', {id: c.id}) : null,
    confirmDelete: c ? `Delete ${c.title}?` : '',
  });
}

export function openCategory(c, after) {
  const input = text(c ? c.title : '', {required: true, maxLength: 120});
  openModal(c ? 'Rename category' : 'Add a category', [field('Title', input, '', true)], {
    submit: () => api('POST', '/api/celebrate/category', {id: c ? c.id : '', title: input.value}),
    afterSave: after,
    onDelete: c ? () => api('DELETE', '/api/celebrate/category', {id: c.id}) : null,
    confirmDelete: c ? `Delete the category ${c.title}?` : '',
    afterDelete: after,
  });
}

export function openSettings() {
  const s = state.model.settings;
  const intro = textarea(s.partiesIntro, 3);
  const note = textarea(s.ticketNote, 4);
  const hosting = checkbox('Hosting open', s.hostingOpen, 'Anyone can post a party. Off, Host a Party goes away for everyone but an admin.');
  openModal('Settings', [
    field('Parties intro', intro, 'The line under the parties page heading.'),
    field('Ticket note', note, 'Shown on the ticket form: how invoicing works, the refund policy.'),
    hosting.wrap,
  ], {
    submit: () => api('POST', '/api/celebrate/settings', {partiesIntro: intro.value, ticketNote: note.value, hostingOpen: hosting.input.checked}),
  });
}
