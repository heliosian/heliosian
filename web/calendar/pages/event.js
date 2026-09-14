import {state, daysLine, timeLine, calendarLink, sourceWords, dayType, eventDates, dayTypeClass, linkURL, call, isParty, mineWords, eventImage, weekdayShort, parseDate, spansDays, monthLabel, monthOf} from '../state.js';
import {el, link, svg, paragraphs, button, toast} from '../dom.js';
import {setTitle} from '../chrome.js';
import {audienceChips, blocks} from '../events.js';

// hero is the picture across the top of the page - the event's own, its
// first tag's, or the calendar's - with the date on a card at its corner:
// the weekday, the day, and the hours or the days it runs across.
function hero(e) {
  const wrap = el('div', 'detail-hero');
  const img = el('img');
  img.src = eventImage(e);
  img.alt = '';
  wrap.append(img);
  const first = eventDates(e)[0];
  const date = el('div', 'hero-date');
  date.append(el('span', 'hero-dow', weekdayShort(first)));
  date.append(el('span', 'hero-day', parseDate(first).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})));
  date.append(el('span', 'hero-time', spansDays(e) ? daysLine(e) : timeLine(e)));
  wrap.append(date);
  return wrap;
}

export function eventPage(e) {
  setTitle(e.title);
  const page = el('div', 'event-page');
  const top = el('div', 'detail-top');
  const back = el('a', 'detail-back');
  back.href = '/day/' + eventDates(e)[0];
  back.setAttribute('data-link', '');
  // The way back is to the calendar open to the event's month, so it says so.
  back.append(svg('back'), el('span', '', monthLabel(monthOf(eventDates(e)[0]))));
  top.append(back);
  page.append(top, hero(e));

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const marks = el('div', 'detail-marks');
  if (e.dayType) {
    marks.append(el('span', 'chip chip-day ' + dayTypeClass(e.dayType), e.dayType));
  }
  marks.append(audienceChips(e));
  main.append(marks);
  main.append(el('h1', 'detail-title', e.title));
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
  // The way into the app that runs a linked event, under when and where.
  if (e.link) {
    side.append(linkedCard(e));
  }

  side.append(sourceCard(e));
  cols.append(side);
  page.append(cols);
  return page;
}

const aboutWords = {
  celebrate: 'Hosted by families in the community. The party page has the hosts, the price, and who is coming.',
  team: 'Run by the Helios Community Association. The event page has the roles to fill, who runs it, and who has signed up.',
};

// stampWords is a sheet timestamp, "2026-08-01 09:00" or a bare date, as
// "Aug 1, 2026".
function stampWords(stamp) {
  const day = (stamp || '').slice(0, 10);
  return parseDate(day) ? parseDate(day).toLocaleDateString('en-US', {month: 'short', day: 'numeric', year: 'numeric'}) : stamp;
}

function outLink(href, words) {
  const a = el('a', 'link-button');
  a.href = href;
  a.target = '_blank';
  a.rel = 'noopener';
  a.append(svg('open'), el('span', '', words));
  return a;
}

// keywordsEditor is the search words an admin can change from the page:
// the words as chips, Edit making them a box, Save writing them to the
// event's Overrides row.
function keywordsEditor(e) {
  const wrap = el('div');
  const paint = () => {
    wrap.replaceChildren();
    const line = el('div', 'side-line', 'Search words' + (e.keywords && e.keywords.length ? ':' : ': none'));
    wrap.append(line);
    if (e.keywords && e.keywords.length) {
      const chips = el('div', 'side-keywords');
      for (const w of e.keywords) {
        chips.append(el('span', 'side-keyword', w));
      }
      wrap.append(chips);
    }
    const edit = button('Edit search words', 'pencil', 'link-button', () => {
      wrap.replaceChildren();
      const input = el('input', 'side-keywords-edit');
      input.type = 'text';
      input.value = (e.keywords || []).join(', ');
      input.placeholder = 'words, separated by commas';
      const save = button('Save', 'check', 'button button-small', async () => {
        const keywords = input.value.split(',').map(w => w.trim()).filter(Boolean);
        const res = await fetch('/api/calendar/keywords', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: e.id, keywords})});
        if (!res.ok) {
          toast(await res.text());
          return;
        }
        toast('Search words saved');
        const {load} = await import('../app.js');
        await load();
      });
      const cancel = button('Cancel', null, 'button button-secondary button-small', paint);
      const row = el('div', 'modal-actions');
      row.append(save, cancel);
      wrap.append(el('div', 'side-line', 'Search words:'), input, row);
      input.focus();
    });
    wrap.append(edit);
  };
  paint();
  return wrap;
}

// sourceCard says where the event's dates came from and links to the
// original: the feed's event in Google Calendar, the school's calendar page
// for a line of the year calendar, the page on the app that runs a party
// or an HCA event, or who added it by hand and when. An HCA event the
// school also lists says so, with the way to HCA-Team's page. An admin also
// sees what the admins' tabs did: the classifier's filing and any
// correction, with its note.
function sourceCard(e) {
  const card = el('div', 'side-card side-card-help');
  const row = el('div', 'side-row');
  const icon = el('div', 'side-icon');
  icon.append(svg('info'));
  const body = el('div', 'side-row-body');
  body.append(el('div', 'side-title', sourceWords(e)));
  const lines = [];
  switch (e.source) {
    case 'pdf':
      lines.push(`Read from the school\u2019s ${e.year ? e.year.replace('-', '\u2013') + ' ' : ''}year calendar.`);
      if (e.sourceTitle && e.sourceTitle !== e.title) {
        lines.push(`Printed there as \u201c${e.sourceTitle}\u201d.`);
      }
      break;
    case 'google':
      if (e.updated) {
        lines.push(`Last changed by the school on ${stampWords(e.updated)}.`);
      }
      if (e.sourceTitle && e.sourceTitle !== e.title) {
        lines.push(`Listed there as \u201c${e.sourceTitle}\u201d.`);
      }
      break;
    case 'sheet': {
      const names = state.model.names || {};
      const who = names[e.addedBy] || e.addedBy;
      if (who) {
        lines.push(`Added by ${who}${e.added ? ' on ' + stampWords(e.added) : ''}.`);
      }
      if (e.sourceNote) {
        lines.push(`Source: ${e.sourceNote}`);
      }
      break;
    }
    default:
      lines.push(aboutWords[e.source] || '');
  }
  for (const words of lines.filter(Boolean)) {
    body.append(el('div', 'side-line', words));
  }
  if (e.source === 'pdf') {
    body.append(outLink(e.sourceUrl, 'Open the school\u2019s calendar page'));
  } else if (e.source === 'google' && e.sourceUrl) {
    body.append(outLink(e.sourceUrl, 'Open in Google Calendar'));
  } else if (e.source === 'sheet' && e.sourceUrl) {
    // The proof behind a hand-added event, named by where it lives; the
    // Veracross portal wants a parent login.
    const host = new URL(e.sourceUrl).hostname.replace(/^www\./, '');
    body.append(outLink(e.sourceUrl, host.includes('veracross') ? 'Open the source on Veracross' : `Open the source (${host})`));
    if (host.includes('veracross')) {
      body.append(el('div', 'side-line', 'Veracross asks for your parent portal login.'));
    }
  } else if (e.link) {
    body.append(outLink(linkURL(e), isParty(e) ? 'Open on Helios Celebrate' : 'Open on HCA-Team'));
  }
  // The school's listing of an HCA event carries HCA-Team's link too.
  if (e.link && e.source !== 'celebrate' && e.source !== 'team') {
    body.append(el('div', 'side-line', 'Also listed on HCA-Team, which runs it.'), outLink(linkURL(e), 'Open on HCA-Team'));
  }
  if (state.model.user.isAdmin) {
    const p = (state.model.provenance || {})[e.id] || {};
    const admin = el('div', 'side-admin');
    admin.append(el('div', 'side-admin-title', 'For admins'));
    if (p.enriched) {
      admin.append(el('div', 'side-line', `Filed under its tags by Claude${p.model ? ' (' + p.model + ')' : ''} on ${stampWords(p.enriched)}.`));
    }
    if (p.corrected && p.corrected.length) {
      admin.append(el('div', 'side-line', `Corrected in Overrides: ${p.corrected.join(', ').toLowerCase()}.${p.note ? ' Note: \u201c' + p.note + '\u201d' : ''}`));
    }
    if (!e.link || e.source === 'google' || e.source === 'pdf' || e.source === 'sheet') {
      admin.append(keywordsEditor(e));
    }
    // A copy of this event, four weeks on, from the add-event tool.
    const clone = link('/admin?clone=' + encodeURIComponent(e.id) + '&weeks=4', 'link-button');
    clone.append(svg('copy'), el('span', '', 'Clone this event 4 weeks on'));
    admin.append(clone);
    body.append(admin);
  }
  row.append(icon, body);
  card.append(row);
  return card;
}

const standing = {
  available: 'Tickets available', waitlist: 'Full, and taking a waitlist', 'sold-out': 'Sold out',
  closed: 'Tickets are not on sale', past: 'This party has happened',
  open: 'Volunteers wanted', full: 'Every spot is taken', done: 'This event is done',
};

const mineStanding = {
  going: {celebrate: 'Your household holds tickets', team: 'Someone in your household has signed up'},
  waitlisted: {celebrate: 'Your household is on the waitlist'},
};

const linkedTitles = {celebrate: 'Fun(d)raiser party', team: 'HCA volunteer event'};

const linkedIcons = {celebrate: 'sun', team: 'people'};

const seeWords = {celebrate: 'See the party', team: 'See the event'};

// linkedCard leads a linked event's rail: how signing up stands - the
// household's own standing when it has one - and the way to its page on the
// app that runs it, where the tickets or the sign-up are.
function linkedCard(e) {
  const kind = isParty(e) ? 'celebrate' : 'team';
  const card = el('div', 'side-card side-card-linked' + (e.mine ? ' is-mine' : ''));
  const row = el('div', 'side-row');
  const icon = el('div', 'side-icon');
  icon.append(svg(linkedIcons[kind]));
  const body = el('div', 'side-row-body');
  const line = e.mine ? `${mineWords(e)} · ${mineStanding[e.mine][kind]}` : standing[e.availability] || '';
  body.append(el('div', 'side-title', linkedTitles[kind]), el('div', 'side-line', line));
  const go = el('a', 'button side-button');
  go.href = linkURL(e);
  go.append(svg('open'), el('span', '', e.mine ? seeWords[kind] : call(e) || seeWords[kind]));
  body.append(go);
  row.append(icon, body);
  card.append(row);
  return card;
}
