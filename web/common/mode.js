// Dark mode, the same across every app: the choice - light, dark, or the
// device's setting - lives in a cookie on the parent domain, so it is one
// choice for heliosian.com and every app under it, and each page applies
// it before its stylesheet paints (the inline script in every index.html
// repeats the read, since this module loads later). A dark page carries
// data-theme="dark" on its root; light carries nothing.
const cookie = 'heliosian-mode';

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
  const m = document.cookie.match(new RegExp('(?:^|; )' + cookie + '=(light|dark|system)'));
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
  if (dark) {
    document.documentElement.dataset.theme = 'dark';
  } else {
    delete document.documentElement.dataset.theme;
  }
}

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
  for (const [mode, words] of [['light', 'Light'], ['dark', 'Dark'], ['system', 'Auto']]) {
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
  const mark = () => {
    const current = readMode();
    for (const [mode, b] of Object.entries(buttons)) {
      b.classList.toggle('is-on', mode === current);
    }
  };
  mark();
  row.append(label, group);
  return row;
}
