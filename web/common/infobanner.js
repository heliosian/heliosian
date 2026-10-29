import {el, svg} from '/elements.js';

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
