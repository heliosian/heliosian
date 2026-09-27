import {modeRow, offerQuan} from '/mode.js';
import {api} from '/api.js';

const homeApp = {key: 'home', name: 'Heliosian', tagline: 'Helios Community Apps'};

function tierLabels() {
  const labels = location.hostname.split('.');
  return labels.length > 2 ? labels.slice(1) : labels;
}

export function currentApp() {
  const labels = location.hostname.split('.');
  return labels.length > 2 ? labels[0] : 'home';
}

export function appOrigin(key) {
  const tier = tierLabels();
  const host = key === 'home' ? tier : [key, ...tier];
  return location.protocol + '//' + host.join('.') + (location.port ? ':' + location.port : '');
}

export function whoLink(email) {
  return appOrigin('who') + '/people/' + encodeURIComponent((email || '').split('@')[0]);
}

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

export function hoverClick(e) {
  if (matchMedia('(hover: none)').matches) {
    return false;
  }
  return !e.pointerType || e.pointerType === 'mouse';
}

export function initUserMenu() {
  const button = document.querySelector('#user');
  const menu = document.querySelector('#user-menu');
  initRSVP();
  initApprovals();
  initLate();
  bellStale();
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

function appRow(app, isCurrent) {
  const row = document.createElement('a');
  row.href = appOrigin(app.key);
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

export function el(tag, className, text) {
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
      await api('POST', '/api/feedback', {
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
      });
      hide();
      form.reset();
      prompt('bug');
      toast('Thanks, we got it.');
    } catch (err) {
      status.textContent = err.message;
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

const alertBuilds = new WeakMap();

export function alertMenu(badge, build) {
  alertBuilds.set(badge, build);
  const wrap = badge.parentElement;
  let menu = wrap.querySelector('.topbar-alert-menu');
  if (!menu) {
    menu = el('div', 'topbar-alert-menu');
    menu.hidden = true;
    wrap.append(menu);
    hoverMenu(badge, menu, () => openAlertMenu(badge, menu), () => {
      menu.hidden = true;
    });
  }
  return menu;
}

function openAlertMenu(badge, menu) {
  menu.replaceChildren(alertBuilds.get(badge)());
  menu.hidden = false;
  clampMenu(badge.parentElement, menu);
}

const shownAlerts = '.topbar-alert-wrap:not(.alerts-summary-wrap) > .topbar-alert:not([hidden])';
const alertCounts = '.rsvp-count, .approvals-count, .late-count, .stale-count, .privacy-count';

function alertSummary(user) {
  const existing = user.parentElement.querySelector('.alerts-summary');
  if (existing) {
    return existing;
  }
  const wrap = el('span', 'topbar-alert-wrap alerts-summary-wrap');
  const badge = el('button', 'topbar-alert alerts-summary');
  badge.type = 'button';
  badge.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>';
  badge.append(el('span', 'alerts-summary-count'));
  wrap.append(badge);
  (user.parentElement.querySelector('.topbar-alert-wrap') || user).before(wrap);
  const menu = alertMenu(badge, () => {
    const count = badge.querySelector('.alerts-summary-count').textContent;
    return alertList({
      count,
      words: count === '1' ? 'alert' : 'alerts',
      items: [...user.parentElement.querySelectorAll(shownAlerts)].map(other => ({
        title: other.getAttribute('aria-label'),
        href: other.href,
        icon: other.querySelector('svg').outerHTML,
      })),
      icon: badge.querySelector('svg').outerHTML,
    });
  });
  badge.addEventListener('click', e => {
    if (hoverClick(e)) {
      return;
    }
    e.stopPropagation();
    if (menu.hidden) {
      openAlertMenu(badge, menu);
    } else {
      menu.hidden = true;
    }
  });
  document.addEventListener('click', e => {
    if (!wrap.contains(e.target)) {
      menu.hidden = true;
    }
  });
  return badge;
}

function fitAlerts() {
  const bar = document.querySelector('.topbar');
  const user = document.querySelector('#user');
  if (!bar || !user) {
    return;
  }
  const summary = alertSummary(user);
  bar.classList.remove('alerts-collapsed');
  const shown = [...bar.querySelectorAll(shownAlerts)];
  const tile = bar.querySelector('.app-switch');
  const edge = bar.getBoundingClientRect().right - parseFloat(getComputedStyle(bar).paddingRight);
  if (shown.length < 2 || tile.getBoundingClientRect().right <= edge + 0.5) {
    return;
  }
  bar.classList.add('alerts-collapsed');
  const total = shown.reduce((sum, badge) => sum + (parseInt(badge.querySelector(alertCounts)?.textContent, 10) || 0), 0);
  summary.querySelector('.alerts-summary-count').textContent = String(total);
  summary.setAttribute('aria-label', `${total} alert${total === 1 ? '' : 's'}`);
}

window.addEventListener('resize', fitAlerts);

function clampMenu(wrap, menu) {
  const margin = 12;
  const wrapRect = wrap.getBoundingClientRect();
  const width = menu.offsetWidth;
  const left = Math.max(margin, Math.min(wrapRect.right - width, window.innerWidth - margin - width));
  menu.style.left = `${left - wrapRect.left}px`;
  menu.style.right = 'auto';
  menu.style.setProperty('--notch', `${wrapRect.left + wrapRect.width / 2 - left}px`);
}

export const alertIcons = {
  photo: '<svg viewBox="0 0 24 24"><path d="M4 8h3l2-3h6l2 3h3a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1z"/><circle cx="12" cy="13.5" r="3.5"/></svg>',
  family: '<svg viewBox="0 0 24 24"><circle cx="9" cy="8" r="3.2"/><path d="M3 20c0-3.6 2.7-6 6-6s6 2.4 6 6"/><circle cx="17" cy="9.5" r="2.6"/><path d="M15.5 14.3c3 .1 5.5 2.3 5.5 5.7"/></svg>',
  facts: '<svg viewBox="0 0 24 24"><rect x="5" y="3" width="14" height="18" rx="2"/><path d="M9 8h6M9 12h6M9 16h4"/></svg>',
  address: '<svg viewBox="0 0 24 24"><path d="M12 22s7-7.6 7-12a7 7 0 1 0-14 0c0 4.4 7 12 7 12z"/><circle cx="12" cy="10" r="2.5"/></svg>',
  phone: '<svg viewBox="0 0 24 24"><path d="M22 16.9v3a2 2 0 0 1-2.2 2 19.8 19.8 0 0 1-8.6-3.1 19.5 19.5 0 0 1-6-6A19.8 19.8 0 0 1 2.1 4.2 2 2 0 0 1 4.1 2h3a2 2 0 0 1 2 1.7c.1.9.4 1.8.7 2.7a2 2 0 0 1-.5 2.1L8 9.8a16 16 0 0 0 6 6l1.3-1.3a2 2 0 0 1 2.1-.4c.9.3 1.8.6 2.7.7a2 2 0 0 1 1.7 2z"/></svg>',
  team: '<svg viewBox="0 0 24 24"><circle cx="9" cy="8" r="3.2"/><path d="M3 20c0-3.6 2.7-6 6-6s6 2.4 6 6"/><path d="M16 5.2a3.2 3.2 0 0 1 0 5.6M18 14.4c2 .7 3 2.8 3 5.6"/></svg>',
  celebrate: '<svg viewBox="0 0 24 24"><path d="M3 9V7a1 1 0 0 1 1-1h16a1 1 0 0 1 1 1v2a2.5 2.5 0 0 0 0 5v2a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-2a2.5 2.5 0 0 0 0-5z"/><path d="M14 6v11" stroke-dasharray="2 2"/></svg>',
  when: '<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/></svg>',
};

function updateIcon(label) {
  if (/^family photo/i.test(label)) {
    return alertIcons.family;
  }
  return /facts$/i.test(label) ? alertIcons.facts : alertIcons.photo;
}

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
    row.addEventListener('click', e => {
      if (!chip.contains(e.target)) {
        chip.click();
      }
    });
    card.append(row);
  }
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

export function renderAlerts({stale = [], privacy = [], broken = false} = {}) {
  bellStale();
  const fields = Array.isArray(privacy) ? privacy : [];
  const updates = Array.isArray(stale) ? stale : [];
  const count = broken ? '?' : String(updates.length);
  privacyCount(fields.length);
  const who = appOrigin('who');
  for (const badge of document.querySelectorAll('.stale-alert')) {
    badge.hidden = !broken && !updates.length;
    badge.href = who + '/my-family';
    badge.setAttribute('aria-label', broken ? 'Couldn’t check for things to update for the new year' : `${updates.length} thing${updates.length === 1 ? '' : 's'} to update for the new year`);
    badge.removeAttribute('title');
    alertMenu(badge, () => alertList({
      count,
      words: broken ? 'couldn’t check for things to update for the new year' : updates.length === 1 ? 'thing to update for the new year' : 'things to update for the new year',
      items: updates.map(label => ({title: label, note: 'Update it for the new year', href: badge.href, icon: updateIcon(label)})),
      icon: '<svg viewBox="0 0 24 24"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>',
      button: 'Open My Family in Helios Who',
      href: badge.href,
    }));
  }
  for (const node of document.querySelectorAll('.stale-count')) {
    node.textContent = count;
  }
  for (const badge of document.querySelectorAll('.privacy-alert')) {
    badge.hidden = !fields.length;
    badge.href = who + '/my-privacy';
    badge.setAttribute('aria-label', 'Your privacy settings don\u2019t match Veracross');
    badge.removeAttribute('title');
    alertMenu(badge, () => privacyCard(fields, badge.href));
  }
  fitAlerts();
}

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
    const when = appOrigin('when');
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
    const first = user.parentElement.querySelector('.topbar-alert');
    (first ? first.closest('.topbar-alert-wrap') || first : user).before(wrap);
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
    fitAlerts();
  }).catch(() => {});
}

const approvalLists = {team: '/approvals', celebrate: '/approvals', when: '/admin'};
const approvalApps = {team: 'HCA-Team', celebrate: 'Helios Celebrate', when: 'Helios When'};

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
    const rsvp = document.querySelector('.rsvp-alert');
    const first = user.parentElement.querySelector('.topbar-alert');
    (rsvp ? rsvp.closest('.topbar-alert-wrap') : first ? first.closest('.topbar-alert-wrap') || first : user).before(wrap);
    alertMenu(badge, () => alertList({
      count: waiting.length,
      words: waiting.length === 1 ? 'thing waits for your approval' : 'things wait for your approval',
      items: waiting.map(a => ({title: a.title, note: approvalApps[a.app] + (a.start ? ' \u00b7 ' + rsvpDay.format(new Date(a.start.slice(0, 10) + 'T12:00:00')) : ''), href: appOrigin(a.app) + a.path, icon: alertIcons[a.app]})),
      icon: '<svg viewBox="0 0 24 24"><path d="M6 3h12M6 21h12M7 3c0 5 5 6 5 9s-5 4-5 9M17 3c0 5-5 6-5 9s5 4 5 9"/></svg>',
      tone: 'amber',
    }));
    fitAlerts();
  }).catch(() => {});
}

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
    const first = user.parentElement.querySelector('.topbar-alert');
    (first ? first.closest('.topbar-alert-wrap') || first : user).before(wrap);
    const day = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});
    alertMenu(badge, () => alertList({
      count: late.length,
      words: late.length === 1 ? 'birthday step is late' : 'birthday steps are late',
      items: late.map(l => {
        let note = `${lateSteps[l.step] || 'Due'} ${day.format(new Date(l.due + 'T12:00:00'))}`;
        if (view.admin && l.step !== 'newsletter') {
          note += ' \u00b7 ' + (l.assignee || 'Unassigned');
        }
        return {title: l.name, note, href: birthday + l.path};
      }),
      icon: '<svg viewBox="0 0 24 24">' + lateCake + '</svg>',
      button: view.admin ? 'Open Process in Birthday' : 'Open My Jobs in Birthday',
      href: badge.href,
    }));
    fitAlerts();
  }).catch(() => {});
}

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
  fitAlerts();

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

  let people = null;
  let active = -1;
  const load = () => {
    if (people) {
      return;
    }
    people = [];
    api('GET', '/auth/spoof/people').then(list => {
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

function spoofRow(p, isCurrent) {
  const row = el('button', 'spoof-row' + (isCurrent ? ' is-current' : ''));
  row.type = 'button';
  row.append(el('span', 'spoof-row-name', p.name), el('span', 'spoof-row-words', p.words || p.email));
  row.addEventListener('click', () => setSpoof(p.email));
  return row;
}

async function setSpoof(email) {
  try {
    await api('POST', '/auth/spoof', {email});
  } catch (err) {
    toast(err.message);
    return;
  }
  if (email) {
    location.href = '/';
  } else {
    location.reload();
  }
}

export function renderProfileLink(email) {
  for (const link of document.querySelectorAll('.user-menu-profile')) {
    link.href = whoLink(email);
  }
}

export function markSuper(on) {
  document.body.classList.toggle('is-super', Boolean(on));
}

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
  fitAlerts();
}

export function isEditableTarget(target) {
  return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT' || target.isContentEditable;
}

export function onSlash(open) {
  document.addEventListener('keydown', e => {
    if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
      e.preventDefault();
      open();
    }
  });
}
