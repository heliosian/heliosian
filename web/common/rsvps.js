import {el, svg} from '/elements.js';
import {parseWhen} from '/datecard.js';
import {anchor} from '/dayrows.js';

let open = false;

export function rsvpPanel(waiting, base) {
  const panel = el('section', 'wg-rsvps');
  panel.classList.toggle('is-open', open);
  const head = el('div', 'wg-rsvps-head');
  const n = waiting.length;
  const toggle = el('button', 'wg-rsvps-words');
  toggle.type = 'button';
  toggle.setAttribute('aria-expanded', String(open));
  toggle.append(el('span', '', `${n} ${n === 1 ? 'event needs' : 'events need'} your RSVP`), svg('chevron-right'));
  toggle.addEventListener('click', () => {
    open = !open;
    panel.classList.toggle('is-open', open);
    toggle.setAttribute('aria-expanded', String(open));
  });
  const all = anchor(base, '/mine/rsvp', 'wg-rsvps-all');
  all.append(el('span', '', 'View all'), svg('chevron-right'));
  head.append(svg('calendar'), toggle, all);
  const list = el('ol', 'wg-rsvp-list');
  for (const rsvp of waiting) {
    const row = el('li');
    const a = anchor(base, rsvp.path, 'wg-rsvp');
    a.title = 'You’re invited - answer on its page';
    const date = parseWhen(rsvp.start).date;
    const when = `${date.toLocaleDateString('en-US', {weekday: 'short'})}, ${date.toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}`;
    a.append(el('span', 'wg-rsvp-date', when), el('span', 'wg-rsvp-title', rsvp.title), el('span', 'wg-rsvp-pill', 'RSVP'));
    row.append(a);
    list.append(row);
  }
  panel.append(head, list);
  return panel;
}
