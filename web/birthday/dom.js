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
  users: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM22 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  chat: 'M21 12a8 8 0 0 1-8 8H8l-5 3 1.5-4.5A8 8 0 1 1 21 12zM8 12h.01M12 12h.01M16 12h.01',
  sort: 'M7 4v16M7 20l-3-3M7 20l3-3M17 20V4M17 4l-3 3M17 4l3 3',
  sparkle: 'M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8zM19 16l.8 2.2L22 19l-2.2.8L19 22l-.8-2.2L16 19l2.2-.8zM5 15l.6 1.4L7 17l-1.4.6L5 19l-.6-1.4L3 17l1.4-.6z',
  cake: 'M4 20h16M5 20v-6a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v6M3 16h18M8 12V8M12 12V7M16 12V8M8 8a1 1 0 1 1 0-2 1 1 0 0 1 0 2M12 7a1 1 0 1 1 0-2 1 1 0 0 1 0 2M16 8a1 1 0 1 1 0-2 1 1 0 0 1 0 2',
  doc: 'M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9zM14 3v6h6M8 13h8M8 17h8',
  send: 'M22 2L11 13M22 2l-7 20-4-9-9-4z',
  circlecheck: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM8.5 12l2.5 2.5 4.5-5',
  vault: 'M12 3c-4.4 0-8 1.3-8 3v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6c0-1.7-3.6-3-8-3zM4 6c0 1.7 3.6 3 8 3s8-1.3 8-3M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
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

// pageHead is the headline over the swoosh, as every app draws it, with
// whatever actions belong beside it on the right.
export function pageHead(title, actions) {
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', title));
  head.append(main);
  if (actions && actions.length) {
    const wrap = el('div', 'page-actions');
    wrap.append(...actions);
    head.append(wrap);
  }
  return head;
}

// tabs draws the list's tabs, each with its count when the item carries one.
export function tabs(items, active, onPick) {
  const bar = el('div', 'tabs');
  for (const item of items) {
    const b = el('button', item.key === active ? 'is-active' : '');
    b.type = 'button';
    b.append(el('span', '', item.label));
    if (item.count !== undefined) {
      b.append(el('span', 'tab-count', String(item.count)));
    }
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

// copyRich puts both a formatted and a plain version on the clipboard, so a
// paste into mail or a document keeps the bold, the picture and the links,
// and a paste into a plain box gets the words.
export async function copyRich(text, html, message) {
  try {
    await navigator.clipboard.write([new ClipboardItem({
      'text/plain': new Blob([text], {type: 'text/plain'}),
      'text/html': new Blob([html], {type: 'text/html'}),
    })]);
  } catch {
    await navigator.clipboard.writeText(text);
  }
  toast(message || 'Copied');
}
