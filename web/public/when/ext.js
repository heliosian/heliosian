const token = location.pathname.split('/').filter(Boolean).pop();
const api = '/open/ext/' + encodeURIComponent(token);
const card = document.getElementById('card');
let view = null;
const icons = {
  yes: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>',
  maybe: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>',
  no: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18M6 6l12 12"/></svg>'
};
function el(tag, cls, text) {
  const n = document.createElement(tag);
  if (cls) {
    n.className = cls;
  }
  if (text != null) {
    n.textContent = text;
  }
  return n;
}
function send(method, path, body) {
  return fetch(path, {method: method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)}).then(res => {
    if (!res.ok) {
      return res.text().then(t => { throw new Error(t || 'Something went wrong'); });
    }
    return res.status === 204 ? null : res.json();
  });
}
function gone(words) {
  card.replaceChildren();
  const body = el('div', 'gone');
  body.append(el('h1', '', 'This invitation is not here'), el('p', 'hosts', words || 'The link may be incomplete, or the event may have been taken down. Ask whoever invited you for a fresh one.'));
  card.append(body);
}
function paint() {
  document.title = view.title + ' · You’re invited';
  card.replaceChildren();
  const banner = el('div', 'banner');
  if (view.banner) {
    banner.style.backgroundImage = 'url("' + view.banner + '")';
  }
  card.append(banner);
  const body = el('div', 'body');
  body.append(el('div', 'kicker', view.past ? 'This event has passed' : 'You’re invited'));
  body.append(el('h1', '', view.title));
  body.append(el('div', 'when', view.day + (view.hours ? ' · ' + view.hours : '')));
  if (view.location) {
    body.append(el('div', 'where', view.location));
  }
  if (view.hosts.length) {
    body.append(el('div', 'hosts', 'Invited by ' + view.hosts.join(' and ') + (view.name ? ' · for ' + view.name : '')));
  }
  if (view.message) {
    body.append(el('div', 'message', view.message));
  }
  if (view.description) {
    body.append(el('div', 'words', view.description));
  }
  if (view.flyer) {
    const flyer = el('a', 'flyer');
    flyer.href = view.flyer;
    flyer.target = '_blank';
    const img = el('img');
    img.src = view.flyer;
    img.alt = 'The flyer';
    flyer.append(img);
    body.append(flyer);
  }
  const ask = el('div', 'ask');
  const word = view.answer;
  ask.append(el('div', 'ask-title', word === 'yes' ? 'You’re going' : word === 'no' ? 'Not going' : word === 'maybe' ? 'Maybe' : 'Are you going?'));
  ask.append(el('div', 'ask-lead', word ? 'Thanks for letting the hosts know. Change your answer any time.' : 'Let the hosts know with one tap.'));
  const buttons = el('div', 'buttons');
  const status = el('div', 'status');
  [['yes', 'Yes'], ['maybe', 'Maybe'], ['no', 'No']].forEach(pair => {
    const b = el('button', 'choice choice-' + pair[0] + (word === pair[0] ? ' is-on' : ''));
    b.type = 'button';
    b.innerHTML = icons[pair[0]] + '<span>' + pair[1] + '</span>';
    b.addEventListener('click', () => {
      const next = word === pair[0] ? '' : pair[0];
      send('POST', api, {answer: next}).then(() => {
        view.answer = next;
        paint();
      }).catch(err => { status.textContent = err.message; });
    });
    buttons.append(b);
  });
  ask.append(buttons, status);
  if (view.family && view.family.length) {
    const fam = el('div', 'guests');
    fam.append(el('div', 'guests-title', 'Your family'));
    view.family.forEach(m => {
      const row = el('div', 'guest family-row');
      row.append(el('span', '', m.name));
      const picks = el('div', 'family-picks');
      [['yes', 'Yes'], ['maybe', 'Maybe'], ['no', 'No']].forEach(pair => {
        const b = el('button', 'family-pick choice-' + pair[0] + (m.answer === pair[0] ? ' is-on' : ''));
        b.type = 'button';
        b.innerHTML = icons[pair[0]] + '<span>' + pair[1] + '</span>';
        b.addEventListener('click', () => {
          const next = m.answer === pair[0] ? '' : pair[0];
          send('POST', api, {answer: next, key: m.key}).then(() => {
            m.answer = next;
            paint();
          }).catch(err => { status.textContent = err.message; });
        });
        picks.append(b);
      });
      row.append(picks);
      fam.append(row);
    });
    ask.append(fam);
  }
  if (view.guests) {
    const guests = el('div', 'guests');
    guests.append(el('div', 'guests-title', view.brought.length ? 'Your guests' : 'Bringing someone?'));
    view.brought.forEach(g => {
      const row = el('div', 'guest');
      row.append(el('span', '', g.name));
      const x = el('button', '', '×');
      x.type = 'button';
      x.title = 'Remove';
      x.addEventListener('click', () => {
        send('DELETE', api + '/guest', {key: g.key}).then(load).catch(err => { status.textContent = err.message; });
      });
      row.append(x);
      guests.append(row);
    });
    const form = el('form', 'add');
    const name = el('input');
    name.placeholder = 'Guest’s name';
    name.required = true;
    const email = el('input');
    email.type = 'email';
    email.placeholder = 'Their email (optional)';
    const add = el('button', '', 'Add guest');
    add.type = 'submit';
    form.append(name, email, add);
    form.addEventListener('submit', ev => {
      ev.preventDefault();
      add.disabled = true;
      send('POST', api + '/guest', {name: name.value.trim(), email: email.value.trim()}).then(load).catch(err => {
        add.disabled = false;
        status.textContent = err.message;
      });
    });
    guests.append(form);
    guests.append(el('div', 'note', 'A guest with an email address gets an invitation of their own.'));
    ask.append(guests);
  }
  body.append(ask);
  card.append(body);
}
function load() {
  return fetch(api).then(res => {
    if (res.status === 404) {
      gone();
      return;
    }
    if (!res.ok) {
      throw new Error();
    }
    return res.json().then(v => { view = v; paint(); });
  }).catch(() => { gone('Something went wrong loading it. Try again in a moment.'); });
}
load();
