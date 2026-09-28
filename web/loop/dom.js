import {whoLink} from '/appswitch.js';
import {el, thumb} from '/elements.js';

export function pageHead(title, actions) {
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', title));
  head.append(main);
  if (actions && actions.length) {
    const wrap = el('div', 'page-actions');
    wrap.append(...actions);
    head.append(wrap);
  }
  return head;
}

export function personRow(person, extra, note) {
  const whole = !extra && !person.outside;
  const row = el(whole ? 'a' : 'div', 'person-row' + (whole ? ' person-row-link' : ''));
  if (whole) {
    row.href = whoLink(person.email);
    row.title = `${person.name} in Helios Who?`;
  }
  row.append(thumb(person, 'small'));
  const body = el('div', 'person-body');
  const name = el(extra && !person.outside ? 'a' : 'span', 'person-name', person.name);
  if (extra && !person.outside) {
    name.href = whoLink(person.email);
  }
  body.append(name);
  const words = [person.words, person.email].filter(Boolean).join(' · ');
  body.append(el('div', 'person-words', words));
  if (note) {
    body.append(el('div', 'person-note', note));
  }
  row.append(body);
  if (extra) {
    row.append(extra);
  }
  return row;
}
