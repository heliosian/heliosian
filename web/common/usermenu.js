import {modeRow} from '/mode.js';
import {whoLink, hoverMenu, hoverClick, closeBarMenus} from '/appswitch.js';

export function initUserMenu() {
  const button = document.querySelector('#user');
  const menu = document.querySelector('#user-menu');
  if (!menu.querySelector('.user-menu-mode')) {
    const signOut = menu.querySelector('form');
    menu.insertBefore(modeRow(), signOut || null);
  }
  const open = () => {
    closeBarMenus();
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

export function renderProfileLink(email) {
  for (const link of document.querySelectorAll('.user-menu-profile')) {
    link.href = whoLink(email);
  }
}

