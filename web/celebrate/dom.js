import {el, svg} from '/elements.js';

export function badge(text, kind) {
  return el('span', 'badge ' + (kind || ''), text);
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
  wrap.append(select, svg('chevron-down'));
  return wrap;
}

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
      p.append(line);
    });
    wrap.append(p);
  }
  return wrap;
}
