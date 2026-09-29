import {el} from '/elements.js';

export function pageHead(title, actions) {
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', title));
  head.append(main);
  if (actions && actions.length) {
    const wrap = el('div', 'page-actions');
    wrap.append(...actions);
    head.append(wrap);
  }
  return head;
}
