// The pieces of the shared toolbar (web/common/toolbar.css) that need script:
// filling the avatar, the "/" shortcut into search, and the switch to the other
// apps. What the search actually searches is each app's own business, wired in
// its chrome.

// The apps the switch lists come from Heliosian (internal/home.Apps), each
// keyed by its hostname's first label with its mark served from
// web/public/common/brand/apps/<key>.png. Home heads the list, and is what
// the switch falls back to when the ask fails - always a way home.
const homeApp = {key: 'home', name: 'Heliosian', tagline: 'Helios Community Apps'};

// Hostnames follow the tier of the page's own: beside who.heliosian.com sits
// team.heliosian.com, beside who.lab.heliosian.com sits team.lab.heliosian.com,
// and beside who.local.heliosian.com:8080 sits team.local...:8080. The link
// portal is the apex in production (heliosian.com, also www) and home.<tier>
// elsewhere. The app label comes off the front and the app's own goes on -
// hca.<tier> is the volunteer portal's older name, so it counts as team's.
const appLabels = ['who', 'team', 'hca', 'celebrate', 'birthday', 'calendar', 'cal', 'when', 'home', 'www'];

// The calendar answers as cal.<tier> and when.<tier> too, the way hca.<tier>
// is the volunteer portal's.
const aliases = {hca: 'team', cal: 'calendar', when: 'calendar'};

function tierLabels() {
  const labels = location.hostname.split('.');
  return appLabels.includes(labels[0]) ? labels.slice(1) : labels;
}

export function currentApp() {
  const first = location.hostname.split('.')[0];
  if (first === 'who' || first === 'team' || first === 'celebrate' || first === 'birthday' || first === 'calendar') {
    return first;
  }
  return aliases[first] || 'home';
}

export function appOrigin(key) {
  const tier = tierLabels();
  const host = key === 'home' && tier.length === 2 ? tier : [key, ...tier];
  return location.protocol + '//' + host.join('.') + (location.port ? ':' + location.port : '');
}

// What the switch lists and which rows Heliosian's Visibility tab keeps off
// this person's - asked of the page's own origin (every app serves the
// route). Purely presentation - a direct link still opens an app - so a
// failed ask lists just the way home rather than nothing.
async function switchList() {
  try {
    const res = await fetch('/api/apps/switch');
    if (!res.ok) {
      return {apps: [homeApp], hidden: []};
    }
    const {apps, hidden} = await res.json();
    return {apps: apps || [homeApp], hidden: hidden || []};
  } catch {
    return {apps: [homeApp], hidden: []};
  }
}

// Fills every .app-switch in the page (the desktop bar's and, where an app has
// one, the phone bar's) and wires it: click toggles the list, a click anywhere
// else or Escape closes it. The rows wait on the switch ask, which is well
// over before anyone opens the list; the app being viewed is always listed,
// whether or not the reader is on its list, since it is where they already
// are.
export function initAppSwitch() {
  const current = currentApp();
  const wraps = document.querySelectorAll('.app-switch');
  switchList().then(({apps, hidden}) => {
    for (const wrap of wraps) {
      const menu = wrap.querySelector('.app-switch-menu');
      for (const app of apps) {
        if (hidden.includes(app.key) && app.key !== current) {
          continue;
        }
        menu.append(appRow(app, app.key === current));
      }
      menu.append(menuFoot());
    }
  });
  for (const wrap of wraps) {
    const menu = wrap.querySelector('.app-switch-menu');
    // The click runs on to the document, where each app's own handler closes
    // its menus - so opening the list closes the account menu, and the
    // document's listener below leaves the switch itself alone.
    wrap.querySelector('.app-switch-button').addEventListener('click', () => {
      const opening = menu.hidden;
      closeAppSwitches();
      menu.hidden = !opening;
    });
  }
  document.addEventListener('click', e => {
    if (!e.target.closest('.app-switch')) {
      closeAppSwitches();
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeAppSwitches();
    }
  });
}

// One app's row of the switch: its mark, name and tagline, linking to it.
function appRow(app, isCurrent) {
  const row = document.createElement('a');
  row.href = appOrigin(app.host || app.key);
  row.className = isCurrent ? 'is-current' : '';
  const icon = document.createElement('img');
  icon.src = `/brand/apps/${app.key}.png` + (app.mark ? `?v=${app.mark}` : '');
  icon.alt = '';
  const text = document.createElement('span');
  const name = document.createElement('span');
  name.className = 'app-switch-name';
  name.textContent = app.name;
  const tagline = document.createElement('span');
  tagline.className = 'app-switch-tagline';
  tagline.textContent = app.tagline;
  text.append(name, tagline);
  row.append(icon, text);
  return row;
}

function menuFoot() {
  const foot = document.createElement('div');
  foot.className = 'app-switch-foot';
  foot.append(feedbackLine(), repoLine());
  return foot;
}

function feedbackLine() {
  const line = document.createElement('button');
  line.type = 'button';
  line.className = 'app-switch-feedback';
  const mark = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  mark.setAttribute('viewBox', '0 0 24 24');
  mark.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', 'M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z');
  mark.append(path);
  line.append(mark, document.createTextNode('Report a problem or idea'));
  line.addEventListener('click', () => {
    closeAppSwitches();
    openFeedback();
  });
  return line;
}

// The line under the apps: the octocat and a link to where they are built.
function repoLine() {
  const line = document.createElement('a');
  line.className = 'app-switch-repo';
  line.href = 'https://github.com/heliosian/heliosian';
  line.target = '_blank';
  line.rel = 'noopener';
  const mark = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  mark.setAttribute('viewBox', '0 0 24 24');
  mark.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', 'M12 .5C5.37.5 0 5.87 0 12.5c0 5.3 3.44 9.8 8.21 11.39.6.11.82-.26.82-.58 0-.29-.01-1.05-.02-2.06-3.34.73-4.04-1.61-4.04-1.61-.55-1.39-1.34-1.76-1.34-1.76-1.09-.75.08-.73.08-.73 1.21.09 1.84 1.24 1.84 1.24 1.07 1.84 2.81 1.31 3.5 1 .11-.78.42-1.31.76-1.61-2.67-.3-5.47-1.33-5.47-5.93 0-1.31.47-2.38 1.24-3.22-.12-.3-.54-1.52.12-3.18 0 0 1.01-.32 3.3 1.23a11.5 11.5 0 0 1 6 0c2.29-1.55 3.3-1.23 3.3-1.23.66 1.66.24 2.88.12 3.18.77.84 1.24 1.91 1.24 3.22 0 4.61-2.81 5.62-5.49 5.92.43.37.81 1.1.81 2.22 0 1.6-.01 2.9-.01 3.29 0 .32.22.7.83.58A12.01 12.01 0 0 0 24 12.5C24 5.87 18.63.5 12 .5z');
  mark.append(path);
  line.append(mark, document.createTextNode('Built for the community, by the community'));
  return line;
}

function closeAppSwitches() {
  for (const menu of document.querySelectorAll('.app-switch-menu')) {
    menu.hidden = true;
  }
}

const recentErrors = [];

function noteError(text) {
  recentErrors.push(String(text).slice(0, 300));
  if (recentErrors.length > 5) {
    recentErrors.shift();
  }
}

window.addEventListener('error', e => {
  const where = e.filename ? ` (${e.filename.split('/').pop()}:${e.lineno})` : '';
  noteError(e.message + where);
});

window.addEventListener('unhandledrejection', e => {
  const reason = e.reason;
  noteError(reason && reason.stack ? reason.stack.split('\n').slice(0, 2).join(' ') : String(reason));
});

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

const prompts = {
  bug: {summary: 'What went wrong?', details: 'What you did, what you expected, and what happened instead.'},
  idea: {summary: 'What would you like?', details: 'Where it would help, and what it would do.'},
};

let feedback;

function buildFeedback() {
  const overlay = el('div', 'feedback-overlay');
  overlay.hidden = true;
  const form = el('form', 'feedback-modal');
  const header = el('div', 'feedback-header');
  const close = el('button', 'feedback-close', '×');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  header.append(el('h2', '', 'Report a problem or idea'), close);
  const summary = document.createElement('input');
  summary.type = 'text';
  summary.maxLength = 120;
  summary.required = true;
  const details = document.createElement('textarea');
  details.rows = 5;
  details.maxLength = 4000;
  const prompt = kind => {
    summary.placeholder = prompts[kind].summary;
    details.placeholder = prompts[kind].details;
  };
  const kinds = el('div', 'feedback-kinds');
  for (const [kind, label] of [['bug', 'Something’s wrong'], ['idea', 'I’d like…']]) {
    const pill = el('label', 'feedback-kind');
    const radio = document.createElement('input');
    radio.type = 'radio';
    radio.name = 'kind';
    radio.value = kind;
    radio.checked = kind === 'bug';
    radio.addEventListener('change', () => prompt(kind));
    pill.append(radio, el('span', '', label));
    kinds.append(pill);
  }
  prompt('bug');
  const summaryField = el('label', 'feedback-field');
  summaryField.append(el('span', '', 'In a line'), summary);
  const detailsField = el('label', 'feedback-field');
  detailsField.append(el('span', '', 'Details'), details);
  const note = el('p', 'feedback-note', 'Goes to the people who build Heliosian, along with this page’s address, your email, and your browser details.');
  const actions = el('div', 'feedback-actions');
  const send = el('button', 'feedback-send', 'Send');
  send.type = 'submit';
  const cancel = el('button', 'feedback-cancel', 'Cancel');
  cancel.type = 'button';
  const status = el('span', 'feedback-status');
  actions.append(send, cancel, status);
  form.append(header, kinds, summaryField, detailsField, note, actions);
  overlay.append(form);
  document.body.append(overlay);
  const hide = () => {
    overlay.hidden = true;
  };
  close.addEventListener('click', hide);
  cancel.addEventListener('click', hide);
  overlay.addEventListener('click', e => {
    if (e.target === overlay) {
      hide();
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      hide();
    }
  });
  form.addEventListener('submit', async e => {
    e.preventDefault();
    status.textContent = 'Sending…';
    send.disabled = true;
    try {
      const res = await fetch('/api/feedback', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({
        kind: form.elements.kind.value,
        summary: summary.value,
        details: details.value,
        url: location.href,
        page: document.title,
        viewport: `${window.innerWidth}×${window.innerHeight}`,
        screen: `${screen.width}×${screen.height} @${window.devicePixelRatio}x`,
        language: navigator.language,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        errors: recentErrors,
      })});
      if (!res.ok) {
        status.textContent = await res.text();
        return;
      }
      hide();
      form.reset();
      prompt('bug');
      toast('Thanks, we got it.');
    } catch {
      status.textContent = 'Couldn’t send; check your connection and try again.';
    } finally {
      send.disabled = false;
    }
  });
  return {overlay, summary, status};
}

function openFeedback() {
  if (!feedback) {
    feedback = buildFeedback();
  }
  feedback.status.textContent = '';
  feedback.overlay.hidden = false;
  feedback.summary.focus();
}

let toastTimer;

function toast(message) {
  let node = document.querySelector('.feedback-toast');
  if (!node) {
    node = el('div', 'feedback-toast');
    document.body.append(node);
  }
  node.textContent = message;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    node.hidden = true;
  }, 2400);
}

// Every .user-avatar in the page (the desktop bar's and, where an app has one,
// the phone bar's) shows the hero photo when there is one, else the initial
// standing in for it.
export function renderAvatars({photoUrl, initial}) {
  for (const avatar of document.querySelectorAll('.user-avatar')) {
    if (photoUrl) {
      const img = document.createElement('img');
      img.src = photoUrl;
      img.alt = '';
      avatar.replaceChildren(img);
    } else {
      avatar.textContent = initial;
    }
  }
}

// Fills the toolbar's alert badges from the directory's reckoning - the
// count of things to update for the new year, and the privacy-mismatch
// triangle - linking each across to the page in Who? that resolves it. Who?
// itself reckons these client-side and does not call this.
export function renderAlerts({stale = 0, privacy = false} = {}) {
  const who = appOrigin('who');
  for (const badge of document.querySelectorAll('.stale-alert')) {
    badge.hidden = !stale;
    badge.href = who + '/my-family';
    badge.title = `${stale} thing${stale === 1 ? '' : 's'} to update for the new year`;
  }
  for (const count of document.querySelectorAll('.stale-count')) {
    count.textContent = String(stale);
  }
  for (const badge of document.querySelectorAll('.privacy-alert')) {
    badge.hidden = !privacy;
    badge.href = who + '/my-privacy';
    badge.title = 'Your privacy settings don\u2019t match Veracross';
  }
}

// markSuper shows or hides the avatar's red ring for Super Admin Mode.
export function markSuper(on) {
  document.body.classList.toggle('is-super', Boolean(on));
}

export function isEditableTarget(target) {
  return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT' || target.isContentEditable;
}

// A bare "/" (no modifiers, and not already typing somewhere) jumps straight to
// search, the way GitHub and Slack do - skipped while any text field, including
// the search box itself, has focus so a literal "/" can still be typed.
export function onSlash(open) {
  document.addEventListener('keydown', e => {
    if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
      e.preventDefault();
      open();
    }
  });
}
