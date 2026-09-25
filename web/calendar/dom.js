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
  filter: 'M3 5h18l-7 8v5l-4 2v-7z',
  eye: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  'eye-off': 'M9.9 4.24A9 9 0 0 1 12 4c6.5 0 10 8 10 8a17 17 0 0 1-2.16 3.19M6.6 6.6A17 17 0 0 0 2 12s3.5 8 10 8a9.7 9.7 0 0 0 5.4-1.6M14.1 14.1a3 3 0 1 1-4.2-4.2M2 2l20 20',
  mail: 'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 7l9 6 9-6',
  help: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.7.4-1 .9-1 1.7M12 17h.01',
  ban: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM5.6 5.6l12.8 12.8',
  reply: 'M9 14 4 9l5-5M4 9h9a7 7 0 0 1 7 7v4',
  chevron: 'M9 6l6 6-6 6',
  down: 'M6 9l6 6 6-6',
  grip: 'M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01',
  star: 'M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20.2l1.1-6.2L3 9.6l6.2-.9z',
  // The pushpin on the default calendar.
  chat: 'M21 12a8 8 0 0 1-11.6 7.2L4 21l1.8-4.4A8 8 0 1 1 21 12z',
  pushpin: 'M9 3h6l-1 6 3 3v2H7v-2l3-3-1-6zM12 14v7',
  // The rule editor's marks (rules.js), as Loop draws them.
  groups: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM22 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  'user-minus': 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM23 11h-6',
  families: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM23 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  party: 'M5.8 11.3 2 22l10.7-3.8M4 3h.01M22 8h.01M15 2h.01M22 20h.01M22 2l-2.2 2.2M15.7 3.7 14 5.4M18 8.3l-1.8 1.8M20 13.5l-1.5 1.5M9 5.5 3.5 11',
  activity: 'M18 8h1a4 4 0 0 1 0 8h-1M2 8h16v9a4 4 0 0 1-4 4H6a4 4 0 0 1-4-4V8zM6 1v3M10 1v3M14 1v3',
  classrooms: 'M3 21h18M5 21V7l8-4v18M19 21V11l-6-4M9 9h.01M9 13h.01M9 17h.01',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z',
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
  gift: 'M20 12v9H4v-9M2 7h20v5H2zM12 22V7M12 7H7.5a2.5 2.5 0 1 1 0-5C11 2 12 7 12 7zM12 7h4.5a2.5 2.5 0 1 0 0-5C13 2 12 7 12 7z',
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
// With lines, every line is a paragraph of its own - the way the school's
// calendar writes an event's description, a paragraph to a line with no
// blank line between - save a list item (a line starting -, •, * or 1.),
// which stays tight under the line before it.
const listItem = /^\s*([-•*]|\d+[.)])\s/;

export function paragraphs(text, className, {lines = false} = {}) {
  const wrap = el('div', className || 'prose');
  const chunks = [];
  for (const chunk of (text || '').split(/\n\s*\n/)) {
    if (!lines) {
      chunks.push(chunk);
      continue;
    }
    for (const line of chunk.split('\n')) {
      if (listItem.test(line) && chunks.length) {
        chunks[chunks.length - 1] += '\n' + line;
      } else {
        chunks.push(line);
      }
    }
  }
  for (const chunk of chunks) {
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

// peopleList is the household's part in an event as a list, one block per
// person behind the icon - the name, and under it each of their roles, or
// the ticket's note - for where there is room, as the event page's side
// has. Someone with two roles is listed once, both roles under them.
export function peopleList(list, icon) {
  const byName = new Map();
  for (const p of list) {
    if (!byName.has(p.name)) {
      byName.set(p.name, []);
    }
    if (p.note) {
      byName.get(p.name).push(p.note);
    }
  }
  const rows = el('ul', 'side-people-list');
  for (const [name, notes] of byName) {
    const row = el('li');
    const words = el('span', 'side-person-words');
    words.append(el('span', 'side-person', name));
    if (notes.length) {
      const roles = el('ul', 'side-person-roles');
      for (const note of notes) {
        roles.append(el('li', '', note));
      }
      words.append(roles);
    }
    row.append(svg(icon), words);
    rows.append(row);
  }
  return rows;
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
// the calendar icon - or, where the calendar is the page's headline, When's
// own symbol, the designer's white outline drawn through a mask so it takes
// the colour of the words beside it. The rail's rows under Calendar and the
// switch menu keep the plain icon, so only the top wears the symbol.
export function feedMark(f, symbol) {
  if (f.emoji) {
    return el('span', 'feed-mark', f.emoji);
  }
  if (!symbol) {
    const icon = svg('calendar');
    icon.classList.add('feed-mark', 'feed-mark-icon');
    return icon;
  }
  const mark = el('span', 'feed-mark feed-mark-symbol');
  mark.setAttribute('aria-hidden', 'true');
  return mark;
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
