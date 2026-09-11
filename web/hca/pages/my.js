import {state, years, myRows, sortByStart, isPrevious, matches, rootOf, descendants} from '../state.js';
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
function list(rows) {
  const shown = rows.filter(r => (state.showPrevious || !isPrevious(r.act)) && (matches(r.act, query) || matches(rootOf(r.act), query)));
  const signed = new Set(shown.map(r => r.act));
  const events = sortByStart([...new Set(shown.map(r => rootOf(r.act)))]);
  if (!events.length) {
    const panel = el('div', 'panel');
    panel.append(el('div', 'panel-empty', query ? 'Nothing matches.' : "You haven't signed up for anything this year. Head to Sign Up!"));
    return panel;
  }
  const grid = el('div', 'card-grid');
  for (const event of events) {
    const own = signed.has(event) ? [event] : [];
    grid.append(activityCard(event, {signUps: [...own, ...sortByStart(descendants(event).filter(n => signed.has(n)))]}));
  }
  return grid;
}

export function myPage() {
  setTitle('My Activities');
  const rows = myRows();
  const present = new Set(rows.map(r => r.act.year));
  present.add(years().current);
  const options = [...present].sort().reverse();
  if (!year || !present.has(year)) {
    year = years().current;
  }
  const page = el('div', 'list-page');
  // The year is a pill beside the title, as on the opportunities page.
  const head = el('div', 'list-head');
  head.append(el('h1', '', 'My Activities'));
  const body = el('div');
  const render = () => {
    body.replaceChildren();
    body.append(toggle('Show Completed Events', state.showPrevious, on => {
      state.showPrevious = on;
      render();
    }));
    body.append(list(rows.filter(r => r.act.year === year)));
  };
  head.append(selectPill('calendar', options.map(y => ({key: y, label: y})), year, picked => {
    year = picked;
    render();
  }));
  render();
  setSearch('Search my sign ups', q => {
    query = q;
    render();
  });
  page.append(head, body);
  return page;
}
