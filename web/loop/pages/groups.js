import {state, matches, groupPath, domain} from '../state.js';
import {pageHead} from '../dom.js';
import {el, svg, link, button, iconButton, copyText} from '/elements.js';
import {setTitle, setSearch} from '/shell.js';
import {navigate} from '/router.js';

export const visibilityWords = {hidden: 'Hidden', members: 'Visible to members', everyone: 'Visible to everyone'};

function groupCard(g) {
  const card = link(groupPath(g), 'group-card');
  const head = el('div', 'group-card-head');
  const icon = el('div', 'group-icon');
  icon.append(svg('groups'));
  const words = el('div', 'group-words');
  words.append(el('div', 'group-title', g.title));
  if (g.address) {
    const address = el('div', 'group-address');
    address.append(el('span', '', g.address));
    address.append(iconButton('copy', 'Copy the address', 'tiny', () => copyText(g.address, 'Address copied')));
    words.append(address);
  }
  head.append(icon, words);
  card.append(head);
  if (g.description) {
    card.append(el('div', 'group-desc', g.description));
  }
  const meta = el('div', 'group-meta');
  const count = g.members.length;
  meta.append(el('span', 'chip', `${count} ${count === 1 ? 'member' : 'members'}`));
  if (g.member) {
    const subscribed = el('span', 'manager-state is-subscribed');
    subscribed.append(svg('check'), el('span', '', 'Subscribed'));
    meta.append(subscribed);
  }
  if (g.run) {
    const star = el('span', 'group-manage');
    star.title = 'You manage this email list';
    star.append(svg('star'));
    card.append(star);
  }
  if (g.run && g.visibility !== 'hidden') {
    meta.append(el('span', 'chip', visibilityWords[g.visibility]));
  }
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
  setTitle('My Email Lists');
  const page = el('div', 'list-page');
  page.append(pageHead('My Email Lists', [button('New Email List', 'plus', 'button', () => navigate('/new'))]));
  page.append(el('p', 'page-lead', 'Each email list is an address at ' + domain + ' whose members follow from its rules, drawn from the directory as it changes.'));
  const list = el('div', 'group-list');
  const empty = el('div', 'panel-empty');
  const render = query => {
    list.replaceChildren();
    const mine = state.model.groups.filter(g => (g.run || g.member) && matches(g, query));
    if (!mine.length) {
      empty.textContent = query ? 'No email list of yours matches that.' : 'You are on no email lists and manage none yet. Make one, and its address is yours to hand out.';
      list.append(empty);
    }
    cards(list, mine);
  };
  render('');
  setSearch('', render);
  page.append(list);
  return page;
}
