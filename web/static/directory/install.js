import {el, svg} from './dom.js';

// Neither iOS Safari nor Chrome expose an install API - the closest thing is
// pointing people at the share sheet's "Add to Home Screen" entry by hand.
// Android/desktop Chrome instead fire `beforeinstallprompt`, which we stash
// here so the card's button can trigger the real native prompt on demand.
let deferredInstallPrompt = null;
const INSTALL_PROMPT_DISMISS_KEY = 'installPromptDismissedAt';
const INSTALL_PROMPT_COOLDOWN_DAYS = 30;

function installPromptDismissedRecently() {
  try {
    const at = Number(localStorage.getItem(INSTALL_PROMPT_DISMISS_KEY));
    return Boolean(at) && Date.now() - at < INSTALL_PROMPT_COOLDOWN_DAYS * 24 * 60 * 60 * 1000;
  } catch (e) {
    return false;
  }
}

function dismissInstallPrompt() {
  try {
    localStorage.setItem(INSTALL_PROMPT_DISMISS_KEY, String(Date.now()));
  } catch (e) {
    // ignore - the card just may reappear sooner than intended
  }
}

function runningStandalone() {
  return window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
}

function iosDevice() {
  return /iphone|ipad|ipod/i.test(navigator.userAgent) ||
    (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
}

// beforeinstallprompt also fires on desktop Chrome/Edge, but "add to home
// screen" is a phone/tablet idea - desktop already has a real taskbar.
function mobileDevice() {
  return iosDevice() || /android/i.test(navigator.userAgent);
}

// iOS 26's Safari redesign folded the toolbar's standalone Share icon into a
// "..." overflow button, so the old "tap Share" instructions point at nothing
// on 26+. Below that, Share still has its own icon in the toolbar.
function iosMajorVersion() {
  const match = navigator.userAgent.match(/OS (\d+)_/) || navigator.userAgent.match(/Version\/(\d+)/);
  return match ? Number(match[1]) : 0;
}

window.addEventListener('beforeinstallprompt', (e) => {
  e.preventDefault();
  deferredInstallPrompt = e;
  maybeShowInstallPrompt();
});

window.addEventListener('appinstalled', () => {
  deferredInstallPrompt = null;
  dismissInstallPrompt();
  document.querySelector('.install-prompt-overlay')?.remove();
});

export function maybeShowInstallPrompt() {
  if (!mobileDevice() || runningStandalone() || installPromptDismissedRecently() ||
      document.querySelector('.install-prompt-overlay')) {
    return;
  }
  const isIOS = iosDevice();
  if (!isIOS && !deferredInstallPrompt) {
    return;
  }
  document.body.append(installPromptOverlay(isIOS));
}

function installPromptOverlay(isIOS) {
  const overlay = el('div', 'install-prompt-overlay');
  const card = el('div', 'install-prompt-card');

  const close = el('button', 'install-prompt-close', '×');
  close.type = 'button';
  close.setAttribute('aria-label', 'Dismiss');
  close.addEventListener('click', () => {
    overlay.remove();
    dismissInstallPrompt();
  });
  card.append(close);

  const icon = el('img', 'install-prompt-icon');
  icon.src = '/static/brand/icon-192.png';
  icon.alt = '';
  card.append(icon);

  card.append(el('div', 'install-prompt-title', 'Install Helios Who?'));
  card.append(el('div', 'install-prompt-desc',
    'Add this app to your home screen for easy access and a better experience.'));

  if (isIOS) {
    const hint = el('div', 'install-prompt-hint');
    if (iosMajorVersion() >= 26) {
      const dots = svg('ellipsis');
      dots.classList.add('install-prompt-dots');
      hint.append('Tap ', dots, ' then ', svg('upload'), ' Share, then “Add to Home Screen”');
    } else {
      hint.append('Tap ', svg('upload'), ' then “Add to Home Screen”');
    }
    card.append(hint);
  } else {
    const button = el('button', 'install-prompt-button', 'Install');
    button.type = 'button';
    button.addEventListener('click', async () => {
      overlay.remove();
      dismissInstallPrompt();
      if (deferredInstallPrompt) {
        deferredInstallPrompt.prompt();
        await deferredInstallPrompt.userChoice;
        deferredInstallPrompt = null;
      }
    });
    card.append(button);
  }

  overlay.append(card);
  return overlay;
}
