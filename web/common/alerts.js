import {api} from '/api.js';
import {el} from '/elements.js';
import {appOrigin, hoverMenu, hoverClick} from '/appswitch.js';
import {noteError} from '/feedback.js';

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

export function fitAlerts() {
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
    words: fields.length === 1 ? 'privacy setting doesn’t match Veracross' : 'privacy settings don’t match Veracross',
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
    badge.setAttribute('aria-label', 'Your privacy settings don’t match Veracross');
    badge.removeAttribute('title');
    alertMenu(badge, () => privacyCard(fields, badge.href));
  }
  fitAlerts();
}

export function initAlerts() {
  bellStale();
  initRSVP();
  initApprovals();
  initLate();
}

const rsvpDay = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric'});

function initRSVP() {
  const user = document.querySelector('#user');
  if (!user || document.querySelector('.rsvp-alert')) {
    return;
  }
  api('GET', '/api/apps/rsvp').then(view => {
    const waiting = view.waiting || [];
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
        const hours = r.allDay || r.start.length < 16 ? '' : ' · ' + new Date(r.start.replace(' ', 'T')).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
        return {title: r.title, note: rsvpDay.format(day) + hours, href: when + r.path};
      }),
      icon: '<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/><circle cx="12" cy="15.5" r="1.6"/></svg>',
      button: 'Open RSVP in Helios When',
      href: badge.href,
    }));
    fitAlerts();
  }).catch(err => noteError('/api/apps/rsvp: ' + err.message));
}

const approvalLists = {team: '/approvals', celebrate: '/approvals', when: '/admin'};
const approvalApps = {team: 'HCA-Team', celebrate: 'Helios Celebrate', when: 'Helios When'};

function initApprovals() {
  const user = document.querySelector('#user');
  if (!user || document.querySelector('.approvals-alert')) {
    return;
  }
  api('GET', '/api/apps/approvals').then(view => {
    const waiting = view.waiting || [];
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
      items: waiting.map(a => ({title: a.title, note: approvalApps[a.app] + (a.start ? ' · ' + rsvpDay.format(new Date(a.start.slice(0, 10) + 'T12:00:00')) : ''), href: appOrigin(a.app) + a.path, icon: alertIcons[a.app]})),
      icon: '<svg viewBox="0 0 24 24"><path d="M6 3h12M6 21h12M7 3c0 5 5 6 5 9s-5 4-5 9M17 3c0 5-5 6-5 9s5 4 5 9"/></svg>',
      tone: 'amber',
    }));
    fitAlerts();
  }).catch(err => noteError('/api/apps/approvals: ' + err.message));
}

const lateCake = '<path d="M4 21V13a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v8"/><path d="M4 16c1.3 0 1.3 1 2.7 1s1.3-1 2.6-1 1.3 1 2.7 1 1.3-1 2.6-1 1.3 1 2.7 1 1.3-1 2.7-1"/><path d="M2 21h20M12 11V7"/><path d="M12 7c-1.1 0-2-.9-2-2 0-1.4 2-3 2-3s2 1.6 2 3c0 1.1-.9 2-2 2z"/>';
const lateSteps = {outreach: 'Outreach was due', info: 'Birthday info was due', newsletter: 'Newsletter went out'};

function initLate() {
  const user = document.querySelector('#user');
  if (!user || document.querySelector('.late-alert')) {
    return;
  }
  api('GET', '/api/apps/late').then(view => {
    const late = view.late || [];
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
          note += ' · ' + (l.assignee || 'Unassigned');
        }
        return {title: l.name, note, href: birthday + l.path};
      }),
      icon: '<svg viewBox="0 0 24 24">' + lateCake + '</svg>',
      button: view.admin ? 'Open Process in Birthday' : 'Open My Jobs in Birthday',
      href: badge.href,
    }));
    fitAlerts();
  }).catch(err => noteError('/api/apps/late: ' + err.message));
}
