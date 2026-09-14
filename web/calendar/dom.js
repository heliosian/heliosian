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
  down: 'M6 9l6 6 6-6',
  grip: 'M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01',
  star: 'M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20.2l1.1-6.2L3 9.6l6.2-.9z',
  lock: 'M6 11h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8a1 1 0 0 1 1-1zM8 11V7a4 4 0 0 1 8 0v4',
  back: 'M15 6l-6 6 6 6',
  plus: 'M12 5v14M5 12h14',
  check: 'M5 12l5 5L20 7',
  save: 'M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2zM17 21v-8H7v8M7 3v5h8',
  open: 'M15 3h6v6M10 14 21 3M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5',
  copy: 'M8 8h11a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2zM16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3',
  link: 'M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7',
  close: 'M6 6l12 12M18 6L6 18',
  trash: 'M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3',
  tag: 'M20 12l-8 8-9-9V3h8zM7 7h.01',
  pencil: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z',
  ticket: 'M3 9V6h18v3a2 2 0 0 0 0 6v3H3v-3a2 2 0 0 0 0-6zM13 6v12',
  copyplus: 'M9 9h10a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2V11a2 2 0 0 1 2-2zM15 5V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2M14 13v6M11 16h6',
  image: 'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM8.5 10a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3zM21 15l-5-5-8 8',
  car: 'M5 16v3M19 16v3M3 16h18v-4l-2-5H5l-2 5zM3 12h18M7.5 14.5h.01M16.5 14.5h.01',
  school: 'M3 21h18M5 21V10l7-5 7 5v11M10 21v-5h4v5M12 5V2M9 12h.01M15 12h.01',
  calcheck: 'M4 5h16a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 10h18M8 3v4M16 3v4M9 15.5l2 2 4-4',
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

// toast shows a line at the foot of the page for a moment - longer, given
// a duration, for a line worth reading twice.
export function toast(message, duration = 2600) {
  const node = document.querySelector('#toast');
  node.textContent = message;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    node.hidden = true;
  }, duration);
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

// peopleLine is the household's part in an event on one line behind one
// icon: the names in order with a dot between each, and a note - a role, a
// waitlist place, a guest still to be named - in parentheses after its name.
export function peopleLine(list, icon) {
  const line = el('div', 'side-people');
  line.append(svg(icon));
  const names = el('span', 'side-people-names');
  list.forEach((p, i) => {
    if (i) {
      names.append(el('span', 'side-people-sep', '\u2022'));
    }
    names.append(el('span', 'side-person', p.name));
    if (p.note) {
      names.append(el('span', 'side-person-note', `(${p.note})`));
    }
  });
  line.append(names);
  return line;
}

// popup is a layer over the page with a titled box: closing on its cross,
// Escape, or a click outside. It hands back the box and the closer.
export function popup(title, node, {wide = false} = {}) {
  const layer = el('div', 'modal-overlay');
  const box = el('div', 'modal' + (wide ? ' modal-wide' : ''));
  const header = el('div', 'modal-header');
  header.append(el('h2', '', title));
  const close = el('button', 'modal-close', '\u00d7');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  const shut = () => {
    layer.remove();
    document.removeEventListener('keydown', onKey, true);
  };
  const onKey = e => {
    if (e.key === 'Escape') {
      e.stopImmediatePropagation();
      shut();
    }
  };
  close.addEventListener('click', shut);
  layer.addEventListener('click', e => {
    if (e.target === layer) {
      shut();
    }
  });
  document.addEventListener('keydown', onKey, true);
  header.append(close);
  box.append(header, node);
  layer.append(box);
  document.body.append(layer);
  return {box, shut};
}

// feedMark is a saved calendar's mark: the emoji its owner gave it, else
// the calendar icon.
export function feedMark(f) {
  if (f.emoji) {
    return el('span', 'feed-mark', f.emoji);
  }
  return svg('calendar');
}

// feedEmoji are the usual marks for a saved calendar, offered beside the
// field that takes any.
const feedEmoji = ['\ud83d\udcc5', '\ud83c\udfeb', '\ud83c\udf92', '\ud83d\ude8c', '\u26bd', '\ud83c\udfad', '\ud83c\udf89', '\ud83c\udfd5\ufe0f', '\ud83d\udc68\u200d\ud83d\udc69\u200d\ud83d\udc67', '\ud83c\udf1f', '\u2764\ufe0f', '\ud83d\udcda'];

// emojiPicker is the field for a saved calendar's mark: a small input to
// type any emoji into, the usual ones to pick, None for the calendar icon,
// and under them the whole library - every emoji by group, searched by its
// Unicode name, loaded from /emoji.json the first time. Returns {node, input}.
let emojiLibrary = null;

export function emojiPicker(value) {
  const node = el('div', 'emoji-field');
  const row = el('div', 'emoji-row');
  const input = el('input');
  input.type = 'text';
  input.maxLength = 12;
  input.placeholder = '\ud83d\udcc5';
  input.className = 'emoji-input';
  input.value = value || '';
  input.setAttribute('aria-label', 'Emoji');
  row.append(input);
  const pick = e => {
    input.value = e;
    input.dispatchEvent(new Event('input', {bubbles: true}));
  };
  for (const e of feedEmoji) {
    const b = el('button', 'emoji-pick-item', e);
    b.type = 'button';
    b.title = 'Use ' + e;
    b.addEventListener('click', () => pick(e));
    row.append(b);
  }
  const none = el('button', 'emoji-pick-none', 'None');
  none.type = 'button';
  none.addEventListener('click', () => pick(''));
  row.append(none);
  node.append(row);
  const search = el('input', 'emoji-search');
  search.type = 'search';
  search.placeholder = 'Search all emoji\u2026';
  search.setAttribute('aria-label', 'Search emoji');
  const library = el('div', 'emoji-library');
  const paint = () => {
    const needle = search.value.trim().toLowerCase();
    library.replaceChildren();
    let shown = 0;
    for (const [group, entries] of emojiLibrary || []) {
      const matches = entries.filter(([, name]) => !needle || name.includes(needle));
      if (!matches.length) {
        continue;
      }
      library.append(el('div', 'emoji-group', group));
      const grid = el('div', 'emoji-grid');
      for (const [emoji, name] of matches) {
        const b = el('button', 'emoji-pick-item', emoji);
        b.type = 'button';
        b.title = name;
        b.setAttribute('aria-label', name);
        b.addEventListener('click', () => pick(emoji));
        grid.append(b);
      }
      library.append(grid);
      shown += matches.length;
    }
    if (!shown) {
      library.append(el('div', 'emoji-none', emojiLibrary ? 'Nothing by that name.' : 'Loading\u2026'));
    }
  };
  search.addEventListener('input', paint);
  node.append(search, library);
  paint();
  if (!emojiLibrary) {
    fetch('/emoji.json').then(res => res.ok ? res.json() : []).then(list => {
      emojiLibrary = list;
      paint();
    }).catch(() => {
      emojiLibrary = [];
      paint();
    });
  }
  return {node, input};
}
