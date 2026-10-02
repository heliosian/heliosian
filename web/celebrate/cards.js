import {partyPath, availabilityLabel, myTickets} from './state.js';
import {parseWhen} from '/datecard.js';
import {badge} from './dom.js';
import {el, link, svg, imageThumb, button} from '/elements.js';
import {openBuy} from './edit.js';

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

export function availabilityBadge(p) {
  return el('span', 'avail avail-' + p.availability, availabilityLabel(p));
}

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

function footButton(p, mine) {
  if ((p.availability === 'available' || p.offered) && p.can.buy) {
    return button('Get Tickets', null, 'button button-small', () => openBuy(p));
  }
  if (p.availability === 'waitlist' && !mine.length && p.can['join-waitlist']) {
    return button('Join Waitlist', null, 'button button-small', () => openBuy(p));
  }
  return link(partyPath(p), 'button button-secondary button-small', 'Learn More');
}

export function partyCard(p) {
  const slot = el('div', 'card-slot');
  slot.append(partyCardBody(p));
  return slot;
}

function partyCardBody(p) {
  const card = el('div', 'card' + (p.status !== 'Open' ? ' is-muted' : ''));
  const media = link(partyPath(p), 'card-media');
  media.append(imageThumb(p.imageUrl, p.title, 'card-image'));
  const stamp = dateStamp(p);
  if (stamp) {
    media.append(stamp);
  }
  if (p.hosting) {
    media.append(el('span', 'card-chip card-chip-hosting', 'Hosting'));
  }
  if (p.availability === 'past') {
    card.classList.add('is-past');
    media.append(el('span', 'card-chip card-chip-past', 'Past'));
  }
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
        item.append(el('span', 'card-under-wait', a.status === 'Offered' ? 'offered' : 'waitlist'));
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
