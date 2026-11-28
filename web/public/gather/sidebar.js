import {el, link, svg} from '/elements.js';
import {themed, sealMark} from '/themes.js';
import {mapQuery} from '/envelope.js';
import {appOrigin} from '/origins.js';

function splitAddress(location) {
  const comma = location.indexOf(',');
  if (comma < 1) {
    return [location, ''];
  }
  return [location.slice(0, comma), location.slice(comma + 1).trim()];
}

function outLink(href, words, title) {
  const a = link(href, 'nav-invite-map');
  a.target = '_blank';
  a.rel = 'noopener';
  if (title) {
    a.title = title;
  }
  a.append(el('span', '', words), el('span', 'nav-invite-arrow', '↗'));
  return a;
}

export function inviteHead({href, name, look, day, hours, calendar, place, location, hostedBy}) {
  const head = el('div', 'nav-invite-head');
  const top = link(href, 'nav-invite-top');
  const tile = themed(el('span', 'nav-invite-tile'), look);
  tile.append(el('span', 'env-seal', sealMark(look, name)));
  const words = el('span', 'nav-invite-heading');
  words.append(el('span', 'nav-invite-title', name));
  top.append(tile, words);
  head.append(top);
  const line = (icon, first, second) => {
    const row = el('div', 'nav-invite-line');
    const text = el('div', 'nav-invite-words');
    text.append(el('div', 'nav-invite-first', first));
    if (second) {
      text.append(second);
    }
    row.append(svg(icon), text);
    return row;
  };
  const when = calendar ? outLink(calendar, hours || 'Add to calendar', 'Add to Google Calendar') : hours ? el('div', 'nav-invite-second', hours) : null;
  head.append(line('calendar', day, when));
  if (place || location) {
    const [street, town] = place ? [place, location.split(',').slice(0, 2).join(',')] : splitAddress(location);
    head.append(line('map', street, outLink('https://www.google.com/maps/search/?api=1&query=' + encodeURIComponent(mapQuery({place, location})), town || 'Map')));
  }
  if (hostedBy) {
    head.append(el('div', 'nav-invite-sub', 'Hosted by ' + hostedBy));
  }
  return head;
}

export function madeWith() {
  const box = el('div', 'nav-made-with');
  const home = link('/', 'nav-made-with-gather');
  home.append(el('span', 'nav-made-with-words', 'Made with'), svg('mail-open'), el('span', 'nav-made-with-name', 'Gather'));
  const helios = el('a', 'nav-made-with-helios', 'Helios Community Apps');
  helios.href = appOrigin('home');
  const from = el('div', 'nav-made-with-from');
  from.append('from ', helios);
  box.append(home, from);
  return box;
}
