import {myEvents, eventPath, eventImage, eventDates, parseDate, timeLine, answerOf} from '../state.js';
import {el, link, svg} from '../dom.js';
import {setTitle} from '../chrome.js';

// minePage is My Events: the viewer's own standing with what is coming
// up, a group per standing as the rail names them - RSVP, the invitations
// waiting for their reply; Attending, the yeses; Hosting - each event a card as
// HCA-Team's and Celebrate's grids have them.
//
// The rail's three rows open one group each: /mine/rsvp, /mine/attending,
// /mine/hosting.
export function minePage(which) {
  const mine = myEvents();
  const all = [
    ['rsvp', 'RSVP', 'RSVP', mine.waiting, 'Nothing waiting on you.', 'The invitations waiting for your reply. Each opens its page, where you answer.'],
    ['attending', 'Attending', 'Attending', mine.going, 'Nothing you said yes to is coming up.', 'The events you said yes to that are coming up.'],
    ['hosting', 'Hosting', 'Hosting', mine.hosted, 'Nothing you host is coming up. Add Event, under the day, starts one.', 'The events you host that are coming up.'],
    ['pending', 'Pending Approval', 'Pending Approval', mine.pending, 'Nothing you shared is waiting for approval.', 'The events you shared that wait for an admin to approve them onto the calendar.'],
  ];
  const one = all.find(g => g[0] === which);
  setTitle(one ? one[1] : 'My Events');
  const page = el('div', 'mine-page');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', one ? one[1] : 'My Events'));
  // A single list's title says what it is; the whole page says what it holds.
  if (!one) {
    main.append(el('p', 'page-intro', 'The invitations waiting for your reply, the events you host, and the ones you said yes to. Each opens its page, where the RSVPs are.'));
  }
  head.append(main);
  page.append(head);
  // The whole page shows only the groups with something in them.
  const shown = one ? [one] : all.filter(g => g[3].length);
  if (!shown.length) {
    page.append(el('p', 'mine-empty', 'Nothing coming up: no invitations waiting on you, nothing you said yes to, and nothing you host.'));
  }
  for (const [key, , label, events, empty] of shown) {
    const group = el('section', 'mine-group');
    // A page of one group needs no heading over it.
    if (!one) {
      group.append(el('h2', 'mine-heading', events.length ? `${label} · ${events.length}` : label));
    }
    if (!events.length) {
      group.append(el('p', 'mine-empty', empty));
    } else {
      const grid = el('div', 'mine-grid');
      for (const e of events) {
        grid.append(eventCard(e, key));
      }
      group.append(grid);
    }
    page.append(group);
  }
  return page;
}

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'short'});

// eventCard is one event as a card: its picture with the tear-off date
// stamp at the corner and a Hosting chip on the viewer's own, then the
// title, the hours and the place, and a foot with what to do - RSVP for an
// invitation waiting, Manage for one hosted, the answer for one attended.
function eventCard(e, key) {
  const slot = el('div', 'mine-slot');
  const card = el('div', 'mine-card');
  const media = link(eventPath(e), 'mine-card-media');
  const img = el('img', 'mine-card-image');
  img.src = eventImage(e);
  img.alt = '';
  img.loading = 'lazy';
  media.append(img);
  const first = parseDate(eventDates(e)[0]);
  const stamp = el('div', 'mine-stamp');
  stamp.append(el('div', 'mine-stamp-month', monthFormat.format(first).toUpperCase()), el('div', 'mine-stamp-day', String(first.getDate())), el('div', 'mine-stamp-dow', first.toLocaleDateString('en-US', {weekday: 'short'}).toUpperCase()));
  media.append(stamp);
  if (e.hosted) {
    media.append(el('span', 'mine-chip', 'Hosting'));
  }
  card.append(media);
  const body = el('div', 'mine-card-body');
  const title = link(eventPath(e), 'mine-card-title');
  title.textContent = e.title;
  body.append(title);
  const when = el('div', 'mine-card-line');
  when.append(svg('clock'), el('span', '', e.allDay ? 'All day' : timeLine(e, eventDates(e)[0])));
  body.append(when);
  if (e.location) {
    const where = el('div', 'mine-card-line');
    where.append(svg('pin'), el('span', '', e.location));
    body.append(where);
  }
  card.append(body);
  const foot = el('div', 'mine-card-foot');
  if (key === 'rsvp') {
    foot.append(link(eventPath(e), 'button button-small', 'RSVP'), el('span', 'mine-card-note is-waiting', 'Waiting for your reply'));
  } else if (key === 'pending') {
    foot.append(link(eventPath(e), 'button button-secondary button-small', 'Details'), el('span', 'mine-card-note is-waiting', 'Waiting for approval'));
  } else if (key === 'hosting') {
    foot.append(link(eventPath(e), 'button button-small', 'Manage'), el('span', 'mine-card-note', 'You host this'));
  } else {
    foot.append(link(eventPath(e), 'button button-secondary button-small', 'Details'), el('span', 'mine-card-note is-going', answerOf(e) === 'yes' ? '✓ You’re going' : ''));
  }
  card.append(foot);
  slot.append(card);
  return slot;
}
