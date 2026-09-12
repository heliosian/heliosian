import {isAdmin, whenParts, parseWhen, priceLine, money, googleCalendarLink, partyPath, myTickets, availabilityLabel} from '../state.js';
import {el, link, svg, button, avatar, thumb, paragraphs, copyText, toast} from '../dom.js';
import {setTitle, partiesPath} from '../chrome.js';
import {openBuy, openParty, openTicket, openPerson, removeTicket, setTicketStatus, setFlags, setPartyStatus, openContacts, savePartyFields, uploadImage} from '../edit.js';
import {statusBadges} from '../cards.js';
import {openPhotoLightbox} from '/crop.js';

// The page is laid out the way HCA-Team lays out an event: the wide banner
// with the date stamp and the tools floating over it, then the words beside a
// rail of facts, the flyer, and who to ask.

// phone is the rail-less layout (style.css's breakpoint); crossing it lays the
// page out again, since the facts card sits in a different place.
const phone = window.matchMedia('(max-width: 900px)');
phone.addEventListener('change', () => document.dispatchEvent(new CustomEvent('celebrate:refresh')));

const weekdayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short'});
const monthShort = new Intl.DateTimeFormat('en-US', {month: 'short'});

// heroStamp is the card floating over the banner: weekday, date, and the
// hours. A party with no date yet says so instead.
function heroStamp(p) {
  const start = parseWhen(p.start);
  const stamp = el('div', 'hero-stamp');
  if (!start) {
    stamp.classList.add('hero-stamp-text');
    stamp.textContent = 'Date to come';
    return stamp;
  }
  const when = whenParts(p);
  stamp.append(el('div', 'hero-stamp-weekday', weekdayFormat.format(start.date).toUpperCase()),
    el('div', 'hero-stamp-date', `${monthShort.format(start.date).toUpperCase()} ${start.date.getDate()}`));
  if (when.time) {
    stamp.append(el('div', 'hero-stamp-time', when.time));
  }
  return stamp;
}

// heroTools are the round buttons at the banner's top-left: the pencil for
// whoever runs the party, the share (copy link), and the expand into a
// full-size view of the picture.
function heroTools(p) {
  const tools = el('div', 'hero-actions');
  const tool = (icon, label, onClick) => {
    const b = button('', icon, 'hero-action', onClick);
    b.title = label;
    b.setAttribute('aria-label', label);
    return b;
  };
  if (p.canEdit) {
    tools.append(tool('edit', 'Edit party', () => openParty(p)));
  }
  tools.append(tool('copy', 'Copy link', () => copyText(location.origin + partyPath(p), 'Link copied')));
  if (p.imageUrl) {
    tools.append(tool('expand', 'View full size', () => openPhotoLightbox(p.imageUrl)));
  }
  return tools;
}

function hero(p) {
  const wrap = el('div', 'detail-hero');
  wrap.append(thumb(p.imageUrl, p.title, 'detail-hero-image'), heroTools(p), heroStamp(p));
  return wrap;
}

// ticketWords is the headline of the ticket band, in the old site's voice.
function ticketWords(p) {
  switch (p.availability) {
    case 'available':
      return 'Tickets Available!';
    case 'waitlist':
      return 'Sold Out - Join the Waitlist';
    case 'sold-out':
      return 'Sold Out';
    case 'past':
      return 'This party has happened';
  }
  return 'Tickets Closed';
}

function ticketBand(p) {
  const band = el('div', 'ticket-band avail-band-' + p.availability);
  band.append(el('h2', 'ticket-words', ticketWords(p)));
  const actions = el('div', 'ticket-actions');
  const selling = p.availability === 'available' || p.availability === 'waitlist';
  if (selling || p.canEdit) {
    const label = !selling ? 'Add Attendee' : p.availability === 'waitlist' ? 'Join the Waitlist' : 'Get Tickets';
    actions.append(button(label, 'ticket', 'button', () => openBuy(p)));
  }
  if (p.capacity > 0 && p.availability === 'available') {
    actions.append(el('span', 'ticket-left', `${p.remaining} of ${p.capacity} left`));
  }
  band.append(actions);
  // The household's own tickets. A sold ticket stays sold - it is a
  // fundraiser - so only a place on the waitlist has a way out here.
  const mine = myTickets(p);
  if (mine.length) {
    const list = el('div', 'my-tickets');
    list.append(el('div', 'my-tickets-head', 'Your family'));
    for (const a of mine) {
      const row = el('div', 'my-ticket');
      row.append(avatar(a, 'my-ticket-face'));
      const words = el('span', 'my-ticket-words');
      words.append(el('span', 'my-ticket-name', a.name), el('span', 'my-ticket-line', a.status === 'Ticket' ? `Ticket · ${money(a.price || 0)}` : 'On the waitlist'));
      row.append(words);
      if (a.status !== 'Ticket' && p.availability !== 'past') {
        row.append(button('Leave waitlist', 'close', 'link-button', () => removeTicket(p, a)));
      }
      list.append(row);
    }
    band.append(list);
  }
  return band;
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
  return tile;
}

function attendeesSection(p) {
  const section = el('section', 'attendees');
  const head = el('div', 'section-head');
  const n = p.attendees.length;
  head.append(swooshHeading(`Who's Coming (${n})`));
  if (p.canEdit) {
    head.append(button('Attendee contact info', 'mail', 'button button-secondary button-small', () => openContacts(p)));
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
  p.waitlisted.forEach((a, i) => {
    const row = el('div', 'wait-row');
    row.append(el('span', 'wait-num', String(i + 1)), avatar(a, 'wait-face'));
    const words = el('span', 'wait-words');
    words.append(el('span', 'wait-name', a.name));
    if (a.line) {
      words.append(el('span', 'wait-line', a.line));
    }
    row.append(words);
    if (p.canEdit) {
      row.append(button('Offer a ticket', 'ticket', 'button button-secondary button-small', () => setTicketStatus(a, 'Ticket')));
      row.append(button('', 'edit', 'edit-icon', () => openTicket(p, a)));
    } else if (a.mine) {
      row.append(button('Leave', 'close', 'link-button', () => removeTicket(p, a)));
    }
    list.append(row);
  });
  section.append(list);
  return section;
}

// callout is the party's need-to-know line in the tinted card HCA-Team uses
// for an event's highlight, behind a megaphone.
function callout(p) {
  if (!p.needToKnow) {
    return null;
  }
  const card = el('div', 'highlight-card');
  card.append(el('div', 'highlight-icon', '📣'));
  const body = el('div', 'highlight-body');
  body.append(el('div', 'highlight-headline', 'Good to know'), el('p', 'highlight-text', p.needToKnow));
  card.append(body);
  return card;
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
    switchRow('Parents', 'Can parents hold a ticket?', p.parents, on => setFlags(p, {parents: on})),
    switchRow('Students', 'Can students hold a ticket?', p.students, on => setFlags(p, {students: on})),
    switchRow('Staff', 'Can staff hold a ticket?', p.staff, on => setFlags(p, {staff: on})),
    switchRow('Drop-off', 'Can kids come without a parent?', p.dropOff, on => setFlags(p, {dropOff: on})),
    switchRow('Parent ticket required', 'If a parent stays, do they need a ticket?', p.parentTicket, on => setFlags(p, {parentTicket: on})),
  );
  band.append(rows);
  const actions = el('div', 'host-actions');
  actions.append(button('Edit Party', 'edit', 'button', () => openParty(p)));
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

// factsCard is when, where, what a ticket costs, and who hosts - with Add to
// Calendar under the date.
function factsCard(p) {
  const card = sideCard('facts-card');
  const when = whenParts(p);
  const cal = googleCalendarLink(p);
  let calButton = null;
  if (cal) {
    calButton = el('a', 'button button-secondary button-small side-button');
    calButton.href = cal;
    calButton.target = '_blank';
    calButton.rel = 'noopener';
    calButton.append(svg('calendar'), el('span', '', 'Add to Calendar'));
  }
  card.append(sideRow('calendar', 'Date & Time', when.longDay || 'Date to come', when.time || '', calButton));
  // Where: the place in words for everyone, and under it the street address
  // - which only signed-in members ever see - with a map link.
  if (p.location || p.address) {
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
    card.append(sideRow('pin', 'Where', p.location || '', mapLink, note));
  }
  const ticketLines = [priceLine(p)];
  if (p.capacity) {
    ticketLines.push(p.remaining > 0 ? `${p.remaining} of ${p.capacity} tickets remaining` : `All ${p.capacity} tickets taken`);
  } else {
    ticketLines.push(`${p.sold} sold · no limit`);
  }
  if (p.minimum) {
    ticketLines.push(`Goes ahead with at least ${p.minimum} tickets sold`);
  }
  card.append(sideRow('ticket', 'Tickets', ...ticketLines));
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
    card.append(hostsRow);
  }
  return card;
}

// flyerCard is the party's poster in the rail, under the facts: the whole
// picture at the rail's width, a click to see it full size, and for whoever
// runs the party a way to put one up or take it down.
function flyerCard(p) {
  if (!p.flyerUrl && !p.canEdit) {
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
  if (p.canEdit) {
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
  const page = el('div', 'party-page');
  const top = el('div', 'detail-top');
  const back = link(partiesPath(), 'detail-back');
  back.append(svg('back'), el('span', '', 'Back to Parties'));
  top.append(back);
  page.append(top, hero(p));

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const side = el('div', 'detail-side');

  const marks = el('div', 'detail-marks');
  if (p.audience) {
    marks.append(el('span', 'audience-chip', p.audience));
  }
  if (p.availability !== 'available') {
    marks.append(el('span', 'avail avail-' + p.availability, availabilityLabel(p)));
  }
  for (const b of statusBadges(p)) {
    marks.append(b);
  }
  main.append(marks);
  main.append(el('h1', 'detail-title', p.title));
  if (p.subtitle) {
    main.append(el('p', 'detail-subtitle', p.subtitle));
  }
  // On a phone the facts come up under the title, where the rail would be.
  if (phone.matches) {
    const facts = factsCard(p);
    facts.classList.add('facts-inline');
    main.append(facts);
  }
  if (p.description) {
    main.append(paragraphs(p.description, 'prose detail-text'));
  } else if (p.summary) {
    main.append(el('p', 'detail-text', p.summary));
  }
  const note = callout(p);
  if (note) {
    main.append(note);
  }
  main.append(ticketBand(p));
  main.append(attendeesSection(p));
  const wait = waitlistSection(p);
  if (wait) {
    main.append(wait);
  }
  if (p.canEdit) {
    main.append(hostBand(p));
  }

  for (const card of [phone.matches ? null : factsCard(p), flyerCard(p), helpCard(p)]) {
    if (card) {
      side.append(card);
    }
  }
  cols.append(main, side);
  page.append(cols);
  return page;
}
