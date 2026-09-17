import {state, isAdmin, managed, matches, groupPath} from '../state.js';
import {el, svg, link, button, iconButton, copyText, pageHead} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {navigate} from '../app.js';

function groupCard(g) {
  const card = link(groupPath(g), 'group-card');
  const head = el('div', 'group-card-head');
  const icon = el('div', 'group-icon');
  icon.append(svg('groups'));
  const words = el('div', 'group-words');
  words.append(el('div', 'group-title', g.title));
  const address = el('div', 'group-address');
  address.append(el('span', '', g.address));
  address.append(iconButton('copy', 'Copy the address', 'tiny', () => copyText(g.address, 'Address copied')));
  words.append(address);
  for (const alias of g.aliases) {
    words.append(el('div', 'group-address', `${alias}@${state.model.domain}`));
  }
  head.append(icon, words);
  card.append(head);
  if (g.description) {
    card.append(el('div', 'group-desc', g.description));
  }
  const meta = el('div', 'group-meta');
  const count = g.members.length;
  meta.append(el('span', 'chip', `${count} ${count === 1 ? 'member' : 'members'}`));
  if (g.visible) {
    meta.append(el('span', 'chip', 'Visible to everyone'));
  }
  meta.append(el('span', 'group-managers', 'Managed by ' + g.managers.map(m => m.name).join(', ')));
  card.append(meta);
  return card;
}

function cards(list, groups) {
  for (const g of groups) {
    const slot = el('div', 'card-slot');
    slot.append(groupCard(g));
    list.append(slot);
  }
}

export function groupsPage() {
  setTitle('My Groups');
  const page = el('div', 'list-page');
  page.append(pageHead(isAdmin() ? 'Every Group' : 'My Groups', [button('New group', 'plus', 'button', () => navigate('/new'))]));
  page.append(el('p', 'page-lead', 'Each group is an email address at ' + state.model.domain + ' whose members follow from its rules, drawn from the directory as it changes.'));
  const list = el('div', 'group-list');
  const empty = el('div', 'panel-empty');
  const others = el('div');
  others.append(el('h2', 'section-title', 'Visible to everyone'));
  others.append(el('p', 'page-lead', 'Groups their managers have opened to everyone in Loop. Open one to see who is on it; if you are, you can take yourself off it there, or put yourself back.'));
  const othersList = el('div', 'group-list');
  others.append(othersList);
  const render = query => {
    list.replaceChildren();
    othersList.replaceChildren();
    const mine = state.model.groups.filter(g => managed(g) && matches(g, query));
    const visible = state.model.groups.filter(g => !managed(g) && matches(g, query));
    if (!mine.length) {
      empty.textContent = query ? 'No group of yours matches that.' : 'You manage no groups yet. Make one, and its address is yours to hand out.';
      list.append(empty);
    }
    cards(list, mine);
    others.hidden = !visible.length;
    cards(othersList, visible);
  };
  render('');
  setSearch(render);
  page.append(list, others);
  return page;
}
