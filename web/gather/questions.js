import {el, button, iconButton} from '/elements.js';
import {openModal, popup} from '/modal.js';
import {field, text, textarea, select, checkbox} from '/form.js';
import {addQuestion, editQuestion, removeQuestion} from './state.js';

const kinds = [
  {value: 'text', label: 'Words'},
  {value: 'choice', label: 'One of some choices'},
  {value: 'choices', label: 'Any of some choices'},
  {value: 'number', label: 'A number'},
  {value: 'yes_no', label: 'Yes or no'},
];

const askedOf = [
  {value: 'everyone', label: 'Every guest'},
  {value: 'adults', label: 'Adults'},
  {value: 'kids', label: 'Kids'},
  {value: 'household', label: 'Once per family'},
];

export function hostQuestions(inv, details) {
  const panel = el('section', 'side-card questions-panel');
  if (!details.questions.length) {
    panel.append(el('p', 'hint', 'Ask guests something as they answer: allergies, a shirt size, whether they can help.'));
  }
  const list = el('ol', 'question-list');
  for (const q of details.questions) {
    const row = el('li', 'question-row');
    const words = el('span', 'question-row-words');
    words.append(el('span', 'question-row-text', q.text), el('span', 'question-row-meta', [
      askedOf.find(a => a.value === q.asked).label, kinds.find(k => k.value === q.kind).label, q.required ? 'Required' : '', q.goingOnly ? 'If going' : '',
    ].filter(Boolean).join(' · ')));
    row.append(words, iconButton('edit', 'Edit question', 'question-edit', () => openQuestion(inv, details, q)));
    list.append(row);
  }
  const actions = el('div', 'host-actions');
  actions.append(button('Add a question', 'plus', 'button button-secondary', () => openQuestion(inv, details, null)));
  if (details.questions.length) {
    actions.append(button('See answers', 'list', 'button button-secondary', () => showAnswers(details)));
  }
  panel.append(list, actions);
  return panel;
}

function openQuestion(inv, details, q) {
  const words = text(q ? q.text : '', {required: true, placeholder: 'Any food allergies?'});
  const kind = select(kinds, q ? q.kind : 'text');
  const choices = textarea(q ? q.choices.join('\n') : '', 4);
  choices.placeholder = 'One choice to a line';
  const choicesField = field('Choices', choices, 'One to a line.');
  const showChoices = () => {
    choicesField.hidden = !['choice', 'choices'].includes(kind.value);
  };
  kind.addEventListener('change', showChoices);
  showChoices();
  const asked = select(askedOf, q ? q.asked : 'everyone');
  const required = checkbox('Required', q ? q.required : false, 'Guests are shown it as still to answer until they do.');
  const goingOnly = checkbox('Only if going', q ? q.goingOnly : true, 'Asked only of guests who say yes.');
  const body = () => {
    const list = choices.value.split('\n').map(c => c.trim()).filter(Boolean);
    if (['choice', 'choices'].includes(kind.value) && list.length < 2) {
      throw new Error('Give at least two choices, one to a line.');
    }
    return {text: words.value.trim(), kind: kind.value, choices: list.join('\n'), asked: asked.value, required: required.input.checked, goingOnly: goingOnly.input.checked};
  };
  const last = details.questions.length ? details.questions[details.questions.length - 1].order : undefined;
  openModal(q ? 'Edit question' : 'Add a question', [
    field('Question', words, '', true),
    field('Answered with', kind),
    choicesField,
    field('Asked of', asked, 'Kids are students, and guests from outside marked as children.'),
    required.wrap,
    goingOnly.wrap,
  ], {
    saveLabel: q ? 'Save' : 'Add question',
    submit: () => (q ? editQuestion(q, body()) : addQuestion(inv, body(), last)),
    onDelete: q ? () => removeQuestion(q) : null,
    deleteLabel: 'Remove question',
    confirmDelete: 'Remove this question and every answer to it?',
  });
}

function showAnswers(details) {
  const node = el('div', 'answers-view');
  for (const q of details.questions) {
    const block = el('section', 'answers-question');
    block.append(el('h3', 'answers-question-text', q.text));
    const rows = Object.values(details.replies).filter(r => r.question === q.id);
    if (!rows.length) {
      block.append(el('p', 'hint', 'No answers yet.'));
    }
    const list = el('ul', 'answers-list');
    for (const r of rows) {
      const li = el('li', 'answers-row');
      li.append(el('span', 'answers-who', details.names[r.person || r.family] || 'Someone'), el('span', 'answers-value', r.value.split('\n').join(', ')));
      list.append(li);
    }
    block.append(list);
    node.append(block);
  }
  popup('Answers', node, {wide: true});
}
