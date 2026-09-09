// Turns a placeholder element into a searchable, autocompleting person picker so a
// list of 400+ people doesn't get dumped into an unusable native <select>. Returns
// an accessor: setPeople() replaces the searchable pool, value reads the selected
// person's email (empty until a suggestion is actually chosen), and reset() clears
// the typed text and selection after a pick is consumed.
export function createPersonPicker(mountEl) {
  mountEl.classList.add('person-picker');
  const input = document.createElement('input');
  input.type = 'text';
  input.placeholder = 'Search by name or email…';
  input.autocomplete = 'off';
  const list = document.createElement('div');
  list.className = 'person-picker-list';
  list.hidden = true;
  mountEl.append(input, list);

  let people = [];
  let selected = null;
  let activeIndex = -1;

  function matches() {
    const q = input.value.trim().toLowerCase();
    const pool = q ? people.filter(p => p.name.toLowerCase().includes(q) || p.email.toLowerCase().includes(q)) : people;
    return pool.slice(0, 50);
  }

  function close() {
    list.hidden = true;
    list.replaceChildren();
    activeIndex = -1;
  }

  function setActive(index) {
    activeIndex = index;
    [...list.children].forEach((el, i) => el.classList.toggle('active', i === index));
    if (index >= 0) list.children[index].scrollIntoView({block: 'nearest'});
  }

  function open() {
    const found = matches();
    list.replaceChildren();
    if (!found.length) {
      const empty = document.createElement('div');
      empty.className = 'person-picker-empty';
      empty.textContent = 'No matches.';
      list.append(empty);
    } else {
      for (const p of found) {
        const opt = document.createElement('div');
        opt.className = 'person-picker-option';
        opt.textContent = `${p.name} (${p.email})`;
        // mousedown (not click) so this fires before the input's blur handler closes the list.
        opt.addEventListener('mousedown', (e) => {
          e.preventDefault();
          choose(p);
        });
        list.append(opt);
      }
    }
    activeIndex = -1;
    list.hidden = false;
  }

  function choose(p) {
    selected = p;
    input.value = `${p.name} (${p.email})`;
    close();
  }

  input.addEventListener('input', () => {
    selected = null;
    open();
  });
  input.addEventListener('focus', open);
  input.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (list.hidden) { open(); return; }
      setActive(Math.min(activeIndex + 1, list.children.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(Math.max(activeIndex - 1, 0));
    } else if (e.key === 'Enter') {
      const found = matches();
      if (activeIndex >= 0 && found[activeIndex]) {
        e.preventDefault();
        choose(found[activeIndex]);
      }
    } else if (e.key === 'Escape') {
      close();
    }
  });
  input.addEventListener('blur', close);

  return {
    setPeople(newPeople) {
      people = newPeople;
    },
    get value() {
      return selected ? selected.email : '';
    },
    reset() {
      selected = null;
      input.value = '';
    },
  };
}
