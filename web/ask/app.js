import {renderAvatars, renderAlerts, renderProfileLink, onSlash, initAppSwitch, initUserMenu, initSpoof, markSuper} from '/toolbar.js';
import {render, stable} from '/markdown.js';

// The chats live in this browser, under the signed-in address: each its
// id on the server (while the server still knows it), a title from its
// first question, when it was last touched, and every turn. The server
// keeps a working copy for an hour; after that, or a restart, it rebuilds
// one from the turns sent along with the next message.
const maxChats = 50;

const state = {model: null, chats: [], current: null, busy: false, stopper: null};

function chatsKey() {
  return 'ask-chats:' + state.model.user.email;
}

function currentKey() {
  return 'ask-current:' + state.model.user.email;
}

function loadChats() {
  try {
    const raw = JSON.parse(localStorage.getItem(chatsKey()) || '[]');
    state.chats = Array.isArray(raw) ? raw.filter(c => c && c.id && Array.isArray(c.turns)) : [];
  } catch {
    state.chats = [];
  }
  state.chats.sort((a, b) => (b.updated || 0) - (a.updated || 0));
  const id = localStorage.getItem(currentKey());
  state.current = state.chats.find(c => c.id === id) || null;
}

function saveChats() {
  state.chats.sort((a, b) => (b.updated || 0) - (a.updated || 0));
  state.chats = state.chats.slice(0, maxChats);
  localStorage.setItem(chatsKey(), JSON.stringify(state.chats));
  if (state.current) {
    localStorage.setItem(currentKey(), state.current.id);
  } else {
    localStorage.removeItem(currentKey());
  }
}

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

let toastTimer;

function toast(message) {
  const node = document.querySelector('#toast');
  node.textContent = message;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    node.hidden = true;
  }, 2600);
}

function thread() {
  return document.querySelector('#thread');
}

function scrollDown() {
  const main = document.querySelector('#main');
  main.scrollTo({top: main.scrollHeight});
}

// A turn on the thread: the person's words plain on the right, the
// answer on the left as a sequence of what happened in order - words,
// then a chip for a lookup, then more words - each piece drawn from
// markdown as it streams and after.
function addTurn(role) {
  const row = el('div', 'turn turn-' + role);
  row.append(el('div', 'turn-body'));
  thread().append(row);
  return row;
}

// segments is an answer as pieces in order: {kind: 'text', text} and
// {kind: 'tool', words}. A chat kept before the pieces were recorded has
// only its tools and text, drawn as chips then words.
function segmentsOf(turn) {
  if (turn.segments) {
    return turn.segments;
  }
  const out = (turn.tools || []).map(words => ({kind: 'tool', words}));
  out.push({kind: 'text', text: turn.text || ''});
  return out;
}

function showSegments(row, segments, streaming, cards) {
  const body = row.querySelector('.turn-body');
  body.replaceChildren();
  segments.forEach((s, i) => {
    if (s.kind === 'tool') {
      const line = el('div', 'turn-tools');
      line.append(el('span', 'tool-chip', s.words));
      body.append(line);
      return;
    }
    const last = streaming && i === segments.length - 1;
    const piece = el('div', 'turn-text');
    piece.append(render(last ? stable(s.text) : s.text, cards));
    body.append(piece);
  });
}

const spinFrames = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];

function spinner() {
  const node = el('span', 'spinner', spinFrames[0]);
  node.setAttribute('aria-label', 'Still answering');
  let frame = 0;
  const timer = setInterval(() => {
    frame = (frame + 1) % spinFrames.length;
    node.textContent = spinFrames[frame];
  }, 120);
  return {node, stop: () => {
    clearInterval(timer);
    node.remove();
  }};
}

const spinnerHosts = new Set(['DIV', 'P', 'UL', 'OL', 'LI', 'BLOCKQUOTE', 'H3']);

function placeSpinner(row, node) {
  let host = row.querySelector('.turn-body');
  while (host.lastElementChild && spinnerHosts.has(host.lastElementChild.tagName)) {
    host = host.lastElementChild;
  }
  host.append(node);
}

// The empty thread: a greeting and the starters the server drew from the
// person's own circumstances, each a question to send as is.
function renderEmpty() {
  const empty = el('div', 'empty');
  empty.append(el('h1', 'page-title', 'Ask'));
  empty.append(el('p', 'page-lead', 'Anything about the school, your family, the calendar, the directory, volunteering, the parties or the email groups. The answer comes from the community’s own apps and links you to them.'));
  const starters = el('div', 'starters');
  for (const q of state.model.starters) {
    const b = el('button', 'starter', q);
    b.type = 'button';
    b.addEventListener('click', () => send(q));
    starters.append(b);
  }
  empty.append(starters);
  thread().replaceChildren(empty);
}

function renderThread() {
  note('');
  if (!state.current) {
    renderEmpty();
    return;
  }
  thread().replaceChildren();
  for (const t of state.current.turns) {
    const row = addTurn(t.role);
    if (t.role === 'user') {
      row.querySelector('.turn-body').textContent = t.text;
    } else {
      showSegments(row, segmentsOf(t), false, t.cards || {});
    }
  }
  if (asked(state.current) >= state.model.maxTurns) {
    note('This chat has run long. Start a new one to keep going.');
  }
  scrollDown();
}

function asked(chat) {
  return chat.turns.filter(t => t.role === 'user').length;
}

// The rail's list of chats, newest first, the open one lit, each with a
// remove; the same list fills the phone's drawer.
function renderChats() {
  for (const list of document.querySelectorAll('.chat-list')) {
    list.replaceChildren();
    if (!state.chats.length) {
      list.append(el('div', 'chat-list-empty', 'Your chats stay in this browser and show here.'));
      continue;
    }
    for (const chat of state.chats) {
      const row = el('div', 'chat-row' + (state.current && chat.id === state.current.id ? ' is-on' : ''));
      const open = el('button', 'chat-open', chat.title || 'Untitled chat');
      open.type = 'button';
      open.title = chat.title || '';
      open.addEventListener('click', () => {
        openChat(chat);
        closeDrawer();
      });
      const remove = el('button', 'chat-remove', '×');
      remove.type = 'button';
      remove.setAttribute('aria-label', 'Remove this chat');
      remove.title = 'Remove this chat';
      remove.addEventListener('click', e => {
        e.stopPropagation();
        removeChat(chat);
      });
      row.append(open, remove);
      list.append(row);
    }
  }
}

function openChat(chat) {
  if (state.busy) {
    return;
  }
  state.current = chat;
  saveChats();
  renderChats();
  renderThread();
}

function removeChat(chat) {
  if (state.busy) {
    return;
  }
  state.chats = state.chats.filter(c => c.id !== chat.id);
  if (state.current && state.current.id === chat.id) {
    state.current = null;
  }
  saveChats();
  renderChats();
  renderThread();
}

function newChat() {
  if (state.busy) {
    return;
  }
  state.current = null;
  saveChats();
  renderChats();
  renderThread();
  closeDrawer();
  composer().focus();
}

function composer() {
  return document.querySelector('#message');
}

function setBusy(busy) {
  state.busy = busy;
  const button = document.querySelector('#send');
  button.setAttribute('aria-label', busy ? 'Stop' : 'Send');
  button.title = busy ? 'Stop' : '';
  composer().disabled = busy;
  document.body.classList.toggle('is-busy', busy);
}

function stop() {
  if (state.stopper) {
    state.stopper.abort();
  }
}

function keepStopped(chat, message, mine, answer, segments, tools, cards) {
  const text = segments.filter(s => s.kind === 'text' && s.text.trim()).map(s => s.text).join('\n\n');
  if (!text) {
    answer.remove();
    mine.remove();
    composer().value = message;
    autosize();
    if (!chat.turns.length) {
      state.chats = state.chats.filter(c => c !== chat);
      state.current = null;
      saveChats();
      renderChats();
      renderThread();
    }
    return;
  }
  showSegments(answer, segments, false, cards);
  chat.turns.push({role: 'user', text: message}, {role: 'assistant', text, tools, segments, cards});
  chat.updated = Date.now();
  saveChats();
  renderChats();
}

function note(text) {
  document.querySelector('#composer-note').textContent = text;
}

// readEvents reads server-sent events off a stream, calling on(kind, data)
// for each complete one; the tail of a chunk waits for the next.
async function readEvents(response, on) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const {value, done} = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, {stream: true});
    let at;
    while ((at = buffer.indexOf('\n\n')) >= 0) {
      const frame = buffer.slice(0, at);
      buffer = buffer.slice(at + 2);
      let kind = 'message';
      const data = [];
      for (const line of frame.split('\n')) {
        if (line.startsWith('event: ')) {
          kind = line.slice(7);
        } else if (line.startsWith('data: ')) {
          data.push(line.slice(6));
        }
      }
      on(kind, JSON.parse(data.join('\n')));
    }
  }
}

async function send(message) {
  message = message.trim();
  if (!message || state.busy) {
    return;
  }
  if (!state.current) {
    state.current = {id: '', title: message.slice(0, 80), updated: Date.now(), turns: []};
    state.chats.unshift(state.current);
    thread().replaceChildren();
  }
  const chat = state.current;
  if (asked(chat) >= state.model.maxTurns) {
    note('This chat has run long. Start a new one to keep going.');
    return;
  }
  setBusy(true);
  note('');
  const mine = addTurn('user');
  mine.querySelector('.turn-body').textContent = message;
  const answer = addTurn('assistant');
  const spin = spinner();
  placeSpinner(answer, spin.node);
  const segments = [];
  const tools = [];
  const cards = {};
  let finished = null;
  let pending = null;
  const draw = () => {
    if (pending) {
      return;
    }
    pending = requestAnimationFrame(() => {
      pending = null;
      showSegments(answer, segments, true, cards);
      placeSpinner(answer, spin.node);
      scrollDown();
    });
  };
  const addText = piece => {
    const last = segments[segments.length - 1];
    if (last && last.kind === 'text') {
      last.text += piece;
    } else {
      segments.push({kind: 'text', text: piece});
    }
  };
  scrollDown();
  composer().value = '';
  autosize();
  const stopper = new AbortController();
  state.stopper = stopper;
  try {
    const res = await fetch('/api/ask/chat', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({conversation: chat.id, message, turns: chat.turns}), signal: stopper.signal});
    if (!res.ok) {
      const why = await res.text();
      answer.remove();
      mine.remove();
      composer().value = message;
      autosize();
      if (res.status === 400 && why.includes('new one')) {
        note(why.trim());
      } else {
        toast(why.trim());
      }
      if (!chat.turns.length) {
        state.chats = state.chats.filter(c => c !== chat);
        state.current = null;
        renderThread();
      }
      return;
    }
    await readEvents(res, (kind, data) => {
      switch (kind) {
        case 'start':
          chat.id = data.conversation;
          break;
        case 'text':
          addText(data);
          draw();
          break;
        case 'tool':
          if (!tools.includes(data)) {
            tools.push(data);
            segments.push({kind: 'tool', words: data});
            draw();
          }
          break;
        case 'card':
          cards[data.url] = data;
          break;
        case 'done':
          finished = data;
          break;
        case 'error':
          segments.push({kind: 'text', text: data.message});
          draw();
          break;
      }
    });
    cancelAnimationFrame(pending);
    showSegments(answer, segments, false, cards);
    scrollDown();
    if (finished) {
      chat.turns.push({role: 'user', text: message}, {role: 'assistant', text: finished.text, tools: finished.tools || [], segments, cards});
      chat.updated = Date.now();
      saveChats();
      renderChats();
      if (finished.turns >= state.model.maxTurns) {
        note('This chat has run long. Start a new one to keep going.');
      }
    } else if (!chat.turns.length) {
      state.chats = state.chats.filter(c => c !== chat);
      state.current = null;
      saveChats();
      renderChats();
    }
  } catch (err) {
    cancelAnimationFrame(pending);
    if (stopper.signal.aborted) {
      keepStopped(chat, message, mine, answer, segments, tools, cards);
      return;
    }
    segments.push({kind: 'text', text: 'The connection dropped; try again.'});
    showSegments(answer, segments, false, cards);
  } finally {
    spin.stop();
    state.stopper = null;
    setBusy(false);
    if (matchMedia('(hover: hover)').matches) {
      composer().focus();
    }
  }
}

function autosize() {
  const box = composer();
  box.style.height = 'auto';
  box.style.height = Math.min(box.scrollHeight, 200) + 'px';
}

function openDrawer() {
  renderChats();
  document.querySelector('#drawer-overlay').hidden = false;
}

function closeDrawer() {
  document.querySelector('#drawer-overlay').hidden = true;
}

function renderUser() {
  const user = state.model.user;
  renderAvatars({photoUrl: user.photoUrl, initial: user.initial});
  renderAlerts(state.model.alerts || {});
  renderProfileLink(user.email);
  for (const line of document.querySelectorAll('.user-menu-email')) {
    line.textContent = user.email;
  }
  markSuper(false);
}

function initChrome() {
  initAppSwitch();
  initUserMenu();
  initSpoof();
  onSlash(() => composer().focus());
  document.addEventListener('click', e => {
    if (!e.target.closest('#user, #user-menu')) {
      document.querySelector('#user-menu').hidden = true;
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      document.querySelector('#user-menu').hidden = true;
      closeDrawer();
    }
  });
  for (const button of document.querySelectorAll('.new-chat')) {
    button.addEventListener('click', newChat);
  }
  document.querySelector('#menu-button').addEventListener('click', openDrawer);
  document.querySelector('#drawer-close').addEventListener('click', closeDrawer);
  document.querySelector('#drawer-overlay').addEventListener('click', e => {
    if (e.target === e.currentTarget) {
      closeDrawer();
    }
  });
  const form = document.querySelector('#composer');
  form.addEventListener('submit', e => {
    e.preventDefault();
    if (state.busy) {
      stop();
      return;
    }
    send(composer().value);
  });
  composer().addEventListener('input', autosize);
  composer().addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send(composer().value);
    }
  });
}

async function load() {
  const res = await fetch('/api/ask/model');
  if (!res.ok) {
    throw new Error(`loading model failed: ${res.status}`);
  }
  state.model = await res.json();
  renderUser();
  loadChats();
  renderChats();
  renderThread();
}

initChrome();
load();
