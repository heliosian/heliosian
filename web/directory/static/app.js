const state = {people: []};

function hue(text) {
  let h = 0;
  for (const c of text) {
    h = (h * 31 + c.codePointAt(0)) % 360;
  }
  return h;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text) {
    node.textContent = text;
  }
  return node;
}

function card(p) {
  const root = el('div', 'card');
  const avatar = el('div', 'avatar', (p.firstName[0] || '') + (p.lastName[0] || ''));
  avatar.style.background = `hsl(${hue(p.firstName + p.lastName)} 60% 45%)`;
  root.append(avatar);
  const info = el('div');
  info.append(el('div', 'name', `${p.firstName} ${p.lastName}`));
  if (p.pronunciation) {
    info.append(el('div', 'pronunciation', p.pronunciation));
  }
  const who = p.role === 'student' ? `Student, grade ${p.grade}` : 'Parent';
  info.append(el('div', 'detail', `${who} · ${p.familyName}`));
  info.append(el('div', 'detail', p.address));
  const contact = [p.phone || p.familyPhone, p.email].filter(Boolean).join(' · ');
  if (contact) {
    info.append(el('div', 'detail', contact));
  }
  root.append(info);
  return root;
}

function render() {
  const q = document.querySelector('#search').value.trim().toLowerCase();
  const list = document.querySelector('#people');
  list.replaceChildren();
  const matches = state.people.filter(p =>
    `${p.firstName} ${p.lastName} ${p.familyName}`.toLowerCase().includes(q));
  if (matches.length === 0) {
    list.append(el('div', 'empty', 'No matches.'));
    return;
  }
  for (const p of matches) {
    list.append(card(p));
  }
}

async function load() {
  const res = await fetch('/directory/api/people');
  if (!res.ok) {
    throw new Error(`loading people failed: ${res.status}`);
  }
  state.people = await res.json();
  render();
}

document.querySelector('#search').addEventListener('input', render);
load();
