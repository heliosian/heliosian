import {state, isAdmin, managed, matches, groupPath} from '../state.js';
import {el, svg, link, button, iconButton, copyText, pageHead} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {navigate} from '../app.js';

export const visibilityWords = {hidden: 'Hidden', members: 'Visible to members', everyone: 'Visible to everyone'};

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
  head.append(icon, words);
  card.append(head);
  if (g.description) {
    card.append(el('div', 'group-desc', g.description));
  }
  const meta = el('div', 'group-meta');
  const count = g.members.length;
  meta.append(el('span', 'chip', `${count} ${count === 1 ? 'member' : 'members'}`));
  if (managed(g) && g.visibility !== 'hidden') {
    meta.append(el('span', 'chip', visibilityWords[g.visibility]));
  }
  card.append(meta);
  return card;
}

const suggestionWords = {
  party: 'Everyone holding tickets, and the parents of any student who does, as the party\'s Magic Tag lists them now.',
  activity: 'Everyone on the volunteer list, and the parents of any student on it, as the activity\'s Magic Tag lists them now.',
  tag: 'Everyone under this tag of yours in Helios Who?, and the parents of any student among them, as the tag stands now.',
};

// A suggestion's icon is the mark of the app its Magic Tag is kept up in -
// Celebrate's for a party, HCA-Team's for an activity - the coloured marks
// the app switch wears, so the card says where the people would come from.
const suggestionApp = {party: 'celebrate', activity: 'team', tag: 'who'};

function suggestionCard(s) {
  const card = el('div', 'group-card');
  const head = el('div', 'group-card-head');
  const icon = el('div', 'group-icon group-icon-app');
  const mark = el('img');
  mark.src = `/brand/apps/${suggestionApp[s.kind]}.png`;
  mark.alt = '';
  icon.append(mark);
  const words = el('div', 'group-words');
  words.append(el('div', 'group-title', s.name));
  words.append(el('div', 'group-desc', suggestionWords[s.kind]));
  head.append(icon, words);
  card.append(head);
  const meta = el('div', 'group-meta');
  meta.append(button('Make this group', 'plus', 'button button-small', () => navigate('/new?from=' + encodeURIComponent(s.key))));
  meta.append(el('span', '', 'Managed by ' + s.managers.map(m => m.name).join(', ')));
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
  const suggested = el('div');
  suggested.append(el('h2', 'section-title', 'Suggested groups'));
  suggested.append(el('p', 'page-lead', 'A party you host, an activity you co-chair or a tag of yours in Helios Who? with no group yet. Make one and it starts with the right rule and managers; change anything before you save.'));
  const suggestedList = el('div', 'group-list');
  suggested.append(suggestedList);
  const others = el('div');
  others.append(el('h2', 'section-title', 'Other groups'));
  others.append(el('p', 'page-lead', 'Groups their managers have opened to everyone in Loop, and groups you are on whose managers have opened them to their members. Open one to see who is on it; if you are, you can take yourself off it there, or put yourself back.'));
  const othersList = el('div', 'group-list');
  others.append(othersList);
  // A group the viewer has archived is off the page altogether; the rail's
  // Archived is where it lives.
  const render = query => {
    list.replaceChildren();
    suggestedList.replaceChildren();
    othersList.replaceChildren();
    const mine = state.model.groups.filter(g => managed(g) && !g.archived && matches(g, query));
    const suggestions = state.model.suggestions.filter(s => !query || s.name.toLowerCase().includes(query));
    const theirs = state.model.groups.filter(g => !managed(g) && !g.archived && matches(g, query));
    if (!mine.length) {
      empty.textContent = query ? 'No group of yours matches that.' : 'You manage no groups yet. Make one, and its address is yours to hand out.';
      list.append(empty);
    }
    cards(list, mine);
    suggested.hidden = !suggestions.length;
    for (const s of suggestions) {
      const slot = el('div', 'card-slot');
      slot.append(suggestionCard(s));
      suggestedList.append(slot);
    }
    others.hidden = !theirs.length;
    cards(othersList, theirs);
  };
  render('');
  setSearch(render);
  page.append(list, suggested, others);
  return page;
}
