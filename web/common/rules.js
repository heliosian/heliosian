import {appOrigin} from '/appswitch.js';
import {el, svg, button, iconButton} from '/elements.js';

function plural(role) {
  return role === 'Staff' ? 'Staff' : role + 's';
}

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

export function rulesEditor({options, personName}) {
  const listIcons = {party: 'party', activity: 'activity', room: 'classrooms'};

  function ruleRow(rule, onChange, onRemove, open, countChip) {
    const row = el('div', 'rule');
    const mine = rule.tags.every(t => options().tags.some(o => o.key === t) || options().lists.some(l => l.key === t));
    let count = null;
    row.refreshCount = () => {
      if (count && countChip) {
        const next = countChip(rule);
        count.replaceWith(next);
        count = next;
      }
    };
    const showWords = () => {
      row.classList.remove('is-open');
      row.classList.add('is-line', 'is-' + rule.kind);
      row.replaceChildren();
      const words = el('div', 'rule-controls');
      const line = el('div', 'rule-line');
      line.append(el('span', 'rule-kind', rule.kind === 'include' ? 'Include' : 'Exclude'));
      line.append(svg(rule.kind === 'include' ? 'groups' : 'user-minus'));
      const sentence = el('span', 'rule-line-words');
      sentence.append(...ruleNodes(rule));
      line.append(sentence);
      if (countChip) {
        count = countChip(rule);
        line.append(count);
      }
      words.append(line);
      if (!mine) {
        row.classList.add('is-theirs');
        words.append(el('div', 'rule-note', 'Names a tag that is not yours to name; it can be removed but not changed.'));
      }
      row.append(words);
      if (mine) {
        row.append(iconButton('edit', 'Change this rule', 'rule-remove', showControls));
      }
      row.append(iconButton('trash', 'Remove this rule', 'rule-remove', onRemove));
    };
    const showControls = () => {
      row.classList.add('is-open', 'is-' + rule.kind);
      row.classList.remove('is-line');
      count = null;
      row.replaceChildren();
      const controls = ruleControls(rule, onChange);
      row.append(controls);
      const done = button('Done', 'check', 'button button-small', () => {
        if (!ruleSaysSomething(rule)) {
          onRemove();
          return;
        }
        showWords();
      });
      controls.querySelector('.rule-said').after(done);
      row.append(iconButton('trash', 'Remove this rule', 'rule-remove', onRemove));
      const first = controls.querySelector('.chip-toggle');
      if (first) {
        first.focus();
      }
    };
    if (open && mine) {
      showControls();
    } else {
      showWords();
    }
    return row;
  }

  function ruleControls(rule, onChange) {
    const controls = el('div', 'rule-controls');
    const said = el('div', 'rule-said');
    const sayIt = () => {
      const words = ruleSaysSomething(rule) ? ruleWords(rule) : '';
      said.textContent = words ? (rule.kind === 'exclude' ? 'Leaves out: ' : 'Includes: ') + words : 'Pick a role, some words, a classroom, a grade or a tag.';
      said.classList.toggle('is-empty', !words);
    };
    const changed = () => {
      sayIt();
      onChange();
    };
    const roles = el('div', 'chip-row');
    for (const role of options().roles) {
      roles.append(chipToggle(plural(role), rule.roles.includes(role), on => {
        rule.roles = on ? [...rule.roles, role] : rule.roles.filter(r => r !== role);
        changed();
      }, role.toLowerCase()));
    }
    controls.append(roles);
    const search = el('input', 'rule-search');
    search.type = 'search';
    search.placeholder = 'Words in a name or address';
    search.maxLength = 80;
    search.value = rule.search;
    let timer;
    search.addEventListener('input', () => {
      rule.search = search.value.trim();
      clearTimeout(timer);
      timer = setTimeout(changed, 300);
    });
    controls.append(search);
    const classrooms = new Set(rule.classrooms);
    controls.append(facetDropdown('Classroom', null, options().classrooms, classrooms, () => {
      rule.classrooms = [...classrooms];
      changed();
    }));
    const grades = new Set(rule.grades);
    controls.append(facetDropdown('Grade', null, options().grades, grades, () => {
      rule.grades = [...grades];
      changed();
    }));
    const tags = new Set(rule.tags);
    const depthOf = l => {
      let depth = 0;
      for (let at = l; at && at.parent; at = options().lists.find(x => x.key === at.parent)) {
        depth++;
      }
      return depth;
    };
    const shortName = l => (l.parent ? l.name.slice(l.name.lastIndexOf(': ') + 2) : l.name);
    const tagValues = [
      ...options().tags.map(t => ({value: t.key, label: t.name, icon: 'tag'})),
      ...options().lists.map(l => ({value: l.key, label: shortName(l), whole: l.name, icon: listIcons[l.kind], depth: depthOf(l)})),
    ];
    controls.append(facetDropdown('Tags', 'tag', tagValues, tags, () => {
      rule.tags = [...tags];
      rule.tagLabels = rule.tags.map(t => {
        const v = tagValues.find(x => x.value === t) || {label: t};
        return v.whole || v.label;
      });
      changed();
    }));
    const family = new Set(rule.family);
    controls.append(familyDropdown(options().relations, family, () => {
      rule.family = [...family];
      changed();
    }));
    controls.append(said);
    sayIt();
    return controls;
  }

  function newRule(kind) {
    return {kind, roles: [], search: '', classrooms: [], grades: [], tags: [], family: []};
  }

  function ruleSaysSomething(r) {
    return r.roles.length || r.search || r.classrooms.length || r.grades.length || r.tags.length;
  }

  const singular = {Student: 'student', Parent: 'parent', Staff: 'staff member'};

  function personWords(r) {
    if (r.search && r.search.includes('@') && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length && personName(r.search)) {
      return 'named in a rule';
    }
    const parts = [];
    if (r.roles.length) {
      parts.push(r.roles.map(x => singular[x]).join(' or '));
    }
    if (r.search) {
      parts.push(`with “${r.search}” in their name or address`);
    }
    if (r.grades.length) {
      parts.push('in ' + r.grades.join(' or '));
    }
    if (r.classrooms.length) {
      parts.push('in ' + r.classrooms.join(' or '));
    }
    if (r.tags.length) {
      parts.push('tagged in ' + (r.tagLabels || r.tags).join(' or '));
    }
    return parts.join(' ');
  }

  function ruleSentence(r, tag) {
    if (r.search && r.search.includes('@') && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length) {
      const name = personName(r.search);
      if (name) {
        return [`${name} (${r.search})`];
      }
    }
    const roles = r.roles.map(plural).join(' or ');
    const parts = [];
    if (r.roles.length && !r.family.length) {
      parts.push(roles);
    } else {
      parts.push('Anyone');
    }
    if (r.search) {
      parts.push(` with “${r.search}” in their name or address`);
    }
    if (r.grades.length) {
      parts.push(' in ' + r.grades.join(' or '));
    }
    if (r.classrooms.length) {
      parts.push(' in ' + r.classrooms.join(' or '));
    }
    if (r.tags.length) {
      parts.push(' tagged in ');
      const labels = r.tagLabels || r.tags;
      r.tags.forEach((key, i) => {
        if (i) {
          parts.push(' or ');
        }
        parts.push(tag(key, labels[i]));
      });
    }
    if (r.family.length) {
      parts.push(', plus their ' + r.family.map(f => f.toLowerCase()).join(' and '));
      if (r.roles.length) {
        parts.push(`, keeping ${roles.toLowerCase()} only`);
      }
    }
    return parts;
  }

  function ruleWords(r) {
    return ruleSentence(r, (key, label) => label).join('');
  }

  const listPages = {activity: ['team', '/activities/'], party: ['celebrate', '/parties/']};

  function ruleNodes(r) {
    return ruleSentence(r, (key, label) => {
      const at = key.indexOf(':');
      const page = at > 0 && listPages[key.slice(0, at)];
      if (!page) {
        return label;
      }
      const a = el('a', 'rule-group-link', label);
      a.href = `${appOrigin(page[0])}${page[1]}${encodeURIComponent(key.slice(at + 1))}`;
      return a;
    });
  }

  return {ruleRow, ruleControls, newRule, ruleSaysSomething, personWords, ruleWords, listIcons};
}
