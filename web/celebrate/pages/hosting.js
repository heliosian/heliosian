import {state, isAdmin, isSystemAdmin, canApprove, hostedParties, pendingParties, partyPath, whenLine, money, canHost} from '../state.js';
import {el, link, button, thumb, svg} from '../dom.js';
import {tabStrip} from '/tabs.js';
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
  counts.push(`${money(p.raised)} raised`);
  body.append(el('div', 'row-text', counts.join(' · ')));
  if (p.status === 'Pending') {
    body.append(el('div', 'row-pending', 'Waiting for approval - an admin will open it for tickets.'));
  }
  row.append(body);
  const actions = el('div', 'row-actions');
  actions.append(availabilityBadge(p));
  if (canApprove(p)) {
    actions.append(button('Approve', 'check', 'button button-small', () => setPartyStatus(p, 'Open')));
  }
  if (p.canEdit) {
    actions.append(button('', 'edit', 'edit-icon', () => openParty(p)));
  }
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
  if (canHost()) {
    head.append(button('Host a Party', 'plus', 'button', () => openParty(null)));
  }
  page.append(head);
  const body = el('div');
  const bar = el('div', 'list-bar');
  page.append(bar, body);
  const items = [{key: 'mine', label: 'My Parties', count: hostedParties().length}];
  // Approval Needed goes with the admin list, hat or no hat; All Parties
  // with the hat.
  if (isSystemAdmin()) {
    items.push({key: 'approvals', label: 'Approval Needed', count: pendingParties().length});
  }
  if (isAdmin()) {
    items.push({key: 'all', label: 'All Parties', count: state.model.parties.length});
  }
  if (!items.some(i => i.key === state.hostingTab)) {
    state.hostingTab = 'mine';
  }
  const paint = () => {
    bar.replaceChildren(tabStrip(items, state.hostingTab, 2, key => {
      state.hostingTab = key;
      paint();
    }));
    body.replaceChildren();
    let list = state.hostingTab === 'mine' ? hostedParties() : state.hostingTab === 'approvals' ? pendingParties() : state.model.parties;
    list = list.filter(p => !query || p.title.toLowerCase().includes(query));
    const panel = el('div', 'panel');
    if (!list.length) {
      panel.append(el('div', 'panel-empty', state.hostingTab === 'approvals' ? 'Nothing waiting for approval.' : state.hostingTab === 'mine' ? (canHost() ? "You aren't hosting a party yet - Host a Party to post one." : "You aren't hosting a party. Hosting is closed for now.") : 'Nothing matches.'));
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
