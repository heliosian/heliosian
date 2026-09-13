import {eventPath, timeLine, whenLine, audienceWords, categoryTags, plan, specials, isSchoolDay, dayLabel, dayTypeClass, today, selectedClassrooms, classroomNames} from './state.js';
import {el, link, svg} from './dom.js';

// audienceChips are who an event is for, compressed: Everyone, a band, or
// classrooms; then what it is, as its category tags.
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

// eventRow is one event in a day's list: its hours in the left column, then
// the title, the chips, and the place.
export function eventRow(e, opts = {}) {
  const row = link(eventPath(e), 'event-row' + (e.dayType ? ' has-day-type' : ''));
  const when = el('div', 'event-when');
  when.append(el('span', 'event-time', opts.showDate ? whenLine(e) : timeLine(e)));
  row.append(when);
  const body = el('div', 'event-body');
  const title = el('div', 'event-title', e.title);
  body.append(title);
  const marks = el('div', 'event-marks');
  if (e.dayType) {
    marks.append(el('span', 'chip chip-day ' + dayTypeClass(e.dayType), e.dayType));
  }
  marks.append(audienceChips(e));
  body.append(marks);
  if (e.location) {
    const place = el('div', 'event-place');
    place.append(svg('pin'), el('span', '', e.location));
    body.append(place);
  }
  row.append(body);
  row.append(svg('chevron'));
  return row;
}

// planChips say what kind of day it is for the selected classrooms, when it
// is not simply regular: "Early Dismissal · Hummingbirds", "No School".
export function planChips(date) {
  const wrap = el('div', 'chips');
  const groups = specials(date);
  const all = selectedClassrooms();
  for (const g of groups) {
    const words = g.classrooms.length === all.length ? g.name : `${g.name} · ${g.classrooms.join(', ')}`;
    wrap.append(el('span', 'chip chip-day ' + dayTypeClass(g.name), words));
  }
  return wrap;
}

// dayHeading heads a day's events: the date, Today when it is, and the plan
// chips beside it.
export function dayHeading(date) {
  const head = el('div', 'day-heading' + (date === today() ? ' is-today' : ''));
  const words = link('/day/' + date, 'day-heading-date');
  words.append(el('span', 'day-heading-label', dayLabel(date)));
  if (date === today()) {
    words.append(el('span', 'day-heading-today', 'Today'));
  }
  head.append(words);
  if (isSchoolDay(date)) {
    head.append(planChips(date));
  }
  return head;
}

// blocks lays out a day type's four blocks as a strip: each block's name
// over its hours, and a block the day does not have greyed.
export function blocks(type) {
  const strip = el('div', 'blocks');
  for (const name of ['Dropoff', 'School', 'Pickup', 'Aftercare']) {
    const block = type.blocks.find(b => b.name === name);
    const cell = el('div', 'block' + (block ? '' : ' is-off'));
    cell.append(el('div', 'block-name', name));
    cell.append(el('div', 'block-hours', block ? `${clockShort(block.start)} – ${clockShort(block.end)}` : '—'));
    strip.append(cell);
  }
  return strip;
}

function clockShort(hhmm) {
  const [h, m] = hhmm.split(':').map(Number);
  const hour = ((h + 11) % 12) + 1;
  return m ? `${hour}:${String(m).padStart(2, '0')}` : `${hour}`;
}

// planCards are the day plan for the selected classrooms: one card per day
// type in force, naming the classrooms it covers when they are not all.
export function planCards(date) {
  const wrap = el('div', 'plan-cards');
  const groups = plan(date);
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

// specialRow is one upcoming departure from the regular day, as a link to it.
export function specialRow(item) {
  const row = link('/day/' + item.date, 'special-row');
  row.append(el('span', 'special-date', dayLabel(item.date)));
  const chips = el('span', 'chips');
  const all = selectedClassrooms();
  for (const g of item.groups) {
    const words = g.classrooms.length === all.length ? g.name : `${g.name} · ${g.classrooms.join(', ')}`;
    chips.append(el('span', 'chip chip-day ' + dayTypeClass(g.name), words));
  }
  row.append(chips);
  return row;
}

export function emptyNote(words) {
  const panel = el('div', 'panel');
  panel.append(el('div', 'panel-empty', words));
  return panel;
}
