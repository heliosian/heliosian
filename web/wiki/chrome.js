import {me, pageAt, pagePath, childrenOf, trail, mine, setOrder} from './state.js';
import {el, svg, link, toast} from '/elements.js';
import {initShell} from '/shell.js';
import {navigate, load} from '/router.js';
import {movedKey} from '/order.js';

const openKey = 'wiki.open';
let dragged = null;

function currentPage() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  return parts[0] === 'p' ? pageAt(parts[1] || '') : null;
}

function opened() {
  try {
    return new Set(JSON.parse(localStorage.getItem(openKey) || '[]'));
  } catch {
    return new Set();
  }
}

function keepOpened(set) {
  try {
    localStorage.setItem(openKey, JSON.stringify([...set]));
  } catch {
    return;
  }
}

function marks(p, row) {
  if (mine(p)) {
    const star = svg('star');
    star.classList.add('nav-mine');
    star.setAttribute('aria-label', 'You started this page');
    row.append(star);
  }
}

async function reorder(target, after) {
  const siblings = childrenOf(target.parent);
  const keys = siblings.map(p => p.order);
  const from = siblings.indexOf(dragged);
  let to = siblings.indexOf(target);
  if (from < 0 || keys.includes('')) {
    return;
  }
  if (to > from) {
    to--;
  }
  if (after) {
    to++;
  }
  if (to === from) {
    return;
  }
  try {
    await setOrder(dragged.id, movedKey(keys, from, to));
    await load();
  } catch (err) {
    toast(err.message);
  }
}

function draggable(row, p) {
  row.draggable = true;
  row.addEventListener('dragstart', e => {
    dragged = p;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', p.id);
  });
  row.addEventListener('dragend', () => {
    dragged = null;
  });
  row.addEventListener('dragover', e => {
    if (!dragged || dragged === p || dragged.parent !== p.parent) {
      return;
    }
    e.preventDefault();
    const box = row.getBoundingClientRect();
    const after = e.clientY > box.top + box.height / 2;
    row.classList.toggle('drop-before', !after);
    row.classList.toggle('drop-after', after);
  });
  row.addEventListener('dragleave', () => row.classList.remove('drop-before', 'drop-after'));
  row.addEventListener('drop', e => {
    e.preventDefault();
    const after = row.classList.contains('drop-after');
    row.classList.remove('drop-before', 'drop-after');
    reorder(p, after);
  });
}

function mainRow(href, icon, label, active) {
  const a = link(href, active ? 'is-active' : '');
  a.append(svg(icon), el('span', 'nav-main-name', label));
  return a;
}

function subRows(list, pages, depth, here, redraw) {
  for (const p of pages) {
    const kids = childrenOf(p.id);
    const onPath = Boolean(here && (here === p || trail(here).includes(p)));
    const open = kids.length > 0 && (onPath || opened().has(p.id));
    const row = el('div', 'nav-tree-row');
    row.style.paddingLeft = `${depth * 14}px`;
    const toggle = el('button', 'nav-tree-toggle' + (open ? ' is-open' : '') + (kids.length ? '' : ' is-leaf'));
    toggle.type = 'button';
    toggle.disabled = !kids.length || onPath;
    toggle.setAttribute('aria-label', open ? 'Collapse' : 'Expand');
    toggle.append(svg('chevron-right'));
    toggle.addEventListener('click', () => {
      const set = opened();
      if (set.has(p.id)) {
        set.delete(p.id);
      } else {
        set.add(p.id);
      }
      keepOpened(set);
      redraw();
    });
    const a = link(pagePath(p), 'nav-sub-item nav-tree-item' + (here === p ? ' is-on' : ''));
    a.append(el('span', 'nav-sub-name', p.name));
    marks(p, a);
    row.append(toggle, a);
    draggable(row, p);
    list.append(row);
    if (open) {
      subRows(list, kids, depth + 1, here, redraw);
    }
  }
}

function fillNav(nav) {
  const redraw = () => {
    nav.replaceChildren();
    fillNav(nav);
  };
  const here = currentPage();
  const root = here ? trail(here)[0] || here : null;
  if (root) {
    const head = mainRow(pagePath(root), childrenOf(root.id).length ? 'book' : 'doc', root.name, here === root);
    marks(root, head);
    nav.append(head);
    const kids = childrenOf(root.id);
    if (kids.length) {
      const list = el('div', 'nav-sub nav-tree');
      subRows(list, kids, 0, here, redraw);
      nav.append(list);
    }
  }
  nav.append(mainRow('/', 'list', 'All Pages', location.pathname === '/'));
  const make = link('/new', 'button button-small nav-action');
  make.append(svg('plus'), el('span', '', 'New Page'));
  nav.append(make);
}

function fillTabbar(bar) {
  bar.append(
    mainRow('/', 'list', 'All Pages', location.pathname === '/'),
    mainRow('/new', 'plus', 'New Page', location.pathname === '/new'),
  );
}

export function initChrome() {
  initShell({
    name: 'Helios Wiki',
    me,
    fillNav,
    fillTabbar,
    search: {placeholder: 'Search pages…', carry: () => navigate('/')},
  });
}
