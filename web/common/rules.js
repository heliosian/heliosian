// The rule editor every app shares: a filter rule as Helios Who?'s filters
// have it - roles, words, classrooms, grades, tags, the relatives to add -
// as a row that reads out as a sentence and opens to its controls, the
// same in Loop's group editor and Heliosian's Visibility. The server reads
// the rules through internal/filter; this is the one way to write them.
//
// rulesEditor takes what differs by app: el and svg (its dom helpers, with
// the icons groups, user-minus, edit, trash, check, chevron, tag, families,
// party, activity and classrooms), options() (the choices, as
// /api/<app>/…/options answers), me() (the signed-in person, whose tags a
// new rule reads), and personName(email) (a name for an address the
// directory holds, or nothing). It gives back the pieces.
// plural is a role as a chip and a sentence say it: Students, Parents, Staff.
function plural(role) {
  return role === 'Staff' ? 'Staff' : role + 's';
}

export function rulesEditor({el, svg, options, me, personName}) {
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

  function chipToggle(label, on, onChange) {
    const b = el('button', 'chip-toggle' + (on ? ' active' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      b.classList.toggle('active');
      onChange(b.classList.contains('active'));
    });
    return b;
  }

  // facetDropdown is a button opening a checklist, as Who?'s Grade and
  // Classroom dropdowns are: values are strings or {value, label, icon}.
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
    foot.append(button('Done', null, 'button button-small', () => {
      panel.hidden = true;
      b.classList.remove('open');
    }));
    panel.append(foot);
    updateLabel();
    wrap.append(b, panel);
    return wrap;
  }

  const listIcons = {party: 'party', activity: 'activity', room: 'classrooms'};

  // ruleRow is one rule in the editor. Read, it is the sentence it makes -
  // "Parents in Grade 4" - with a pencil and the way to drop it; opened, its
  // facets as controls with Done to close it, which a new rule starts as. A
  // rule another manager wrote is read and never opened: its tags are theirs.
  function ruleRow(rule, onChange, onRemove, open, countChip) {
    const row = el('div', 'rule');
    const mine = rule.owner === me().email;
    // Read: a tinted band with the kind's label and mark, the sentence, and
    // how many people the rule touches once the preview has said - asked
    // afresh each time the band is drawn, and again when the preview answers.
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
      line.append(el('span', 'rule-line-words', ruleWords(rule)));
      if (countChip) {
        count = countChip(rule);
        line.append(count);
      }
      words.append(line);
      if (!mine) {
        row.classList.add('is-theirs');
        words.append(el('div', 'rule-note', `Written by ${personName(rule.owner) || rule.owner}, reading their tags; it can be removed but not changed.`));
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
      // Done closes the rule to its sentence; a rule that still says nothing
      // is dropped instead of kept empty.
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

  // ruleControls is a rule's facets as controls, with the rule read out under
  // them as it stands, so a choice reads back as the sentence it makes.
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
      }));
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
      ...options().tags,
      ...options().shared.map(s => ({value: s.key, label: `${s.name} (${s.ownerName}'s)`, icon: 'tag'})),
      ...options().lists.map(l => ({value: l.key, label: l.name, icon: listIcons[l.kind]})),
    ];
    controls.append(facetDropdown('Tags', 'tag', tagValues, tags, () => {
      rule.tags = [...tags];
      rule.tagLabels = rule.tags.map(t => (tagValues.find(v => (typeof v === 'string' ? v : v.value) === t) || {label: t}).label || t);
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
    return {kind, roles: [], search: '', classrooms: [], grades: [], tags: [], family: [], owner: me().email};
  }

  function ruleSaysSomething(r) {
    return r.roles.length || r.search || r.classrooms.length || r.grades.length || r.tags.length;
  }

  const singular = {Student: 'student', Parent: 'parent', Staff: 'staff member'};

  // personWords is a rule said of one person who matches it, lowercase, for
  // a member's reason: "parent in Grade 5 or Grade 6", "tagged in Tech Team".
  function personWords(r) {
    // A rule that is one address and nothing else names them.
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

  // ruleWords is a rule as the group's page reads it out.
  function ruleWords(r) {
    // A rule that is one address and nothing else - as excluding someone
    // from the member list makes - reads as the person.
    if (r.search && r.search.includes('@') && !r.roles.length && !r.grades.length && !r.classrooms.length && !r.tags.length) {
      const name = personName(r.search);
      if (name) {
        return `${name} (${r.search})`;
      }
    }
    // With Add family the roles keep their kind after the widening, as Who?
    // reads the same choices, so they are said last: "Anyone tagged in
    // Carpool, plus their parents, keeping parents only".
    const roles = r.roles.map(plural).join(' or ');
    const parts = [];
    if (r.roles.length && !r.family.length) {
      parts.push(roles);
    } else {
      parts.push('Anyone');
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
    let words = parts.join(' ');
    if (r.family.length) {
      words += ', plus their ' + r.family.map(f => f.toLowerCase()).join(' and ');
      if (r.roles.length) {
        words += `, keeping ${roles.toLowerCase()} only`;
      }
    }
    return words;
  }

  return {chipToggle, facetDropdown, ruleRow, ruleControls, newRule, ruleSaysSomething, personWords, ruleWords, listIcons};
}
