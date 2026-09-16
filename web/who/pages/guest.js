import {lists, byEmail} from '../state.js';
import {el, svg, withFrom, iconButton, copyButton, contactRow} from '../dom.js';
import {personLink, photoOrInitials} from '../people.js';
import {fromCrumbs, breadcrumbs} from '../crumbs.js';
import {resetMain} from '../chrome.js';

function findGuest(id) {
  for (const list of Object.values(lists)) {
    const guest = list.guests.find(g => g.id === id);
    if (guest) {
      return {list, guest};
    }
  }
  return null;
}

// A guest's page: a party's ticket holder the directory has no record for,
// or someone a manager put on a group by hand, either way with the list
// they came in on and the address to reach them at.
export function renderGuestDetail(id) {
  const main = resetMain();
  const found = findGuest(id);
  if (!found) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
  }
  const {list, guest} = found;
  const listHref = withFrom('/people?list=' + encodeURIComponent(list.key));
  const origin = fromCrumbs() || [['People', '/people']];
  main.append(breadcrumbs([...origin, [guest.name, null]]));

  const content = el('div', 'container detail-content');
  const card = el('div', 'detail-card');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  const wrap = el('div', 'photo-wrap');
  wrap.append(photoOrInitials(null, guest.name, 'detail-photo detail-photo-empty'));
  left.append(wrap);
  grid.append(left);

  const right = el('div');
  const top = el('div', 'detail-top');
  const role = el('a', 'role-label role-label-guest', 'Guest');
  role.href = listHref;
  top.append(role);
  right.append(top);
  const name = el('h1', 'detail-name');
  name.append(el('span', '', guest.name));
  right.append(name);
  const party = el('div', 'detail-sub');
  const partyLink = el('a', '', list.name);
  partyLink.href = listHref;
  party.append(el('span', '', list.kind === 'group' ? 'On ' : 'Guest at '), partyLink);
  right.append(party);
  if (guest.purchaserName) {
    const by = el('div', 'detail-sub');
    by.append(el('span', '', 'Guest of '));
    const host = byEmail[guest.purchaser];
    if (host) {
      const hostLink = el('a', '', host.fullName);
      hostLink.href = personLink(host);
      by.append(hostLink);
    } else {
      by.append(el('span', '', guest.purchaserName));
    }
    right.append(by);
  }
  if (guest.email) {
    const emailValue = el('div', 'contact-value');
    emailValue.append(svg('mail'), el('span', '', guest.email));
    right.append(contactRow(emailValue, [
      iconButton('mail', 'Email', 'mailto:' + guest.email),
      copyButton(guest.email),
    ]));
  }
  grid.append(right);
  card.append(grid);
  content.append(card);
  main.append(content);
}
