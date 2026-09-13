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
  today: 'M4 5h16a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 10h18M8 3v4M16 3v4M12 13v4M10 15h4',
  upcoming: 'M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01',
  calendar: 'M4 5h16a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 10h18M8 3v4M16 3v4',
  feed: 'M4 11a9 9 0 0 1 9 9M4 4a16 16 0 0 1 16 16M5 19a1 1 0 1 0 0-2 1 1 0 0 0 0 2z',
  clock: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM12 7v5l3 2',
  pin: 'M12 22s7-7.6 7-12a7 7 0 1 0-14 0c0 4.4 7 12 7 12zM12 12.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z',
  people: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM23 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  menu: 'M4 7h16M4 12h16M4 17h16',
  chevron: 'M9 6l6 6-6 6',
  back: 'M15 6l-6 6 6 6',
  plus: 'M12 5v14M5 12h14',
  check: 'M5 12l5 5L20 7',
  open: 'M15 3h6v6M10 14 21 3M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5',
  copy: 'M8 8h11a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2zM16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3',
  close: 'M6 6l12 12M18 6L6 18',
  trash: 'M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3',
  tag: 'M20 12l-8 8-9-9V3h8zM7 7h.01',
  sun: 'M12 4v2M12 18v2M4 12h2M18 12h2M6.3 6.3l1.4 1.4M16.3 16.3l1.4 1.4M6.3 17.7l1.4-1.4M16.3 7.7l1.4-1.4M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
  info: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM12 11v5M12 8h.01',
  search: 'M11 4a7 7 0 1 1 0 14 7 7 0 0 1 0-14zM20 20l-4-4',
  share: 'M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM8.6 13.5l6.8 4M15.4 6.5l-6.8 4',
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
  if (label) {
    node.append(el('span', '', label));
  }
  if (onClick) {
    node.addEventListener('click', e => {
      e.preventDefault();
      e.stopPropagation();
      onClick(e);
    });
  }
  return node;
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
  node.textContent = (person.name || person.email || '?').slice(0, 1).toUpperCase();
  return node;
}

export function segmented(items, active, onPick) {
  const bar = el('div', 'segmented');
  for (const item of items) {
    const b = el('button', 'segment' + (item.key === active ? ' is-on' : ''), item.label);
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
  }, 2600);
}

export async function copyText(text, message) {
  await navigator.clipboard.writeText(text);
  toast(message || 'Copied');
}

const urlForm = /https?:\/\/[^\s<>"']+/g;

// paragraphs renders sheet text with its blank-line breaks kept and every
// web address made a link, since a Zoom link is the point of some.
export function paragraphs(text, className) {
  const wrap = el('div', className || 'prose');
  for (const chunk of (text || '').split(/\n\s*\n/)) {
    if (!chunk.trim()) {
      continue;
    }
    const p = el('p');
    chunk.split('\n').forEach((line, i) => {
      if (i > 0) {
        p.append(el('br'));
      }
      let last = 0;
      for (const m of line.matchAll(urlForm)) {
        p.append(line.slice(last, m.index));
        const a = el('a', 'prose-link', m[0]);
        a.href = m[0];
        a.target = '_blank';
        a.rel = 'noopener';
        p.append(a);
        last = m.index + m[0].length;
      }
      p.append(line.slice(last));
    });
    wrap.append(p);
  }
  return wrap;
}
