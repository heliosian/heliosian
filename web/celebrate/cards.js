import {partyPath, parseWhen, availabilityLabel, myTickets, isKid} from './state.js';
import {el, link, svg, thumb, badge, button} from './dom.js';
import {openBuy} from './edit.js';

// statusBadges are the marks on a card for what is not simply open: pending,
// hidden, past.
export function statusBadges(p) {
  const out = [];
  if (p.status === 'Pending') {
    out.push(badge('Needs approval', 'pending'));
  }
  if (p.status === 'Hidden') {
    out.push(badge('Hidden', 'hidden'));
  }
  return out;
}

// availabilityBadge is the small line over a card's title, the way the old
// site wrote it: TICKETS AVAILABLE, WAITLIST, SOLD OUT, TICKETS CLOSED.
export function availabilityBadge(p) {
  return el('span', 'avail avail-' + p.availability, availabilityLabel(p));
}

// spotsNote is what the card says about room: "Tickets available" while
// there is plenty, then a count once the party is getting full - under 40%
// of its capacity left, or ten tickets or fewer, whichever comes first - so
// the number is a nudge, not a tally. A waitlist says how many are waiting.
export function spotsNote(p) {
  if (p.availability === 'available') {
    if (p.capacity > 0 && (p.remaining <= 10 || p.remaining < p.capacity * 0.4)) {
      return `${p.remaining} ticket${p.remaining === 1 ? '' : 's'} left`;
    }
    return 'Tickets available';
  }
  if (p.availability === 'waitlist' && p.waiting) {
    return `${p.waiting} waiting`;
  }
  return '';
}

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'short'});

// dateStamp is the tear-off calendar page in the picture's corner, as the
// volunteer portal draws it: the month as a red band, the day large, the
// weekday under it - or, once the party has been, the year, since a past
// one may be a season or three back and the weekday no longer matters. A
// party with no date yet gets no stamp.
function dateStamp(p) {
  const start = parseWhen(p.start);
  if (!start) {
    return null;
  }
  const stamp = el('div', 'card-stamp');
  const under = p.availability === 'past' ? String(start.date.getFullYear()) : start.date.toLocaleDateString('en-US', {weekday: 'short'}).toUpperCase();
  stamp.append(el('div', 'card-stamp-month', monthFormat.format(start.date).toUpperCase()),
    el('div', 'card-stamp-day', String(start.date.getDate())),
    el('div', 'card-stamp-dow', under));
  return stamp;
}

// footButton is the card's action: Get Tickets or Join Waitlist where the
// party sells, else Learn More into the page. A family already on a full
// party - holding a ticket, or waiting - is not asked to join its waitlist:
// their names sit on the card, and the page carries their place.
function footButton(p, mine) {
  // A student browses; a parent takes the tickets.
  if (isKid()) {
    return link(partyPath(p), 'button button-secondary button-small', 'Learn More');
  }
  if (p.availability === 'available') {
    return button('Get Tickets', null, 'button button-small', () => openBuy(p));
  }
  if (p.availability === 'waitlist' && !mine.length) {
    return button('Join Waitlist', null, 'button button-small', () => openBuy(p));
  }
  return link(partyPath(p), 'button button-secondary button-small', 'Learn More');
}

// partyCard is the grid tile: the picture with its date stamp, then the
// title, the summary, the household's tickets on it - everyone in the
// family and the guests they brought, whichever page the card is on - and
// a foot with the action and what is left.
export function partyCard(p) {
  const slot = el('div', 'card-slot');
  slot.append(partyCardBody(p));
  return slot;
}

// partyCardBody is the card itself; partyCard puts it in a slot with a
// card of an accent peeking out behind, as Heliosian's do, the accents
// cycling by place in the grid.
function partyCardBody(p) {
  const card = el('div', 'card' + (p.status !== 'Open' ? ' is-muted' : ''));
  const media = link(partyPath(p), 'card-media');
  media.append(thumb(p.imageUrl, p.title, 'card-image'));
  const stamp = dateStamp(p);
  if (stamp) {
    media.append(stamp);
  }
  // A party the viewer hosts says so in the picture's top corner, so their
  // own stand out from the rest of the list.
  if (p.hosting) {
    media.append(el('span', 'card-chip card-chip-hosting', 'Hosting'));
  }
  // One that has been says so in the other top corner, and its stamp goes
  // grey, so it reads as past wherever the card sits - a list in date
  // order, the Past Parties tab.
  if (p.availability === 'past') {
    card.classList.add('is-past');
    media.append(el('span', 'card-chip card-chip-past', 'Past'));
  }
  // Who the party is for, as a chip in the picture's other corner. The
  // category is a filter, not a tag: it stays off the card.
  if (p.audience) {
    const chips = el('div', 'card-chips');
    chips.append(el('span', 'card-chip', p.audience));
    media.append(chips);
  }
  card.append(media);
  const body = el('div', 'card-body');
  const title = link(partyPath(p), 'card-title');
  title.textContent = p.title;
  body.append(title);
  if (p.subtitle) {
    body.append(el('div', 'card-subtitle', p.subtitle));
  }
  if (p.summary) {
    body.append(el('div', 'card-text clamp', p.summary));
  }
  const marks = el('div', 'card-marks');
  for (const b of statusBadges(p)) {
    marks.append(b);
  }
  if (p.availability !== 'available') {
    marks.append(availabilityBadge(p));
  }
  if (marks.children.length) {
    body.append(marks);
  }
  const mine = myTickets(p);
  if (mine.length) {
    const under = el('div', 'card-under');
    for (const a of mine) {
      const item = el('div', 'card-under-item');
      item.append(svg(a.status === 'Ticket' ? 'ticket' : 'hourglass'), el('span', 'card-under-name', a.name));
      if (a.status !== 'Ticket') {
        item.append(el('span', 'card-under-wait', 'waitlist'));
      }
      under.append(item);
    }
    body.append(under);
  }
  card.append(body);
  const foot = el('div', 'card-foot');
  foot.append(footButton(p, mine));
  const note = spotsNote(p);
  if (note) {
    foot.append(el('span', 'card-note', note));
  }
  card.append(foot);
  return card;
}
