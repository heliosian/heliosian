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
const inlinePattern = /(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\([^)\s]+\)|https?:\/\/[^\s<>)]+[^\s<>).,;:!?])/g;

function inline(node, text) {
  let last = 0;
  for (const m of text.matchAll(inlinePattern)) {
    if (m.index > last) {
      node.append(text.slice(last, m.index));
    }
    const token = m[0];
    if (token.startsWith('**')) {
      const strong = el('strong');
      inline(strong, token.slice(2, -2));
      node.append(strong);
    } else if (token.startsWith('`')) {
      node.append(el('code', '', token.slice(1, -1)));
    } else if (token.startsWith('[')) {
      const close = token.indexOf('](');
      node.append(anchor(token.slice(close + 2, -1), token.slice(1, close)));
    } else {
      node.append(anchor(token, token));
    }
    last = m.index + token.length;
  }
  if (last < text.length) {
    node.append(text.slice(last));
  }
}

function anchor(href, label) {
  const a = el('a', '', label);
  a.href = localizeLink(href);
  a.rel = 'noopener';
  return a;
}

// render draws markdown text into a fresh fragment: blocks split on blank
// lines, each a list, a heading, a quote, a fenced block, or a paragraph.
export function render(text) {
  const out = document.createDocumentFragment();
  const lines = text.replace(/\r/g, '').split('\n');
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
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
      inline(h, heading[2]);
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
        inline(item, body);
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
      inline(quote, body.join(' '));
      out.append(quote);
      continue;
    }
    const p = el('p');
    const body = [];
    while (i < lines.length && lines[i].trim() && !lines[i].startsWith('```') && !/^(#{1,6})\s/.test(lines[i]) && !/^\s*[-*]\s+/.test(lines[i]) && !/^\s*\d+[.)]\s+/.test(lines[i]) && !lines[i].startsWith('> ')) {
      body.push(lines[i]);
      i++;
    }
    body.forEach((l, index) => {
      if (index > 0) {
        p.append(el('br'));
      }
      inline(p, l);
    });
    out.append(p);
  }
  return out;
}
