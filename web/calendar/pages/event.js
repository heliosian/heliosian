import {whenLine, daysLine, timeLine, calendarLink, sourceWords, dayType, eventDates, dayTypeClass, linkURL, call} from '../state.js';
import {el, link, svg, paragraphs, button, copyText} from '../dom.js';
import {setTitle} from '../chrome.js';
import {audienceChips, blocks} from '../events.js';

export function eventPage(e) {
  setTitle(e.title);
  const page = el('div', 'event-page');
  const top = el('div', 'detail-top');
  const back = el('a', 'detail-back');
  back.href = '/day/' + eventDates(e)[0];
  back.setAttribute('data-link', '');
  back.append(svg('back'), el('span', '', 'That day'));
  top.append(back);
  page.append(top);

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const marks = el('div', 'detail-marks');
  if (e.dayType) {
    marks.append(el('span', 'chip chip-day ' + dayTypeClass(e.dayType), e.dayType));
  }
  marks.append(audienceChips(e));
  main.append(marks);
  main.append(el('h1', 'detail-title', e.title));
  main.append(el('p', 'detail-when', whenLine(e)));
  if (e.description) {
    main.append(paragraphs(e.description, 'prose detail-text'));
  }
  const type = e.dayType ? dayType(e.dayType) : null;
  if (type && e.allDay) {
    const card = el('div', 'plan-card ' + dayTypeClass(type.name));
    const head = el('div', 'plan-head');
    head.append(el('div', 'plan-type', type.name));
    card.append(head);
    if (type.blocks.length) {
      card.append(blocks(type));
    } else {
      card.append(el('div', 'plan-note', 'No dropoff, school, pickup, or aftercare.'));
    }
    main.append(el('h2', 'section-title', 'The day for ' + (e.classrooms.length ? e.classrooms.join(', ') : 'everyone')));
    main.append(card);
  }
  cols.append(main);

  const side = el('div', 'detail-side');
  if (e.link) {
    side.append(linkedCard(e));
  }
  const when = el('div', 'side-card');
  const whenRow = el('div', 'side-row');
  const whenIcon = el('div', 'side-icon');
  whenIcon.append(svg('clock'));
  const whenBody = el('div', 'side-row-body');
  whenBody.append(el('div', 'side-title', daysLine(e)));
  if (!e.allDay) {
    whenBody.append(el('div', 'side-line', timeLine(e)));
  }
  const add = el('a', 'button button-secondary button-small side-button');
  add.href = calendarLink(e);
  add.target = '_blank';
  add.rel = 'noopener';
  add.append(svg('calendar'), el('span', '', 'Add to Google Calendar'));
  whenBody.append(add);
  whenRow.append(whenIcon, whenBody);
  when.append(whenRow);
  if (e.location) {
    const whereRow = el('div', 'side-row');
    const whereIcon = el('div', 'side-icon');
    whereIcon.append(svg('pin'));
    const whereBody = el('div', 'side-row-body');
    whereBody.append(el('div', 'side-title', 'Where'), el('div', 'side-line', e.location));
    whereRow.append(whereIcon, whereBody);
    when.append(whereRow);
  }
  side.append(when);

  const about = el('div', 'side-card side-card-help');
  const aboutRow = el('div', 'side-row');
  const aboutIcon = el('div', 'side-icon');
  aboutIcon.append(svg('info'));
  const aboutBody = el('div', 'side-row-body');
  aboutBody.append(el('div', 'side-title', sourceWords(e)));
  aboutBody.append(el('div', 'side-line', aboutWords[e.source] || 'Something wrong? Tell the office, and the calendar admins can correct it here.'));
  aboutBody.append(button('Copy link', 'copy', 'link-button', () => copyText(location.origin + location.pathname, 'Link copied')));
  aboutRow.append(aboutIcon, aboutBody);
  about.append(aboutRow);
  side.append(about);
  cols.append(side);
  page.append(cols);
  return page;
}

const aboutWords = {
  celebrate: 'Hosted by families in the community. The party page has the hosts, the price, and who is coming.',
  team: 'Run by the Helios Community Association. The event page has the roles to fill, who runs it, and who has signed up.',
};

const standing = {
  available: 'Tickets available', waitlist: 'Full, and taking a waitlist', 'sold-out': 'Sold out',
  closed: 'Tickets are not on sale', past: 'This party has happened',
  open: 'Volunteers wanted', full: 'Every spot is taken', done: 'This event is done',
};

const linkedTitles = {celebrate: 'Fun(d)raiser party', team: 'HCA volunteer event'};

const linkedIcons = {celebrate: 'sun', team: 'people'};

const seeWords = {celebrate: 'See the party', team: 'See the event'};

// linkedCard leads a linked event's rail: how signing up stands, and the way
// to its page on the app that runs it, where the tickets or the sign-up are.
function linkedCard(e) {
  const card = el('div', 'side-card side-card-linked');
  const row = el('div', 'side-row');
  const icon = el('div', 'side-icon');
  icon.append(svg(linkedIcons[e.source]));
  const body = el('div', 'side-row-body');
  body.append(el('div', 'side-title', linkedTitles[e.source]), el('div', 'side-line', standing[e.availability] || ''));
  const go = el('a', 'button side-button');
  go.href = linkURL(e);
  go.append(svg('open'), el('span', '', call(e) || seeWords[e.source]));
  body.append(go);
  row.append(icon, body);
  card.append(row);
  return card;
}
