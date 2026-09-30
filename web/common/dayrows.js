import {el, link, svg} from '/elements.js';
import {parseWhen} from '/datecard.js';
import {appOrigin} from '/appswitch.js';
import {dayTypeClass} from '/daytype.js';

export function anchor(base, path, className, text) {
  if (!base) {
    return link(path, className, text);
  }
  const a = el('a', className, text);
  a.href = base + path;
  return a;
}

export function eventPath(e) {
  if (e.address) {
    return '/e/' + encodeURIComponent(e.address);
  }
  return '/e/' + e.id.split('/').map(encodeURIComponent).join('/');
}

export function standing(event) {
  const pill = el('span', 'wg-standing');
  if (event.mine && event.call) {
    pill.append(svg('check'), el('span', '', event.call));
    return pill;
  }
  if (event.answer === 'yes') {
    pill.append(svg('check'), el('span', '', 'Going'));
    return pill;
  }
  if (event.answer === 'maybe') {
    pill.append(el('span', '', 'Maybe'));
    return pill;
  }
  return null;
}

function action(event, base) {
  if (event.invited && !event.hosted && !event.cancelled && !event.answer) {
    const pill = anchor(base, event.path, 'wg-pill is-rsvp', 'RSVP');
    pill.title = 'You’re invited - answer on its page';
    return pill;
  }
  if (event.link && event.call && !event.mine && ['available', 'open', 'waitlist'].includes(event.availability)) {
    const pill = el('a', 'wg-pill', event.call);
    pill.href = appOrigin(event.linkApp) + event.link;
    return pill;
  }
  return null;
}

export function eventRow(event, {base, time, className}) {
  const title = anchor(base, event.path, 'wg-title', event.title);
  if (event.hosted) {
    const star = svg('star');
    star.classList.add('host-star');
    star.setAttribute('aria-label', 'You host this');
    title.prepend(star);
  }
  return dayRow(className, {title, chips: [standing(event) || action(event, base)], time}, () => title.click());
}

export function dayChip(name, words) {
  return el('span', 'wg-day-chip ' + dayTypeClass(name), words);
}

export function dayBar(day, side = []) {
  const bar = el('div', 'wg-day');
  const label = el('span', 'wg-day-label');
  if (day) {
    const date = parseWhen(day).date;
    label.append(el('span', 'wg-day-weekday', date.toLocaleDateString('en-US', {weekday: 'short'})), el('span', 'wg-day-sep', '·'), el('span', '', date.toLocaleDateString('en-US', {month: 'short', day: 'numeric'})));
  } else {
    label.append(el('span', '', 'Ongoing'));
  }
  const end = el('span', 'wg-day-side');
  end.append(...side);
  bar.append(label, end);
  return bar;
}

export function dayRow(className, {title, chips, time, after}, click) {
  const row = el('li', 'wg-row ' + className);
  row.dataset.row = '';
  const side = el('div', 'wg-side');
  side.append(el('span', 'wg-time', time));
  const shown = chips.filter(Boolean);
  if (shown.length) {
    const tags = el('div', 'wg-tags');
    tags.append(...shown);
    side.append(tags);
  }
  const text = el('div', 'wg-text');
  text.append(side, title, ...(after ? [after] : []));
  row.append(el('span', 'wg-dot'), text);
  row.addEventListener('click', e => {
    if (!e.target.closest('a')) {
      click(e);
    }
  });
  return row;
}
