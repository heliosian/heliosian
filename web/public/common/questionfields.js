import {el, svg} from '/elements.js';

export function replyKey(question, person, family) {
  return `${question}|${person || ''}|${family || ''}`;
}

function asks(q, person) {
  if (q.goingOnly && !person.going) {
    return false;
  }
  return q.asked === 'everyone' || (q.asked === 'kids' && person.kid) || (q.asked === 'adults' && !person.kid);
}

export function personQuestions(details, person) {
  return details.questions.filter(q => q.asked !== 'household' && asks(q, person));
}

export function familyQuestions(details) {
  if (!details.family) {
    return [];
  }
  const going = details.household.some(p => p.going);
  return details.questions.filter(q => q.asked === 'household' && (!q.goingOnly || going));
}

function answered(details, q, person, family) {
  const r = details.replies[replyKey(q.id, person, family)];
  return Boolean(r && r.value);
}

export function missingAnswers(details) {
  const missing = [];
  for (const person of details.household) {
    for (const q of personQuestions(details, person).filter(q => q.required && !answered(details, q, person.id, ''))) {
      missing.push(`${q.text} (${person.short})`);
    }
  }
  for (const q of familyQuestions(details).filter(q => q.required && !answered(details, q, '', details.family.id))) {
    missing.push(q.text);
  }
  return missing;
}

function textInput(value, type) {
  const input = el('input');
  input.type = type;
  input.value = value;
  if (type === 'number') {
    input.min = '0';
    input.step = '1';
  }
  return input;
}

function control(q, value, save) {
  if (q.kind === 'yes_no') {
    const wrap = el('div', 'question-choices');
    for (const [word, label] of [['yes', 'Yes'], ['no', 'No']]) {
      const on = value === word;
      const b = el('button', 'question-choice' + (on ? ' is-on' : ''), label);
      b.type = 'button';
      b.setAttribute('aria-pressed', String(on));
      b.addEventListener('click', () => save(on ? '' : word));
      wrap.append(b);
    }
    return wrap;
  }
  if (q.kind === 'choice') {
    const input = el('select');
    for (const [v, label] of [['', 'Choose…'], ...q.choices.map(c => [c, c])]) {
      const option = el('option', '', label);
      option.value = v;
      option.selected = v === value;
      input.append(option);
    }
    input.addEventListener('change', () => save(input.value));
    return input;
  }
  if (q.kind === 'choices') {
    const chosen = new Set(value.split('\n').filter(Boolean));
    const wrap = el('div', 'question-checks');
    for (const c of q.choices) {
      const label = el('label', 'question-check');
      const box = el('input');
      box.type = 'checkbox';
      box.checked = chosen.has(c);
      box.addEventListener('change', () => save(q.choices.filter(x => (x === c ? box.checked : chosen.has(x))).join('\n')));
      label.append(box, el('span', '', c));
      wrap.append(label);
    }
    return wrap;
  }
  const input = textInput(value, q.kind === 'number' ? 'number' : 'text');
  input.addEventListener('change', () => save(input.value.trim()));
  return input;
}

let justSaved = '';

function savedMark() {
  const mark = el('span', 'question-saved');
  mark.append(svg('check'), el('span', '', 'Saved'));
  return mark;
}

export function questionField(q, target, details, redraw) {
  const key = replyKey(q.id, target.person, target.family);
  const existing = details.replies[key] || null;
  const value = existing ? existing.value : '';
  const wrap = el('div', 'question' + (q.required ? ' is-required' : '') + (q.required && !value ? ' is-missing' : ''));
  const status = el('span', 'question-status');
  if (justSaved === key) {
    justSaved = '';
    status.append(savedMark());
  }
  const save = async next => {
    status.replaceChildren('Saving…');
    try {
      await details.save(q, target, next, existing);
      justSaved = key;
      await redraw();
    } catch (err) {
      justSaved = '';
      status.replaceChildren(err.message);
    }
  };
  wrap.append(el('span', 'question-text', q.text), control(q, value, save), status);
  return wrap;
}

export function personBlock(details, person, redraw) {
  const asked = personQuestions(details, person);
  if (!asked.length) {
    return null;
  }
  const block = el('div', 'question-block');
  block.append(...asked.map(q => questionField(q, {person: person.id}, details, redraw)));
  return block;
}

export function familyBlock(details, redraw) {
  const asked = familyQuestions(details);
  if (!asked.length) {
    return null;
  }
  const block = el('div', 'question-block question-family');
  block.append(el('h3', 'question-family-title', `For ${details.family.name}`));
  block.append(...asked.map(q => questionField(q, {family: details.family.id}, details, redraw)));
  return block;
}

export function replyRows(details, rowOf, redraw) {
  const nodes = [];
  for (const person of details.household) {
    nodes.push(rowOf(person));
    const block = personBlock(details, person, redraw);
    if (block) {
      nodes.push(block);
    }
  }
  for (const node of [familyBlock(details, redraw), missingNote(details)]) {
    if (node) {
      nodes.push(node);
    }
  }
  return nodes;
}

export function missingNote(details) {
  const missing = missingAnswers(details);
  if (!missing.length) {
    return null;
  }
  const note = el('div', 'question-missing');
  note.append(el('span', 'question-missing-title', 'Still to answer'), el('span', '', missing.join(' · ')));
  return note;
}
