import {el, svg} from '/elements.js';
import {face, target} from '/personrow.js';

const rsvpWords = {yes: 'RSVP: Yes', maybe: 'RSVP: Maybe', no: 'RSVP: No', none: 'No RSVP yet'};

const roleWords = {chair: 'Chair', option: 'Chair opt'};

const noteIcon = '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="5" y="3" width="14" height="18" rx="2.5" fill="currentColor"/><path class="note-ink" d="M5 5.5A2.5 2.5 0 0 1 7.5 3h9A2.5 2.5 0 0 1 19 5.5V7.5H5z"/><rect class="note-ink" x="8" y="10.5" width="8" height="1.6" rx="0.8"/><rect class="note-ink" x="8" y="14" width="8" height="1.6" rx="0.8"/><rect class="note-ink" x="8" y="17.5" width="5" height="1.6" rx="0.8"/></svg>';

export function roleTag(role) {
  return el('span', 'person-role is-' + role, roleWords[role]);
}

function rsvpLine(rsvp) {
  return el('div', 'person-rsvp is-' + rsvp, rsvpWords[rsvp]);
}

export function personTile(person, opts = {}) {
  const tile = target('person-tile' + (opts.editable ? ' is-editable' : '') + (opts.role ? ' is-' + opts.role : ''), opts);
  tile.title = opts.title || person.name || person.email;
  const photo = face(person, 'person-tile-face', opts.gradeColors);
  if (opts.note) {
    const bubble = el('span', 'note-badge');
    bubble.setAttribute('aria-label', 'Left a note');
    bubble.innerHTML = noteIcon;
    photo.append(bubble);
  }
  tile.append(photo, el('div', 'person-tile-name', opts.name || person.name || person.email));
  if (opts.role) {
    tile.append(roleTag(opts.role));
  }
  if (opts.rsvp) {
    tile.append(rsvpLine(opts.rsvp));
  }
  if (opts.tip || opts.note) {
    const tip = el('div', 'tile-tip');
    tip.setAttribute('role', 'tooltip');
    if (opts.tip) {
      tip.append(el('div', '', opts.tip));
    }
    if (opts.note) {
      tip.append(el('div', 'tile-tip-note', `“${opts.note}”`));
    }
    tile.append(tip);
  }
  return tile;
}

export function andList(names) {
  return names.length > 1 ? names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1] : names[0] || '';
}

export function peopleRow({title, line, notes = [], tiles, after = []}) {
  const row = el('div', 'side-row people-row');
  const icon = el('div', 'side-icon');
  icon.append(svg('people'));
  const body = el('div', 'side-row-body');
  body.append(el('div', 'side-title', title));
  if (line) {
    body.append(el('div', 'side-line', line));
  }
  body.append(...notes);
  if (tiles.length) {
    const list = el('div', 'person-tiles');
    list.append(...tiles);
    body.append(list);
  }
  body.append(...after);
  row.append(icon, body);
  return row;
}

export function offerTile(tile, action) {
  const offer = el('div', 'person-offer');
  offer.append(tile, action);
  return offer;
}

export function personCard(person, opts = {}) {
  const card = target('person-card', opts);
  card.title = opts.title || person.name || person.email;
  const photo = face(person, 'person-card-face', opts.gradeColors);
  if (opts.corner) {
    photo.append(opts.corner);
  }
  card.append(photo, el('div', 'person-card-name', person.name || person.email));
  if (opts.line) {
    card.append(el('div', 'person-card-line', opts.line));
  }
  if (opts.mine) {
    card.append(el('div', 'person-card-mine', 'Your family'));
  }
  if (opts.rsvp) {
    card.append(rsvpLine(opts.rsvp));
  }
  return card;
}
