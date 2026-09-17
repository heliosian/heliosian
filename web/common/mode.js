// Dark mode, the same across every app: the choice - light, dark, or the
// device's setting - lives in a cookie on the parent domain, so it is one
// choice for heliosian.com and every app under it, and each page applies
// it before its stylesheet paints (the inline script in every index.html
// repeats the read, since this module loads later). A dark page carries
// data-theme="dark" on its root; light carries nothing. Quan mode is the
// easter egg beside them (quan.css): found by typing "quan" on a page or
// opening one with ?mode=quan, and offered in the menu only while it is
// on - except to the super admins and anyone with "quan" in their address,
// who always see the choice and are switched onto it once, whatever they
// had picked, so they know it is there; from then on their choice stands
// (offerQuan, which the toolbar calls once sign-in says so).
const cookie = 'heliosian-mode';
const modes = 'light|dark|system|quan';
// The second cookie, set the once Quan has been shown, so it is never
// forced again.
const shownCookie = 'heliosian-quan-shown';

// The cookie sits on the root domain - heliosian.com - so heliosian.com,
// who.heliosian.com and the local hosts under it all read one choice; a
// bare host or an address gets no domain.
function domain() {
  const labels = location.hostname.split('.');
  if (labels.length < 2 || /^\d+$/.test(labels[labels.length - 1])) {
    return '';
  }
  return '.' + labels.slice(-2).join('.');
}

// Where earlier builds left the cookie - on the host alone, and one label
// up - which would shadow the root one; setMode clears those.
function strays() {
  const labels = location.hostname.split('.');
  const places = [''];
  if (labels.length > 2) {
    places.push('.' + labels.slice(1).join('.'));
  }
  return places.filter(d => d !== domain());
}

export function readMode() {
  const m = document.cookie.match(new RegExp('(?:^|; )' + cookie + '=(' + modes + ')'));
  return m ? m[1] : 'system';
}

export function setMode(mode) {
  const d = domain();
  for (const stray of strays()) {
    document.cookie = `${cookie}=; path=/; max-age=0${stray ? '; domain=' + stray : ''}${location.protocol === 'https:' ? '; Secure' : ''}`;
  }
  document.cookie = `${cookie}=${mode}; path=/; max-age=31536000; SameSite=Lax${d ? '; domain=' + d : ''}${location.protocol === 'https:' ? '; Secure' : ''}`;
  applyMode();
}

export function applyMode() {
  const mode = readMode();
  const dark = mode === 'dark' || (mode === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  if (mode === 'quan') {
    document.documentElement.dataset.theme = 'quan';
  } else if (dark) {
    document.documentElement.dataset.theme = 'dark';
  } else {
    delete document.documentElement.dataset.theme;
  }
}

// offered says the menu shows Quan as a choice in its own right.
let offered = false;

// offerQuan is for the people Quan mode is meant for: the choice joins the
// menu for good, and the first time it is offered it is switched on over
// whatever they had, once - the menu is right there to change it back.
export function offerQuan() {
  offered = true;
  if (!new RegExp('(?:^|; )' + shownCookie + '=').test(document.cookie)) {
    const d = domain();
    document.cookie = `${shownCookie}=1; path=/; max-age=31536000; SameSite=Lax${d ? '; domain=' + d : ''}${location.protocol === 'https:' ? '; Secure' : ''}`;
    setMode('quan');
  }
  document.dispatchEvent(new CustomEvent('heliosian-mode'));
}

// The egg: the letters q-u-a-n typed in a row, anywhere but a field, turn
// Quan mode on - or, when it is on, off again to the device's setting.
let typed = '';
document.addEventListener('keydown', e => {
  if (e.metaKey || e.ctrlKey || e.altKey || e.key.length !== 1) {
    return;
  }
  const t = e.target;
  if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) {
    return;
  }
  typed = (typed + e.key.toLowerCase()).slice(-4);
  if (typed === 'quan') {
    typed = '';
    setMode(readMode() === 'quan' ? 'system' : 'quan');
    document.dispatchEvent(new CustomEvent('heliosian-mode'));
  }
});

// The device's setting can change while a page is open.
window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if (readMode() === 'system') {
    applyMode();
  }
});

// modeRow is the user menu's row for it: three small choices, the current
// one lit.
export function modeRow() {
  const row = document.createElement('div');
  row.className = 'user-menu-mode';
  const label = document.createElement('span');
  label.textContent = 'Appearance';
  const group = document.createElement('span');
  group.className = 'user-menu-mode-choices';
  const buttons = {};
  for (const [mode, words] of [['light', 'Light'], ['dark', 'Dark'], ['system', 'Auto'], ['quan', 'Quan']]) {
    const b = document.createElement('button');
    b.type = 'button';
    b.textContent = words;
    b.title = mode === 'system' ? 'Follow the device’s setting' : words + ' mode';
    b.addEventListener('click', e => {
      e.stopPropagation();
      setMode(mode);
      mark();
    });
    buttons[mode] = b;
    group.append(b);
  }
  // Quan's button shows while Quan is on - the way back, not the way in -
  // and always for those it is offered to.
  const mark = () => {
    const current = readMode();
    for (const [mode, b] of Object.entries(buttons)) {
      b.classList.toggle('is-on', mode === current);
    }
    buttons.quan.hidden = current !== 'quan' && !offered;
  };
  mark();
  document.addEventListener('heliosian-mode', mark);
  row.append(label, group);
  return row;
}
