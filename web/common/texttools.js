import {el, button} from '/elements.js';
import {openModal, closeModal} from '/modal.js';
import {field, text as textInput} from '/form.js';

function replaceRange(textarea, start, end, text, selectStart, selectEnd) {
  textarea.focus();
  textarea.setSelectionRange(start, end);
  document.execCommand('insertText', false, text);
  textarea.setSelectionRange(start + selectStart, start + selectEnd);
}

function wrapSelection(textarea, mark) {
  const {selectionStart: start, selectionEnd: end, value} = textarea;
  const words = value.slice(start, end);
  replaceRange(textarea, start, end, mark + words + mark, mark.length, mark.length + words.length);
}

function selectedLines(textarea) {
  const {selectionStart, selectionEnd, value} = textarea;
  const start = value.lastIndexOf('\n', selectionStart - 1) + 1;
  const last = selectionEnd > selectionStart && value[selectionEnd - 1] === '\n' ? selectionEnd - 1 : selectionEnd;
  const newline = value.indexOf('\n', last);
  const end = newline === -1 ? value.length : newline;
  return {start, end, lines: value.slice(start, end).split('\n')};
}

export function prefixLines(textarea, first, prefix, blank) {
  const {start, end, lines} = selectedLines(textarea);
  const text = first + (lines.length === 1 && !lines[0] ? prefix : lines.map(line => line ? prefix + line : blank).join('\n'));
  replaceRange(textarea, start, end, text, text.length, text.length);
}

const listItem = /^\s*([-*]|\d+[.)])\s+/;

export function indentLists(textarea) {
  textarea.addEventListener('keydown', e => {
    if (e.key !== 'Tab' || e.ctrlKey || e.metaKey || e.altKey) {
      return;
    }
    const {start, end, lines} = selectedLines(textarea);
    if (!lines.some(line => listItem.test(line))) {
      return;
    }
    e.preventDefault();
    const shifted = lines.map(line => {
      if (!listItem.test(line)) {
        return line;
      }
      return e.shiftKey ? line.replace(/^( {1,2}|\t)/, '') : '  ' + line;
    });
    const text = shifted.join('\n');
    if (lines.length === 1 && textarea.selectionStart === textarea.selectionEnd) {
      const caret = Math.max(0, textarea.selectionStart - start + text.length - lines[0].length);
      replaceRange(textarea, start, end, text, caret, caret);
      return;
    }
    replaceRange(textarea, start, end, text, 0, text.length);
  });
}

function linkPopup(textarea) {
  const {selectionStart: start, selectionEnd: end, value} = textarea;
  const words = textInput(value.slice(start, end), {placeholder: 'What the link says', required: true});
  const address = textInput('', {type: 'url', placeholder: 'https://', required: true});
  openModal('Add Link', [field('Words', words), field('Address', address)], {
    saveLabel: 'Add Link',
    stay: true,
    submit: async () => {
      const markdown = `[${words.value.trim()}](${address.value.trim()})`;
      closeModal();
      replaceRange(textarea, start, end, markdown, markdown.length, markdown.length);
    },
  });
  if (words.value) {
    address.focus();
  }
}

export function textTools(textarea, ...more) {
  const tool = (icon, label, onClick) => {
    const node = button('', icon, 'icon-button', onClick);
    node.setAttribute('aria-label', label);
    node.title = label;
    return node;
  };
  const bar = el('div', 'text-tools');
  bar.append(
    tool('bold', 'Bold', () => wrapSelection(textarea, '**')),
    tool('italic', 'Italic', () => wrapSelection(textarea, '*')),
    tool('list', 'List', () => prefixLines(textarea, '', '- ', '')),
    tool('link', 'Link', () => linkPopup(textarea)),
    ...more,
  );
  return bar;
}
