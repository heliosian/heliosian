import {state, years, myRows, sortByStart, isPrevious, matches, rootOf} from '../state.js';
import {el, toggle, tabs} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {childRow} from '../cards.js';

let year = null;
let query = '';

function list(rows) {
  const root = el('div');
  const shown = rows.filter(r => (state.showPrevious || !isPrevious(r.act)) && (matches(r.act, query) || matches(rootOf(r.act), query)));
  let any = false;
  for (const category of state.model.categories) {
    // A child has no category of its own; it files under its root's.
    const items = shown.filter(r => rootOf(r.act).category === category.id);
    if (!items.length) {
      continue;
    }
    any = true;
    root.append(el('div', 'section-title', category.title));
    const panel = el('div', 'panel');
    const ordered = sortByStart(items.map(r => ({...r.act, _row: r}))).map(n => n._row);
    for (const r of ordered) {
      panel.append(childRow(r.act));
    }
    root.append(panel);
  }
  if (!any) {
    const panel = el('div', 'panel');
    panel.append(el('div', 'panel-empty', query ? 'Nothing matches.' : "You haven't signed up for anything this year. Head to Sign Up!"));
    root.append(panel);
  }
  return root;
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
  page.append(el('h1', '', 'My Activities'));
  const body = el('div');
  const render = () => {
    body.replaceChildren();
    body.append(tabs(options.map(y => ({key: y, label: y})), year, key => {
      year = key;
      render();
    }, true));
    body.append(toggle('Show Completed Events', state.showPrevious, on => {
      state.showPrevious = on;
      render();
    }));
    const sub = el('div', 'year-sub');
    sub.append(el('h2', '', year));
    body.append(sub);
    const wrap = el('div', 'year-list');
    wrap.append(list(rows.filter(r => r.act.year === year)));
    body.append(wrap);
  };
  render();
  setSearch('Search my sign ups', q => {
    query = q;
    render();
  });
  page.append(body);
  return page;
}
