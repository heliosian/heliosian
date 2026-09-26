function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

function face(person) {
  const node = el('span', 'person-picker-face');
  if (person.photoUrl) {
    const img = el('img');
    img.src = person.photoUrl;
    img.alt = '';
    img.loading = 'lazy';
    node.append(img);
    return node;
  }
  node.textContent = (person.name || person.email || '?').slice(0, 1).toUpperCase();
  return node;
}

export function createPersonPicker(mountEl, {people, placeholder = 'Search by name or email…', allow, onPick, address = false}) {
  mountEl.classList.add('person-picker');
  const input = el('input');
  input.type = 'text';
  input.placeholder = placeholder;
  input.autocomplete = 'off';
  const list = el('div', 'person-picker-list');
  list.hidden = true;
  mountEl.append(input, list);

  let selected = null;
  let found = [];
  let activeIndex = -1;
  let asked = 0;

  const typed = () => {
    const q = input.value.trim().toLowerCase();
    return address && q.includes('@') ? q : '';
  };

  function close() {
    list.hidden = true;
    list.replaceChildren();
    found = [];
    activeIndex = -1;
  }

  function setActive(index) {
    activeIndex = index;
    [...list.children].forEach((row, i) => row.classList.toggle('active', i === index));
    if (index >= 0) {
      list.children[index].scrollIntoView({block: 'nearest'});
    }
  }

  function row(person) {
    const option = el('div', 'person-picker-option');
    const words = el('span', 'person-picker-words');
    words.append(el('span', 'person-picker-name', person.name || person.email), el('span', 'person-picker-line', person.title || person.email));
    option.append(face(person), words);
    // mousedown, not click, so it lands before the input's blur closes the list.
    option.addEventListener('mousedown', e => {
      e.preventDefault();
      choose(person);
    });
    return option;
  }

  function note(text) {
    list.replaceChildren(el('div', 'person-picker-empty', text));
  }

  async function open() {
    const ask = ++asked;
    list.hidden = false;
    let pool;
    try {
      pool = await people();
    } catch (err) {
      if (ask !== asked) {
        return;
      }
      const retry = el('button', 'link-button', 'Try again');
      retry.type = 'button';
      retry.addEventListener('mousedown', e => {
        e.preventDefault();
        open();
      });
      const failed = el('div', 'person-picker-empty', `Couldn’t load the directory: ${err.message} `);
      failed.append(retry);
      list.replaceChildren(failed);
      return;
    }
    if (ask !== asked) {
      return;
    }
    const q = input.value.trim().toLowerCase();
    found = pool.filter(p => (!allow || allow(p)) && (!q || p.name.toLowerCase().includes(q) || p.email.toLowerCase().includes(q))).slice(0, 50);
    activeIndex = -1;
    if (!found.length) {
      note(typed() ? `Nobody in the directory - “${typed()}” will be used as typed.` : 'Nobody matches.');
      return;
    }
    list.replaceChildren(...found.map(row));
  }

  function choose(person) {
    if (onPick) {
      input.value = '';
      close();
      onPick(person);
      return;
    }
    selected = person;
    input.value = person.name || person.email;
    close();
  }

  input.addEventListener('input', () => {
    selected = null;
    open();
  });
  input.addEventListener('focus', open);
  input.addEventListener('keydown', e => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (list.hidden) {
        open();
        return;
      }
      setActive(Math.min(activeIndex + 1, found.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(Math.max(activeIndex - 1, 0));
    } else if (e.key === 'Enter') {
      if (activeIndex >= 0 && found[activeIndex]) {
        e.preventDefault();
        choose(found[activeIndex]);
      } else if (onPick && typed()) {
        e.preventDefault();
        choose({email: typed(), name: typed()});
      }
    } else if (e.key === 'Escape') {
      close();
    }
  });
  input.addEventListener('blur', close);

  return {
    mount: mountEl,
    input,
    get value() {
      return selected ? selected.email : typed();
    },
    get person() {
      return selected;
    },
    reset() {
      selected = null;
      input.value = '';
    },
  };
}
