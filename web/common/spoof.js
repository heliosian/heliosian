import {offerQuan} from '/mode.js';
import {api} from '/api.js';
import {listed, emailOf, wordsOf} from '/directory.js';
import {el, toast} from '/elements.js';
import {hoverMenu, hoverClick, closeBarMenus} from '/appswitch.js';
import {fitAlerts} from '/alerts.js';
import {noteError} from '/feedback.js';

export function initSpoof() {
  const user = document.querySelector('#user');
  if (!user) {
    return;
  }
  api('GET', '/auth/spoof').then(state => {
    if (state.quan) {
      offerQuan();
    }
    if (state.canSpoof) {
      buildSpoof(user, state);
    }
  }).catch(err => noteError('/auth/spoof: ' + err.message));
}

function buildSpoof(user, state) {
  const {wrap, pill, button} = spoofPill(state.spoofing);
  const menu = el('div', 'spoof-menu');
  menu.hidden = true;
  wrap.append(menu);
  document.body.classList.toggle('is-spoofing', Boolean(state.spoofing));
  user.before(wrap);
  fitAlerts();
  if (state.spoofing) {
    menu.append(spoofHead(state.spoofing));
  }
  menu.append(...spoofRecent(state));
  const search = spoofSearch();
  menu.append(el('div', 'spoof-section', state.recent.length ? 'Someone else' : 'View as'), search.box, search.results);
  wireSpoofMenu(pill, button, menu, search.load);
}

function spoofEye() {
  const eye = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  eye.setAttribute('viewBox', '0 0 24 24');
  eye.setAttribute('aria-hidden', 'true');
  const outline = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  outline.setAttribute('d', 'M1 12s4-7 11-7 11 7 11 7-4 7-11 7S1 12 1 12z');
  const pupil = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  pupil.setAttribute('cx', '12');
  pupil.setAttribute('cy', '12');
  pupil.setAttribute('r', '3');
  eye.append(outline, pupil);
  return eye;
}

function spoofPill(spoofing) {
  const wrap = el('span', 'spoof-wrap');
  const pill = el('span', 'spoof-pill');
  const button = el('button', 'spoof-button');
  button.type = 'button';
  button.setAttribute('aria-label', 'Spoof Mode');
  button.setAttribute('aria-haspopup', 'menu');
  button.title = 'Spoof Mode: view every app as someone else';
  button.append(spoofEye());
  pill.append(button);
  wrap.append(pill);
  if (!spoofing) {
    return {wrap, pill, button};
  }
  wrap.classList.add('is-on');
  const as = el('span', 'spoof-as');
  as.append(el('span', 'spoof-as-lead', 'Viewing as '), el('b', '', spoofing.name));
  button.append(as);
  const stop = el('button', 'spoof-stop', '×');
  stop.type = 'button';
  stop.title = 'Stop viewing as ' + spoofing.name;
  stop.setAttribute('aria-label', stop.title);
  stop.addEventListener('click', () => setSpoof(''));
  pill.append(stop);
  return {wrap, pill, button};
}

function spoofHead(spoofing) {
  const head = el('div', 'spoof-head');
  head.append(el('span', '', 'Viewing as '), el('b', '', spoofing.name));
  const stop = el('button', 'spoof-head-stop', 'Stop');
  stop.type = 'button';
  stop.addEventListener('click', () => setSpoof(''));
  head.append(stop);
  return head;
}

function spoofRecent(state) {
  if (!state.recent.length) {
    return [];
  }
  return [
    el('div', 'spoof-section', 'Recent'),
    ...state.recent.map(p => spoofRow(p.name, p.email, '', state.spoofing && p.email === state.spoofing.email)),
  ];
}

function spoofSearch() {
  const box = el('div', 'spoof-search');
  const input = document.createElement('input');
  input.type = 'search';
  input.placeholder = 'Search by name or email…';
  input.autocomplete = 'off';
  input.setAttribute('aria-label', 'Search for someone to view as');
  box.append(input);
  const results = el('div', 'spoof-results');

  let people = null;
  let loading = false;
  let failure = '';
  let active = -1;
  const load = async () => {
    if (people || loading) {
      return;
    }
    loading = true;
    failure = '';
    try {
      people = await listed();
    } catch (err) {
      failure = err.message;
      noteError('/api/q: ' + err.message);
    }
    loading = false;
    filter();
  };
  const filter = () => {
    const q = input.value.trim().toLowerCase();
    results.replaceChildren();
    active = -1;
    if (!q) {
      return;
    }
    if (failure) {
      results.append(el('div', 'spoof-empty', 'Couldn’t load people: ' + failure));
      return;
    }
    if (!people) {
      results.append(el('div', 'spoof-empty', 'Loading…'));
      return;
    }
    const found = people.filter(p => [p.name_show, emailOf(p), wordsOf(p)].some(v => v.toLowerCase().includes(q))).slice(0, 8);
    if (!found.length) {
      results.append(el('div', 'spoof-empty', 'Nobody matches.'));
      return;
    }
    for (const p of found) {
      results.append(spoofRow(p.name_show, emailOf(p), wordsOf(p), false));
    }
  };
  const setActive = index => {
    const rows = [...results.querySelectorAll('.spoof-row')];
    active = Math.max(-1, Math.min(index, rows.length - 1));
    rows.forEach((row, i) => row.classList.toggle('is-active', i === active));
  };
  input.addEventListener('focus', load);
  input.addEventListener('input', filter);
  input.addEventListener('keydown', e => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive(active + 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(active - 1);
    } else if (e.key === 'Enter') {
      const rows = results.querySelectorAll('.spoof-row');
      const row = rows[active >= 0 ? active : 0];
      if (row) {
        e.preventDefault();
        row.click();
      }
    }
  });
  return {box, results, load};
}

function wireSpoofMenu(pill, button, menu, load) {
  const open = () => {
    closeBarMenus();
    menu.hidden = false;
    load();
  };
  const close = () => {
    if (menu.contains(document.activeElement)) {
      return;
    }
    menu.hidden = true;
  };
  hoverMenu(pill, menu, open, close);
  button.addEventListener('click', e => {
    e.stopPropagation();
    if (menu.hidden) {
      open();
    } else if (!hoverClick(e)) {
      menu.hidden = true;
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

function spoofRow(name, email, words, isCurrent) {
  const row = el('button', 'spoof-row' + (isCurrent ? ' is-current' : ''));
  row.type = 'button';
  row.append(el('span', 'spoof-row-name', name), el('span', 'spoof-row-words', words || email));
  row.addEventListener('click', () => setSpoof(email));
  return row;
}

async function setSpoof(email) {
  try {
    await api('POST', '/auth/spoof', {email});
  } catch (err) {
    toast(err.message);
    return;
  }
  location.reload();
}
