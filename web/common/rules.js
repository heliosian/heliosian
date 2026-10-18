import {el, svg} from '/elements.js';

function afterPaint(run) {
  requestAnimationFrame(() => setTimeout(run));
}

export function chipToggle(label, on, onChange, key) {
  const b = el('button', 'chip-toggle' + (key ? ' chip-toggle-' + key : '') + (on ? ' active' : ''), label);
  b.type = 'button';
  b.addEventListener('click', () => {
    const on = b.classList.toggle('active');
    afterPaint(() => onChange(on));
  });
  return b;
}

const panelMargin = 12;

function placeTop(wrap, panel) {
  const top = wrap.getBoundingClientRect().bottom + 8;
  panel.style.top = `${top}px`;
  panel.style.maxHeight = `${window.innerHeight - top - panelMargin}px`;
}

export function clampFilterPanel(wrap, panel) {
  const wrapRect = wrap.getBoundingClientRect();
  const panelWidth = panel.offsetWidth;
  const column = wrap.closest('.container');
  const minLeft = Math.max(panelMargin, column ? column.getBoundingClientRect().left + parseFloat(getComputedStyle(column).paddingLeft) : 0);
  const fitsRight = wrapRect.left + panelWidth <= window.innerWidth - panelMargin;
  const left = fitsRight ? wrapRect.left : Math.max(minLeft, Math.min(wrapRect.right - panelWidth, window.innerWidth - panelMargin - panelWidth));
  panel.style.left = `${left}px`;
  placeTop(wrap, panel);
}

export function closeFilterPanels() {
  for (const panel of document.querySelectorAll('.filter-panel')) {
    panel.hidden = true;
  }
  for (const open of document.querySelectorAll('.filter-button.open')) {
    open.classList.remove('open');
  }
}

document.addEventListener('click', e => {
  for (const panel of document.querySelectorAll('.filter-panel')) {
    if (!panel.hidden && !panel.parentElement.contains(e.target)) {
      panel.hidden = true;
      panel.parentElement.querySelector('.filter-button').classList.remove('open');
    }
  }
});

document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    closeFilterPanels();
  }
});

function followButtons() {
  for (const panel of document.querySelectorAll('.filter-panel:not([hidden])')) {
    placeTop(panel.parentElement, panel);
  }
}

document.addEventListener('scroll', followButtons, true);
window.addEventListener('resize', followButtons);

function dropdown(className, buttonParts) {
  const wrap = el('div', 'filter-wrap');
  const b = el('button', 'filter-button' + (className ? ' ' + className : ''));
  b.type = 'button';
  b.append(...buttonParts);
  const panel = el('div', 'filter-panel');
  panel.hidden = true;
  b.addEventListener('click', () => {
    const opening = panel.hidden;
    closeFilterPanels();
    panel.hidden = !opening;
    b.classList.toggle('open', opening);
    if (opening) {
      clampFilterPanel(wrap, panel);
    }
  });
  const close = () => {
    panel.hidden = true;
    b.classList.remove('open');
  };
  wrap.append(b, panel);
  return {wrap, panel, close};
}

function optionRow(v, chosen, onChange) {
  const {value, label, icon, depth} = typeof v === 'string' ? {value: v, label: v} : v;
  const row = el('label', 'filter-option' + (depth ? ' is-under' : ''));
  if (depth) {
    row.style.setProperty('--depth', depth);
  }
  const box = el('input');
  box.type = 'checkbox';
  box.checked = chosen.has(value);
  box.addEventListener('change', () => {
    if (box.checked) {
      chosen.add(value);
    } else {
      chosen.delete(value);
    }
    afterPaint(onChange);
  });
  if (icon) {
    row.append(svg(icon));
  }
  row.append(el('span', '', label), box);
  return row;
}

function footer(clearLabel, onClear, onDone) {
  const foot = el('div', 'filter-footer');
  const clear = el('button', 'filter-clear', clearLabel);
  clear.type = 'button';
  clear.addEventListener('click', onClear);
  const done = el('button', 'filter-done', 'Done');
  done.type = 'button';
  done.addEventListener('click', onDone);
  foot.append(clear, done);
  return foot;
}

export function facetDropdown(label, icon, values, chosen, onChange) {
  const labelSpan = el('span', '', label);
  const {wrap, panel, close} = dropdown('facet-button', [...(icon ? [svg(icon)] : []), labelSpan, svg('chevron-down')]);
  const updateLabel = () => {
    labelSpan.textContent = chosen.size ? `${label} (${chosen.size})` : label;
  };
  const changed = () => {
    updateLabel();
    onChange();
  };
  const body = el('div', 'filter-options');
  if (!values.length) {
    body.append(el('div', 'filter-empty', 'Nothing to choose yet.'));
  }
  for (const v of values) {
    body.append(optionRow(v, chosen, changed));
  }
  panel.append(body, footer('Clear', () => {
    chosen.clear();
    for (const box of body.querySelectorAll('input')) {
      box.checked = false;
    }
    changed();
  }, close));
  updateLabel();
  return wrap;
}

export function familyDropdown(values, chosen, onChange) {
  const labelSpan = el('span', '', 'Add');
  const {wrap, panel} = dropdown('facet-button', [svg('families'), labelSpan, svg('chevron-down')]);
  panel.classList.add('family-panel');
  const updateLabel = () => {
    labelSpan.textContent = chosen.size ? `Add (${chosen.size})` : 'Add';
  };
  const head = el('div', 'family-head');
  const text = el('div');
  text.append(el('div', 'family-title', 'Also add family members'),
    el('div', 'family-desc', 'Add parents, children, or siblings of the people already matched above.'));
  head.append(svg('families'), text);
  const body = el('div', 'family-options');
  for (const value of values) {
    const row = el('label', 'family-option');
    const box = el('input');
    box.type = 'checkbox';
    box.checked = chosen.has(value);
    box.addEventListener('change', () => {
      if (box.checked) {
        chosen.add(value);
      } else {
        chosen.delete(value);
      }
      updateLabel();
      afterPaint(onChange);
    });
    row.append(box, el('span', '', `Their ${value.toLowerCase()}`));
    body.append(row);
  }
  const note = el('div', 'family-note');
  note.append(svg('info'), el('span', '', 'Adds these relatives to the people already matched above.'));
  panel.append(head, body, note);
  updateLabel();
  return wrap;
}

export function filterControl(sections, onChange, toggles = []) {
  const labelSpan = el('span', '', 'Filter');
  const {wrap, panel, close} = dropdown('', [svg('filter'), labelSpan, svg('chevron-down')]);
  const updateLabels = () => {
    let total = 0;
    for (const s of sections) {
      total += s.chosen.size;
      s.labelSpan.textContent = s.chosen.size ? `${s.label} (${s.chosen.size})` : s.label;
    }
    labelSpan.textContent = total ? `Filter (${total})` : 'Filter';
  };
  const changed = () => {
    updateLabels();
    onChange();
  };
  for (const s of sections) {
    const head = el('div', 'filter-section');
    s.labelSpan = el('span', '', s.label);
    if (s.icon) {
      head.append(svg(s.icon));
    }
    head.append(s.labelSpan, svg('chevron-down'));
    const body = el('div', 'filter-options');
    body.hidden = !s.chosen.size;
    head.classList.toggle('open', !body.hidden);
    head.addEventListener('click', () => {
      body.hidden = !body.hidden;
      head.classList.toggle('open', !body.hidden);
    });
    for (const v of s.values) {
      body.append(optionRow(v, s.chosen, changed));
    }
    panel.append(head, body);
  }
  for (const t of toggles) {
    const row = el('label', 'filter-toggle-row');
    const box = el('input', 'filter-switch');
    box.type = 'checkbox';
    box.checked = t.on;
    box.addEventListener('change', () => {
      const on = box.checked;
      afterPaint(() => t.onChange(on));
    });
    row.append(el('span', '', t.label), box);
    panel.append(row);
  }
  panel.append(footer('Clear all', () => {
    for (const s of sections) {
      s.chosen.clear();
    }
    for (const t of toggles) {
      t.onChange(false);
    }
    for (const box of panel.querySelectorAll('input')) {
      box.checked = false;
    }
    changed();
  }, close));
  updateLabels();
  return wrap;
}
