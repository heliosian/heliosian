// Dark mode, the same across every app: the choice - light, dark, or the
// device's setting - lives in a cookie on the parent domain, so it is one
// choice for heliosian.com and every app under it, and each page applies
// it before its stylesheet paints (the inline script in every index.html
// repeats the read, since this module loads later). A dark page carries
// data-theme="dark" on its root; light carries nothing.
const cookie = 'heliosian-mode';

function domain() {
  const labels = location.hostname.split('.');
  // who.heliosian.com and who.local.heliosian.com share heliosian.com and
  // local.heliosian.com with their siblings; a bare host gets no domain.
  return labels.length > 2 ? '.' + labels.slice(1).join('.') : '';
}

export function readMode() {
  const m = document.cookie.match(new RegExp('(?:^|; )' + cookie + '=(light|dark|system)'));
  return m ? m[1] : 'system';
}

export function setMode(mode) {
  const d = domain();
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
