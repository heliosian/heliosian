import {el, svg} from '/elements.js';

const urlForm = /https?:\/\/[^\s<>"']+/g;

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

const feedEmoji = ['\ud83d\udcc5', '\ud83c\udfeb', '\ud83c\udf92', '\ud83d\ude8c', '\u26bd', '\ud83c\udfad', '\ud83c\udf89', '\ud83c\udfd5\ufe0f', '\ud83d\udc68\u200d\ud83d\udc69\u200d\ud83d\udc67', '\ud83c\udf1f', '\u2764\ufe0f', '\ud83d\udcda'];

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
  let failed = false;
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
      library.append(el('div', 'emoji-none', failed ? 'Couldn\u2019t load emoji.' : emojiLibrary ? 'Nothing by that name.' : 'Loading\u2026'));
    }
  };
  search.addEventListener('input', paint);
  node.append(search, library);
  paint();
  if (!emojiLibrary) {
    fetch('/emoji.json').then(res => {
      if (!res.ok) {
        throw new Error('emoji.json: ' + res.status);
      }
      return res.json();
    }).then(list => {
      emojiLibrary = list;
      paint();
    }).catch(err => {
      failed = true;
      paint();
      throw err;
    });
  }
  return {node, input};
}
