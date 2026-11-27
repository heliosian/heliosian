import {el, svg} from '/elements.js';
import {loadThemes, themed} from '/themes.js';
import {sealedEnvelope, inviteCard, andList} from '/envelope.js';
import {replyKey, personBlock, familyBlock, missingNote} from '/questionfields.js';
import {openSheet, fillDeck} from '/sheets.js';
const token = location.pathname.split('/').filter(Boolean).pop();
const preview = location.pathname.startsWith('/preview/');
const api = '/open/ext/' + encodeURIComponent(token);
const root = document.getElementById('invite');
let view = null;
let opened = false;
let shownPage = '';
let made = 0;

function person(key) {
  return key === '' || key === view.key ? view : [...view.family, ...view.brought].find(p => p.key === key);
}

function keep(method, path, body) {
  if (path === '') {
    person(body.key).answer = body.answer;
  } else if (path === '/reply') {
    view.replies = view.replies.filter(r => !(r.question === body.question && (r.person || '') === body.person && (r.family || '') === body.family));
    if (body.value !== '') {
      view.replies.push(body);
    }
  } else if (path === '/guest' && method === 'POST') {
    made += 1;
    view.brought.push({key: 'preview-guest-' + made, name: body.name, kid: body.child});
  } else if (path === '/guest') {
    view.brought = view.brought.filter(p => p.key !== body.key);
  } else if (path === '/comment') {
    view.notes.push({text: body.text});
  }
  return Promise.resolve(null);
}

function send(method, path, body) {
  if (preview) {
    return keep(method, path, body);
  }
  return fetch(api + path, {method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)}).then(res => {
    if (!res.ok) {
      return res.text().then(t => {
        throw new Error(t || 'Something went wrong');
      });
    }
    return res.status === 204 ? null : res.json();
  });
}

function gone(words) {
  const box = el('div', 'invite-gone');
  box.append(el('h1', '', 'This invitation is not here'), el('p', '', words || 'The link may be incomplete, or the event may have been taken down. Ask whoever invited you for a fresh one.'));
  root.replaceChildren(box);
}

function people() {
  return [
    {key: view.key, name: view.name, short: 'you', answer: view.answer, kid: view.kid},
    ...view.family.map(p => ({...p, short: p.name.split(' ')[0]})),
    ...view.brought.map(p => ({...p, short: p.name.split(' ')[0], brought: true})),
  ];
}

function details() {
  const replies = {};
  for (const r of view.replies) {
    replies[replyKey(r.question, r.person, r.family)] = r;
  }
  return {
    questions: view.questions,
    replies,
    family: view.familyGroup || null,
    household: people().map(p => ({id: p.key, short: p.short, kid: Boolean(p.kid), going: p.answer === 'yes'})),
    save: (q, target, value) => send('POST', '/reply', {question: q.id, person: target.person || '', family: target.family || '', value}),
  };
}

function answerRow(p, status) {
  const row = el('div', 'reply-row');
  row.append(el('span', 'reply-name', p.key === view.key ? `${p.name} (you)` : p.name));
  const choices = el('div', 'reply-choices');
  for (const [word, label] of [['yes', 'Yes'], ...(view.maybe ? [['maybe', 'Maybe']] : []), ['no', 'No']]) {
    const on = p.answer === word;
    const b = el('button', 'reply-choice' + (on ? ' is-on' : ''));
    b.type = 'button';
    b.setAttribute('aria-pressed', String(on));
    if (on) {
      b.append(svg('check'));
    }
    b.append(el('span', '', label));
    b.addEventListener('click', () => {
      b.disabled = true;
      send('POST', '', {answer: on ? '' : word, key: p.key === view.key ? '' : p.key}).then(load).catch(err => {
        b.disabled = false;
        status.textContent = err.message;
      });
    });
    choices.append(b);
  }
  row.append(choices);
  if (p.brought) {
    const x = el('button', 'reply-remove', '×');
    x.type = 'button';
    x.title = 'Remove this guest';
    x.addEventListener('click', () => send('DELETE', '/guest', {key: p.key}).then(load).catch(err => {
      status.textContent = err.message;
    }));
    row.append(x);
  }
  return row;
}

function guestForm(status) {
  const box = el('div', 'bring');
  box.append(el('h3', 'bring-title', view.brought.length ? 'Bringing someone else?' : 'Bringing someone?'));
  const form = el('form', 'bring-form');
  const name = el('input');
  name.placeholder = 'Guest’s name';
  name.required = true;
  const email = el('input');
  email.type = 'email';
  email.placeholder = 'Their email (optional)';
  const child = el('label', 'bring-child');
  child.hidden = !view.questions.some(q => q.asked === 'kids' || q.asked === 'adults');
  const childBox = el('input');
  childBox.type = 'checkbox';
  child.append(childBox, el('span', '', 'Child'));
  const add = el('button', 'bring-add', 'Add guest');
  add.type = 'submit';
  form.append(name, email, child, add);
  form.addEventListener('submit', ev => {
    ev.preventDefault();
    add.disabled = true;
    send('POST', '/guest', {name: name.value.trim(), email: email.value.trim(), child: childBox.checked}).then(load).catch(err => {
      add.disabled = false;
      status.textContent = err.message;
    });
  });
  box.append(form, el('p', 'invite-panel-note', 'A guest with an email address gets an invitation of their own.'));
  return box;
}

function rsvpPanel() {
  const panel = el('section', 'invite-panel');
  const waiting = people().some(p => !p.answer);
  panel.append(el('h2', 'invite-panel-title', view.past ? 'This event has passed' : waiting ? 'Will you be there?' : 'Your RSVP'));
  const status = el('p', 'reply-status');
  const d = details();
  for (const p of people()) {
    panel.append(answerRow(p, status));
    const block = personBlock(d, d.household.find(h => h.id === p.key), load);
    if (block) {
      panel.append(block);
    }
  }
  for (const node of [familyBlock(d, load), missingNote(d)]) {
    if (node) {
      panel.append(node);
    }
  }
  panel.append(status);
  if (view.guests) {
    panel.append(guestForm(status));
  }
  return panel;
}

function notesPanel() {
  const panel = el('section', 'invite-panel');
  panel.append(el('h2', 'invite-panel-title', 'Write the hosts'), el('p', 'invite-panel-note', 'A question or a note just for the hosts. Other guests don’t see it.'));
  for (const n of view.notes) {
    const line = el('div', 'note' + (n.host ? ' is-reply' : ''));
    line.append(el('strong', '', n.host ? n.by : 'You'), el('p', '', n.text));
    panel.append(line);
  }
  const text = el('textarea', 'note-text');
  text.rows = 2;
  text.maxLength = 2000;
  text.placeholder = 'Write the hosts…';
  const status = el('p', 'reply-status');
  const go = el('button', 'bring-add', 'Send');
  go.type = 'button';
  go.addEventListener('click', () => {
    if (!text.value.trim()) {
      return;
    }
    go.disabled = true;
    send('POST', '/comment', {text: text.value.trim()}).then(load).catch(err => {
      go.disabled = false;
      status.textContent = err.message;
    });
  });
  const row = el('div', 'note-write');
  row.append(text, go);
  panel.append(row, status);
  return panel;
}

function lookOf() {
  return {
    theme: view.theme, envelope: view.envelope || '', seal: view.seal || '', liner: view.liner || '', design: view.design ? JSON.parse(view.design) : {},
    cardPicture: view.cardPicture || '', backdrop: view.backdrop || '',
  };
}

function card() {
  return inviteCard({
    look: lookOf(), name: view.title, subtitle: view.subtitle || '', cancelled: view.cancelled, day: view.day, hours: view.hours || '',
    location: view.location || '', place: view.place || '', description: view.description || '', hosts: view.hosts, hostedBy: view.hostedBy || '',
  });
}

function paint() {
  document.title = view.title + ' · You’re invited';
  const page = themed(el('div', 'invite-page'), lookOf());
  const column = el('div', 'invite-column');
  const above = el('div', 'invite-sheets is-above');
  const stage = el('div', 'invite-stage');
  const below = el('div', 'invite-sheets is-below');
  const side = el('div', 'invite-side');
  column.append(above, stage, below);
  page.append(column, side);
  root.replaceChildren(...(preview ? [previewBar()] : []), page);
  const deck = [{id: '', title: view.title, card: true}, ...view.pages.map(p => ({...p, pictureUrl: p.picture || ''}))];
  const show = item => {
    shownPage = item.id;
    paintDeck();
    column.scrollIntoView({behavior: 'smooth', block: 'start'});
  };
  const paintDeck = () => {
    const item = fillDeck(lookOf(), deck, shownPage, above, below, show);
    stage.replaceChildren(item.card ? card() : openSheet(lookOf(), item));
  };
  const fill = () => {
    paintDeck();
    side.replaceChildren(...(view.cancelled ? [] : [rsvpPanel(), notesPanel()]));
  };
  if (opened) {
    fill();
    return;
  }
  page.classList.add('is-sealed');
  const to = view.family.length ? 'For ' + andList(['you', ...view.family.map(p => p.name.split(' ')[0])]) : 'For ' + view.name;
  stage.append(sealedEnvelope(lookOf(), view.title, to, () => {
    opened = true;
    page.classList.remove('is-sealed');
    fill();
  }));
}

function previewBar() {
  const bar = el('div', 'preview-bar');
  const back = el('a', 'preview-bar-back', 'Back to editing');
  back.href = '/';
  if (history.length > 1) {
    back.addEventListener('click', ev => {
      ev.preventDefault();
      history.back();
    });
  }
  const again = el('button', 'preview-bar-again', 'Start over');
  again.type = 'button';
  again.addEventListener('click', () => location.reload());
  bar.append(el('span', '', `Preview as the McTweets: ${andList([view.name, ...view.family.map(p => p.name)].map(n => n.split(' ')[0]))}. Nothing here is saved or sent.`), again, back);
  return bar;
}

async function load() {
  if (preview && view) {
    paint();
    return;
  }
  const res = preview
    ? await fetch('/api/do/preview-invitation', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({group: token})})
    : await fetch(api);
  if (res.status === 404) {
    gone();
    return;
  }
  if (!res.ok) {
    gone('Something went wrong loading it. Try again in a moment.');
    return;
  }
  view = await res.json();
  await loadThemes([view.theme, view.envelope, view.seal, view.liner]);
  paint();
}

load().catch(() => gone('Something went wrong loading it. Try again in a moment.'));
