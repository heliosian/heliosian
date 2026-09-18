import {appOrigin} from '/toolbar.js';

// The answers arrive as light markdown - paragraphs, lists, bold, links -
// and are drawn as DOM, never as HTML strings. Links to the apps are
// written by their production names and land on the page's own tier.

const appHosts = /^https?:\/\/(who|team|hca|celebrate|birthday|calendar|cal|when|loop|ask|home|www)\.heliosian\.com(\/.*)?$/;
const apex = /^https?:\/\/heliosian\.com(\/.*)?$/;

export function localizeLink(href) {
  let m = href.match(appHosts);
  if (m) {
    const label = {hca: 'team', cal: 'calendar', when: 'calendar', www: 'home'}[m[1]] || m[1];
    const path = m[2] || '/';
    return appOrigin(label === 'calendar' ? 'when' : label) + path;
  }
  m = href.match(apex);
  if (m) {
    return appOrigin('home') + (m[1] || '/');
  }
  return href;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

// stable is the part of a streaming answer safe to draw: a link still being
// written shows as its words alone until it closes, and drawing stops short
// of a bold run, code span or bare address still being written.
export function stable(text) {
  text = text.replace(/\[([^[\]\n]*)(\](\([^)\s]*)?)?$/, '$1');
  let cut = text.length;
  const bolds = text.split('**').length - 1;
  if (bolds % 2 === 1) {
    cut = Math.min(cut, text.lastIndexOf('**'));
  }
  const singles = [...text.replace(/^\s*\*\s/gm, '  ').replace(/\*\*/g, '  ').matchAll(/\*/g)].map(m => m.index);
  if (singles.length % 2 === 1) {
    cut = Math.min(cut, singles[singles.length - 1]);
  }
  const ticks = text.split('`').length - 1;
  if (ticks % 2 === 1) {
    cut = Math.min(cut, text.lastIndexOf('`'));
  }
  const tail = text.slice(text.search(/\S+$/) >= 0 ? text.search(/\S+$/) : text.length);
  if (/^https?:\/\//.test(tail)) {
    cut = Math.min(cut, text.length - tail.length);
  }
  return text.slice(0, cut);
}

// inline fills a node with a line's text: bold (with a link inside it, when
// there is one), code, links in brackets, and bare addresses.
const inlinePattern = /(\*\*[^*]+\*\*|\*[^*\s](?:[^*]*[^*\s])?\*|`[^`]+`|\[[^\]]+\]\([^)\s]+\)|https?:\/\/[^\s<>)]+[^\s<>).,;:!?])/g;

function inline(node, text, cards) {
  let last = 0;
  for (const m of text.matchAll(inlinePattern)) {
    if (m.index > last) {
      node.append(text.slice(last, m.index));
    }
    const token = m[0];
    if (token.startsWith('**')) {
      const strong = el('strong');
      inline(strong, token.slice(2, -2), cards);
      node.append(strong);
    } else if (token.startsWith('*')) {
      const em = el('em');
      inline(em, token.slice(1, -1), cards);
      node.append(em);
    } else if (token.startsWith('`')) {
      node.append(el('code', '', token.slice(1, -1)));
    } else if (token.startsWith('[')) {
      const close = token.indexOf('](');
      node.append(anchor(token.slice(close + 2, -1), token.slice(1, close), cards));
    } else {
      node.append(anchor(token, token, cards));
    }
    last = m.index + token.length;
  }
  if (last < text.length) {
    node.append(text.slice(last));
  }
}

function anchor(href, label, cards) {
  const a = el('a');
  a.href = localizeLink(href);
  a.rel = 'noopener';
  const card = cards[href];
  if (!card) {
    a.textContent = label;
    return a;
  }
  a.className = 'chip-link chip-' + card.kind;
  if (card.badge) {
    a.append(el('span', 'chip-badge', card.badge));
  } else if (card.kind === 'group') {
    a.append(groupIcon());
  } else if (card.image) {
    const img = el('img', 'chip-image');
    img.src = localizeLink(card.image);
    img.alt = '';
    a.append(img);
  } else {
    a.append(el('span', 'chip-image chip-initial', (label.trim()[0] || '•').toUpperCase()));
  }
  a.append(el('span', 'chip-label', label));
  return a;
}

function groupIcon() {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('class', 'chip-image chip-icon');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '2');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  const path = document.createElementNS(ns, 'path');
  path.setAttribute('d', 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75');
  svg.append(path);
  return svg;
}

const tableRule = /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$/;

function tableStart(lines, i) {
  return lines[i].trim().startsWith('|') && i + 1 < lines.length && tableRule.test(lines[i + 1]);
}

function cells(line) {
  return line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map(c => c.trim());
}

function table(lines, i, cards) {
  const wrap = el('div', 'table-wrap');
  const t = el('table');
  const head = el('tr');
  for (const c of cells(lines[i])) {
    const th = el('th');
    inline(th, c, cards);
    head.append(th);
  }
  const thead = el('thead');
  thead.append(head);
  const tbody = el('tbody');
  i += 2;
  while (i < lines.length && lines[i].trim().startsWith('|')) {
    const row = el('tr');
    for (const c of cells(lines[i])) {
      const td = el('td');
      inline(td, c, cards);
      row.append(td);
    }
    tbody.append(row);
    i++;
  }
  t.append(thead, tbody);
  wrap.append(t);
  return [wrap, i];
}

// render draws markdown text into a fresh fragment: blocks split on blank
// lines, each a table, a list, a heading, a quote, a fenced block, or a
// paragraph; a link with a card is drawn as a chip.
export function render(text, cards = {}) {
  const out = document.createDocumentFragment();
  const lines = text.replace(/\r/g, '').split('\n');
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }
    if (tableStart(lines, i)) {
      const [node, next] = table(lines, i, cards);
      out.append(node);
      i = next;
      continue;
    }
    if (line.startsWith('```')) {
      const code = [];
      i++;
      while (i < lines.length && !lines[i].startsWith('```')) {
        code.push(lines[i++]);
      }
      i++;
      out.append(el('pre', '', code.join('\n')));
      continue;
    }
    const heading = line.match(/^(#{1,6})\s+(.*)$/);
    if (heading) {
      const h = el('h3');
      inline(h, heading[2], cards);
      out.append(h);
      i++;
      continue;
    }
    const bullet = line.match(/^\s*[-*]\s+/);
    const numbered = line.match(/^\s*\d+[.)]\s+/);
    if (bullet || numbered) {
      const list = el(bullet ? 'ul' : 'ol');
      const pattern = bullet ? /^\s*[-*]\s+/ : /^\s*\d+[.)]\s+/;
      while (i < lines.length && pattern.test(lines[i])) {
        const item = el('li');
        let body = lines[i].replace(pattern, '');
        i++;
        while (i < lines.length && lines[i].trim() && !pattern.test(lines[i]) && /^\s+/.test(lines[i])) {
          body += ' ' + lines[i].trim();
          i++;
        }
        inline(item, body, cards);
        list.append(item);
      }
      out.append(list);
      continue;
    }
    if (line.startsWith('> ')) {
      const quote = el('blockquote');
      const body = [];
      while (i < lines.length && lines[i].startsWith('> ')) {
        body.push(lines[i].slice(2));
        i++;
      }
      inline(quote, body.join(' '), cards);
      out.append(quote);
      continue;
    }
    const p = el('p');
    const body = [];
    while (i < lines.length && lines[i].trim() && !lines[i].startsWith('```') && !/^(#{1,6})\s/.test(lines[i]) && !/^\s*[-*]\s+/.test(lines[i]) && !/^\s*\d+[.)]\s+/.test(lines[i]) && !lines[i].startsWith('> ') && !tableStart(lines, i)) {
      body.push(lines[i]);
      i++;
    }
    body.forEach((l, index) => {
      if (index > 0) {
        p.append(el('br'));
      }
      inline(p, l, cards);
    });
    out.append(p);
  }
  return out;
}
