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
  groups: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM22 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  plus: 'M12 5v14M5 12h14',
  menu: 'M4 7h16M4 12h16M4 17h16',
  close: 'M6 6l12 12M18 6L6 18',
  chevron: 'M6 9l6 6 6-6',
  back: 'M15 6l-6 6 6 6',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z',
  trash: 'M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3',
  copy: 'M8 8h11a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2zM16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3',
  mail: 'M3 6h18v12H3zM3 6l9 7 9-7',
  check: 'M20 6L9 17l-5-5',
  tag: 'M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L2 12V2h10l8.6 8.6a2 2 0 0 1 0 2.8zM7 7h.01',
  party: 'M5.8 11.3 2 22l10.7-3.8M4 3h.01M22 8h.01M15 2h.01M22 20h.01M22 2l-2.2 2.2M15.7 3.7 14 5.4M18 8.3l-1.8 1.8M20 13.5l-1.5 1.5M9 5.5 3.5 11',
  activity: 'M18 8h1a4 4 0 0 1 0 8h-1M2 8h16v9a4 4 0 0 1-4 4H6a4 4 0 0 1-4-4V8zM6 1v3M10 1v3M14 1v3',
  classrooms: 'M3 21h18M5 21V7l8-4v18M19 21V11l-6-4M9 9h.01M9 13h.01M9 17h.01',
  families: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM23 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  filter: 'M22 3H2l8 9.5V19l4 2v-8.5z',
  warn: 'M12 3l9.5 17h-19zM12 10v4M12 17h.01',
  sync: 'M4 12a8 8 0 0 1 14-5l2 2M20 4v5h-5M20 12a8 8 0 0 1-14 5l-2-2M4 20v-5h5',
  tools: 'M14.7 6.3a4 4 0 0 0 5 5L13 18l-1 4-4-1 6.7-6.7a4 4 0 0 0-5-5L3 3l4 1 1 4z',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0',
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

// personRow is one person as the member and manager lists show them: face,
// name, the word that places them, and a note under those when there is
// one - on a member, why they are on the group.
export function personRow(person, extra, note) {
  const row = el('div', 'person-row');
  row.append(thumb(person, 'small'));
  const body = el('div', 'person-body');
  body.append(el('div', 'person-name', person.name));
  const words = [person.words, person.email].filter(Boolean).join(' · ');
  body.append(el('div', 'person-words', words));
  if (note) {
    body.append(el('div', 'person-note', note));
  }
  row.append(body);
  if (extra) {
    row.append(extra);
  }
  return row;
}
