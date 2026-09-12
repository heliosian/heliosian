import {me, isAdmin} from './state.js';
import {el, svg, link} from './dom.js';

const appName = 'Helios Staff Birthdays';

const primary = [
  {href: '/', icon: 'jobs', label: 'My Jobs'},
  {href: '/process', icon: 'process', label: 'Process'},
  {href: '/calendar', icon: 'calendar', label: 'Calendar'},
];

const more = [
  {href: '/charities', icon: 'gift', label: 'Charities'},
  {href: '/newsletters', icon: 'newsletter', label: 'Newsletters'},
  {href: '/skipped', icon: 'skipped', label: 'Skipped'},
  {href: '/admin', icon: 'tools', label: 'Admin Tools', admin: true},
];

function active(href) {
  const path = location.pathname;
  if (href === '/') {
    return path === '/';
  }
  if (href === '/process') {
    return path === '/process' || path.startsWith('/staff/');
  }
  return path === href || path.startsWith(href + '/');
}

function navLink(item) {
  const a = link(item.href, active(item.href) ? 'is-active' : '');
  a.append(svg(item.icon), el('span', '', item.label));
  return a;
}

function closeMenus() {
  for (const menu of document.querySelectorAll('.more-menu, .row-menu, #user-menu')) {
    menu.hidden = true;
  }
}

function renderNav() {
  const nav = document.querySelector('#nav');
  nav.replaceChildren();
  for (const item of primary) {
    nav.append(navLink(item));
  }
  const wrap = el('div', 'more-wrap');
  const button = el('button', 'more-button' + (more.some(item => active(item.href)) ? ' is-active' : ''));
  button.type = 'button';
  button.append(svg('menu'), el('span', '', 'More'));
  const menu = el('div', 'more-menu');
  menu.hidden = true;
  for (const item of more.filter(item => !item.admin || isAdmin())) {
    menu.append(navLink(item));
  }
  button.addEventListener('click', e => {
    e.stopPropagation();
    const opening = menu.hidden;
    closeMenus();
    menu.hidden = !opening;
  });
  wrap.append(button, menu);
  nav.append(wrap);
}

function renderTabbar() {
  const bar = document.querySelector('#tabbar');
  bar.replaceChildren();
  for (const item of primary) {
    bar.append(navLink(item));
  }
}

function renderDrawer() {
  const drawer = document.querySelector('#drawer');
  drawer.replaceChildren();
  const head = el('div', 'drawer-head');
  const icon = el('img');
  icon.src = '/brand/icon-192.png';
  icon.alt = '';
  const close = el('button', 'icon-button');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  close.append(svg('close'));
  close.addEventListener('click', closeDrawer);
  head.append(icon, el('span', '', appName), close);
  drawer.append(head);
  for (const item of [...primary, ...more].filter(item => !item.admin || isAdmin())) {
    drawer.append(navLink(item));
  }
  const user = el('div', 'drawer-user');
  user.append(el('div', 'name', me().name), el('div', 'email', me().email));
  const form = el('form');
  form.method = 'post';
  form.action = '/auth/logout';
  form.append(el('button', 'button button-secondary button-small', 'Sign Out'));
  user.append(form);
  drawer.append(user);
}

export function openDrawer() {
  renderDrawer();
  document.querySelector('#drawer-overlay').hidden = false;
}

export function closeDrawer() {
  document.querySelector('#drawer-overlay').hidden = true;
}

function renderUser() {
  const user = me();
  const avatar = document.querySelector('.user-avatar');
  avatar.replaceChildren();
  if (user.photoUrl) {
    const img = el('img');
    img.src = user.photoUrl;
    img.alt = '';
    avatar.append(img);
  } else {
    avatar.textContent = user.initial;
  }
  document.querySelector('.user-menu-email').textContent = user.email;
  document.querySelector('.user-menu-admin').hidden = !isAdmin();
}

export function setTitle(title) {
  document.querySelector('#mobile-title').textContent = title;
  document.title = title === appName ? title : `${title} · ${appName}`;
}

export function renderChrome() {
  renderUser();
  renderNav();
  renderTabbar();
}

export function initChrome() {
  document.querySelector('#menu-button').append(svg('menu'));
  document.querySelector('#menu-button').addEventListener('click', openDrawer);
  document.querySelector('#drawer-overlay').addEventListener('click', e => {
    if (e.target === e.currentTarget) {
      closeDrawer();
    }
  });
  document.querySelector('#drawer').addEventListener('click', e => {
    if (e.target.closest('a')) {
      closeDrawer();
    }
  });
  const menu = document.querySelector('#user-menu');
  document.querySelector('#user').addEventListener('click', e => {
    e.stopPropagation();
    const opening = menu.hidden;
    closeMenus();
    menu.hidden = !opening;
  });
  document.addEventListener('click', closeMenus);
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      closeMenus();
      closeDrawer();
    }
  });
}
