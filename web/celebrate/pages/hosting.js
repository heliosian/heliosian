import {state, isAdmin, hostedParties, pendingParties, partyPath, whenLine, money} from '../state.js';
import {el, link, button, tabs, thumb, svg} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {openParty, setPartyStatus} from '../edit.js';
import {availabilityBadge} from '../cards.js';

let query = '';

// hostRow is one party as its host sees it: the picture, the title and date,
// the counts, and the ticket switch's state.
function hostRow(p) {
  const row = link(partyPath(p), 'row is-link' + (p.status !== 'Open' ? ' is-muted' : ''));
  row.append(thumb(p.imageUrl, p.title));
  const body = el('div', 'row-body');
  const label = el('div', 'label');
  label.append(el('span', '', whenLine(p) || 'Date to come'));
  body.append(label);
  body.append(el('div', 'row-title', p.title));
  const counts = [];
  counts.push(`${p.sold} sold${p.capacity ? ` of ${p.capacity}` : ''}`);
  if (p.waiting) {
    counts.push(`${p.waiting} waiting`);
  }
  counts.push(`${money(p.sold * p.price)} raised`);
  body.append(el('div', 'row-text', counts.join(' · ')));
  if (p.status === 'Pending') {
    body.append(el('div', 'row-pending', 'Waiting for approval - an admin will open it for tickets.'));
  }
  row.append(body);
  const actions = el('div', 'row-actions');
  actions.append(availabilityBadge(p));
  if (isAdmin() && p.status === 'Pending') {
    actions.append(button('Approve', 'check', 'button button-small', () => setPartyStatus(p, 'Open')));
  }
  actions.append(button('', 'edit', 'edit-icon', () => openParty(p)));
  const chevron = svg('chevron');
  chevron.classList.add('chevron');
  actions.append(chevron);
  row.append(actions);
  return row;
}

export function hostingPage() {
  setTitle('Hosting');
  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Hosting'));
  main.append(el('p', 'page-intro', 'The parties you run. Open one to see who is coming, offer waitlist places, and change its settings.'));
  head.append(main);
  head.append(button('Host a Party', 'plus', 'button', () => openParty(null)));
  page.append(head);
  const body = el('div');
  const bar = el('div', 'list-bar');
  page.append(bar, body);
  const items = [{key: 'mine', label: 'My Parties', count: hostedParties().length}];
  if (isAdmin()) {
    items.push({key: 'approvals', label: 'Approval Needed', count: pendingParties().length});
    items.push({key: 'all', label: 'All Parties', count: state.model.parties.length});
  }
  if (!items.some(i => i.key === state.tab)) {
    state.tab = 'mine';
  }
  const paint = () => {
    bar.replaceChildren(tabs(items, state.tab, key => {
      state.tab = key;
      paint();
    }));
    body.replaceChildren();
    let list = state.tab === 'mine' ? hostedParties() : state.tab === 'approvals' ? pendingParties() : state.model.parties;
    list = list.filter(p => !query || p.title.toLowerCase().includes(query));
    const panel = el('div', 'panel');
    if (!list.length) {
      panel.append(el('div', 'panel-empty', state.tab === 'approvals' ? 'Nothing waiting for approval.' : state.tab === 'mine' ? "You aren't hosting a party yet - Host a Party to post one." : 'Nothing matches.'));
    }
    for (const p of list) {
      panel.append(hostRow(p));
    }
    body.append(panel);
  };
  paint();
  setSearch('Search these parties…', q => {
    query = q;
    paint();
  });
  return page;
}
