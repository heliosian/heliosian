import {partyPath, availabilityLabel, myTickets} from './state.js';
import {parseWhen} from '/datecard.js';
import {badge} from './dom.js';
import {el, link, svg, button} from '/elements.js';
import {card} from '/cardgrid.js';
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
  const past = p.availability === 'past';
  const media = [];
  const stamp = dateStamp(p);
  if (stamp) {
    media.push(stamp);
  }
  if (p.hosting) {
    media.push(el('span', 'card-chip card-chip-mine', 'Hosting'));
  }
  if (past) {
    media.push(el('span', 'card-chip card-chip-past', 'Past'));
  }
  const marks = statusBadges(p);
  if (p.availability !== 'available') {
    marks.push(availabilityBadge(p));
  }
  const mine = myTickets(p);
  let under = null;
  if (mine.length) {
    under = el('div', 'card-under');
    for (const a of mine) {
      const item = el('div', 'card-under-item');
      item.append(svg(a.status === 'Ticket' ? 'ticket' : 'hourglass'), el('span', 'card-under-name', a.name));
      if (a.status !== 'Ticket') {
        item.append(el('span', 'card-under-wait', a.status === 'Offered' ? 'offered' : 'waitlist'));
      }
      under.append(item);
    }
  }
  return card({
    href: partyPath(p),
    imageUrl: p.imageUrl,
    title: p.title,
    subtitle: p.subtitle,
    text: p.summary,
    media,
    chips: p.audience ? [p.audience] : [],
    marks,
    under,
    foot: [footButton(p, mine)],
    note: spotsNote(p),
    className: [p.status !== 'Open' && 'is-muted', past && 'is-past'].filter(Boolean).join(' '),
  });
}
