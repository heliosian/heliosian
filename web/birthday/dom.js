import {el, svg, iconButton, toast} from '/elements.js';

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

export function menu(items) {
  const wrap = el('div', 'more-wrap');
  const trigger = iconButton('more', 'More', '', () => {
    const opening = list.hidden;
    for (const open of document.querySelectorAll('.row-menu')) {
      open.hidden = true;
    }
    list.hidden = !opening;
  });
  const list = el('div', 'row-menu');
  list.hidden = true;
  for (const item of items) {
    const b = el('button', item.danger ? 'danger' : '');
    b.type = 'button';
    if (item.icon) {
      b.append(svg(item.icon));
    }
    b.append(el('span', '', item.label));
    b.addEventListener('click', e => {
      e.preventDefault();
      e.stopPropagation();
      list.hidden = true;
      item.onClick();
    });
    list.append(b);
  }
  wrap.append(trigger, list);
  return wrap;
}

export async function copyRich(text, html, message) {
  try {
    await navigator.clipboard.write([new ClipboardItem({
      'text/plain': new Blob([text], {type: 'text/plain'}),
      'text/html': new Blob([html], {type: 'text/html'}),
    })]);
  } catch (err) {
    toast('Couldn’t copy: ' + err.message);
    return;
  }
  toast(message || 'Copied');
}
