import {state, household, isFamily} from '../state.js';
import {el, avatar, link} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {partyCard} from '../cards.js';

let query = '';

// myPage is the family's parties: one section per member of the household,
// each with the parties they hold a ticket to (or wait for), and a section
// for the guests the family has brought along.
// myPage is the family's parties, or with an address one member's alone -
// the rail's links under My Family's Parties.
export function myPage(email) {
  const only = email ? household().find(p => p.email === email) : null;
  setTitle(only ? `${only.name}'s Parties` : "My Family's Parties");
  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', only ? `${only.name}'s Parties` : "My Family's Parties"));
  main.append(el('p', 'page-intro', only
    ? `The parties ${only.name} holds a ticket to or waits for.`
    : "The parties your family holds tickets to. Don't see one? Make sure you're signed in with the address the directory has for you."));
  head.append(main);
  page.append(head);
  const body = el('div');
  page.append(body);

  const paint = () => {
    body.replaceChildren();
    const all = state.model.parties;
    let any = false;
    const sections = [];
    for (const person of household()) {
      if (only && person.email !== only.email) {
        continue;
      }
      const mine = all.filter(p => [...p.attendees, ...p.waitlisted].some(a => a.email === person.email));
      sections.push({person, parties: mine});
    }
    // Guests the family brought: tickets by name, billed to the household.
    const guests = all.filter(p => [...p.attendees, ...p.waitlisted].some(a => a.mine && !isFamily(a.email)));
    if (guests.length && !only) {
      sections.push({person: {name: 'Guests', email: ''}, parties: guests, guests: true});
    }
    for (const section of sections) {
      // What is still to come first, then what has been.
      const list = section.parties.filter(p => !query || p.title.toLowerCase().includes(query))
        .sort((a, b) => (a.availability === 'past') - (b.availability === 'past'));
      if (!list.length) {
        continue;
      }
      any = true;
      const heading = el('div', 'family-head');
      heading.append(avatar(section.person, 'family-face'), el('h2', 'family-name', section.person.name));
      body.append(heading);
      const grid = el('div', 'card-grid');
      for (const p of list) {
        const mine = section.guests
          ? [...p.attendees, ...p.waitlisted].filter(a => a.mine && !isFamily(a.email))
          : [...p.attendees, ...p.waitlisted].filter(a => a.email === section.person.email);
        grid.append(partyCard(p, {mine}));
      }
      body.append(grid);
    }
    if (!any) {
      const panel = el('div', 'panel');
      const empty = el('div', 'panel-empty');
      empty.append('No parties yet. ', link('/', '', 'See what\'s available'), '.');
      panel.append(empty);
      body.append(panel);
    }
  };
  paint();
  setSearch('Search your parties…', q => {
    query = q;
    paint();
  });
  return page;
}
