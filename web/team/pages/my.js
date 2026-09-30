import {state, me, years, myRows, sortByStart, matches, rootOf, parentOf, descendants, family} from '../state.js';
import {toggle, selectPill} from '../dom.js';
import {el} from '/elements.js';
import {setTitle, setSearch} from '/shell.js';
import {activityCard} from '../cards.js';

let year = null;
let query = '';

function list(rows, who) {
  const over = node => node.past || (parentOf(node) ? over(parentOf(node)) : false);
  const shown = rows.filter(r => (state.showPrevious || !over(r.act)) && (matches(r.act, query) || matches(rootOf(r.act), query)));
  const signed = new Set(shown.map(r => r.act));
  const events = sortByStart([...new Set(shown.map(r => rootOf(r.act)))]);
  if (!events.length) {
    const panel = el('div', 'panel');
    const nobody = who ? `${who.name} isn't signed up for anything this year.` : "You haven't signed up for anything this year. Head to Sign Up!";
    panel.append(el('div', 'panel-empty', query ? 'Nothing matches.' : nobody));
    return panel;
  }
  const grid = el('div', 'card-grid');
  const email = who ? who.email : me().email;
  for (const event of events) {
    const own = signed.has(event) ? [event] : [];
    grid.append(activityCard(event, {signUps: [...own, ...sortByStart(descendants(event).filter(n => signed.has(n)))], email}));
  }
  return grid;
}

export function myPage(email) {
  const who = email ? family().find(c => c.email === email) : null;
  const heading = who ? `${who.name}'s Activities` : 'My Activities';
  setTitle(heading);
  const rows = myRows(who ? who.email : me().email);
  const present = new Set(rows.map(r => r.act.year));
  present.add(years().current);
  const options = [...present].sort().reverse();
  if (!year || !present.has(year)) {
    year = years().current;
  }
  const page = el('div', 'list-page');
  const head = el('div', 'list-head');
  head.append(el('h1', '', heading));
  const body = el('div');
  const render = () => {
    body.replaceChildren();
    body.append(toggle('Show Past Events', state.showPrevious, on => {
      state.showPrevious = on;
      render();
    }));
    body.append(list(rows.filter(r => r.act.year === year), who));
  };
  head.append(selectPill('calendar', options.map(y => ({key: y, label: y})), year, picked => {
    year = picked;
    render();
  }));
  render();
  setSearch(who ? `Search ${who.name}'s sign ups` : 'Search my sign ups', q => {
    query = q;
    render();
  });
  page.append(head, body);
  return page;
}
