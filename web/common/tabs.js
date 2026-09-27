import {el, svg} from '/elements.js';

function closeMenus() {
  for (const menu of document.querySelectorAll('.tab-strip-menu')) {
    menu.hidden = true;
  }
}

let listening = false;

function listen() {
  if (listening) {
    return;
  }
  listening = true;
  document.addEventListener('click', closeMenus);
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeMenus();
    }
  });
}

function fill(target, item) {
  if (item.icon) {
    target.append(item.icon.cloneNode(true));
  }
  target.append(el('span', '', item.label));
  if (item.count !== undefined) {
    target.append(el('span', 'tab-strip-count', String(item.count)));
  }
}

export function tabParam(fallback) {
  return new URLSearchParams(location.search).get('tab') || fallback;
}

export function tabHref(key) {
  const params = new URLSearchParams(location.search);
  params.set('tab', key);
  return location.pathname + '?' + params;
}

export function tabStrip(items, activeKey, mobileVisible, onSelect) {
  const strip = el('div', 'tab-strip');
  const mobile = matchMedia('(max-width: 900px)').matches;
  const visible = mobile ? items.slice(0, mobileVisible) : items;
  const hidden = mobile ? items.slice(mobileVisible) : [];
  for (const item of visible) {
    const tab = el('div', 'tab-strip-item' + (item.key === activeKey ? ' active' : ''));
    fill(tab, item);
    tab.addEventListener('click', () => onSelect(item.key));
    strip.append(tab);
  }
  if (hidden.length) {
    listen();
    const wrap = el('div', 'tab-strip-more');
    const more = el('div', 'tab-strip-item' + (hidden.some(t => t.key === activeKey) ? ' active' : ''));
    more.append(svg('more'), el('span', '', 'More'), svg('chevron-down'));
    const menu = el('div', 'tab-strip-menu');
    menu.hidden = true;
    for (const item of hidden) {
      const entry = el('div', 'tab-strip-menu-item' + (item.key === activeKey ? ' active' : ''));
      fill(entry, item);
      entry.addEventListener('click', () => onSelect(item.key));
      menu.append(entry);
    }
    more.addEventListener('click', e => {
      e.stopPropagation();
      const opening = menu.hidden;
      closeMenus();
      menu.hidden = !opening;
    });
    wrap.append(more, menu);
    strip.append(wrap);
  }
  return strip;
}

export function tabbedFields(panels) {
  const wrap = el('div', 'form-tabs');
  const items = panels.map((panel, i) => ({key: String(i), label: panel.label, icon: panel.icon || null}));
  const bodies = [];
  let active = 0;
  let bar = null;
  const show = i => {
    active = i;
    const next = tabStrip(items, String(i), items.length, key => show(Number(key)));
    if (bar) {
      bar.replaceWith(next);
    }
    bar = next;
    bodies.forEach((body, j) => {
      body.hidden = j !== i;
    });
  };
  show(0);
  panels.forEach((panel, i) => {
    const body = el('div', 'form-tab-body');
    body.hidden = i !== 0;
    body.append(...panel.fields);
    body.addEventListener('invalid', () => {
      if (active !== i) {
        show(i);
      }
    }, true);
    bodies.push(body);
  });
  wrap.append(bar, ...bodies);
  return wrap;
}
