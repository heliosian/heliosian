import {state, years, myRows, sortByStart, isPrevious, matches, rolePath} from '../state.js';
import {el, searchBox, toggle, tabs} from '../dom.js';
import {setTitle} from '../chrome.js';
import {activityRow} from '../cards.js';

let year = null;
let query = '';

function list(rows) {
  const root = el('div');
  const shown = rows.filter(r => (state.showPrevious || !isPrevious(r.role || r.act)) && (matches(r.act, query) || (r.role && matches(r.role, query))));
  let any = false;
  for (const category of state.model.categories) {
    const items = shown.filter(r => r.act.category === category.title);
    if (!items.length) {
      continue;
    }
    any = true;
    root.append(el('div', 'section-title', category.title));
    const panel = el('div', 'panel');
    const ordered = sortByStart(items.map(r => ({...(r.role || r.act), _row: r}))).map(n => n._row);
    for (const r of ordered) {
      panel.append(activityRow(r.act, {role: r.role, href: r.role ? rolePath(r.act, r.role) : undefined, noJoin: true}));
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
    body.append(toggle('Show Previous Things', state.showPrevious, on => {
      state.showPrevious = on;
      render();
    }));
    const sub = el('div', 'year-sub');
    sub.append(el('h2', '', year));
    sub.append(searchBox('Search', q => {
      query = q;
      body.querySelector('.year-list').replaceChildren(list(rows.filter(r => r.act.year === year)));
    }, true));
    body.append(sub);
    const wrap = el('div', 'year-list');
    wrap.append(list(rows.filter(r => r.act.year === year)));
    body.append(wrap);
  };
  render();
  page.append(body);
  return page;
}
