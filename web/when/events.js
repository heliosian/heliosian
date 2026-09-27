import {eventPath, timeColumn, whenLine, timeRange, audienceWords, categoryTags, eventColors, plan, specials, isSchoolDay, dayLabel, today, selectedClassrooms, classroomNames, linkURL} from './state.js';
import {dayTypeClass} from '/daytype.js';
import {el, link, svg} from '/elements.js';

export function audienceChips(e) {
  const wrap = el('div', 'chips');
  for (const word of audienceWords(e)) {
    wrap.append(el('span', 'chip chip-who', word));
  }
  for (const tag of categoryTags(e)) {
    wrap.append(el('span', 'chip chip-tag', tag));
  }
  return wrap;
}

export function roomDots(e) {
  const wrap = el('span', 'room-dots');
  const entries = eventColors(e);
  if (!entries.length) {
    const dot = el('span', 'room-dot is-plain');
    dot.title = 'Everyone';
    wrap.append(dot);
    return wrap;
  }
  for (const entry of entries) {
    const dot = el('span', 'room-dot' + (entry.color ? '' : ' is-plain'));
    if (entry.color) {
      dot.style.background = entry.color;
    }
    dot.title = entry.classrooms.join(', ');
    wrap.append(dot);
  }
  return wrap;
}

export function eventRow(e, opts = {}) {
  const row = link(eventPath(e), 'event-row');
  const first = eventColors(e)[0];
  if (first && first.color) {
    row.style.borderLeftColor = first.color;
  }
  row.append(el('span', 'event-time', opts.showDate ? whenLine(e) : timeColumn(e, opts.date)));
  const body = el('span', 'event-body');
  const title = el('span', 'event-title', e.title);
  if (e.hosted) {
    const star = svg('star');
    star.classList.add('host-star');
    star.setAttribute('aria-label', 'You host this');
    title.prepend(star);
  }
  body.append(title);
  if (e.location) {
    body.append(el('span', 'event-place', e.location));
  }
  if (e.link && e.call) {
    body.append(callPill(e));
  }
  row.append(body, roomDots(e));
  return row;
}

export function callPill(e) {
  const open = e.availability === 'available' || e.availability === 'open';
  const pill = el('span', 'event-pill' + (e.mine ? ' is-mine' : open ? ' is-open' : ''), e.call);
  pill.addEventListener('click', ev => {
    ev.preventDefault();
    ev.stopPropagation();
    location.href = linkURL(e);
  });
  return pill;
}

export function dayWords(date) {
  const wrap = el('span', 'day-words');
  const all = selectedClassrooms();
  specials(date).forEach((g, i) => {
    const words = g.classrooms.length === all.length ? g.name : `${g.name} · ${g.classrooms.join(', ')}`;
    wrap.append(el('span', 'day-type-words ' + dayTypeClass(g.name), (i ? ', ' : '') + words));
  });
  return wrap;
}

export function dayHeading(date, withDayWords) {
  const head = el('div', 'day-heading' + (date === today() ? ' is-today' : ''));
  const words = link('/day/' + date, 'day-heading-date');
  words.append(el('span', 'day-heading-label', dayLabel(date)));
  if (date === today()) {
    words.append(el('span', 'day-heading-today', 'Today'));
  }
  head.append(words);
  if (withDayWords && isSchoolDay(date)) {
    head.append(dayWords(date));
  }
  return head;
}

const blockIcons = {Dropoff: 'car', School: 'school', Pickup: 'car', Aftercare: 'people'};

export function blocks(type) {
  const strip = el('div', 'blocks');
  for (const name of ['Dropoff', 'School', 'Pickup', 'Aftercare']) {
    const block = type.blocks.find(b => b.name === name);
    const cell = el('div', 'block block-' + name.toLowerCase() + (block ? '' : ' is-off'));
    const icon = el('span', 'block-icon');
    icon.append(svg(blockIcons[name]));
    const body = el('span', 'block-body');
    body.append(el('span', 'block-name', name));
    body.append(el('span', 'block-hours', block ? timeRange(block.start, block.end).trimStart() : '—'));
    cell.append(icon, body);
    strip.append(cell);
  }
  return strip;
}

export function planCards(date, groups = plan(date)) {
  const wrap = el('div', 'plan-cards');
  const all = selectedClassrooms();
  if (!groups.length) {
    const card = el('div', 'plan-card plan-card-none');
    card.append(el('div', 'plan-type', isWeekend(date) ? 'Weekend' : 'No school day'));
    card.append(el('div', 'plan-note', isWeekend(date) ? 'Nothing on the school calendar.' : 'Outside the school year, or the year calendar has not been read yet.'));
    wrap.append(card);
    return wrap;
  }
  for (const g of groups) {
    const card = el('div', 'plan-card ' + dayTypeClass(g.name));
    const head = el('div', 'plan-head');
    head.append(el('div', 'plan-type', g.name));
    if (g.classrooms.length !== all.length || all.length !== classroomNames().length) {
      head.append(el('div', 'plan-rooms', g.classrooms.join(', ')));
    }
    card.append(head);
    if (g.type.blocks.length) {
      card.append(blocks(g.type));
    } else {
      card.append(el('div', 'plan-note', 'No dropoff, school, pickup, or aftercare.'));
    }
    wrap.append(card);
  }
  return wrap;
}

function isWeekend(date) {
  const day = new Date(date + 'T00:00:00').getDay();
  return day === 0 || day === 6;
}

export function specialRow(item) {
  const row = link('/day/' + item.date, 'special-row');
  row.append(el('span', 'special-date', dayLabel(item.date)));
  const all = selectedClassrooms();
  const words = el('span', 'day-words');
  item.groups.forEach((g, i) => {
    const text = g.classrooms.length === all.length ? g.name : `${g.name} · ${g.classrooms.join(', ')}`;
    words.append(el('span', 'day-type-words ' + dayTypeClass(g.name), (i ? ', ' : '') + text));
  });
  row.append(words);
  return row;
}

export function emptyNote(words) {
  const panel = el('div', 'panel');
  panel.append(el('div', 'panel-empty', words));
  return panel;
}
