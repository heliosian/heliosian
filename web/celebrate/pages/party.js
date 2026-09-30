import {state, canApprove, mayTake, whenParts, priceLine, money, partyCalendarLink, partyPath, myTickets, availabilityLabel} from '../state.js';
import {paragraphs} from '../dom.js';
import {el, link, svg, button, imageThumb, copyText, toast} from '/elements.js';
import {listPath} from '../chrome.js';
import {setTitle} from '/shell.js';
import {appOrigin} from '/appswitch.js';
import {openBuy, openParty, openTickets, openFreeTicket, openTicket, openPerson, openReassign, removeTicket, offerTickets, setPartyStatus, openContacts, savePartyFields, uploadImage, imageSearchOn, openImageSearch} from '../edit.js';
import {statusBadges} from '../cards.js';
import {openPhotoLightbox} from '/crop.js';
import {heroImageBar} from '/heroimage.js';
import {dateCard, parseWhen} from '/datecard.js';
import {render} from '/router.js';
import {personTile, personCard, peopleRow, andList} from '/people.js';
import {personRow} from '/personrow.js';

const phone = window.matchMedia('(max-width: 900px)');
phone.addEventListener('change', render);

function heroStamp(p) {
  if (!parseWhen(p.start)) {
    const stamp = el('div', 'hero-stamp hero-stamp-text');
    stamp.textContent = 'Date to come';
    return stamp;
  }
  return dateCard({start: p.start, end: p.end, location: p.location || '', add: partyCalendarLink(p)});
}

function heroTools(p) {
  const tools = el('div', 'hero-actions');
  const tool = (icon, label, onClick) => {
    const b = button('', icon, 'hero-action', onClick);
    b.title = label;
    b.setAttribute('aria-label', label);
    return b;
  };
  tools.append(tool('share', 'Share this party', async () => {
    const url = location.origin + partyPath(p);
    if (navigator.share) {
      try {
        await navigator.share({title: p.title, url});
        return;
      } catch {
      }
    }
    copyText(url, 'Link copied');
  }));
  return tools;
}

function hero(p, save) {
  const wrap = el('div', 'detail-hero');
  const image = imageThumb(p.imageUrl, p.title, 'detail-hero-image');
  if (p.imageUrl) {
    image.classList.add('is-openable');
    image.addEventListener('click', () => openPhotoLightbox(p.imageUrl));
  }
  wrap.append(image, heroTools(p), heroStamp(p));
  if (p.can.edit) {
    wrap.append(heroImageBar({image: p.image, imageUrl: p.imageUrl, query: p.title, tools: {uploadImage, imageSearchOn, openImageSearch}, save: image => save({image})}));
  }
  return wrap;
}

function ticketWords(p, mine) {
  switch (p.availability) {
    case 'available':
      return 'Tickets Available!';
    case 'waitlist':
      if (mine.some(a => a.status === 'Ticket')) {
        return 'Sold Out - Your Family Has Tickets';
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
  for (const a of mine.filter(a => a.status !== 'Ticket')) {
    const n = a.quantity || 1;
    band.append(personRow(a, {
      className: 'ticket-mine',
      open: true,
      lines: [`Waiting for ${n} ${n === 1 ? 'ticket' : 'tickets'}`, a.note ? el('div', 'ticket-mine-note', `\u201c${a.note}\u201d`) : null],
    }));
  }
  const actions = el('div', 'ticket-actions');
  const selling = p.availability === 'available' || p.availability === 'waitlist';
  const waiting = p.availability === 'waitlist' && mine.some(a => a.status !== 'Ticket');
  if (!mayTake(p)) {
    if (selling && !mine.length) {
      actions.append(el('span', 'ticket-kid-note', 'Ask a parent to sign in to get tickets.'));
    }
  } else if (selling || p.can.edit) {
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

function myTicketsSection(p) {
  const mine = myTickets(p).filter(a => a.status === 'Ticket');
  if (!mine.length) {
    return null;
  }
  const section = el('section', 'my-tickets');
  section.append(swooshHeading('My Tickets'));
  const list = el('div', 'my-ticket-list');
  for (const a of mine) {
    list.append(personRow(a, {
      className: 'my-ticket',
      open: true,
      lines: [a.price ? `Ticket · ${money(a.price)}` : 'Free ticket'],
      after: p.availability !== 'past' && a.can.reassign ? [button('Reassign', 'people', 'link-button', () => openReassign(p, a))] : [],
    }));
  }
  section.append(list);
  return section;
}

function swooshHeading(text) {
  return el('h2', 'section-swoosh', text);
}

function attendeeCard(p, a) {
  return personCard(a, {
    onClick: p.can.edit ? () => openTicket(p, a) : () => openPerson(a),
    title: p.can.edit ? `Open ${a.name}'s ticket` : a.name,
    line: a.line,
    mine: a.mine,
    rsvp: p.can.edit ? a.rsvp : '',
  });
}

function attendeesSection(p) {
  const section = el('section', 'attendees');
  const head = el('div', 'section-head');
  const n = p.attendees.length;
  head.append(swooshHeading(`Who's Coming (${n})`));
  if (p.can.edit) {
    const tools = el('div', 'section-tools');
    tools.append(button('Attendee contact info', 'mail', 'button button-secondary button-small', () => openContacts(p)));
    if (p.availability !== 'past') {
      const invite = el('a', 'button button-small' + (p.started ? ' button-secondary' : ''));
      invite.href = invitePath(p);
      invite.append(svg('calendar'), el('span', '', p.invited ? 'RSVPs on Helios When' : p.started ? 'The invite on Helios When' : 'Create Invite'));
      tools.append(invite);
    }
    head.append(tools);
  }
  section.append(head);
  if (n) {
    const grid = el('div', 'person-cards');
    for (const a of p.attendees) {
      grid.append(attendeeCard(p, a));
    }
    section.append(grid);
  } else {
    section.append(el('p', 'section-note', 'Nobody yet - be the first!'));
  }
  if (p.can.edit && p.availability !== 'past') {
    const foot = el('div', 'attendees-foot');
    foot.append(button('Add Free Ticket', 'ticket', 'button button-secondary button-small', () => openFreeTicket(p)));
    section.append(foot);
  }
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
    const n = a.quantity || 1;
    const after = a.mine ? [el('span', 'wait-mine', 'Your family')] : [];
    if (p.can.edit) {
      after.push(button(`Offer ${n === 1 ? 'a ticket' : n + ' tickets'}`, 'ticket', 'button button-secondary button-small', () => offerTickets(p, a)));
      after.push(button('', 'edit', 'edit-icon', () => openTicket(p, a)));
    } else if (a.mine && a.can.delete) {
      after.push(button('Leave waitlist', 'close', 'link-button', () => removeTicket(p, a)));
    }
    list.append(personRow(a, {
      className: 'wait-row',
      open: true,
      before: [el('span', 'wait-num', String(i + 1))],
      lines: [`${n} ${n === 1 ? 'ticket' : 'tickets'}${a.line ? ` · ${a.line}` : ''}`],
      after,
    }));
  });
  section.append(list);
  return section;
}

function callout(p) {
  if (!p.needToKnow) {
    return null;
  }
  const card = el('div', 'highlight-card');
  card.append(el('div', 'highlight-icon', p.noteEmoji || '📣'));
  const body = el('div', 'highlight-body');
  body.append(el('div', 'highlight-headline', p.noteTitle || 'Good to know'), el('p', 'highlight-text', p.needToKnow));
  card.append(body);
  return card;
}

function approvalButtons(actions, p) {
  actions.append(
    button('Approve', 'check', 'button', () => setPartyStatus(p, 'Open')),
    button('Hide', 'eye', 'button button-secondary', () => setPartyStatus(p, 'Hidden')),
  );
}

function approvalBand(p) {
  const band = el('div', 'host-band');
  const head = el('div', 'host-band-head');
  head.append(svg('hourglass'), el('span', '', 'Waiting for approval'));
  band.append(head);
  const actions = el('div', 'host-actions');
  approvalButtons(actions, p);
  band.append(actions);
  return band;
}

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

function factsCard(p) {
  const card = sideCard('facts-card');
  const when = whenParts(p);
  if (!when.longDay) {
    card.append(sideRow('calendar', 'Date & Time', 'Date to come', when.time || ''));
  }
  if (p.address) {
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
    card.append(sideRow('map', 'Where', p.location || '', mapLink, note));
  }
  const ticketLines = [priceLine(p)];
  if (p.capacity) {
    ticketLines.push(p.remaining > 0 ? `${p.remaining} of ${p.capacity} tickets remaining` : `All ${p.capacity} tickets taken`);
  } else {
    ticketLines.push(`${p.sold} sold · no limit`);
  }
  if (p.minimum && p.can.edit) {
    ticketLines.push(`Goes ahead with at least ${p.minimum} tickets sold`);
  }
  card.append(sideRow('ticket', 'Tickets', ...ticketLines));
  if (p.hostPeople.length || p.hosts) {
    card.append(peopleRow({
      title: p.hostPeople.length === 1 ? 'Host' : 'Hosts',
      line: p.hosts || andList(p.hostPeople.map(h => h.name)),
      tiles: p.hostPeople.map(h => personTile(h, {onClick: () => openPerson(h)})),
    }));
  }
  return card;
}

function flyerCard(p) {
  if (!p.flyerUrl && !p.can.edit) {
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
  if (p.can.edit) {
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
        const {name} = await uploadImage(file.files[0]);
        await savePartyFields(p, {flyer: name});
      } catch (err) {
        toast(err.message);
      }
    });
    const upload = el('label', 'button button-secondary button-small');
    upload.append(svg('up'), el('span', '', p.flyer ? 'Replace' : 'Upload'), file);
    bar.append(upload);
    if (p.flyer) {
      bar.append(button('Remove', 'trash', 'button button-secondary button-small', () => savePartyFields(p, {flyer: ''})));
    }
    card.append(bar);
  }
  return card;
}

function invitePath(p) {
  return appOrigin('when') + '/e/' + encodeURIComponent(p.partyId) + (p.started ? '' : '?invite=1');
}

function inviteCard(p) {
  if (!p.can.edit || p.availability === 'past') {
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
  const save = changes => savePartyFields(p, changes);
  const page = el('div', 'party-page');
  const top = el('div', 'detail-top');
  const back = link(listPath(state.tab, state.category), 'detail-back');
  back.append(svg('chevron-left'), el('span', '', 'Back to Parties'));
  top.append(back);
  if (p.can.edit) {
    const tools = el('div', 'detail-tools');
    tools.append(
      button('Edit Tickets', 'ticket', 'button button-small button-secondary', () => openTickets(p)),
      button('Edit Event', 'edit', 'button button-small button-secondary', () => openParty(p)),
    );
    top.append(tools);
  }
  page.append(top, hero(p, save));

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const side = el('div', 'detail-side');

  const marks = el('div', 'detail-marks');
  if (p.hosting) {
    marks.append(el('span', 'audience-chip hosting-chip', 'Hosting'));
  }
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
  const mine = myTicketsSection(p);
  if (mine) {
    main.append(mine);
  }
  main.append(attendeesSection(p));
  const wait = waitlistSection(p);
  if (wait) {
    main.append(wait);
  }
  if (canApprove(p)) {
    main.append(approvalBand(p));
  }

  for (const card of [inviteCard(p), phone.matches ? null : factsCard(p), flyerCard(p), helpCard(p)]) {
    if (card) {
      side.append(card);
    }
  }
  cols.append(main, side);
  page.append(cols);
  return page;
}
