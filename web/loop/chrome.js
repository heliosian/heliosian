import {state, me} from './state.js';
import {el, svg, link} from '/elements.js';
import {initShell, appSymbol} from '/shell.js';
import {fillNav as drawRail} from '/rail.js';

const items = [
  {href: '/', icon: 'app', label: 'My Email Lists'},
  {href: '/new', icon: 'plus', label: 'New Email List'},
];

function tabLink(item) {
  const a = link(item.href, location.pathname === item.href ? 'is-active' : '');
  a.append(item.icon === 'app' ? appSymbol() : svg(item.icon), el('span', '', item.label));
  return a;
}

function fillNav(nav) {
  drawRail(nav, {
    lists: state.model ? state.model.nav : {tags: [], lists: []},
    redraw: fillNav,
  });
}

function fillTabbar(bar) {
  for (const item of items) {
    bar.append(tabLink(item));
  }
}

export function initChrome() {
  initShell({
    name: 'Helios Loop',
    me,
    fillNav,
    fillTabbar,
    search: {placeholder: 'Search email lists…'},
  });
}
