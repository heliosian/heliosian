import {state, me, household, familyShown, myTickets} from '../state.js';
import {el, link} from '../dom.js';
import {setTitle, setSearch, renderChrome} from '../chrome.js';
import {partyCard} from '../cards.js';
import {checkbox} from '../edit.js';

let query = '';

// myPage is the family's parties - every party anyone in the household holds
// a ticket to or waits for, guests they brought included, in date order - or
// with an address one member's alone: the rail's links under My Family's
// Parties.
export function myPage(email) {
  const only = email ? household().find(p => p.email === email) : null;
  // The viewer's own page is "My Parties"; another member's is theirs by
  // name; with no address it is the whole family's.
  const self = only && only.email === me().email;
  const title = self ? 'My Parties' : only ? `${only.name}'s Parties` : "My Family's Parties";
  setTitle(title);
  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', title));
  main.append(el('p', 'page-intro', self
    ? 'The parties you hold a ticket to or wait for.'
    : only
      ? `The parties ${only.name} holds a ticket to or waits for.`
      : "The parties your family holds tickets to. Don't see one? Make sure you're signed in with the address the directory has for you."));
  head.append(main);
  // The past switch: on, the parties that have been follow those to come in
  // each section; off, only what is ahead. It holds across the family's
  // pages and the rail's counts, for the visit.
  const past = checkbox('Show past parties', state.showPast);
  past.wrap.classList.add('page-head-switch');
  past.input.addEventListener('change', () => {
    state.showPast = past.input.checked;
    paint();
    renderChrome();
  });
  head.append(past.wrap);
  page.append(head);
  const body = el('div');
  page.append(body);

  const paint = () => {
    body.replaceChildren();
    // One grid in date order - the family's page is every party anyone in
    // the household is on, guests they brought included, since each card
    // already names who; a member's page is the parties with them on it.
    const on = p => only
      ? [...p.attendees, ...p.waitlisted].some(a => a.email === only.email)
      : myTickets(p).length > 0;
    const list = state.model.parties.filter(p => familyShown(p) && on(p) && (!query || p.title.toLowerCase().includes(query)));
    if (list.length) {
      const grid = el('div', 'card-grid');
      for (const p of list) {
        grid.append(partyCard(p));
      }
      body.append(grid);
      return;
    }
    const panel = el('div', 'panel');
    const empty = el('div', 'panel-empty');
    // With the past hidden, say so rather than that there is nothing.
    const hidden = !state.showPast && state.model.parties.some(p => !familyShown(p) && on(p));
    empty.append(hidden ? 'Nothing coming up. Turn on Show past parties for the ones that have been, or ' : 'No parties yet. ',
      link('/', '', hidden ? 'see what\'s available' : 'See what\'s available'), '.');
    panel.append(empty);
    body.append(panel);
  };
  paint();
  setSearch('Search your parties…', q => {
    query = q;
    paint();
  });
  return page;
}
