import {eventPath, timeColumn, whenLine, timeRange, clock, audienceWords, categoryTags, tagName, eventColors, plan, selectedClassrooms, classroomNames, linkURL} from './state.js';
import {dayTypeClass} from '/daytype.js';
import {el, link, svg} from '/elements.js';

export function audienceChips(e) {
  const wrap = el('div', 'chips');
  for (const word of audienceWords(e)) {
    wrap.append(el('span', 'chip chip-who', word));
  }
  for (const tag of categoryTags(e)) {
    wrap.append(el('span', 'chip chip-tag', tagName(tag)));
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
    card.append(el('div', 'plan-type', 'No School'));
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
    }
    wrap.append(card);
  }
  return wrap;
}

const openFolds = new Set();

export function planFolds(date, groups = plan(date)) {
  const wrap = el('div', 'plan-cards');
  const all = selectedClassrooms();
  if (!groups.length) {
    groups = [{name: 'No School', type: {blocks: []}, classrooms: all}];
  }
  for (const g of groups) {
    const school = g.type.blocks.find(b => b.name === 'School');
    const words = [school ? clock(school.end).trimStart() : ''];
    if (g.classrooms.length !== all.length || all.length !== classroomNames().length) {
      words.push(g.classrooms.join(', '));
    }
    const summary = el(g.type.blocks.length ? 'summary' : 'div', 'plan-fold-head');
    summary.append(svg(g.type.role === 'regular' ? 'school' : 'bell'));
    const text = el('span', 'plan-fold-text');
    text.append(el('span', 'plan-fold-title', g.name));
    if (words.some(Boolean)) {
      text.append(el('span', 'plan-fold-time', words.filter(Boolean).join(' · ')));
    }
    summary.append(text);
    if (!g.type.blocks.length) {
      const card = el('div', 'plan-fold ' + dayTypeClass(g.name));
      card.append(summary);
      wrap.append(card);
      continue;
    }
    const chevron = svg('chevron-right');
    chevron.classList.add('plan-fold-chevron');
    summary.append(chevron);
    const card = el('details', 'plan-fold ' + dayTypeClass(g.name));
    card.open = openFolds.has(g.name);
    card.addEventListener('toggle', () => {
      if (card.open) {
        openFolds.add(g.name);
      } else {
        openFolds.delete(g.name);
      }
    });
    card.append(summary, blocks(g.type));
    wrap.append(card);
  }
  return wrap;
}

export function emptyNote(words) {
  const panel = el('div', 'panel');
  panel.append(el('div', 'panel-empty', words));
  return panel;
}
