let all = [];

const roles = ['kicker', 'title', 'day', 'hours', 'place', 'message', 'hosts'];

function merged(base, over) {
  const out = {...base};
  for (const [key, value] of Object.entries(over)) {
    out[key] = value && typeof value === 'object' && !Array.isArray(value) ? merged(base[key], value) : value;
  }
  return out;
}

function color(c) {
  return c === 'ink' ? 'var(--paper-ink)' : c === 'accent' ? 'var(--accent)' : c;
}

function font(f) {
  return f === 'display' ? 'var(--display)' : f === 'body' ? 'var(--body)' : f;
}

const sealRadius = {circle: '50%', square: '10px', stamp: '3px'};

function cssVars(t) {
  const vars = {
    '--display': t.fonts.display, '--body': t.fonts.body,
    '--env': t.envelope.body, '--env-side': t.envelope.side, '--env-front': t.envelope.front, '--env-flap': t.envelope.flap, '--liner': t.envelope.liner, '--env-radius': t.envelope.radius,
    '--seal': t.seal.color, '--seal-ink': t.seal.ink, '--seal-radius': sealRadius[t.seal.shape], '--seal-outline': t.seal.shape === 'stamp' ? '2px dashed currentColor' : 'none', '--seal-picture': t.seal.picture,
    '--paper': t.card.background, '--paper-ink': t.card.ink, '--accent': t.card.accent, '--accent-ink': t.card.accentInk,
    '--card-radius': t.card.radius, '--card-padding': t.card.padding, '--card-align': t.card.align, '--card-items': t.card.align === 'left' ? 'flex-start' : 'center',
    '--card-width': t.card.width, '--card-min': t.card.minHeight, '--card-border': t.card.border, '--card-shadow': t.card.shadow,
    '--card-art': t.card.art, '--art-mask': t.card.artMask, '--art-opacity': t.card.artOpacity,
    '--frame-inset': t.card.frame.inset, '--frame-border': t.card.frame.border, '--frame-shadow': t.card.frame.shadow,
    '--rule-width': t.text.rule.width, '--rule-height': t.text.rule.height, '--rule-style': t.text.rule.style, '--rule-color': color(t.text.rule.color),
    '--backdrop': t.page.backdrop, '--backdrop-ink': t.page.ink, '--backdrop-muted': t.page.muted,
  };
  for (const role of roles) {
    const r = t.text[role];
    Object.assign(vars, {
      [`--${role}-font`]: font(r.font), [`--${role}-size`]: r.size, [`--${role}-weight`]: r.weight, [`--${role}-style`]: r.style,
      [`--${role}-case`]: r.case, [`--${role}-spacing`]: r.spacing, [`--${role}-line`]: r.lineHeight, [`--${role}-color`]: color(r.color),
    });
  }
  return vars;
}

function loadFonts(list) {
  const families = [...new Set(list.flatMap(t => t.fonts.load))];
  if (!families.length) {
    return;
  }
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = 'https://fonts.googleapis.com/css2?' + families.map(f => 'family=' + f.replaceAll(' ', '+')).join('&') + '&display=swap';
  document.head.append(link);
}

export function addThemes(list) {
  const fresh = list.filter(t => !all.some(x => x.id === t.id));
  all = [...all, ...fresh];
  loadFonts(fresh);
}

export async function loadThemes(ids) {
  const wanted = [...new Set(ids.filter(id => id && !all.some(t => t.id === id)))];
  if (!wanted.length) {
    return;
  }
  const res = await fetch('/open/themes?ids=' + wanted.join(','));
  if (!res.ok) {
    throw new Error(`themes: ${res.status}`);
  }
  const {themes: list} = await res.json();
  addThemes(list.map(t => ({...t.design, addedBy: t.added_by})));
}

export function forgetThemes(ids) {
  all = all.filter(t => !ids.includes(t.id));
}

export async function refreshThemes(ids) {
  forgetThemes(ids);
  await loadThemes(ids);
}

export function themes() {
  return all;
}

export function themeOf(id) {
  const t = all.find(x => x.id === id);
  if (!t) {
    throw new Error(`theme ${id} is not loaded`);
  }
  return t;
}

export function fontChoices() {
  const seen = new Map();
  for (const t of all) {
    for (const stack of [t.fonts.display, t.fonts.body]) {
      if (!seen.has(stack)) {
        seen.set(stack, stack.split(',')[0].replaceAll('\'', '').trim());
      }
    }
  }
  return [...seen].map(([stack, name]) => ({stack, name}));
}

function picture(url) {
  return `url("${url}") center / cover no-repeat`;
}

export function lookTheme(look) {
  const t = merged(themeOf(look.theme), look.design || {});
  const over = {};
  if (look.cardPicture) {
    over.card = {art: picture(look.cardPicture), artMask: 'none', artOpacity: '1'};
  }
  if (look.backdrop) {
    over.page = {backdrop: picture(look.backdrop)};
  }
  return merged(t, over);
}

function setVars(node, t) {
  for (const [name, value] of Object.entries(cssVars(t))) {
    node.style.setProperty(name, value);
  }
  return node;
}

export function themed(node, look) {
  return setVars(node, lookTheme(look));
}

function stickerOf(look) {
  return themeOf(look.seal || look.envelope || look.theme);
}

export function wrapped(node, look) {
  const env = themeOf(look.envelope || look.theme);
  const liner = themeOf(look.liner || look.envelope || look.theme);
  return setVars(node, merged(env, {envelope: {liner: liner.envelope.liner}, seal: stickerOf(look).seal}));
}

export function wordless(look) {
  return Boolean(look.cardPicture) && look.design?.card?.words === false;
}

const sidebarVars = ['--side-bg', '--side-ink', '--side-active'];

export function sidebarIn(look) {
  const body = document.body.style;
  document.body.classList.toggle('is-themed', Boolean(look));
  if (!look) {
    for (const name of sidebarVars) {
      body.removeProperty(name);
    }
    return;
  }
  const t = themeOf(look.theme);
  const page = look.design?.page?.backdrop;
  if (page) {
    body.setProperty('--side-bg', `linear-gradient(180deg, color-mix(in srgb, ${page} 28%, #000), color-mix(in srgb, ${page} 48%, #000))`);
    body.setProperty('--side-ink', '#ffffff');
    body.setProperty('--side-active', 'rgba(255, 255, 255, 0.14)');
    return;
  }
  body.setProperty('--side-bg', t.sidebar.background);
  body.setProperty('--side-ink', t.sidebar.ink);
  body.setProperty('--side-active', t.sidebar.active);
}

export function sealMark(look, name) {
  const t = stickerOf(look);
  if (t.seal.picture !== 'none') {
    return '';
  }
  if (t.seal.text !== 'initial') {
    return t.seal.text;
  }
  const letter = (name || '').trim().match(/[A-Za-z0-9]/);
  return letter ? letter[0].toUpperCase() : '✦';
}

export function kickerWords(look) {
  return lookTheme(look).text.kicker.words;
}
