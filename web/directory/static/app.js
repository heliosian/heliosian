const state = {model: null};

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

function initials(name) {
  const words = name.trim().split(/\s+/);
  if (words.length === 0 || !words[0]) {
    return '?';
  }
  const first = words[0][0];
  return words.length > 1 ? first + words[words.length - 1][0] : first;
}

function roles(p) {
  return [p.isStudent && 'Student', p.isParent && 'Parent', p.isStaff && 'Staff'].filter(Boolean);
}

function card(p, family) {
  const root = el('div', 'card');
  if (p.photoUrl) {
    const img = el('img', 'avatar');
    img.src = p.photoUrl;
    img.loading = 'lazy';
    root.append(img);
  } else {
    const avatar = el('div', 'avatar', initials(p.fullName));
    avatar.style.background = `hsl(${hue(p.fullName)} 60% 45%)`;
    root.append(avatar);
  }
  const info = el('div');
  info.append(el('div', 'name', p.fullName));
  if (p.pronouns) {
    info.append(el('div', 'pronouns', p.pronouns));
  }
  const who = [roles(p).join(' / ')];
  if (p.grade) {
    who.push([p.grade, p.classroom, p.section].filter(Boolean).join(' ▶ '));
  }
  if (p.jobTitle) {
    who.push(p.jobTitle);
  }
  info.append(el('div', 'detail', who.join(' · ')));
  if (family && family.name) {
    info.append(el('div', 'detail', family.name));
  }
  if (family && family.address) {
    info.append(el('div', 'detail', family.address));
  }
  const contact = [p.phone, p.email].filter(Boolean).join(' · ');
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
  if (!state.model) {
    return;
  }
  const matches = state.model.people.filter(p => {
    const family = state.model.families[p.familyKey];
    return `${p.fullName} ${family ? family.name : ''}`.toLowerCase().includes(q);
  });
  if (matches.length === 0) {
    list.append(el('div', 'empty', 'No matches.'));
    return;
  }
  for (const p of matches) {
    list.append(card(p, state.model.families[p.familyKey]));
  }
}

async function load() {
  const res = await fetch('/directory/api/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  state.model = await res.json();
  render();
}

document.querySelector('#search').addEventListener('input', render);
load();
