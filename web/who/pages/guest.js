import {state, lists} from '../state.js';
import {withFrom, copyButton, contactRow} from '../dom.js';
import {el, svg, iconLink} from '/elements.js';
import {personLink, photoOrInitials} from '../people.js';
import {fromCrumbs, breadcrumbs} from '../crumbs.js';

function findGuest(id) {
  for (const list of Object.values(lists)) {
    const guest = list.guests.find(g => g.id === id);
    if (guest) {
      return {list, guest};
    }
  }
  return null;
}

export function guestPage(id) {
  const page = document.createDocumentFragment();
  const found = findGuest(id);
  if (!found) {
    page.append(el('div', 'empty', 'Not found.'));
    return page;
  }
  const {list, guest} = found;
  const listHref = withFrom('/people?list=' + encodeURIComponent(list.key));
  const origin = fromCrumbs() || [['People', '/people']];
  page.append(breadcrumbs([...origin, [guest.name, null]]));

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
    const host = state.model.people.find(p => p.email && p.email === guest.purchaser);
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
      iconLink('mail', 'Email', 'mailto:' + guest.email),
      copyButton(guest.email),
    ]));
  }
  grid.append(right);
  card.append(grid);
  content.append(card);
  page.append(content);
  return page;
}
