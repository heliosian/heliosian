import {state, me, years, myRows, sortByStart, isPrevious, matches, rootOf, parentOf, descendants, family} from '../state.js';
import {el, toggle, selectPill} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {activityCard} from '../cards.js';

let year = null;
let query = '';

// list is the year's sign-ups in date order, one card per event, as on the
// opportunities page, each naming the sign-ups on it: the event itself when
// signed up for directly, then whatever was signed up for under it, by date
// and then in the event's own order - a committee alone does not say which
// event it belongs to. Events with no date come last.
function list(rows, who) {
  // A sign-up under a done or past event is done with it, whatever its own
  // status says.
  const over = node => isPrevious(node) || (parentOf(node) ? over(parentOf(node)) : false);
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

// myPage is the viewer's own sign-ups, or - given the address of someone in
// their household - that person's, which are theirs to change as well.
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
  // The year is a pill beside the title, as on the opportunities page.
  const head = el('div', 'list-head');
  head.append(el('h1', '', heading));
  const body = el('div');
  const render = () => {
    body.replaceChildren();
    body.append(toggle('Show Completed Events', state.showPrevious, on => {
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
