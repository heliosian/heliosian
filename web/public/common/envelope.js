import {el, svg} from '/elements.js';
import {andList} from '/andlist.js';
import {themed, wrapped, wordless, sealMark, kickerWords, lookTheme} from '/themes.js';

export function dayIn(look, start) {
  const [year, month, day] = (start || '').slice(0, 10).split('-').map(Number);
  if (!year || !month || !day) {
    return 'Date to come';
  }
  const date = new Date(year, month - 1, day);
  const name = options => date.toLocaleDateString('en-US', options);
  const parts = {weekday: name({weekday: 'long'}), wkd: name({weekday: 'short'}), month: name({month: 'long'}), mon: name({month: 'short'}), day: String(day), year: String(year)};
  return lookTheme(look).text.day.format.replace(/\{([^{}]*)\}/g, (all, part) => parts[part] || all);
}

export function splitAddress(location) {
  const comma = location.indexOf(',');
  if (comma < 1) {
    return [location, ''];
  }
  return [location.slice(0, comma), location.slice(comma + 1).trim()];
}

function placeLine(words) {
  const place = el('p', 'card-place');
  const [name, address] = words.place ? [words.place, words.location] : splitAddress(words.location);
  place.append(el('span', 'card-place-name', name));
  if (address) {
    place.append(el('span', 'card-place-address', address));
  }
  return place;
}

function detailParts(words) {
  return {
    when: [el('p', 'card-day', dayIn(words.look, words.start)), ...(words.hours ? [el('p', 'card-hours', words.hours)] : [])],
    where: words.place || words.location ? [placeLine(words)] : [],
  };
}

function sideBySide({when, where}) {
  const row = el('div', 'card-details');
  for (const [icon, nodes] of [['calendar', when], ['map', where]]) {
    if (!nodes.length) {
      continue;
    }
    const column = el('div', 'card-detail');
    column.append(svg(icon), ...nodes);
    row.append(column);
  }
  return row;
}

function shell(look, className) {
  const env = wrapped(el('div', `envelope ${className || ''}`), look);
  env.append(el('div', 'env-back'), el('div', 'env-peek'), el('div', 'env-pocket'));
  const flap = el('div', 'env-flap');
  flap.append(el('div', 'env-flap-out'), el('div', 'env-flap-in'));
  env.append(flap);
  return env;
}

export function miniEnvelope(look, name) {
  const env = shell(look, 'envelope-mini');
  env.append(el('span', 'env-seal', sealMark(look, name)));
  return env;
}

export function sealedEnvelope(look, name, to, onOpen) {
  const stage = themed(el('div', 'envelope-stage'), look);
  const env = shell(look);
  const peek = themed(env.querySelector('.env-peek'), look);
  peek.append(el('span', 'env-peek-kicker', kickerWords(look)), el('span', 'env-peek-title', name));
  const seal = el('button', 'env-seal', sealMark(look, name));
  seal.type = 'button';
  seal.setAttribute('aria-label', 'Open the invitation');
  env.append(seal);
  const address = el('p', 'env-address');
  address.append(el('span', 'env-to', to), el('span', 'env-hint', 'Tap the seal to open'));
  stage.append(env, address);
  const open = () => {
    if (stage.classList.contains('is-opening')) {
      return;
    }
    stage.classList.add('is-opening');
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) {
      onOpen();
      return;
    }
    setTimeout(() => onOpen(peek.getBoundingClientRect()), 1650);
  };
  seal.addEventListener('click', open);
  env.addEventListener('click', open);
  return stage;
}

export function hostLine(hostedBy, hosts) {
  return hostedBy || andList(hosts);
}

export function inviteCard(words) {
  const card = themed(el('article', 'invite-card'), words.look);
  const art = el('div', 'card-art');
  art.setAttribute('aria-hidden', 'true');
  const body = el('div', 'card-body');
  if (words.cancelled) {
    body.append(el('p', 'card-cancelled', 'This event has been cancelled'));
  }
  body.append(el('p', 'card-kicker', kickerWords(words.look)), el('h1', 'card-title', words.name));
  if (words.subtitle) {
    body.append(el('p', 'card-subtitle', words.subtitle));
  }
  body.append(el('div', 'card-rule'));
  const parts = detailParts(words);
  if (lookTheme(words.look).card.details === 'side-by-side') {
    body.append(sideBySide(parts));
  } else {
    body.append(...parts.when, ...parts.where);
  }
  if (words.description) {
    body.append(el('p', 'card-message', words.description));
  }
  const hosts = hostLine(words.hostedBy, words.hosts);
  if (hosts) {
    body.append(el('p', 'card-hosts', 'Hosted by ' + hosts));
  }
  if (!wordless(words.look)) {
    card.append(art, body);
    return card;
  }
  card.classList.add('is-wordless');
  const img = el('img', 'card-picture');
  img.src = words.look.cardPicture;
  img.alt = '';
  body.classList.add('card-words-hidden');
  card.append(img, body);
  return card;
}

export function whereWords(w) {
  return [w.place, w.location].filter(Boolean).join(', ');
}

export function mapQuery(w) {
  return w.location || w.place;
}
