import {el, link, imageThumb} from '/elements.js';

export function card({href, imageUrl, title, subtitle, text, media = [], chips = [], marks = [], under = null, foot = [], note = '', className = ''}) {
  const slot = el('div', 'card-slot');
  const box = el('div', className ? `card ${className}` : 'card');
  const top = link(href, 'card-media');
  top.append(imageThumb(imageUrl, title, 'card-image'), ...media);
  if (chips.length) {
    const row = el('div', 'card-chips');
    row.append(...chips.map(words => el('span', 'card-chip', words)));
    top.append(row);
  }
  const body = el('div', 'card-body');
  const heading = link(href, 'card-title');
  heading.textContent = title;
  body.append(heading);
  if (subtitle) {
    body.append(el('div', 'card-subtitle', subtitle));
  }
  if (text) {
    body.append(el('div', 'card-text clamp', text));
  }
  if (marks.length) {
    const row = el('div', 'card-marks');
    row.append(...marks);
    body.append(row);
  }
  if (under) {
    body.append(under);
  }
  const bottom = el('div', 'card-foot');
  bottom.append(...foot);
  if (note) {
    bottom.append(el('span', 'card-note', note));
  }
  box.append(top, body, bottom);
  slot.append(box);
  return slot;
}
