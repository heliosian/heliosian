export function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

export function link(href, className, text) {
  const node = el('a', className, text);
  node.href = href;
  node.setAttribute('data-link', '');
  return node;
}

const paths = {
  jobs: 'M9 6h12M9 12h12M9 18h12M3 6l1.5 1.5L7 5M3 12l1.5 1.5L7 11M3 18l1.5 1.5L7 17',
  process: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 8v4l3 2',
  calendar: 'M4 5h16a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 10h18M8 3v4M16 3v4',
  gift: 'M20 12v9H4v-9M2 7h20v5H2zM12 7v14M12 7s-3-5-5-4-1 4 5 4M12 7s3-5 5-4 1 4-5 4',
  newsletter: 'M4 4h16v16H4zM8 8h8M8 12h8M8 16h5',
  skipped: 'M12 3l9.5 17h-19zM12 10v4M12 17h.01',
  menu: 'M4 7h16M4 12h16M4 17h16',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
  chevron: 'M9 6l6 6-6 6',
  back: 'M15 6l-6 6 6 6',
  plus: 'M12 5v14M5 12h14',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z',
  open: 'M15 3h6v6M10 14 21 3M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5',
  copy: 'M8 8h11a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2zM16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3',
  search: 'M11 4a7 7 0 1 1 0 14 7 7 0 0 1 0-14zM20 20l-4-4',
  tools: 'M14.7 6.3a4 4 0 0 0 5 5L13 18l-1 4-4-1 6.7-6.7a4 4 0 0 0-5-5L3 3l4 1 1 4z',
  close: 'M6 6l12 12M18 6L6 18',
  next: 'M9 6l6 6-6 6',
  prev: 'M15 6l-6 6 6 6',
  check: 'M20 6L9 17l-5-5',
  bolt: 'M13 2L4 14h7l-1 8 9-12h-7z',
  mail: 'M3 6h18v12H3zM3 6l9 7 9-7',
  reuse: 'M4 12a8 8 0 0 1 14-5l2 2M20 4v5h-5M20 12a8 8 0 0 1-14 5l-2-2M4 20v-5h5',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0',
  trash: 'M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3',
};

export function svg(name) {
  const node = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  node.setAttribute('viewBox', '0 0 24 24');
  node.setAttribute('fill', 'none');
  node.setAttribute('stroke', 'currentColor');
  node.setAttribute('stroke-width', '2');
  node.setAttribute('stroke-linecap', 'round');
  node.setAttribute('stroke-linejoin', 'round');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', paths[name]);
  node.append(path);
  return node;
}

export function button(label, icon, className, onClick) {
  const node = el('button', className || 'button');
  node.type = 'button';
  if (icon) {
    node.append(svg(icon));
  }
  node.append(el('span', '', label));
  if (onClick) {
    node.addEventListener('click', e => {
      e.preventDefault();
      e.stopPropagation();
      onClick(e);
    });
  }
  return node;
}

export function iconButton(icon, label, className, onClick) {
  const node = el('button', 'icon-button ' + (className || ''));
  node.type = 'button';
  node.setAttribute('aria-label', label);
  node.title = label;
  node.append(svg(icon));
  node.addEventListener('click', e => {
    e.preventDefault();
    e.stopPropagation();
    onClick(e);
  });
  return node;
}

export function thumb(person, className) {
  if (person.photoUrl) {
    const img = el('img', 'thumb ' + (className || ''));
    img.src = person.photoUrl;
    img.alt = '';
    img.loading = 'lazy';
    return img;
  }
  return el('div', 'thumb initial ' + (className || ''), (person.name || '?').slice(0, 1).toUpperCase());
}

export function searchBox(placeholder, onInput) {
  const box = el('div', 'search');
  const input = el('input');
  input.type = 'search';
  input.placeholder = placeholder || 'Search';
  input.addEventListener('input', () => onInput(input.value.trim().toLowerCase()));
  box.append(svg('search'), input);
  return box;
}

export function tabs(items, active, onPick) {
  const bar = el('div', 'tabs');
  for (const item of items) {
    const b = el('button', item.key === active ? 'is-active' : '', item.label);
    b.type = 'button';
    b.addEventListener('click', () => onPick(item.key));
    bar.append(b);
  }
  return bar;
}

export function menu(items) {
  const wrap = el('div', 'more-wrap');
  const trigger = iconButton('more', 'More', '', () => {
    const opening = list.hidden;
    for (const open of document.querySelectorAll('.row-menu')) {
      open.hidden = true;
    }
    list.hidden = !opening;
  });
  const list = el('div', 'row-menu');
  list.hidden = true;
  for (const item of items) {
    const b = el('button', item.danger ? 'danger' : '');
    b.type = 'button';
    if (item.icon) {
      b.append(svg(item.icon));
    }
    b.append(el('span', '', item.label));
    b.addEventListener('click', e => {
      e.preventDefault();
      e.stopPropagation();
      list.hidden = true;
      item.onClick();
    });
    list.append(b);
  }
  wrap.append(trigger, list);
  return wrap;
}

let toastTimer;

export function toast(message) {
  const node = document.querySelector('#toast');
  node.textContent = message;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    node.hidden = true;
  }, 2200);
}

export async function copyText(text, message) {
  await navigator.clipboard.writeText(text);
  toast(message || 'Copied');
}
