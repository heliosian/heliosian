const cookie = 'heliosian-mode';
const modes = 'light|dark|system|quan';

export function domain() {
  const labels = location.hostname.split('.');
  if (labels.length < 2 || /^\d+$/.test(labels[labels.length - 1])) {
    return '';
  }
  return '.' + labels.slice(-2).join('.');
}

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

let offered = false;

export function offerQuan() {
  offered = true;
  document.dispatchEvent(new CustomEvent('heliosian-mode'));
}

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

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if (readMode() === 'system') {
    applyMode();
  }
});

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
