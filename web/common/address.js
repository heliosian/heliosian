// addressSuggest attaches address suggestions to a text input, the way a
// map app's search box offers them: as the person types, the app's
// /api/address/suggest is asked - Google's Places behind it - and whole
// addresses drop down under the box; an arrow key or a click takes one,
// Escape or a click elsewhere leaves the typing as it is. Nothing about
// the box changes otherwise - what is typed by hand is what is saved.

export function addressSuggest(input) {
  if (!input || input.dataset.suggest) {
    return input;
  }
  input.dataset.suggest = '1';
  input.setAttribute('autocomplete', 'off');
  // The box may not be in the page yet when it is handed over: the list
  // is put beside it once it is - on the next tick, and at the latest
  // when it first takes focus, before anything is typed (moving a box
  // with focus in it would lose the focus).
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
      const res = await fetch('/api/address/suggest?q=' + encodeURIComponent(q), {signal: controller.signal});
      if (!res.ok) {
        return;
      }
      const got = await res.json();
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
