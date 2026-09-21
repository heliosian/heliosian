import {isAdmin, isKid, whenParts, parseWhen, priceLine, money, googleCalendarLink, partyPath, myTickets, availabilityLabel} from '../state.js';
import {el, link, svg, button, avatar, thumb, paragraphs, copyText, toast} from '../dom.js';
import {setTitle, partiesPath} from '../chrome.js';
import {appOrigin} from '/toolbar.js';
import {openBuy, openParty, openFreeTicket, openTicket, openPerson, openReassign, removeTicket, offerTickets, setFlags, setPartyStatus, openContacts, savePartyFields, uploadImage, editable, editPencil, fieldEditor, textInput, textAreaInput, whenInputs, emojiPicker, uploadAndSave, imageSearchOn, openImageSearch} from '../edit.js';
import {statusBadges} from '../cards.js';
import {openPhotoLightbox, openCropTool} from '/crop.js';
import {dateCard} from '/datecard.js';

// The page is laid out the way HCA-Team lays out an event: the wide banner
// with the date stamp and the tools floating over it, then the words beside a
// rail of facts, the flyer, and who to ask.

// Which party is in edit mode, by id - as HCA-Team's event page has it: the
// hero's pencil reveals every section's pencil, and becomes Done. Keyed
// rather than a bare boolean so opening another party never inherits it.
let editingId = null;

// phone is the rail-less layout (style.css's breakpoint); crossing it lays the
// page out again, since the facts card sits in a different place.
const phone = window.matchMedia('(max-width: 900px)');
phone.addEventListener('change', () => document.dispatchEvent(new CustomEvent('celebrate:refresh')));

// heroStamp is the card floating over the banner - the date card every
// app shares (datecard.js): the day's tile, the hours, the place, and
// Add. A party with no date yet says so on a plain stamp instead.
function heroStamp(p) {
  if (!parseWhen(p.start)) {
    const stamp = el('div', 'hero-stamp hero-stamp-text');
    stamp.textContent = 'Date to come';
    return stamp;
  }
  return dateCard(el, {start: p.start, end: p.end, location: p.location || '', add: googleCalendarLink(p)});
}

// heroTools are the round buttons at the banner's top-left: the pencil for
// whoever runs the party (edit mode on and off) and the share. The picture
// itself opens full size on a click.
function heroTools(p, editing) {
  const tools = el('div', 'hero-actions');
  const tool = (icon, label, onClick) => {
    const b = button('', icon, 'hero-action', onClick);
    b.title = label;
    b.setAttribute('aria-label', label);
    return b;
  };
  if (p.canEdit) {
    const toggle = tool(editing ? 'check' : 'edit', editing ? 'Done editing' : 'Edit party', () => {
      editingId = editing ? null : p.id;
      document.dispatchEvent(new CustomEvent('celebrate:refresh'));
    });
    toggle.classList.toggle('is-editing', editing);
    tools.append(toggle);
  }
  // Share hands the link to the phone's share sheet, as HCA-Team does; where
  // there is none it copies the link instead.
  tools.append(tool('share', 'Share this party', async () => {
    const url = location.origin + partyPath(p);
    if (navigator.share) {
      try {
        await navigator.share({title: p.title, url});
        return;
      } catch {
        // Dismissed or refused: fall through to the clipboard.
      }
    }
    copyText(url, 'Link copied');
  }));
  return tools;
}

// heroImageBar is the strip across the foot of the banner while editing:
// pick a file and it uploads and saves in one go, find one in the image
// libraries, crop, or remove.
function heroImageBar(p, save) {
  const bar = el('div', 'hero-image-bar');
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  file.addEventListener('change', async () => {
    if (!file.files.length) {
      return;
    }
    bar.replaceChildren(el('span', 'hero-image-status', 'Uploading…'));
    await uploadAndSave(save, file.files[0]);
  });
  const label = p.image ? 'Replace image' : 'Add an image';
  if (imageSearchOn()) {
    const holder = el('div', 'hero-image-menu-holder');
    const toggle = el('button', 'hero-image-action');
    toggle.type = 'button';
    toggle.setAttribute('aria-haspopup', 'menu');
    toggle.append(svg('image'), el('span', '', label), svg('caret'));
    const menu = el('div', 'hero-image-menu');
    menu.hidden = true;
    const item = (icon, words, onClick) => {
      const b = el('button', 'hero-image-menu-item');
      b.type = 'button';
      b.append(svg(icon), el('span', '', words));
      b.addEventListener('click', () => {
        menu.hidden = true;
        onClick();
      });
      menu.append(b);
    };
    item('up', 'Upload image', () => file.click());
    item('search', 'Find an image', () => openImageSearch(p.title, picked => save({image: picked})));
    toggle.addEventListener('click', e => {
      e.stopPropagation();
      menu.hidden = !menu.hidden;
    });
    document.addEventListener('click', () => {
      menu.hidden = true;
    }, {once: true, capture: true});
    holder.append(toggle, menu, file);
    bar.append(holder);
  } else {
    const choose = el('label', 'hero-image-action');
    choose.append(svg('image'), el('span', '', label), file);
    bar.append(choose);
  }
  if (p.image) {
    const crop = el('button', 'hero-image-action');
    crop.type = 'button';
    crop.append(svg('expand'), el('span', '', 'Crop'));
    crop.addEventListener('click', () => openCropTool(p.imageUrl, false, async blob => {
      await uploadAndSave(save, new File([blob], 'crop.jpg', {type: 'image/jpeg'}));
      return true;
    }));
    bar.append(crop);
    const remove = el('button', 'hero-image-action');
    remove.type = 'button';
    remove.append(svg('trash'), el('span', '', 'Remove'));
    remove.addEventListener('click', () => save({image: ''}));
    bar.append(remove);
  }
  return bar;
}

function hero(p, editing, save) {
  const wrap = el('div', 'detail-hero');
  // The banner itself opens full size on a click; there is no tool for it.
  const image = thumb(p.imageUrl, p.title, 'detail-hero-image');
  if (p.imageUrl) {
    image.classList.add('is-openable');
    image.addEventListener('click', () => openPhotoLightbox(p.imageUrl));
  }
  wrap.append(image, heroTools(p, editing), heroStamp(p));
  if (editing) {
    wrap.append(heroImageBar(p, save));
  }
  return wrap;
}

// withPencil puts a value and its pencil on one row, while editing.
function withPencil(node, pencil) {
  const row = el('div', 'edit-row');
  row.append(node, pencil);
  return row;
}

// emptyPrompt is what an empty section shows in edit mode: an invitation
// that opens the editor for it.
function emptyPrompt(label, make, submit) {
  const b = el('button', 'edit-empty');
  b.type = 'button';
  b.append(svg('plus'), el('span', '', label));
  b.addEventListener('click', () => {
    const built = make();
    fieldEditor(b, null, {input: built.input, hint: built.hint, value: built.value, validate: built.validate, submit});
  });
  return b;
}

// ticketWords is the headline of the ticket band, in the old site's voice.
// A full party a family is already on says so rather than asking them to
// join its waitlist.
function ticketWords(p, mine) {
  switch (p.availability) {
    case 'available':
      return 'Tickets Available!';
    case 'waitlist':
      if (mine.some(a => a.status === 'Ticket')) {
        return 'Sold Out - Your Family Is Going';
      }
      if (mine.length) {
        return "Sold Out - You're on the Waitlist";
      }
      return 'Sold Out - Join the Waitlist';
    case 'sold-out':
      return 'Sold Out';
    case 'past':
      return 'This party has happened';
  }
  return 'Tickets Closed';
}

function ticketBand(p) {
  const mine = myTickets(p);
  const band = el('div', 'ticket-band avail-band-' + p.availability);
  band.append(el('h2', 'ticket-words', ticketWords(p, mine)));
  // The family's own place on the waitlist sits in the band with the
  // request as they made it - who asked, for how many, and their note -
  // so the button to update it has something to update in view.
  for (const a of mine.filter(a => a.status !== 'Ticket')) {
    const row = el('div', 'ticket-mine');
    row.append(avatar(a, 'my-ticket-face'));
    const words = el('span', 'my-ticket-words');
    const n = a.quantity || 1;
    words.append(el('span', 'my-ticket-name', a.name), el('span', 'my-ticket-line', `Waiting for ${n} ${n === 1 ? 'ticket' : 'tickets'}`));
    if (a.note) {
      words.append(el('span', 'ticket-mine-note', `\u201c${a.note}\u201d`));
    }
    row.append(words);
    band.append(row);
  }
  const actions = el('div', 'ticket-actions');
  const selling = p.availability === 'available' || p.availability === 'waitlist';
  // A family holding a ticket to a full party may still want more, so the
  // way onto the waitlist stays; a family already waiting gets a way to
  // change their request instead. A host's family is no different: a full
  // party sells nothing more until a place is offered off the waitlist.
  const waiting = p.availability === 'waitlist' && mine.some(a => a.status !== 'Ticket');
  // A student sees the party and who is coming; a parent takes the
  // tickets and passes them on.
  if (isKid() && !p.canEdit) {
    if (selling && !mine.length) {
      actions.append(el('span', 'ticket-kid-note', 'Ask a parent to sign in to get tickets.'));
    }
  } else if (selling || p.canEdit) {
    const label = !selling ? 'Add Attendee'
      : p.availability !== 'waitlist' ? 'Get Tickets'
      : waiting ? 'Update Waitlist Request' : 'Join the Waitlist';
    actions.append(button(label, 'ticket', 'button', () => openBuy(p)));
  }
  if (p.capacity > 0 && p.availability === 'available') {
    actions.append(el('span', 'ticket-left', `${p.remaining} of ${p.capacity} left`));
  }
  band.append(actions);
  return band;
}

// myTicketsSection lists the household's own sold tickets under their own
// heading, so a family's tickets don't read as part of the sales band
// above - a sold-out party's band is all about the waitlist, and a ticket
// the family holds is not. A place on the waitlist is not a ticket: it
// stays in the waitlist below, in its turn, with its way out there. A sold
// ticket stays sold - it is a fundraiser - so here a ticket can only be
// passed on.
function myTicketsSection(p) {
  const mine = myTickets(p).filter(a => a.status === 'Ticket');
  if (!mine.length) {
    return null;
  }
  const section = el('section', 'my-tickets');
  section.append(swooshHeading('My Tickets'));
  const list = el('div', 'my-ticket-list');
  for (const a of mine) {
    const row = el('div', 'my-ticket');
    row.append(avatar(a, 'my-ticket-face'));
    const words = el('span', 'my-ticket-words');
    words.append(el('span', 'my-ticket-name', a.name), el('span', 'my-ticket-line', a.price ? `Ticket · ${money(a.price)}` : 'Free ticket'));
    row.append(words);
    if (p.availability !== 'past' && (!isKid() || p.canEdit)) {
      row.append(button('Reassign', 'people', 'link-button', () => openReassign(p, a)));
    }
    list.append(row);
  }
  section.append(list);
  return section;
}

// swooshHeading is the section heading with the brand's yellow swipe under
// it, as HCA-Team heads Volunteers.
function swooshHeading(text) {
  return el('h2', 'section-swoosh', text);
}

// attendeeTile is one face on the Who's Coming grid: photo, name, and the
// line that places them. A click opens the person's card; whoever runs the
// party gets the ticket instead, since a host clicking a face wants that.
function attendeeTile(p, a) {
  const tile = el('button', 'attendee' + (p.canEdit ? ' is-editable' : ''));
  tile.type = 'button';
  if (p.canEdit) {
    tile.title = `Open ${a.name}'s ticket`;
    tile.addEventListener('click', () => openTicket(p, a));
  } else {
    tile.title = a.name;
    tile.addEventListener('click', () => openPerson(a));
  }
  tile.append(avatar(a, 'attendee-face'));
  tile.append(el('div', 'attendee-name', a.name));
  if (a.line) {
    tile.append(el('div', 'attendee-line', a.line));
  }
  if (a.mine) {
    tile.append(el('div', 'attendee-mine', 'Your family'));
  }
  // The hosts read each face's answer to the party's invitation on
  // Helios When, once the invites have gone out.
  if (a.rsvp) {
    tile.append(el('div', 'attendee-rsvp is-' + a.rsvp, rsvpWords[a.rsvp] || a.rsvp));
  }
  return tile;
}

const rsvpWords = {yes: 'RSVP: Yes', maybe: 'RSVP: Maybe', no: 'RSVP: No', none: 'No RSVP yet'};

function attendeesSection(p) {
  const section = el('section', 'attendees');
  const head = el('div', 'section-head');
  const n = p.attendees.length;
  head.append(swooshHeading(`Who's Coming (${n})`));
  if (p.canEdit) {
    const tools = el('div', 'section-tools');
    tools.append(button('Attendee contact info', 'mail', 'button button-secondary button-small', () => openContacts(p)));
    // The invitation lives on Helios When: Create Invite starts the party's
    // guest list there from the ticket holders; once the invites are out
    // the same page is where the RSVPs are.
    if (p.availability !== 'past') {
      const invite = el('a', 'button button-small' + (p.started ? ' button-secondary' : ''));
      invite.href = invitePath(p);
      invite.append(svg('calendar'), el('span', '', p.invited ? 'RSVPs on Helios When' : p.started ? 'The invite on Helios When' : 'Create Invite'));
      tools.append(invite);
    }
    head.append(tools);
  }
  section.append(head);
  if (!n) {
    section.append(el('p', 'section-note', 'Nobody yet - be the first!'));
    return section;
  }
  const grid = el('div', 'attendee-grid');
  for (const a of p.attendees) {
    grid.append(attendeeTile(p, a));
  }
  section.append(grid);
  return section;
}

function waitlistSection(p) {
  if (!p.waitlisted.length) {
    return null;
  }
  const section = el('section', 'attendees waitlist');
  section.append(el('h3', 'section-title', `Waitlist (${p.waitlisted.length})`));
  const list = el('div', 'wait-list');
  // Each entry is a family's request: who asked, and for how many.
  p.waitlisted.forEach((a, i) => {
    const row = el('div', 'wait-row');
    row.append(el('span', 'wait-num', String(i + 1)), avatar(a, 'wait-face'));
    const words = el('span', 'wait-words');
    words.append(el('span', 'wait-name', a.name));
    const n = a.quantity || 1;
    words.append(el('span', 'wait-line', `${n} ${n === 1 ? 'ticket' : 'tickets'}${a.line ? ` · ${a.line}` : ''}`));
    row.append(words);
    // The family's own request is marked as their faces are on the grid.
    if (a.mine) {
      row.append(el('span', 'wait-mine', 'Your family'));
    }
    if (p.canEdit) {
      row.append(button(`Offer ${n === 1 ? 'a ticket' : n + ' tickets'}`, 'ticket', 'button button-secondary button-small', () => offerTickets(p, a)));
      row.append(button('', 'edit', 'edit-icon', () => openTicket(p, a)));
    } else if (a.mine && !isKid()) {
      row.append(button('Leave waitlist', 'close', 'link-button', () => removeTicket(p, a)));
    }
    list.append(row);
  });
  section.append(list);
  return section;
}

// callout is the party's need-to-know line in the tinted card HCA-Team uses
// for an event's highlight, behind a megaphone and "Good to know" unless
// the host has dressed it with their own emoji and title.
function callout(p, editing, save) {
  const make = () => noteInputs(p);
  if (!p.needToKnow) {
    return editing ? emptyPrompt('Add a need-to-know line', make, save) : null;
  }
  const card = el('div', 'highlight-card');
  card.append(el('div', 'highlight-icon', p.noteEmoji || '📣'));
  const body = el('div', 'highlight-body');
  body.append(el('div', 'highlight-headline', p.noteTitle || 'Good to know'), el('p', 'highlight-text', p.needToKnow));
  card.append(body);
  if (editing) {
    return withPencil(card, editable(card, 'Edit the need-to-know line', make, save));
  }
  return card;
}

// noteInputs is the callout's editor: the emoji and the title on one line,
// the words under them.
function noteInputs(p) {
  const emojiPick = emojiPicker(p.noteEmoji);
  const emoji = emojiPick.input;
  const title = textInput(p.noteTitle || '', {maxLength: 60, placeholder: 'Good to know'});
  title.setAttribute('aria-label', 'Title');
  const text = textAreaInput(p.needToKnow || '', 3);
  const head = el('div', 'note-head-inputs');
  head.append(emojiPick.wrap, title);
  const wrap = el('div', 'note-inputs');
  wrap.append(head, text);
  return {
    input: wrap,
    value: () => ({noteEmoji: emoji.value.trim(), noteTitle: title.value.trim(), needToKnow: text.value}),
    hint: p.needToKnow ? 'Clear the words to take the callout off. Leave the emoji or title blank for the megaphone and "Good to know".' : 'Leave the emoji or title blank for the megaphone and "Good to know".',
  };
}

// switchRow is one of the host's switches, saved the moment it is flipped.
function switchRow(label, hint, on, onChange) {
  const row = el('div', 'host-switch');
  const words = el('div', 'host-switch-words');
  words.append(el('div', 'host-switch-label', label), el('div', 'host-switch-hint', hint));
  const knob = el('label', 'switch');
  const input = el('input');
  input.type = 'checkbox';
  input.checked = on;
  input.addEventListener('change', () => onChange(input.checked));
  knob.append(input, el('span'));
  row.append(words, knob);
  return row;
}

// hostBand is the band whoever runs the party gets: the switches, Edit
// Party, and for an admin the status.
function hostBand(p) {
  const band = el('div', 'host-band');
  const head = el('div', 'host-band-head');
  head.append(svg('star'), el('span', '', p.hosting ? "You're hosting this party" : 'Admin'));
  band.append(head);
  const rows = el('div', 'host-switches');
  rows.append(
    switchRow('Tickets on sale', 'Off, the party is listed but sells nothing', p.ticketsOpen, on => setFlags(p, {ticketsOpen: on})),
    switchRow('Waitlist when full', 'Off, a full party shows Sold Out', p.waitlist, on => setFlags(p, {waitlist: on})),
    switchRow('Adults', 'Can parents and staff hold a ticket?', p.adults, on => setFlags(p, {adults: on})),
    switchRow('Students', 'Can students hold a ticket?', p.students, on => setFlags(p, {students: on})),
    switchRow('Drop-off', 'Can kids come without a parent?', p.dropOff, on => setFlags(p, {dropOff: on})),
    switchRow('Parent ticket required', 'If a parent stays, do they need a ticket?', p.parentTicket, on => setFlags(p, {parentTicket: on})),
  );
  band.append(rows);
  const actions = el('div', 'host-actions');
  actions.append(button('Edit all fields', 'edit', 'button button-secondary', () => openParty(p)));
  if (p.availability !== 'past') {
    actions.append(button('Add Free Ticket', 'ticket', 'button button-secondary', () => openFreeTicket(p)));
  }
  if (isAdmin()) {
    if (p.status === 'Pending') {
      actions.append(button('Approve', 'check', 'button', () => setPartyStatus(p, 'Open')));
    }
    if (p.status !== 'Hidden') {
      actions.append(button('Hide', 'eye', 'button button-secondary', () => setPartyStatus(p, 'Hidden')));
    } else {
      actions.append(button('Unhide', 'eye', 'button button-secondary', () => setPartyStatus(p, 'Open')));
    }
  }
  band.append(actions);
  return band;
}

// The rail's cards, as HCA-Team draws them: an icon, a title, and lines.
function sideCard(className) {
  return el('div', 'side-card ' + (className || ''));
}

function sideRow(icon, title, ...lines) {
  const row = el('div', 'side-row');
  const iconBox = el('div', 'side-icon');
  iconBox.append(svg(icon));
  const body = el('div', 'side-row-body');
  body.append(el('div', 'side-title', title));
  for (const line of lines) {
    if (typeof line === 'string') {
      if (line) {
        body.append(el('div', 'side-line', line));
      }
    } else if (line) {
      body.append(line);
    }
  }
  row.append(iconBox, body);
  return row;
}

// factsCard is the rail's facts: the street address, what a ticket costs,
// and who hosts - the date and the place in words being on the banner's
// card, with Add.
function factsCard(p, editing, save) {
  const card = sideCard('facts-card');
  const when = whenParts(p);
  // In edit mode each row grows a pencil; the row's own body is what the
  // editor stands in for.
  const pencilFor = (row, label, make) => {
    if (!editing) {
      return row;
    }
    const body = row.querySelector('.side-row-body');
    const pencil = editable(body, label, make, save);
    row.append(pencil);
    return row;
  };
  // The date and hours are on the banner's card, with Add; the row is
  // here while editing, where the date editor lives, and for a party
  // with no date yet.
  if (editing || !when.longDay) {
    card.append(pencilFor(sideRow('calendar', 'Date & Time', when.longDay || 'Date to come', when.time || ''), 'Edit the date and time', () => {
      const w = whenInputs(p.start, p.end);
      return {input: w.input, value: w.value, validate: w.validate};
    }));
  }
  // Where: the place in words is on the banner's card; the row is here
  // for the street address - which only signed-in members ever see - with
  // its map link, and while editing, where the place editor lives.
  if (p.address || (p.location && editing)) {
    let mapLink = null;
    let note = null;
    if (p.address) {
      mapLink = el('a', 'side-line side-address');
      mapLink.href = `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(p.address)}`;
      mapLink.target = '_blank';
      mapLink.rel = 'noopener';
      mapLink.append(svg('open'), el('span', '', p.address));
      note = el('div', 'side-note', 'Address shown to signed-in Helios members only');
    }
    card.append(pencilFor(sideRow('pin', 'Where', p.location || '', mapLink, note), 'Edit where', () => {
      const place = textInput(p.location || '', {placeholder: "The Parks' House in Los Altos", maxLength: 120});
      const address = textInput(p.address || '', {placeholder: '1420 Alder Court, Los Altos, CA 94024', maxLength: 200});
      const stack = el('div', 'field-editor-stack');
      const l1 = el('label');
      l1.append('In words, for everyone', place);
      const l2 = el('label');
      l2.append('Street address, for signed-in members', address);
      stack.append(l1, l2);
      return {input: stack, value: () => ({location: place.value, address: address.value})};
    }));
  } else if (editing) {
    card.append(pencilFor(sideRow('pin', 'Where', 'Not set yet'), 'Edit where', () => {
      const place = textInput('', {placeholder: "The Parks' House in Los Altos", maxLength: 120});
      const address = textInput('', {placeholder: '1420 Alder Court, Los Altos, CA 94024', maxLength: 200});
      const stack = el('div', 'field-editor-stack');
      const l1 = el('label');
      l1.append('In words, for everyone', place);
      const l2 = el('label');
      l2.append('Street address, for signed-in members', address);
      stack.append(l1, l2);
      return {input: stack, value: () => ({location: place.value, address: address.value})};
    }));
  }
  const ticketLines = [priceLine(p)];
  if (p.capacity) {
    ticketLines.push(p.remaining > 0 ? `${p.remaining} of ${p.capacity} tickets remaining` : `All ${p.capacity} tickets taken`);
  } else {
    ticketLines.push(`${p.sold} sold · no limit`);
  }
  // The minimum is the host's business - a line for whoever runs it, not a
  // worry for guests.
  if (p.minimum && p.canEdit) {
    ticketLines.push(`Goes ahead with at least ${p.minimum} tickets sold`);
  }
  card.append(pencilFor(sideRow('ticket', 'Tickets', ...ticketLines), 'Edit the price and tickets', () => {
    const price = textInput(p.price, {type: 'number', min: 0, step: '0.01'});
    const unit = textInput(p.unit || '', {placeholder: 'person, adult, child', maxLength: 40});
    const capacity = textInput(p.capacity || '', {type: 'number', min: 1, step: 1, placeholder: 'No limit'});
    const minimum = textInput(p.minimum || '', {type: 'number', min: 1, step: 1, placeholder: 'None'});
    const stack = el('div', 'field-editor-stack');
    for (const [words, input] of [['Price, in dollars', price], ['One ticket covers ("per …")', unit], ['Tickets available', capacity], ['Minimum to go ahead', minimum]]) {
      const l = el('label');
      l.append(words, input);
      stack.append(l);
    }
    return {input: stack, value: () => ({price: Number(price.value || 0), unit: unit.value, capacity: Number(capacity.value || 0), minimum: Number(minimum.value || 0)})};
  }));
  // The hosts: faces with names, the way the portal shows co-chairs.
  if (p.hostPeople.length || p.hosts) {
    const hostsRow = sideRow('people', p.hostPeople.length === 1 ? 'Host' : 'Hosts', p.hosts || '');
    const faces = el('div', 'side-chairs');
    // Each face opens the host's card, as a co-chair's does in HCA-Team.
    for (const h of p.hostPeople) {
      const tile = el('button', 'side-chair');
      tile.type = 'button';
      tile.title = h.name;
      tile.append(avatar(h, ''), el('div', 'side-chair-name', h.name));
      tile.addEventListener('click', () => openPerson(h));
      faces.append(tile);
    }
    if (faces.children.length) {
      hostsRow.querySelector('.side-row-body').append(faces);
    }
    if (editing) {
      // Who runs it is a list of people; the full editor's Hosts tab does that.
      const pencil = editPencil('Edit the hosts');
      pencil.addEventListener('click', () => openParty(p));
      hostsRow.append(pencil);
    }
    card.append(hostsRow);
  }
  return card;
}

// flyerCard is the party's poster in the rail, under the facts: the whole
// picture at the rail's width, a click to see it full size, and for whoever
// runs the party a way to put one up or take it down.
function flyerCard(p, editing) {
  if (!p.flyerUrl && !editing) {
    return null;
  }
  const card = sideCard('flyer-card');
  card.append(el('div', 'side-title flyer-head', 'Flyer'));
  if (p.flyerUrl) {
    const open = el('button', 'flyer-open');
    open.type = 'button';
    open.title = 'View full size';
    const img = el('img', 'flyer-image');
    img.src = p.flyerUrl;
    img.alt = `${p.title} flyer`;
    open.append(img);
    open.addEventListener('click', () => openPhotoLightbox(p.flyerUrl));
    card.append(open);
  } else {
    card.append(el('div', 'side-line', 'No flyer yet - upload the party’s poster and it shows here.'));
  }
  if (editing) {
    const bar = el('div', 'flyer-actions');
    const file = el('input');
    file.type = 'file';
    file.accept = 'image/*';
    file.hidden = true;
    file.addEventListener('change', async () => {
      if (!file.files.length) {
        return;
      }
      try {
        const name = await uploadImage(file.files[0]);
        await savePartyFields(p, {flyer: name});
      } catch (err) {
        toast(err.message);
      }
    });
    const upload = el('label', 'button button-secondary button-small');
    upload.append(svg('up'), el('span', '', p.flyer ? 'Replace' : 'Upload'), file);
    // A flyer is a finished poster: it is uploaded whole, never found in a
    // library or cropped.
    bar.append(upload);
    if (p.flyer) {
      bar.append(button('Remove', 'trash', 'button button-secondary button-small', () => savePartyFields(p, {flyer: ''})));
    }
    card.append(bar);
  }
  return card;
}

// helpCard is who to ask: a mail to the hosts.
// invitePath is the party's page on Helios When - with ?invite=1, which
// starts the guest list there, while none exists yet.
function invitePath(p) {
  return appOrigin('calendar') + '/e/celebrate/' + encodeURIComponent(p.id) + (p.started ? '' : '?invite=1');
}

// inviteCard is a host's own word in the rail, highlighted so it is not
// missed: the invitation lives on Helios When. Before a guest list exists
// it says how Create Invite starts one; with a list still to be sent, that
// the invite is waiting there; once the invites are out, that the RSVPs
// are there.
function inviteCard(p) {
  if (!p.canEdit || p.availability === 'past') {
    return null;
  }
  const card = sideCard('side-card-invite');
  const [title, words, label] = p.invited
    ? ['RSVPs on Helios When', 'The invites are out. The guest list and the RSVPs are on the party\u2019s page on Helios When - who has a ticket, who has said they are coming, and the way to remind whoever has not.', 'See the RSVPs']
    : p.started
      ? ['Your invite is waiting', 'The guest list is started on Helios When, and nobody has been sent the invitation yet. Look it over there and send it when it is ready.', 'View the invite']
      : ['Invite your guests', 'Create an invite on Helios When and start collecting RSVPs. If more people get tickets, the invite can be automatically updated.', 'Create Invite'];
  card.append(el('div', 'side-title', title), el('div', 'side-line', words));
  const a = el('a', 'button button-small side-button');
  a.href = invitePath(p);
  a.append(svg('calendar'), el('span', '', label));
  card.append(a);
  return card;
}

function helpCard(p) {
  const card = sideCard('side-card-help');
  card.append(el('div', 'side-title', 'Questions?'), el('div', 'side-line', `Have a question about ${p.title}? Ask whoever is hosting it.`));
  const emails = p.hostPeople.map(h => h.email).filter(Boolean);
  if (emails.length) {
    const a = el('a', 'button button-secondary button-small side-button');
    a.href = `mailto:${emails.join(',')}?subject=${encodeURIComponent(p.title)}`;
    a.append(svg('mail'), el('span', '', p.hostPeople.length === 1 ? 'Contact the Host' : 'Contact the Hosts'));
    card.append(a);
  }
  return card;
}

export function partyPage(p) {
  setTitle(p.title);
  const editing = Boolean(p.canEdit && editingId === p.id);
  const save = changes => savePartyFields(p, changes);
  const page = el('div', 'party-page' + (editing ? ' is-editing' : ''));
  const top = el('div', 'detail-top');
  const back = link(partiesPath(), 'detail-back');
  back.append(svg('back'), el('span', '', 'Back to Parties'));
  top.append(back);
  page.append(top, hero(p, editing, save));

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const side = el('div', 'detail-side');

  const marks = el('div', 'detail-marks');
  // The viewer's own party says so first, in the card's yellow.
  if (p.hosting) {
    marks.append(el('span', 'audience-chip hosting-chip', 'Hosting'));
  }
  const audienceMake = () => {
    const input = textInput(p.audience || '', {placeholder: 'Adults, Families, Kids & Adults, Grades 3-6', maxLength: 60});
    return {input, value: () => ({audience: input.value}), hint: 'The words on the card: who the party is for.'};
  };
  if (p.audience) {
    const chip = el('span', 'audience-chip', p.audience);
    marks.append(chip);
    if (editing) {
      marks.append(editable(chip, 'Edit who it is for', audienceMake, save));
    }
  } else if (editing) {
    marks.append(emptyPrompt('Say who it is for', audienceMake, save));
  }
  if (p.availability !== 'available') {
    marks.append(el('span', 'avail avail-' + p.availability, availabilityLabel(p)));
  }
  for (const b of statusBadges(p)) {
    marks.append(b);
  }
  main.append(marks);
  const title = el('h1', 'detail-title', p.title);
  if (editing) {
    main.append(withPencil(title, editable(title, 'Edit the title', () => {
      const input = textInput(p.title, {maxLength: 120});
      return {input, value: () => ({title: input.value}), validate: v => (v.title.trim() ? '' : 'A party needs a title.')};
    }, save)));
  } else {
    main.append(title);
  }
  const subtitleMake = () => {
    const input = textInput(p.subtitle || '', {maxLength: 120, placeholder: 'Sweet & Savory Fondue, plus Build-Your-Own Fort'});
    return {input, value: () => ({subtitle: input.value})};
  };
  if (p.subtitle) {
    const subtitle = el('p', 'detail-subtitle', p.subtitle);
    main.append(editing ? withPencil(subtitle, editable(subtitle, 'Edit the subtitle', subtitleMake, save)) : subtitle);
  } else if (editing) {
    main.append(emptyPrompt('Add a subtitle', subtitleMake, save));
  }
  // On a phone the facts come up under the title, where the rail would be.
  if (phone.matches) {
    const facts = factsCard(p, editing, save);
    facts.classList.add('facts-inline');
    main.append(facts);
  }
  const descriptionMake = () => {
    const input = textAreaInput(p.description || '', 8);
    return {input, value: () => ({description: input.value})};
  };
  if (p.description) {
    const prose = paragraphs(p.description, 'prose detail-text');
    main.append(editing ? withPencil(prose, editable(prose, 'Edit the description', descriptionMake, save)) : prose);
  } else if (editing) {
    main.append(emptyPrompt('Add a description', descriptionMake, save));
  } else if (p.summary) {
    main.append(el('p', 'detail-text', p.summary));
  }
  if (editing) {
    const summaryMake = () => {
      const input = textAreaInput(p.summary || '', 2);
      return {input, value: () => ({summary: input.value}), hint: 'One or two sentences for the party card.'};
    };
    const summary = el('p', 'detail-summary', p.summary ? `Card summary: ${p.summary}` : '');
    main.append(p.summary ? withPencil(summary, editable(summary, 'Edit the card summary', summaryMake, save)) : emptyPrompt('Add a card summary', summaryMake, save));
  }
  const note = callout(p, editing, save);
  if (note) {
    main.append(note);
  }
  main.append(ticketBand(p));
  const mine = myTicketsSection(p);
  if (mine) {
    main.append(mine);
  }
  main.append(attendeesSection(p));
  const wait = waitlistSection(p);
  if (wait) {
    main.append(wait);
  }
  if (p.canEdit) {
    main.append(hostBand(p));
  }

  for (const card of [inviteCard(p), phone.matches ? null : factsCard(p, editing, save), flyerCard(p, editing), helpCard(p)]) {
    if (card) {
      side.append(card);
    }
  }
  cols.append(main, side);
  page.append(cols);
  return page;
}
