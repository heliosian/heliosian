import {el, button} from '/elements.js';

const emailIn = /[^\s<>,;:"'()[\]]+@[^\s<>,;:"'()[\]]+\.[^\s<>,;:"'()[\]]+/;
const emailsIn = new RegExp(emailIn.source, 'g');

function tidyName(words) {
  return words.replace(/[<>"'()[\]]/g, ' ').replace(/\s+/g, ' ').replace(/^[\s,;:-]+|[\s,;:-]+$/g, '').trim();
}

function unfold(text) {
  return text.replace(/\r\n/g, '\n').replace(/\n[ \t]/g, '');
}

function fromVCards(text) {
  const out = [];
  for (const card of unfold(text).split(/BEGIN:VCARD/i).slice(1)) {
    const lines = card.split('\n');
    const value = key => {
      const line = lines.find(l => new RegExp(`^(item\\d+\\.)?${key}[;:]`, 'i').test(l));
      return line ? line.slice(line.indexOf(':') + 1).trim() : '';
    };
    const email = (value('EMAIL').match(emailIn) || [''])[0];
    let name = value('FN');
    if (!name) {
      const [last = '', first = ''] = value('N').split(';');
      name = `${first} ${last}`.trim();
    }
    out.push({name: tidyName(name.replace(/\\,/g, ',')), email});
  }
  return out;
}

function csvRows(text) {
  const rows = [];
  let row = [];
  let cell = '';
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (c === '"') {
        quoted = false;
      } else {
        cell += c;
      }
      continue;
    }
    if (c === '"') {
      quoted = true;
    } else if (c === ',' || c === '\t') {
      row.push(cell);
      cell = '';
    } else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') {
        i++;
      }
      row.push(cell);
      rows.push(row);
      row = [];
      cell = '';
    } else {
      cell += c;
    }
  }
  row.push(cell);
  rows.push(row);
  return rows.filter(r => r.some(c => c.trim()));
}

function fromTable(text) {
  const rows = csvRows(text);
  const head = rows[0].map(c => c.trim().toLowerCase());
  const col = test => head.findIndex(test);
  const email = col(h => h.includes('mail') && !h.includes('type') && !h.includes('label'));
  if (email < 0) {
    return null;
  }
  const full = col(h => h === 'name' || h === 'full name' || h === 'display name');
  const first = col(h => h.startsWith('first') || h === 'given name');
  const last = col(h => h.startsWith('last') || h === 'family name' || h === 'surname');
  return rows.slice(1).map(r => {
    const name = full >= 0 ? r[full] || '' : `${first >= 0 ? r[first] || '' : ''} ${last >= 0 ? r[last] || '' : ''}`;
    return {name: tidyName(name), email: ((r[email] || '').match(emailIn) || [''])[0]};
  });
}

function fromText(text) {
  const out = [];
  for (const line of text.split(/\r?\n/)) {
    const found = [...line.matchAll(emailsIn)];
    if (!found.length) {
      if (tidyName(line)) {
        out.push({name: tidyName(line), email: ''});
      }
      continue;
    }
    let from = 0;
    found.forEach((m, i) => {
      const next = found[i + 1] ? found[i + 1].index : line.length;
      const before = line.slice(from, m.index);
      const after = found.length === 1 ? line.slice(m.index + m[0].length, next) : '';
      out.push({name: tidyName(before) || tidyName(after), email: m[0]});
      from = m.index + m[0].length;
    });
  }
  return out;
}

export function parseContacts(text) {
  if (/BEGIN:VCARD/i.test(text)) {
    return fromVCards(text);
  }
  return fromTable(text) || fromText(text);
}

export function importAdder({isOn, onAdd, note}) {
  const wrap = el('div', 'import-adder');
  wrap.append(el('div', 'picker-note', note));
  const text = el('textarea', 'import-text');
  text.rows = 6;
  text.placeholder = 'Jamie Rivera <jamie@example.com>\nPat Chen, pat@example.com\n\nor paste a column from a spreadsheet';
  const file = el('input');
  file.type = 'file';
  file.accept = '.csv,.tsv,.txt,.vcf,text/csv,text/vcard';
  file.hidden = true;
  const pick = button('Choose a file', 'upload', 'button button-secondary button-small', () => file.click());
  const source = el('div', 'import-source');
  source.append(pick, el('span', 'picker-note', 'A spreadsheet saved as CSV, or contacts exported as a .vcf card.'), file);
  const list = el('div', 'import-list');
  const status = el('span', 'save-status');
  let rows = [];
  const paint = () => {
    list.replaceChildren();
    if (!rows.length) {
      return;
    }
    const seen = new Set();
    for (const r of rows) {
      const key = r.email.toLowerCase();
      r.problem = !r.email ? 'No email address' : isOn(r.email) ? 'On the list already' : seen.has(key) ? 'Listed twice' : '';
      seen.add(key);
      if (r.on === undefined) {
        r.on = !r.problem;
      }
      const row = el('label', 'import-row' + (r.problem ? ' is-off' : ''));
      const box = el('input');
      box.type = 'checkbox';
      box.checked = r.on && !r.problem;
      box.disabled = Boolean(r.problem);
      box.addEventListener('change', () => {
        r.on = box.checked;
        count();
      });
      const words = el('span', 'import-words');
      words.append(el('strong', '', r.name || r.email || 'Nobody'), el('small', '', r.problem || (r.name ? r.email : 'No name: the address stands in for one')));
      row.append(box, words);
      list.append(row);
    }
    count();
  };
  const add = button('Add to the list', 'plus', 'button', async () => {
    const people = rows.filter(r => r.on && !r.problem).map(r => ({name: r.name, email: r.email, via: 'outside', household: r.email}));
    if (!people.length) {
      status.textContent = 'Paste some names and email addresses, or choose a file.';
      status.classList.add('error');
      return;
    }
    add.disabled = true;
    status.classList.remove('error');
    status.textContent = '';
    try {
      await onAdd(people);
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
    }
    add.disabled = false;
  });
  const count = () => {
    const n = rows.filter(r => r.on && !r.problem).length;
    add.lastChild.textContent = n ? `Add ${n} to the list` : 'Add to the list';
  };
  const read = raw => {
    rows = parseContacts(raw);
    paint();
  };
  text.addEventListener('input', () => read(text.value));
  file.addEventListener('change', async () => {
    const f = file.files[0];
    if (!f) {
      return;
    }
    text.value = await f.text();
    read(text.value);
  });
  const foot = el('div', 'modal-actions');
  foot.append(add, status);
  wrap.append(text, source, list, foot);
  wrap.focus = () => text.focus();
  return wrap;
}
