import {el} from '/elements.js';
import {api} from '/api.js';
import {listed, emailOf, wordsOf} from '/directory.js';

const pages = [['resources', '/resources'], ['query', '/query'], ['search', '/search'], ['policies', '/policies'], ['erd', '/erd']];
const systemPages = [['dashboard', '/'], ['queues', '/queues']];

function navLink(current, name, href) {
  const a = el('a', name === current ? 'here' : '', name);
  a.href = href;
  return a;
}

export function chrome(current) {
  const header = document.querySelector('header');
  const bar = el('div', 'bar');
  const brand = el('a', 'brand');
  brand.href = '/';
  brand.append(el('span', 'mark', '◆'), ' admin');
  const nav = el('nav');
  for (const [name, href] of pages) {
    nav.append(navLink(current, name, href));
  }
  api('GET', '/api/system').then(system => {
    if (system.allowed) {
      nav.prepend(...systemPages.map(([name, href]) => navLink(current, name, href)));
    }
  }).catch(err => nav.append(el('span', 'error', err.message)));
  const spoof = el('div', 'spoof');
  bar.append(brand, nav, spoof);
  header.prepend(bar);
  initSpoof(spoof).catch(err => spoof.append(el('span', 'error', err.message)));
}

export function query(tree) {
  return api('QUERY', '/api/q', tree);
}

export async function tables() {
  const spec = await api('GET', '/api/openapi.json');
  const out = [];
  for (const [name, s] of Object.entries(spec.components.schemas)) {
    if (!s['x-columns']) {
      continue;
    }
    out.push({
      name,
      sheet: s['x-generated'] ? 'generated' : s['x-sheet'],
      appendOnly: s['x-appendOnly'],
      description: s.description ?? '',
      columns: s['x-columns'].map(c => ({name: c, kind: s.properties[c]['x-kind'], relation: s.properties[c]['x-relation'] ?? '', schema: s.properties[c]})),
    });
  }
  return out;
}

export function labelOf(row) {
  return row.name_show || row.name || row.address || row.subject || row.slug || row.key || row.id;
}

async function setSpoof(email) {
  await api('POST', '/auth/spoof', {email});
  location.reload();
}

async function initSpoof(box) {
  const state = await api('GET', '/auth/spoof');
  if (!state.canSpoof) {
    return;
  }
  const open = el('button', '', state.spoofing ? `viewing as ${state.spoofing.name}` : 'view as…');
  open.type = 'button';
  box.append(open);
  if (state.spoofing) {
    box.classList.add('on');
    const stop = el('button', 'stop', '×');
    stop.type = 'button';
    stop.title = 'Stop viewing as ' + state.spoofing.name;
    stop.addEventListener('click', () => setSpoof(''));
    box.append(stop);
  }
  const menu = el('div', 'spoof-menu');
  menu.hidden = true;
  const search = el('input');
  search.type = 'search';
  search.placeholder = 'name or email';
  const results = el('div');
  menu.append(search, results);
  box.append(menu);
  const row = (name, email, words) => {
    const b = el('button', 'person');
    b.type = 'button';
    b.append(el('span', '', name), el('span', 'words', words || email));
    b.addEventListener('click', () => setSpoof(email));
    return b;
  };
  let people;
  const show = () => {
    const q = search.value.trim().toLowerCase();
    if (!q) {
      results.replaceChildren(...(state.recent.length ? [el('div', 'section', 'recent'), ...state.recent.map(p => row(p.name, p.email, ''))] : []));
      return;
    }
    if (!people) {
      results.replaceChildren(el('div', 'empty', 'loading…'));
      return;
    }
    const found = people.filter(p => [p.name_show, emailOf(p), wordsOf(p)].some(v => v.toLowerCase().includes(q))).slice(0, 10);
    results.replaceChildren(...(found.length ? found.map(p => row(p.name_show, emailOf(p), wordsOf(p))) : [el('div', 'empty', 'nobody matches')]));
  };
  open.addEventListener('click', e => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
    if (menu.hidden) {
      return;
    }
    show();
    search.focus();
    if (!people) {
      listed().then(list => {
        people = list;
        show();
      }).catch(err => results.replaceChildren(el('div', 'error', err.message)));
    }
  });
  search.addEventListener('input', show);
  search.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      results.querySelector('.person')?.click();
    }
  });
  menu.addEventListener('click', e => e.stopPropagation());
  document.addEventListener('click', () => {
    menu.hidden = true;
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      menu.hidden = true;
    }
  });
}
