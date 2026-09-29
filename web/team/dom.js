import {shiftedEnd, defaultEnd, whenLabel, parentOf} from './state.js';
import {parseWhen} from '/datecard.js';
import {whenPickers} from '/form.js';
import {el, svg} from '/elements.js';

export function badge(text, kind) {
  return el('span', 'badge ' + (kind || ''), text);
}

export function displayURL(url) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '');
}

export function searchBox(placeholder, onInput) {
  const box = el('div', 'search');
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
  wrap.append(select, svg('chevron-down'));
  return wrap;
}

export function treeFilter(node, below, onChange, start) {
  const chosen = new Set(start || [node.id]);
  const self = `${node.title} (itself)`;
  const filter = el('div', 'side-filter');
  const opener = el('button', 'side-filter-toggle');
  opener.type = 'button';
  const summary = el('span', '', self);
  opener.append(summary, svg('chevron-down'));
  const menu = el('div', 'side-filter-menu');
  menu.hidden = true;
  const depthOf = n => {
    let d = 0;
    for (let p = parentOf(n); p && p !== node; p = parentOf(p)) {
      d++;
    }
    return d;
  };
  const boxes = [];
  const sources = () => [node, ...below].filter(n => chosen.has(n.id));
  const paint = () => {
    const count = chosen.size;
    summary.textContent = count === 1 && chosen.has(node.id) ? self : (count ? `${count} selected` : 'None');
  };
  const refresh = () => {
    paint();
    onChange(sources());
  };
  const option = (n, label) => {
    const item = el('label', 'side-filter-item');
    item.style.paddingLeft = `${10 + (n === node ? 0 : (depthOf(n) + 1) * 14)}px`;
    const box = el('input');
    box.type = 'checkbox';
    box.checked = chosen.has(n.id);
    box.addEventListener('change', () => {
      if (box.checked) {
        chosen.add(n.id);
      } else {
        chosen.delete(n.id);
      }
      refresh();
    });
    boxes.push({box, id: n.id});
    item.append(box, el('span', '', label));
    return item;
  };
  const bulk = el('div', 'side-filter-bulk');
  const setAll = on => {
    chosen.clear();
    for (const {box, id} of boxes) {
      box.checked = on;
      if (on) {
        chosen.add(id);
      }
    }
    refresh();
  };
  const all = el('button', 'link-button', 'Select All');
  all.type = 'button';
  all.addEventListener('click', () => setAll(true));
  const none = el('button', 'link-button', 'Clear All');
  none.type = 'button';
  none.addEventListener('click', () => setAll(false));
  bulk.append(all, none);
  menu.append(bulk);
  menu.append(option(node, self));
  for (const n of below) {
    menu.append(option(n, n.title));
  }
  paint();
  opener.addEventListener('click', e => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
  });
  menu.addEventListener('click', e => e.stopPropagation());
  document.addEventListener('click', () => {
    menu.hidden = true;
  });
  filter.append(opener, menu);
  return {wrap: filter, sources, self};
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

export function whenEditor(start, end, timing, parent, layout) {
  const rows = layout === 'rows';
  const wrap = el('div', 'field-when' + (rows ? ' is-rows' : ''));
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
  let filled = '';
  const shiftEnd = () => {
    const now = from.value();
    const current = to.value();
    const fresh = !current || current === filled;
    const moved = fresh ? defaultEnd(now) : shiftedEnd(last, now, current, to.hasTime());
    if (moved) {
      to.set(moved);
    }
    if (fresh) {
      filled = moved;
    }
    if (parseWhen(now)) {
      last = now;
      to.date.min = from.date.value;
    }
  };
  const from = whenPickers('Starts', start, shiftEnd, 'No start time');
  const to = whenPickers('Ends', end, null, 'No end time');
  to.date.min = from.date.value;
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
