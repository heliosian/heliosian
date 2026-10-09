import {el, svg, iconButton} from '/elements.js';

export function segments() {
  return location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
}

export function infoBanner(kind, iconName, title, desc, buttonLabel, buttonHref, external, onDismiss) {
  const wrap = el('div', 'container infobanner-wrap');
  const card = el('div', `infobanner infobanner-${kind}`);

  const main = el('div', 'infobanner-main');
  const iconBadge = el('div', 'infobanner-icon');
  iconBadge.append(svg(iconName));
  main.append(iconBadge);
  const body = el('div', 'infobanner-body');
  body.append(el('div', 'infobanner-title', title));
  body.append(el('div', 'infobanner-desc', desc));
  main.append(body);
  card.append(main);

  const action = el('a', 'infobanner-button');
  action.href = buttonHref;
  if (external) {
    action.target = '_blank';
    action.rel = 'noopener';
  } else {
    action.setAttribute('data-link', '');
  }
  action.append(el('span', '', buttonLabel), svg('chevron-right'));
  card.append(action);

  if (onDismiss) {
    const close = el('button', 'infobanner-close', '×');
    close.type = 'button';
    close.setAttribute('aria-label', 'Dismiss');
    close.addEventListener('click', () => {
      wrap.remove();
      onDismiss();
    });
    card.append(close);
  }

  wrap.append(card);
  return wrap;
}

export function hue(text) {
  let h = 0;
  for (const c of text) {
    h = (h * 31 + c.codePointAt(0)) % 360;
  }
  return h;
}

const brandPalette = ['var(--cat-teal)', 'var(--cat-olive)', 'var(--cat-coral)', 'var(--cat-yellow)', 'var(--cat-deep-teal)'];

export function paletteColor(text) {
  return brandPalette[hue(text) % brandPalette.length];
}

export function trimMiddle(text, max) {
  if (text.length <= max) {
    return text;
  }
  const head = Math.ceil((max - 1) * 0.55);
  return text.slice(0, head).trimEnd() + '…' + text.slice(text.length - (max - 1 - head)).trimStart();
}

export function firstName(fullName) {
  return fullName.trim().split(/\s+/)[0];
}

export function lastName(fullName) {
  const parts = (fullName || '').trim().split(/\s+/);
  return parts[parts.length - 1] || '';
}

export function shuffled(items) {
  const copy = [...items];
  for (let i = copy.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [copy[i], copy[j]] = [copy[j], copy[i]];
  }
  return copy;
}

export function ordinal(gradeName) {
  const n = Number(gradeName.split(' ')[1]);
  return n + ({1: 'st', 2: 'nd', 3: 'rd'}[n] || 'th');
}

export function csvField(value) {
  if (/^[=+\-@\t\r]/.test(value) && isNaN(Number(value))) {
    value = "'" + value;
  }
  return /[",\n\r]/.test(value) ? '"' + value.replaceAll('"', '""') + '"' : value;
}

export function pronouncePill(url, name) {
  const btn = el('button', 'pronounce-pill');
  btn.type = 'button';
  btn.title = 'Hear how to pronounce ' + name;
  btn.append(svg('volume'), el('span', '', name));
  btn.addEventListener('click', () => new Audio(url).play());
  return btn;
}

export function copyButton(text, label = 'Copy') {
  const btn = iconButton('copy', label, '', () => {
    navigator.clipboard.writeText(text);
    btn.classList.add('copied');
    btn.title = 'Copied';
    btn.replaceChildren(svg('check'));
    setTimeout(() => {
      btn.classList.remove('copied');
      btn.title = label;
      btn.replaceChildren(svg('copy'));
    }, 1200);
  });
  return btn;
}

export function contactRow(value, buttons) {
  const row = el('div', 'contact-row');
  row.append(value);
  const actions = el('div', 'contact-actions');
  for (const b of buttons) {
    actions.append(b);
  }
  row.append(actions);
  return row;
}

const LIST_SUB_TRUNCATE_LENGTH = 280;
const LIST_SUB_LINE_PREVIEW = 4;
const BULLET_LINE = /^\s*(?:[*]|-{1,2})\s+(.+)$/;

export function aboutMeText(className, facts) {
  const lines = facts.split(/\r?\n/).map(l => l.trim()).filter(Boolean);
  const bullets = lines.length > 1 ? parseBullets(lines) : null;
  if (!bullets) {
    return el('div', className, facts);
  }
  const list = el('ul', className + ' about-bullets');
  for (const item of bullets) {
    list.append(el('li', '', item));
  }
  return list;
}

function parseBullets(lines) {
  const items = [];
  for (const line of lines) {
    const m = line.match(BULLET_LINE);
    if (!m) {
      return null;
    }
    items.push(m[1].trim());
  }
  return items;
}

export function listSub(text) {
  const wrap = el('div', 'list-sub');
  const lines = text.split(/\r?\n/).map(l => l.trim()).filter(Boolean);
  const bullets = lines.length > 1 ? parseBullets(lines) : null;
  if (bullets) {
    wrap.append(collapsibleLines(bullets, 'ul', 'list-sub-bullets', 'li'));
    return wrap;
  }
  const full = lines.join('\n');
  if (full.length <= LIST_SUB_TRUNCATE_LENGTH) {
    wrap.textContent = full;
    return wrap;
  }
  let short = full.slice(0, LIST_SUB_TRUNCATE_LENGTH);
  const cutAt = Math.max(short.lastIndexOf(' '), short.lastIndexOf('\n'));
  short = cutAt > 0 ? short.slice(0, cutAt) : short;
  const textSpan = el('span', '', short + '… ');
  const toggle = el('button', 'list-sub-more', 'More »');
  toggle.type = 'button';
  let expanded = false;
  toggle.addEventListener('click', e => {
    e.preventDefault();
    e.stopPropagation();
    expanded = !expanded;
    textSpan.textContent = expanded ? full + ' ' : short + '… ';
    toggle.textContent = expanded ? 'Less' : 'More »';
  });
  wrap.append(textSpan, toggle);
  return wrap;
}

function collapsibleLines(items, wrapTag, wrapClass, itemTag) {
  const frag = document.createDocumentFragment();
  const list = el(wrapTag, wrapClass);
  const renderItems = shown => {
    list.replaceChildren();
    for (const item of shown) {
      list.append(el(itemTag, '', item));
    }
  };
  if (items.length <= LIST_SUB_LINE_PREVIEW) {
    renderItems(items);
    frag.append(list);
    return frag;
  }
  const collapsed = items.slice(0, LIST_SUB_LINE_PREVIEW);
  const hiddenCount = items.length - LIST_SUB_LINE_PREVIEW;
  renderItems(collapsed);
  const toggle = el('button', 'list-sub-more', `More (+${hiddenCount}) »`);
  toggle.type = 'button';
  let expanded = false;
  toggle.addEventListener('click', e => {
    e.preventDefault();
    e.stopPropagation();
    expanded = !expanded;
    renderItems(expanded ? items : collapsed);
    toggle.textContent = expanded ? 'Less' : `More (+${hiddenCount}) »`;
  });
  frag.append(list, toggle);
  return frag;
}
