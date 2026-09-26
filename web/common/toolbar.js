import {modeRow, offerQuan} from '/mode.js';
// The pieces of the shared toolbar (web/common/toolbar.css) that need script:
// filling the avatar and opening its menu, the "/" shortcut into search, and
// the switch to the other apps. What the search actually searches is each
// app's own business, wired in its chrome.

// The apps the switch lists come from Heliosian (internal/home.Apps), each
// keyed by its hostname's first label with its mark served from
// web/public/common/brand/apps/<key>.png. Home heads the list, and is what
// the switch falls back to when the ask fails - always a way home.
const homeApp = {key: 'home', name: 'Heliosian', tagline: 'Helios Community Apps'};

// Hostnames follow the tier of the page's own: beside who.heliosian.com sits
// team.heliosian.com, and beside who.heliosiandev.com:8080 sits
// team.heliosiandev.com:8080. The link portal is the apex (heliosian.com or
// heliosiandev.com, also www). The app label comes off the front and the
// app's own goes on - hca.<tier> is the volunteer portal's older name, so it
// counts as team's.
const appLabels = ['who', 'team', 'hca', 'celebrate', 'birthday', 'calendar', 'cal', 'when', 'loop', 'ask', 'home', 'www'];

// The calendar answers as cal.<tier> and when.<tier> too, the way hca.<tier>
// is the volunteer portal's.
const aliases = {hca: 'team', cal: 'calendar', when: 'calendar'};

function tierLabels() {
  const labels = location.hostname.split('.');
  return appLabels.includes(labels[0]) ? labels.slice(1) : labels;
}

export function currentApp() {
  const first = location.hostname.split('.')[0];
  if (first === 'who' || first === 'team' || first === 'celebrate' || first === 'birthday' || first === 'calendar' || first === 'loop' || first === 'ask') {
    return first;
  }
  return aliases[first] || 'home';
}

export function signedIn(res) {
  if (res.status === 401) {
    location.reload();
    return new Promise(() => {});
  }
  return res;
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

// Both of the bar's dropdowns - the account menu under the avatar and the
// switch under the Heliosian tile - open on hover: the menu shows while the
// mouse is over its button or over the menu itself, and goes once the mouse
// has left both. The going waits a moment so that crossing the gap between
// button and menu does not shut it. Touch and pen make no hover, so their
// taps fall to the click handlers, which is where the menus toggle.
const hoverGrace = 150;

export function hoverMenu(button, menu, open, close) {
  let timer;
  const enter = e => {
    if (e.pointerType !== 'mouse') {
      return;
    }
    clearTimeout(timer);
    open();
  };
  const leave = e => {
    if (e.pointerType !== 'mouse') {
      return;
    }
    clearTimeout(timer);
    timer = setTimeout(close, hoverGrace);
  };
  for (const node of [button, menu]) {
    node.addEventListener('pointerenter', enter);
    node.addEventListener('pointerleave', leave);
  }
}

// Whether a click came from something that hovers. On a phone or a tablet -
// any device whose primary pointer cannot hover - nothing does, whatever the
// event claims, since some mobile browsers report a tap as a mouse click;
// the media query is the reliable tell, and Safari, Chrome and Firefox all
// answer it. Elsewhere it is the event's pointer: a mouse, or the keyboard
// (whose synthetic click names no pointer), does; a finger or a pen on a
// laptop's touchscreen does not.
export function hoverClick(e) {
  if (matchMedia('(hover: none)').matches) {
    return false;
  }
  return !e.pointerType || e.pointerType === 'mouse';
}

// Wires the avatar to the account menu under it: the menu opens on hover,
// and a click leaves it open where the mouse already holds it (or opens it
// again after Escape), while a tap toggles it. The click stops at the button
// so the document click each app uses to close its menus lets this one be.
// The menu's rows, and closing it from that document click and Escape, are
// each app's own.
export function initUserMenu() {
  const button = document.querySelector('#user');
  const menu = document.querySelector('#user-menu');
  // Every app calls this, so the RSVP and approvals badges ride along, and
  // the directory's count gets its bell.
  initRSVP();
  initApprovals();
  initLate();
  bellStale();
  // Dark mode's row goes in every app's menu, above Sign Out.
  if (!menu.querySelector('.user-menu-mode')) {
    const signOut = menu.querySelector('form');
    menu.insertBefore(modeRow(), signOut || null);
  }
  const open = () => {
    closeAppSwitches();
    menu.hidden = false;
  };
  const close = () => {
    menu.hidden = true;
  };
  hoverMenu(button, menu, open, close);
  button.addEventListener('click', e => {
    e.stopPropagation();
    if (menu.hidden) {
      open();
    } else if (!hoverClick(e)) {
      close();
    }
  });
}

function closeUserMenus() {
  for (const menu of document.querySelectorAll('.user-menu')) {
    menu.hidden = true;
  }
}

// Fills every .app-switch in the page (the desktop bar's and, where an app has
// one, the phone bar's) and wires it: the list opens on hover, and the tile
// is a link home - to Heliosian on the page's own tier - so a click goes
// there. A tap, which cannot hover, toggles the list instead, since it is
// the only way to the list on a phone; a tap anywhere else or Escape closes
// it. The rows wait on the switch ask, which is well over before anyone
// opens the list; the app being viewed is always listed, whether or not the
// reader is on its list, since it is where they already are.
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
    const button = wrap.querySelector('.app-switch-button');
    const menu = wrap.querySelector('.app-switch-menu');
    button.href = appOrigin('home');
    const open = () => {
      closeUserMenus();
      closeAppSwitches();
      menu.hidden = false;
    };
    hoverMenu(button, menu, open, closeAppSwitches);
    // The tap runs on to the document, where each app's own handler closes
    // its menus - so opening the list closes the account menu, and the
    // document's listener below leaves the switch itself alone.
    button.addEventListener('click', e => {
      if (hoverClick(e)) {
        return;
      }
      e.preventDefault();
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
      for (const menu of document.querySelectorAll('.topbar-alert-menu')) {
        menu.hidden = true;
      }
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

// The card under an alert badge, on the same hover as the menus, in the
// badge's .topbar-alert-wrap: built afresh by build() on each open, so it
// says what the badge says now, and kept inside the viewport on a narrow
// screen, where a card right-aligned to a badge near the bar's left would
// run off it. A tap does nothing here - the badge is a link, and a phone
// follows it - so the card is wired once, whatever fills it later.
export function alertMenu(badge, build) {
  const wrap = badge.parentElement;
  let menu = wrap.querySelector('.topbar-alert-menu');
  if (!menu) {
    menu = el('div', 'topbar-alert-menu');
    menu.hidden = true;
    wrap.append(menu);
    hoverMenu(badge, menu, () => {
      menu.replaceChildren(build());
      menu.hidden = false;
      clampMenu(wrap, menu);
    }, () => {
      menu.hidden = true;
    });
  }
  return menu;
}

function clampMenu(wrap, menu) {
  const margin = 12;
  const wrapRect = wrap.getBoundingClientRect();
  const width = menu.offsetWidth;
  const left = Math.max(margin, Math.min(wrapRect.right - width, window.innerWidth - margin - width));
  menu.style.left = `${left - wrapRect.left}px`;
  menu.style.right = 'auto';
  // The card's point sits under the badge it hangs from.
  menu.style.setProperty('--notch', `${wrapRect.left + wrapRect.width / 2 - left}px`);
}

// alertIcons are the rows' own marks, so each says what it is at a glance:
// a photo, a family photo, facts, an address, a phone, and the three apps'
// things waiting for approval.
export const alertIcons = {
  photo: '<svg viewBox="0 0 24 24"><path d="M4 8h3l2-3h6l2 3h3a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1z"/><circle cx="12" cy="13.5" r="3.5"/></svg>',
  family: '<svg viewBox="0 0 24 24"><circle cx="9" cy="8" r="3.2"/><path d="M3 20c0-3.6 2.7-6 6-6s6 2.4 6 6"/><circle cx="17" cy="9.5" r="2.6"/><path d="M15.5 14.3c3 .1 5.5 2.3 5.5 5.7"/></svg>',
  facts: '<svg viewBox="0 0 24 24"><rect x="5" y="3" width="14" height="18" rx="2"/><path d="M9 8h6M9 12h6M9 16h4"/></svg>',
  address: '<svg viewBox="0 0 24 24"><path d="M12 22s7-7.6 7-12a7 7 0 1 0-14 0c0 4.4 7 12 7 12z"/><circle cx="12" cy="10" r="2.5"/></svg>',
  phone: '<svg viewBox="0 0 24 24"><path d="M22 16.9v3a2 2 0 0 1-2.2 2 19.8 19.8 0 0 1-8.6-3.1 19.5 19.5 0 0 1-6-6A19.8 19.8 0 0 1 2.1 4.2 2 2 0 0 1 4.1 2h3a2 2 0 0 1 2 1.7c.1.9.4 1.8.7 2.7a2 2 0 0 1-.5 2.1L8 9.8a16 16 0 0 0 6 6l1.3-1.3a2 2 0 0 1 2.1-.4c.9.3 1.8.6 2.7.7a2 2 0 0 1 1.7 2z"/></svg>',
  team: '<svg viewBox="0 0 24 24"><circle cx="9" cy="8" r="3.2"/><path d="M3 20c0-3.6 2.7-6 6-6s6 2.4 6 6"/><path d="M16 5.2a3.2 3.2 0 0 1 0 5.6M18 14.4c2 .7 3 2.8 3 5.6"/></svg>',
  celebrate: '<svg viewBox="0 0 24 24"><path d="M3 9V7a1 1 0 0 1 1-1h16a1 1 0 0 1 1 1v2a2.5 2.5 0 0 0 0 5v2a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-2a2.5 2.5 0 0 0 0-5z"/><path d="M14 6v11" stroke-dasharray="2 2"/></svg>',
  calendar: '<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/></svg>',
};

// updateIcon is the mark for one thing the directory wants updated, by
// what its label says it is.
function updateIcon(label) {
  if (/^family photo/i.test(label)) {
    return alertIcons.family;
  }
  return /facts$/i.test(label) ? alertIcons.facts : alertIcons.photo;
}

// alertList is the card for a badge that lists things - RSVP's invitations,
// the approvals: the count large in its tone beside what they are and a
// close, a row for each - its icon in a pale disc beside a chip with the
// title and its note, to its page - and a button across the foot to the
// whole list. tone is the card's colour, red or amber.
export function alertList({count, words, items, icon, button, href, tone}) {
  const card = el('div', 'alert-card alert-list' + (tone ? ' is-' + tone : ''));
  const head = el('div', 'alert-list-head');
  head.append(el('span', 'alert-list-count', String(count)), el('span', 'alert-list-title', words));
  const close = el('button', 'alert-list-close');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  close.innerHTML = '<svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6 6 18"/></svg>';
  close.addEventListener('click', () => {
    const menu = card.closest('.topbar-alert-menu');
    if (menu) {
      menu.hidden = true;
    }
  });
  head.append(close);
  card.append(head);
  for (const item of items) {
    const row = el('div', 'alert-list-row');
    const disc = el('span', 'alert-card-disc');
    disc.innerHTML = item.icon || icon;
    // A row goes to its page, or - given an action - does it in place, as
    // Who?'s photo rows open the file picker; the note can be told how it
    // went (item.status is handed the note's node).
    const chip = el(item.action ? 'button' : 'a', 'alert-list-chip');
    if (item.action) {
      chip.type = 'button';
      chip.addEventListener('click', item.action);
    } else {
      chip.href = item.href;
    }
    const note = el('span', 'alert-list-chip-note', item.note);
    const text = el('span', 'alert-list-words');
    text.append(el('span', 'alert-list-chip-title', item.title), note);
    chip.append(text, el('span', 'alert-card-chevron'));
    if (item.status) {
      item.status(note);
    }
    row.append(disc, chip);
    // The whole row is the chip's: its icon too.
    row.addEventListener('click', e => {
      if (!chip.contains(e.target)) {
        chip.click();
      }
    });
    card.append(row);
  }
  // The foot's link to the whole list, where there is one list to go to.
  if (button) {
    const go = el('a', 'alert-list-button');
    go.href = href;
    go.append(el('span', '', button));
    const arrow = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    arrow.setAttribute('viewBox', '0 0 24 24');
    arrow.innerHTML = '<path d="M5 12h14M13 6l6 6-6 6"/>';
    go.append(arrow);
    card.append(go);
  }
  return card;
}

// The card an alert badge drops down: a pale yellow card with a gold bar
// at its left and a warning mark in a gold disc, the badge's message as
// its title, a line more when there is one, and a chevron at its end - the
// whole card the link to what resolves it, linkText its title.
export function alertCard(title, text, linkText, href) {
  const card = el('a', 'alert-card');
  card.href = href;
  card.title = linkText;
  const disc = el('span', 'alert-card-disc');
  disc.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/><line x1="12" x2="12" y1="9" y2="13"/><line x1="12" x2="12.01" y1="17" y2="17"/></svg>';
  const words = el('span', 'alert-card-words');
  words.append(el('span', 'alert-card-title', title));
  if (text) {
    words.append(el('span', 'alert-card-text', text));
  }
  card.append(disc, words, el('span', 'alert-card-chevron'));
  return card;
}

// Fills the toolbar's alert badges from the directory's reckoning - the
// count of things to update for the new year, and the privacy-mismatch
// triangle - linking each across to the page in Who? that resolves it, and
// hanging a card off each that says so on hover. Who? itself reckons these
// client-side, with the checklist itself under its count, and does not call
// this.
// bellStale puts a bell in each of the directory's count badges - Who?'s own
// and the one every other app draws - so it wears the count on a dot at
// its corner, as the RSVP and approvals badges do.
function bellStale() {
  for (const badge of document.querySelectorAll('.stale-alert')) {
    if (badge.querySelector('svg')) {
      continue;
    }
    const bell = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    bell.setAttribute('viewBox', '0 0 24 24');
    bell.setAttribute('aria-hidden', 'true');
    bell.innerHTML = '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>';
    badge.prepend(bell);
  }
}

// privacyCount puts on the triangle how many details Veracross shows that
// the directory hides - the address, the phone - on a dot at its corner,
// as the other badges carry theirs; Who? calls it with its own reckoning.
export function privacyCount(n) {
  for (const badge of document.querySelectorAll('.privacy-alert')) {
    let dot = badge.querySelector('.privacy-count');
    if (!dot) {
      dot = el('span', 'privacy-count');
      badge.append(dot);
    }
    dot.textContent = String(n);
    dot.hidden = !n;
  }
}

// privacyCard is the triangle's card: how many details Veracross shows that
// Helios Who hides, and a row for each - the address, the phone number -
// to My Privacy, where they are set. fields are the server's words
// ("address", "phone") or Who?'s own ("address", "phone number").
const privacyNames = {address: 'Address', phone: 'Phone number', 'phone number': 'Phone number'};

export function privacyCard(fields, href, button = 'Review My Privacy in Helios Who') {
  return alertList({
    count: fields.length,
    words: fields.length === 1 ? 'privacy setting doesn\u2019t match Veracross' : 'privacy settings don\u2019t match Veracross',
    items: fields.map(f => ({title: `${privacyNames[f] || f} mismatch`, note: 'Visible on Veracross, hidden in Helios Who', href, icon: f === 'address' ? alertIcons.address : alertIcons.phone})),
    icon: '<svg viewBox="0 0 24 24"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/><line x1="12" x2="12" y1="9" y2="13"/><line x1="12" x2="12.01" y1="17" y2="17"/></svg>',
    button,
    href,
    tone: 'yellow',
  });
}

export function renderAlerts({stale = [], privacy = []} = {}) {
  bellStale();
  const fields = Array.isArray(privacy) ? privacy : [];
  const updates = Array.isArray(stale) ? stale : [];
  privacyCount(fields.length);
  const who = appOrigin('who');
  // The bell's card lists each thing to update - "Sam's photo", "Family
  // photo" - to My Family, where it is done.
  for (const badge of document.querySelectorAll('.stale-alert')) {
    badge.hidden = !updates.length;
    badge.href = who + '/my-family';
    badge.setAttribute('aria-label', `${updates.length} thing${updates.length === 1 ? '' : 's'} to update for the new year`);
    badge.removeAttribute('title');
    alertMenu(badge, () => alertList({
      count: updates.length,
      words: updates.length === 1 ? 'thing to update for the new year' : 'things to update for the new year',
      items: updates.map(label => ({title: label, note: 'Update it for the new year', href: badge.href, icon: updateIcon(label)})),
      icon: '<svg viewBox="0 0 24 24"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>',
      button: 'Open My Family in Helios Who',
      href: badge.href,
    }));
  }
  for (const count of document.querySelectorAll('.stale-count')) {
    count.textContent = String(updates.length);
  }
  for (const badge of document.querySelectorAll('.privacy-alert')) {
    badge.hidden = !fields.length;
    badge.href = who + '/my-privacy';
    badge.setAttribute('aria-label', 'Your privacy settings don\u2019t match Veracross');
    badge.removeAttribute('title');
    alertMenu(badge, () => privacyCard(fields, badge.href));
  }
}

// The RSVP badge, in every app's bar before the other alerts while any
// invitation waits for the viewer's reply: a calendar with their count on
// a red dot, going to the RSVP list on Helios When, and under it - on the
// same hover as the other badges - a card listing each, soonest first, to
// its page there. The page's own host answers (/api/apps/rsvp, the
// calendar's, served on every app), so it is the same list everywhere.
const rsvpDay = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric'});

function initRSVP() {
  const user = document.querySelector('#user');
  if (!user || document.querySelector('.rsvp-alert')) {
    return;
  }
  fetch('/api/apps/rsvp').then(res => res.ok ? res.json() : null).then(view => {
    const waiting = (view && view.waiting) || [];
    if (!waiting.length) {
      return;
    }
    const when = appOrigin('calendar');
    const wrap = el('span', 'topbar-alert-wrap');
    const badge = el('a', 'topbar-alert rsvp-alert');
    badge.href = when + '/mine/rsvp';
    const words = `${waiting.length} ${waiting.length === 1 ? 'invitation waits' : 'invitations wait'} for your reply`;
    badge.setAttribute('aria-label', words);
    const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    icon.setAttribute('viewBox', '0 0 24 24');
    icon.innerHTML = '<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/>';
    badge.append(icon, el('span', 'rsvp-count', String(waiting.length)));
    wrap.append(badge);
    // First among the bar's badges, before the directory's.
    // Beside the bar's other badges, not inside one: Who? keeps its count in
    // a box of its own (.stale-wrap).
    const first = user.parentElement.querySelector('.topbar-alert');
    (first ? first.closest('.topbar-alert-wrap, .stale-wrap') || first : user).before(wrap);
    alertMenu(badge, () => alertList({
      count: waiting.length,
      words: waiting.length === 1 ? 'invitation waits for your reply' : 'invitations wait for your reply',
      items: waiting.map(r => {
        const day = new Date(r.start.slice(0, 10) + 'T12:00:00');
        const hours = r.allDay || r.start.length < 16 ? '' : ' \u00b7 ' + new Date(r.start.replace(' ', 'T')).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
        return {title: r.title, note: rsvpDay.format(day) + hours, href: when + r.path};
      }),
      icon: '<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/><circle cx="12" cy="15.5" r="1.6"/></svg>',
      button: 'Open RSVP in Helios When',
      href: badge.href,
    }));
  }).catch(() => {});
}

// The approvals badge, for an admin of any app, in every app's bar while
// anything waits for their approval - with Super Admin Mode on or off,
// approvals being an admin's alert: an hourglass with the count on an amber
// dot, going to the approvals of the app with the first of them, and under
// it a card listing each under its app's name, soonest first, to its page
// there (/api/apps/approvals, served on every app).
const approvalLists = {team: '/approvals', celebrate: '/approvals', calendar: '/admin'};
const approvalApps = {team: 'HCA-Team', celebrate: 'Helios Celebrate', calendar: 'Helios When'};

function initApprovals() {
  const user = document.querySelector('#user');
  if (!user || document.querySelector('.approvals-alert')) {
    return;
  }
  fetch('/api/apps/approvals').then(res => res.ok ? res.json() : null).then(view => {
    const waiting = (view && view.waiting) || [];
    if (!waiting.length) {
      return;
    }
    const wrap = el('span', 'topbar-alert-wrap');
    const badge = el('a', 'topbar-alert approvals-alert');
    badge.href = appOrigin(waiting[0].app) + approvalLists[waiting[0].app];
    const words = `${waiting.length} ${waiting.length === 1 ? 'thing waits' : 'things wait'} for your approval`;
    badge.setAttribute('aria-label', words);
    const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    icon.setAttribute('viewBox', '0 0 24 24');
    icon.innerHTML = '<path d="M6 3h12M6 21h12M7 3c0 5 5 6 5 9s-5 4-5 9M17 3c0 5-5 6-5 9s5 4 5 9"/>';
    badge.append(icon, el('span', 'approvals-count', String(waiting.length)));
    wrap.append(badge);
    // Beside the RSVP badge when there is one, first among the rest.
    const rsvp = document.querySelector('.rsvp-alert');
    const first = user.parentElement.querySelector('.topbar-alert');
    (rsvp ? rsvp.closest('.topbar-alert-wrap') : first ? first.closest('.topbar-alert-wrap, .stale-wrap') || first : user).before(wrap);
    alertMenu(badge, () => alertList({
      count: waiting.length,
      words: waiting.length === 1 ? 'thing waits for your approval' : 'things wait for your approval',
      items: waiting.map(a => ({title: a.title, note: approvalApps[a.app] + (a.start ? ' \u00b7 ' + rsvpDay.format(new Date(a.start.slice(0, 10) + 'T12:00:00')) : ''), href: appOrigin(a.app) + a.path, icon: alertIcons[a.app]})),
      icon: '<svg viewBox="0 0 24 24"><path d="M6 3h12M6 21h12M7 3c0 5 5 6 5 9s-5 4-5 9M17 3c0 5-5 6-5 9s5 4 5 9"/></svg>',
      // No foot: the things come from several apps, each row to its own.
      tone: 'amber',
    }));
  }).catch(() => {});
}

// The late-birthdays badge, in every app's bar while a birthday step the
// viewer owes is past its day - a birthday assigned to them whose charity is
// not in by its due-by day, or whose outreach is past the day to ask; or, on
// the comms team, a birthday past its newsletter not yet marked used; a birthday admin sees everyone's, the unassigned included, with or
// without Super Admin Mode, as approvals. A cake with the count on a red
// dot, to Birthday's My Jobs (an admin's, Process), and under it a card
// listing each, oldest first, to the staff member's page there
// (/api/apps/late, served on every app).
const lateCake = '<path d="M4 21V13a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v8"/><path d="M4 16c1.3 0 1.3 1 2.7 1s1.3-1 2.6-1 1.3 1 2.7 1 1.3-1 2.6-1 1.3 1 2.7 1 1.3-1 2.7-1"/><path d="M2 21h20M12 11V7"/><path d="M12 7c-1.1 0-2-.9-2-2 0-1.4 2-3 2-3s2 1.6 2 3c0 1.1-.9 2-2 2z"/>';
const lateSteps = {outreach: 'Outreach was due', info: 'Birthday info was due', newsletter: 'Newsletter went out'};

function initLate() {
  const user = document.querySelector('#user');
  if (!user || document.querySelector('.late-alert')) {
    return;
  }
  fetch('/api/apps/late').then(res => res.ok ? res.json() : null).then(view => {
    const late = (view && view.late) || [];
    if (!late.length) {
      return;
    }
    const birthday = appOrigin('birthday');
    const wrap = el('span', 'topbar-alert-wrap');
    const badge = el('a', 'topbar-alert late-alert');
    badge.href = birthday + (view.admin ? '/process' : '/jobs');
    const words = `${late.length} birthday ${late.length === 1 ? 'step is' : 'steps are'} late`;
    badge.setAttribute('aria-label', words);
    const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    icon.setAttribute('viewBox', '0 0 24 24');
    icon.innerHTML = lateCake;
    badge.append(icon, el('span', 'late-count', String(late.length)));
    wrap.append(badge);
    // First among the bar's badges: it is the most overdue thing there.
    const first = user.parentElement.querySelector('.topbar-alert');
    (first ? first.closest('.topbar-alert-wrap, .stale-wrap') || first : user).before(wrap);
    const day = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});
    alertMenu(badge, () => alertList({
      count: late.length,
      words: late.length === 1 ? 'birthday step is late' : 'birthday steps are late',
      items: late.map(l => {
        let note = `${lateSteps[l.step] || 'Due'} ${day.format(new Date(l.due + 'T12:00:00'))}`;
        // An admin sees whose each assignee's step is.
        if (view.admin && l.step !== 'newsletter') {
          note += ' \u00b7 ' + (l.assignee || 'Unassigned');
        }
        return {title: l.name, note, href: birthday + l.path};
      }),
      icon: '<svg viewBox="0 0 24 24">' + lateCake + '</svg>',
      button: view.admin ? 'Open Process in Birthday' : 'Open My Jobs in Birthday',
      href: badge.href,
    }));
  }).catch(() => {});
}

// Spoof Mode's switch, for super admins: an eye beside the avatar, a red
// pill saying whom while it is on, and under it - on the same hover as the
// bar's other menus - Stop, the last five people viewed as, and a search of
// the whole directory. Sign-in answers for all of it (/auth/spoof,
// internal/auth) as the person really signed in, so the switch is there and
// the way back stays open whoever the page is drawn for; a page drawn for
// someone who may not spoof gets no switch at all. Starting lands on the
// app's front page, since the person viewed as may not be allowed where the
// admin was; stopping reloads in place.
export function initSpoof() {
  const user = document.querySelector('#user');
  if (!user) {
    return;
  }
  fetch('/auth/spoof').then(res => res.ok ? res.json() : null).then(state => {
    if (state && state.quan) {
      offerQuan();
    }
    if (state && state.canSpoof) {
      buildSpoof(user, state);
    }
  }).catch(() => {});
}

function buildSpoof(user, state) {
  const wrap = el('span', 'spoof-wrap');
  const pill = el('span', 'spoof-pill');
  const button = el('button', 'spoof-button');
  button.type = 'button';
  button.setAttribute('aria-label', 'Spoof Mode');
  button.setAttribute('aria-haspopup', 'menu');
  button.title = 'Spoof Mode: view every app as someone else';
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
  button.append(eye);
  pill.append(button);
  const menu = el('div', 'spoof-menu');
  menu.hidden = true;
  wrap.append(pill, menu);
  // While viewing as someone else the whole bar goes hot pink, in every app
  // and every theme, so it cannot be forgotten whose account this is.
  document.body.classList.toggle('is-spoofing', Boolean(state.spoofing));
  if (state.spoofing) {
    wrap.classList.add('is-on');
    const as = el('span', 'spoof-as');
    as.append(el('span', 'spoof-as-lead', 'Viewing as '), el('b', '', state.spoofing.name));
    button.append(as);
    const stop = el('button', 'spoof-stop', '×');
    stop.type = 'button';
    stop.title = 'Stop viewing as ' + state.spoofing.name;
    stop.setAttribute('aria-label', stop.title);
    stop.addEventListener('click', () => setSpoof(''));
    pill.append(stop);
  }
  user.before(wrap);

  // The menu: Stop while viewing as someone, the recent five, then the search.
  if (state.spoofing) {
    const head = el('div', 'spoof-head');
    head.append(el('span', '', 'Viewing as '), el('b', '', state.spoofing.name));
    const stop = el('button', 'spoof-head-stop', 'Stop');
    stop.type = 'button';
    stop.addEventListener('click', () => setSpoof(''));
    head.append(stop);
    menu.append(head);
  }
  if (state.recent.length) {
    menu.append(el('div', 'spoof-section', 'Recent'));
    for (const p of state.recent) {
      menu.append(spoofRow(p, state.spoofing && p.email === state.spoofing.email));
    }
  }
  menu.append(el('div', 'spoof-section', state.recent.length ? 'Someone else' : 'View as'));
  const search = el('div', 'spoof-search');
  const input = document.createElement('input');
  input.type = 'search';
  input.placeholder = 'Search by name or email…';
  input.autocomplete = 'off';
  input.setAttribute('aria-label', 'Search for someone to view as');
  search.append(input);
  const results = el('div', 'spoof-results');
  menu.append(search, results);

  // The directory comes over once, the first time the menu opens; the box
  // filters it by name, address or the word that places someone, eight
  // rows at a time, and Enter takes the first (or the one arrowed to).
  let people = null;
  let active = -1;
  const load = () => {
    if (people) {
      return;
    }
    people = [];
    fetch('/auth/spoof/people').then(res => res.ok ? res.json() : []).then(list => {
      people = list;
      filter();
    }).catch(() => {});
  };
  const filter = () => {
    const q = input.value.trim().toLowerCase();
    results.replaceChildren();
    active = -1;
    if (!q) {
      return;
    }
    const found = (people || []).filter(p => p.name.toLowerCase().includes(q) || p.email.toLowerCase().includes(q) || (p.words || '').toLowerCase().includes(q)).slice(0, 8);
    if (!found.length) {
      results.append(el('div', 'spoof-empty', people && people.length ? 'Nobody matches.' : 'Loading…'));
      return;
    }
    for (const p of found) {
      results.append(spoofRow(p, false));
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

  const open = () => {
    closeUserMenus();
    closeAppSwitches();
    menu.hidden = false;
    load();
  };
  // Leaving the menu with the mouse does not close it while the search box
  // has focus - the mouse wanders while typing - so a click elsewhere, or
  // Escape, is what closes it then.
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

// One person of the switch's menu: their name over the word that places
// them, or their address; the one being viewed as is marked.
function spoofRow(p, isCurrent) {
  const row = el('button', 'spoof-row' + (isCurrent ? ' is-current' : ''));
  row.type = 'button';
  row.append(el('span', 'spoof-row-name', p.name), el('span', 'spoof-row-words', p.words || p.email));
  row.addEventListener('click', () => setSpoof(p.email));
  return row;
}

async function setSpoof(email) {
  try {
    const res = await fetch('/auth/spoof', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({email})});
    if (!res.ok) {
      toast(await res.text());
      return;
    }
  } catch {
    toast('Couldn’t switch; check your connection and try again.');
    return;
  }
  if (email) {
    location.href = '/';
  } else {
    location.reload();
  }
}

// Points every View Profile row of the account menu at the signed-in
// person's page in Who?, whose address is their email's local part. Who?
// fills its own, from its model.
export function renderProfileLink(email) {
  const slug = email.split('@')[0];
  for (const link of document.querySelectorAll('.user-menu-profile')) {
    link.href = appOrigin('who') + '/people/' + encodeURIComponent(slug);
  }
}

// markSuper shows or hides the avatar's red ring for Super Admin Mode.
export function markSuper(on) {
  document.body.classList.toggle('is-super', Boolean(on));
}

// Super Admin Mode's switch, for whoever is on the app's admin list: a pencil
// in the bar, before Spoof Mode's eye and the avatar - teal while the mode is
// off, white on red while it is on - and the avatar's ring to match
// (markSuper). Each app calls it whenever it draws the account, with whether
// to offer it, whether it is on, and what a click does with the new state;
// the button is made once and a click goes to the latest handler.
let superButton = null;
let superToggle = () => {};

export function renderSuperToggle({show, on, onToggle}) {
  const user = document.querySelector('#user');
  if (!user) {
    return;
  }
  if (!superButton) {
    superButton = el('button', 'super-toggle');
    superButton.type = 'button';
    const pencil = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    pencil.setAttribute('viewBox', '0 0 24 24');
    pencil.setAttribute('aria-hidden', 'true');
    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('d', 'M17 3a2.85 2.85 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z');
    const edge = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    edge.setAttribute('d', 'm15 5 4 4');
    pencil.append(path, edge);
    superButton.append(pencil);
    superButton.addEventListener('click', () => superToggle(!superButton.classList.contains('is-on')));
    // Spoof Mode's eye always lands right before the avatar, whenever its
    // fetch comes back, so the pencil goes ahead of it either way.
    (user.parentElement.querySelector('.spoof-wrap') || user).before(superButton);
  }
  superToggle = onToggle;
  on = Boolean(show && on);
  superButton.hidden = !show;
  superButton.classList.toggle('is-on', on);
  superButton.setAttribute('aria-pressed', String(on));
  superButton.setAttribute('aria-label', 'Super Admin Mode');
  superButton.title = on ? 'Super Admin Mode is on: click to turn it off' : 'Super Admin Mode: edit anything';
  markSuper(on);
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
