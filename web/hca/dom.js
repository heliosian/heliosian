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
  signup: 'M12 3v18M4.5 7.5l15 9M19.5 7.5l-15 9',
  star: 'M12 3l2.9 6 6.6.9-4.8 4.6 1.2 6.5L12 17.8 6.1 21l1.2-6.5L2.5 9.9l6.6-.9z',
  calendar: 'M4 5h16a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 10h18M8 3v4M16 3v4',
  menu: 'M4 7h16M4 12h16M4 17h16',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
  chevron: 'M9 6l6 6-6 6',
  caret: 'M6 9l6 6 6-6',
  up: 'M12 19V5M5 12l7-7 7 7',
  mail: 'M3 6h18v12H3zM3 7l9 6 9-6',
  image: 'M3 5h18v14H3zM3 16l5-5 4 4 3-3 6 6',
  share: 'M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM8.6 13.5l6.8 4M15.4 6.5l-6.8 4',
  down: 'M12 5v14M5 12l7 7 7-7',
  back: 'M15 6l-6 6 6 6',
  plus: 'M12 5v14M5 12h14',
  join: 'M20 12a8 8 0 1 1-4-6.9M9 12l2.5 2.5L20 6',
  check: 'M5 12l5 5L20 7',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z',
  open: 'M15 3h6v6M10 14 21 3M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5',
  copy: 'M8 8h11a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2zM16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3',
  idea: 'M9 18h6M10 21h4M12 3a6 6 0 0 0-4 10.5c.7.6 1 1.3 1 2.5h6c0-1.2.3-1.9 1-2.5A6 6 0 0 0 12 3z',
  receipt: 'M5 3h14v18l-3-2-2 2-2-2-2 2-2-2-3 2zM8 8h8M8 12h8M8 16h5',
  search: 'M11 4a7 7 0 1 1 0 14 7 7 0 0 1 0-14zM20 20l-4-4',
  list: 'M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01',
  people: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM23 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  tools: 'M14.7 6.3a4 4 0 0 0 5 5L13 18l-1 4-4-1 6.7-6.7a4 4 0 0 0-5-5L3 3l4 1 1 4z',
  close: 'M6 6l12 12M18 6L6 18',
  next: 'M9 6l6 6-6 6',
  prev: 'M15 6l-6 6 6 6',
  trash: 'M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3',
  download: 'M12 3v12M6 11l6 6 6-6M4 21h16',
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

export function thumb(url, title, className) {
  if (url) {
    const img = el('img', 'thumb ' + (className || ''));
    img.src = url;
    img.alt = '';
    img.loading = 'lazy';
    return img;
  }
  return el('div', 'thumb initial ' + (className || ''), (title || '?').slice(0, 1).toUpperCase());
}

export function avatar(person, className) {
  const node = el('div', 'avatar ' + (className || ''));
  if (person.photoUrl) {
    const img = el('img');
    img.src = person.photoUrl;
    img.alt = '';
    img.loading = 'lazy';
    node.append(img);
    return node;
  }
  node.textContent = (person.name || person.email).slice(0, 1).toUpperCase();
  return node;
}

export function badge(text, kind) {
  return el('span', 'badge ' + (kind || ''), text);
}

export function displayURL(url) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '');
}

export function searchBox(placeholder, onInput, light) {
  const box = el('div', 'search' + (light ? ' light' : ''));
  const input = el('input');
  input.type = 'search';
  input.placeholder = placeholder || 'Search';
  input.addEventListener('input', () => onInput(input.value.trim().toLowerCase()));
  box.append(svg('search'), input);
  return box;
}

export function selectPill(icon, options, value, onPick) {
  const wrap = el('label', 'select-pill');
  wrap.append(svg(icon));
  const select = el('select');
  for (const o of options) {
    const opt = el('option', '', o.label);
    opt.value = o.key;
    opt.selected = o.key === value;
    select.append(opt);
  }
  select.addEventListener('change', () => onPick(select.value));
  wrap.append(select, svg('caret'));
  return wrap;
}

export function toggle(label, checked, onChange) {
  const row = el('div', 'toggle-row');
  row.append(el('span', '', label));
  const wrap = el('label', 'switch');
  const input = el('input');
  input.type = 'checkbox';
  input.checked = checked;
  input.addEventListener('change', () => onChange(input.checked));
  wrap.append(input, el('span'));
  row.append(wrap);
  return row;
}

export function tabs(items, active, onPick, light) {
  const bar = el('div', 'tabs' + (light ? ' light' : ''));
  for (const item of items) {
    const b = el('button', item.key === active ? 'is-active' : '', item.label);
    b.type = 'button';
    b.addEventListener('click', () => onPick(item.key));
    bar.append(b);
  }
  return bar;
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
