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

export function field(label, input, hint, required) {
  const wrap = el('label', 'field' + (required ? ' is-required' : ''));
  wrap.append(typeof label === 'string' ? el('span', '', label) : label, input);
  if (hint) {
    wrap.append(el('small', '', hint));
  }
  return wrap;
}

export function text(value, options = {}) {
  const input = el('input');
  input.type = options.type || 'text';
  input.value = value === undefined || value === null ? '' : String(value);
  if (options.placeholder) {
    input.placeholder = options.placeholder;
  }
  if (options.required) {
    input.required = true;
  }
  if (options.maxLength) {
    input.maxLength = options.maxLength;
  }
  if (options.min !== undefined) {
    input.min = options.min;
  }
  if (options.max !== undefined) {
    input.max = options.max;
  }
  if (options.step !== undefined) {
    input.step = options.step;
  }
  return input;
}

export function textarea(value, rows) {
  const input = el('textarea');
  input.rows = rows || 4;
  input.value = value || '';
  return input;
}

export function select(options, value) {
  const input = el('select');
  fillSelect(input, options, value);
  return input;
}

export function fillSelect(input, options, value) {
  input.replaceChildren();
  for (const option of options) {
    const node = el('option', '', option.label === undefined ? option : option.label);
    node.value = option.value === undefined ? option : option.value;
    node.selected = node.value === value;
    input.append(node);
  }
}

export function checkbox(label, checked, hint) {
  const wrap = el('label', 'field field-toggle');
  const input = el('input');
  input.type = 'checkbox';
  input.checked = Boolean(checked);
  const words = el('span');
  words.append(el('span', '', label));
  if (hint) {
    words.append(el('small', '', hint));
  }
  const knob = el('span', 'switch');
  knob.append(input, el('span'));
  wrap.append(knob, words);
  return {wrap, input};
}

export function segmented(options, initial, onChange) {
  const wrap = el('div', 'segmented');
  wrap.setAttribute('role', 'radiogroup');
  let value = initial;
  const buttons = options.map(o => {
    const b = el('button', 'segment' + (o.value === initial ? ' is-on' : ''), o.label);
    b.type = 'button';
    b.setAttribute('role', 'radio');
    b.setAttribute('aria-checked', String(o.value === initial));
    b.addEventListener('click', () => {
      value = o.value;
      for (const other of buttons) {
        const on = other === b;
        other.classList.toggle('is-on', on);
        other.setAttribute('aria-checked', String(on));
      }
      onChange(o.value);
    });
    wrap.append(b);
    return b;
  });
  return {wrap, get value() { return value; }};
}
