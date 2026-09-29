import {el, avatar} from '/elements.js';
import {openPersonCard} from '/personcard.js';

export function gradeBadge(grade, colors) {
  const badge = el('span', 'grade-badge', /^kindergarten$/i.test(grade) ? 'K' : grade.replace(/^grade\s*/i, ''));
  badge.title = grade;
  const color = (colors || {})[grade];
  if (color) {
    badge.style.background = `color-mix(in srgb, ${color} 65%, black)`;
  }
  return badge;
}

export function face(person, className, colors) {
  const node = avatar(person, className);
  if (person.grade) {
    node.append(gradeBadge(person.grade, colors));
  }
  return node;
}

export function target(className, opts) {
  const node = el(opts.href ? 'a' : opts.onClick || opts.button ? 'button' : 'div', className);
  if (opts.href) {
    node.href = opts.href;
  }
  if (opts.onClick || opts.button) {
    node.type = 'button';
  }
  if (opts.onClick) {
    node.addEventListener('click', opts.onClick);
  }
  return node;
}

export function personRow(person, opts = {}) {
  const row = target('person-row' + (opts.className ? ' ' + opts.className : ''), opts);
  const name = opts.name || person.name || person.email;
  if (opts.title) {
    row.title = opts.title;
  }
  let photo = face(person, 'person-row-face', opts.gradeColors);
  if (opts.open) {
    const open = el('button', 'person-row-open');
    open.type = 'button';
    open.title = `About ${person.name || person.email}`;
    open.append(photo);
    open.addEventListener('click', () => openPersonCard(person));
    photo = open;
  }
  const words = el('div', 'person-row-words');
  words.append(el('div', 'person-row-name', name));
  for (const line of (opts.lines || []).filter(Boolean)) {
    words.append(typeof line === 'string' ? el('div', 'person-row-line', line) : line);
  }
  row.append(...(opts.before || []), photo, words, ...(opts.after || []));
  return row;
}
