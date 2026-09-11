import {shiftedEnd, parseWhen, whenLabel} from './state.js';

export function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

export function link(href, className, text) {
  const node = el('a', className, text);
  node.href = href;
  node.setAttribute('data-link', '');
  return node;
}

const paths = {
  signup: 'M12 3v18M4.5 7.5l15 9M19.5 7.5l-15 9',
  star: 'M12 3l2.9 6 6.6.9-4.8 4.6 1.2 6.5L12 17.8 6.1 21l1.2-6.5L2.5 9.9l6.6-.9z',
  calendar: 'M4 5h16a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 10h18M8 3v4M16 3v4',
  doc: 'M7 3h7l5 5v13H7zM14 3v5h5M10 13h6M10 17h6',
  clock: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM12 7v5l3 2',
  chat: 'M4 5h16v11H8l-4 4z',
  phone: 'M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2z',
  menu: 'M4 7h16M4 12h16M4 17h16',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
  chevron: 'M9 6l6 6-6 6',
  caret: 'M6 9l6 6 6-6',
  up: 'M12 19V5M5 12l7-7 7 7',
  mail: 'M3 6h18v12H3zM3 7l9 6 9-6',
  image: 'M3 5h18v14H3zM3 16l5-5 4 4 3-3 6 6',
  share: 'M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM8.6 13.5l6.8 4M15.4 6.5l-6.8 4',
  down: 'M12 5v14M5 12l7 7 7-7',
  back: 'M15 6l-6 6 6 6',
  plus: 'M12 5v14M5 12h14',
  join: 'M20 12a8 8 0 1 1-4-6.9M9 12l2.5 2.5L20 6',
  check: 'M5 12l5 5L20 7',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z',
  open: 'M15 3h6v6M10 14 21 3M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5',
  copy: 'M8 8h11a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2zM16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3',
  idea: 'M9 18h6M10 21h4M12 3a6 6 0 0 0-4 10.5c.7.6 1 1.3 1 2.5h6c0-1.2.3-1.9 1-2.5A6 6 0 0 0 12 3z',
  receipt: 'M5 3h14v18l-3-2-2 2-2-2-2 2-2-2-3 2zM8 8h8M8 12h8M8 16h5',
  search: 'M11 4a7 7 0 1 1 0 14 7 7 0 0 1 0-14zM20 20l-4-4',
  list: 'M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01',
  people: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM23 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  tools: 'M14.7 6.3a4 4 0 0 0 5 5L13 18l-1 4-4-1 6.7-6.7a4 4 0 0 0-5-5L3 3l4 1 1 4z',
  close: 'M6 6l12 12M18 6L6 18',
  next: 'M9 6l6 6-6 6',
  prev: 'M15 6l-6 6 6 6',
  trash: 'M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3',
  download: 'M12 3v12M6 11l6 6 6-6M4 21h16',
};

export function svg(name) {
  const node = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  node.setAttribute('viewBox', '0 0 24 24');
  node.setAttribute('fill', 'none');
  node.setAttribute('stroke', 'currentColor');
  node.setAttribute('stroke-width', '2');
  node.setAttribute('stroke-linecap', 'round');
  node.setAttribute('stroke-linejoin', 'round');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', paths[name]);
  node.append(path);
  return node;
}

export function button(label, icon, className, onClick) {
  const node = el('button', className || 'button');
  node.type = 'button';
  if (icon) {
    node.append(svg(icon));
  }
  node.append(el('span', '', label));
  if (onClick) {
    node.addEventListener('click', e => {
      e.preventDefault();
      e.stopPropagation();
      onClick(e);
    });
  }
  return node;
}

export function thumb(url, title, className) {
  if (url) {
    const img = el('img', 'thumb ' + (className || ''));
    img.src = url;
    img.alt = '';
    img.loading = 'lazy';
    return img;
  }
  return el('div', 'thumb initial ' + (className || ''), (title || '?').slice(0, 1).toUpperCase());
}

export function avatar(person, className) {
  const node = el('div', 'avatar ' + (className || ''));
  if (person.photoUrl) {
    const img = el('img');
    img.src = person.photoUrl;
    img.alt = '';
    img.loading = 'lazy';
    node.append(img);
    return node;
  }
  node.textContent = (person.name || person.email).slice(0, 1).toUpperCase();
  return node;
}

export function badge(text, kind) {
  return el('span', 'badge ' + (kind || ''), text);
}

export function displayURL(url) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '');
}

export function searchBox(placeholder, onInput, light) {
  const box = el('div', 'search' + (light ? ' light' : ''));
  const input = el('input');
  input.type = 'search';
  input.placeholder = placeholder || 'Search';
  input.addEventListener('input', () => onInput(input.value.trim().toLowerCase()));
  box.append(svg('search'), input);
  return box;
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
  wrap.append(select, svg('caret'));
  return wrap;
}

export function toggle(label, checked, onChange) {
  const row = el('div', 'toggle-row');
  row.append(el('span', '', label));
  const wrap = el('label', 'switch');
  const input = el('input');
  input.type = 'checkbox';
  input.checked = checked;
  input.addEventListener('change', () => onChange(input.checked));
  wrap.append(input, el('span'));
  row.append(wrap);
  return row;
}

export function tabs(items, active, onPick, light) {
  const bar = el('div', 'tabs' + (light ? ' light' : ''));
  for (const item of items) {
    const b = el('button', item.key === active ? 'is-active' : '', item.label);
    b.type = 'button';
    b.addEventListener('click', () => onPick(item.key));
    bar.append(b);
  }
  return bar;
}

let toastTimer;

export function toast(message) {
  const node = document.querySelector('#toast');
  node.textContent = message;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    node.hidden = true;
  }, 2200);
}

export async function copyText(text, message) {
  await navigator.clipboard.writeText(text);
  toast(message || 'Copied');
}

// whenPickers is a labelled date picker beside an optional time picker. The
// sheet stores "YYYY-MM-DD" or "YYYY-MM-DD HH:MM", and a blank time is
// meaningful - it is how an all-day thing is written - so the two stay separate
// controls rather than one datetime-local, which would silently stamp midnight
// onto every all-day date. Beside the label sits a way to clear both: a thing
// with no start is one that runs by its timing text, and a start on its own
// is a perfectly good way to describe something.
export function whenPickers(label, value, onChange, clearLabel) {
  const m = /^(\d{4}-\d{2}-\d{2})(?: (\d{2}:\d{2}))?$/.exec(value || '');
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
  const read = () => (date.value ? (time.value ? `${date.value} ${time.value}` : date.value) : '');
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
    value: read,
    hasTime: () => Boolean(time.value),
    // set writes a Date back out in the shape the sheet uses, keeping whether
    // this end of the range carries a time - shifting an all-day date by a few
    // hours must not give it one.
    set: written => {
      const m2 = /^(\d{4}-\d{2}-\d{2})(?: (\d{2}:\d{2}))?$/.exec(written);
      date.value = m2 ? m2[1] : '';
      time.value = m2 && m2[2] ? m2[2] : '';
    },
  };
}

// whenEditor is the one editor for when a thing happens, shared by the event
// page's inline pencil and the edit form: a start and an end, each clearable,
// an all-day switch that puts the times away, and the free-text timing for
// things without a date. Moving the start drags the end with it, so the length
// of the thing stays put and only its position changes.
export function whenEditor(start, end, timing, parent, layout) {
  const rows = layout === 'rows';
  const wrap = el('div', 'field-when' + (rows ? ' is-rows' : ''));
  // Under a parent, the usual answer is "when the parent is": a switch at the
  // top says so, and while it is on the pickers stay out of sight and the
  // thing is saved with no dates of its own, so it follows the parent's even
  // when those change. Switching it off starts from the parent's times.
  const own = el('div', 'field-when-own');
  let same = null;
  if (parent) {
    const line = el('label', 'field-when-same');
    same = el('input');
    same.type = 'checkbox';
    same.checked = !start && !timing;
    const text = el('span');
    text.append(el('span', '', `Same as ${parent.title}`));
    const theirs = whenLabel(parent);
    if (theirs) {
      text.append(el('small', '', theirs));
    }
    const knob = el('span', 'switch');
    knob.append(same, el('span'));
    if (rows) {
      // A tinted band: what the switch does on the left, the switch and the
      // parent's own time on the right.
      const lead = el('span', 'field-when-same-lead');
      lead.append(el('span', '', 'Use the same date and time as the parent event'),
        el('small', '', 'Keep this activity in sync with the event it is part of.'));
      const control = el('span', 'field-when-same-control');
      control.append(knob, text);
      line.append(lead, control);
    } else {
      line.append(text, knob);
    }
    wrap.append(line);
    if (same.checked) {
      start = parent.start || '';
      end = parent.end || '';
      timing = parent.timing || '';
    }
    same.addEventListener('change', () => {
      own.hidden = same.checked;
    });
  }
  wrap.append(own);
  let last = start || '';
  const shiftEnd = () => {
    const now = from.value();
    const moved = shiftedEnd(last, now, to.value(), to.hasTime());
    if (moved) {
      to.set(moved);
    }
    if (parseWhen(now)) {
      last = now;
      to.date.min = from.date.value;
    }
  };
  const from = whenPickers('Starts', start, shiftEnd, 'No start time');
  const to = whenPickers('Ends', end, null, 'No end time');
  to.date.min = from.date.value;
  // All day: the times are put away and cleared, so the dates stand alone.
  const allDay = el('label', 'field-when-allday');
  const allDayBox = el('input');
  allDayBox.type = 'checkbox';
  allDayBox.checked = Boolean(from.date.value) && !from.hasTime() && !to.hasTime();
  allDay.append(allDayBox, el('span', '', 'All-day event'));
  const applyAllDay = () => {
    for (const p of [from, to]) {
      p.wrap.classList.toggle('is-all-day', allDayBox.checked);
      if (allDayBox.checked) {
        p.set(p.date.value);
      }
    }
    last = from.value();
  };
  allDayBox.addEventListener('change', applyAllDay);
  applyAllDay();
  const timingInput = el('input');
  timingInput.type = 'text';
  timingInput.value = timing || '';
  timingInput.placeholder = 'All Year, Late February, A few times per year';
  if (rows) {
    // Settings rows: each picker's label and a sentence on the left, the
    // pickers and their clear control in one line on the right.
    const row = (label, hint, control) => {
      const r = el('div', 'setting-row');
      const t = el('div', 'setting-text');
      t.append(el('div', 'setting-label', label));
      if (hint) {
        t.append(el('div', 'setting-hint', hint));
      }
      const c = el('div', 'setting-control');
      c.append(control);
      r.append(t, c);
      return r;
    };
    for (const p of [from, to]) {
      // The label moves to the row's left side; the clear control joins the pickers' line.
      const head = p.wrap.querySelector('.field-when-head');
      const clear = head.querySelector('.field-when-clear');
      p.wrap.querySelector('.field-when-pair').append(clear);
      head.remove();
    }
    allDay.querySelector('span').append(el('small', '', 'This activity lasts the entire day.'));
    own.append(
      row('Start', 'When does this activity begin?', from.wrap),
      row('End', 'When does this activity end?', to.wrap),
      row('', '', allDay),
      row('Or in words', 'Shown instead of the date and time, when set.', timingInput));
  } else {
    own.append(from.wrap, to.wrap, allDay, el('span', 'field-when-label', 'Or in words'), timingInput);
  }
  own.hidden = Boolean(same && same.checked);
  return {
    wrap,
    focus: () => (same && same.checked ? same.focus() : from.focus()),
    value: () => (same && same.checked
      ? {start: '', end: '', timing: ''}
      : {start: from.value(), end: to.value(), timing: timingInput.value.trim()}),
    validate: v => {
      const a = parseWhen(v.start);
      const b = parseWhen(v.end);
      if (v.end && !v.start) {
        return 'Give it a start as well as an end.';
      }
      return a && b && b.date <= a.date ? 'The end has to come after the start.' : '';
    },
  };
}
