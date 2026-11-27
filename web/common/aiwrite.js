import {el, button} from '/elements.js';
import {api} from '/api.js';

export function writeWithAI(box, path, ask) {
  const status = el('small', 'describe-status');
  const generate = button('Write with AI', 'sparkle', 'button button-small button-secondary', async () => {
    generate.disabled = true;
    status.classList.remove('error');
    status.textContent = 'Writing…';
    try {
      const {description} = await api('POST', path, {...ask(), notes: box.value});
      box.value = description;
      box.dispatchEvent(new Event('input', {bubbles: true}));
      status.textContent = '';
      box.focus();
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
    } finally {
      generate.disabled = false;
    }
  });
  const line = el('div', 'describe-line');
  line.append(generate, status);
  return line;
}

export function writeGuestWords(box, group, kind, heading) {
  return writeWithAI(box, '/api/do/describe-guest-words', () => ({group, kind, heading: heading()}));
}
