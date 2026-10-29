import {el, svg} from '/elements.js';

export function searchBox({placeholder = 'Search', value = '', type = '', onInput}) {
  const box = el('div', 'search');
  const input = el('input');
  if (type) {
    input.type = type;
  }
  input.placeholder = placeholder;
  input.value = value;
  input.addEventListener('input', () => onInput(input.value.trim().toLowerCase()));
  box.append(svg('search'), input);
  return box;
}
