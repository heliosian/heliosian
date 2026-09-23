import {appOrigin} from '/toolbar.js';

function plural(role) {
  return role === 'Staff' ? 'Staff' : role + 's';
}

export function filterWidgets({el, svg, button}) {
  function chipToggle(label, on, onChange, key) {
    const b = el('button', 'chip-toggle' + (key ? ' chip-toggle-' + key : '') + (on ? ' active' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      b.classList.toggle('active');
      onChange(b.classList.contains('active'));
    });
    return b;
  }

  function facetDropdown(label, icon, values, chosen, onChange) {
    const wrap = el('div', 'facet-wrap');
    const b = el('button', 'facet-button');
    b.type = 'button';
    const labelSpan = el('span', '', label);
    if (icon) {
      b.append(svg(icon));
    }
    b.append(labelSpan, svg('chevron'));
    const panel = el('div', 'facet-panel');
    panel.hidden = true;
    const updateLabel = () => {
      labelSpan.textContent = chosen.size ? `${label} (${chosen.size})` : label;
    };
    b.addEventListener('click', () => {
      const opening = panel.hidden;
      for (const other of document.querySelectorAll('.facet-panel')) {
        other.hidden = true;
      }
      for (const open of document.querySelectorAll('.facet-button.open')) {
        open.classList.remove('open');
      }
      panel.hidden = !opening;
      b.classList.toggle('open', opening);
    });
    if (!values.length) {
      panel.append(el('div', 'facet-empty', 'Nothing to choose yet.'));
    }
    for (const v of values) {
      const {value, label: text, icon: mark} = typeof v === 'string' ? {value: v, label: v} : v;
      const row = el('label', 'facet-option');
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
        onChange();
      });
      if (mark) {
        row.append(svg(mark));
      }
      row.append(el('span', '', text), box);
      panel.append(row);
    }
    const foot = el('div', 'facet-foot');
    foot.append(button('Clear', null, 'button button-secondary button-small', () => {
      chosen.clear();
      for (const box of panel.querySelectorAll('input')) {
        box.checked = false;
      }
      updateLabel();
      onChange();
    }));
    foot.append(button('Done', null, 'button button-small', () => {
      panel.hidden = true;
      b.classList.remove('open');
    }));
    panel.append(foot);
    updateLabel();
    wrap.append(b, panel);
    return wrap;
  }
  function filterControl(sections, onChange) {
    const wrap = el('div', 'facet-wrap');
    const b = el('button', 'facet-button');
    b.type = 'button';
    const labelSpan = el('span', '', 'Filter');
    b.append(svg('filter'), labelSpan, svg('chevron'));
    const panel = el('div', 'facet-panel facet-panel-sections');
    panel.hidden = true;
    const heads = [];
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
    b.addEventListener('click', () => {
      const opening = panel.hidden;
      for (const other of document.querySelectorAll('.facet-panel')) {
        other.hidden = true;
      }
      for (const open of document.querySelectorAll('.facet-button.open')) {
        open.classList.remove('open');
      }
      panel.hidden = !opening;
      b.classList.toggle('open', opening);
    });
    for (const s of sections) {
      const head = el('button', 'facet-section');
      head.type = 'button';
      s.labelSpan = el('span', '', s.label);
      if (s.icon) {
        head.append(svg(s.icon));
      }
      head.append(s.labelSpan, svg('chevron'));
      const body = el('div', 'facet-section-body');
      body.hidden = !s.chosen.size;
      head.classList.toggle('open', !body.hidden);
      head.addEventListener('click', () => {
        body.hidden = !body.hidden;
        head.classList.toggle('open', !body.hidden);
      });
      for (const v of s.values) {
        const {value, label: text} = typeof v === 'string' ? {value: v, label: v} : v;
        const row = el('label', 'facet-option');
        const box = el('input');
        box.type = 'checkbox';
        box.checked = s.chosen.has(value);
        box.addEventListener('change', () => {
          if (box.checked) {
            s.chosen.add(value);
          } else {
            s.chosen.delete(value);
          }
          changed();
        });
        row.append(el('span', '', text), box);
        body.append(row);
      }
      heads.push(head);
      panel.append(head, body);
    }
    const foot = el('div', 'facet-foot');
    foot.append(button('Clear all', null, 'button button-secondary button-small', () => {
      for (const s of sections) {
        s.chosen.clear();
      }
      for (const box of panel.querySelectorAll('input')) {
        box.checked = false;
      }
      changed();
    }));
    foot.append(button('Done', null, 'button button-small', () => {
      panel.hidden = true;
      b.classList.remove('open');
    }));
    panel.append(foot);
    updateLabels();
    wrap.append(b, panel);
    return wrap;
  }

  return {chipToggle, facetDropdown, filterControl};
}

export function rulesEditor({el, svg, options, personName}) {
  function button(label, icon, className, onClick) {
    const node = el('button', className || 'button');
    node.type = 'button';
    if (icon) {
      node.append(svg(icon));
    }
    node.append(el('span', '', label));
    if (onClick) {
      node.addEventListener('click', e => {
        e.preventDefault();
        onClick(e);
      });
    }
    return node;
  }

  function iconButton(icon, label, className, onClick) {
    const node = el('button', 'icon-button ' + (className || ''));
    node.type = 'button';
    node.setAttribute('aria-label', label);
    node.title = label;
    node.append(svg(icon));
    node.addEventListener('click', e => {
      e.preventDefault();
      e.stopPropagation();
      onClick(e);
    });
    return node;
  }

  const {chipToggle, facetDropdown} = filterWidgets({el, svg, button});

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
    const tagValues = [
      ...options().tags.map(t => ({value: t.key, label: t.name, icon: 'tag'})),
      ...options().lists.map(l => ({value: l.key, label: l.name, icon: listIcons[l.kind]})),
    ];
    controls.append(facetDropdown('Tags', 'tag', tagValues, tags, () => {
      rule.tags = [...tags];
      rule.tagLabels = rule.tags.map(t => (tagValues.find(v => v.value === t) || {label: t}).label);
      changed();
    }));
    const family = new Set(rule.family);
    controls.append(facetDropdown('Add family', 'families', options().relations.map(r => ({value: r, label: 'Their ' + r.toLowerCase()})), family, () => {
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

  const listPages = {group: ['loop', '/groups/'], activity: ['team', '/activities/'], party: ['celebrate', '/parties/']};

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

  return {chipToggle, facetDropdown, ruleRow, ruleControls, newRule, ruleSaysSomething, personWords, ruleWords, listIcons};
}
