import {el, svg} from '/elements.js';

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

const when = /^(\d{4}-\d{2}-\d{2})(?: (\d{2}:\d{2}))?$/;

export function whenPickers(label, value, onChange, clearLabel) {
  const m = when.exec(value || '');
  const wrap = el('div', 'field-when-block');
  const head = el('div', 'field-when-head');
  head.append(el('span', 'field-when-label', label));
  const pair = el('div', 'field-when-pair');
  const date = el('input');
  date.type = 'date';
  date.value = m ? m[1] : '';
  const time = el('input');
  time.type = 'time';
  time.value = m && m[2] ? m[2] : '';
  pair.append(date, time);
  const clear = el('button', 'field-when-clear');
  clear.type = 'button';
  clear.title = clearLabel;
  clear.setAttribute('aria-label', clearLabel);
  clear.append(svg('close'), el('span', '', clearLabel));
  clear.addEventListener('click', () => {
    date.value = '';
    time.value = '';
    if (onChange) {
      onChange();
    }
    date.focus();
  });
  head.append(clear);
  wrap.append(head, pair);
  if (onChange) {
    date.addEventListener('change', onChange);
    time.addEventListener('change', onChange);
  }
  return {
    wrap,
    date,
    focus: () => date.focus(),
    value: () => (date.value ? (time.value ? `${date.value} ${time.value}` : date.value) : ''),
    hasTime: () => Boolean(time.value),
    set: written => {
      const m2 = when.exec(written);
      date.value = m2 ? m2[1] : '';
      time.value = m2 && m2[2] ? m2[2] : '';
    },
  };
}
