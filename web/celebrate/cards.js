import {partyPath, parseWhen, availabilityLabel, myTickets, priceLine} from './state.js';
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

// spotsNote is "41 of 70 left" for a party with a cap and room, or nothing.
export function spotsNote(p) {
  if (p.capacity > 0 && p.availability === 'available') {
    return `${p.remaining} of ${p.capacity} left`;
  }
  if (p.availability === 'waitlist' && p.waiting) {
    return `${p.waiting} waiting`;
  }
  return '';
}

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'short'});

// dateStamp is the tear-off calendar page in the picture's corner, as the
// volunteer portal draws it: the month as a red band, the day large, the
// weekday under it. A party with no date yet gets no stamp.
function dateStamp(p) {
  const start = parseWhen(p.start);
  if (!start) {
    return null;
  }
  const stamp = el('div', 'card-stamp');
  stamp.append(el('div', 'card-stamp-month', monthFormat.format(start.date).toUpperCase()),
    el('div', 'card-stamp-day', String(start.date.getDate())),
    el('div', 'card-stamp-dow', start.date.toLocaleDateString('en-US', {weekday: 'short'}).toUpperCase()));
  return stamp;
}

// footButton is the card's action: Get Tickets or Join Waitlist where the
// party sells, else Learn More into the page.
function footButton(p) {
  if (p.availability === 'available') {
    return button('Get Tickets', null, 'button button-small', () => openBuy(p));
  }
  if (p.availability === 'waitlist') {
    return button('Join Waitlist', null, 'button button-small', () => openBuy(p));
  }
  return link(partyPath(p), 'button button-secondary button-small', 'Learn More');
}

// partyCard is the grid tile: the picture with its date stamp, then the
// title, the summary, and a foot with the action and what is left. opts.mine
// lists the household's tickets on it, for My Family's Parties.
export function partyCard(p, opts = {}) {
  const card = el('div', 'card' + (p.status !== 'Open' ? ' is-muted' : ''));
  const media = link(partyPath(p), 'card-media');
  media.append(thumb(p.imageUrl, p.title, 'card-image'));
  const stamp = dateStamp(p);
  if (stamp) {
    media.append(stamp);
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
  if (opts.mine) {
    const under = el('div', 'card-under');
    for (const a of opts.mine) {
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
  foot.append(footButton(p));
  const note = spotsNote(p) || (p.availability === 'available' ? priceLine(p) : '');
  if (note) {
    foot.append(el('span', 'card-note', note));
  }
  card.append(foot);
  return card;
}

// mineOn is the household's tickets on a party, for the My page's cards.
export function mineOn(p) {
  return myTickets(p);
}
