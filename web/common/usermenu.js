import {modeRow} from '/mode.js';
import {el} from '/elements.js';
import {whoLink, hoverMenu, hoverClick, closeBarMenus} from '/appswitch.js';
import {fitAlerts} from '/alerts.js';

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
