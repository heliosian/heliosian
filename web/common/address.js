import {api} from '/api.js';

export function addressSuggest(input) {
  if (!input || input.dataset.suggest) {
    return input;
  }
  input.dataset.suggest = '1';
  input.setAttribute('autocomplete', 'off');
  const list = document.createElement('div');
  list.className = 'address-list';
  list.hidden = true;
  const mount = () => {
    if (list.isConnected || !input.parentNode) {
      return;
    }
    const wrap = document.createElement('div');
    wrap.className = 'address-wrap';
    input.parentNode.insertBefore(wrap, input);
    wrap.append(input, list);
  };
  let items = [];
  let active = -1;
  let timer = null;
  let asked = '';
  let controller = null;
  const close = () => {
    list.hidden = true;
    list.replaceChildren();
    items = [];
    active = -1;
  };
  const paint = () => {
    list.replaceChildren();
    items.forEach((text, i) => {
      const row = document.createElement('button');
      row.type = 'button';
      row.className = 'address-item' + (i === active ? ' is-active' : '');
      row.textContent = text;
      row.addEventListener('mousedown', ev => {
        // Before the input's blur, so the pick lands.
        ev.preventDefault();
        pick(i);
      });
      list.append(row);
    });
    list.hidden = !items.length;
  };
  const pick = i => {
    if (i < 0 || i >= items.length) {
      return;
    }
    input.value = items[i];
    asked = input.value;
    input.dispatchEvent(new Event('input', {bubbles: true}));
    input.dispatchEvent(new Event('change', {bubbles: true}));
    close();
  };
  const ask = async () => {
    const q = input.value.trim();
    if (q.length < 3 || q === asked) {
      if (q.length < 3) {
        close();
      }
      return;
    }
    asked = q;
    if (controller) {
      controller.abort();
    }
    controller = new AbortController();
    try {
      const got = await api('GET', '/api/address/suggest?q=' + encodeURIComponent(q), undefined, controller.signal);
      if (input.value.trim() !== q) {
        return;
      }
      items = got.map(s => s.text).filter(t => t && t !== q);
      active = -1;
      paint();
    } catch (err) {
      if (err.name !== 'AbortError') {
        close();
      }
    }
  };
  setTimeout(mount, 0);
  input.addEventListener('focus', mount, {once: true});
  input.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(ask, 220);
  });
  input.addEventListener('keydown', ev => {
    if (list.hidden) {
      return;
    }
    if (ev.key === 'ArrowDown') {
      ev.preventDefault();
      active = (active + 1) % items.length;
      paint();
    } else if (ev.key === 'ArrowUp') {
      ev.preventDefault();
      active = (active - 1 + items.length) % items.length;
      paint();
    } else if (ev.key === 'Enter' && active >= 0) {
      ev.preventDefault();
      pick(active);
    } else if (ev.key === 'Escape') {
      close();
    }
  });
  input.addEventListener('blur', () => setTimeout(close, 120));
  return input;
}
