const moreIcon = '<svg viewBox="0 0 24 24"><path d="M4 7h16M4 12h16M4 17h10"/></svg>';
const chevronIcon = '<svg viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></svg>';

function node(tag, className, text) {
  const n = document.createElement(tag);
  n.className = className;
  if (text !== undefined) {
    n.textContent = text;
  }
  return n;
}

function icon(markup) {
  const holder = document.createElement('template');
  holder.innerHTML = markup;
  return holder.content.firstChild;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.tab-strip-menu')) {
    menu.hidden = true;
  }
}

document.addEventListener('click', closeMenus);
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    closeMenus();
  }
});

function fill(target, item) {
  if (item.icon) {
    target.append(item.icon.cloneNode(true));
  }
  target.append(node('span', '', item.label));
  if (item.count !== undefined) {
    target.append(node('span', 'tab-strip-count', String(item.count)));
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
  const strip = node('div', 'tab-strip');
  const mobile = matchMedia('(max-width: 900px)').matches;
  const visible = mobile ? items.slice(0, mobileVisible) : items;
  const hidden = mobile ? items.slice(mobileVisible) : [];
  for (const item of visible) {
    const tab = node('div', 'tab-strip-item' + (item.key === activeKey ? ' active' : ''));
    fill(tab, item);
    tab.addEventListener('click', () => onSelect(item.key));
    strip.append(tab);
  }
  if (hidden.length) {
    const wrap = node('div', 'tab-strip-more');
    const more = node('div', 'tab-strip-item' + (hidden.some(t => t.key === activeKey) ? ' active' : ''));
    more.append(icon(moreIcon), node('span', '', 'More'), icon(chevronIcon));
    const menu = node('div', 'tab-strip-menu');
    menu.hidden = true;
    for (const item of hidden) {
      const entry = node('div', 'tab-strip-menu-item' + (item.key === activeKey ? ' active' : ''));
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
